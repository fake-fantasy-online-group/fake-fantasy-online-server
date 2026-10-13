package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"time"
)

// Graceful shutdown must not abandon a completion entitlement while the store is
// closing. The router's existing drain deadline reports an unavailable store;
// ambiguous writes remain owned, rather than pretending the reward was saved.
func (s *Scene) finishEndgameRunWrites() {
	backend, ok := s.endgameBackend.(domain.EndgameRunBackend)
	if !ok {
		return
	}
	if x := s.trialRun; x != nil && x.Completed && !x.Saved {
		for !x.Saved {
			if x.Saving == nil {
				c := x.Completion
				b, _ := json.Marshal(c)
				x.Saving = backend.SaveRun(domain.EndgameRunRecord{RunID: "trial-" + x.Nonce, Revision: 1, State: "trial.completed", Payload: string(b)}, 0)
			}
			result := <-x.Saving
			x.Saving = nil
			if result.Err == nil {
				x.Saved = true
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
	x := s.rift
	if x == nil || x.retired {
		return
	}
	// Resolve an accepted floor write before shutdown releases the actor. The
	// durable record is sufficient for ordinary restart compensation.
	if x.transition != nil {
		t := x.transition
		if t.save != nil {
			<-t.save
			t.save = nil
		}
		for {
			if t.load == nil {
				t.load = s.endgameBackend.LoadRun(x.data.Run.RunID)
			}
			result := <-t.load
			t.load = nil
			if d, valid := decodeEndgameRift(result.Run, s.riftRules); result.Err == nil && valid {
				x.data = d
				x.revision = result.Run.Revision
				x.savedRevision = d.Run.Revision
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		x.transition = nil
	}

	if x.saving != nil {
		s.acceptEndgameRiftSave(<-x.saving, s.riftMS())
	}
	s.drainEndgameRiftReload()
	// An authoritative death can have arrived while reconciliation froze
	// economics. Drain it exactly once before deciding what shutdown must save.
	if len(x.delayedDeaths) > 0 {
		deaths := x.delayedDeaths
		x.delayedDeaths = nil
		for _, death := range deaths {
			s.applyEndgameRiftDeath(death)
		}
	}
	if x.data.Run.Phase == domain.RiftCompleted && !x.data.GrantsReady {
		s.generateEndgameRiftGrants()
		x.data.Run.Revision++
	}
	if x.data.Run.Terminal() || x.data.GuardianArrival != nil {
		for x.savedRevision < x.data.Run.Revision {
			record := x.data.record(x.revision + 1)
			x.savingRecord = &record
			x.savingRevision = x.data.Run.Revision
			s.acceptEndgameRiftSave(<-backend.SaveRun(record, x.revision), s.riftMS())
			s.drainEndgameRiftReload()
			if x.savedRevision < x.data.Run.Revision {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

func (s *Scene) drainEndgameRiftReload() {
	x := s.rift
	if x.reload == nil && x.reloadAt == 0 && x.reconcilingSave == nil {
		return
	}
	for {
		if x.reload == nil {
			x.reload = s.endgameBackend.LoadRun(x.data.Run.RunID)
		}
		result := <-x.reload
		x.reload = nil
		if s.acceptEndgameRiftReload(result) {
			x.reloadAt = 0
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
