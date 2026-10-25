package scene

import (
	"reflect"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Replication is actor-owned. The transport may refuse an event, but never waits
// for socket progress. Initial entity rows are references, not encoded packets.
const replicationTickBudget = 32
const replicationDeltaBudget = 2048
const replicationWorkByteBudget = 512 << 10
const replicationQueueLimit = 16384
const replicationByteLimit = 4 << 20
const replicationSnapshotLimit = 65536
const replicationStallTimeout = 30 * time.Second

type pacedEventSink interface {
	TryEmit(event.Event) bool
	CancelPendingEmit()
}

// Epoch cancellation is distinct from retrying one refused atomic Spawn: only
// a real world reset/handoff may discard admitted but unwritten world frames.
type replicationEpochSink interface{ CancelReplicationEpoch() }

func cancelReplicationSink(sink event.Sink) {
	if epoch, ok := sink.(replicationEpochSink); ok {
		epoch.CancelReplicationEpoch()
		return
	}
	if paced, ok := sink.(pacedEventSink); ok {
		paced.CancelPendingEmit()
	}
}

type replicationItem struct {
	ev       event.Event
	entity   *entity.Entity
	spawn    bool
	force    bool
	bytes    int
	refs     []*entity.Entity
	queuedAt time.Time
}
type replicationSpawnPresentation struct {
	entity           *entity.Entity
	fresh, freshDrop bool
}

type replicationStateKey struct {
	id   domain.EntityID
	kind uint8
}

type replicationStream struct {
	coalesced            map[replicationStateKey]uint64
	queueBase            uint64
	normalUsed, examined int
	workBytes            int
	presentation         map[domain.EntityID]replicationSpawnPresentation
	owner                *entity.Entity
	known                map[domain.EntityID]*entity.Entity
	planned              map[domain.EntityID]*entity.Entity
	pinned               map[domain.EntityID]*entity.Entity
	removing             map[domain.EntityID]*entity.Entity
	initial              []*entity.Entity
	initialAt            int
	initialSince         time.Time
	queue                []replicationItem
	bytes                int
	ready                bool
	initialReady         bool
	headPending          bool
	budgetTick           domain.Tick
	used                 int
	blockedSince         time.Time
	closed               bool
}

func (s *Scene) replicationFor(p *entity.Entity) *replicationStream {
	if s.replication == nil {
		s.replication = make(map[domain.EntityID]*replicationStream)
	}
	r := s.replication[p.ID]
	if r != nil && r.owner == p {
		return r
	}
	r = &replicationStream{coalesced: make(map[replicationStateKey]uint64), presentation: make(map[domain.EntityID]replicationSpawnPresentation), owner: p, known: make(map[domain.EntityID]*entity.Entity), planned: make(map[domain.EntityID]*entity.Entity), pinned: make(map[domain.EntityID]*entity.Entity), removing: make(map[domain.EntityID]*entity.Entity)}
	s.replication[p.ID] = r
	return r
}
func (s *Scene) localRiftEntity(e *entity.Entity) bool {
	return s.rift != nil && e != nil && e.Kind == domain.KindMonster
}
func (s *Scene) ensureRiftVisibility() {
	if s.rift == nil || s.riftVisibility != nil {
		return
	}
	s.riftVisibility = newRiftBlockIndex(s.nativeMapWidth, s.nativeMapHeight, s.riftMembershipChanged)
	for _, e := range s.monsters {
		s.riftVisibility.updateEntity(e, true)
	}
	s.aoi.onChange = func(e *entity.Entity, present bool) {
		if present && s.entities[e.ID] != e {
			return
		}
		if e.Kind == domain.KindMonster {
			s.riftVisibility.updateEntity(e, present)
		}
		if e.Kind == domain.KindPlayer {
			if r := s.replication[e.ID]; r != nil && r.ready {
				if present {
					s.riftVisibility.subscribe(e.ID, e.Pos, s.replicationMovementBuffer(e))
				} else {
					s.riftVisibility.unsubscribe(e.ID)
				}
			}
		}
	}
}
func (s *Scene) riftMembershipChanged(to domain.EntityID, e *entity.Entity, present bool) {
	r := s.replication[to]
	if r == nil || !r.ready || r.closed {
		return
	}
	if present {
		s.queueReplicationSpawn(r, e, false)
		return
	}
	if r.pinned[e.ID] == e && s.riftVisibility.entities[e.ID].entity == e {
		return
	}
	s.replicationLeaves = append(s.replicationLeaves, replicationLeave{to: to, entity: e})
}
func (s *Scene) replicationVisible(r *replicationStream, e *entity.Entity) bool {
	return !s.localRiftEntity(e) || r.pinned[e.ID] == e || s.riftVisibility != nil && s.riftVisibility.visible(r.owner.ID, e)
}
func (s *Scene) queueReplicationSpawn(r *replicationStream, e *entity.Entity, force bool) {
	if e == nil || e.ID == r.owner.ID || r.closed || r.planned[e.ID] == e {
		return
	}
	if s.localRiftEntity(e) && !e.Alive() {
		return
	}
	if force {
		r.pinned[e.ID] = e
	}
	if !force && !s.replicationVisible(r, e) {
		return
	}
	if old := r.planned[e.ID]; old != nil && old != e {
		s.replicationLeaves = append(s.replicationLeaves, replicationLeave{to: r.owner.ID, entity: old})
	}
	r.planned[e.ID] = e
	if force {
		s.appendReplication(r, replicationItem{entity: e, spawn: true, force: true, bytes: 128})
	} else {
		if len(r.initial)-r.initialAt >= replicationSnapshotLimit {
			s.failReplication(r, "snapshot reference limit")
			return
		}
		if r.initialAt > 1024 {
			r.initial = append([]*entity.Entity(nil), r.initial[r.initialAt:]...)
			r.initialAt = 0
		}
		if len(r.initial) == 0 {
			r.initialSince = time.Now()
		}
		r.initial = append(r.initial, e)
	}
}
func (s *Scene) queueReplicationRemove(r *replicationStream, e *entity.Entity, reason event.DespawnReason) {
	if e == nil || r.closed {
		return
	}
	if r.planned[e.ID] != e && r.known[e.ID] != e {
		return
	}
	if r.removing[e.ID] == e {
		return
	}
	r.removing[e.ID] = e
	if r.presentation[e.ID].entity == e {
		delete(r.presentation, e.ID)
	}
	if r.planned[e.ID] == e {
		delete(r.planned, e.ID)
	}
	if r.pinned[e.ID] == e {
		delete(r.pinned, e.ID)
	}
	s.appendReplication(r, replicationItem{ev: event.EntityDespawned{ID: e.ID, Kind: e.Kind, Reason: reason}, entity: e, bytes: 128})
}
func (s *Scene) failReplication(r *replicationStream, why string) {
	if r.closed {
		return
	}
	r.closed = true
	s.log.Warn("replication backpressure limit", "entity", r.owner.ID, "reason", why)
	r.owner.Player.Sink.Close()
	cancelReplicationSink(r.owner.Player.Sink)
	clear(r.queue)
	r.queue = nil
	clear(r.initial)
	r.initial = nil
	r.initialAt = 0
	r.bytes = 0
	clear(r.planned)
	clear(r.known)
	clear(r.pinned)
	clear(r.removing)
	clear(r.coalesced)
	clear(r.presentation)
}
func (s *Scene) appendReplication(r *replicationStream, item replicationItem) {
	if r.closed {
		return
	}
	if item.queuedAt.IsZero() {
		item.queuedAt = time.Now()
	}
	if item.bytes == 0 {
		item.bytes = replicationRetainedBytes(item.ev)
	}
	key, state := replicationState(item.ev)
	if state {
		if seq, ok := r.coalesced[key]; ok && seq >= r.queueBase {
			at := int(seq - r.queueBase)
			if at < len(r.queue) && !(at == 0 && r.headPending) {
				old := r.queue[at]
				item.queuedAt = old.queuedAt
				r.bytes += item.bytes - old.bytes
				r.queue[at] = item
				if r.bytes > replicationByteLimit {
					s.failReplication(r, "event byte limit")
				}
				return
			}
		}
	} else {
		// Any critical reference is a per-entity ordering barrier. Other entities'
		// movements may still coalesce across it without changing that relationship.
		ids := replicationReferences(item.ev)
		if item.entity != nil {
			ids = append(ids, item.entity.ID)
		}
		for _, id := range ids {
			delete(r.coalesced, replicationStateKey{id, 1})
			delete(r.coalesced, replicationStateKey{id, 2})
		}
	}
	if len(r.queue) >= replicationQueueLimit {
		s.failReplication(r, "event count limit")
		return
	}
	if state {
		r.coalesced[key] = r.queueBase + uint64(len(r.queue))
	}
	r.queue = append(r.queue, item)
	r.bytes += item.bytes

	if r.bytes > replicationByteLimit {
		s.failReplication(r, "event byte limit")
	}
}
func replicationState(ev event.Event) (replicationStateKey, bool) {
	switch e := ev.(type) {
	case event.EntityMoved:
		return replicationStateKey{e.ID, 1}, true
	case event.EntityStatusSnapshot:
		return replicationStateKey{e.Who, 2}, true
	}
	return replicationStateKey{}, false
}

// Include every native entity reference, not just Subject (which is frequently
// the victim). Globals such as rift.state, including its full Aura list, bypass
// this filter and retain their existing protocol/pagination.
func replicationStagedEvent(ev event.Event) bool {
	if worldEntityEvent(ev) {
		return true
	}
	switch ev.(type) {
	case replicationBarrier, event.GroundEffect, event.ChildSkillReleased, event.TargetStatusSnapshot, event.EntityOwnerChanged, event.EntityEffectRequested:
		return true
	}
	return false
}
func replicationReferences(ev event.Event) []domain.EntityID {
	if ev == nil {
		return nil
	}
	switch e := ev.(type) {
	case event.DamageDealt:
		return []domain.EntityID{e.Src, e.Dst}
	case event.HealDone:
		return []domain.EntityID{e.Dst} // native heal number uses recipient anchor, not source ID
	case event.SkillCastChanged:
		return []domain.EntityID{e.Caster, e.Target}
	case event.ChildSkillReleased:
		return []domain.EntityID{e.Caster, e.Target}
	case event.EntityDied:
		return []domain.EntityID{e.ID} // native life packet does not encode Killer
	case event.TargetStatusSnapshot:
		return []domain.EntityID{e.Target}
	case event.EntityOwnerChanged:
		return []domain.EntityID{e.Who, e.Owner}
	case event.RidingChanged:
		return []domain.EntityID{e.Who, e.Anchor}
	case event.EntityEffectRequested:
		return []domain.EntityID{e.Who}
	}
	if worldEntityEvent(ev) {
		return []domain.EntityID{ev.Subject()}
	}
	return nil
}
func (s *Scene) replicationEventRefs(ev event.Event) (refs [2]*entity.Entity) {
	for n, id := range replicationReferences(ev) {
		if n < len(refs) {
			refs[n] = s.entities[id]
		}
	}
	return
}

type replicationBarrier struct{ who domain.EntityID }

func (e replicationBarrier) Subject() domain.EntityID { return e.who }

func (s *Scene) deliverReplication(p *entity.Entity, ev event.Event, captured [2]*entity.Entity) {
	_, paced := p.Player.Sink.(pacedEventSink)
	if !paced && s.rift == nil {
		p.Player.Sink.Emit(ev)
		return
	}
	if s.replicationDeparting[p.ID] && replicationStagedEvent(ev) {
		return
	}
	if !replicationStagedEvent(ev) {
		p.Player.Sink.Emit(ev)
		return
	}
	r := s.replicationFor(p)
	if _, barrier := ev.(replicationBarrier); barrier {
		s.appendReplication(r, replicationItem{ev: ev, bytes: 128})
		return
	}
	if r.closed {
		return
	}
	lookup := func(id domain.EntityID) *entity.Entity {
		for _, e := range captured {
			if e != nil && e.ID == id {
				return e
			}
		}
		return nil
	}
	if spawn, ok := ev.(event.EntitySpawned); ok {
		e := lookup(spawn.ID)
		if e == nil || s.entities[e.ID] != e {
			return
		}
		style := replicationSpawnPresentation{entity: e, fresh: spawn.Fresh}
		if spawn.Ground != nil {
			style.freshDrop = spawn.Ground.FreshDrop
		}
		r.presentation[e.ID] = style
		s.queueReplicationSpawn(r, e, !s.localRiftEntity(e))
		return
	}
	if gone, ok := ev.(event.EntityDespawned); ok {
		old := lookup(gone.ID)
		if old == nil {
			old = r.known[gone.ID]
		}
		if old != nil && (r.planned[gone.ID] == old || r.known[gone.ID] == old) {
			s.queueReplicationRemove(r, old, gone.Reason)
			return
		}
		// Non-local entities may have been sent before this stream was introduced.
		if gone.Kind == domain.KindMonster && s.rift != nil {
			return
		}
		s.appendReplication(r, replicationItem{ev: ev})
		return
	}
	if dead, ok := ev.(event.EntityDied); ok && dead.ID != p.ID && r.known[dead.ID] == nil {
		return
	}
	ids := replicationReferences(ev)
	subject := lookup(ev.Subject())
	if subject == nil {
		subject = r.known[ev.Subject()]
	}
	if s.localRiftEntity(subject) && !s.replicationVisible(r, subject) {
		relevant := false
		for _, id := range ids {
			if id == p.ID {
				relevant = true
			}
		}
		if !relevant {
			return
		}
	}
	if s.riftVisibility != nil {
		hasLocal, relevant := false, false
		for _, id := range ids {
			e := lookup(id)
			if s.localRiftEntity(e) {
				hasLocal = true
			}
			if id == p.ID || e != nil && s.riftVisibility.visible(p.ID, e) {
				relevant = true
			}
		}
		if _, targetQuery := ev.(event.TargetStatusSnapshot); targetQuery {
			relevant = true
		}
		if hit, ok := ev.(event.DamageDealt); ok {
			if target := lookup(hit.Dst); target != nil && !s.localRiftEntity(target) {
				relevant = true
			}
		}
		if hasLocal && !relevant {
			return
		}
	}
	item := replicationItem{ev: ev}
	for _, id := range ids {
		e := lookup(id)
		if e == nil || e.ID == p.ID {
			continue
		}
		// Unseen routine state is represented by the current lazy Spawn snapshot.
		if r.known[id] != e || r.removing[id] == e {
			switch ev.(type) {
			case event.EntityMoved, event.EntityStatusSnapshot, event.EntityStatusEffect, event.MonsterSpoke, event.EntityDied:
				return
			case event.DamageDealt:
				if ev.Subject() == id {
					return
				}
			}
			if !e.Alive() {
				return
			}
			// A pending initial row must precede a critical reference, so move it to
			// the critical FIFO without marking it known before transport acceptance.
			if r.planned[id] == e {
				delete(r.planned, id)
			}
			s.queueReplicationSpawn(r, e, true)
		}
		item.refs = append(item.refs, e)
	}
	s.appendReplication(r, item)
}

func (s *Scene) cancelReplication(to domain.EntityID) {
	kept := s.replicationLeaves[:0]
	for _, leave := range s.replicationLeaves {
		if leave.to != to {
			kept = append(kept, leave)
		}
	}
	clear(s.replicationLeaves[len(kept):])
	s.replicationLeaves = kept
	r := s.replication[to]
	if r == nil {
		return
	}
	cancelReplicationSink(r.owner.Player.Sink)
	delete(s.replication, to)
	if s.riftVisibility != nil {
		s.riftVisibility.unsubscribe(to)
	}
	clear(r.queue)
	clear(r.initial)
	clear(r.known)
	clear(r.planned)
	clear(r.pinned)
	clear(r.removing)
	clear(r.coalesced)
	clear(r.presentation)
}

type replicationLeave struct {
	to     domain.EntityID
	entity *entity.Entity
}

func (s *Scene) drainReplication() {
	for _, leave := range s.replicationLeaves {
		if r := s.replication[leave.to]; r != nil {
			s.queueReplicationRemove(r, leave.entity, event.DespawnOutOfSight)
			if e := s.entities[leave.entity.ID]; e != nil && s.replicationVisible(r, e) {
				s.queueReplicationSpawn(r, e, false)
			}
		}
	}
	clear(s.replicationLeaves)
	s.replicationLeaves = nil
	for id, r := range s.replication {
		if s.players[id] != r.owner {
			delete(s.replication, id)
			continue
		}
		if r.closed {
			continue
		}

		if len(r.queue) > 0 && !r.queue[0].queuedAt.IsZero() && time.Since(r.queue[0].queuedAt) > replicationStallTimeout {
			s.failReplication(r, "event backlog age limit")
			continue
		}
		sink, paced := r.owner.Player.Sink.(pacedEventSink)
		if r.budgetTick != s.tick {
			r.budgetTick = s.tick
			r.used = 0
			r.normalUsed = 0
			r.examined = 0
			r.workBytes = 0
		}
		budget := replicationTickBudget
		normalBudget := replicationDeltaBudget
		if !paced {
			budget = 1 << 20
			normalBudget = budget
		}
		for (r.used < budget || r.normalUsed < normalBudget) && r.examined < (budget+normalBudget)*4 {
			r.examined++
			// Reserve every fourth slot for initial progress; a captured FIFO head
			// must finish first because the sink retains exactly that encoded event.
			fromInitial := r.initialReady && r.used < budget && r.initialAt < len(r.initial) && (len(r.queue) == 0 || !r.headPending && (r.normalUsed%4 == 3 || r.normalUsed >= normalBudget))
			if fromInitial {
				e := r.initial[r.initialAt]
				if e != nil && r.removing[e.ID] != nil && len(r.queue) > 0 {
					fromInitial = false
				}
			}
			var item replicationItem
			if fromInitial {
				e := r.initial[r.initialAt]
				item = replicationItem{entity: e, spawn: true}
			} else if len(r.queue) > 0 && r.normalUsed < normalBudget {
				item = r.queue[0]
			} else {
				break
			}
			valid := true
			if item.spawn {
				e := item.entity
				valid = e != nil && s.entities[e.ID] == e && r.planned[e.ID] == e && r.known[e.ID] != e && s.replicationVisible(r, e) && (!s.localRiftEntity(e) || e.Alive())
				if valid {
					spawn := s.spawnEvent(e)
					if style := r.presentation[e.ID]; style.entity == e {
						spawn.Fresh = style.fresh
						if spawn.Ground != nil {
							spawn.Ground.FreshDrop = style.freshDrop
						}
					}
					item.ev = s.withNativeAttackRange(spawn)
				}
			} else if item.entity != nil {
				valid = r.known[item.entity.ID] == item.entity
			} else {
				for _, e := range item.refs {
					if r.known[e.ID] != e {
						valid = false
						break
					}
				}
			}
			if !valid {
				if !item.spawn && item.entity != nil && r.removing[item.entity.ID] == item.entity {
					delete(r.removing, item.entity.ID)
				}
				if r.headPending && !fromInitial && paced {
					sink.CancelPendingEmit()
					r.headPending = false
				}
			} else {
				cost := item.bytes
				if cost == 0 {
					cost = replicationRetainedBytes(item.ev)
				}
				// Bound per-tick encoding work as well as event count. One existing large
				// global event is allowed alone; its transport prefix remains <=64KiB.
				if paced && r.workBytes > 0 && r.workBytes+cost > replicationWorkByteBudget {
					break
				}
				r.workBytes += cost
				if fromInitial {
					r.used++
				} else {
					r.normalUsed++
				}
				accepted := true
				if _, barrier := item.ev.(replicationBarrier); barrier {
					r.initialReady = true
				} else if paced {
					accepted = sink.TryEmit(item.ev)
				} else {
					r.owner.Player.Sink.Emit(item.ev)
				}
				if !accepted {
					if item.spawn { // Atomic spawn was not accepted: regenerate fresh pose/HP/status next tick.
						sink.CancelPendingEmit()
					} else {
						r.headPending = true
					}
					if r.blockedSince.IsZero() {
						r.blockedSince = time.Now()
					}
					if time.Since(r.blockedSince) > replicationStallTimeout {
						s.failReplication(r, "no transport progress")
					}
					break
				}
				r.headPending = false
				r.blockedSince = time.Time{}
				if item.spawn {
					r.known[item.entity.ID] = item.entity
					delete(r.presentation, item.entity.ID)
					// Status icons are in Spawn; persistent native effects require their own events.
					for _, st := range s.visibleStatuses(item.entity) {
						if st.effect.Effect != "" {
							s.appendReplication(r, replicationItem{ev: st.effect, refs: []*entity.Entity{item.entity}})
						}
					}
				} else if item.entity != nil {
					if r.known[item.entity.ID] == item.entity {
						delete(r.known, item.entity.ID)
					}
					if r.removing[item.entity.ID] == item.entity {
						delete(r.removing, item.entity.ID)
					}
				}
			}
			if fromInitial {
				r.initial[r.initialAt] = nil
				r.initialAt++
			} else {
				r.bytes -= r.queue[0].bytes
				if key, ok := replicationState(r.queue[0].ev); ok {
					if seq, exists := r.coalesced[key]; exists && seq == r.queueBase {
						delete(r.coalesced, key)
					}
				}
				r.queueBase++
				r.queue[0] = replicationItem{}
				r.queue = r.queue[1:]
			}
			if r.initialAt == len(r.initial) {
				r.initial = nil
				r.initialAt = 0
			}
		}
	}
}

// Bound retained event payloads, including nested Aura/inventory slices. This is
// only the staging path, not a protocol estimate. Cycles/shared pointers count
// once; a traversal cap fails closed rather than making admission unbounded.
func replicationRetainedBytes(ev event.Event) int {
	seen := map[uintptr]bool{}
	visits := 0
	var extra func(reflect.Value) int
	extra = func(v reflect.Value) int {
		if !v.IsValid() {
			return 0
		}
		visits++
		if visits > 65536 {
			return replicationByteLimit + 1
		}
		n := 0
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				x := v.Elem()
				n = int(x.Type().Size()) + extra(x)
			}
		case reflect.Pointer:
			if !v.IsNil() && !seen[v.Pointer()] {
				seen[v.Pointer()] = true
				x := v.Elem()
				n = int(x.Type().Size()) + extra(x)
			}
		case reflect.String:
			n = v.Len()
		case reflect.Slice:
			n = v.Cap() * int(v.Type().Elem().Size())
			for j := 0; j < v.Len() && n <= replicationByteLimit; j++ {
				n += extra(v.Index(j))
			}
		case reflect.Array:
			for j := 0; j < v.Len() && n <= replicationByteLimit; j++ {
				n += extra(v.Index(j))
			}
		case reflect.Struct:
			for j := 0; j < v.NumField() && n <= replicationByteLimit; j++ {
				n += extra(v.Field(j))
			}
		case reflect.Map:
			n = v.Len() * 32
			it := v.MapRange()
			for it.Next() && n <= replicationByteLimit {
				n += int(v.Type().Key().Size()+v.Type().Elem().Size()) + extra(it.Key()) + extra(it.Value())
			}
		}
		return n
	}
	v := reflect.ValueOf(ev)
	if !v.IsValid() {
		return 128
	}
	return 128 + int(v.Type().Size()) + extra(v)
}

func (s *Scene) retireReplicationWorld(r *replicationStream) {
	r.ready = false
	r.initialReady = false
	cancelReplicationSink(r.owner.Player.Sink)
	r.headPending = false
	clear(r.queue)
	r.queue = nil
	r.bytes = 0
	r.queueBase = 0
	clear(r.initial)
	r.initial = nil
	r.initialAt = 0
	clear(r.planned)
	clear(r.removing)
	clear(r.coalesced)
	clear(r.presentation)
	clear(r.pinned)
	if s.riftVisibility != nil {
		s.riftVisibility.unsubscribe(r.owner.ID)
	}
}

// A native same-map rebuild may already have happened before its ready receipt.
// Accepted old frames cannot be revoked: explicitly remove every admitted row
// before the replacement snapshot, including rows outside the new block window.
func (s *Scene) resetReplicationSnapshot(r *replicationStream) {
	s.retireReplicationWorld(r)
	for _, e := range r.known {
		s.queueReplicationRemove(r, e, event.DespawnOutOfSight)
	}
}
