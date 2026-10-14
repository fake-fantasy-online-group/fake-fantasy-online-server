package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// A disconnected eligible member can complete a run without a live entity. The
// character's durable current-run pointer survives login and reward collection;
// recover from that run without requiring the player to claim loot or open UI.
type endgameRiftQuestRecovery struct {
	profileLoad    <-chan domain.EndgameLoadResult
	load           <-chan domain.EndgameRunLoadResult
	runID          string
	checkedProfile bool
	terminal       bool
	retryAfter     int64
}

func (s *Scene) hasPendingRiftQuest(p *entity.Entity) bool {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return false
	}
	for id, entry := range p.Player.Char.Quests {
		if entry.State == domain.QuestFinished || entry.AcceptedAtMS <= 0 || entry.RiftCompletionRunID != "" {
			continue
		}
		if step, ok := p.Player.Char.Quests.CurrentStep(s.quests[id]); ok && step.Kind == domain.QuestStepRiftComplete {
			return true
		}
	}
	return false
}

// Call only for a successfully loaded run or a live run whose completion and
// grants revision has been acknowledged by the durable backend. Merely moving
// between floors, joining the roster, or killing a boss before save is not proof.
func (s *Scene) recordEndgameRiftQuestCompletion(p *entity.Entity, d endgameRiftPersisted) bool {
	if !s.hasPendingRiftQuest(p) || endgameEconomicBusy(p) || d.Run == nil || !d.GrantsReady {
		return false
	}
	r := d.Run
	if r.Phase != domain.RiftCompleted && r.Phase != domain.RiftClosed {
		return false
	}
	char := domain.CharID(p.Player.Char.ID)
	member := r.Members[char]
	if member == nil || !member.Eligible {
		return false
	}
	if _, ok := d.Grants[char]; !ok {
		return false
	}
	if len(p.Player.Char.Quests.RecordRiftCompletion(s.quests, r.RunID, r.CompletedMS)) == 0 {
		return false
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p).Clone())
	}
	s.pushQuestLog(p)
	s.pushQuestOffers(p, p.Player.QuestNPC)
	return true
}

func validEndgameRiftQuestRecord(record *domain.EndgameRunRecord, d endgameRiftPersisted) bool {
	return record != nil && record.Revision > 0 && d.Run != nil && record.RunID == d.Run.RunID && record.State == string(d.Run.Phase)
}

func (s *Scene) stepEndgameRiftQuestRecoveries() {
	if s.endgameBackend == nil || s.rift != nil {
		return
	}
	now := s.riftMS()
	for id := range s.riftQuestRecoveries {
		if !s.hasPendingRiftQuest(s.players[id]) {
			delete(s.riftQuestRecoveries, id)
		}
	}
	for id, p := range s.players {
		if !s.hasPendingRiftQuest(p) || endgameEconomicBusy(p) {
			continue
		}
		if s.riftQuestRecoveries == nil {
			s.riftQuestRecoveries = map[domain.EntityID]*endgameRiftQuestRecovery{}
		}
		x := s.riftQuestRecoveries[id]
		if x == nil {
			x = &endgameRiftQuestRecovery{}
			s.riftQuestRecoveries[id] = x
		}
		if x.profileLoad != nil {
			select {
			case result := <-x.profileLoad:
				x.profileLoad = nil
				if result.Err != nil {
					x.retryAfter = now + 5000
					continue
				}
				x.checkedProfile = true
				x.runID = riftProfile(result.State.Payload).RunID
			default:
				continue
			}
		}
		if x.load != nil {
			select {
			case result := <-x.load:
				x.load = nil
				x.retryAfter = now + 5000
				if result.Err != nil {
					continue
				}
				d, valid := decodeEndgameRift(result.Run, s.riftRules)
				if valid && validEndgameRiftQuestRecord(result.Run, d) {
					s.recordEndgameRiftQuestCompletion(p, d)
					// A completion whose grants are still being frozen must be
					// checked again; terminal phase alone is not full evidence.
					x.terminal = d.Run.Terminal() && (d.GrantsReady || (d.Run.Phase != domain.RiftCompleted && d.Run.Phase != domain.RiftClosed))
				}
			default:
				continue
			}
		}
		if !s.hasPendingRiftQuest(p) {
			delete(s.riftQuestRecoveries, id)
			continue
		}
		// A new admission changes this authoritative profile before teleport.
		// The prior-run load also credits completion before replacing the ID.
		if state := s.endgamePlayers[id]; state != nil && state.loaded {
			runID := riftProfile(state.state.Payload).RunID
			if runID != x.runID {
				x.runID, x.terminal, x.retryAfter = runID, false, 0
			}
			x.checkedProfile = true
		}
		if now < x.retryAfter {
			continue
		}
		if !x.checkedProfile {
			x.profileLoad = s.endgameBackend.Load(p.Player.Char.ID)
		} else if x.runID != "" && !x.terminal {
			x.load = s.endgameBackend.LoadRun(x.runID)
		}
	}
}
