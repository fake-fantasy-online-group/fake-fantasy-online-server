package scene

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type cubeTestBackend struct {
	plan    domain.EndgameCommitPlan
	result  chan domain.EndgameCommitResult
	receipt *domain.EndgameReceipt
	commits int
}

func (b *cubeTestBackend) Load(int64) <-chan domain.EndgameLoadResult {
	ch := make(chan domain.EndgameLoadResult, 1)
	ch <- domain.EndgameLoadResult{}
	return ch
}
func (b *cubeTestBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	ch := make(chan domain.EndgameReceiptLoadResult, 1)
	ch <- domain.EndgameReceiptLoadResult{Receipt: b.receipt}
	return ch
}
func (b *cubeTestBackend) LoadRun(string) <-chan domain.EndgameRunLoadResult {
	ch := make(chan domain.EndgameRunLoadResult, 1)
	ch <- domain.EndgameRunLoadResult{}
	return ch
}
func (b *cubeTestBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	b.plan = p
	b.commits++
	b.result = make(chan domain.EndgameCommitResult, 1)
	return b.result
}
func cubeSceneFixture(t *testing.T) (*Scene, *fakeSink, *cubeTestBackend) {
	t.Helper()
	s := newTestScene(t, nil)
	s.items = map[domain.ItemID]domain.ItemDef{}
	for _, d := range domain.CubeMaterialDefinitions() {
		s.items[d.ID] = d
	}
	for _, d := range domain.AstralRuneItems(7) {
		s.items[d.ID] = d
	}
	s.items[1001] = domain.ItemDef{ID: 1001, Name: "铁剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: 4, LevelReq: 1, Durable: 100}}
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	p.Player.Char.Money = moneyFromInventory(500000)
	p.Player.Bag.Set(100, domain.Stack{Item: domain.CubeStarMarrow, Count: 500, Bound: true})
	p.Player.Bag.Set(101, domain.Stack{Item: domain.CubePurifiedDust, Count: 500, Bound: true})
	p.Player.Bag.Set(200, domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Durability: 100, MaxDurability: 100})
	b := &cubeTestBackend{}
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	cubeTake(s, sink)
	return s, sink, b
}
func cubeTake(s *Scene, sink *fakeSink) []event.Event { s.flush(); return sink.take() }
func cubeQuote(t *testing.T, s *Scene, sink *fakeSink) domain.EndgameQuoteView {
	t.Helper()
	s.onEndgameCube(s.players[1], domain.EndgameRequest{Op: "cube.quote", RequestID: "preview-request", RecipeID: 1010, TargetUID: "42", ExpectedRevision: "0"})
	ev, ok := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if !ok || !ev.Message.OK || ev.Message.Quote == nil {
		t.Fatalf("missing quote %+v", ev.Message)
	}
	return *ev.Message.Quote
}
func cubeExecute(q domain.EndgameQuoteView) domain.EndgameRequest {
	return domain.EndgameRequest{Op: "cube.execute", RequestID: "cube-execute-0001", QuoteID: q.QuoteID, ExpectedRevision: q.Revision}
}
func TestEndgameCubeQuoteExpiresAndBindsExactInventory(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	now := time.Unix(100000, 0)
	s.now = func() time.Time { return now }
	before := s.players[1].Player.Bag.Clone()
	q := cubeQuote(t, s, sink)
	expires, err := time.Parse(time.RFC3339, q.ExpiresAt)
	if err != nil || expires.Sub(now) != 60*time.Second {
		t.Fatal("quote lifetime is not60seconds")
	}
	if !reflect.DeepEqual(before, s.players[1].Player.Bag) || b.commits != 0 {
		t.Fatal("preview consumed inventory")
	}
	st := s.players[1].Player.Bag.At(200)
	s.players[1].Player.Bag.Set(200, domain.Stack{})
	s.players[1].Player.Bag.Set(201, st)
	s.onEndgameCube(s.players[1], cubeExecute(q))
	ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "stale_revision" || b.commits != 0 {
		t.Fatalf("native move accepted %+v", ev.Message)
	}
	s.players[1].Player.Bag = before
	q = cubeQuote(t, s, sink)
	now = now.Add(60 * time.Second)
	s.onEndgameCube(s.players[1], cubeExecute(q))
	ev, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "quote_expired" {
		t.Fatalf("expired quote accepted %+v", ev.Message)
	}
}
func TestEndgameCubeCandidateCommitAndReceiptReplay(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	before := p.Player.Bag.Clone()
	money := p.Player.Char.Money
	q := cubeQuote(t, s, sink)
	req := cubeExecute(q)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || !p.Player.TradeBusy || !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money {
		t.Fatal("candidate exposed before durable commit")
	}
	if b.plan.Snapshot.Bag.At(200).Endgame.UniqueID != 1001 || b.plan.Snapshot.Bag.CountOf(domain.CubeStarMarrow) != 380 {
		t.Fatal("candidate missing output/consumption")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	if p.Player.TradeBusy || p.Player.Bag.At(200).Endgame.UniqueID != 1001 || s.endgamePlayer(p).cube != nil {
		t.Fatal("committed candidate unpublished")
	}
	after := p.Player.Bag.Clone()
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 1 || !reflect.DeepEqual(after, p.Player.Bag) {
		t.Fatal("receipt replay charged twice after quote was cleared")
	}
}
func TestEndgameCubeDefiniteFailureRollsBack(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	before := p.Player.Bag.Clone()
	money := p.Player.Char.Money
	q := cubeQuote(t, s, sink)
	s.onEndgameCube(p, cubeExecute(q))
	b.result <- domain.EndgameCommitResult{Err: errors.New("transaction rollback")}
	s.finishEndgame(false)
	if !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money || p.Player.TradeBusy || s.endgamePlayer(p).state.Revision != 0 {
		t.Fatal("failed commit consumed or locked inventory")
	}
}
func TestEndgameCubeRejectsTamperedAndSupersededQuotes(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	q := cubeQuote(t, s, sink)
	req := cubeExecute(q)
	req.RuneUID = "51"
	s.onEndgameCube(s.players[1], req)
	ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "invalid_request" || b.commits != 0 {
		t.Fatal("unquoted extra arguments accepted")
	}
	q = cubeQuote(t, s, sink)
	newer := cubeQuote(t, s, sink)
	if q.QuoteID == newer.QuoteID {
		t.Fatal("reused quote nonce")
	}
	s.onEndgameCube(s.players[1], cubeExecute(q))
	ev, _ = firstOf[event.EndgameMessage](cubeTake(s, sink))
	if ev.Message.Error != "quote_expired" || b.commits != 0 {
		t.Fatal("superseded quote executed")
	}
}

func TestEndgameRandomSocketRetryReceiptAndExpiry(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	now := time.Unix(100000, 0)
	s.now = func() time.Time { return now }
	s.rng = rand.New(rand.NewSource(456))
	expected := rand.New(rand.NewSource(456))
	want := expected.Intn(5) + 1
	requestQuote := func() domain.EndgameQuoteView {
		s.onEndgameCube(p, domain.EndgameRequest{Op: "cube.quote", RequestID: "random-preview", RecipeID: domain.CubeOpenSocket, TargetUID: "42", ExpectedRevision: "0"})
		ev, _ := firstOf[event.EndgameMessage](cubeTake(s, sink))
		if !ev.Message.OK || ev.Message.Quote == nil {
			t.Fatalf("no quote: %+v", ev.Message)
		}
		return *ev.Message.Quote
	}
	requestQuote()
	q := requestQuote()
	before := p.Player.Bag.Clone()
	money := p.Player.Char.Money
	req := cubeExecute(q)
	s.onEndgameCube(p, req)
	if got := int(b.plan.Snapshot.Bag.At(200).SocketCount); got != want {
		t.Fatalf("outcome=%d want%d", got, want)
	}
	candidate := s.endgamePlayer(p).cube.candidate
	b.result <- domain.EndgameCommitResult{Err: errors.New("transaction rollback")}
	s.finishEndgame(false)
	if !reflect.DeepEqual(before, p.Player.Bag) || p.Player.Char.Money != money {
		t.Fatal("failed transaction exposed result or costs")
	}
	events := cubeTake(s, sink)
	for _, ev := range events {
		if msg, ok := ev.(event.EndgameMessage); ok && msg.Message.Kind == "cube.result" {
			t.Fatal("failed transaction exposed random result")
		}
	}
	s.onEndgameCube(p, req)
	if s.endgamePlayer(p).cube.candidate != candidate || int(b.plan.Snapshot.Bag.At(200).SocketCount) != want {
		t.Fatal("retry rerolled candidate")
	}
	if s.rng.Intn(100000) != expected.Intn(100000) {
		t.Fatal("preview or retry consumed extra RNG")
	}
	b.receipt = &b.plan.Receipt
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	cubeTake(s, sink)
	after := p.Player.Bag.Clone()
	if int(after.At(200).SocketCount) != want || after.CountOf(domain.CubeStarMarrow) != 496 || p.Player.Char.Money != moneyFromInventory(495000) {
		t.Fatal("wrong socket result/cost")
	}
	var receipt domain.EndgameResponse
	if err := json.Unmarshal([]byte(b.plan.Receipt.Response), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Result["socketCount"] != strconv.Itoa(want) {
		t.Fatal("receipt missing committed socket outcome")
	}
	now = now.Add(time.Hour)
	s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
	s.finishEndgame(false)
	if b.commits != 2 || !reflect.DeepEqual(after, p.Player.Bag) {
		t.Fatal("expired-quote receipt replay charged or rerolled")
	}
}
