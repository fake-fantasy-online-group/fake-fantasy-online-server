package showcase

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type checkedTx struct {
	store.Store
	store.EndgameStore
	saves   int
	failAt  int
	collide bool
}

// Memory's legacy txView.LoadWarehouse takes its outer mutex again. Use its
// transaction-aware snapshot reader here; production uses pgTx.LoadWarehouse.
func (t *checkedTx) LoadWarehouse(ctx context.Context, id int64, r domain.WarehouseRule) (*domain.Warehouse, error) {
	s, e := t.EndgameStore.LoadEndgameSnapshot(ctx, domain.Snapshot{Char: &domain.Character{ID: id}})
	if e != nil {
		return nil, e
	}
	if s.Warehouse != nil {
		return s.Warehouse, nil
	}
	return domain.NewWarehouse(r.InitialPages, r.SlotsPerPage, r.MaxStack), nil
}
func (t *checkedTx) CheckShowcaseUIDsUnused(context.Context, []int64) error {
	if t.collide {
		return fmt.Errorf("synthetic UID collision")
	}
	return nil
}
func (t *checkedTx) SaveSnapshot(ctx context.Context, s domain.Snapshot) error {
	t.saves++
	if t.saves == t.failAt {
		return fmt.Errorf("injected save failure")
	}
	return t.Store.SaveSnapshot(ctx, s)
}
func testStore(t *testing.T) *store.Memory {
	t.Helper()
	ctx := context.Background()
	m := store.NewMemory()
	a, e := m.CreateAccount(ctx, "test001", "synthetic-not-a-secret")
	if e != nil {
		t.Fatal(e)
	}
	for id := int64(1); id <= 206; id++ {
		c := &domain.Character{Name: fmt.Sprintf("unused-%d", id), AccountID: a.ID}
		if id == 42 || id == 183 || id == 206 {
			c = fixtureCharacter(id, uint8(id%2))
			c.AccountID = a.ID
			c.BagSlots = 120
			c.FreePoints = 23
			c.Money.Gold = 77
			c.OwnedTitles = []string{"fixture-title"}
			c.Caiyu = 33
		}
		if e = m.CreateChar(ctx, c); e != nil {
			t.Fatal(e)
		}
		if id == 42 || id == 183 || id == 206 {
			bag := domain.NewBag(120)
			bag.Set(0, domain.Stack{Item: 999, Count: 3})
			if e = m.SaveSnapshot(ctx, domain.Snapshot{Char: c, Bag: bag, Worn: domain.NewEquipSet(), ChangeSet: domain.NewChangeSet()}); e != nil {
				t.Fatal(e)
			}
		}
	}
	return m
}
func snapshots(t *testing.T, m *store.Memory) []domain.Snapshot {
	t.Helper()
	out := []domain.Snapshot{}
	e := m.WithTx(context.Background(), func(tx store.Store) error {
		eg := tx.(store.EndgameStore)
		for _, id := range targetIDs {
			s, e := eg.LoadEndgameSnapshot(context.Background(), domain.Snapshot{Char: &domain.Character{ID: id}})
			if e != nil {
				return e
			}
			out = append(out, s)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func transact(t *testing.T, m *store.Memory, apply bool, fail int, collision bool) (Report, error, int) {
	t.Helper()
	var r Report
	var n int
	defs := fixtureDefs(t)
	e := m.WithTx(context.Background(), func(tx store.Store) error {
		w := &checkedTx{Store: tx, EndgameStore: tx.(store.EndgameStore), failAt: fail, collide: collision}
		var err error
		r, err = RunTransaction(context.Background(), w, defs, apply, domain.WarehouseRule{InitialPages: 1, MaxPages: 4, SlotsPerPage: 20, MaxStack: domain.MaxStack})
		n = w.saves
		return err
	})
	return r, e, n
}
func TestGrantDryRunNoWrites(t *testing.T) {
	m := testStore(t)
	before := snapshots(t, m)
	r, e, n := transact(t, m, false, 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if n != 0 || r.Status != "dry-run" || !reflect.DeepEqual(before, snapshots(t, m)) {
		t.Fatal("dry-run mutated state")
	}
}
func TestGrantAtomicIdempotentPreservesState(t *testing.T) {
	m := testStore(t)
	before := snapshots(t, m)
	r, e, n := transact(t, m, true, 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if n != 3 {
		t.Fatal(n)
	}
	after := snapshots(t, m)
	for i := range before {
		if e = verifyDelta(before[i], after[i], r.Characters[i].Granted); e != nil {
			t.Fatal(e)
		}
	}
	again, e, n := transact(t, m, true, 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if n != 0 || again.Status != "already-applied" || !reflect.DeepEqual(after, snapshots(t, m)) {
		t.Fatal("repeat duplicated or mutated grant")
	}
}
func TestGrantAllTargetsRollbackOnSecondSaveFailure(t *testing.T) {
	m := testStore(t)
	before := snapshots(t, m)
	_, e, n := transact(t, m, true, 2, false)
	if e == nil || n != 2 {
		t.Fatal("failure not exercised", e, n)
	}
	if !reflect.DeepEqual(before, snapshots(t, m)) {
		t.Fatal("partial grant persisted")
	}
	_, e, n = transact(t, m, true, 0, false)
	if e != nil || n != 3 {
		t.Fatal("failed grant incorrectly recorded receipt", e, n)
	}
}
func TestGrantGlobalUIDCollisionNoWrites(t *testing.T) {
	m := testStore(t)
	before := snapshots(t, m)
	_, e, n := transact(t, m, true, 0, true)
	if e == nil || n != 0 || !reflect.DeepEqual(before, snapshots(t, m)) {
		t.Fatal("collision did not fail closed")
	}
}
func TestGrantWrongOwnerNoWrites(t *testing.T) {
	m := testStore(t)
	s := snapshots(t, m)[2]
	s.Char.AccountID = 987
	if e := m.SaveSnapshot(context.Background(), s); e != nil {
		t.Fatal(e)
	}
	_, e, n := transact(t, m, true, 0, false)
	if e == nil || n != 0 {
		t.Fatal("wrong owner accepted")
	}
}
func TestGrantFullEquipmentTabNoWrites(t *testing.T) {
	m := testStore(t)
	s := snapshots(t, m)[2]
	for i := 0; i < s.Bag.Cap(); i++ {
		s.Bag.Set(i, domain.Stack{Item: 999, Count: 1})
	}
	if e := m.SaveSnapshot(context.Background(), s); e != nil {
		t.Fatal(e)
	}
	before := snapshots(t, m)
	_, e, n := transact(t, m, true, 0, false)
	if e == nil || n != 0 || !reflect.DeepEqual(before, snapshots(t, m)) {
		t.Fatal("full bag did not fail atomically")
	}
}

// A failure after all inventory saves must also roll back all three inventories
// and the earlier receipts, not merely the last character.
type receiptFailureTx struct {
	*checkedTx
	receiptSaves int
}

func (t *receiptFailureTx) SaveEndgameReceipt(ctx context.Context, r domain.EndgameReceipt) error {
	t.receiptSaves++
	if t.receiptSaves == 2 {
		return fmt.Errorf("injected receipt failure")
	}
	return t.EndgameStore.SaveEndgameReceipt(ctx, r)
}
func TestGrantLateReceiptFailureRollsBackEverything(t *testing.T) {
	m := testStore(t)
	before := snapshots(t, m)
	defs := fixtureDefs(t)
	saves := 0
	err := m.WithTx(context.Background(), func(tx store.Store) error {
		w := &receiptFailureTx{checkedTx: &checkedTx{Store: tx, EndgameStore: tx.(store.EndgameStore)}}
		_, e := RunTransaction(context.Background(), w, defs, true, domain.WarehouseRule{InitialPages: 1, MaxPages: 4, SlotsPerPage: 20, MaxStack: domain.MaxStack})
		saves = w.saves
		return e
	})
	if err == nil || saves != 3 || !reflect.DeepEqual(before, snapshots(t, m)) {
		t.Fatal("late receipt failure did not roll back all inventories", err, saves)
	}
	_, err, n := transact(t, m, true, 0, false)
	if err != nil || n != 3 {
		t.Fatal("rollback left receipts or items", err, n)
	}
}

func TestVerifyDeltaNormalizesOnlyCloneRepresentation(t *testing.T) {
	before := domain.Snapshot{Char: &domain.Character{Attrs: []int32{}, OwnedTitles: []string{}, SavedStatuses: []domain.SavedStatus{}}, Bag: domain.NewBag(10), Wardrobe: domain.NewWardrobe(10)}
	after := before.Clone()
	if err := verifyDelta(before, after, nil); err != nil {
		t.Fatal("empty slice clone representation rejected", err)
	}
	after.Char.FreePoints = 1
	if err := verifyDelta(before, after, nil); err == nil {
		t.Fatal("actual scalar change hidden")
	}
	after = before.Clone()
	after.Char.Attrs = []int32{3}
	if err := verifyDelta(before, after, nil); err == nil {
		t.Fatal("actual slice value change hidden")
	}
}

func TestGrantedStackProductionEquipmentKindAlias(t *testing.T) {
	expected := domain.Stack{Item: 2509, UID: 123, Count: 1, InstanceKind: domain.ItemInstanceEquipment}
	actual := expected
	actual.InstanceKind = domain.ItemInstanceNone
	if !sameGrantedStack(actual, expected) {
		t.Fatal("production legacy equipment kind rejected")
	}
	actual.UID++
	if sameGrantedStack(actual, expected) {
		t.Fatal("UID difference hidden")
	}
	actual = expected
	actual.InstanceKind = domain.ItemInstanceSocketCard
	if sameGrantedStack(actual, expected) {
		t.Fatal("wrong instance kind accepted")
	}
	actual = expected
	actual.Endgame.UniqueRollBP = 9999
	if sameGrantedStack(actual, expected) {
		t.Fatal("roll difference hidden")
	}
}
