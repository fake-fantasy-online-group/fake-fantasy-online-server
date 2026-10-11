package store

import (
	"context"
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func endgameFixture(t *testing.T) (*Memory, domain.Snapshot) {
	t.Helper()
	m := NewMemory()
	ctx := context.Background()
	a, err := m.CreateAccount(ctx, "endgame", "hash")
	if err != nil {
		t.Fatal(err)
	}
	c := &domain.Character{Name: "test", AccountID: a.ID, Level: 50, BagSlots: 400, Caiyu: 500}
	if err = m.CreateChar(ctx, c); err != nil {
		t.Fatal(err)
	}
	s := domain.Snapshot{Char: c, Bag: domain.NewBag(400), Worn: domain.NewEquipSet(), ChangeSet: domain.NewChangeSet(), Warehouse: domain.NewWarehouse(1, 100, 999), Wardrobe: domain.NewWardrobe(0)}
	s.Bag.Set(0, domain.Stack{UID: 9007199254740993, Item: 4001, Count: 1, InstanceKind: domain.ItemInstanceEquipment})
	if err = m.SaveSnapshot(ctx, s); err != nil {
		t.Fatal(err)
	}
	return m, s
}
func endgamePlan(s domain.Snapshot) domain.EndgameCommitPlan {
	after := s.Clone()
	after.Char.Caiyu -= 10
	item := after.Bag.At(0)
	item.Endgame = domain.EndgameItemState{Revision: 1, UniqueID: 4001, UniqueRollBP: 10123}
	after.Bag.Set(0, item)
	return domain.EndgameCommitPlan{Before: s, Snapshot: after, State: domain.EndgameState{Revision: 1, Payload: `{"active":"run-1"}`}, Receipt: domain.EndgameReceipt{CharID: s.Char.ID, RequestID: "request-000000001", Operation: "cube.execute", Digest: strings.Repeat("a", 64), Revision: 1, Response: `{"ok":true}`}, Run: &domain.EndgameRunRecord{RunID: "run-1", Revision: 1, State: "active", Payload: `{"n":1}`}}
}
func awaitEndgame(t *testing.T, ch <-chan domain.EndgameCommitResult) domain.EndgameCommitResult {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("backend stayed pending")
		return domain.EndgameCommitResult{}
	}
}
func durableSnapshot(t *testing.T, m *Memory, s domain.Snapshot) domain.Snapshot {
	t.Helper()
	var out domain.Snapshot
	err := m.WithTx(context.Background(), func(tx Store) error {
		var err error
		out, err = tx.(EndgameStore).LoadEndgameSnapshot(context.Background(), s)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestEndgameCommitReplayAndCAS(t *testing.T) {
	m, s := endgameFixture(t)
	b := NewEndgameBackend(m, nil)
	p := endgamePlan(s)
	got := awaitEndgame(t, b.Commit(p))
	if got.Err != nil || got.Replayed || got.State.Revision != 1 {
		t.Fatalf("commit %+v", got)
	}
	if durableSnapshot(t, m, s).Char.Caiyu != 490 {
		t.Fatal("debit")
	}
	got = awaitEndgame(t, b.Commit(p))
	if got.Err != nil || !got.Replayed || !got.RequiresReload || got.Authoritative == nil {
		t.Fatalf("replay %+v", got)
	}
	if got.Authoritative.Char.Caiyu != 490 || got.Authoritative.Bag.At(0).Endgame.UniqueRollBP != 10123 {
		t.Fatal("replay aggregate")
	}
	got.Authoritative.Char.Caiyu = 1
	if durableSnapshot(t, m, s).Char.Caiyu != 490 {
		t.Fatal("aliased recovery")
	}
	other := p
	other.Receipt.RequestID = "request-000000002"
	got = awaitEndgame(t, b.Commit(other))
	if !errors.Is(got.Err, ErrEndgameRevision) || !got.RequiresReload {
		t.Fatalf("CAS %+v", got)
	}
	conflict := p
	conflict.Receipt.Digest = strings.Repeat("b", 64)
	got = awaitEndgame(t, b.Commit(conflict))
	if !errors.Is(got.Err, ErrEndgameReceiptConflict) {
		t.Fatal("digest reused")
	}
	receipt := <-b.LoadReceipt(s.Char.ID, p.Receipt.RequestID)
	if receipt.Err != nil || receipt.Receipt == nil || receipt.Receipt.Revision != 1 {
		t.Fatal("durable lookup")
	}
	if durableSnapshot(t, m, s).Char.Caiyu != 490 {
		t.Fatal("retry altered wallet")
	}
}

type endgameFaultStore struct {
	*Memory
	stage   string
	fired   atomic.Bool
	lostAck bool
}
type endgameFaultTx struct {
	Store
	EndgameStore
	owner *endgameFaultStore
}

func (f *endgameFaultStore) WithTx(ctx context.Context, fn func(Store) error) error {
	err := f.Memory.WithTx(ctx, func(tx Store) error { return fn(&endgameFaultTx{Store: tx, EndgameStore: tx.(EndgameStore), owner: f}) })
	if err == nil && f.lostAck && f.fired.CompareAndSwap(false, true) {
		return errors.New("lost COMMIT ack")
	}
	return err
}
func (t *endgameFaultTx) fail(stage string) error {
	if t.owner.stage == stage && t.owner.fired.CompareAndSwap(false, true) {
		return errors.New("injected " + stage)
	}
	return nil
}
func (t *endgameFaultTx) SaveSnapshot(ctx context.Context, s domain.Snapshot) error {
	if err := t.Store.SaveSnapshot(ctx, s); err != nil {
		return err
	}
	return t.fail("snapshot")
}
func (t *endgameFaultTx) SaveEndgameState(ctx context.Context, id int64, v domain.EndgameState, n int64) error {
	if err := t.EndgameStore.SaveEndgameState(ctx, id, v, n); err != nil {
		return err
	}
	return t.fail("state")
}
func (t *endgameFaultTx) SaveEndgameReceipt(ctx context.Context, v domain.EndgameReceipt) error {
	if err := t.EndgameStore.SaveEndgameReceipt(ctx, v); err != nil {
		return err
	}
	return t.fail("receipt")
}
func (t *endgameFaultTx) SaveEndgameRun(ctx context.Context, v domain.EndgameRunRecord, n int64) error {
	if err := t.EndgameStore.SaveEndgameRun(ctx, v, n); err != nil {
		return err
	}
	return t.fail("run")
}
func TestEndgameAtomicFaults(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "run", "receipt"} {
		t.Run(stage, func(t *testing.T) {
			m, s := endgameFixture(t)
			f := &endgameFaultStore{Memory: m, stage: stage}
			b := NewEndgameBackend(f, nil)
			p := endgamePlan(s)
			got := awaitEndgame(t, b.Commit(p))
			if got.Err == nil {
				t.Fatal("expected error")
			}
			after := durableSnapshot(t, m, s)
			if after.Char.Caiyu != 500 || after.Bag.At(0).Endgame.UniqueID != 0 {
				t.Fatal("partial wallet/item change")
			}
			state, _ := m.LoadEndgameState(context.Background(), s.Char.ID)
			receipt, _ := m.LoadEndgameReceipt(context.Background(), s.Char.ID, p.Receipt.RequestID)
			run, _ := m.LoadEndgameRun(context.Background(), "run-1")
			if state.Revision != 0 || receipt != nil || run != nil {
				t.Fatal("partial ledger/run")
			}
		})
	}
}
func TestEndgameLostCommitAckIsSuccess(t *testing.T) {
	m, s := endgameFixture(t)
	f := &endgameFaultStore{Memory: m, lostAck: true}
	got := awaitEndgame(t, NewEndgameBackend(f, nil).Commit(endgamePlan(s)))
	if got.Err != nil || !got.Replayed || got.Authoritative == nil || got.Authoritative.Char.Caiyu != 490 {
		t.Fatalf("ambiguous commit %+v", got)
	}
}
func TestEndgameRunCASAndLostAck(t *testing.T) {
	m, _ := endgameFixture(t)
	f := &endgameFaultStore{Memory: m, lostAck: true}
	b := NewEndgameBackend(f, nil).(domain.EndgameRunBackend)
	v := domain.EndgameRunRecord{RunID: "test-run", Revision: 1, State: "active", Payload: `{"uid":9007199254740993}`}
	r := <-b.SaveRun(v, 0)
	if r.Err != nil || r.Run == nil {
		t.Fatalf("lost ack %+v", r)
	}
	r = <-b.SaveRun(v, 0)
	if r.Err != nil {
		t.Fatalf("same replay %+v", r)
	}
	v.Payload = `{"uid":9007199254740992}`
	r = <-b.SaveRun(v, 0)
	if !errors.Is(r.Err, ErrEndgameRevision) {
		t.Fatal("rounded UID/conflict accepted")
	}
}
func TestMemoryTransactionPanicRollback(t *testing.T) {
	m, s := endgameFixture(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing panic")
			}
		}()
		_ = m.WithTx(context.Background(), func(tx Store) error {
			p := endgamePlan(s)
			if err := tx.SaveSnapshot(context.Background(), p.Snapshot); err != nil {
				t.Fatal(err)
			}
			_, _ = tx.CreateAccount(context.Background(), "rollback", "hash")
			panic("test")
		})
	}()
	if durableSnapshot(t, m, s).Char.Caiyu != 500 {
		t.Fatal("panic wallet persisted")
	}
	a, _ := m.AccountByName(context.Background(), "rollback")
	if a != nil {
		t.Fatal("panic account persisted")
	}
}
func TestEndgameInvalidOverlayDoesNotChangeWallet(t *testing.T) {
	m, s := endgameFixture(t)
	p := endgamePlan(s)
	st := p.Snapshot.Bag.At(0)
	st.Endgame.UniqueID = 4002
	p.Snapshot.Bag.Set(0, st)
	if err := m.SaveSnapshot(context.Background(), p.Snapshot); err == nil {
		t.Fatal("different base accepted")
	}
	if durableSnapshot(t, m, s).Char.Caiyu != 500 {
		t.Fatal("wallet changed before validation")
	}
}
func TestEndgameWriteBackOrderingAndClone(t *testing.T) {
	m, s := endgameFixture(t)
	wb := NewWriteBack(m, time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)
	b := NewEndgameBackend(m, wb)
	p := endgamePlan(s)
	ch := b.Commit(p)
	p.Snapshot.Char.Caiyu = 0
	p.Snapshot.Bag.Set(0, domain.Stack{})
	got := awaitEndgame(t, ch)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	after := durableSnapshot(t, m, s)
	if after.Char.Caiyu != 490 || after.Bag.At(0).Empty() {
		t.Fatal("candidate was not cloned synchronously")
	}
}
