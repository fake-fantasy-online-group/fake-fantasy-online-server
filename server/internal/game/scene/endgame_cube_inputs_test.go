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
	return domain.EndgameRequest{Op: "cube.quote", RequestID: "match-input-0001", ExpectedRevision: "0", NPCEntityID: 77, CubeInputs: []domain.CubeInput{{ItemID: domain.CubeStarMarrow, Count: 30}, {ItemID: domain.CubePurifiedDust, Count: 10}}}
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
	req.NPCEntityID = 77
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
		"no NPC":                 func(s *Scene) { delete(s.entities, 77) },
		"copied name":            func(s *Scene) { s.entities[77].NPC.Script = "N611FY" },
		"not database placement": func(s *Scene) { s.entities[77].NPC.ConfigID = 0 },
		"other dialogue":         func(s *Scene) { s.players[1].Player.QuestNPC = "巧匠左慈" },
		"out of range":           func(s *Scene) { s.players[1].Pos.X += 481 },
		"other map":              func(s *Scene) { s.players[1].Pos.MapID++ },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			s, sink, b := cubeSceneFixture(t)
			inputCubeNPC(s)
			alter(s)
			s.onEndgameCube(s.players[1], inputCubeQuoteRequest())
			ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if ev.Message.Error != "npc_required" || b.commits != 0 {
				t.Fatalf("unauthorized match accepted: %+v", ev.Message)
			}
		})
	}
	s, sink, b := cubeSceneFixture(t)
	inputCubeNPC(s)
	q := inputCubeQuote(t, s, sink)
	s.players[1].Pos.X += 481
	req := cubeExecute(q)
	req.NPCEntityID = 77
	s.onEndgameCube(s.players[1], req)
	ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "npc_required" || b.commits != 0 {
		t.Fatal("stale NPC quote executed")
	}
	s.players[1].Pos.X -= 481
	before := s.players[1].Player.Bag.Clone()
	r := inputCubeQuoteRequest()
	r.CubeInputs[0].Count++
	s.onEndgameCube(s.players[1], r)
	ev, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "no_match" || !reflect.DeepEqual(before, s.players[1].Player.Bag) || s.endgamePlayer(s.players[1]).cube != nil {
		t.Fatal("unmatched quantity consumed or preserved executable quote")
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
			req.NPCEntityID = 77
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
