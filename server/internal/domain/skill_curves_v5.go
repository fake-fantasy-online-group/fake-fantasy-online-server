package domain

import (
	"fmt"
	"math"
)

// OriginalSkillRankCap is the authored legacy range. Those five rows are
// immutable gameplay definitions, not endpoints to spread across hard ranks.
const OriginalSkillRankCap int32 = 5

func HasV5SkillCurve(id SkillID, c *SkillProgressionCatalog) bool { _, ok := c.Identity(id); return ok }
func V5SkillCurveIDs(c *SkillProgressionCatalog) []SkillID        { return V5SkillIDs(c) }

// EvaluateV5Skill preserves exact supplied legacy rows. Ranks 2–4 require their
// own row: nonordinal status IDs and authored rounding cannot be reconstructed
// from endpoints. Above rank 5 only original numeric effects are extended; no
// damage, stat, shield or passive target is invented to force rank growth.
func EvaluateV5Skill(first, fifth SkillDef, rank int32, originalRows ...SkillDef) (SkillDef, error) {
	if !HasV5SkillCurve(first.ID, first.Progression) {
		return SkillDef{}, fmt.Errorf("skill %d is outside the class-battle catalogue", first.ID)
	}
	if rank < 1 {
		return SkillDef{}, fmt.Errorf("skill %d: rank must be positive", first.ID)
	}
	if first.ID != fifth.ID || first.Kind != fifth.Kind || first.Prof != fifth.Prof || first.HurtType != fifth.HurtType {
		return SkillDef{}, fmt.Errorf("skill %d: legacy anchor identity mismatch", first.ID)
	}
	if rank <= OriginalSkillRankCap {
		var original SkillDef
		switch rank {
		case 1:
			original = first
		case 5:
			original = fifth
		default:
			found := false
			for _, row := range originalRows {
				if row.ID == first.ID && row.Level == rank {
					original = row
					found = true
					break
				}
			}
			if !found {
				return SkillDef{}, fmt.Errorf("skill %d: original rank %d row missing", first.ID, rank)
			}
		}
		out := cloneOriginalSkill(original)
		out.Level = rank
		out.FormulaVersion = SkillFormulaVersion
		return out, nil
	}
	e := v5Evaluator{id: first.ID, rank: rank, catalog: first.Progression}
	out := e.continueOriginal(first, fifth)
	if e.err != nil {
		return SkillDef{}, e.err
	}
	out.Level = rank
	out.FormulaVersion = SkillFormulaVersion
	// This remains the historic prose. Scene presentation substitutes the actual
	// evaluated values; it must never label these fifth-rank numerals as current.
	out.Description = fifth.Description
	if len(out.Effects) > 0 {
		out.Effect = out.Effects[0]
	}
	return out, nil
}

func cloneOriginalSkill(d SkillDef) SkillDef {
	out := d
	out.Prerequisites = append([]SkillID(nil), d.Prerequisites...)
	out.SnapshotRawRanks = d.SnapshotRawRanks.Clone()
	out.SnapshotHardRanks = d.SnapshotHardRanks.Clone()
	cloneEffect := func(f SkillEffect) SkillEffect {
		f.RemoveStatusIDs = append([]StatusID(nil), f.RemoveStatusIDs...)
		return f
	}
	out.Effect = cloneEffect(d.Effect)
	if d.Effects != nil {
		out.Effects = make([]SkillEffect, len(d.Effects))
		for i, f := range d.Effects {
			out.Effects[i] = cloneEffect(f)
		}
	}
	out.Statuses = append([]StatusApplication(nil), d.Statuses...)
	for i := range out.Statuses {
		out.Statuses[i].ExtraAffixes = append([]Affix(nil), d.Statuses[i].ExtraAffixes...)
		out.Statuses[i].ProgressionAffixes = append([]Affix(nil), d.Statuses[i].ProgressionAffixes...)
	}
	out.PassiveModifiers = append([]PassiveModifier(nil), d.PassiveModifiers...)
	out.PassiveStats = append([]PassiveStatModifier(nil), d.PassiveStats...)
	out.PassiveResists = append([]PassiveStatusResist(nil), d.PassiveResists...)
	out.PassiveTriggers = append([]PassiveTriggerStatus(nil), d.PassiveTriggers...)
	out.PassiveStatusAffixes = append([]PassiveStatusAffix(nil), d.PassiveStatusAffixes...)
	out.PassiveTriggerEffects = append([]PassiveTriggerEffect(nil), d.PassiveTriggerEffects...)
	for i := range out.PassiveTriggerEffects {
		out.PassiveTriggerEffects[i].Effect = cloneEffect(out.PassiveTriggerEffects[i].Effect)
	}
	if d.PassiveCounter != nil {
		c := *d.PassiveCounter
		out.PassiveCounter = &c
	}
	return out
}

type v5Evaluator struct {
	catalog *SkillProgressionCatalog
	id      SkillID
	rank    int32
	err     error
}

func (e *v5Evaluator) fail(reason string) {
	if e.err == nil {
		e.err = fmt.Errorf("v5 curve %d rank %d: %s", e.id, e.rank, reason)
	}
}
func (e *v5Evaluator) checked(v int64, field string) int32 {
	if v < math.MinInt32 || v > math.MaxInt32 {
		e.fail(field + " is outside engine int32 representation")
		return 0
	}
	return int32(v)
}

// Follow the original four-rank slope without compressing it. Constant
// fields remain constant unless an explicit magnitude channel opts into growth.
func (e *v5Evaluator) fraction(a, b int32) (int64, int64) {
	if b == 0 {
		return 0, 1
	}
	n, d := int64(b)*4+(int64(b)-int64(a))*int64(e.rank-5), int64(4)
	if b > 0 && n < 0 || b < 0 && n > 0 {
		n = 0
	}
	e.checked(n/d, "original effect continuation")
	return n, d
}
func (e *v5Evaluator) magnitudeFraction(a, b int32) (int64, int64) {
	if a == b && b != 0 {
		n, d := int64(b)*(50+int64(e.rank-5)), int64(50)
		e.checked(n/d, "original magnitude continuation")
		return n, d
	}
	return e.fraction(a, b)
}
func (e *v5Evaluator) blend(a, b int32) int32 {
	n, d := e.fraction(a, b)
	return e.checked(n/d, "original effect continuation")
}
func (e *v5Evaluator) magnitude(a, b int32) int32 {
	n, d := e.magnitudeFraction(a, b)
	return e.checked(n/d, "original magnitude continuation")
}

// Linear original probability/reduction growth continues until its actual
// bound. There is no early diminishing-return compression after rank five.
func (e *v5Evaluator) bounded(a, b, lo, hi int32, field string) int32 {
	if a < lo || b < lo || a > hi || b > hi {
		e.fail(field + " anchor violates its effect bounds")
		return 0
	}
	v := int64(b) + (int64(b)-int64(a))*int64(e.rank-5)/4
	return int32(min(int64(hi), max(int64(lo), v)))
}
func (e *v5Evaluator) passive(a, b PassiveModifier) int32 {
	switch b.Kind {
	case PassiveCastTimeReductionPct, PassiveIncomingMagicHalfChance, PassivePreserveInvisibleChance,
		PassiveSkillDurabilityReductionPct, PassiveAttackDurabilityReductionPct, PassiveTrapHalfDamageChance,
		PassiveCastInterruptReductionPct:
		return e.bounded(a.Value, b.Value, 0, 100, "passive chance/reduction")
	case PassiveStealthDurationSec:
		return e.bounded(a.Value, b.Value, 0, max(int32(3600), b.Value), "stealth duration")
	case PassiveStealthMoveSpeedPct:
		return e.bounded(a.Value, b.Value, 0, max(int32(100), b.Value), "stealth movement")
	default:
		return e.blend(a.Value, b.Value)
	}
}

func (e *v5Evaluator) continueOriginal(a, b SkillDef) SkillDef {
	out := cloneOriginalSkill(b)
	out.Dist = b.Dist
	out.Radius = b.Radius
	// Keep the authored resource/cast slope. Only six explicitly listed skills
	// acquire efficiency, and only after their native mechanic reaches its cap.
	out.ManaCostReductionRanks = 0
	out.ManaCostReductionApplied = false
	out.ManaCostFractionNumerator, out.ManaCostFractionDenominator = 0, 0
	out.CooldownFractionNumeratorMS, out.CooldownFractionDenominator = 0, 0
	out.RoundingPreviewReady, out.RoundingPreviewNext = false, SkillRoundingState{}
	out.CooldownRoundingApplied = false
	out.PrepareMS = max(int32(0), e.blend(a.PrepareMS, b.PrepareMS))
	out.CooldownMS = max(int32(0), e.blend(a.CooldownMS, b.CooldownMS))
	out.MPCost = max(int32(0), e.blend(a.MPCost, b.MPCost))
	if a.Area.Shape != b.Area.Shape || a.Area.Center != b.Area.Center {
		e.fail("area identity mismatch")
	}
	out.Area.Radius = b.Area.Radius
	out.Area.Angle = b.Area.Angle
	out.Area.Length = b.Area.Length
	out.Area.Width = b.Area.Width
	af, bf := a.DirectEffects(), b.DirectEffects()
	if len(af) != len(bf) {
		e.fail("effect anchor count mismatch")
		return out
	}
	out.Effects = make([]SkillEffect, len(af))
	for i := range af {
		out.Effects[i] = e.effect(af[i], bf[i])
	}
	if len(a.Statuses) != len(b.Statuses) || len(a.PassiveModifiers) != len(b.PassiveModifiers) || len(a.PassiveStats) != len(b.PassiveStats) || len(a.PassiveResists) != len(b.PassiveResists) || len(a.PassiveTriggers) != len(b.PassiveTriggers) || len(a.PassiveStatusAffixes) != len(b.PassiveStatusAffixes) || len(a.PassiveTriggerEffects) != len(b.PassiveTriggerEffects) {
		e.fail("status/passive anchor count mismatch")
		return out
	}
	out.Statuses = make([]StatusApplication, len(a.Statuses))
	for i, x := range a.Statuses {
		y := b.Statuses[i]
		if x.ID != y.ID || x.TargetType != y.TargetType {
			e.fail("status identity mismatch")
		}
		z := y
		z.SourceSkill = e.id
		z.Level = y.Level
		z.ProgressionRanks = e.rank - 5
		z.ProgressionFirstLevel = x.Level
		z.ChanceBP = e.bounded(x.ChanceBP, y.ChanceBP, 0, 10000, "status chance")
		z.DurationSec = y.DurationSec
		z.ProgressionFirstDurationSec = x.DurationSec
		z.ProgressionBaseDurationSec = y.DurationSec
		z.StealthDamagePct = e.blend(x.StealthDamagePct, y.StealthDamagePct)
		z.PhysicalDamageTakenPct = e.blend(x.PhysicalDamageTakenPct, y.PhysicalDamageTakenPct)
		z.ExtraAffixes = e.affixes(x.ExtraAffixes, y.ExtraAffixes)
		z.ProgressionAffixes = e.affixes(x.ProgressionAffixes, y.ProgressionAffixes)
		out.Statuses[i] = z
	}
	out.PassiveModifiers = append([]PassiveModifier(nil), a.PassiveModifiers...)
	for i, x := range a.PassiveModifiers {
		y := b.PassiveModifiers[i]
		if x.Kind != y.Kind || x.TargetSkill != y.TargetSkill || x.EffectKind != y.EffectKind {
			e.fail("passive modifier identity mismatch")
		}
		out.PassiveModifiers[i].Value = e.passive(x, y)
	}
	out.PassiveStats = append([]PassiveStatModifier(nil), a.PassiveStats...)
	for i, x := range a.PassiveStats {
		y := b.PassiveStats[i]
		if x.Attr != y.Attr || x.Mode != y.Mode || x.RequiredEquipType != y.RequiredEquipType {
			e.fail("passive stat identity mismatch")
		}
		out.PassiveStats[i].Value = e.affixValue(Affix{Attr: x.Attr, Value: x.Value, Mode: x.Mode}, Affix{Attr: y.Attr, Value: y.Value, Mode: y.Mode})
	}
	out.PassiveResists = append([]PassiveStatusResist(nil), a.PassiveResists...)
	for i, x := range a.PassiveResists {
		y := b.PassiveResists[i]
		if x.Status != y.Status {
			e.fail("passive resistance identity mismatch")
		}
		out.PassiveResists[i].Pct = e.blend(x.Pct, y.Pct)
	}
	out.PassiveTriggers = append([]PassiveTriggerStatus(nil), a.PassiveTriggers...)
	for i, x := range a.PassiveTriggers {
		y := b.PassiveTriggers[i]
		if x.TargetSkill != y.TargetSkill || x.Status != y.Status {
			e.fail("passive trigger identity mismatch")
		}
		z := &out.PassiveTriggers[i]
		z.SourceSkill = e.id
		z.StatusLevel = y.StatusLevel
		z.ProgressionRanks = e.rank - 5
		z.ProgressionFirstLevel = x.StatusLevel
		z.ChanceBP = e.bounded(x.ChanceBP, y.ChanceBP, 0, 10000, "passive trigger chance")
		z.DurationSec = y.DurationSec
		z.ProgressionFirstDurationSec = x.DurationSec
		z.ProgressionBaseDurationSec = y.DurationSec
	}
	out.PassiveStatusAffixes = append([]PassiveStatusAffix(nil), a.PassiveStatusAffixes...)
	for i, x := range a.PassiveStatusAffixes {
		y := b.PassiveStatusAffixes[i]
		if x.TargetSkill != y.TargetSkill || x.Status != y.Status || x.Affix.Attr != y.Affix.Attr || x.Affix.Mode != y.Affix.Mode {
			e.fail("passive status affix identity mismatch")
		}
		out.PassiveStatusAffixes[i].Affix.Value = e.affixValue(x.Affix, y.Affix)
	}
	out.PassiveTriggerEffects = append([]PassiveTriggerEffect(nil), a.PassiveTriggerEffects...)
	for i, x := range a.PassiveTriggerEffects {
		y := b.PassiveTriggerEffects[i]
		if x.TargetSkill != y.TargetSkill {
			e.fail("passive trigger effect target mismatch")
		}
		out.PassiveTriggerEffects[i].Effect = e.effect(x.Effect, y.Effect)
	}
	if (a.PassiveCounter == nil) != (b.PassiveCounter == nil) {
		e.fail("counter anchor presence mismatch")
	} else if a.PassiveCounter != nil {
		x, y := a.PassiveCounter, b.PassiveCounter
		z := *x
		if x.RequiredEquipType != y.RequiredEquipType {
			e.fail("counter equipment condition mismatch")
		}
		z.ChanceBP = e.bounded(x.ChanceBP, y.ChanceBP, 0, 10000, "counter chance")
		z.DamagePct = e.blend(x.DamagePct, y.DamagePct)
		out.PassiveCounter = &z
	}
	e.continuePassiveOverflow(a, b, &out)
	e.applyGrowthPolicy(a, b, &out)
	return out
}

func (e *v5Evaluator) effect(a, b SkillEffect) SkillEffect {
	if a.Kind != b.Kind || a.Target != b.Target || a.RequiresPreviousLanded != b.RequiresPreviousLanded || a.GuaranteedHit != b.GuaranteedHit || a.ClearHarmful != b.ClearHarmful {
		e.fail("direct effect identity mismatch")
	}
	z := b
	z.DamagePctNumerator, z.DamagePctDenominator = e.magnitudeFraction(a.DamagePct, b.DamagePct)
	z.DamagePct = e.checked(z.DamagePctNumerator/z.DamagePctDenominator, "damage percentage")
	z.DamageFlat = e.magnitude(a.DamageFlat, b.DamageFlat)
	z.HealPctOfMax = e.bounded(a.HealPctOfMax, b.HealPctOfMax, 0, 100, "healing percentage")
	z.HealFlatNumerator, z.HealFlatDenominator = e.magnitudeFraction(a.HealFlat, b.HealFlat)
	z.HealFlat = e.checked(z.HealFlatNumerator/z.HealFlatDenominator, "healing")
	z.RestoreMPFlat = e.magnitude(a.RestoreMPFlat, b.RestoreMPFlat)
	z.SourceAttackPct = e.magnitude(a.SourceAttackPct, b.SourceAttackPct)
	z.MaxResourcePct = e.bounded(a.MaxResourcePct, b.MaxResourcePct, 0, 100, "maximum resource percentage")
	z.TrapRadius = b.TrapRadius
	z.RevealRadius = b.RevealRadius
	z.WeaponDurabilityCostPct = b.WeaponDurabilityCostPct
	z.DispelChanceBP = e.bounded(a.DispelChanceBP, b.DispelChanceBP, 0, 10000, "dispel chance")
	z.TauntPower = e.blend(a.TauntPower, b.TauntPower)
	z.ChanceBP = e.bounded(a.ChanceBP, b.ChanceBP, 0, 10000, "effect chance")
	if e.id == 10210 && b.Kind == EffectInstantDeath {
		z.ChanceBP = b.ChanceBP
		z.EffectHitBP = skillGrowthEffectHit(e.rank - 5)
	}
	z.LossRefundPct = e.bounded(a.LossRefundPct, b.LossRefundPct, 0, 100, "death loss refund")
	z.KnockbackDistance = b.KnockbackDistance
	z.RemoveStatusIDs = append([]StatusID(nil), a.RemoveStatusIDs...)
	if len(a.RemoveStatusIDs) != len(b.RemoveStatusIDs) {
		e.fail("dispel whitelist mismatch")
	} else {
		for i, id := range a.RemoveStatusIDs {
			if id != b.RemoveStatusIDs[i] {
				e.fail("dispel whitelist identity mismatch")
			}
		}
	}
	return z
}
func (e *v5Evaluator) affixes(a, b []Affix) []Affix {
	out := append([]Affix(nil), a...)
	if len(a) != len(b) {
		e.fail("status affix anchor count mismatch")
		return out
	}
	for i, x := range a {
		if x.Attr != b[i].Attr || x.Mode != b[i].Mode {
			e.fail("status affix identity mismatch")
		}
		out[i].Value = e.affixValue(x, b[i])
	}
	return out
}

func (e *v5Evaluator) affixValue(a, b Affix) int32 {
	// A penalty which reaches zero is complete, not a new positive buff.
	if b.Value == 0 {
		return 0
	}
	if b.Attr == AttrMoveSpeed || b.Attr == AttrAtkSpeed {
		return e.bounded(a.Value, b.Value, min(int32(-100), min(a.Value, b.Value)), max(int32(100), max(a.Value, b.Value)), "movement/attack speed")
	}
	if b.Mode == ModePercent && a.Value <= 0 && b.Value <= 0 {
		return -e.bounded(-a.Value, -b.Value, 0, 100, "percentage attribute reduction")
	}
	return e.blend(a.Value, b.Value)
}
