package domain

// ApplyMonsterStatusStats preserves the existing interval-based attribute
// pipeline except for explicitly verified attack-frequency status penalties.
// Rebuilding from the template on each refresh prevents repeated inversion.
func ApplyMonsterStatusStats(baseline Stats, statuses *StatusSet) Stats {
	var affixes []Affix
	var frequencyLoss int64
	if statuses != nil {
		statuses.EachEffective(func(a Active) {
			for _, affix := range a.Def.Affixes {
				if a.Def.AttackSpeedIsFrequency && affix.Attr == AttrAtkSpeed &&
					affix.Mode == ModePercent && affix.Value > 0 {
					frequencyLoss += int64(affix.Value)
					continue
				}
				affixes = append(affixes, affix)
			}
		})
	}
	out := ApplyStatusStats(baseline, affixes)
	if frequencyLoss <= 0 {
		return out
	}
	// Same minimum 50% attack frequency as PlayerAttackSpeedPercent. Extended
	// ranks cannot divide by zero or turn a speed penalty into acceleration.
	speed := max(int64(50), int64(100)-frequencyLoss)
	ms := int64(out.AtkSpeedMS)
	if ms <= 0 {
		ms = 1500 // Same default as Stats.AttackIntervalTicks.
	}
	out.AtkSpeedMS = SaturateInt32((ms*100 + speed - 1) / speed)
	out.AtkSpeedPct = SaturateInt32(int64(out.AtkSpeedPct) - frequencyLoss)
	return out
}
