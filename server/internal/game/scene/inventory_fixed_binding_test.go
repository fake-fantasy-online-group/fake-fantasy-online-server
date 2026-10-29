package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"os"
	"testing"
)

func fixedRuneDefsFixture(t *testing.T) map[domain.ItemID]domain.ItemDef {
	t.Helper()
	defs := map[domain.ItemID]domain.ItemDef{}
	raw, err := testfixture.ReadMigrationJSON("000000_init-127.sql", "$fixed_item_bindings$")
	if err != nil {
		t.Fatal(err)
	}
	var rules []struct {
		ID    domain.ItemID `json:"id"`
		Bound bool          `json:"bound"`
	}
	if err = json.Unmarshal(raw, &rules); err != nil || len(rules) != 66 {
		t.Fatal("audited binding rules", err)
	}
	policies := map[domain.ItemID]bool{}
	for _, r := range rules {
		policies[r.ID] = r.Bound
	}
	for _, d := range astralRuneItemsFixture(0) {
		if !d.CanTrade || !d.CanMail {
			t.Fatal("authored rune transfer policy changed")
		}
		bound, known := policies[d.ID]
		if !known || bound {
			t.Fatal("canonical rune rule missing or changed")
		}
		d.FixedBinding = domain.ItemBindingFixedUnbound
		defs[d.ID] = d
	}
	if len(defs) != 28 {
		t.Fatal("missing authored rune definitions")
	}
	return defs
}

func TestFixedBindingRealThirteenPairsBagAndWarehouse(t *testing.T) {
	raw, err := os.ReadFile("testdata/sort-fixed-binding-20261029.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Pairs []struct {
			Item domain.ItemID `json:"item"`
			A    int32         `json:"unbound_count"`
			B    int32         `json:"bound_count"`
		}
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Pairs) != 13 {
		t.Fatal("real pair fixture missing", err)
	}
	for _, warehouse := range []bool{false, true} {
		s := newTestScene(t, nil)
		s.items = fixedRuneDefsFixture(t)
		join(t, s, 1, 100, "fixed-binding", 0, 0)
		p := s.players[1]
		p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
		p.Player.Warehouse = domain.NewWarehouse(2, 100, domain.MaxStack)
		p.Player.WarehouseOpen = true
		before := map[domain.ItemID]domain.Stack{}
		for i, pair := range fixture.Pairs {
			a, b := domain.NewStack(s.items[pair.Item], pair.A), domain.NewStack(s.items[pair.Item], pair.B)
			b.Bound = true
			b.Card.Bound = true
			before[pair.Item] = a
			if warehouse {
				p.Player.Warehouse.Set(1, 2*i, a)
				p.Player.Warehouse.Set(1, 2*i+1, b)
			} else {
				p.Player.Bag.Set(100+2*i, a)
				p.Player.Bag.Set(101+2*i, b)
			}
		}
		if warehouse {
			s.exec(SortWarehouse{ID: 1, Tab: 1})
		} else {
			s.exec(SortBag{ID: 1, Tab: 1})
		}
		for i, pair := range fixture.Pairs {
			got := p.Player.Bag.At(100 + i)
			if warehouse {
				got = p.Player.Warehouse.At(1, i)
			}
			want := before[pair.Item]
			want.Count = pair.A + pair.B
			if got != want {
				t.Fatalf("warehouse=%t item=%d: canonical count/identity/effects differ", warehouse, pair.Item)
			}
		}
		for i := 13; i < 100; i++ {
			got := p.Player.Bag.At(100 + i)
			if warehouse {
				got = p.Player.Warehouse.At(1, i)
			}
			if !got.Empty() {
				t.Fatal("unreclaimed slot")
			}
		}
		if warehouse {
			s.exec(SortWarehouse{ID: 1, Tab: 1})
		} else {
			s.exec(SortBag{ID: 1, Tab: 1})
		}
		for i, pair := range fixture.Pairs {
			got := p.Player.Bag.At(100 + i)
			if warehouse {
				got = p.Player.Warehouse.At(1, i)
			}
			if got.Count != pair.A+pair.B || got.Bound || got.Card.Bound {
				t.Fatal("not idempotent")
			}
		}
	}
}

func TestFixedBindingSortPreservesEffectsLocksAndWarehouseLimit(t *testing.T) {
	s := newTestScene(t, nil)
	s.items = fixedRuneDefsFixture(t)
	d := s.items[domain.FirstAstralRune]
	a := domain.NewStack(d, 2)
	b := domain.NewStack(d, 2)
	b.Bound = true
	b.Card.Bound = true
	b.Card.Effects[0].Value++
	c := domain.NewStack(d, 3)
	c.Bound = true
	c.Card.Bound = true
	locked := domain.NewStack(d, 1)
	locked.Locked = true
	locked.Card.Locked = true
	got := s.mergeInventoryStacks([]domain.Stack{a, b, c, locked}, 3)
	if len(got) != 4 || got[0].Count != 3 || got[1].Card.Effects != b.Card.Effects || got[2].Count != 2 || got[2].UID != c.UID || !got[3].Locked || !got[3].Card.Locked {
		t.Fatal("different rolls/locks merged or configured limit ignored")
	}
	for _, st := range got {
		if st.Bound || st.Card.Bound {
			t.Fatal("old channel binding survived")
		}
	}
	// Splitting a legacy stack to a smaller configured warehouse limit must not duplicate UID.
	large := domain.NewStack(d, 8)
	got = s.mergeInventoryStacks([]domain.Stack{large}, 3)
	seen := map[int64]bool{}
	var total int32
	for _, st := range got {
		if st.Count > 3 || seen[st.UID] || st.Card != large.Card {
			t.Fatal("split lost identity/effects")
		}
		seen[st.UID] = true
		total += st.Count
	}
	if total != 8 {
		t.Fatal("split lost quantity")
	}
}

func TestFixedBindingRiftRewardsIgnoreChannelOverride(t *testing.T) {
	s, b, now := riftFixture(t)
	for id, def := range fixedRuneDefsFixture(t) {
		s.items[id] = def
	}
	activateTestRift(t, s, b, *now)
	d := &s.rift.data
	d.Run.Timely = true
	d.Run.Phase = domain.RiftCompleted
	for char, m := range d.Run.Members {
		m.Eligible = true
		d.Run.Members[char] = m
	}
	s.generateEndgameRiftGrants()
	found := 0
	for _, grant := range d.Grants {
		for _, st := range grant.Items {
			if st.Item < domain.FirstAstralRune || st.Item > domain.LastAstralRune {
				continue
			}
			found++
			if st.Bound || st.Card.Bound || !domain.IsAstralRuneStack(st) {
				t.Fatal("rift reward overrode canonical rune binding")
			}
		}
	}
	if found == 0 {
		t.Fatal("no actual rune reward exercised")
	}
	// Old frozen pending payloads are normalized before publication on the ground.
	legacy := domain.NewStack(s.items[domain.FirstAstralRune], 1)
	legacy.Bound = true
	legacy.Card.Bound = true
	s.spawnGeneratedLoot(s.items[legacy.Item], legacy, s.players[1].Pos, 1)
	for _, drop := range s.ground {
		if drop.Drop.Stack.Item == legacy.Item && (drop.Drop.Stack.Bound || drop.Drop.Stack.Card.Bound) {
			t.Fatal("legacy reward reintroduced channel binding")
		}
	}
}
