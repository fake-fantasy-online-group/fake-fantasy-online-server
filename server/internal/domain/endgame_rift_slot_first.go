package domain

import "sort"

// RollEquipmentBase consumes an already wearer/source/power-filtered pool.
// Version 15 gives each nonempty semantic equipment part equal probability,
// then each template within that part equal probability. One- and two-handed
// weapons share a part; shields remain separate. No missing part is fabricated.
// Historical snapshots keep their original ordering and single RNG draw.
func (r *RiftRules) RollEquipmentBase(pool []ItemID, items map[ItemID]ItemDef, intn func(int) int) (ItemID, bool) {
	if len(pool) == 0 {
		return 0, false
	}
	if r.Version < 15 {
		return pool[intn(len(pool))], true
	}
	groups := map[EquipSlot][]ItemID{}
	seen := map[ItemID]bool{}
	for _, id := range pool {
		def := items[id]
		if def.Equip == nil || seen[id] {
			continue
		}
		seen[id] = true
		slot := EquipSlot(def.Equip.Slot)
		if slot == SlotTwoHand {
			slot = SlotWeapon
		}
		if slot < SlotFace || slot > SlotBag {
			continue
		}
		groups[slot] = append(groups[slot], id)
	}
	parts := make([]EquipSlot, 0, len(groups))
	for slot, templates := range groups {
		parts = append(parts, slot)
		sort.Slice(templates, func(i, j int) bool { return templates[i] < templates[j] })
	}
	if len(parts) == 0 {
		return 0, false
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i] < parts[j] })
	templates := groups[parts[intn(len(parts))]]
	return templates[intn(len(templates))], true
}
