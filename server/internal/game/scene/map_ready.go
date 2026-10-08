package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// mapLoadSink connects the scene-owned visibility barrier to the session's
// expected-map guard. Epoch is server-local; the client only acknowledges map ID.
type mapLoadSink interface {
	BeginMapLoad(domain.EntityID, domain.SceneID) (uint64, bool)
	CompleteMapLoad(domain.EntityID, domain.SceneID, uint64) bool
}

// Separate from mapLoadSink so the existing loading handshake keeps its boundary.
type currentSceneReadySink interface {
	CurrentSceneReady(domain.EntityID, domain.SceneID, uint64) bool
}

type mapLoad struct {
	epoch uint64
	// The client itself retains only the latest destination while loading.
	// Keep that pending request here so only one teleport is in flight.
	next *Teleport
}

type MapReady struct {
	ID    domain.EntityID
	Scene domain.SceneID
	Epoch uint64
}

func (MapReady) CmdName() string { return "MapReady" }

// ClientSceneReady is an entered=1 for the current, already settled scene. Epoch
// identifies the session binding at receipt; the command carries no client coordinates.
type ClientSceneReady struct {
	ID    domain.EntityID
	Scene domain.SceneID
	Epoch uint64
}

func (ClientSceneReady) CmdName() string { return "ClientSceneReady" }

func (s *Scene) onClientSceneReady(cmd ClientSceneReady) {
	e := s.players[cmd.ID]
	if e == nil || e.Player == nil || cmd.Scene != s.id || s.mapLoads[cmd.ID] != nil {
		return
	}
	sink, ok := e.Player.Sink.(currentSceneReadySink)
	if !ok || !sink.CurrentSceneReady(e.ID, s.id, cmd.Epoch) {
		return
	}
	s.flush()
	// Session teardown may run while a flush is delivering prior events.
	if !sink.CurrentSceneReady(e.ID, s.id, cmd.Epoch) {
		return
	}
	s.syncWorldAfterLoad(e)
	s.flush()
	s.log.Info("客户端同图重建，已同步全图实体", "entity", e.ID, "epoch", cmd.Epoch)
}

func (s *Scene) beginMapLoad(e *entity.Entity, alreadyReady bool) bool {
	sink, supported := e.Player.Sink.(mapLoadSink)
	if !supported {
		return true // non-network scene consumers do not perform client loading
	}
	epoch, ok := sink.BeginMapLoad(e.ID, s.id)
	if !ok {
		e.Player.Sink.Close()
		return false
	}
	if epoch == 0 {
		return true
	}
	if alreadyReady {
		if !sink.CompleteMapLoad(e.ID, s.id, epoch) {
			e.Player.Sink.Close()
			return false
		}
		return true
	}
	if s.mapLoads == nil {
		s.mapLoads = make(map[domain.EntityID]*mapLoad)
	}
	loading := &mapLoad{epoch: epoch}
	s.mapLoads[e.ID] = loading
	s.log.Info("等待客户端地图就绪", "entity", e.ID, "epoch", epoch)
	// Timeout is only a failure boundary, never an alternate readiness signal.
	s.timer.after(s.tick, domain.Ticks(60000), func() {
		if s.mapLoads[e.ID] == loading && s.players[e.ID] == e {
			s.log.Warn("客户端地图确认超时，断开连接", "entity", e.ID, "epoch", epoch)
			e.Player.Sink.Close()
		}
	})
	return true
}

func (s *Scene) onMapReady(cmd MapReady) {
	e := s.players[cmd.ID]
	loading := s.mapLoads[cmd.ID]
	if e == nil || loading == nil || cmd.Scene != s.id || loading.epoch != cmd.Epoch {
		return
	}
	sink, ok := e.Player.Sink.(mapLoadSink)
	if !ok {
		return
	}
	// Flush while the barrier is still closed: old deltas must not overtake the
	// new snapshot when the same frame contains other actors' updates.
	s.flush()
	if !sink.CompleteMapLoad(e.ID, s.id, cmd.Epoch) {
		return
	}
	delete(s.mapLoads, e.ID)
	s.syncWorldAfterLoad(e)
	s.publishEnteredPlayer(e)
	s.flush()
	s.log.Info("客户端地图就绪，已同步全图实体", "entity", e.ID, "epoch", cmd.Epoch)
	if loading.next != nil {
		s.onTeleport(*loading.next)
	}
}

func (s *Scene) syncWorldAfterLoad(e *entity.Entity) {
	if s.trial.IsMap(s.id.MapID) {
		s.emitTo(e.ID, event.NewbieTextNotice{Who: e.ID, Text: "进入妖魔结界！"})
	}
	s.pushNpcChatter(e.ID)
	s.sendAllEntities(e.ID)
	s.sendIdleMonsterGreetingTo(e)
	s.sendStallSigns(e.ID)
	for _, status := range s.visibleStatuses(e) {
		if status.effect.Effect != "" {
			s.emitTo(e.ID, status.effect)
		}
	}
	// BuildWorld resets the local riding flag; restore both the model and speed.
	s.emitTo(e.ID, s.ridingEvent(e))
	s.refreshStats(e) // 宠物实体已注册，重算天赋与装备并下发同一份属性。
	s.emitTo(e.ID, s.buffSnapshot(e))
}

func (s *Scene) publishEnteredPlayer(e *entity.Entity) {
	for _, other := range s.players {
		if other.ID != e.ID {
			s.emitTo(other.ID, s.spawnEvent(e))
			s.sendEntityStatusEffects(other.ID, e)
		}
	}
	if pet := s.entities[e.Player.Pet]; pet != nil && !e.Player.Riding {
		s.emitExcept(s.spawnEvent(pet), e.ID)
	}
}

func (s *Scene) entityMapLoading(e *entity.Entity) bool {
	if e == nil {
		return false
	}
	if s.mapLoads[e.ID] != nil {
		return true
	}
	return e.Pet != nil && s.mapLoads[e.Pet.Owner] != nil
}

func worldEntityEvent(ev event.Event) bool {
	switch ev.(type) {
	case event.EntitySpawned, event.EntityDespawned, event.EntityMoved,
		event.MonsterSpoke,
		event.EntityStatusSnapshot, event.EntityStatusEffect, event.SkillCastChanged,
		event.SkillHitEffect, event.DamageDealt, event.HealDone, event.EntityDied,
		event.PlayerRevived, event.RidingChanged, event.PlayerVisibilityChanged,
		event.PlayerPKStateChanged, event.PlayerRenamed, event.AppearanceChanged,
		event.HairChanged, event.PlayerEmoted, event.StallOwnerChanged, event.StallClosed:
		return true
	}
	return false
}

func (s *Scene) canDeliverWorldEvent(to domain.EntityID, ev event.Event) bool {
	if _, despawn := ev.(event.EntityDespawned); despawn {
		return s.mapLoads[to] == nil
	}
	return !worldEntityEvent(ev) || (s.mapLoads[to] == nil && !s.entityMapLoading(s.entities[ev.Subject()]))
}
