package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (s *Scene) stepEndgameRift() {
	s.stepEndgameRiftRequests()
	s.stepEndgameTrial()
	if s.rift == nil {
		if d, ok := s.router.riftAdmission(s.id); ok {
			if !s.initEndgameRift(d) {
				return
			}
		} else {
			return
		}
	}
	x := s.rift
	r := x.data.Run
	now := s.riftMS()
	if x.retired {
		s.transferRetiredRiftPlayers()
		return
	}
	if x.transition != nil {
		s.pollEndgameRiftTransition(now)
		return
	}
	s.pollEndgameRiftReload(now)
	r = x.data.Run
	if x.saving != nil {
		select {
		case result := <-x.saving:
			x.saving = nil
			if result.Err == nil && result.Run != nil {
				x.revision = result.Run.Revision
				x.savedRevision = x.savingRevision
			} else {
				x.lastSave = now
			}
		default:
		}
	}
	if !x.economicPending && len(x.delayedDeaths) > 0 {
		ids := x.delayedDeaths
		x.delayedDeaths = nil
		for _, death := range ids {
			s.applyEndgameRiftDeath(death)
		}
		r = x.data.Run
	}
	if !x.economicPending {
		present := map[domain.CharID]bool{}
		for _, p := range s.players {
			char := domain.CharID(p.Player.Char.ID)
			if x.ready[char] {
				present[char] = true
			}
		}
		r.ObserveClock(now, present)
		if x.floorLoading && !r.Terminal() {
			all := true
			for char, m := range r.Members {
				if !m.Departed && !x.ready[char] {
					all = false
				}
			}
			if all || now >= x.loadUntil {
				if len(x.ready) == 0 {
					r.Phase = domain.RiftFailed
					r.Reason = "loading_failed"
					r.Revision++
				} else {
					x.floorLoading = false
					s.populateEndgameRift(r.Phase == domain.RiftBoss)
				}
			}
		}
		if r.Phase == domain.RiftReserved {
			all := len(x.ready) == len(r.Members)
			if all || now >= x.loadUntil {
				if len(x.ready) == 0 {
					r.Phase = domain.RiftFailed
					r.Reason = "loading_failed"
					r.Revision++
				} else {
					for id, m := range r.Members {
						if !x.ready[id] {
							m.Departed = true
							s.router.withdrawRift(s.id, id)
						}
					}
					r.Activate(now)
					s.populateEndgameRift(false)
				}
			}
		}
		if r.Phase == domain.RiftActive {
			for id, g := range r.Globes {
				for _, p := range s.players {
					char := domain.CharID(p.Player.Char.ID)
					if p.Alive() && x.ready[char] && riftDistance2(p.Pos, g.Pos) <= 75*75 {
						r.CollectGlobe(id, char)
						break
					}
				}
			}
			if r.Phase == domain.RiftBoss {
				s.clearRiftMonsters()
				s.populateEndgameRift(true)
			}
		}
		if !r.Terminal() && r.Phase != domain.RiftReserved && len(present) == 0 {
			latest := r.StartedMS
			for _, m := range r.Members {
				if m.LastSeenMS > latest {
					latest = m.LastSeenMS
				}
			}
			if now-latest > x.data.Run.Rules.ReconnectMS {
				r.Phase = domain.RiftAborted
				r.Reason = "abandoned"
				r.Revision++
			}
		}
		if r.Phase == domain.RiftCompleted && !x.data.GrantsReady {
			s.generateEndgameRiftGrants()
			r.Revision++
		}
		if x.saving == nil && x.savedRevision < r.Revision && (r.Terminal() || now-x.lastSave >= 5000) {
			if backend, ok := s.endgameBackend.(domain.EndgameRunBackend); ok {
				x.savingRevision = r.Revision
				record := x.data.record(x.revision + 1)
				x.saving = backend.SaveRun(record, x.revision)
				x.lastSave = now
			}
		}
	}
	s.syncEndgameRiftCompletion()
	s.stepEndgameRiftAbilities(now)
	if now-x.lastBroadcast >= 1000 {
		x.lastBroadcast = now
		for _, p := range s.players {
			s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
		}
	}
	if r.Terminal() && x.savedRevision >= r.Revision && !x.economicPending {
		if x.terminalUntil == 0 {
			x.terminalUntil = now + 120000
		}
		if now >= x.terminalUntil || len(s.players) == 0 {
			for id := range s.players {
				s.onTeleport(Teleport{ID: id, To: domain.SceneID{MapID: x.data.Return.MapID}, At: x.data.Return})
			}
			s.Stop()
		}
	}
}

// A replayed/lost-ACK character commit can have changed the run even though the
// generic candidate callback must not run. Read the durable run before unlocking.
func (s *Scene) reloadEndgameRift() {
	x := s.rift
	if x == nil {
		return
	}
	x.economicPending = true
	x.reload = s.endgameBackend.LoadRun(x.data.Run.RunID)
}
func (s *Scene) pollEndgameRiftReload(now int64) {
	x := s.rift
	if x.reload == nil {
		if x.reloadAt > 0 && now >= x.reloadAt {
			x.reloadAt = 0
			s.reloadEndgameRift()
		}
		return
	}
	select {
	case result := <-x.reload:
		x.reload = nil
		d, valid := decodeEndgameRift(result.Run, s.riftRules)
		if result.Err != nil || !valid {
			x.reloadAt = now + 1000
			return
		}
		if result.Run.Revision > x.revision {
			x.data = d
			x.revision = result.Run.Revision
			x.savedRevision = d.Run.Revision
		}
		x.economicPending = false
		for id, p := range s.players {
			char := domain.CharID(p.Player.Char.ID)
			if !x.data.Run.Admitted(char) {
				s.router.withdrawRift(s.id, char)
				s.onTeleport(Teleport{ID: id, To: domain.SceneID{MapID: x.data.Return.MapID}, At: x.data.Return})
			}
		}
	default:
	}
}
