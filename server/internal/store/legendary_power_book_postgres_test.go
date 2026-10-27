package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// These tests deliberately require an explicit disposable database DSN. They
// reuse testStore's _test suffix guard and never fall back to a developer DB.
func powerBookPostgresFixture(t *testing.T) (*Postgres, domain.Snapshot) {
	t.Helper()
	if os.Getenv("fantasy_TEST_DSN") == "" {
		t.Skip("explicit isolated fantasy_TEST_DSN required")
	}
	p := testStore(t)
	ctx := context.Background()
	a, err := p.CreateAccount(ctx, "powerbook-pg", "fixture-not-a-credential")
	if err != nil {
		t.Fatal(err)
	}
	ch := &domain.Character{AccountID: a.ID, Name: "powerbook-pg", Race: domain.Warlock, Level: 100, BagSlots: 400}
	if err = p.CreateChar(ctx, ch); err != nil {
		t.Fatal(err)
	}
	s := domain.Snapshot{Char: ch, Bag: domain.NewBag(400), Worn: domain.NewEquipSet(), ChangeSet: domain.NewChangeSet(), Warehouse: domain.NewWarehouse(1, 100, 999), Wardrobe: domain.NewWardrobe(0)}
	st := legendaryStoreFixture()
	st.UID = domain.NewItemInstanceID()
	st.Legendary.PowerID = "TY-A01"
	st.Legendary.PowerRollBP = 8123
	s.Bag.Set(0, st)
	if err = p.SaveSnapshot(ctx, s); err != nil {
		t.Fatal(err)
	}
	return p, s
}
func powerBookPostgresPlan(t *testing.T, s domain.Snapshot) domain.EndgameCommitPlan {
	t.Helper()
	book := domain.LegendaryPowerBook{}
	if _, err := book.RecordSalvagedPower(s.Bag.At(0)); err != nil {
		t.Fatal(err)
	}
	payload, err := domain.WriteLegendaryPowerBook(`{"rift":{"tier":10}}`, book)
	if err != nil {
		t.Fatal(err)
	}
	after := s.Clone()
	after.Bag.Set(0, domain.Stack{})
	return domain.EndgameCommitPlan{Before: s, Snapshot: after, State: domain.EndgameState{Revision: 1, Payload: payload}, Receipt: domain.EndgameReceipt{CharID: s.Char.ID, RequestID: "powerbook-pg-salvage-0001", Operation: "cube.execute", Digest: strings.Repeat("a", 64), Revision: 1, Response: `{"kind":"cube.result","ok":true,"revision":"1"}`}}
}
func powerBookPostgresSnapshot(t *testing.T, p *Postgres, s domain.Snapshot) domain.Snapshot {
	t.Helper()
	var got domain.Snapshot
	err := p.WithTx(context.Background(), func(tx Store) error {
		var err error
		got, err = tx.(EndgameStore).LoadEndgameSnapshot(context.Background(), s)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Faults occur after actual SQL mutations inside PostgreSQL's transaction, not
// in an in-memory imitation of the persistence layer.
type powerBookPostgresFaultStore struct {
	*Postgres
	stage   string
	lostAck bool
	fired   atomic.Bool
}
type powerBookPostgresFaultTx struct {
	Store
	EndgameStore
	owner *powerBookPostgresFaultStore
}

func (f *powerBookPostgresFaultStore) WithTx(ctx context.Context, fn func(Store) error) error {
	err := f.Postgres.WithTx(ctx, func(tx Store) error {
		return fn(&powerBookPostgresFaultTx{Store: tx, EndgameStore: tx.(EndgameStore), owner: f})
	})
	if err == nil && f.lostAck && f.fired.CompareAndSwap(false, true) {
		return errors.New("injected lost PostgreSQL commit acknowledgement")
	}
	return err
}
func (t *powerBookPostgresFaultTx) fail(stage string) error {
	if t.owner.stage == stage && t.owner.fired.CompareAndSwap(false, true) {
		return errors.New("injected PostgreSQL failure after " + stage)
	}
	return nil
}
func (t *powerBookPostgresFaultTx) SaveSnapshot(ctx context.Context, s domain.Snapshot) error {
	if err := t.Store.SaveSnapshot(ctx, s); err != nil {
		return err
	}
	return t.fail("snapshot")
}
func (t *powerBookPostgresFaultTx) SaveEndgameState(ctx context.Context, id int64, v domain.EndgameState, n int64) error {
	if err := t.EndgameStore.SaveEndgameState(ctx, id, v, n); err != nil {
		return err
	}
	return t.fail("state")
}
func (t *powerBookPostgresFaultTx) SaveEndgameReceipt(ctx context.Context, v domain.EndgameReceipt) error {
	if err := t.EndgameStore.SaveEndgameReceipt(ctx, v); err != nil {
		return err
	}
	return t.fail("receipt")
}
func TestPowerBookPostgresAtomicRollback(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "receipt"} {
		t.Run(stage, func(t *testing.T) {
			p, s := powerBookPostgresFixture(t)
			plan := powerBookPostgresPlan(t, s)
			f := &powerBookPostgresFaultStore{Postgres: p, stage: stage}
			got := awaitEndgame(t, NewEndgameBackend(f, nil).Commit(plan))
			if got.Err == nil || !f.fired.Load() {
				t.Fatalf("SQL fault was not exercised: %+v", got)
			}
			restored := powerBookPostgresSnapshot(t, p, s)
			if restored.Bag.At(0) != s.Bag.At(0) {
				t.Fatal("failed transaction changed source item")
			}
			state, err := p.LoadEndgameState(context.Background(), s.Char.ID)
			if err != nil {
				t.Fatal(err)
			}
			book, err := domain.ReadLegendaryPowerBook(state.Payload)
			if err != nil || len(book) != 0 || state.Revision != 0 {
				t.Fatal("record survived rollback", state, book, err)
			}
			receipt, err := p.LoadEndgameReceipt(context.Background(), s.Char.ID, plan.Receipt.RequestID)
			if err != nil || receipt != nil {
				t.Fatal("receipt survived rollback", receipt, err)
			}
		})
	}
}
func TestPowerBookPostgresLostAckReplayRelog(t *testing.T) {
	p, s := powerBookPostgresFixture(t)
	plan := powerBookPostgresPlan(t, s)
	f := &powerBookPostgresFaultStore{Postgres: p, lostAck: true}
	b := NewEndgameBackend(f, nil)
	got := awaitEndgame(t, b.Commit(plan))
	if got.Err != nil || !got.Replayed || !f.fired.Load() {
		t.Fatalf("lost SQL ACK recovery failed: %+v", got)
	}
	for i := 0; i < 3; i++ {
		got = awaitEndgame(t, b.Commit(plan))
		if got.Err != nil || !got.Replayed {
			t.Fatalf("retry: %+v", got)
		}
	}
	// A fresh pool prevents an in-process cached object from masquerading as
	// durable item / collection / receipt persistence after a relog.
	reopened, err := Open(context.Background(), os.Getenv("fantasy_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := powerBookPostgresSnapshot(t, reopened, s)
	if !restored.Bag.At(0).Empty() {
		t.Fatal("source item returned after reconnect")
	}
	state, err := reopened.LoadEndgameState(context.Background(), s.Char.ID)
	if err != nil {
		t.Fatal(err)
	}
	book, err := domain.ReadLegendaryPowerBook(state.Payload)
	if err != nil || len(book) != 1 || book["TY-A01"] != 8123 || state.Revision != 1 {
		t.Fatal("rolled record/revision lost", book, state, err)
	}
	receipt, err := reopened.LoadEndgameReceipt(context.Background(), s.Char.ID, plan.Receipt.RequestID)
	if err != nil || receipt == nil || receipt.Revision != 1 {
		t.Fatal("receipt missing after reconnect", receipt, err)
	}
	a, err := reopened.CreateAccount(context.Background(), "powerbook-pg-other", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	other := &domain.Character{AccountID: a.ID, Name: "powerbook-pg-other", Level: 50, BagSlots: 400}
	if err = reopened.CreateChar(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	otherState, err := reopened.LoadEndgameState(context.Background(), other.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherBook, err := domain.ReadLegendaryPowerBook(otherState.Payload)
	if err != nil || len(otherBook) != 0 {
		t.Fatal("record leaked between characters", otherBook, err)
	}
}
func TestPowerBookPostgresConcurrentCAS(t *testing.T) {
	p, s := powerBookPostgresFixture(t)
	one := powerBookPostgresPlan(t, s)
	two := one
	two.Receipt.RequestID = "powerbook-pg-salvage-0002"
	b := NewEndgameBackend(p, nil)
	first, second := b.Commit(one), b.Commit(two)
	results := []domain.EndgameCommitResult{awaitEndgame(t, first), awaitEndgame(t, second)}
	successes, conflicts := 0, 0
	for _, r := range results {
		if r.Err == nil {
			successes++
		} else if errors.Is(r.Err, ErrEndgameRevision) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrency result: %+v", r)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent stale commits both applied", successes, conflicts)
	}
	state, err := p.LoadEndgameState(context.Background(), s.Char.ID)
	if err != nil || state.Revision != 1 {
		t.Fatal("concurrent revision", state, err)
	}
	receipts := 0
	for _, id := range []string{one.Receipt.RequestID, two.Receipt.RequestID} {
		r, e := p.LoadEndgameReceipt(context.Background(), s.Char.ID, id)
		if e != nil {
			t.Fatal(e)
		}
		if r != nil {
			receipts++
		}
	}
	if receipts != 1 {
		t.Fatal("concurrent retry wrote multiple receipts", receipts)
	}
	if !powerBookPostgresSnapshot(t, p, s).Bag.At(0).Empty() {
		t.Fatal("successful salvage not durable")
	}
}
