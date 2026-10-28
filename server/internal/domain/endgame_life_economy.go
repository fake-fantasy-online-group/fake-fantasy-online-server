package domain

// EquipmentSalvageQuality counts only innate quality, never paid sockets,
// refinement or washing (which would create a salvage amplification loop).
// 0=plain, 1=one/two lines, 2=three, 3=four/five, 4=darkgold.
func EquipmentSalvageQuality(st Stack, def ItemDef) int32 {
	if st.Legendary.PowerID != "" || st.Endgame.UniqueID != 0 {
		return 4
	}
	n := NaturalAttributeCount(def, st)
	switch {
	case n >= 4:
		return 3
	case n == 3:
		return 2
	case n > 0:
		return 1
	}
	return 0
}

// EquipmentSalvageOreCount is bounded independently of equipment level.
// Accessories yield base 4, armour 6, weapons/body 8. Quality multipliers are
// 1/2/3/4/6; gear level changes ore composition, not quantity.
func EquipmentSalvageOreCount(slot, quality int32) int32 {
	base := int32(4)
	switch EquipSlot(slot) {
	case SlotHead, SlotShield, SlotGlove, SlotShoe, SlotBelt:
		base = 6
	case SlotWeapon, SlotTwoHand, SlotBody:
		base = 8
	}
	return base * [5]int32{1, 2, 3, 4, 6}[max(0, min(4, quality))]
}

// RollEquipmentSalvageOres mirrors native mining's cumulative 2:1 weights.
// Every fifteen equipment levels unlock the next raw ore, capped at ten.
// RNG is injected; a nil RNG is a deterministic preview, never a real roll.
// Sorted aggregated outputs make bag capacity checks and receipts stable.
func RollEquipmentSalvageOres(level, slot, quality int32, intn func(int) int) []CubeMaterialDef {
	unlocked := max(1, min(10, 1+int(max(0, level-1))/15))
	counts := make([]int32, unlocked)
	for n := int32(0); n < EquipmentSalvageOreCount(slot, quality); n++ {
		pick := 0
		if intn != nil {
			pick = intn(unlocked + 1)
		}
		if pick < 0 || pick >= unlocked+1 {
			return nil
		}
		index := max(0, pick-1)
		counts[index]++
	}
	out := make([]CubeMaterialDef, 0, unlocked)
	for i, n := range counts {
		if n > 0 {
			out = append(out, CubeMaterialDef{ItemID: ItemID(4001 + 3*i), Count: n})
		}
	}
	return out
}
