package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// A floor change owns the run write until its result (including lost-ACK
// reconciliation) is known. No key, inventory or reward mutation is involved.
type endgameRiftTransition struct {
	rewardContexts map[domain.CharID]endgameRiftRewardContext
	target         domain.SceneID
	next           endgameRiftPersisted
	revision       int64
	save           <-chan domain.EndgameRunSaveResult
	load           <-chan domain.EndgameRunLoadResult
	retryAt        int64
	requester      domain.EntityID
	request        domain.EndgameRequest
}

func (s *Scene) riftCombatReady() bool {
	x := s.rift
	return x != nil && !x.economicPending && !x.floorLoading && !x.retired && x.transition == nil && (x.data.Run.Phase == domain.RiftActive || x.data.Run.Phase == domain.RiftBoss)
}
func (s *Scene) canAdvanceEndgameRift(p *entity.Entity) bool {
	x := s.rift
	if x == nil || x.data.Floor == nil || !s.riftCombatReady() || x.data.Run.Phase != domain.RiftActive || p == nil || p.Player == nil || !p.Alive() {
		return false
	}
	char := domain.CharID(p.Player.Char.ID)
	return char == x.data.Run.Payer && x.ready[char] && x.data.Run.Admitted(char) && s.atEndgameRiftExit(p.Pos)
}

// Both the seeded exit marker and every usable native map gate lead forward.
// Use the actor's position, never client-supplied coordinates or destinations.
// The same 160px tolerance as the seeded exit covers the movement report cadence.
func (s *Scene) atEndgameRiftExit(pos domain.Pos) bool {
	x := s.rift
	if x == nil || x.data.Floor == nil || pos.MapID != s.id.MapID {
		return false
	}
	if riftDistance2(pos, x.data.Floor.Exit) <= 160*160 {
		return true
	}
	for _, portal := range x.portals {
		area := portal.Area
		if len(area) < 3 {
			continue
		}
		if area.Contains(pos) {
			return true
		}
		for i, a := range area {
			b := area[(i+1)%len(area)]
			dx, dy := b.X-a.X, b.Y-a.Y
			length := dx*dx + dy*dy
			u := 0.0
			if length > 0 {
				u = ((pos.X-a.X)*dx + (pos.Y-a.Y)*dy) / length
			}
			if u < 0 {
				u = 0
			} else if u > 1 {
				u = 1
			}
			near := domain.Pos{X: a.X + u*dx, Y: a.Y + u*dy}
			if riftDistance2(pos, near) <= 160*160 {
				return true
			}
		}
	}
	return false
}

func (s *Scene) advanceEndgameRift(p *entity.Entity, req domain.EndgameRequest) {
	x := s.rift
	if x == nil || x.data.Floor == nil {
		s.endgameError(p, req, "legacy_run", "旧洞天请完成或离开后再开启新洞天")
		return
	}
	if req.RunID != x.data.Run.RunID || req.FloorIndex != x.data.Floor.FloorIndex {
		s.endgameError(p, req, "stale_floor", "当前地图层已变化，请刷新")
		s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
		return
	}
	if !s.canAdvanceEndgameRift(p) {
		s.endgameError(p, req, "exit", "请由开启者抵达本层出口或传送门；首领战中不能换层")
		return
	}
	if x.saving != nil || x.economicPending {
		s.endgameError(p, req, "pending", "洞天存档中，请稍候")
		return
	}
	// Never move ownership away from a character whose economic operation is
	// pending. Its immutable callback still belongs to this scene actor.
	for _, peer := range s.players {
		if q := s.endgamePlayers[peer.ID]; q != nil && q.commit != nil {
			s.endgameError(p, req, "pending", "队员存档中，请稍候")
			return
		}
	}
	if x.data.Floor.FloorIndex >= 2147483646 {
		s.endgameError(p, req, "floor_limit", "洞天地图层已达上限")
		return
	}
	plan, err := s.planEndgameRiftFloor(x.data.Run.RunID, x.data.Floor.FloorIndex+1, x.data.MapID)
	if err != nil {
		s.endgameError(p, req, "layout", "暂无可用下一层，请稍后重试")
		return
	}
	if x.data.Run.Rules.CompletionLevelExperience || x.data.Run.Rules.NormalBossEquipment > 0 {
		for char, context := range s.freezeEndgameRiftRewardContexts() {
			s.cacheEndgameRiftRewardContext(char, context)
		}
	}
	next := x.data.clone()
	next.Floor = &plan
	next.MapID = plan.MapID
	next.DeathTimes = map[domain.CharID]int64{}
	for char, at := range x.deathAt {
		next.DeathTimes[char] = at
	}
	// Retired floors cannot be revisited. Drop all source floor IDs and globes;
	// cumulative progress, deaths, membership, timer and tier are unchanged.
	next.Run.Monsters = map[domain.EntityID]*domain.RiftMonster{}
	next.Run.Globes = map[domain.EntityID]domain.RiftGlobe{}
	next.Run.Revision++
	next.StorageRevision = x.revision + 1
	target, err := s.router.reserveEndgameRift(next.MapID, next)
	if err != nil {
		s.endgameError(p, req, "instance", "下一层暂不可用，请重试")
		return
	}
	backend, ok := s.endgameBackend.(domain.EndgameRunBackend)
	if !ok {
		s.router.discardEndgameRift(target)
		s.endgameError(p, req, "unavailable", "洞天存档暂不可用")
		return
	}
	x.transition = &endgameRiftTransition{target: target, next: next, revision: x.revision + 1, requester: p.ID, request: req, save: backend.SaveRun(next.record(x.revision+1), x.revision)}
	s.cancelEndgameRiftAbilities(s.riftMS())
	for id := range s.entities {
		s.cancelPendingCombat(id)
	}
}
func (s *Scene) pollEndgameRiftTransition(now int64) {
	x := s.rift
	if x == nil || x.transition == nil {
		return
	}
	t := x.transition
	if t.save != nil {
		select {
		case result := <-t.save:
			t.save = nil
			if result.Err == nil && result.Run != nil && result.Run.Revision == t.revision {
				s.completeEndgameRiftTransition()
				return
			}
			// A failed acknowledgement does not prove rollback. Read before allowing
			// source combat or retrying; do not refund or create a second floor.
			t.load = s.endgameBackend.LoadRun(x.data.Run.RunID)
		default:
			return
		}
	}
	if t.load == nil && now >= t.retryAt {
		t.load = s.endgameBackend.LoadRun(x.data.Run.RunID)
	}
	if t.load != nil {
		select {
		case result := <-t.load:
			t.load = nil
			d, valid := decodeEndgameRift(result.Run, s.riftRules)
			if result.Err != nil || !valid {
				t.retryAt = now + 1000
				return
			}
			if result.Run.Revision > x.revision && result.Run.Revision <= t.revision && d.Floor != nil && d.Floor.FloorIndex == t.next.Floor.FloorIndex && d.Floor.Seed == t.next.Floor.Seed {
				t.next = d
				t.revision = result.Run.Revision
				s.completeEndgameRiftTransition()
				return
			}
			// The durable record remains on this floor. Release only the unpaid next
			// reservation and preserve all source monsters/progress for retry.
			if result.Run.Revision <= x.revision {
				s.router.discardEndgameRift(t.target)
				x.transition = nil
				if p := s.players[t.requester]; p != nil {
					s.endgameError(p, t.request, "storage", "下一层未保存，仍停留本层，可重试")
				}
				return
			}
			t.retryAt = now + 1000 // unexpected competing write: keep ownership quarantined
		default:
		}
	}
}
func (s *Scene) completeEndgameRiftTransition() {
	x := s.rift
	t := x.transition
	changed := false
	for char, context := range t.rewardContexts {
		changed = cacheRiftRewardContext(&t.next, char, context) || changed
	}
	if changed {
		// The floor may already be committed, but its offline recipient observations
		// were newer than that write. Persist one more CAS before transferring ownership.
		previous := t.revision
		t.revision++
		t.next.StorageRevision = t.revision
		t.save = s.endgameBackend.(domain.EndgameRunBackend).SaveRun(t.next.record(t.revision), previous)
		return
	}
	if !s.router.publishEndgameRiftFloor(s.id, t.target, t.next) {
		// The prepared actor may have stopped while storage was in flight.
		// Recreate only the same committed plan, never regenerate or charge a key.
		s.router.discardEndgameRift(t.target)
		if target, err := s.router.reserveEndgameRift(t.next.MapID, t.next); err == nil {
			t.target = target
		}
		t.retryAt = s.riftMS() + 1000
		return
	}
	x.retired = true
	x.data = t.next.clone()
	x.revision = t.revision
	x.savedRevision = x.data.Run.Revision
	x.delayedDeaths = nil
	s.clearRiftMonsters()
	for _, p := range s.players {
		s.endgameReply(p, domain.EndgameResponse{Kind: "rift.advance", RequestID: t.request.RequestID, OK: true, Message: "进入下一层，进度与计时保留", Rift: s.riftView(p, t.next, true, true)})
	}
	s.transferRetiredRiftPlayers()
}
func (s *Scene) transferRetiredRiftPlayers() {
	x := s.rift
	if x == nil || !x.retired || x.transition == nil {
		return
	}
	for id := range s.players {
		s.onTeleport(Teleport{RiftRunID: x.data.Run.RunID, ID: id, To: x.transition.target, At: x.data.Floor.Entry})
	}
	if len(s.players) == 0 {
		s.Stop()
	}
}
