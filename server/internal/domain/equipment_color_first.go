package domain

// ColorFirstSocketWeights gives the exact arithmetic distribution on 0..M.
// Sum=10*M*(M+1); last weight=M*(M+1), exactly 10%. M=0 is certain zero.
func ColorFirstSocketWeights(maximum int) []int64 {
	m := min(MaxEquipmentAttributes, max(0, maximum))
	if m == 0 {
		return []int64{1}
	}
	weights := make([]int64, m+1)
	for i := range weights {
		weights[i] = int64(m*(m+1) + 2*(9-m)*(m-i))
	}
	return weights
}

// RollColorFirstEquipment preserves the MF count roll, then determines its
// feasible color budget using only compatible affixes. Fixed native lines are
// never replaced: sampled N is the TOTAL budget, with native lines as a floor.
// Insufficient distinct options reduce the feasible budget
// BEFORE sockets are rolled, never after color has been determined. Sockets
// replace random lines, preserving the resulting color exactly.
// M is the feasible random budget. New color-first drops do not use legacy
// level-based socket caps: even low-level green equipment may roll four holes.
// Use only for NEW drops, never load, pickup, trade or persistence.
func (t EquipmentRollTable) RollColorFirstEquipment(def ItemDef, profile ColorProfile, magicFind int64, _ *LootRuleSet, countRNG, socketRNG, valueRNG func(int64) int64) (uint8, []InstanceAffix) {
	if !t.Enabled() || def.Equip == nil || countRNG == nil || socketRNG == nil || valueRNG == nil {
		return 0, nil
	}
	count := max(0, min(int(t.rollColorCountV5(profile, magicFind, countRNG)), MaxEquipmentAttributes)-len(def.Equip.Affixes))
	affixes := t.rollAffixOptions(def, uint8(count), valueRNG)
	maximum := len(affixes)
	slot := EquipSlot(def.Equip.Slot)
	if slot < SlotFace || slot > SlotShoe && slot != SlotTwoHand {
		maximum = 0
	}
	if maximum <= 0 {
		return 0, affixes
	}
	weights := ColorFirstSocketWeights(maximum)
	var total int64
	for _, w := range weights {
		total += w
	}
	roll := socketRNG(total)
	if roll < 0 || roll >= total {
		return 0, affixes
	}
	for holes, weight := range weights {
		if roll < weight {
			return uint8(holes), affixes[:len(affixes)-holes]
		}
		roll -= weight
	}
	return 0, affixes
}
