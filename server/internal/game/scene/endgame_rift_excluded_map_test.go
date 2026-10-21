package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
)

func TestRiftExcludedSavedFloorRemainsReadableForCompensation(t *testing.T) {
	for _, mapID := range []int32{23800, 23900, 24000, 24100, 24200, 24300, 20100, 28502, 28503, 28504, 28505, 24600, 24700, 24800, 36700, 933, 934, 935, 936, 937} {
		s, _, _ := seededRiftFloorFixture(t)
		d := s.rift.data.clone()
		d.MapID = mapID
		d.Floor.MapID = mapID
		d.Floor.Entry.MapID = mapID
		d.Floor.Exit.MapID = mapID
		d.Floor.Guardian.Point.Pos.MapID = mapID
		for i := range d.Floor.Spawns {
			d.Floor.Spawns[i].Point.Pos.MapID = mapID
		}
		record := d.record(d.StorageRevision)
		restored, ok := decodeEndgameRift(&record, s.riftRules)
		if !ok || !reflect.DeepEqual(d.Floor, restored.Floor) || d.Run.Progress != restored.Run.Progress {
			t.Fatalf("map %d saved progress rewritten or unreadable", mapID)
		}
		if !riftNeedsCompensation(restored.Run, false, 100) || riftNeedsCompensation(restored.Run, true, 100) || riftNeedsCompensation(restored.Run, false, 200) {
			t.Fatalf("map %d interrupted payer entitlement changed", mapID)
		}
		if _, live := s.router.riftScene(restored.Run.RunID); !live {
			t.Fatal("fixture live run missing")
		}
		// Restart loses the live actor registry, not the durable run record.
		s.router.riftAdmissions = nil
		if _, live := s.router.riftScene(restored.Run.RunID); live {
			t.Fatal("restart revived excluded saved floor")
		}
		if v := s.riftView(s.players[1], restored, true, false); v.Phase != "interrupted" || !v.Claimable {
			t.Fatalf("map %d missing interruption compensation", mapID)
		}
		restored.Run.Phase = domain.RiftCompleted
		if riftNeedsCompensation(restored.Run, false, 100) {
			t.Fatal("completed reward replaced with compensation")
		}
	}
}
