package scene

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// This sink models admission, not delivery. A refused event must be retried,
// never counted as known by the scene. The real TCP boundary is tested in net.
type replicationTestSink struct {
	accepted  []event.Event
	allowance int
	closed    bool
	bypass    int
	cancels   int
}

func (q *replicationTestSink) Emit(e event.Event) {
	if replicationStagedEvent(e) {
		q.bypass++
	}
	q.accepted = append(q.accepted, e)
}
func (q *replicationTestSink) TryEmit(e event.Event) bool {
	if q.closed || q.allowance <= 0 {
		return false
	}
	q.allowance--
	q.accepted = append(q.accepted, e)
	return true
}
func (q *replicationTestSink) CancelPendingEmit()  { q.cancels++ }
func (q *replicationTestSink) Close()              { q.closed = true }
func (q *replicationTestSink) Closed() bool        { return q.closed }
func (q *replicationTestSink) take() []event.Event { out := q.accepted; q.accepted = nil; return out }

func replicationScene(t *testing.T, players int, rift bool) (*Scene, []*replicationTestSink) {
	t.Helper()
	s := newTestScene(t, nil)
	sinks := make([]*replicationTestSink, players)
	for i := range sinks {
		id := domain.EntityID(i + 1)
		join(t, s, id, int64(100+i), fmt.Sprintf("observer-%d", i), 6000, 6000)
	}
	s.flush()
	for i := range sinks {
		sinks[i] = &replicationTestSink{allowance: 1024}
		s.players[domain.EntityID(i+1)].Player.Sink = sinks[i]
	}
	if rift {
		s.rift = &endgameRiftState{}
	}
	return s, sinks
}
func replicationTick(s *Scene, sinks []*replicationTestSink, allowance int) {
	s.tick++
	for _, q := range sinks {
		q.allowance = allowance
	}
	s.flush()
}
func replicationMonster(s *Scene, id domain.EntityID, hp int32, stats domain.Stats, x, y float64) *entity.Entity {
	e := putMonster(s, id, hp, stats, x, y)
	s.monsters[id] = e
	return e
}
func replicationMonsters(s *Scene, n int, x, y float64) []*entity.Entity {
	out := make([]*entity.Entity, n)
	for i := range out {
		out[i] = replicationMonster(s, domain.EntityID(100+i), 100, domain.Stats{}, x, y)
	}
	return out
}
func replicationSpawns(evs []event.Event) map[domain.EntityID]event.EntitySpawned {
	out := make(map[domain.EntityID]event.EntitySpawned)
	for _, ev := range evs {
		if e, ok := ev.(event.EntitySpawned); ok && e.Kind == domain.KindMonster {
			out[e.ID] = e
		}
	}
	return out
}

func TestReplicationDenseInitialIsBoundedNonblocking(t *testing.T) {
	for _, n := range []int{1160, 4097} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s, qs := replicationScene(t, 1, true)
			replicationMonsters(s, n, 6000, 6000)
			s.sendAllEntities(1)
			start := time.Now()
			s.flush()
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("snapshot blocked actor for %v", elapsed)
			}
			if got := len(qs[0].accepted); got > 32 {
				t.Fatalf("initial burst %d exceeds tick budget", got)
			}
			total := replicationSpawns(qs[0].take())
			for tick := 0; tick < n+100 && len(total) < n; tick++ {
				replicationTick(s, qs, 7) // slower than producer, but making steady progress
				for id, sp := range replicationSpawns(qs[0].take()) {
					if _, dup := total[id]; dup {
						t.Fatalf("duplicate initial spawn %d", id)
					}
					total[id] = sp
				}
			}
			if len(total) != n {
				t.Fatalf("accepted %d/%d entities", len(total), n)
			}
			if qs[0].closed || qs[0].bypass != 0 {
				t.Fatalf("closed=%v bypass=%d", qs[0].closed, qs[0].bypass)
			}
		})
	}
}

func TestReplicationPressureOneViewerDoesNotBlockFivePlayers(t *testing.T) {
	s, qs := replicationScene(t, 5, true)
	replicationMonsters(s, 1160, 6000, 6000)
	for i := range qs {
		qs[i].allowance = 0
		s.sendAllEntities(domain.EntityID(i + 1))
	}
	s.flush()
	counts := make([]int, 5)
	for tick := 0; tick < 100; tick++ {
		s.tick++
		for i, q := range qs {
			if i > 0 {
				q.allowance = 64
			}
		}
		s.flush()
		for i, q := range qs {
			counts[i] += len(replicationSpawns(q.take()))
		}
	}
	if counts[0] != 0 {
		t.Fatalf("refused viewer accepted %d entities", counts[0])
	}
	for i := 1; i < 5; i++ {
		if counts[i] != 1160 {
			t.Fatalf("viewer %d starved: %d/1160", i, counts[i])
		}
	}
	for _, q := range qs {
		if q.bypass != 0 {
			t.Fatal("staging bypassed")
		}
	}
}

func TestReplicationStationaryObserverMonsterCrossesAndReenters(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	s.aoi.Move(m, domain.Pos{MapID: 7, X: 30000, Y: 30000})
	s.emit(event.EntityMoved{ID: m.ID, To: m.Pos})
	replicationTick(s, qs, 100)
	evs := qs[0].take()
	removed := false
	for _, ev := range evs {
		if e, ok := ev.(event.EntityDespawned); ok && e.ID == m.ID {
			removed = true
			if e.Reason != event.DespawnOutOfSight {
				t.Fatalf("remove reason %v", e.Reason)
			}
		}
	}
	if !removed {
		t.Fatal("stationary player did not remove moving monster")
	}
	m.HP = 47
	s.aoi.Move(m, domain.Pos{MapID: 7, X: 6000, Y: 6000})
	s.emit(event.EntityMoved{ID: m.ID, To: m.Pos})
	replicationTick(s, qs, 100)
	sp, ok := replicationSpawns(qs[0].take())[m.ID]
	if !ok || sp.HP != 47 {
		t.Fatalf("reentry must spawn current state: %+v present=%v", sp, ok)
	}
}

func TestReplicationLazySpawnUsesLatestHPAndPosition(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	qs[0].allowance = 0
	s.sendAllEntities(1)
	s.flush()
	m.HP = 37
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 91001, Level: 1, Icon: 5038, DurationSec: 30}, s.tick, 1)
	s.aoi.Move(m, domain.Pos{MapID: 7, X: 6100, Y: 6100})
	s.emit(event.EntityMoved{ID: m.ID, To: m.Pos})
	s.emit(event.DamageDealt{Src: 1, Dst: m.ID, Amount: 63, DstHP: 37})
	replicationTick(s, qs, 100)
	sp, ok := replicationSpawns(qs[0].take())[m.ID]
	if !ok || sp.HP != 37 || sp.Pos != m.Pos {
		t.Fatalf("stale pending snapshot: %+v", sp)
	}
	if len(sp.StatusIcons) != 1 || sp.StatusIcons[0].Status != 91001 || sp.StatusIcons[0].Icon != 5038 {
		t.Fatalf("latest status absent from lazy spawn: %+v", sp.StatusIcons)
	}
}

func TestReplicationCancelInitialNeverMaterializesOldIncarnation(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	old := replicationMonsters(s, 1, 6000, 6000)[0]
	qs[0].allowance = 0
	s.sendAllEntities(1)
	s.flush()
	s.cancelReplication(1)
	s.aoi.Leave(old)
	delete(s.entities, old.ID)
	fresh := replicationMonster(s, old.ID, 77, domain.Stats{}, 6200, 6200)
	s.sendAllEntities(1)
	replicationTick(s, qs, 100)
	evs := qs[0].take()
	sp, ok := replicationSpawns(evs)[fresh.ID]
	if !ok || sp.HP != 77 || sp.Pos != fresh.Pos {
		t.Fatalf("old incarnation survived cancellation: %+v", sp)
	}
	if got := countOf[event.EntitySpawned](evs); got != 1 {
		t.Fatalf("got %d incarnations", got)
	}
	if qs[0].cancels == 0 {
		t.Fatal("transport partial event was not canceled")
	}
}

func TestReplicationDeadPendingMonsterDoesNotSpawn(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	qs[0].allowance = 0
	s.sendAllEntities(1)
	s.flush()
	m.HP = 0
	s.emit(event.EntityDied{ID: m.ID, Kind: domain.KindMonster, Killer: 1})
	s.emit(event.EntityDespawned{ID: m.ID, Kind: domain.KindMonster, Reason: event.DespawnDeath})
	s.aoi.Leave(m)
	delete(s.entities, m.ID)
	replicationTick(s, qs, 100)
	for _, ev := range qs[0].take() {
		if ev.Subject() == m.ID {
			t.Fatalf("never-known dead monster leaked %T", ev)
		}
	}
}

func TestReplicationRiftKeepsGlobalNPCDropAndAllAuras(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	replicationMonsters(s, 1, 30000, 30000)
	npc := &entity.Entity{ID: 50, Kind: domain.KindNPC, Pos: domain.Pos{MapID: 7, X: 30000, Y: 30000}, NPC: &entity.NPCInfo{Sprite: "npc601"}}
	drop := &entity.Entity{ID: 51, Kind: domain.KindDrop, Pos: npc.Pos, Drop: &entity.Drop{Item: 1, Count: 1}}
	for _, e := range []*entity.Entity{npc, drop} {
		s.entities[e.ID] = e
		s.aoi.Enter(e)
	}
	auras := make([]domain.EndgameRiftAuraView, 1160)
	msg := event.EndgameMessage{Who: 1, Message: domain.EndgameResponse{Kind: "rift.state", Rift: &domain.EndgameRiftView{Auras: auras}}}
	s.emitTo(1, msg)
	s.sendAllEntities(1)
	replicationTick(s, qs, 100)
	evs := qs[0].take()
	seen := map[domain.EntityID]bool{}
	auraCount := -1
	for _, ev := range evs {
		switch e := ev.(type) {
		case event.EntitySpawned:
			seen[e.ID] = true
		case event.EndgameMessage:
			if e.Message.Rift != nil {
				auraCount = len(e.Message.Rift.Auras)
			}
		}
	}
	if seen[100] || !seen[50] || !seen[51] {
		t.Fatalf("global/local scope changed: %v", seen)
	}
	if auraCount != 1160 {
		t.Fatalf("global aura list filtered: %d", auraCount)
	}
}

func TestReplicationNonRiftStillStreamsDistantMonsters(t *testing.T) {
	s, qs := replicationScene(t, 1, false)
	replicationMonsters(s, 1, 30000, 30000)
	s.sendAllEntities(1)
	replicationTick(s, qs, 100)
	if len(replicationSpawns(qs[0].take())) != 1 {
		t.Fatal("non-Rift full-map replication regressed")
	}
}

func TestReplicationFarCombatDependenciesPrecedeReferences(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	m := replicationMonsters(s, 1, 30000, 30000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	s.emit(event.SkillCastChanged{Caster: m.ID, Target: 1, Skill: 1, Phase: event.SkillCastRelease})
	s.emit(event.DamageDealt{Src: m.ID, Dst: 1, Amount: 5, DstHP: 95})
	replicationTick(s, qs, 100)
	known := false
	cast, hit := false, false
	for _, ev := range qs[0].take() {
		switch e := ev.(type) {
		case event.EntitySpawned:
			if e.ID == m.ID {
				known = true
			}
		case event.EntityDespawned:
			if e.ID == m.ID {
				known = false
			}
		case event.SkillCastChanged:
			if e.Caster == m.ID {
				if !known {
					t.Fatal("cast references unknown far source")
				}
				cast = true
			}
		case event.DamageDealt:
			if e.Src == m.ID {
				if !known {
					t.Fatal("hit references unknown far source")
				}
				hit = true
			}
		}
	}
	if !cast || !hit {
		t.Fatalf("far authoritative combat presentation lost: cast=%v hit=%v", cast, hit)
	}
}

func TestReplicationAcceptedMonsterDeathThenRemove(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	m.HP = 0
	s.emit(event.DamageDealt{Src: 1, Dst: m.ID, Amount: 100, DstHP: 0, Flag: event.DamageFatal})
	s.emit(event.EntityDied{ID: m.ID, Kind: domain.KindMonster, Killer: 1})
	s.emit(event.EntityDespawned{ID: m.ID, Kind: domain.KindMonster, Reason: event.DespawnDeath})
	s.aoi.Leave(m)
	delete(s.entities, m.ID)
	replicationTick(s, qs, 100)
	dead, removed := false, false
	for _, ev := range qs[0].take() {
		switch e := ev.(type) {
		case event.EntityDied:
			if e.ID == m.ID {
				if removed {
					t.Fatal("death after removal")
				}
				dead = true
			}
		case event.EntityDespawned:
			if e.ID == m.ID {
				if !dead {
					t.Fatal("remove overtook death")
				}
				removed = true
			}
		}
	}
	if !dead || !removed {
		t.Fatalf("known lifecycle incomplete: dead=%v removed=%v", dead, removed)
	}
}

type replicationMapSink struct {
	*replicationTestSink
	epoch uint64
	ready bool
}

func (q *replicationMapSink) BeginMapLoad(domain.EntityID, domain.SceneID) (uint64, bool) {
	q.epoch++
	q.ready = false
	return q.epoch, true
}
func (q *replicationMapSink) CompleteMapLoad(_ domain.EntityID, _ domain.SceneID, epoch uint64) bool {
	if epoch != q.epoch {
		return false
	}
	q.ready = true
	return true
}

func TestReplicationTeleportBarrierCancelsPendingInitial(t *testing.T) {
	s, qs := replicationScene(t, 1, false)
	q := &replicationMapSink{replicationTestSink: qs[0]}
	s.players[1].Player.Sink = q
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	q.allowance = 0
	s.sendAllEntities(1)
	s.flush()
	if !s.onTeleport(Teleport{ID: 1, To: s.id, At: domain.Pos{MapID: 7, X: 9000, Y: 9000}}) {
		t.Fatal("same-map teleport rejected")
	}
	replicationTick(s, qs, 100)
	if len(replicationSpawns(q.take())) != 0 {
		t.Fatal("old snapshot crossed pending map barrier")
	}
	m.HP = 33
	s.onMapReady(MapReady{ID: 1, Scene: s.id, Epoch: q.epoch + 1})
	replicationTick(s, qs, 100)
	if len(replicationSpawns(q.take())) != 0 {
		t.Fatal("wrong epoch released snapshot")
	}
	s.onMapReady(MapReady{ID: 1, Scene: s.id, Epoch: q.epoch})
	replicationTick(s, qs, 100)
	sp, ok := replicationSpawns(q.take())[m.ID]
	if !ok || sp.HP != 33 {
		t.Fatalf("ready snapshot missing/latest state lost: %+v", sp)
	}
	if q.cancels == 0 {
		t.Fatal("map boundary did not cancel pending transport event")
	}
}

func TestReplicationRepeatedHighSpeedBlockChangesKeepKnownBounded(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	for i := 0; i < 100; i++ {
		replicationMonster(s, domain.EntityID(100+i), 100, domain.Stats{}, 6000+float64(i)*10000, 6000)
	}
	s.sendAllEntities(1)
	known := map[domain.EntityID]bool{}
	for i := 0; i < 100; i++ {
		p := s.players[1]
		s.aoi.Move(p, domain.Pos{MapID: 7, X: 6000 + float64(i)*10000, Y: 6000})
		s.emitExcept(event.EntityMoved{ID: 1, To: p.Pos}, 1)
		replicationTick(s, qs, 100)
		for _, ev := range qs[0].take() {
			switch e := ev.(type) {
			case event.EntitySpawned:
				if e.Kind == domain.KindMonster {
					known[e.ID] = true
				}
			case event.EntityDespawned:
				delete(known, e.ID)
			}
		}
		if len(known) > 2 {
			t.Fatalf("known entities grew with travel at block %d: %d", i, len(known))
		}
		if !known[domain.EntityID(100+i)] {
			t.Fatalf("high-speed arrival %d missing current block entity", i)
		}
	}
}

func TestReplicationStalledViewerTimesOutAndReleasesPendingState(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	replicationMonsters(s, 1160, 6000, 6000)
	qs[0].allowance = 0
	s.sendAllEntities(1)
	s.flush()
	r := s.replication[1]
	if r == nil || r.blockedSince.IsZero() {
		t.Fatal("refusal did not establish stalled deadline")
	}
	r.blockedSince = time.Now().Add(-replicationStallTimeout - time.Second)
	replicationTick(s, qs, 0)
	if !qs[0].closed {
		t.Fatal("permanently stalled connection retained forever")
	}
	if len(r.queue) != 0 || len(r.initial) != 0 || len(r.known) != 0 || len(r.planned) != 0 || r.bytes != 0 {
		t.Fatal("closed stream retained pending or known entities")
	}
}

func TestReplicationRepeatedFlushCannotMultiplyTickBudget(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	replicationMonsters(s, 1160, 6000, 6000)
	s.sendAllEntities(1)
	for i := 0; i < 100; i++ {
		qs[0].allowance = 1024
		s.flush()
	}
	if got := len(qs[0].accepted); got > replicationTickBudget {
		t.Fatalf("repeated flush accepted %d events in one tick", got)
	}
	replicationTick(s, qs, 1024)
	if got := len(qs[0].accepted); got > 2*replicationTickBudget {
		t.Fatalf("second tick accepted %d cumulative events", got)
	}
}

func TestReplicationReenterBeforeQueuedRemovePreservesRespawn(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	for i := 0; i < 3; i++ {
		s.emitTo(1, event.EntityStatusEffect{Who: m.ID, Effect: "control"})
	}
	qs[0].allowance = 0
	s.flush()
	s.aoi.Move(m, domain.Pos{MapID: 7, X: 30000, Y: 30000})
	s.aoi.Move(m, domain.Pos{MapID: 7, X: 6000, Y: 6000})
	known := true
	removes, spawns := 0, 0
	for i := 0; i < 3; i++ {
		replicationTick(s, qs, 100)
		for _, ev := range qs[0].take() {
			switch e := ev.(type) {
			case event.EntityDespawned:
				if e.ID == m.ID {
					known = false
					removes++
				}
			case event.EntitySpawned:
				if e.ID == m.ID {
					known = true
					spawns++
				}
			}
		}
	}
	if !known {
		t.Fatalf("reentry lost after fairness slot: removes=%d spawns=%d", removes, spawns)
	}
}

func TestReplicationNonlocalSnapshotPrecedesLiveMovement(t *testing.T) {
	s, qs := replicationScene(t, 2, true)
	s.sendAllEntities(1)
	qs[0].allowance = 0
	s.flush()
	p := s.players[2]
	s.aoi.Move(p, domain.Pos{MapID: 7, X: 6100, Y: 6100})
	s.emitExcept(event.EntityMoved{ID: p.ID, To: p.Pos}, p.ID)
	replicationTick(s, qs, 100)
	known := false
	for _, ev := range qs[0].take() {
		switch e := ev.(type) {
		case event.EntitySpawned:
			if e.ID == p.ID {
				known = true
			}
		case event.EntityMoved:
			if e.ID == p.ID && !known {
				t.Fatal("global player movement preceded lazy initial spawn")
			}
		}
	}
	if !known {
		t.Fatal("global player snapshot never completed")
	}
}

func TestReplicationNonlocalLiveSpawnDoesNotDuplicateInitial(t *testing.T) {
	s, qs := replicationScene(t, 2, true)
	s.sendAllEntities(1)
	s.emitTo(1, s.spawnEvent(s.players[2]))
	replicationTick(s, qs, 100)
	count := 0
	for _, ev := range qs[0].take() {
		if e, ok := ev.(event.EntitySpawned); ok && e.ID == 2 {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("global player initial/live spawn duplicated: %d", count)
	}
}

func TestReplicationStaleRemoveCannotDeleteNewPlannedIncarnation(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	old := replicationMonsters(s, 1, 6000, 6000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	s.aoi.Leave(old)
	delete(s.entities, old.ID)
	delete(s.monsters, old.ID)
	fresh := replicationMonster(s, old.ID, 77, domain.Stats{}, 6000, 6000)
	r := s.replication[1]
	s.queueReplicationRemove(r, old, event.DespawnOutOfSight)
	if r.planned[fresh.ID] != fresh {
		t.Fatal("stale old incarnation remove cleared new planned spawn")
	}
	for i := 0; i < 3; i++ {
		replicationTick(s, qs, 100)
	}
	latest, ok := replicationSpawns(qs[0].take())[fresh.ID]
	if !ok || latest.HP != 77 {
		t.Fatalf("new incarnation absent after stale remove: %+v", latest)
	}
}

func TestReplicationCombatPinEndsAtDespawnAndDoesNotCarryToReusedID(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	old := replicationMonsters(s, 1, 30000, 30000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	s.emit(event.SkillCastChanged{Caster: old.ID, Target: 1, Skill: 1, Phase: event.SkillCastRelease})
	replicationTick(s, qs, 100)
	if _, ok := replicationSpawns(qs[0].take())[old.ID]; !ok {
		t.Fatal("combat source was not introduced")
	}
	s.emit(event.EntityDespawned{ID: old.ID, Kind: domain.KindMonster, Reason: event.DespawnRemoved})
	s.aoi.Leave(old)
	delete(s.entities, old.ID)
	delete(s.monsters, old.ID)
	replicationTick(s, qs, 100)
	qs[0].take()
	r := s.replication[1]
	if r.known[old.ID] != nil || r.planned[old.ID] != nil || r.pinned[old.ID] != nil {
		t.Fatal("despawn retained old combat dependency")
	}
	fresh := replicationMonster(s, old.ID, 77, domain.Stats{}, 30000, 30000)
	s.emit(s.spawnEvent(fresh))
	replicationTick(s, qs, 100)
	if len(replicationSpawns(qs[0].take())) != 0 {
		t.Fatal("reused ID inherited old offscreen combat pin")
	}
	s.cancelReplication(1)
	if len(s.replication) != 0 {
		t.Fatal("map reset retained replication state")
	}
}

func TestReplicationGlobalEntityKindsStageSpawnBeforeDelta(t *testing.T) {
	for _, kind := range []domain.EntityKind{domain.KindNPC, domain.KindDrop, domain.KindPet} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			s, qs := replicationScene(t, 1, true)
			e := &entity.Entity{ID: 50, Kind: kind, HP: 100, MaxHP: 100, Pos: domain.Pos{MapID: 7, X: 30000, Y: 30000}}
			switch kind {
			case domain.KindNPC:
				e.NPC = &entity.NPCInfo{Sprite: "npc601"}
			case domain.KindDrop:
				e.Drop = &entity.Drop{Item: 1, Count: 1}
			case domain.KindPet:
				e.Pet = &entity.Pet{Owner: 1}
			}
			s.entities[e.ID] = e
			s.aoi.Enter(e)
			qs[0].allowance = 0
			s.sendAllEntities(1)
			s.flush()
			s.aoi.Move(e, domain.Pos{MapID: 7, X: 30100, Y: 30100})
			s.emit(event.EntityMoved{ID: e.ID, To: e.Pos})
			s.emit(s.spawnEvent(e))
			replicationTick(s, qs, 100)
			known := false
			spawns := 0
			for _, ev := range qs[0].take() {
				switch ev := ev.(type) {
				case event.EntitySpawned:
					if ev.ID == e.ID {
						known = true
						spawns++
					}
				case event.EntityMoved:
					if ev.ID == e.ID && !known {
						t.Fatalf("%v delta before spawn", kind)
					}
				}
			}
			if !known || spawns != 1 {
				t.Fatalf("%v spawn count %d", kind, spawns)
			}
		})
	}
}

func TestReplicationPendingBytesAreBoundedPerViewer(t *testing.T) {
	s, qs := replicationScene(t, 2, true)
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	s.sendAllEntities(1)
	s.sendAllEntities(2)
	s.flush()
	qs[0].take()
	qs[1].take()
	qs[0].allowance = 0
	s.emitTo(1, event.EntityStatusEffect{Who: m.ID, Effect: strings.Repeat("x", replicationByteLimit+1)})
	s.emitTo(2, event.EntityStatusEffect{Who: m.ID, Effect: "healthy viewer"})
	replicationTick(s, qs, 0)
	if !qs[0].closed {
		t.Fatal("oversized staged payload escaped byte bound")
	}
	if qs[1].closed {
		t.Fatal("one viewer's byte overflow closed another")
	}
	r := s.replication[1]
	if r != nil && (r.bytes != 0 || len(r.queue) != 0) {
		t.Fatal("failed stream retained oversized payload")
	}
	replicationTick(s, qs, 100)
	found := false
	for _, ev := range qs[1].take() {
		if e, ok := ev.(event.EntityStatusEffect); ok && e.Effect == "healthy viewer" {
			found = true
		}
	}
	if !found {
		t.Fatal("healthy viewer did not continue")
	}
}

func TestReplicationFreshSpawnPresentationSurvivesPacing(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	drop := &entity.Entity{ID: 50, Kind: domain.KindDrop, Pos: domain.Pos{MapID: 7, X: 6000, Y: 6000}, Drop: &entity.Drop{Item: 1, Count: 1}}
	s.entities[drop.ID] = drop
	s.aoi.Enter(drop)
	fresh := s.freshGroundSpawnEvent(drop)
	if fresh.Ground == nil || !fresh.Ground.FreshDrop {
		t.Fatal("fixture did not produce fresh drop")
	}
	s.emit(fresh)
	m := replicationMonster(s, 100, 100, domain.Stats{}, 6000, 6000)
	monsterSpawn := s.spawnEvent(m)
	monsterSpawn.Fresh = true
	s.emit(monsterSpawn)
	qs[0].allowance = 0
	s.flush()
	replicationTick(s, qs, 100)
	gotDrop, gotMonster := false, false
	for _, ev := range qs[0].take() {
		if sp, ok := ev.(event.EntitySpawned); ok {
			switch sp.ID {
			case drop.ID:
				gotDrop = true
				if sp.Ground == nil || !sp.Ground.FreshDrop || sp.Ground.DropToken != fresh.Ground.DropToken {
					t.Fatalf("fresh drop metadata lost: %+v", sp.Ground)
				}
			case m.ID:
				gotMonster = true
				if !sp.Fresh {
					t.Fatal("new monster freshness lost")
				}
			}
		}
	}
	if !gotDrop || !gotMonster {
		t.Fatalf("fresh spawn missing: drop=%v monster=%v", gotDrop, gotMonster)
	}
}

func TestReplicationOutboxCapturesEventTimeIncarnation(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	old := replicationMonsters(s, 1, 6000, 6000)[0]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	stalePosition := domain.Pos{MapID: 7, X: 9999, Y: 9999}
	s.emit(event.EntityMoved{ID: old.ID, To: stalePosition})
	s.emit(event.EntityDied{ID: old.ID, Kind: domain.KindMonster, Killer: 1})
	s.emit(event.EntityDespawned{ID: old.ID, Kind: domain.KindMonster, Reason: event.DespawnDeath})
	s.aoi.Leave(old)
	delete(s.entities, old.ID)
	delete(s.monsters, old.ID)
	fresh := replicationMonster(s, old.ID, 77, domain.Stats{}, 6200, 6200)
	s.emit(s.spawnEvent(fresh))
	currentHP := int32(100)
	known := true
	newSeen := false
	for i := 0; i < 3; i++ {
		replicationTick(s, qs, 100)
		for _, ev := range qs[0].take() {
			switch e := ev.(type) {
			case event.EntitySpawned:
				if e.ID == fresh.ID {
					known = true
					currentHP = e.HP
					if e.HP == 77 {
						newSeen = true
					}
				}
			case event.EntityMoved:
				if e.ID == fresh.ID && newSeen && e.To == stalePosition {
					t.Fatal("old movement updated replacement incarnation")
				}
			case event.EntityDied:
				if e.ID == fresh.ID && newSeen {
					t.Fatal("old death killed replacement incarnation")
				}
			case event.EntityDespawned:
				if e.ID == fresh.ID {
					known = false
				}
			}
		}
	}
	if !newSeen || !known || currentHP != 77 {
		t.Fatalf("stale outbox corrupted replacement: seen=%v known=%v hp=%d", newSeen, known, currentHP)
	}
}

func TestReplicationCanceledOldQueuedSpawnCannotBecomeReplacementEvent(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	old := replicationMonsters(s, 1, 6000, 6000)[0]
	qs[0].allowance = 0
	s.sendAllEntities(1)
	s.flush()
	s.emit(s.spawnEvent(old))
	s.emit(event.EntityDied{ID: old.ID, Kind: domain.KindMonster, Killer: 1})
	s.aoi.Leave(old)
	delete(s.entities, old.ID)
	delete(s.monsters, old.ID)
	fresh := replicationMonster(s, old.ID, 77, domain.Stats{}, 6200, 6200)
	s.emit(s.spawnEvent(fresh))
	replicationTick(s, qs, 100)
	spawns := 0
	for _, ev := range qs[0].take() {
		switch e := ev.(type) {
		case event.EntitySpawned:
			if e.ID == fresh.ID {
				spawns++
				if e.HP != 77 {
					t.Fatalf("old queued spawn materialized: hp=%d", e.HP)
				}
			}
		case event.EntityDied:
			if e.ID == fresh.ID {
				t.Fatal("never-known old death attached to replacement")
			}
		}
	}
	if spawns != 1 {
		t.Fatalf("replacement spawn count %d", spawns)
	}
}

func TestReplicationUnrelatedRemoteCombatDoesNotPinFarMonster(t *testing.T) {
	s, qs := replicationScene(t, 2, true)
	p := s.players[2]
	s.aoi.Move(p, domain.Pos{MapID: 7, X: 30000, Y: 30000})
	m := replicationMonsters(s, 1, 30000, 30000)[0]
	s.sendAllEntities(1)
	s.sendAllEntities(2)
	s.flush()
	qs[0].take()
	qs[1].take()
	s.emit(event.SkillCastChanged{Caster: p.ID, Target: m.ID, Skill: 1, Phase: event.SkillCastRelease})
	replicationTick(s, qs, 100)
	for _, ev := range qs[0].take() {
		switch e := ev.(type) {
		case event.EntitySpawned:
			if e.ID == m.ID {
				t.Fatal("unrelated far combat escaped local AOI")
			}
		case event.SkillCastChanged:
			if e.Target == m.ID {
				t.Fatal("unrelated far cast references unseen monster")
			}
		}
	}
	if s.replication[1].pinned[m.ID] != nil {
		t.Fatal("unrelated far combat left a persistent pin")
	}
	cast := false
	for _, ev := range qs[1].take() {
		if e, ok := ev.(event.SkillCastChanged); ok && e.Target == m.ID {
			cast = true
		}
	}
	if !cast {
		t.Fatal("attacker lost own far combat event")
	}
}

func TestReplicationSustainedDenseMovementCoalescesByEntity(t *testing.T) {
	const count = 4097
	s, qs := replicationScene(t, 1, true)
	monsters := replicationMonsters(s, count, 6000, 6000)
	s.sendAllEntities(1)
	client := make(map[domain.EntityID]domain.Pos, count)
	consume := func() {
		for _, ev := range qs[0].take() {
			switch e := ev.(type) {
			case event.EntitySpawned:
				if e.Kind == domain.KindMonster {
					client[e.ID] = e.Pos
				}
			case event.EntityMoved:
				if _, ok := client[e.ID]; !ok {
					t.Fatalf("move before spawn %d", e.ID)
				}
				client[e.ID] = e.To
			}
		}
	}
	for tick := 0; tick < 200 && len(client) < count; tick++ {
		replicationTick(s, qs, 256)
		consume()
	}
	if len(client) != count {
		t.Fatalf("initial snapshot incomplete: %d/%d", len(client), count)
	}
	for tick := 1; tick <= 100; tick++ {
		for _, m := range monsters {
			pos := domain.Pos{MapID: 7, X: 6000 + float64(tick), Y: 6000}
			s.aoi.Move(m, pos)
			s.emit(event.EntityMoved{ID: m.ID, To: pos})
		}
		replicationTick(s, qs, 256)
		consume()
		r := s.replication[1]
		if qs[0].closed {
			t.Fatalf("healthy dense movement closed at tick%d", tick)
		}
		if len(r.queue) > count+64 {
			t.Fatalf("queue grows with N*ticks: tick=%d queue=%d", tick, len(r.queue))
		}
		if r.bytes > replicationByteLimit {
			t.Fatalf("movement byte bound exceeded: %d", r.bytes)
		}
	}
	for tick := 0; tick < 100 && len(s.replication[1].queue) > 0; tick++ {
		replicationTick(s, qs, 256)
		consume()
	}
	if len(s.replication[1].queue) != 0 {
		t.Fatal("coalesced movement did not drain after quiet")
	}
	for _, m := range monsters {
		if client[m.ID] != m.Pos {
			t.Fatalf("latest position lost for%d: client=%+v authoritative=%+v", m.ID, client[m.ID], m.Pos)
		}
	}
}

func TestReplicationCoalescingCannotCrossCriticalEntityBarrier(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	a, b := replicationMonsters(s, 2, 6000, 6000)[0], s.entities[101]
	s.sendAllEntities(1)
	s.flush()
	qs[0].take()
	move := func(e *entity.Entity, x float64) {
		s.aoi.Move(e, domain.Pos{MapID: 7, X: x, Y: 6000})
		s.emit(event.EntityMoved{ID: e.ID, To: e.Pos})
	}
	move(a, 6100)
	move(b, 6100)
	s.emit(event.DamageDealt{Src: 1, Dst: a.ID, Amount: 1, DstHP: 99})
	move(a, 6200)
	move(b, 6200)
	move(a, 6300)
	qs[0].allowance = 0
	s.flush()
	a.HP = 0
	s.emit(event.DamageDealt{Src: 1, Dst: a.ID, Amount: 99, DstHP: 0, Flag: event.DamageFatal})
	s.emit(event.EntityDied{ID: a.ID, Kind: domain.KindMonster, Killer: 1})
	s.emit(event.EntityDespawned{ID: a.ID, Kind: domain.KindMonster, Reason: event.DespawnDeath})
	s.aoi.Leave(a)
	delete(s.entities, a.ID)
	delete(s.monsters, a.ID)
	replicationTick(s, qs, 256)
	phase := 0
	for _, ev := range qs[0].take() {
		switch e := ev.(type) {
		case event.EntityMoved:
			if e.ID == a.ID {
				if phase == 0 && e.To.X == 6100 {
					phase = 1
				} else if phase == 2 && e.To.X == 6300 {
					phase = 3
				} else {
					t.Fatalf("move crossed critical barrier: phase%d pos%v", phase, e.To)
				}
			}
		case event.DamageDealt:
			if e.Dst == a.ID {
				if !e.Flag.Has(event.DamageFatal) && phase == 1 {
					phase = 2
				} else if e.Flag.Has(event.DamageFatal) && phase == 3 {
					phase = 4
				} else {
					t.Fatalf("damage reordered at phase%d", phase)
				}
			}
		case event.EntityDied:
			if e.ID == a.ID {
				if phase != 4 {
					t.Fatalf("death at phase%d", phase)
				}
				phase = 5
			}
		case event.EntityDespawned:
			if e.ID == a.ID {
				if phase != 5 {
					t.Fatalf("remove at phase%d", phase)
				}
				phase = 6
			}
		}
	}
	if phase != 6 {
		t.Fatalf("critical lifecycle incomplete: phase%d", phase)
	}
}

func TestReplicationInitialEntitiesWaitForWorldLifecyclePrefix(t *testing.T) {
	for _, kind := range []string{"self", "teleport"} {
		t.Run(kind, func(t *testing.T) {
			s, qs := replicationScene(t, 1, true)
			replicationMonsters(s, 20, 6000, 6000)
			for i := 0; i < 6; i++ {
				s.emitTo(1, event.NewbieTextNotice{Who: 1, Text: "prefix"})
			}
			if kind == "self" {
				s.emitTo(1, event.SelfEntered{ID: 1, Scene: s.id, Char: s.players[1].Player.Char.Clone()})
			} else {
				s.emitTo(1, event.Teleported{Who: 1, Scene: s.id, To: s.players[1].Pos})
			}
			s.sendAllEntities(1)
			lifecycle := false
			spawns := 0
			for tick := 0; tick < 100 && spawns < 20; tick++ {
				replicationTick(s, qs, 2)
				for _, ev := range qs[0].take() {
					switch e := ev.(type) {
					case event.SelfEntered:
						if kind == "self" && e.ID == 1 {
							lifecycle = true
						}
					case event.Teleported:
						if kind == "teleport" && e.Who == 1 {
							lifecycle = true
						}
					case event.EntitySpawned:
						if e.Kind == domain.KindMonster {
							if !lifecycle {
								t.Fatal("initial spawn overtook mandatory lifecycle prefix")
							}
							spawns++
						}
					}
				}
			}
			if spawns != 20 {
				t.Fatalf("initial snapshot incomplete after lifecycle fence: %d", spawns)
			}
		})
	}
}

func TestReplicationInitialSnapshotPrioritizesNearestEntities(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	for i := 99; i >= 0; i-- {
		replicationMonster(s, domain.EntityID(100+i), 100, domain.Stats{}, 6000+float64(i)*10, 6000)
	}
	s.sendAllEntities(1)
	replicationTick(s, qs, 256)
	index := 0
	for _, ev := range qs[0].take() {
		if e, ok := ev.(event.EntitySpawned); ok && e.Kind == domain.KindMonster {
			if e.ID != domain.EntityID(100+index) {
				t.Fatalf("initial row%d not nearest: id%d", index, e.ID)
			}
			index++
		}
	}
	if index != replicationTickBudget {
		t.Fatalf("initial pacing count%d", index)
	}
}

func TestReplicationDenseMovementAndStatusBurstFitsBoundedStaging(t *testing.T) {
	const count = 4097
	s, qs := replicationScene(t, 1, true)
	monsters := replicationMonsters(s, count, 6000, 6000)
	s.sendAllEntities(1)
	for tick := 0; tick < 200 && len(s.replication[1].known) < count; tick++ {
		replicationTick(s, qs, 2048)
		qs[0].take()
	}
	if len(s.replication[1].known) != count {
		t.Fatal("initial known set incomplete")
	}
	for _, m := range monsters {
		s.aoi.Move(m, domain.Pos{MapID: 7, X: 6100, Y: 6000})
		s.emit(event.EntityMoved{ID: m.ID, To: m.Pos})
		s.emit(event.EntityStatusSnapshot{Who: m.ID, Icons: []event.StatusIconView{{Status: 91001, Icon: 5038, RemainMS: 1000}}})
	}
	replicationTick(s, qs, 2048)
	r := s.replication[1]
	if qs[0].closed {
		t.Fatal("healthy4097 movement+status burst hit artificial count overflow")
	}
	if len(r.queue) > replicationQueueLimit || r.bytes > replicationByteLimit {
		t.Fatalf("unbounded burst queue=%d bytes=%d", len(r.queue), r.bytes)
	}
	moves, statuses := map[domain.EntityID]bool{}, map[domain.EntityID]bool{}
	consume := func() {
		for _, ev := range qs[0].take() {
			switch e := ev.(type) {
			case event.EntityMoved:
				moves[e.ID] = true
			case event.EntityStatusSnapshot:
				statuses[e.Who] = true
			}
		}
	}
	consume()
	for tick := 0; tick < 100 && len(r.queue) > 0; tick++ {
		replicationTick(s, qs, 2048)
		consume()
	}
	if len(moves) != count || len(statuses) != count || len(r.queue) != 0 {
		t.Fatalf("burst not fully delivered: moves%d statuses%d queue%d", len(moves), len(statuses), len(r.queue))
	}
}

func TestReplicationHealthyCombatViewerProgressesBesideBlockedJoin(t *testing.T) {
	const count = 1160
	s, qs := replicationScene(t, 2, true)
	monsters := replicationMonsters(s, count, 6000, 6000)
	for _, m := range monsters {
		m.HP = 100000
		m.MaxHP = 100000
	}
	s.sendAllEntities(1)
	s.sendAllEntities(2)
	tick := func() { s.tick++; qs[0].allowance = 2048; qs[1].allowance = 0; s.flush() }
	for frame := 0; frame < 100 && len(s.replication[1].known) < count; frame++ {
		tick()
		qs[0].take()
	}
	if len(s.replication[1].known) < count {
		t.Fatal("healthy initial view incomplete")
	}
	wantRoot, gotRoot := uint64(0), uint64(0)
	consume := func() {
		for _, ev := range qs[0].take() {
			if hit, ok := ev.(event.DamageDealt); ok {
				gotRoot++
				if hit.RootID != gotRoot {
					t.Fatalf("critical damage order lost: gotroot%d want%d", hit.RootID, gotRoot)
				}
			}
		}
	}
	for frame := 0; frame < 100; frame++ {
		for _, m := range monsters {
			s.aoi.Move(m, domain.Pos{MapID: 7, X: 6000 + float64(frame), Y: 6000})
			s.emit(event.EntityMoved{ID: m.ID, To: m.Pos})
		}
		for j := 0; j < 10; j++ {
			m := monsters[(frame*10+j)%count]
			m.HP--
			wantRoot++
			s.emit(event.DamageDealt{Src: 1, Dst: m.ID, Amount: 1, DstHP: m.HP, RootID: wantRoot})
		}
		tick()
		consume()
		if qs[0].closed {
			t.Fatalf("healthy combat viewer closed at frame%d", frame)
		}
		if n := len(s.replication[1].queue); n != 0 {
			t.Fatalf("healthy combat viewer retained tick backlog: frame%d queue%d", frame, n)
		}
	}
	for frame := 0; frame < 100 && len(s.replication[1].queue) > 0; frame++ {
		tick()
		consume()
	}
	if gotRoot != wantRoot || gotRoot != 1000 {
		t.Fatalf("critical HP events missing: got%d want%d", gotRoot, wantRoot)
	}
	if len(qs[1].accepted) != 0 {
		t.Fatalf("blockedjoiningviewer accepted%d events", len(qs[1].accepted))
	}
	if len(s.replication[1].queue) != 0 {
		t.Fatal("healthy combat queue did not settle after quiet")
	}
}

func TestReplicationControlsRemainImmediateWhileEntitySnapshotBlocked(t *testing.T) {
	s, qs := replicationScene(t, 1, false)
	q := &replicationMapSink{replicationTestSink: qs[0]}
	s.players[1].Player.Sink = q
	m := replicationMonsters(s, 1, 6000, 6000)[0]
	q.allowance = 0
	s.sendAllEntities(1)
	s.flush()
	s.emitTo(1, event.InventorySnapshot{Who: 1})
	s.emitTo(1, event.StatsChanged{Who: 1, Current: []int32{1, 0, 0, 75, 100}})
	s.emitTo(1, event.NewbieTextNotice{Who: 1, Text: "control stays immediate"})
	s.flush()
	controls := q.take()
	if countOf[event.InventorySnapshot](controls) != 1 || countOf[event.StatsChanged](controls) != 1 || countOf[event.NewbieTextNotice](controls) != 1 {
		t.Fatalf("entity admission blocked or duplicated immediate inventory/stats/control: inv=%d stats=%d notice=%d events=%#v", countOf[event.InventorySnapshot](controls), countOf[event.StatsChanged](controls), countOf[event.NewbieTextNotice](controls), controls)
	}
	if len(replicationSpawns(controls)) != 0 {
		t.Fatal("refused entity snapshot unexpectedly admitted")
	}
	landing := domain.Pos{MapID: 7, X: 9000, Y: 9000}
	if !s.onTeleport(Teleport{ID: 1, To: s.id, At: landing}) {
		t.Fatal("entity backpressure blocked immediate teleport")
	}
	s.flush()
	reset, ok := firstOf[event.Teleported](q.take())
	if !ok || reset.To != landing || q.epoch == 0 || s.mapLoads[1] == nil {
		t.Fatal("immediate teleport/mapload was deferred behind entity snapshot")
	}
	replicationTick(s, qs, 256)
	if len(replicationSpawns(q.take())) != 0 {
		t.Fatal("old entity snapshot escaped mapload cancellation")
	}
	m.HP = 55
	s.onMapReady(MapReady{ID: 1, Scene: s.id, Epoch: q.epoch})
	replicationTick(s, qs, 256)
	sp, ok := replicationSpawns(q.take())[m.ID]
	if !ok || sp.HP != 55 {
		t.Fatalf("fresh mapready snapshot lost: %+v", sp)
	}
	if q.bypass != 0 {
		t.Fatalf("world entity events bypassed staged lane: %d", q.bypass)
	}
	if q.cancels == 0 {
		t.Fatal("map boundary failed to cancel blocked entity generation")
	}
}

type replicationNativeRebuildSink struct {
	*replicationTestSink
	epoch uint64
}

func (q *replicationNativeRebuildSink) CurrentSceneReady(_ domain.EntityID, _ domain.SceneID, epoch uint64) bool {
	return epoch == q.epoch
}
func (q *replicationNativeRebuildSink) ConfirmClientGateReady(_ domain.EntityID, _ domain.SceneID, epoch uint64) bool {
	return epoch == q.epoch
}

func TestReplicationNativePrebuiltGateRemovesLateOldWorldBeforeNewSpawns(t *testing.T) {
	s, qs := replicationScene(t, 1, true)
	s.rift.data.Run = &domain.RiftRun{Phase: domain.RiftAborted}
	q := &replicationNativeRebuildSink{replicationTestSink: qs[0], epoch: 7}
	s.players[1].Player.Sink = q
	old := replicationMonsters(s, 100, 6000, 6000)
	for i := 0; i < 100; i++ {
		replicationMonster(s, domain.EntityID(1000+i), 100, domain.Stats{}, 30000, 30000)
	}
	s.sendAllEntities(1)
	s.flush()
	lateOldPackets := q.take()
	oldKnown := replicationSpawns(lateOldPackets)
	if len(oldKnown) != replicationTickBudget {
		t.Fatalf("fixture old accepted count%d", len(oldKnown))
	}
	if r := s.replication[1]; r.initialAt >= len(r.initial) {
		t.Fatal("fixture has no pending old initial rows")
	}
	// Native BuildWorld has ALREADY cleared the client before the ready report
	// reaches this actor. Previously admitted TCP rows can arrive after that clear.
	client := map[domain.EntityID]bool{}
	for id := range oldKnown {
		client[id] = true
	}
	landing := domain.Pos{MapID: 7, X: 30000, Y: 30000}
	s.portals = []domain.Portal{domain.NewPortal(domain.Teleport{ClientOwned: true, To: landing})}
	for _, m := range old {
		if _, known := oldKnown[m.ID]; known {
			s.emit(event.EntityMoved{ID: m.ID, To: m.Pos})
			s.emit(event.EntityStatusSnapshot{Who: m.ID})
		}
	}
	q.allowance = 2
	s.onClientSceneReady(ClientSceneReady{ID: 1, Scene: s.id, Epoch: q.epoch, At: landing})
	if s.players[1].Pos != landing {
		t.Fatal("native same-map gate did not land")
	}
	removed := map[domain.EntityID]bool{}
	newSeen := map[domain.EntityID]bool{}
	consume := func() {
		for _, ev := range q.take() {
			switch e := ev.(type) {
			case event.Teleported:
				t.Fatal("native prebuilt gate unexpectedly relied on a second native world clear")
			case event.EntityDespawned:
				if _, wasOld := oldKnown[e.ID]; wasOld {
					if e.Reason != event.DespawnOutOfSight {
						t.Fatalf("old rebuild remove reason%v", e.Reason)
					}
					removed[e.ID] = true
				}
				delete(client, e.ID)
			case event.EntitySpawned:
				if e.Kind != domain.KindMonster {
					continue
				}
				if e.ID < 1000 {
					t.Fatalf("pending old-window spawn escaped rebuild: %d", e.ID)
				}
				if len(removed) != len(oldKnown) {
					t.Fatalf("new spawn before old reconciliation: removed%d/%d", len(removed), len(oldKnown))
				}
				client[e.ID] = true
				newSeen[e.ID] = true
			case event.EntityMoved:
				if _, wasOld := oldKnown[e.ID]; wasOld && !client[e.ID] {
					t.Fatalf("movement after remove without re-spawn: %d", e.ID)
				}
			case event.EntityStatusSnapshot:
				if _, wasOld := oldKnown[e.Who]; wasOld && !client[e.Who] {
					t.Fatalf("status after remove without re-spawn: %d", e.Who)
				}
			}
		}
	}
	consume()
	for frame := 0; frame < 100 && len(newSeen) < 100; frame++ {
		replicationTick(s, qs, 8)
		consume()
	}
	if len(removed) != len(oldKnown) || len(newSeen) != 100 {
		t.Fatalf("rebuild incomplete: oldremoves%d/%d newspawns%d/100", len(removed), len(oldKnown), len(newSeen))
	}
	for id := range oldKnown {
		if client[id] {
			t.Fatalf("old-only late TCP actor remained ghost: %d", id)
		}
	}
	if len(client) != 100 || q.closed {
		t.Fatalf("rebuilt model count%d closed%v", len(client), q.closed)
	}
}
