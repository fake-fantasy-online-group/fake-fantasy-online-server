package domain

// SpiritAppearanceDefinition preserves original ov_avatarcheck requirements,
// ov_avatararmdesc equipment identity, game_equip_extra bonuses, and
// ov_souleffect effect IDs. The 21 later cloth records without requirements or
// bonuses are intentionally excluded. BonusItemID is NOT a render model ID.
type SpiritAppearanceDefinition struct {
	ItemID, BonusItemID       ItemID
	Kind                      uint8
	Slot                      EquipSlot
	AttributeCount, Threshold int32
	Total                     bool
	EffectID                  int32
	Extra                     [2]Affix
}

var spiritAppearances = [...]SpiritAppearanceDefinition{
	{13698, 1077, 9, 4, 4, 800, true, 0, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{13699, 1078, 9, 4, 4, 900, true, 0, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{13700, 1079, 9, 4, 4, 1000, true, 0, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{13701, 1080, 9, 4, 4, 1000, true, 0, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{13703, 1081, 9, 4, 4, 270, false, 0, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{13704, 1082, 9, 4, 4, 270, false, 0, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{13705, 1083, 9, 4, 4, 285, false, 0, [2]Affix{{Attr: 34, Value: 2000, Mode: 0}, {Attr: 36, Value: 2000, Mode: 0}}},
	{13706, 1084, 9, 4, 4, 285, false, 0, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{13707, 1085, 9, 4, 4, 270, false, 0, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{13708, 1086, 9, 4, 4, 270, false, 0, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{14900, 24618, 5, 3, 4, 800, true, 9873, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{14901, 24619, 5, 3, 4, 900, true, 9874, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{14902, 24620, 5, 3, 4, 1000, true, 9875, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{14903, 24621, 5, 3, 4, 1000, true, 9876, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{14904, 24622, 5, 3, 4, 270, false, 9877, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{14905, 24623, 5, 3, 4, 270, false, 9878, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{14906, 24624, 5, 3, 4, 285, false, 9879, [2]Affix{{Attr: 34, Value: 2000, Mode: 0}, {Attr: 36, Value: 2000, Mode: 0}}},
	{14907, 24625, 5, 3, 4, 285, false, 9880, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{14908, 24626, 5, 3, 4, 270, false, 9881, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{14909, 24627, 5, 3, 4, 270, false, 9881, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{14914, 24630, 1, 9, 4, 800, true, 9505, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{14915, 24631, 1, 9, 4, 900, true, 9847, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{14916, 24632, 1, 9, 4, 1000, true, 9848, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{14917, 24633, 1, 9, 4, 270, false, 9507, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{14918, 24634, 1, 9, 4, 285, false, 9511, [2]Affix{{Attr: 34, Value: 1800, Mode: 0}, {Attr: 36, Value: 1800, Mode: 0}}},
	{14919, 24635, 1, 9, 4, 270, false, 9683, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{14920, 24636, 1, 9, 4, 1000, true, 9846, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{14921, 24637, 1, 9, 4, 270, false, 9506, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{14922, 24638, 1, 9, 4, 285, false, 9509, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{14923, 24639, 1, 9, 4, 270, false, 9683, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{25096, 29849, 7, 2, 4, 800, true, 8055, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{25097, 29850, 7, 2, 4, 900, true, 8056, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{25098, 29851, 7, 2, 4, 1000, true, 8057, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{25099, 29852, 7, 2, 4, 1000, true, 8058, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{25100, 29853, 7, 2, 4, 270, false, 8059, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{25101, 29854, 7, 2, 4, 270, false, 8060, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{25102, 29855, 7, 2, 4, 285, false, 8061, [2]Affix{{Attr: 34, Value: 2000, Mode: 0}, {Attr: 36, Value: 2000, Mode: 0}}},
	{25103, 29856, 7, 2, 4, 285, false, 8062, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{25104, 29857, 7, 2, 4, 270, false, 8063, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{25105, 29858, 7, 2, 4, 270, false, 8063, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{42002, 29835, 2, 6, 4, 800, true, 9700, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{42003, 29836, 2, 6, 4, 900, true, 9701, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{42004, 29837, 2, 6, 4, 1000, true, 9702, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{42005, 29838, 2, 6, 4, 270, false, 9704, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{42006, 29839, 2, 6, 4, 285, false, 9706, [2]Affix{{Attr: 34, Value: 1800, Mode: 0}, {Attr: 36, Value: 1800, Mode: 0}}},
	{42007, 29840, 2, 6, 4, 270, false, 9881, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{42008, 29841, 2, 6, 4, 1000, true, 9702, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{42009, 29842, 2, 6, 4, 270, false, 9705, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{42010, 29843, 2, 6, 4, 285, false, 9707, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{42011, 29844, 2, 6, 4, 270, false, 9881, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{42635, 47491, 6, 1, 4, 800, true, 8089, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{42636, 47492, 6, 1, 4, 900, true, 8090, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{42637, 47493, 6, 1, 4, 1000, true, 8091, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{42638, 47494, 6, 1, 4, 1000, true, 8091, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{42639, 47495, 6, 1, 4, 270, false, 8092, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{42640, 47496, 6, 1, 4, 270, false, 8093, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{42641, 47497, 6, 1, 4, 285, false, 8094, [2]Affix{{Attr: 34, Value: 2000, Mode: 0}, {Attr: 36, Value: 2000, Mode: 0}}},
	{42642, 47498, 6, 1, 4, 285, false, 8095, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{42643, 47499, 6, 1, 4, 270, false, 8096, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{42644, 47500, 6, 1, 4, 270, false, 8096, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{42646, 47502, 3, 10, 4, 800, true, 8089, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{42647, 47503, 3, 10, 4, 900, true, 8090, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{42648, 47504, 3, 10, 4, 1000, true, 8091, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{42649, 47505, 3, 10, 4, 1000, true, 8091, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{42650, 47506, 3, 10, 4, 270, false, 8092, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{42651, 47507, 3, 10, 4, 270, false, 8093, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{42652, 47508, 3, 10, 4, 285, false, 8094, [2]Affix{{Attr: 34, Value: 1800, Mode: 0}, {Attr: 36, Value: 1800, Mode: 0}}},
	{42653, 47509, 3, 10, 4, 285, false, 8095, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{42654, 47510, 3, 10, 4, 270, false, 8096, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{42655, 47511, 3, 10, 4, 270, false, 8096, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{42685, 47513, 2, 6, 4, 270, false, 8105, [2]Affix{{Attr: 34, Value: 4000, Mode: 0}, {Attr: 36, Value: 4000, Mode: 0}}},
	{42686, 47514, 2, 6, 4, 270, false, 8106, [2]Affix{{Attr: 15, Value: 400, Mode: 0}, {Attr: 47, Value: 380, Mode: 0}}},
	{42974, 29887, 4, 8, 4, 800, true, 8115, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{42975, 29888, 4, 8, 4, 900, true, 8115, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{42976, 29889, 4, 8, 4, 1000, true, 8116, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{42977, 29890, 4, 8, 4, 1000, true, 8116, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{42978, 29891, 4, 8, 4, 270, false, 8117, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{42979, 29892, 4, 8, 4, 270, false, 8117, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{42980, 29893, 4, 8, 4, 285, false, 8118, [2]Affix{{Attr: 34, Value: 2000, Mode: 0}, {Attr: 36, Value: 2000, Mode: 0}}},
	{42981, 29894, 4, 8, 4, 285, false, 8118, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{42982, 29895, 4, 8, 4, 270, false, 8122, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{42983, 29896, 4, 8, 4, 270, false, 8122, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
	{43184, 47641, 8, 11, 4, 800, true, 8134, [2]Affix{{Attr: 34, Value: 500, Mode: 0}, {Attr: 36, Value: 500, Mode: 0}}},
	{43185, 47642, 8, 11, 4, 900, true, 8135, [2]Affix{{Attr: 34, Value: 800, Mode: 0}, {Attr: 36, Value: 800, Mode: 0}}},
	{43186, 47643, 8, 11, 4, 1000, true, 8136, [2]Affix{{Attr: 34, Value: 1000, Mode: 0}, {Attr: 36, Value: 1000, Mode: 0}}},
	{43187, 47644, 8, 11, 4, 1000, true, 8136, [2]Affix{{Attr: 47, Value: 80, Mode: 0}, {Attr: 15, Value: 100, Mode: 0}}},
	{43188, 47645, 8, 11, 4, 270, false, 8137, [2]Affix{{Attr: 34, Value: 1200, Mode: 0}, {Attr: 36, Value: 1200, Mode: 0}}},
	{43189, 47646, 8, 11, 4, 270, false, 8137, [2]Affix{{Attr: 47, Value: 130, Mode: 0}, {Attr: 15, Value: 150, Mode: 0}}},
	{43190, 47647, 8, 11, 4, 285, false, 8138, [2]Affix{{Attr: 34, Value: 2000, Mode: 0}, {Attr: 36, Value: 2000, Mode: 0}}},
	{43191, 47648, 8, 11, 4, 285, false, 8138, [2]Affix{{Attr: 47, Value: 180, Mode: 0}, {Attr: 15, Value: 200, Mode: 0}}},
	{43192, 47649, 8, 11, 4, 270, false, 8139, [2]Affix{{Attr: 34, Value: 2500, Mode: 0}, {Attr: 36, Value: 2500, Mode: 0}}},
	{43193, 47650, 8, 11, 4, 270, false, 8139, [2]Affix{{Attr: 47, Value: 220, Mode: 0}, {Attr: 15, Value: 250, Mode: 0}}},
}

func SpiritAppearances() []SpiritAppearanceDefinition {
	return append([]SpiritAppearanceDefinition(nil), spiritAppearances[:]...)
}
func SpiritAppearanceForItem(id ItemID) (SpiritAppearanceDefinition, bool) {
	for _, d := range spiritAppearances {
		if d.ItemID == id {
			return d, true
		}
	}
	return SpiritAppearanceDefinition{}, false
}
func SpiritAppearanceIDs() []ItemID {
	out := make([]ItemID, 0, len(spiritAppearances))
	for _, d := range spiritAppearances {
		out = append(out, d.ItemID)
	}
	return out
}

// Meets checks only dynamic spirit properties, never the equipment's own stats.
// Thresholds are inclusive, matching the reference's >= wording.
func (d SpiritAppearanceDefinition) Meets(state SpiritState) bool {
	if state.Validate() != nil || state.Kind != d.Kind || int32(state.AttrCount) < d.AttributeCount {
		return false
	}
	values := state.Values()
	var total int32
	for i := 0; i < int(state.AttrCount); i++ {
		if !d.Total && values[i] < d.Threshold {
			return false
		}
		total += values[i]
	}
	return !d.Total || total >= d.Threshold
}
