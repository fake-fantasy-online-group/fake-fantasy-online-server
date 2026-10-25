package main

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// All base items have already come through data.LoadItems. Register only their
// protocol artwork aliases and gameplay catalogs; never synthesize definitions.
func registerEndgameItems(w *worldData) error {
	if len(w.items) == 0 {
		return nil
	}
	if err := domain.ValidateUniqueCatalog(w.items,w.endgameCatalog); err != nil {
		return err
	}
	if err := domain.ValidateAstralCatalog(w.endgameCatalog); err != nil {
		return err
	}
	if err := domain.ValidateEndgameItemDefinitions(w.items); err != nil {
		return err
	}
	mapped := make(map[domain.ItemID]bool, len(w.itemIcons))
	for _, pair := range w.itemIcons {
		mapped[pair.Item] = true
	}
	for _, id := range domain.EndgameBaseItemIDs() {
		if !mapped[id] {
			w.itemIcons = append(w.itemIcons, domain.ItemIconMapping{Item: id, Icon: w.items[id].Icon})
		}
	}
	registerEndgameEquipmentIcons(w)
	registerEndgameToolIcons(w)
	w.itemIconsKnown = true
	return nil
}
