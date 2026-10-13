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
	if x.reload != nil || x.reloadAt > 0 {
		for {
			if x.reload == nil {
				x.reload = s.endgameBackend.LoadRun(x.data.Run.RunID)
			}
			result := <-x.reload
			x.reload = nil
			d, valid := decodeEndgameRift(result.Run, s.riftRules)
			if result.Err == nil && valid {
				if result.Run.Revision > x.revision {
					x.data = d
					x.revision = result.Run.Revision
					x.savedRevision = d.Run.Revision
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	if x.saving != nil {
		result := <-x.saving
		x.saving = nil
		if result.Err == nil && result.Run != nil {
			x.revision = result.Run.Revision
			x.savedRevision = x.savingRevision
		}
	}
	if x.data.Run.Phase == domain.RiftCompleted && !x.data.GrantsReady {
		s.generateEndgameRiftGrants()
		x.data.Run.Revision++
	}
	if x.data.Run.Terminal() {
		for x.savedRevision < x.data.Run.Revision {
			record := x.data.record(x.revision + 1)
			result := <-backend.SaveRun(record, x.revision)
			if result.Err == nil && result.Run != nil {
				x.revision = result.Run.Revision
				x.savedRevision = x.data.Run.Revision
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}
