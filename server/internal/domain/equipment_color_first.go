package domain

// RollColorFirstEquipment preserves the MF count roll and every compatible
// random affix. Sampled N is the TOTAL budget, with fixed native lines as a
// floor. Natural drops never exchange affixes for sockets.
// The socket result/argument remain for caller compatibility; no socket RNG is
// consumed. Use only for NEW drops, never load, pickup, trade or persistence.
func (t EquipmentRollTable) RollColorFirstEquipment(def ItemDef, profile ColorProfile, magicFind int64, _ *LootRuleSet, countRNG, _ func(int64) int64, valueRNG func(int64) int64) (uint8, []InstanceAffix) {
	if !t.Enabled() || def.Equip == nil || countRNG == nil || valueRNG == nil {
		return 0, nil
	}
	count := max(0, min(int(t.rollColorCountV5(profile, magicFind, countRNG)), MaxEquipmentAttributes)-len(def.Equip.Affixes))
	return 0, t.rollAffixOptions(def, uint8(count), valueRNG)
}
