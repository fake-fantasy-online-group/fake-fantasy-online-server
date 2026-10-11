package scene

import (
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// A new session enters with a new entity ID and no Runtime. Do not call
// syncEndgameRift directly: that used to hide the missing login readiness hook.
func reconnectRiftPlayer(t *testing.T, s *Scene, snapshot domain.Snapshot, id domain.EntityID, party domain.PartyID, runtime *entity.PlayerRuntime, sink event.Sink) *entity.Entity {
	t.Helper()
	s.exec(Enter{ID: id, Char: snapshot.Char, Bag: snapshot.Bag, Worn: snapshot.Worn,
		ChangeSet: snapshot.ChangeSet, Warehouse: snapshot.Warehouse, Wardrobe: snapshot.Wardrobe,
		Party: party, Runtime: runtime, Sink: sink})
	p := s.players[id]
	if p == nil {
		t.Fatal("admitted reconnect did not enter its existing scene")
	}
	return p
}

func TestRiftLoginReconnectRestoresPresence(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "seeded"}[seeded], func(t *testing.T) {
			var s *Scene
			var now *int64
			if seeded {
				s, _, now = seededRiftFloorFixture(t)
			} else {
				var b *riftTestBackend
				s, b, now = riftFixture(t)
				activateTestRift(t, s, b, *now)
				s.id.Instance = 90
				s.router.scenes = map[domain.SceneID]*Scene{s.id: s}
				s.router.riftAdmissions = map[domain.SceneID]*endgameRiftAdmission{s.id: {
					Data: s.rift.data.clone(), Paid: true, Allowed: map[domain.CharID]bool{100: true},
				}}
			}
			before := s.rift.data.clone()
			monsters := make(map[domain.EntityID]*entity.Entity, len(s.monsters))
			for id, monster := range s.monsters {
				monsters[id] = monster
			}
			p := s.players[1]
			keys := p.Player.Bag.CountOf(domain.RiftKeyItemID)
			for i := 0; i < 2; i++ {
				snapshot := s.snapshotOf(p).Clone()
				oldID := p.ID
				lastSeen := s.rift.data.Run.Members[100].LastSeenMS
				presentMS := s.rift.data.Run.Members[100].PresentMS
				s.exec(Leave{ID: oldID, Reason: "reconnect test"})
				if len(s.players) != 0 || s.rift.ready[100] {
					t.Fatal("disconnect retained a live player or readiness")
				}
				*now += 63000
				s.stepEndgameRift()
				if s.rift.data.Run.Terminal() || s.rift.data.Run.Members[100].PresentMS != presentMS {
					t.Fatal("disconnect lost the grace window or earned offline presence")
				}
				sink := &fakeSink{}
				p = reconnectRiftPlayer(t, s, snapshot, oldID+1000000, 0, nil, sink)
				s.flush()
				if _, ok := firstOf[event.SelfEntered](sink.take()); !ok {
					t.Fatal("reconnect did not follow the fresh-login bootstrap")
				}
				if !s.rift.ready[100] || s.rift.data.Run.Members[100].LastSeenMS != *now {
					t.Fatal("fresh-login entry left the admitted player absent from rift presence")
				}
				// A delayed cleanup for the previous session must not remove the
				// new entity's presence. No extra map-ready packet is required.
				s.exec(Leave{ID: oldID, Reason: "late old-session cleanup"})
				*now = lastSeen + domain.RiftReconnectMS + 4
				s.stepEndgameRift()
				r := s.rift.data.Run
				if r.Terminal() || !s.rift.ready[100] || r.Members[100].LastSeenMS != *now || r.Members[100].PresentMS <= presentMS {
					t.Fatalf("online reconnect expired against old disconnect deadline: phase=%s reason=%s member=%+v", r.Phase, r.Reason, r.Members[100])
				}
				if !reflect.DeepEqual(before.Floor, s.rift.data.Floor) || r.RunID != before.Run.RunID || r.StartedMS != before.Run.StartedMS || r.Progress != before.Run.Progress || r.PenaltyMS != before.Run.PenaltyMS || p.Player.Bag.CountOf(domain.RiftKeyItemID) != keys {
					t.Fatal("reconnect changed the paid run, floor, timer, progress or keys")
				}
				if len(s.monsters) != len(monsters) {
					t.Fatal("reconnect changed the population")
				}
				for id, monster := range monsters {
					if s.monsters[id] != monster {
						t.Fatal("reconnect recreated a live monster")
					}
				}
			}
			// A genuine second disconnect still expires after its own grace.
			s.exec(Leave{ID: p.ID, Reason: "abandon"})
			*now += domain.RiftReconnectMS + 1
			s.stepEndgameRift()
			if s.rift.data.Run.Phase != domain.RiftAborted || s.rift.data.Run.Reason != "abandoned" {
				t.Fatal("restored reconnect presence prevented genuine empty-run expiration")
			}
		})
	}
}

func TestRiftLoginReconnectKeepsFrozenPartyAdmission(t *testing.T) {
	s, _, now := seededRiftFloorFixture(t, domain.TrialKeyMember{CharID: 100, Level: 60}, domain.TrialKeyMember{CharID: 200, Level: 60})
	join(t, s, 900000, 200, "乙", 100, 100)
	guest := s.players[900000]
	guest.Player.Party = 777
	snapshot := s.snapshotOf(guest).Clone()
	s.exec(Leave{ID: 1, Reason: "leader disconnected"})
	s.exec(Leave{ID: guest.ID, Reason: "guest disconnected"})
	*now += 63000
	s.stepEndgameRift()
	guest = reconnectRiftPlayer(t, s, snapshot, 900001, 888, nil, &fakeSink{})
	*now += 60000
	s.stepEndgameRift()
	if s.rift.data.Run.Terminal() || !s.rift.ready[200] || s.rift.ready[100] || !s.canStayInDungeon(guest) || !s.rift.data.Run.Admitted(100) || len(s.rift.data.Run.Members) != 2 {
		t.Fatal("guest reconnect after party change lost the frozen roster or presence")
	}
	outsider := snapshot.Clone()
	outsider.Char.ID = 300
	sink := &fakeSink{}
	s.exec(Enter{ID: 900002, Char: outsider.Char, Bag: outsider.Bag, Party: 888, Sink: sink})
	if s.players[900002] != nil || !sink.isClosed() || s.rift.ready[300] {
		t.Fatal("new party membership granted outsider rift presence")
	}
	// Readiness recovery must not extend the shared run's hard deadline.
	*now = s.rift.data.Run.StartedMS + domain.RiftHardMS
	s.stepEndgameRift()
	if s.rift.data.Run.Phase != domain.RiftFailed || s.rift.data.Run.Reason != "timeout" {
		t.Fatal("reconnect reset the shared hard deadline")
	}
}

type riftReconnectLoadingSink struct{ fakeSink }

func (*riftReconnectLoadingSink) BeginMapLoad(domain.EntityID, domain.SceneID) (uint64, bool) {
	return 17, true
}
func (*riftReconnectLoadingSink) CompleteMapLoad(_ domain.EntityID, _ domain.SceneID, epoch uint64) bool {
	return epoch == 17
}

func TestRiftTransferStillRequiresMapReady(t *testing.T) {
	s, _, now := seededRiftFloorFixture(t)
	p := s.players[1]
	snapshot := s.snapshotOf(p).Clone()
	runtime := p.CapturePlayerRuntime(s.tick)
	s.exec(Leave{ID: p.ID, Reason: "transfer test"})
	*now += 1000
	sink := &riftReconnectLoadingSink{}
	p = reconnectRiftPlayer(t, s, snapshot, 900000, 0, &runtime, sink)
	if s.mapLoads[p.ID] == nil || s.rift.ready[100] {
		t.Fatal("asynchronous transfer bypassed the map-load readiness barrier")
	}
	s.exec(MapReady{ID: p.ID, Scene: s.id, Epoch: 16})
	if s.rift.ready[100] {
		t.Fatal("stale map acknowledgement restored presence")
	}
	s.exec(MapReady{ID: p.ID, Scene: s.id, Epoch: 17})
	if s.mapLoads[p.ID] != nil || !s.rift.ready[100] || s.rift.data.Run.Members[100].LastSeenMS != *now {
		t.Fatal("valid map acknowledgement failed to restore presence")
	}
}
