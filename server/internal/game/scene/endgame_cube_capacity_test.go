package scene

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Extend only two test-fixture variants to exercise a full-grid random pool.
// Shipped recipes and their material requirements remain unchanged.
func fullGridCubeRequest(t *testing.T, s *Scene) domain.EndgameRequest {
	t.Helper()
	req := inputCubeQuoteRequest()
	var recipes []domain.CubeRecipeDef
	for _, recipe := range s.cubeRules.Recipes() {
		if recipe.Operation == "rune.inscribe" {
			recipes = append(recipes, recipe)
			if len(recipes) == 2 {
				break
			}
		}
	}
	for i := len(req.CubeInputs); i < domain.CubeInputCapacity; i++ {
		id := domain.ItemID(1999990000 + i)
		s.items[id] = domain.ItemDef{ID: id, Name: "容量测试材料", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
		s.players[1].Player.Bag.Set(100+i, domain.Stack{Item: id, Count: 5})
		req.CubeInputs = append(req.CubeInputs, domain.CubeInput{ItemID: id, Count: 2})
		for j := range recipes {
			recipes[j].Materials = append(recipes[j].Materials, domain.CubeMaterialDef{ItemID: id, Count: 2})
		}
	}
	var err error
	s.cubeRules, err = domain.NewCubeRuleTable(recipes, s.items)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func quoteFullCubeGrid(t *testing.T, s *Scene, sink *fakeSink, req domain.EndgameRequest) domain.EndgameQuoteView {
	t.Helper()
	s.onEndgameCube(s.players[1], req)
	ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || !ev.Message.OK || ev.Message.Quote == nil || len(ev.Message.Quote.Materials) != domain.CubeInputCapacity {
		t.Fatalf("full grid quote rejected: %+v", ev.Message)
	}
	raw, err := json.Marshal(ev.Message)
	if err != nil || len(raw) > domain.EndgameMaxJSONBytes {
		t.Fatalf("full grid quote exceeds response limit: bytes=%d err=%v", len(raw), err)
	}
	return *ev.Message.Quote
}

func TestEndgameCubeFullGridAtomicRetryAndReceiptReplay(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	inputCubeNPC(s)
	selection := fullGridCubeRequest(t, s)
	p := s.players[1]
	before, money := p.Player.Bag.Clone(), p.Player.Char.Money
	q := quoteFullCubeGrid(t, s, sink, selection)
	if !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money || b.commits != 0 || len(s.endgamePlayer(p).cube.choices) != 2 {
		t.Fatal("full grid preview consumed inputs or lost random pool")
	}
	req := cubeExecute(q)
	req.NPCEntityID = 0
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money {
		t.Fatal("full grid candidate visible before commit")
	}
	candidate := b.plan.Snapshot.Bag.Clone()
	for _, input := range selection.CubeInputs {
		if candidate.UsableCountOf(input.ItemID) != before.UsableCountOf(input.ItemID)-input.Count {
			t.Fatalf("candidate failed to consume input %+v exactly once", input)
		}
	}
	s.onEndgameCube(p, req)
	if b.commits != 1 {
		t.Fatal("double click committed full grid twice")
	}
	cubeTake(s, sink)
	b.result <- domain.EndgameCommitResult{Err: errors.New("rollback")}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money {
		t.Fatal("rollback consumed full grid")
	}
	s.onEndgameCube(p, req)
	if b.commits != 2 || !reflect.DeepEqual(candidate, b.plan.Snapshot.Bag) {
		t.Fatal("full grid retry rerolled output or UID")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if !reflect.DeepEqual(candidate, p.Player.Bag) || s.endgamePlayer(p).state.Revision != 1 {
		t.Fatal("full grid durable commit did not publish candidate")
	}
	afterMoney := p.Player.Char.Money
	// As with a reconnect, the durable receipt must work without a live quote/NPC.
	s.endgamePlayer(p).cube = nil
	p.Player.QuestNPC = ""
	delete(s.entities, 77)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 2 || !reflect.DeepEqual(candidate, p.Player.Bag) || p.Player.Char.Money != afterMoney {
		t.Fatal("full grid receipt replay consumed inputs or charged twice")
	}
}

func TestEndgameCubeFullGridInvalidatesMovedOrChangedInputs(t *testing.T) {
	for _, change := range []string{"move", "decrease", "lock"} {
		t.Run(change, func(t *testing.T) {
			s, sink, b := cubeSceneFixture(t)
			inputCubeNPC(s)
			selection := fullGridCubeRequest(t, s)
			p := s.players[1]
			q := quoteFullCubeGrid(t, s, sink, selection)
			lastSlot := 100 + domain.CubeInputCapacity - 1
			st := p.Player.Bag.At(lastSlot)
			switch change {
			case "move":
				p.Player.Bag.Set(lastSlot, domain.Stack{})
				p.Player.Bag.Set(150, st)
			case "decrease":
				st.Count--
				p.Player.Bag.Set(lastSlot, st)
			case "lock":
				st.Locked = true
				p.Player.Bag.Set(lastSlot, st)
			}
			before, money := p.Player.Bag.Clone(), p.Player.Char.Money
			req := cubeExecute(q)
			req.NPCEntityID = 0
			s.onEndgameCube(p, req)
			ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if ev.Message.Error != "stale_revision" || b.commits != 0 || !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money {
				t.Fatalf("stale full grid committed: %+v", ev.Message)
			}
		})
	}
}
