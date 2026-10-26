package scene

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func inputCubeNPC(s *Scene) {
	p := s.players[1]
	p.Pos.MapID = s.id.MapID
	npc := &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "周天鼎", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 1, Script: "Nzhoutianding"}}
	s.entities[77] = npc
	p.Player.QuestNPC = npc.Name
}
func inputCubeQuoteRequest() domain.EndgameRequest {
	return domain.EndgameRequest{Op: "cube.quote", RequestID: "match-input-0001", ExpectedRevision: "0", CubeInputs: []domain.CubeInput{{ItemID: domain.CubeStarMarrow, Count: 30}, {ItemID: domain.CubePurifiedDust, Count: 10}}}
}
func inputCubeQuote(t *testing.T, s *Scene, sink *fakeSink) domain.EndgameQuoteView {
	t.Helper()
	s.onEndgameCube(s.players[1], inputCubeQuoteRequest())
	ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || !ev.Message.OK || ev.Message.Quote == nil {
		t.Fatalf("missing matched quote: %+v", ev.Message)
	}
	return *ev.Message.Quote
}
func TestEndgameCubeInputsRandomPoolAtomicRetryAndReconnectReplay(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	inputCubeNPC(s)
	p := s.players[1]
	before := p.Player.Bag.Clone()
	money := p.Player.Char.Money
	s.rng = rand.New(rand.NewSource(431))
	wantRng := rand.New(rand.NewSource(431))
	q := inputCubeQuote(t, s, sink)
	if len(s.endgamePlayer(p).cube.choices) != 28 || q.RecipeID != 0 || strings.Contains(q.Description, "角") || q.Description != "鼎中投料将随机炼成一枚星篆。" {
		t.Fatalf("preview leaked a selected recipe/result: %+v", q)
	}
	if !reflect.DeepEqual(before, p.Player.Bag) || b.commits != 0 {
		t.Fatal("quote consumed inputs")
	}
	chosen := s.endgamePlayer(p).cube.choices[wantRng.Intn(28)].RecipeID
	req := cubeExecute(q)
	req.NPCEntityID = 0
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money {
		t.Fatal("candidate visible before commit")
	}
	candidate := b.plan.Snapshot.Bag.Clone()
	result := s.endgamePlayer(p).cube.candidate
	if result == nil || result.Recipe.ID != chosen {
		t.Fatal("quote consumed RNG or selection not equal-weight DB pool")
	}
	s.onEndgameCube(p, req)
	if b.commits != 1 {
		t.Fatal("double click committed twice")
	}
	cubeTake(s, sink)
	b.result <- domain.EndgameCommitResult{Err: errors.New("rollback")}
	s.finishEndgame(false)
	cubeTake(s, sink)
	s.onEndgameCube(p, req)
	if b.commits != 2 || !reflect.DeepEqual(candidate, b.plan.Snapshot.Bag) {
		t.Fatal("retry rerolled output or UID")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	after := p.Player.Bag.Clone()
	if reflect.DeepEqual(before, after) {
		t.Fatal("committed inventory not published")
	}
	// Reconnection drops the ephemeral quote and interaction, but persistent receipt
	// replay must still acknowledge the exact original request without crafting twice.
	s.endgamePlayer(p).cube = nil
	p.Player.QuestNPC = ""
	delete(s.entities, 77)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 2 || !reflect.DeepEqual(after, p.Player.Bag) {
		t.Fatal("reconnect replay duplicated inputs/output")
	}
}
func TestEndgameCubeInputsWorkbenchBindingAndNoMatch(t *testing.T) {
	cases := map[string]func(*Scene){
		"no tool": func(s *Scene) { s.players[1].Player.Bag.Set(98, domain.Stack{}) },
		"unbound tool": func(s *Scene) {
			st := s.players[1].Player.Bag.At(98)
			st.Bound = false
			s.players[1].Player.Bag.Set(98, st)
		},
		"unfinished quest": func(s *Scene) { delete(s.players[1].Player.Char.Quests, portableCubeTestQuest) },
		"locked tool": func(s *Scene) {
			st := s.players[1].Player.Bag.At(98)
			st.Locked = true
			s.players[1].Player.Bag.Set(98, st)
		},
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			s, sink, b := cubeSceneFixture(t)
			inputCubeNPC(s)
			alter(s)
			s.onEndgameCube(s.players[1], inputCubeQuoteRequest())
			ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if ev.Message.Error != "tool_required" || b.commits != 0 {
				t.Fatalf("unauthorized match accepted: %+v", ev.Message)
			}
		})
	}
	s, sink, b := cubeSceneFixture(t)
	inputCubeNPC(s)
	q := inputCubeQuote(t, s, sink)
	tool := s.players[1].Player.Bag.At(98)
	s.players[1].Player.Bag.Set(98, domain.Stack{})
	req := cubeExecute(q)
	req.NPCEntityID = 0
	s.onEndgameCube(s.players[1], req)
	ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "tool_required" || b.commits != 0 {
		t.Fatal("quote executed after tool left the bag")
	}
	s.players[1].Player.Bag.Set(98, tool)
	// Quantities are hints: make the actual selected inventory insufficient.
	short := s.players[1].Player.Bag.At(100)
	short.Count = 29
	s.players[1].Player.Bag.Set(100, short)
	before := s.players[1].Player.Bag.Clone()
	r := inputCubeQuoteRequest()
	r.CubeInputs[0].Count++
	s.onEndgameCube(s.players[1], r)
	ev, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "insufficient_count" || !reflect.DeepEqual(before, s.players[1].Player.Bag) || s.endgamePlayer(s.players[1]).cube != nil {
		t.Fatal("insufficient actual inventory consumed or preserved executable quote")
	}
}

func TestEndgameCubeQuoteBindsImmutableRulesAcrossCachedRetry(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "unexecuted quote", true: "cached rolled candidate"}[cached], func(t *testing.T) {
			s, sink, b := cubeSceneFixture(t)
			inputCubeNPC(s)
			p := s.players[1]
			before := p.Player.Bag.Clone()
			q := inputCubeQuote(t, s, sink)
			req := cubeExecute(q)
			req.NPCEntityID = 0
			if cached {
				s.onEndgameCube(p, req)
				b.result <- domain.EndgameCommitResult{Err: errors.New("rollback")}
				s.finishEndgame(false)
				cubeTake(s, sink)
			}
			committed := b.commits
			defs := s.cubeRules.Recipes()
			for i := range defs {
				if defs[i].Operation == "rune.inscribe" {
					defs[i].Money++
				}
			}
			var err error
			s.cubeRules, err = domain.NewCubeRuleTable(defs, s.items)
			if err != nil {
				t.Fatal(err)
			}
			s.onEndgameCube(p, req)
			ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if ev.Message.Error != "quote_expired" || b.commits != committed || !reflect.DeepEqual(before, p.Player.Bag) || s.endgamePlayer(p).cube != nil {
				t.Fatalf("replaced rule snapshot executed: %+v", ev.Message)
			}
		})
	}
}

func TestEndgameCubeMismatchUsesExistingErrorEnvelopeWithoutConsumption(t *testing.T) {
	for _, tc := range []struct {
		name, code, message string
		inputs              []domain.CubeInput
	}{
		{"none", "no_match", "鼎中诸物，未合炼方。", []domain.CubeInput{{ItemID: 1999999999, Count: 1}}},
		{"partial", "partial_match", "部分材料相合，尚需调配。", []domain.CubeInput{{ItemID: domain.CubeStarMarrow, Count: 30}}},
		{"count", "insufficient_count", "材料虽合，数目尚缺。", []domain.CubeInput{{ItemID: domain.CubeStarMarrow, Count: 29}, {ItemID: domain.CubePurifiedDust, Count: 10}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, sink, b := cubeSceneFixture(t)
			inputCubeNPC(s)
			p := s.players[1]
			s.items[1999999999] = domain.ItemDef{ID: 1999999999, Name: "无关材料", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
			p.Player.Bag.Set(102, domain.Stack{Item: 1999999999, Count: 1})
			if tc.name == "count" {
				st := p.Player.Bag.At(100)
				st.Count = 29
				p.Player.Bag.Set(100, st)
			}
			before, money := p.Player.Bag.Clone(), p.Player.Char.Money
			req := inputCubeQuoteRequest()
			req.CubeInputs = tc.inputs
			s.onEndgameCube(p, req)
			ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if !ok || ev.Message.OK || ev.Message.Error != tc.code || ev.Message.Message != tc.message || ev.Message.Quote != nil {
				t.Fatalf("wrong envelope: %+v", ev.Message)
			}
			if b.commits != 0 || !reflect.DeepEqual(before, p.Player.Bag) || money != p.Player.Char.Money || s.endgamePlayer(p).cube != nil {
				t.Fatal("mismatch created a quote or consumed inputs")
			}
		})
	}
}

func TestEndgameCubeCountOneSelectsSufficientInventory(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	inputCubeNPC(s)
	p := s.players[1]
	before := p.Player.Bag.Clone()
	req := inputCubeQuoteRequest()
	for i := range req.CubeInputs {
		req.CubeInputs[i].Count = 1
	}
	s.onEndgameCube(p, req)
	ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || !ev.Message.OK || ev.Message.Quote == nil || !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatalf("count1 did not select actual inventory: %+v", ev.Message)
	}
	q := *ev.Message.Quote
	s.onEndgame(EndgameRequest{ID: p.ID, Request: cubeExecute(q)})
	s.finishEndgame(false)
	if b.commits != 1 || !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("quote/commit isolation")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	for _, m := range q.Materials {
		if m.Count <= 1 || p.Player.Bag.CountOf(domain.ItemID(m.ItemID)) != before.CountOf(domain.ItemID(m.ItemID))-m.Count {
			t.Fatalf("did not debit actual recipe cost: %+v", m)
		}
	}
	after := p.Player.Bag.Clone()
	s.onEndgame(EndgameRequest{ID: p.ID, Request: cubeExecute(q)})
	s.finishEndgame(false)
	cubeTake(s, sink)
	if b.commits != 1 || !reflect.DeepEqual(after, p.Player.Bag) {
		t.Fatal("count1 replay charged twice")
	}
}
