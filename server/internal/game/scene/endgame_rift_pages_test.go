package scene

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

func fullCoverageSceneFixture(t *testing.T, width, height int) *Scene {
	t.Helper()
	s, _ := denseCoverageSceneFixture(t, 22, 22, 1, 1)
	old := s.riftRules.Active()
	fresh := *old
	fresh.Version, fresh.FloorLayoutVersion = 10, domain.RiftFloorFullCoverageVersion
	items := balanceItemsFixture()
	items[1234] = domain.ItemDef{ID: 1234, Equip: &domain.EquipDef{LevelReq: 40}}
	var err error
	s.riftRules, err = domain.NewRiftRuleTable(10, []domain.RiftRules{*riftRulesFixture(), *old, fresh}, lootRulesTableFixture(), items, riftRulesTableFixture().Entrances()...)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Run.Rules, s.rift.data.Run.RulesVersion = &fresh, 10
	region := domain.RiftCombatRegion{CombatArea: float64(width * height * 64 * 64)}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			region.Population = append(region.Population, domain.SpawnPoint{ID: -int32(len(region.Population) + 1), Pos: domain.Pos{MapID: 7, X: float64(32 + x*64), Y: float64(32 + y*64)}})
		}
	}
	s.riftLayouts[0].FullCombatRegions = []domain.RiftCombatRegion{region}
	s.walkable = func(p domain.Pos) bool {
		return p.MapID == 7 && p.X >= 0 && p.Y >= 0 && p.X < float64(width*64) && p.Y < float64(height*64)
	}
	plan, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Floor = &plan
	s.players[1].Player.EndgameRiftPages = true
	return s
}

func TestRiftFullCoverageScenePrepopulatesWithoutCountCeiling(t *testing.T) {
	for _, tc := range []struct{ side, count int }{{22, 36}, {88, 474}, {160, 1568}, {224, 3072}} {
		t.Run(fmt.Sprint(tc.side), func(t *testing.T) {
			s := fullCoverageSceneFixture(t, tc.side, tc.side)
			p := s.players[1]
			plan := s.rift.data.Floor
			if plan.GeneratorVersion != 6 || len(plan.Spawns) != tc.count {
				t.Fatalf("population=%d want=%d", len(plan.Spawns), tc.count)
			}
			s.populateEndgameRift(false)
			if len(s.monsters) != tc.count || len(s.rift.data.Run.Monsters) != tc.count {
				t.Fatal("not all monsters pre-generated")
			}
			for _, m := range s.monsters {
				if !s.walkable(m.Pos) {
					t.Fatal("unsafe native coordinate")
				}
			}
			view := s.riftView(p, s.rift.data, true, true)
			if len(view.Auras) != tc.count {
				t.Fatal("sidecar hides live actors")
			}
			s.flush()
			p.Player.Sink.(*fakeSink).take()
			s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
			s.flush()
			var frames int
			for _, ev := range p.Player.Sink.(*fakeSink).take() {
				if m, ok := ev.(event.EndgameMessage); ok {
					if !m.Message.OK {
						t.Fatalf("snapshot failed: %+v", m.Message)
					}
					frames++
				}
			}
			if frames == 0 || tc.count > 2048 && frames < 2 {
				t.Fatal("large snapshot was not delivered in pages")
			}
			body, _ := json.Marshal(domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: view})
			t.Logf("all %d monsters simultaneously registered, full snapshot %d bytes, %d frames", tc.count, len(body), frames)
		})
	}
}

func TestRiftPagesOptInIsLightweightAndConnectionScoped(t *testing.T) {
	s, _, _ := riftFixture(t)
	p := s.players[1]
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: []string{domain.EndgameV1}})
	s.flush()
	events := p.Player.Sink.(*fakeSink).take()
	found := false
	for _, ev := range events {
		if m, ok := ev.(event.EndgameMessage); ok && m.Message.Kind == "hello" {
			found = m.Message.Result["riftPages"] == "1"
		}
	}
	if !found || p.Player.EndgameRiftPages {
		t.Fatal("offer must not grant capability")
	}
	req := domain.EndgameRequest{Op: "capabilities", RequestID: "caps-1", Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPagesV1}}
	s.dispatchEndgame(p, req)
	s.flush()
	events = p.Player.Sink.(*fakeSink).take()
	if len(events) != 1 {
		t.Fatalf("opt-in refreshed unrelated state: %d events", len(events))
	}
	ack, ok := events[0].(event.EndgameMessage)
	if !ok || !ack.Message.OK || ack.Message.Kind != "capabilities" || ack.Message.RequestID != req.RequestID || !p.Player.EndgameRiftPages {
		t.Fatal("missing explicit capability ack")
	}
	runtime := p.CapturePlayerRuntime(s.tick)
	if !runtime.EndgameRiftPages {
		t.Fatal("map transfer lost capability")
	}
	fresh := &entity.Entity{Player: &entity.Player{}}
	if fresh.Player.EndgameRiftPages {
		t.Fatal("fresh connection inherited capability")
	}
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "capabilities", Capabilities: []string{domain.EndgameV1}})
	if !p.Player.EndgameRiftPages {
		t.Fatal("failed opt-in revoked existing capability")
	}
}

func TestRiftPagesOldClientAndPartyFailBeforeKeyCharge(t *testing.T) {
	for _, party := range []bool{false, true} {
		t.Run(fmt.Sprint(party), func(t *testing.T) {
			s := fullCoverageSceneFixture(t, 88, 88)
			p := s.players[1]
			b := s.endgameBackend.(*riftTestBackend)
			s.rift = nil
			s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "八仙使者", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"}}
			p.Player.QuestNPC = "八仙使者"
			p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 3, Bound: true})
			if party {
				join(t, s, 2, 200, "旧版队员", 110, 110)
				peer := s.players[2]
				peer.Player.Char.Level = 60
				peer.HP = 100
				p.Player.Party, peer.Player.Party = 1, 1
				s.dungeonParty = func(domain.CharID) domain.DungeonParty { return domain.DungeonParty{Leader: 100} }
			} else {
				p.Player.EndgameRiftPages = false
			}
			s.flush()
			p.Player.Sink.(*fakeSink).take()
			s.startEndgameRift(p, domain.EndgameRequest{Op: "rift.start", RequestID: "new-density-start", NPCEntityID: 77, Tier: 1})
			s.flush()
			found := false
			for _, ev := range p.Player.Sink.(*fakeSink).take() {
				if m, ok := ev.(event.EndgameMessage); ok && m.Message.Error == "update_required" {
					found = true
				}
			}
			if !found || b.commits != 0 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 || len(s.router.riftAdmissions) != 0 {
				t.Fatal("old client charged or admission was allocated")
			}
		})
	}
}

func TestRiftPagesNewClientPaysExactlyOneKey(t *testing.T) {
	s := fullCoverageSceneFixture(t, 88, 88)
	p := s.players[1]
	b := s.endgameBackend.(*riftTestBackend)
	s.rift = nil
	frames := make(chan time.Time)
	router := NewRouter(func(id domain.SceneID) (*Scene, error) {
		return New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frames, Trial: s.trial, Dungeons: s.dungeons}), nil
	}, nil)
	router.Start(context.Background())
	defer router.Drain(time.Second)
	s.router = router
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "八仙使者", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"}}
	p.Player.QuestNPC = "八仙使者"
	p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 3, Bound: true})
	s.startEndgameRift(p, domain.EndgameRequest{Op: "rift.start", RequestID: "new-density-start", ExpectedRevision: "0", NPCEntityID: 77, Tier: 1})
	if b.commits != 1 || b.plan.Snapshot.Bag.CountOf(domain.RiftKeyItemID) != 2 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 {
		t.Fatal("new client did not reserve atomic one-key admission")
	}
	b.finish(true)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 2 {
		t.Fatal("wrong committed debit")
	}
}

func TestRiftPagesJoinAndAdvanceRejectOldClientWithoutMutation(t *testing.T) {
	s := fullCoverageSceneFixture(t, 88, 88)
	p := s.players[1]
	p.Player.EndgameRiftPages = false
	b := s.endgameBackend.(*riftTestBackend)
	before := s.rift.data.clone()
	s.flush()
	p.Player.Sink.(*fakeSink).take()
	s.advanceEndgameRift(p, domain.EndgameRequest{Op: "rift.advance", RunID: before.Run.RunID, FloorIndex: 1})
	if s.rift.transition != nil || !reflect.DeepEqual(s.rift.data.Floor, before.Floor) {
		t.Fatal("old client advanced a floor")
	}
	s.rift = nil
	s.joinEndgameRift(p, domain.EndgameRequest{Op: "rift.start", RunID: before.Run.RunID}, before)
	s.flush()
	rejected := 0
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if m, ok := ev.(event.EndgameMessage); ok && m.Message.Error == "update_required" {
			rejected++
		}
	}
	if rejected != 2 || b.commits != 0 || s.timer.pending() != 0 {
		t.Fatal("join/advance failed without an explicit non-mutating rejection")
	}
}

func TestRiftPagesReconnectWaitsSafelyThenAutomaticallyResumes(t *testing.T) {
	s := fullCoverageSceneFixture(t, 88, 88)
	s.id = domain.SceneID{MapID: 7, Instance: 90}
	s.rift.data.Return = domain.Pos{MapID: 9, X: 100, Y: 100}
	s.populateEndgameRift(false)
	s.rift.data.Run.Progress = 123
	s.rift.data.Run.PenaltyMS = 5000
	before := s.rift.data.clone()
	p := s.players[1]
	snapshot := s.snapshotOf(p).Clone()
	s.exec(Leave{ID: 1, Reason: "fresh session"})
	s.router.scenes = map[domain.SceneID]*Scene{s.id: s}
	s.router.riftAdmissions = map[domain.SceneID]*endgameRiftAdmission{s.id: {Data: before.clone(), Paid: true, Allowed: map[domain.CharID]bool{100: true}}}
	id, entry, safe, runID, ok := s.router.PagedRiftReconnect(100)
	if !ok || id != s.id || entry != before.Floor.Entry || safe != before.Return || runID != before.Run.RunID {
		t.Fatal("missing safe reconnect routing")
	}
	if s.prepareEndgameRift(Enter{Char: snapshot.Char}) {
		t.Fatal("unnegotiated login entered dense instance")
	}
	public := New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: safe.MapID}, Now: s.now, Alloc: s.alloc, Defs: s.defs, Items: s.items, Trial: s.trial, EndgameBackend: s.endgameBackend, RiftRules: s.riftRules})
	public.router = s.router
	s.router.scenes[public.id] = public
	snapshot.Char.Pos, snapshot.Char.SceneInstance = safe, 0
	sink := &fakeSink{}
	public.exec(Enter{ID: 900000, Char: snapshot.Char, Bag: snapshot.Bag, Worn: snapshot.Worn, ChangeSet: snapshot.ChangeSet, Warehouse: snapshot.Warehouse, Wardrobe: snapshot.Wardrobe, Sink: sink})
	public.flush()
	sink.take()
	reconnected := public.players[900000]
	if reconnected == nil || reconnected.Player.EndgameRiftPages || s.players[900000] != nil {
		t.Fatal("fresh login did not remain safe before negotiation")
	}
	public.onEndgameRift(reconnected, domain.EndgameRequest{Op: "rift.state"})
	public.flush()
	found := false
	for _, ev := range sink.take() {
		if m, ok := ev.(event.EndgameMessage); ok && m.Message.Error == "update_required" {
			found = true
		}
	}
	if !found || public.players[900000] == nil {
		t.Fatal("old client has no explicit recoverable update message")
	}
	public.dispatchEndgame(reconnected, domain.EndgameRequest{Op: "capabilities", RequestID: "reconnect-pages", Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPagesV1}})
	public.flush()
	events := sink.take()
	if len(events) != 1 {
		t.Fatal("opt-in replayed unrelated inventory")
	}
	if m, ok := events[0].(event.EndgameMessage); !ok || m.Message.Kind != "capabilities" || !m.Message.OK {
		t.Fatal("capability ack must precede transfer")
	}
	public.timer.advance(public.tick + 1)
	s.drain()
	s.flush()
	resumed := s.players[900000]
	if resumed == nil || !resumed.Player.EndgameRiftPages || public.players[900000] != nil || len(s.monsters) != len(before.Floor.Spawns) {
		t.Fatal("capability did not automatically restore existing instance")
	}
	if s.rift.data.Run.Progress != 123 || s.rift.data.Run.PenaltyMS != 5000 || s.rift.data.Run.StartedMS != before.Run.StartedMS || !reflect.DeepEqual(s.rift.data.Floor, before.Floor) || s.endgameBackend.(*riftTestBackend).commits != 0 {
		t.Fatal("reconnect reset run or charged another key")
	}
	if !s.rift.ready[100] {
		t.Fatal("new client did not regain readiness")
	}
	s.router.withdrawRift(s.id, 100)
	if _, _, _, _, ok := s.router.PagedRiftReconnect(100); ok {
		t.Fatal("withdrawn player may not auto rejoin")
	}
}

func TestRiftPagesLargestNativeFloorFitsCompleteWireEnvelope(t *testing.T) {
	path := os.Getenv("FFO_RIFT_FULL_WIRE_PLAN")
	if path == "" {
		t.Skip("set FFO_RIFT_FULL_WIRE_PLAN from native catalog audit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan domain.RiftFloorPlan
	if json.Unmarshal(raw, &plan) != nil || !plan.Valid() {
		t.Fatal("invalid audited native floor")
	}
	s := fullCoverageSceneFixture(t, 22, 22)
	if err := s.riftRules.Active().ValidateFloorWire(plan); err != nil {
		t.Fatal(err)
	}
	t.Logf("native map %d %s: all %d spawns plus conservative elite, 96 clone, globe and hazard envelope fits 1MiB / 65536-node protection", plan.MapID, plan.MapName, len(plan.Spawns))
	// No actor-count ceiling: even a larger future geometry is valid until its
	// actual generic resource envelope would fail before admission, never truncate.
	plan.Spawns = make([]domain.RiftFloorSpawn, 30000)
	if err := s.riftRules.Active().ValidateFloorWire(plan); err == nil {
		t.Fatal("invalid resource envelope accepted")
	}
}
