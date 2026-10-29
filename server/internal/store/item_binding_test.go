package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"testing"
)

type bindingRecordingTx struct {
	sortedBagRecordingTx
	roots map[int64][]any
}

func (q *bindingRecordingTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "INSERT INTO item_instances") {
		if q.roots == nil {
			q.roots = map[int64][]any{}
		}
		q.roots[args[0].(int64)] = args
	}
	return q.sortedBagRecordingTx.Exec(ctx, sql, args...)
}
func TestFixedBindingStorageCanonicalFactsAndCache(t *testing.T) {
	st := persistedRuneFixture()
	st.Count = 3
	st.Locked = true
	st.Card.Locked = true
	def := domain.ItemDef{ID: st.Item, FixedBinding: domain.ItemBindingFixedUnbound}
	rec := &bindingRecordingTx{}
	q := &endgameCatalogTx{Tx: rec, items: map[domain.ItemID]domain.ItemDef{st.Item: def}}
	bag := domain.NewBag(domain.DefaultBagSlots)
	bag.Set(110, st)
	if err := saveBag(context.Background(), q, 7, bag); err != nil {
		t.Fatal(err)
	}
	if rec.roots[st.UID][3] != false || rec.roots[st.UID][4] != true || rec.cards[st.UID][7] != false || rec.cards[st.UID][8] != true || rec.rows[0][8] != false || rec.rows[0][9] != true {
		t.Fatal("root/card/location policy diverged or locks changed")
	}
	if bag.At(110) != st {
		t.Fatal("save mutated caller snapshot")
	}
	got, err := hydrateStack(domain.Stack{Item: st.Item, UID: st.UID, Count: st.Count}, map[int64]domain.Stack{st.UID: st}, q)
	want := st
	want.Bound = false
	want.Card.Bound = false
	if err != nil || got != want {
		t.Fatalf("hydration lost identity/count/effects/lock: %v", err)
	}
	ordinary := domain.Stack{Item: 1900028201, Count: 42, Locked: true}
	q.items[ordinary.Item] = domain.ItemDef{ID: ordinary.Item, FixedBinding: domain.ItemBindingFixedBound}
	got, err = hydrateStack(ordinary, nil, q)
	ordinary.Bound = true
	if err != nil || got != ordinary {
		t.Fatal("UIDless cache not canonical", err)
	}
}
func TestFixedBindingMemoryAndNestedEquipmentPreservesState(t *testing.T) {
	st := persistedRuneFixture()
	st.Locked = true
	st.Card.Locked = true
	equip := domain.Stack{Item: 777, UID: 123, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Bound: true, Locked: true}
	equip.Sockets[0] = st.Item
	equip.SocketUIDs[0] = st.UID
	equip.SocketCards[0] = st.Card
	defs := map[domain.ItemID]domain.ItemDef{st.Item: {ID: st.Item, FixedBinding: domain.ItemBindingFixedUnbound}, 777: {ID: 777, Equip: &domain.EquipDef{}, FixedBinding: domain.ItemBindingFixedUnbound}}
	m := NewMemory()
	if err := SetItemDefinitions(m, defs); err != nil {
		t.Fatal(err)
	}
	// Map copy must isolate caller edits; unknown definitions retain their state.
	defs[st.Item] = domain.ItemDef{ID: st.Item, FixedBinding: domain.ItemBindingFixedBound}
	m.bags[7] = domain.NewBag(10)
	m.bags[7].Set(0, st)
	m.bags[7].Set(1, equip)
	got, err := m.LoadBag(context.Background(), 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := st
	want.Bound = false
	want.Card.Bound = false
	wantEquip := equip
	wantEquip.SocketCards[0].Bound = false
	if got.At(0) != want || got.At(1) != wantEquip || m.bags[7].At(0) != st {
		t.Fatal("memory load changed storage, parent equipment or nonbinding fields")
	}
	if again := normalizeBindingFromDefinitions(m.itemDefinitions, wantEquip); again != wantEquip {
		t.Fatal("normalization not idempotent")
	}
}
