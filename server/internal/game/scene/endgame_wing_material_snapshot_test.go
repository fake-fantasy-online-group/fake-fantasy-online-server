package scene

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func wingSnapshotScene(t *testing.T) (*Scene, *fakeSink, *cubeTestBackend) {
	t.Helper()
	s, sink, b := cubeSceneFixture(t)
	raw, err := os.ReadFile("../../../config/cube-wing-progression-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg domain.WingProgressionConfig
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, w := range cfg.Items {
		s.items[w.Item] = domain.ItemDef{ID: w.Item, Name: "羽翼", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotBag), Durable: 100, AscensionOnly: w.Ascended}}
	}
	for _, id := range cfg.FeatherIDs {
		s.items[id] = domain.ItemDef{ID: id, Name: "灵羽", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
	}
	s.wingRules, err = domain.NewWingProgressionRules(cfg, s.items)
	if err != nil {
		t.Fatal(err)
	}
	p := s.players[1]
	st := p.Player.Bag.At(200)
	st.Item = 20609
	p.Player.Bag.Set(200, st)
	for i, id := range cfg.FeatherIDs {
		p.Player.Bag.Set(110+i, domain.Stack{Item: id, Count: 100, Bound: true})
	}
	return s, sink, b
}
func wingSnapshotResponses(t *testing.T, s *Scene, sink *fakeSink) []domain.EndgameResponse {
	t.Helper()
	s.sendEndgameSnapshot(s.players[1])
	var out []domain.EndgameResponse
	for _, ev := range cubeTake(s, sink) {
		if msg, ok := ev.(event.EndgameMessage); ok && msg.Message.Kind == "snapshot" {
			out = append(out, msg.Message)
		}
	}
	if len(out) == 0 {
		t.Fatal("no actual scene snapshot")
	}
	return out
}
func exportWingSnapshotFixture(t *testing.T, name, file string, value any) {
	t.Helper()
	dir := os.Getenv("FFO_WING_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	dir = filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, file), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestWingMaterialSnapshotQuoteExecuteNativeInputs(t *testing.T) {
	for _, tc := range []struct {
		name        string
		wing        bool
		materials   []domain.CubeInput
		target      domain.ItemID
		output      domain.ItemID
		outputCount int32
	}{
		{"rune_stack_count1", false, []domain.CubeInput{{UID: "501", Count: 1}}, 0, domain.FirstAstralRune, 1},
		{"automatic_high_change", true, []domain.CubeInput{{ItemID: 25029, Count: 1}}, 20611, 25027, 15},
		{"automatic_low_count1", true, []domain.CubeInput{{ItemID: 25027, Count: 1}}, 20611, 0, 0},
		{"upgrade", true, []domain.CubeInput{{ItemID: 25029, Count: 2}, {ItemID: 25028, Count: 2}}, 20611, 0, 0},
		{"top_upgrade_low", true, []domain.CubeInput{{ItemID: 25027, Count: 4000}}, 21905, 0, 0},
		{"top_upgrade_mid", true, []domain.CubeInput{{ItemID: 25028, Count: 800}}, 21905, 0, 0},
		{"upgrade_low", true, []domain.CubeInput{{ItemID: 25027, Count: 60}}, 20611, 0, 0},
		{"upgrade_mid", true, []domain.CubeInput{{ItemID: 25028, Count: 12}}, 20611, 0, 0},
		{"upgrade_mixed", true, []domain.CubeInput{{ItemID: 25027, Count: 10}, {ItemID: 25028, Count: 5}, {ItemID: 25029, Count: 1}}, 20611, 0, 0},
		{"convert_mixed", true, []domain.CubeInput{{ItemID: 25027, Count: 100}, {ItemID: 25028, Count: 20}, {ItemID: 11682, Count: 1}}, 22863, 0, 0},
		{"convert_light_marker", true, []domain.CubeInput{{ItemID: 25027, Count: 51}, {ItemID: 25029, Count: 6}}, 20609, 0, 0},
		{"convert", true, []domain.CubeInput{{ItemID: 25029, Count: 8}, {ItemID: 11682, Count: 1}}, 22863, 0, 0},
		{"low_to_mid", false, []domain.CubeInput{{ItemID: 25027, Count: 5}}, 0, 25028, 1},
		{"mid_to_low", false, []domain.CubeInput{{ItemID: 25028, Count: 1}}, 0, 25027, 5},
		{"mid_to_high", false, []domain.CubeInput{{ItemID: 25028, Count: 5}}, 0, 25029, 1},
		{"high_to_mid", false, []domain.CubeInput{{ItemID: 25029, Count: 1}}, 0, 25028, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, sink, b := wingSnapshotScene(t)
			p := s.players[1]
			var runeSource domain.Stack
			var runeCost int32
			if tc.name == "rune_stack_count1" {
				p.Player.Bag.Set(200, domain.Stack{})
				recipe, ok := s.cubeRules.Recipe(domain.CubeConvertRuneBase + 1)
				if !ok {
					t.Fatal("missing authored recipe")
				}
				var err error
				s.cubeRules, err = domain.NewCubeRuleTable([]domain.CubeRecipeDef{recipe}, s.items)
				if err != nil {
					t.Fatal(err)
				}
				runeCost = recipe.SourceCount
				id := domain.FirstAstralRune + 1
				runeSource = domain.Stack{Item: id, UID: 501, Count: runeCost + 2, InstanceKind: domain.ItemInstanceSocketCard, Card: s.items[id].CardDefaults, Bound: true}
				runeSource.Card.Bound = true
				p.Player.Bag.Set(120, runeSource)
				for _, m := range recipe.Materials {
					tc.materials = append(tc.materials, domain.CubeInput{ItemID: m.ItemID, Count: 1})
				}
			}
			if tc.target == 21905 {
				st := p.Player.Bag.At(200)
				st.Item = 20625
				p.Player.Bag.Set(200, st)
				for i, id := range []domain.ItemID{25027, 25028, 25029, 11682} {
					p.Player.Bag.Set(110+i, domain.Stack{Item: id, Count: 5000, Bound: true})
				}
			}
			if tc.target == 20609 {
				st := p.Player.Bag.At(200)
				st.Item = 22863
				p.Player.Bag.Set(200, st)
			}
			before := p.Player.Bag.Clone()
			snapshots := wingSnapshotResponses(t, s, sink)
			exportWingSnapshotFixture(t, tc.name, "snapshot.json", snapshots)
			nativeBag := []map[string]any{}
			p.Player.Bag.Each(func(slot int, st domain.Stack) {
				if st.Empty() {
					return
				}
				def := s.items[st.Item]
				nativeBag = append(nativeBag, map[string]any{"itemId": st.Item, "tab": def.InventoryTab, "slot": slot % domain.BagPageSlots, "count": st.Count, "locked": st.Locked, "uid": strconv.FormatInt(st.UID, 10)})
			})
			exportWingSnapshotFixture(t, tc.name, "native-bag.json", nativeBag)

			counts := map[domain.ItemID]int32{}
			for _, snap := range snapshots {
				for _, m := range snap.Materials {
					id := domain.ItemID(m.ItemID)
					if _, dup := counts[id]; dup {
						t.Fatal("duplicate material", id)
					}
					counts[id] = m.Count
				}
			}
			for _, id := range []domain.ItemID{25027, 25028, 25029, 11682} {
				if counts[id] != p.Player.Bag.CountOf(id) {
					t.Fatalf("native client would reject %d: %d", id, counts[id])
				}
			}
			req := domain.EndgameRequest{Op: "cube.quote", RequestID: "wing-snapshot-quote", ExpectedRevision: snapshots[0].Revision}
			if tc.wing {
				req.CubeInputs = append(req.CubeInputs, domain.CubeInput{UID: "42", Count: 1})
			}
			req.CubeInputs = append(req.CubeInputs, tc.materials...)
			exportWingSnapshotFixture(t, tc.name, "quote-request.json", req)
			// An optional compatibility run feeds this exact scene snapshot through the
			// released C# client and returns its real serialized request. Normal Go tests
			// remain hermetic; the release harness exercises both directions.
			if dir := os.Getenv("FFO_WING_SNAPSHOT_DIR"); dir != "" {
				raw, err := os.ReadFile(filepath.Join(dir, tc.name, "client-quote-request.json"))
				if err == nil {
					var clientRequest domain.EndgameRequest
					if err = json.Unmarshal(raw, &clientRequest); err != nil {
						t.Fatal(err)
					}
					if clientRequest.Op != "cube.quote" || len(clientRequest.CubeInputs) != len(req.CubeInputs) {
						t.Fatal("unexpected client reservation", clientRequest)
					}
					req = clientRequest
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
			s.finishEndgame(false)
			reply, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if !ok || !reply.Message.OK || reply.Message.Quote == nil {
				t.Fatalf("quote: %+v", reply.Message)
			}
			exportWingSnapshotFixture(t, tc.name, "quote-response.json", reply.Message)
			if !reflect.DeepEqual(before, p.Player.Bag) || b.commits != 0 {
				t.Fatal("quote consumed items")
			}
			execute := cubeExecute(*reply.Message.Quote)
			if bridge := os.Getenv("FFO_WING_CLIENT_EXECUTE"); bridge != "" {
				dir := filepath.Join(os.Getenv("FFO_WING_SNAPSHOT_DIR"), tc.name)
				if output, err := exec.Command(bridge, dir).CombinedOutput(); err != nil {
					t.Fatalf("released client confirmation bridge: %v\n%s", err, output)
				}
				raw, err := os.ReadFile(filepath.Join(dir, "client-execute-request.json"))
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(raw, &execute); err != nil {
					t.Fatal(err)
				}
				if execute.Op != "cube.execute" {
					t.Fatal("unexpected client execution", execute)
				}
			}

			s.onEndgame(EndgameRequest{ID: p.ID, Request: execute})
			s.finishEndgame(false)
			if b.commits != 1 || !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatal("missing commit or published before durable result")
			}
			b.receipt = &b.plan.Receipt
			b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			s.finishEndgame(false)
			events := cubeTake(s, sink)
			var result domain.EndgameResponse
			for _, ev := range events {
				if m, ok := ev.(event.EndgameMessage); ok && m.Message.Kind == "cube.result" {
					result = m.Message
				}
			}
			if !result.OK {
				t.Fatalf("missing commit receipt: %+v", result)
			}
			exportWingSnapshotFixture(t, tc.name, "execute-response.json", result)
			for _, m := range reply.Message.Quote.Materials {
				if got := p.Player.Bag.CountOf(domain.ItemID(m.ItemID)); got != before.CountOf(domain.ItemID(m.ItemID))-m.Count {
					t.Fatal("wrong consumed amount", m, got)
				}
			}
			if tc.name == "rune_stack_count1" {
				remain := p.Player.Bag.At(120)
				if remain.UID != runeSource.UID || remain.Count != 2 || before.At(120).Count-remain.Count != runeCost {
					t.Fatalf("single selected UID did not consume authored cost %d: %+v", runeCost, remain)
				}
			}
			if tc.target != 0 && (p.Player.Bag.At(200).Item != tc.target || p.Player.Bag.At(200).UID != 42) {
				t.Fatal("wing identity not preserved")
			}
			if tc.output != 0 && p.Player.Bag.CountOf(tc.output) != before.CountOf(tc.output)+tc.outputCount {
				t.Fatal("wrong feather exchange output")
			}
			after := p.Player.Bag.Clone()
			s.onEndgame(EndgameRequest{ID: p.ID, Request: execute})
			s.finishEndgame(false)
			cubeTake(s, sink)
			if b.commits != 1 || !reflect.DeepEqual(after, p.Player.Bag) {
				t.Fatal("duplicate execute charged twice")
			}
			refreshed := wingSnapshotResponses(t, s, sink)
			exportWingSnapshotFixture(t, tc.name, "after-snapshot.json", refreshed)
			for _, snap := range refreshed {
				for _, m := range snap.Materials {
					if m.Count != p.Player.Bag.CountOf(domain.ItemID(m.ItemID)) {
						t.Fatal("post-commit snapshot stale", m)
					}
				}
			}
		})
	}
}
func TestWingMaterialSnapshotDoesNotWaiveInputValidation(t *testing.T) {
	for _, name := range []string{"absent", "locked", "duplicate", "stale"} {
		t.Run(name, func(t *testing.T) {
			s, sink, b := wingSnapshotScene(t)
			p := s.players[1]
			snap := wingSnapshotResponses(t, s, sink)
			req := domain.EndgameRequest{Op: "cube.quote", RequestID: "wing-reject", ExpectedRevision: snap[0].Revision, CubeInputs: []domain.CubeInput{{ItemID: 25027, Count: 5}}}
			switch name {
			case "absent":
				p.Player.Bag.Set(110, domain.Stack{})
			case "locked":
				st := p.Player.Bag.At(110)
				st.Locked = true
				p.Player.Bag.Set(110, st)
			case "duplicate":
				req.CubeInputs = append(req.CubeInputs, req.CubeInputs[0])
			case "stale":
				req.ExpectedRevision = strconv.FormatInt(s.endgamePlayer(p).state.Revision+1, 10)
			}
			before := p.Player.Bag.Clone()
			s.onEndgameCube(p, req)
			reply, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if !ok || reply.Message.OK || reply.Message.Quote != nil || b.commits != 0 || !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatalf("unsafe input accepted: %+v", reply.Message)
			}
		})
	}
}

func TestWingValueQuoteRejectsChangedSelectedDenomination(t *testing.T) {
	for _, locked := range []bool{false, true} {
		s, sink, b := wingSnapshotScene(t)
		p := s.players[1]
		snap := wingSnapshotResponses(t, s, sink)
		req := domain.EndgameRequest{Op: "cube.quote", RequestID: "mixed-quote", ExpectedRevision: snap[0].Revision, CubeInputs: []domain.CubeInput{{UID: "42", Count: 1}, {ItemID: 25027, Count: 10}, {ItemID: 25028, Count: 5}, {ItemID: 25029, Count: 1}}}
		s.onEndgameCube(p, req)
		reply, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if !ok || !reply.Message.OK || reply.Message.Quote == nil {
			t.Fatal("quote failed", reply.Message)
		}
		st := p.Player.Bag.At(110)
		if locked {
			st.Locked = true
		} else {
			st.Count--
		}
		p.Player.Bag.Set(110, st)
		before := p.Player.Bag.Clone()
		s.onEndgameCube(p, cubeExecute(*reply.Message.Quote))
		s.finishEndgame(false)
		result, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if !ok || result.Message.OK || result.Message.Error != "stale_revision" || b.commits != 0 || !reflect.DeepEqual(before, p.Player.Bag) {
			t.Fatal("stale mixed quote executed", result.Message)
		}
	}
}
