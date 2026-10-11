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
	if len(w.items) != base+32 || len(w.itemIcons) != base+32 || !w.itemIconsKnown {
		t.Fatal("missing runtime definitions")
	}
	seen := map[domain.ItemID]bool{}
	for _, m := range w.itemIcons {
		d := w.items[m.Item]
		if seen[m.Item] || d.Icon != m.Icon || !d.InventoryTabKnown {
			t.Fatal("bad existing-art mapping", m, d)
		}
		if d.Equip == nil && m.Icon != endgameItemIcon(m.Item) {
			t.Fatal("bad added item mapping", m)
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

func TestEndgameIconCatalogDistinctExistingSprites(t *testing.T) {
	// The resource audit checks pixels separately. This guards registration coverage
	// and prevents a regression to the first arbitrary icon in the item table.
	seen := map[int32]domain.ItemID{}
	ids := []domain.ItemID{domain.RiftKeyItemID, domain.CubeStarMarrow, domain.CubePurifiedDust, domain.CubeImmortalEcho}
	for _, r := range domain.AstralRunes() {
		ids = append(ids, r.Item)
	}
	for _, id := range ids {
		icon := endgameItemIcon(id)
		if icon <= 0 {
			t.Fatalf("missing icon for %d", id)
		}
		if old, ok := seen[icon]; ok {
			t.Fatalf("shared icon %d for %d and %d", icon, old, id)
		}
		seen[icon] = id
	}
	if len(ids) != 32 || len(seen) != 32 {
		t.Fatal("incomplete icon catalog")
	}
	if endgameItemIcon(0) != 0 || endgameItemIcon(domain.FirstAstralRune-1) != 0 || endgameItemIcon(domain.FirstAstralRune+28) != 0 {
		t.Fatal("icon lookup leaked outside catalog")
	}
}

func TestRegisterEndgameIconsDoNotDependOnIncidentalItemOrder(t *testing.T) {
	w := endgameWorldFixture()
	w.items[1] = domain.ItemDef{ID: 1, Icon: 999999}
	if err := registerEndgameItems(&w); err != nil {
		t.Fatal(err)
	}
	if w.items[domain.FirstAstralRune].Icon != astralRuneIcons[0] {
		t.Fatal("used unrelated first icon")
	}
}
