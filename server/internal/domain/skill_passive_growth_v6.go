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

func (e *v5Evaluator) continuePassiveOverflow(a, b SkillDef, out *SkillDef) {
	for i, m := range b.PassiveModifiers {
		cap := int32(100)
		switch m.Kind {
		case PassiveCastTimeReductionPct, PassiveCastInterruptReductionPct, PassivePreserveInvisibleChance,
			PassiveSkillDurabilityReductionPct, PassiveAttackDurabilityReductionPct,
			PassiveTrapHalfDamageChance, PassiveIncomingMagicHalfChance,
			PassiveStatusPhysicalDamageTakenPct:
		default:
			continue
		}
		out.PassiveModifiers[i].ProgressionOverflow = e.overflowRanks(a.PassiveModifiers[i].Value, m.Value, cap)
		if a.PassiveModifiers[i].Value == m.Value && m.Value > 0 {
			out.PassiveModifiers[i].ProgressionOverflow = e.rank - 5
		}
		// Never advertise impossible probabilities.
		out.PassiveModifiers[i].Value = min(out.PassiveModifiers[i].Value, cap)
	}
	for i, r := range b.PassiveResists {
		out.PassiveResists[i].ProgressionRestoreMP = e.overflowRanks(a.PassiveResists[i].Pct, r.Pct, 100)
	}
	for i, m := range b.PassiveTriggers {
		out.PassiveTriggers[i].ManaCostReductionRanks = e.overflowRanks(a.PassiveTriggers[i].ChanceBP, m.ChanceBP, 10000)
		if a.PassiveTriggers[i].ChanceBP == m.ChanceBP && m.ChanceBP > 0 {
			out.PassiveTriggers[i].ManaCostReductionRanks = e.rank - 5
		}
	}
	for i, m := range b.PassiveTriggerEffects {
		if m.Effect.Kind == EffectKnockback {
			out.PassiveTriggerEffects[i].ManaCostReductionRanks = e.overflowRanks(a.PassiveTriggerEffects[i].Effect.ChanceBP, m.Effect.ChanceBP, 10000)
		}
	}
	for i, m := range b.PassiveStatusAffixes {
		if m.Affix.Mode == ModePercent && m.Affix.Value < 0 {
			out.PassiveStatusAffixes[i].ManaCostReductionRanks = e.overflowRanks(-a.PassiveStatusAffixes[i].Affix.Value, -m.Affix.Value, 100)
		}
	}
	if b.PassiveCounter != nil && a.PassiveCounter.DamagePct == b.PassiveCounter.DamagePct {
		out.PassiveCounter.DamagePct = e.magnitude(a.PassiveCounter.DamagePct, b.PassiveCounter.DamagePct)
	}
}
