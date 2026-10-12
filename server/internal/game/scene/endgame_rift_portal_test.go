package scene

import (
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func nativeRiftTestPortal(s *Scene) domain.Portal {
	at := s.rift.data.Floor.Entry
	return domain.NewPortal(domain.Teleport{ProcID: 42, To: domain.Pos{MapID: 999, X: 500, Y: 500}, Area: domain.Polygon{
		{MapID: s.id.MapID, X: at.X - 20, Y: at.Y - 20}, {MapID: s.id.MapID, X: at.X + 20, Y: at.Y - 20},
		{MapID: s.id.MapID, X: at.X + 20, Y: at.Y + 20}, {MapID: s.id.MapID, X: at.X - 20, Y: at.Y + 20},
	}})
}

func TestRiftNativePortalDirectFloorHandoff(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t, domain.TrialKeyMember{CharID: 100, Level: 60}, domain.TrialKeyMember{CharID: 200, Level: 60})
	portal := nativeRiftTestPortal(s)
	s.rift.portals = []domain.Portal{portal}
	p := s.players[1]
	p.Player.EndgameRiftPortals = true
	p.Pos = s.rift.data.Floor.Entry
	p.Player.Party = 73
	join(t, s, 2, 200, "乙", p.Pos.X, p.Pos.Y)
	s.players[2].Player.Party = 73
	s.rift.ready[200] = true
	old := s.rift.data.clone()
	if riftDistance2(p.Pos, old.Floor.Exit) <= 160*160 || !s.canAdvanceEndgameRift(p) {
		t.Fatal("native portal must be independent of seeded exit")
	}
	target := prepareRiftFloorTarget(t, s)
	req := advanceRiftFloorRequest(s)
	s.advanceEndgameRift(p, req)
	s.advanceEndgameRift(p, req)
	if len(b.writes) != 1 || len(s.players) != 2 || s.router.scenes[domain.SceneID{MapID: 999}] != nil {
		t.Fatal("portal entered public map or duplicated floor write")
	}
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	target.drain()
	target.stepEndgameRift()
	next := target.rift.data
	if target.players[1] == nil || target.players[2] == nil || target.players[1].Player.Party != 73 || target.players[2].Player.Party != 73 {
		t.Fatal("portal split party")
	}
	if next.Run.RunID != old.Run.RunID || next.Run.Tier != old.Run.Tier || next.Run.Progress != old.Run.Progress || next.Run.StartedMS != old.Run.StartedMS || next.Run.PenaltyMS != old.Run.PenaltyMS || next.Floor.FloorIndex != old.Floor.FloorIndex+1 {
		t.Fatal("portal reset shared run state")
	}
	expected, err := s.planEndgameRiftFloor(old.Run.RunID, 2, old.MapID)
	if err != nil || !reflect.DeepEqual(*next.Floor, expected) {
		t.Fatal("portal replaced deterministic floor seed/plan")
	}
	if b.commits != 0 || target.players[1].Player.Bag.CountOf(domain.RiftKeyItemID) != 3 {
		t.Fatal("portal charged another key")
	}
	target.players[1].Pos = next.Floor.Exit
	target.advanceEndgameRift(target.players[1], req)
	if len(b.writes) != 1 {
		t.Fatal("old gate request skipped another floor")
	}
	target.flush()
	evs := p.Player.Sink.(*fakeSink).take()
	contextIndex, teleportIndex := -1, -1
	for i, ev := range evs {
		if message, ok := ev.(event.EndgameMessage); ok && message.Message.Kind == "rift.context" && message.Message.Result["inInstance"] == "true" {
			contextIndex = i
		}
		if tp, ok := ev.(event.Teleported); ok {
			if tp.Scene == target.id {
				teleportIndex = i
			}
			if tp.Scene.Persistent() {
				t.Fatal("portal emitted public teleport")
			}
		}
	}
	if contextIndex < 0 || teleportIndex < contextIndex {
		t.Fatal("private context did not precede authoritative map load")
	}
}

func TestRiftNativePortalGeometryAdmission(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	s.rift.portals = []domain.Portal{nativeRiftTestPortal(s)}
	p := s.players[1]
	p.Player.EndgameRiftPortals = true
	at := s.rift.data.Floor.Entry
	for _, tc := range []struct {
		name   string
		dx, dy float64
		want   bool
	}{
		{"inside", 0, 0, true}, {"within_movement_tolerance", 170, 0, true}, {"outside_tolerance", 181, 0, false}, {"outside_corner", 150, 150, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.Pos = at
			p.Pos.X += tc.dx
			p.Pos.Y += tc.dy
			if got := s.canAdvanceEndgameRift(p); got != tc.want {
				t.Fatalf("advance=%v want %v", got, tc.want)
			}
		})
	}
	p.Pos = at
	p.Pos.MapID++
	if s.canAdvanceEndgameRift(p) {
		t.Fatal("accepted wrong map identity")
	}
	p.Pos = at
	s.rift.data.Run.Phase = domain.RiftBoss
	if s.canAdvanceEndgameRift(p) {
		t.Fatal("portal bypassed guardian fight")
	}
	s.rift.data.Run.Phase = domain.RiftActive
	s.rift.data.Run.Payer = 200
	s.advanceEndgameRift(p, advanceRiftFloorRequest(s))
	if len(b.writes) != 0 {
		t.Fatal("non-opener advanced party")
	}
}

func TestRiftNativePortalPreservesOnlyGeometry(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	portal := nativeRiftTestPortal(s)
	d := s.rift.data.clone()
	s.rift = nil
	s.portals = []domain.Portal{portal}
	s.initEndgameRift(d)
	if len(s.portals) != 0 || len(s.rift.portals) != 1 || !reflect.DeepEqual(s.rift.portals[0], portal) {
		t.Fatal("rift failed to isolate public gate destination")
	}
	s.initEndgameRift(d)
	if len(s.rift.portals) != 1 {
		t.Fatal("reinitialization lost gate geometry")
	}
}

func TestRiftSceneContextReflectsActorNotMapOrSavedRun(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	p := s.players[1]
	p.Player.EndgameRiftPortals = true
	check := func(want string) {
		t.Helper()
		s.sendRiftSceneContext(p)
		s.flush()
		evs := p.Player.Sink.(*fakeSink).take()
		for _, ev := range evs {
			if message, ok := ev.(event.EndgameMessage); ok && message.Message.Kind == "rift.context" {
				if message.Message.Result["inInstance"] != want {
					t.Fatal("incorrect actor context")
				}
				return
			}
		}
		t.Fatal("no scene context")
	}
	check("true")
	s.rift.data.Run.Phase = domain.RiftCompleted
	check("true")
	s.rift.data.Run.Members[100].Departed = true
	check("true") // The durable leave is not a completed map handoff yet.
	s.rift.data.Run.Members[100].Departed = false
	private := s.rift
	s.rift = nil
	check("false")

	s.rift = private
	s.id.Instance = 0
	check("false")
}

func TestRiftSceneContextRepeatedAfterHello(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	p := s.players[1]
	p.Player.EndgameRiftPortals = true
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPortalsV1}})
	s.flush()
	evs := p.Player.Sink.(*fakeSink).take()
	hello, context, snapshot := -1, -1, -1
	for i, ev := range evs {
		if m, ok := ev.(event.EndgameMessage); ok {
			switch m.Message.Kind {
			case "hello":
				hello = i
			case "rift.context":
				context = i
			case "snapshot":
				if snapshot < 0 {
					snapshot = i
				}
			}
		}
	}
	if hello < 0 || context <= hello || snapshot <= context {
		t.Fatalf("reconnect context order hello=%d context=%d snapshot=%d", hello, context, snapshot)
	}
}

func TestRiftPublicExitContextFollowsAuthoritativeTeleport(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	p := s.players[1]
	p.Player.EndgameRiftPortals = true
	// Exercise the real existing-only actor handoff to the public version of the
	// SAME map: map identity cannot be used to infer ownership in either direction.
	delete(s.dungeons, s.id.MapID) // This case models reused wilderness, not the fixture dungeon.
	publicID := domain.SceneID{MapID: s.id.MapID}
	public := New(Config{ID: publicID, Now: s.now, Alloc: s.alloc, Defs: s.defs, Items: s.items, EndgameBackend: s.endgameBackend})
	public.router = s.router
	s.router.scenes[publicID] = public
	s.rift.data.Run.Members[100].Departed = true
	if !s.onTeleport(Teleport{ID: p.ID, To: publicID, At: p.Pos}) {
		t.Fatal("authorized leave failed")
	}
	public.drain()
	public.flush()
	contextIndex, teleportIndex := -1, -1
	for i, ev := range p.Player.Sink.(*fakeSink).take() {
		if m, ok := ev.(event.EndgameMessage); ok && m.Message.Kind == "rift.context" && m.Message.Result["inInstance"] == "false" {
			contextIndex = i
		}
		if m, ok := ev.(event.Teleported); ok && m.Scene == publicID {
			teleportIndex = i
		}
	}
	if teleportIndex < 0 || contextIndex <= teleportIndex {
		t.Fatalf("public context released old portal before map edge: teleport=%d context=%d", teleportIndex, contextIndex)
	}
}

func TestRiftLegacyClientDoesNotReceiveNewContext(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	p := s.players[1]
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: []string{domain.EndgameV1}})
	s.sendRiftSceneContext(p)
	s.flush()
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if m, ok := ev.(event.EndgameMessage); ok && m.Message.Kind == "rift.context" {
			t.Fatal("unnegotiated legacy client received new context")
		}
	}
	if p.Player.EndgameRiftPortals {
		t.Fatal("legacy client acquired portal capability")
	}
}
