package domain

import "testing"

func TestSpiritQuantityBounds(t *testing.T) {
	for _, v := range []struct{ tier, lo, hi int32 }{{0, 0, 0}, {1, 1, 1}, {2, 1, 1}, {3, 2, 2}, {10, 5, 5}, {20, 9, 11}, {100, 45, 55}, {150, 68, 82}, {151, 0, 0}} {
		lo, hi := SpiritRiftQuantityBounds(v.tier)
		if lo != v.lo || hi != v.hi {
			t.Fatal(v, lo, hi)
		}
	}
	for _, s := range SpiritDefinitions() {
		d, ok := SpiritDefinitionForShop(SpiritShopID(s.Kind))
		if !ok || d.Kind != s.Kind {
			t.Fatal(s)
		}
	}
	if SpiritShopID(10) != 0 {
		t.Fatal("ring added")
	}
}

func TestSpiritRiftPoolFailsClosed(t *testing.T) {
	r := RiftRules{Version: 18}
	if err := r.validateSpiritRewards(nil); err != nil {
		t.Fatal("old run changed")
	}
	r.Version = 19
	items := map[ItemID]ItemDef{}
	for _, d := range SpiritDefinitions() {
		items[d.EssenceID] = ItemDef{ID: d.EssenceID, Stackable: true}
	}
	if err := r.validateSpiritRewards(items); err != nil {
		t.Fatal(err)
	}
	id := SpiritDefinitions()[0].EssenceID
	delete(items, id)
	if err := r.validateSpiritRewards(items); err == nil {
		t.Fatal("missing essence accepted")
	}
	items[id] = ItemDef{ID: id, Stackable: false}
	if err := r.validateSpiritRewards(items); err == nil {
		t.Fatal("nonstackable essence accepted")
	}
}
