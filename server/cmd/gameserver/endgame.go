package main

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"sort"
)

// Register new rules in the authoritative runtime table while keeping external
// artwork in the existing client resource installation. No synthetic asset IDs.
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
	ids := make([]int, 0, len(w.items))
	for id := range w.items {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	genericIcon, runeIcon := int32(0), int32(0)
	for _, id := range ids {
		d := w.items[domain.ItemID(id)]
		if d.Icon <= 0 {
			continue
		}
		if genericIcon == 0 {
			genericIcon = d.Icon
		}
		if runeIcon == 0 && d.InstanceKind == domain.ItemInstanceSocketCard {
			runeIcon = d.Icon
		}
	}
	if genericIcon == 0 {
		return fmt.Errorf("endgame requires an existing item icon")
	}
	if runeIcon == 0 {
		runeIcon = genericIcon
	}
	defs := domain.AstralRuneItems(runeIcon)
	defs = append(defs, domain.ItemDef{ID: domain.RiftKeyItemID, Name: "洞天符钥", Description: "试炼功成所得，一枚开启一次演劫洞天，可邀同队道友同行。", Icon: genericIcon, Weight: 0, Stackable: true, InventoryTab: 1, InventoryTabKnown: true})
	defs = append(defs, domain.CubeMaterialDefinitions()...)
	for _, d := range defs {
		if _, exists := w.items[d.ID]; exists {
			return fmt.Errorf("endgame item ID collision %d", d.ID)
		}
		if d.Icon <= 0 {
			d.Icon = genericIcon
		}
		w.items[d.ID] = d
		w.itemIcons = append(w.itemIcons, domain.ItemIconMapping{Item: d.ID, Icon: d.Icon})
	}
	w.itemIconsKnown = true
	return nil
}
