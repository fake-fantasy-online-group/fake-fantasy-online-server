package main

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Portable cube tools were added after the fixed 32-material/rune/key catalog.
// Their IDs are configured in the database, as is their native artwork alias.
// Publish that same alias through 0x804d, not the new synthetic ItemID as a sprite.
func registerEndgameToolIcons(w *worldData) {
	mapped := make(map[domain.ItemID]bool, len(w.itemIcons))
	for _, pair := range w.itemIcons {
		mapped[pair.Item] = true
	}
	var ids []domain.ItemID
	for id, def := range w.items {
		if !mapped[id] && def.UseAction == "cube" && def.Icon > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		w.itemIcons = append(w.itemIcons, domain.ItemIconMapping{Item: id, Icon: w.items[id].Icon})
	}
}

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
