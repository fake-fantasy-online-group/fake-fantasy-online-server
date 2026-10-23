package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func legendaryStoreFixture() domain.Stack {
	st := domain.Stack{UID: 9007199254740993, Item: 4001, Count: 1, InstanceKind: domain.ItemInstanceEquipment}
	st.Legendary.PowerID = domain.LegendaryPowers()[0].ID
	for i, a := range []int32{domain.AttrSTR, domain.AttrINT, domain.AttrVIT, domain.AttrDEX, domain.AttrCrit, domain.AttrPhysRes} {
		st.Legendary.Affixes[i] = domain.InstanceAffix{CardID: int32(i + 1), Affix: domain.Affix{Attr: a, Value: 40}}
		st.Legendary.Bounds[i] = domain.LegendaryAffixBounds{Min: 30, Max: 60}
	}
	return st
}
func TestLegendaryMemoryPersistenceAndValidation(t *testing.T) {
	m, s := endgameFixture(t)
	st := legendaryStoreFixture()
	s.Bag.Set(0, st)
	if err := m.SaveSnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	loaded, err := m.LoadBag(context.Background(), s.Char.ID, 400)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.At(0) != st {
		t.Fatal("legendary facts lost on reload")
	}
	for _, mutate := range []func(*domain.Stack){func(s *domain.Stack) { s.RolledAffixCount = 1 }, func(s *domain.Stack) { s.SocketCount = 1 }, func(s *domain.Stack) { s.Legendary.Affixes[0].Affix.Value = 1000 }, func(s *domain.Stack) { s.Legendary.Affixes[1] = s.Legendary.Affixes[0] }, func(s *domain.Stack) { s.Legendary.PowerID = "missing" }} {
		bad := st
		mutate(&bad)
		if validateEndgameItem(bad, nil) == nil {
			t.Fatal("invalid legendary accepted")
		}
	}
}

func TestLegendaryPostgresRoundTrip(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()
	st := legendaryStoreFixture()
	st.UID = domain.NewItemInstanceID()
	if err := saveItemInstances(ctx, p.catalogQuery(), "legendary-test", st); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = p.pool.Exec(context.Background(), `DELETE FROM item_instances WHERE uid=$1`, st.UID) })
	loaded, err := loadItemInstances(ctx, p.catalogQuery(), []int64{st.UID})
	if err != nil {
		t.Fatal(err)
	}
	if loaded[st.UID] != st {
		t.Fatal("PostgreSQL overlay roundtrip changed instance")
	}
}
