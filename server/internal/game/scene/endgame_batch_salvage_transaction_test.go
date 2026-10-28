package scene

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func batchSalvageSceneFixture(t *testing.T) (*Scene, *fakeSink, *cubeTestBackend, domain.EndgameRequest) {
	t.Helper()
	s, sink, b := powerBookSceneFixture(t)
	p := s.players[1]
	configurePowerCodexFixture(s, p)
	delete(p.Player.Char.Quests, domain.PowerCodexQuestID)
	p.Player.Bag.Set(5, domain.Stack{})
	// Exercise the old client contract: no codex capability is needed to salvage.
	p.Player.EndgamePowerCodex = false
	s.endgamePlayer(p).state.Payload = `{"other":{"preserve":true}}`
	for i := 0; i < 10; i++ {
		id := domain.ItemID(4001 + 3*i)
		s.items[id] = domain.ItemDef{ID: id, Name: "原矿", Stackable: true, InventoryTabKnown: true, InventoryTab: 1}
	}
	var err error
	s.cubeRules, err = domain.NewCubeRuleTable([]domain.CubeRecipeDef{
		{ID: 13001, Name: "装备分解", Description: "分解原矿", Operation: "life.salvage.normal", TargetRequired: true},
		{ID: 13002, Name: "暗金分解", Description: "分解原矿", Operation: "life.salvage.unique", TargetRequired: true},
	}, s.items)
	if err != nil {
		t.Fatal(err)
	}
	original := p.Player.Bag.At(200)
	req := domain.EndgameRequest{Op: "cube.quote", RequestID: "batch-salvage-preview", ExpectedRevision: "0"}
	for i, roll := range []int32{8000, 9500, 8123, 0} {
		st := original
		st.UID = int64(42 + i)
		st.Legendary.PowerRollBP = roll
		if i == 2 {
			st.Legendary.PowerID = "TY-A02"
		}
		if i == 3 {
			st.Legendary = domain.LegendaryInstance{}
		}
		p.Player.Bag.Set(200+i, st)
		req.CubeInputs = append(req.CubeInputs, domain.CubeInput{UID: strconv.FormatInt(st.UID, 10), Count: int32(i - 1)})
	}
	return s, sink, b, req
}

func batchCodexNotices(events []event.Event) int {
	n := 0
	for _, ev := range events {
		if e, ok := ev.(event.NewbieTextNotice); ok && e.Text == powerCodexNotice {
			n++
		}
	}
	return n
}

func TestBatchSalvageAtomicBestRecordsQuestRetryReplay(t *testing.T) {
	s, sink, b, quoteReq := batchSalvageSceneFixture(t)
	p := s.players[1]
	before := p.Player.Bag.Clone()
	s.onEndgameCube(p, quoteReq)
	events := cubeTake(s, sink)
	ev, ok := firstOf[event.EndgameMessage](events)
	if !ok || ev.Message.Quote == nil {
		t.Fatalf("quote: %+v", ev.Message)
	}
	if batchCodexNotices(events) != 0 || p.Player.Char.Quests.Active(domain.PowerCodexQuestID) {
		t.Fatal("preview accepted quest")
	}
	req := cubeExecute(*ev.Message.Quote)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("missing one transaction", cubeTake(s, sink))
	}
	candidate := b.plan.Snapshot.Clone()
	payload := b.plan.State.Payload
	book, err := domain.ReadLegendaryPowerBook(payload)
	if err != nil || book["TY-A01"] != 9500 || book["TY-A02"] != 8123 || len(book) != 2 {
		t.Fatal("not all best records committed together", book, err)
	}
	for i := 200; i < 204; i++ {
		if !candidate.Bag.At(i).Empty() {
			t.Fatal("selected equipment survives", i)
		}
	}
	if !candidate.Char.Quests.Active(domain.PowerCodexQuestID) || p.Player.Char.Quests.Active(domain.PowerCodexQuestID) || !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("quest and bag are not atomic candidates")
	}
	s.onEndgameCube(p, req)
	if b.commits != 1 {
		t.Fatal("double click committed twice")
	}
	b.result <- domain.EndgameCommitResult{Err: errors.New("injected rollback")}
	s.finishEndgame(false)
	if batchCodexNotices(cubeTake(s, sink)) != 0 || !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Quests.Active(domain.PowerCodexQuestID) {
		t.Fatal("rollback leaked equipment/quest")
	}
	s.onEndgameCube(p, req)
	if b.commits != 2 || !reflect.DeepEqual(candidate.Bag, b.plan.Snapshot.Bag) || b.plan.State.Payload != payload {
		t.Fatal("retry rerolled or changed book")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if batchCodexNotices(cubeTake(s, sink)) != 1 || !p.Player.Char.Quests.Active(domain.PowerCodexQuestID) {
		t.Fatal("missing exactly one first-salvage quest notice")
	}
	after := p.Player.Bag.Clone()
	s.endgamePlayer(p).cube = nil
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 2 || !reflect.DeepEqual(after, p.Player.Bag) || batchCodexNotices(cubeTake(s, sink)) != 0 {
		t.Fatal("receipt replay spent again or repeated quest")
	}
}

func TestBatchSalvageAuthoritativeCASAndLostACK(t *testing.T) {
	for _, lostACK := range []bool{false, true} {
		t.Run(strconv.FormatBool(lostACK), func(t *testing.T) {
			s, sink, b, quoteReq := batchSalvageSceneFixture(t)
			p := s.players[1]
			before := s.snapshotOf(p).Clone()
			s.onEndgameCube(p, quoteReq)
			ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
			if ev.Message.Quote == nil {
				t.Fatalf("quote %+v", ev.Message)
			}
			req := cubeExecute(*ev.Message.Quote)
			s.onEndgameCube(p, req)
			if b.commits != 1 {
				t.Fatal("missing transaction")
			}
			result := domain.EndgameCommitResult{Err: domain.ErrEndgameRevision, RequiresReload: true, Authoritative: &before, State: domain.EndgameState{Revision: 2, Payload: `{"other":{"preserve":true}}`}}
			want := before.Bag
			if lostACK {
				authoritative := b.plan.Snapshot.Clone()
				result = domain.EndgameCommitResult{Replayed: true, RequiresReload: true, Authoritative: &authoritative, State: b.plan.State, Receipt: b.plan.Receipt}
				want = authoritative.Bag
			}
			b.result <- result
			s.finishEndgame(false)
			events := cubeTake(s, sink)
			if !reflect.DeepEqual(want, p.Player.Bag) || batchCodexNotices(events) != 0 {
				t.Fatal("recovery published candidate/duplicate notice")
			}
			if p.Player.Char.Quests.Active(domain.PowerCodexQuestID) != lostACK {
				t.Fatal("quest differs from authoritative snapshot")
			}
			if lostACK {
				ev, ok := firstOf[event.EndgameMessage](events)
				if !ok || !ev.Message.OK || ev.Message.RequestID != req.RequestID {
					t.Fatal("lost ACK did not replay receipt", ev)
				}
			}
		})
	}
}
