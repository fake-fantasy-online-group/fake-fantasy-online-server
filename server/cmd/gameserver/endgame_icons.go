package main

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Unique rolls keep their original equipment ItemID. Publish those base sprites
// too: relying only on a client's installed item_icons table leaves renamed
// equipment vulnerable to stale/missing aliases (for example boots 22581 use
// sprite 30041, not sprite 22581). Explicit ov_item_icon_special mappings remain
// authoritative, including the unique-eligible extended equipment slots.
func registerEndgameEquipmentIcons(w *worldData) {
	mapped := make(map[domain.ItemID]bool, len(w.itemIcons))
	for _, pair := range w.itemIcons {
		mapped[pair.Item] = true
	}
	for _, baseID := range domain.UniqueAppearanceBaseIDs(w.endgameCatalog) { // stable order, includes legacy-owned bases
		def := w.items[baseID]
		if mapped[baseID] || def.Equip == nil || def.Icon <= 0 {
			continue // Do not guess a generic weapon sprite for unknown artwork.
		}
		w.itemIcons = append(w.itemIcons, domain.ItemIconMapping{Item: baseID, Icon: def.Icon})
	}
}
