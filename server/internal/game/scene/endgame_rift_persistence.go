package scene

import (
	"reflect"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// These acknowledgement helpers are shared by normal stepping and shutdown.
// A run snapshot is an observation: newer actor-owned kills, progress and grants
// must survive an uncertain acknowledgement of that older observation.
func (s *Scene) acceptEndgameRiftSave(result domain.EndgameRunSaveResult, now int64) {
	x := s.rift
	x.saving = nil
	if result.Err == nil && result.Run != nil && result.Run.Revision == x.revision+1 {
		x.revision = result.Run.Revision
		x.savedRevision = x.savingRevision
	} else {
		s.log.Warn("洞天快照写入待核对", "run", x.data.Run.RunID, "revision", x.revision, "err", result.Err)
		x.lastSave = now
		x.saveRetryAt = now + 1000
		x.reconcilingSave = x.savingRecord
		s.reloadEndgameRift()
	}
	x.savingRecord = nil
}

func (s *Scene) acceptEndgameRiftReload(result domain.EndgameRunLoadResult) bool {
	x := s.rift
	d, valid := decodeEndgameRift(result.Run, s.riftRules)
	if result.Err != nil || !valid {
		s.log.Debug("洞天存档核对重试", "run", x.data.Run.RunID, "valid", valid, "err", result.Err)
		return false
	}
	if expected := x.reconcilingSave; expected != nil {
		own, validOwn := decodeEndgameRift(expected, s.riftRules)
		switch {
		case result.Run.Revision == expected.Revision && validOwn && reflect.DeepEqual(d, own):
			// Advance only the verified CAS. Keep all changes made since that snapshot.
			x.revision = result.Run.Revision
			x.savedRevision = own.Run.Revision
		case result.Run.Revision == x.revision:
			// Verified rollback: retain live state and retry on the same CAS.
		default:
			// A genuinely competing writer is not our acknowledgement. Stay frozen
			// rather than restoring an unrelated or stale actor snapshot.
			s.log.Debug("洞天存档版本冲突，保持隔离", "run", x.data.Run.RunID, "expected", expected.Revision, "actual", result.Run.Revision)
			return false
		}
		x.reconcilingSave = nil
	} else if result.Run.Revision > x.revision {
		// Character economic commits own a complete aggregate and retain the
		// established durable-reload behavior, separately from snapshot writes.
		x.data = d
		x.revision = result.Run.Revision
		x.savedRevision = d.Run.Revision
	}
	x.economicPending = false
	return true
}
