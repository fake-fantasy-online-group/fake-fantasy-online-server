package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestLifeFurnaceAlternativeMatchesDisplayedRouteAndStacksRune(t *testing.T) {
	fish := domain.ItemDef{ID: 4401, Name: "小鱼", Stackable: true, InventoryTabKnown: true, InventoryTab: 1}
	rare := fish
	rare.ID = 4410
	rare.Name = "珍鱼"
	rune := astralRuneItemsFixture(7)[0]
	rune.Stackable = true
	s := New(Config{ID: domain.SceneID{MapID: 7}, Alloc: domain.NewEntityAlloc(), SaveEvery: 10000, Items: map[domain.ItemID]domain.ItemDef{fish.ID: fish, rare.ID: rare, rune.ID: rune}})
	join(t, s, 1, 100, "合成者", 100, 100)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	a := domain.CraftRecipe{RowNo: 5200, Product: rune.ID, OutputQty: 1, SuccessRate: 100, Materials: []domain.CraftMaterial{{Item: fish.ID, Qty: 10}}}
	b := a
	b.RowNo++
	b.Materials = []domain.CraftMaterial{{Item: rare.ID, Qty: 3}}
	s.crafts = domain.CraftTable{ByType: map[uint8][]domain.CraftRecipe{0: {a, b}}, ByProduct: map[uint8]map[domain.ItemID][]domain.CraftRecipe{0: {rune.ID: {a, b}}}}
	if p.Player.Bag.Add(fish, 20) != 0 || p.Player.Bag.Add(rare, 3) != 0 {
		t.Fatal("fixture bag")
	}
	snap := s.craftRecipeSnapshot(p, 0)
	if len(snap.Recipes) != 1 || snap.Recipes[0].Materials[0].Item != fish.ID {
		t.Fatalf("duplicate/ambiguous recipes: %+v", snap)
	}
	for i := 0; i < 2; i++ {
		s.onCraftItem(CraftItem{ID: 1, MakeType: 0, Product: rune.ID})
	}
	if p.Player.Bag.CountOf(fish.ID) != 0 || p.Player.Bag.CountOf(rare.ID) != 3 || p.Player.Bag.CountOf(rune.ID) != 2 {
		t.Fatal("wrong route consumption")
	}
	var stack domain.Stack
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == rune.ID {
			stack = st
		}
	})
	if stack.Count != 2 || stack.UID <= 0 || !stack.Card.Initialized {
		t.Fatalf("rune instance lost: %+v", stack)
	}
	snap = s.craftRecipeSnapshot(p, 0)
	if len(snap.Recipes) != 1 || snap.Recipes[0].Materials[0].Item != rare.ID {
		t.Fatal("fallback route not shown")
	}
	s.onCraftItem(CraftItem{ID: 1, MakeType: 0, Product: rune.ID})
	if p.Player.Bag.CountOf(rune.ID) != 3 || p.Player.Bag.CountOf(rare.ID) != 0 {
		t.Fatal("fallback failed")
	}
	s.onCraftItem(CraftItem{ID: 1, MakeType: 0, Product: rune.ID})
	if p.Player.Bag.CountOf(rune.ID) != 3 {
		t.Fatal("replay without materials minted rune")
	}
}
