package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestRiftAdmittedMemberAdvancesWithoutOpener(t *testing.T) {
	for _, scenario := range []string{"present", "native_portal", "disconnected", "departed"} {
		t.Run(scenario, func(t *testing.T) {
			s, b, _ := seededRiftFloorFixture(t, domain.TrialKeyMember{CharID: 100, Level: 60}, domain.TrialKeyMember{CharID: 200, Level: 60})
			opener := s.players[1]
			join(t, s, 2, 200, "guest", opener.Pos.X, opener.Pos.Y)
			guest := s.players[2]
			guest.Pos = s.rift.data.Floor.Exit
			s.rift.ready[200] = true
			switch scenario {
			case "disconnected":
				s.exec(Leave{ID: opener.ID, Reason: "opener disconnected"})
			case "departed":
				s.leaveEndgameRift(opener, domain.EndgameRequest{Op: "rift.leave", RequestID: "opener-leave-floor-test", ExpectedRevision: "0"})
				b.finish(true)
				s.finishEndgame(false)
				if s.rift.data.Run.Admitted(100) || s.rift.data.Run.Terminal() {
					t.Fatal("opener departure was not durably committed")
				}
				if s.canAdvanceEndgameRift(opener) {
					t.Fatal("departed opener retained transfer permission")
				}
				s.exec(Leave{ID: opener.ID, Reason: "departed opener disconnects"})
			}
			s.clearRiftMonsters()
			s.rift.data.Run.Monsters = map[domain.EntityID]*domain.RiftMonster{}
			s.rift.data.Run.Progress = 9999
			s.rift.portals = nil
			if scenario == "native_portal" {
				s.rift.portals = []domain.Portal{nativeRiftTestPortal(s)}
				guest.Pos = s.rift.data.Floor.Entry
			}
			keys := guest.Player.Bag.CountOf(domain.RiftKeyItemID)
			commits := b.commits
			if !s.canAdvanceEndgameRift(guest) || !s.riftView(guest, s.rift.data, true, true).CanAdvance {
				t.Fatal("admitted guest cannot use exit or HUD")
			}
			oldFloor := s.rift.data.Floor.FloorIndex
			target := prepareRiftFloorTarget(t, s)
			req := advanceRiftFloorRequest(s)
			s.advanceEndgameRift(guest, req)
			// Actor serialization must fence requests from both the same and another member.
			s.advanceEndgameRift(guest, req)
			if scenario == "present" {
				s.advanceEndgameRift(opener, req)
			}
			if len(b.writes) != 1 || s.riftView(guest, s.rift.data, true, true).CanAdvance {
				t.Fatal("duplicate request escaped transition fence")
			}
			b.finishFloorWrite(t, 0, true, false)
			s.stepEndgameRift()
			target.drain()
			target.stepEndgameRift()
			if target.players[guest.ID] == nil || target.rift.data.Floor.FloorIndex != oldFloor+1 || target.rift.data.Run.Progress != 9999 || target.rift.data.Run.Payer != 100 {
				t.Fatal("member transfer lost run state or economic payer")
			}
			if b.commits != commits || guest.Player.Bag.CountOf(domain.RiftKeyItemID) != keys {
				t.Fatal("floor transition performed economic operation")
			}
			guest.Pos = target.rift.data.Floor.Exit
			target.advanceEndgameRift(guest, req)
			requireRiftFloorError(t, target, guest, "stale_floor")
			if len(b.writes) != 1 {
				t.Fatal("old floor request advanced twice")
			}
		})
	}
}
