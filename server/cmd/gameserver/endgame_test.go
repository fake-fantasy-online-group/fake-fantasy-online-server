package main

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func endgameWorldFixture() worldData {
	w := worldData{items: make(data.ItemDefs)}
	for _, u := range domain.UniqueCatalog() {
		w.items[u.BaseID] = domain.ItemDef{ID: u.BaseID, Name: u.BaseName, Icon: 7, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: u.Slot, LevelReq: u.LevelRequirement, ResourceLevel: u.ResourceLevel, Need: domain.Requirement{Professions: u.Professions, BeginnerAllowed: u.BeginnerAllowed, Sex: u.Sex}}}
	}
	return w
}
func TestRegisterEndgameItemsCompleteNoAssetInvention(t *testing.T) {
	w := endgameWorldFixture()
	base := len(w.items)
	if err := registerEndgameItems(&w); err != nil {
		t.Fatal(err)
	}
	if len(w.items) != base+32 || len(w.itemIcons) != 32 || !w.itemIconsKnown {
		t.Fatal("missing runtime definitions")
	}
	seen := map[domain.ItemID]bool{}
	for _, m := range w.itemIcons {
		d := w.items[m.Item]
		if seen[m.Item] || m.Icon != 7 || d.Icon != 7 || !d.InventoryTabKnown {
			t.Fatal("bad existing-art mapping", m, d)
		}
		seen[m.Item] = true
	}
	if !w.items[domain.RiftKeyItemID].Stackable || w.items[domain.RiftKeyItemID].CanTrade {
		t.Fatal("key definition")
	}
	for _, r := range domain.AstralRunes() {
		if d := w.items[r.Item]; d.InstanceKind != domain.ItemInstanceSocketCard || !d.CardDefaults.Initialized {
			t.Fatal("rune lost real instance semantics")
		}
	}
}
func TestRegisterEndgameRejectsIDCollision(t *testing.T) {
	w := endgameWorldFixture()
	w.items[domain.RiftKeyItemID] = domain.ItemDef{ID: domain.RiftKeyItemID, Name: "existing"}
	if registerEndgameItems(&w) == nil {
		t.Fatal("overwrote existing ID")
	}
	if w.items[domain.RiftKeyItemID].Name != "existing" {
		t.Fatal("existing definition overwritten")
	}
}
