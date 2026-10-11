package main

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Register new rules in the authoritative runtime table while keeping external
// artwork in the existing client resource installation. Missing client artwork
// uses bounded semantic fallbacks there; it must not prevent the server starting.
func registerEndgameItems(w *worldData) error {
	if len(w.items) == 0 {
		return nil
	}
	if err := domain.ValidateUniqueCatalog(w.items); err != nil {
		return err
	}
	if err := domain.ValidateAstralCatalog(); err != nil {
		return err
	}
	defs := domain.AstralRuneItems(0)
	defs = append(defs, domain.ItemDef{ID: domain.RiftKeyItemID, Name: "洞天符钥", Description: "试炼功成所得，一枚开启一次演劫洞天，可邀同队道友同行。", Weight: 0, Stackable: true, InventoryTab: 1, InventoryTabKnown: true})
	defs = append(defs, domain.CubeMaterialDefinitions()...)
	for _, d := range defs {
		if _, exists := w.items[d.ID]; exists {
			return fmt.Errorf("endgame item ID collision %d", d.ID)
		}
		d.Icon = endgameItemIcon(d.ID)
		w.items[d.ID] = d
		w.itemIcons = append(w.itemIcons, domain.ItemIconMapping{Item: d.ID, Icon: d.Icon})
	}
	registerEndgameEquipmentIcons(w)
	w.itemIconsKnown = true
	return nil
}
