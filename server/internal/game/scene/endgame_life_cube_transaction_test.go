package scene

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestLifeCubeTemperAtomicRetryAndReceiptReplay(t *testing.T) {
	s, sink, b := powerBookSceneFixture(t)
	p := s.players[1]
	st := p.Player.Bag.At(200)
	st.RefineLevel = 15
	p.Player.Bag.Set(200, st)
	s.items[4478] = domain.ItemDef{ID: 4478, Name: "免爆宝石", Stackable: true, InventoryTabKnown: true, InventoryTab: 1}
	p.Player.Bag.Set(110, domain.Stack{Item: 4478, Count: 20})
	var err error
	s.cubeRules, err = domain.NewCubeRuleTable([]domain.CubeRecipeDef{{ID: 13202, Name: "暗金淬炼", Description: "单词缀提高50%", Operation: "life.affix.temper", TargetRequired: true, Materials: []domain.CubeMaterialDef{{ItemID: 4478, Count: 10}}}}, s.items)
	if err != nil {
		t.Fatal(err)
	}
	before := p.Player.Bag.Clone()
	quoteReq := domain.EndgameRequest{Op: "cube.quote", RequestID: "life-temper-preview", ExpectedRevision: "0", CubeInputs: []domain.CubeInput{{UID: "42"}, {ItemID: 4478, Count: -1}}}
	s.onEndgameCube(p, quoteReq)
	ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || ev.Message.Quote == nil {
		t.Fatal("quote", ev)
	}
	req := cubeExecute(*ev.Message.Quote)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("not atomic", cubeTake(s, sink))
	}
	candidate := b.plan.Snapshot.Bag.Clone()
	if candidate.At(200).Legendary.TemperedSlot == 0 || candidate.At(110).Count != 10 {
		t.Fatal("missing result/cost")
	}
	s.onEndgameCube(p, req)
	if b.commits != 1 {
		t.Fatal("double click")
	}
	b.result <- domain.EndgameCommitResult{Err: errors.New("injected rollback")}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("failed storage mutated bag")
	}
	s.onEndgameCube(p, req)
	if b.commits != 2 || !reflect.DeepEqual(candidate, b.plan.Snapshot.Bag) {
		t.Fatal("retry rerolled")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	after := p.Player.Bag.Clone()
	s.endgamePlayer(p).cube = nil
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 2 || !reflect.DeepEqual(after, p.Player.Bag) {
		t.Fatal("replay spent twice")
	}
}
func TestLifeSalvagePowerBookUsesSameAtomicRecord(t *testing.T) {
	s, _, _ := powerBookSceneFixture(t)
	bag := s.players[1].Player.Bag
	p := domain.CubePlan{Recipe: domain.EndgameRecipeView{Operation: "life.salvage.unique"}, ConsumedUIDs: []int64{42}}
	payload, err := powerBookAfterCube("{}", bag, p)
	if err != nil {
		t.Fatal(err)
	}
	book, err := domain.ReadLegendaryPowerBook(payload)
	if err != nil || len(book) != 1 {
		t.Fatal("life salvage lost power book", err)
	}
	if _, err = powerBookAfterCube(`{"powerBook":null}`, bag, p); err == nil {
		t.Fatal("corrupt book bypassed")
	}
}
