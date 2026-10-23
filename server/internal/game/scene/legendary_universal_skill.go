package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"math/big"
)

func init() {
	for _, id := range []string{"TY-A04", "TY-A06", "TY-A08", "TY-A24", "TY-U03", "TY-U10", "TY-U13", "TY-U15"} {
		registerLegendarySynergyHandler(id, legendaryHandler{PrepareSkill: universalSynergyPrepare})
	}
	registerLegendarySynergyHandler("TY-A34", legendaryHandler{TargetDefinition: universalPrimaryModifier})
	for _, id := range []string{"TY-A18", "TY-A27", "TY-D16", "TY-U05"} {
		registerLegendarySynergyHandler(id, legendaryHandler{StatusApplied: universalNativeStatus})
	}
	for _, id := range []string{"TY-D05", "TY-D19"} {
		registerLegendarySynergyHandler(id, legendaryHandler{ModifyIncoming: universalSynergyIncoming})
	}
	registerLegendarySynergyHandler("TY-D11", legendaryHandler{AbsorbIncoming: universalSynergyIncoming})
	registerLegendarySynergyHandler("TY-U02", legendaryHandler{AfterHit: universalBasicMana, RootFinished: func(c *legendaryContext) { c.DeleteState(0, "basic-root") }})
	// Receiver-side native status modifiers are called at the common boundary.
	for _, id := range []string{"TY-D09", "TY-U08"} {
		registerLegendarySynergyHandler(id, legendaryHandler{StatusReceiving: universalNativeStatusReceiving})
	}
}
func universalArea(d *domain.SkillDef) bool {
	return d.Area.Valid() || d.Kind == domain.SkillArea || d.Radius > 0
}
func universalSingleDamage(d *domain.SkillDef) bool {
	return d.Kind == domain.SkillSingle && d.HasDamageEffect() && !universalArea(d) && !d.GroundEffect
}
func universalHealingSkill(d *domain.SkillDef) bool {
	for _, e := range d.DirectEffects() {
		if (e.Kind == domain.EffectHeal || e.Kind == domain.EffectRestoreHP) && e.Target != domain.EffectTargetCaster {
			return true
		}
	}
	return false
}
func universalShieldSkill(c *legendaryContext, d *domain.SkillDef) bool {
	for _, app := range d.Statuses {
		if app.ProgressionDamageShield > 0 {
			return true
		}
		if st, ok := c.Scene.resolveSkillStatusDef(*d, app, 0); ok && st.DamageShield > 0 {
			return true
		}
	}
	return false
}
func universalSynergyPrepare(c *legendaryContext, d *domain.SkillDef) {
	if !d.Kind.Active() {
		return
	}
	m := &d.LegendaryModifiers
	switch c.Power.ID {
	case "TY-A04":
		if d.Area.Valid() {
			switch d.Area.Shape {
			case domain.SkillAreaCircle:
				m.RadiusBP += universalTuning.AreaRadiusBP
			case domain.SkillAreaCone:
				m.AngleBP += universalTuning.ConeAngleBP
			}
		}
		if !d.Area.Valid() && d.Kind == domain.SkillArea && d.Radius > 0 {
			m.RadiusBP += universalTuning.AreaRadiusBP
		}
	case "TY-A06":
		if d.Area.Valid() && d.Area.Shape == domain.SkillAreaRectangle {
			m.WidthBP += universalTuning.RectangleWidthBP
		}
	case "TY-A08":
		if !d.LegendaryBasic && universalSingleDamage(d) {
			m.DamageBP += universalTuning.SingleTargetDamageBP
		}
	case "TY-A24":
		if !d.LegendaryBasic && d.HasDamageEffect() {
			m.ConservationDamagePerReductionBP += universalTuning.ConservationDamagePerReductionBP
		}
	case "TY-U03":
		if !d.LegendaryBasic {
			m.ManaReductionBP += universalTuning.ManaReductionBP
		}
	case "TY-U10":
		if universalHealingSkill(d) && d.Area.Valid() && (d.Area.Shape == domain.SkillAreaCircle || d.Area.Shape == domain.SkillAreaCone) {
			m.RadiusBP += universalTuning.HealingRadiusBP
		}
		if universalHealingSkill(d) && !d.Area.Valid() && d.Kind == domain.SkillArea && d.Radius > 0 {
			m.RadiusBP += universalTuning.HealingRadiusBP
		}
	case "TY-U13":
		if universalShieldSkill(c, d) {
			m.ManaReductionBP += universalTuning.ShieldManaReductionBP
		}
	case "TY-U15":
		m.CooldownReductionBP += universalTuning.CooldownReductionBP
	}
}
func (s *Scene) legendaryFinalizeSkill(d *domain.SkillDef) {
	if d == nil || d.LegendaryModifiers.Applied {
		return
	}
	m := &d.LegendaryModifiers
	m.Applied = true
	if !m.GeometryInherited && d.Area.Valid() {
		switch d.Area.Shape {
		case domain.SkillAreaCircle:
			d.Area.Radius = universalScaleBP(d.Area.Radius, m.RadiusBP)
			d.Radius = d.Area.Radius
		case domain.SkillAreaCone:
			d.Area.Radius = universalScaleBP(d.Area.Radius, m.RadiusBP)
			d.Area.Angle = min(int32(360), universalScaleBP(d.Area.Angle, m.AngleBP))
			d.Radius = d.Area.Radius
		case domain.SkillAreaRectangle:
			d.Area.Width = universalScaleBP(d.Area.Width, m.WidthBP)
			d.Area.Length = universalScaleBP(d.Area.Length, m.LengthBP)
		}
	}
	if !m.GeometryInherited && !d.Area.Valid() && (d.Kind == domain.SkillArea || universalHasEffect(d, domain.EffectDetectTrap)) && d.Radius > 0 {
		d.Radius = universalScaleBP(d.Radius, m.RadiusBP)
	}
	if !m.GeometryInherited && (m.RadiusBP != 0 || m.TrapRadiusBP != 0) {
		d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
		for i := range d.Effects {
			if d.Effects[i].Kind == domain.EffectPlaceTrap {
				d.Effects[i].TrapRadius = universalScaleBP(d.Effects[i].TrapRadius, m.RadiusBP+m.TrapRadiusBP)
			}
		}
	}
	if m.PrepareReductionBP > 0 {
		d.PrepareMS = universalScaleBP(d.PrepareMS, -min(int32(10000), m.PrepareReductionBP))
	}
	if m.HealingBP != 0 || m.RestoreMPBP != 0 || m.LifeStealBP != 0 {
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectHeal || e.Kind == domain.EffectRestoreHP {
				bp := m.HealingBP
				if e.Kind == domain.EffectRestoreHP && e.Target == domain.EffectTargetCaster {
					bp += m.LifeStealBP
				}
				universalScaleHealing(e, bp)
			}
			if e.Kind == domain.EffectRestoreMP {
				e.RestoreMPFlat = universalScaleBP(e.RestoreMPFlat, m.RestoreMPBP)
				e.SourceAttackPct = universalScaleBP(e.SourceAttackPct, m.RestoreMPBP)
			}
		})
	}
	reduction := min(max(int32(0), m.ManaReductionBP), universalTuning.ManaReductionCapBP)
	damage := domain.SaturateInt32(int64(m.DamageBP) + int64(min(int32(10000), reduction+max(int32(0), m.NativeManaReductionBP)))*int64(m.ConservationDamagePerReductionBP)/10000)
	legendaryApplyDamageBucket(d, damage)
	m.AppliedDamageBP = damage
	if d.MPCost > 0 && reduction > 0 {
		n, den := d.ManaCostFractionNumerator, d.ManaCostFractionDenominator
		if den <= 0 {
			n, den = int64(d.MPCost), 1
		}
		n, den = universalFraction(n, den, int64(10000-reduction), 10000)
		d.ManaCostFractionNumerator = n
		d.ManaCostFractionDenominator = den
		d.MPCost = domain.SaturateInt32((n + den - 1) / den)
		if n > 0 {
			d.MPCost = max(int32(1), d.MPCost)
		}
		d.RoundingPreviewReady = false
	}
	cdReduction := min(max(int32(0), m.CooldownReductionBP), int32(10000))
	if d.CooldownMS > 0 && cdReduction > 0 {
		n, den := d.CooldownFractionNumeratorMS, d.CooldownFractionDenominator
		if den <= 0 {
			n, den = int64(d.CooldownMS), 1
		}
		n, den = universalFraction(n, den, int64(10000-cdReduction), 10000)
		d.CooldownFractionNumeratorMS = n
		d.CooldownFractionDenominator = den
		d.CooldownMS = domain.SaturateInt32((n + den - 1) / den)
		d.RoundingPreviewReady = false
	}
}
func universalFraction(n, d, mn, md int64) (int64, int64) {
	r := new(big.Rat).SetFrac64(n, d)
	r.Mul(r, new(big.Rat).SetFrac64(mn, md))
	if r.Num().IsInt64() && r.Denom().IsInt64() {
		return r.Num().Int64(), r.Denom().Int64()
	}
	return int64(domain.SaturateInt32(n / d)), 1
}
func legendaryApplyDamageBucket(d *domain.SkillDef, bp int32) {
	if bp == 0 {
		return
	}
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		e := &d.Effects[i]
		if e.Kind == domain.EffectDamage {
			domain.ScaleSkillDamageFraction(e, 10000+int64(bp), 10000)
			e.DamageFlat = universalScaleBP(e.DamageFlat, bp)
		}
	}
}

// Target-specific bonuses join the same already-applied bucket, rather than
// multiplying all equipment damage a second time.
func legendaryAddTargetDamage(d *domain.SkillDef, bp int32) {
	if bp == 0 {
		return
	}
	old := d.LegendaryModifiers.AppliedDamageBP
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		e := &d.Effects[i]
		if e.Kind == domain.EffectDamage {
			domain.ScaleSkillDamageFraction(e, 10000+int64(old)+int64(bp), 10000+int64(old))
			e.DamageFlat = domain.SaturateInt32(int64(e.DamageFlat) * (10000 + int64(old) + int64(bp)) / (10000 + int64(old)))
		}
	}
	d.LegendaryModifiers.AppliedDamageBP = old + bp
}
func universalPrimaryModifier(c *legendaryContext, d *domain.SkillDef) {
	if c.Target != nil && c.Primary != 0 && c.Target.ID == c.Primary && d.HasDamageEffect() && universalArea(d) {
		legendaryAddTargetDamage(d, universalTuning.PrimaryAreaDamageBP)
	}
}

func universalScaleHealing(e *domain.SkillEffect, bp int32) {
	if bp == 0 {
		return
	}
	n, d := e.HealFlatNumerator, e.HealFlatDenominator
	if d <= 0 {
		n, d = int64(e.HealFlat), 1
	}
	e.HealFlatNumerator, e.HealFlatDenominator = universalFraction(n, d, int64(10000+bp), 10000)
	e.HealFlat = domain.SaturateInt32(e.HealFlatNumerator / e.HealFlatDenominator)
	n, d = e.HealPctNumerator, e.HealPctDenominator
	if d <= 0 {
		n, d = int64(e.HealPctOfMax), 1
	}
	e.HealPctNumerator, e.HealPctDenominator = universalFraction(n, d, int64(10000+bp), 10000)
	e.HealPctOfMax = domain.SaturateInt32(e.HealPctNumerator / e.HealPctDenominator)
	e.SourceAttackPct = universalScaleBP(e.SourceAttackPct, bp)
}
