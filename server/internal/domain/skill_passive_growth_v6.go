package domain

// overflowRanks begins only after the first rank at which the original linear
// probability/reduction reaches its mechanical cap. It does not change ranks 1–5.
func (e *v5Evaluator) overflowRanks(first, fifth, cap int32) int32 {
	if e.rank <= 5 {
		return 0
	}
	if fifth >= cap {
		return e.rank - 5
	}
	if fifth <= first {
		return 0
	}
	delta := int64(fifth - first)
	capRank := int64(5) + (int64(cap-fifth)*4+delta-1)/delta
	return e.checked(max(int64(0), int64(e.rank)-capRank), "passive overflow ranks")
}

// Legacy overflow carriers no longer award mana, damage mitigation, basic
// attack damage, or resist-triggered MP. Explicit policy helpers own the new
// attribute and original-target damage channels.
func (e *v5Evaluator) continuePassiveOverflow(a, b SkillDef, out *SkillDef) {
	for i := range out.PassiveModifiers {
		out.PassiveModifiers[i].ProgressionOverflow = 0
	}
	for i := range out.PassiveResists {
		out.PassiveResists[i].ProgressionRestoreMP = 0
	}
	for i := range out.PassiveTriggers {
		out.PassiveTriggers[i].ManaCostReductionRanks = 0
	}
	for i := range out.PassiveTriggerEffects {
		out.PassiveTriggerEffects[i].ManaCostReductionRanks = 0
	}
	for i := range out.PassiveStatusAffixes {
		out.PassiveStatusAffixes[i].ManaCostReductionRanks = 0
	}
	if b.PassiveCounter != nil && a.PassiveCounter.DamagePct == b.PassiveCounter.DamagePct {
		out.PassiveCounter.DamagePct = e.magnitude(a.PassiveCounter.DamagePct, b.PassiveCounter.DamagePct)
	}
}
