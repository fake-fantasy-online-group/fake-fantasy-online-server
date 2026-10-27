package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func init() {
	for _, id := range []string{"TY-A02", "TY-A03", "TY-A07", "TY-A09", "TY-A19", "TY-A21", "TY-A40", "TY-U01", "TY-U04", "TY-U06", "TY-U07", "TY-U12", "TY-U14", "TY-U16", "TY-U17", "TY-U18", "TY-U19"} {
		registerLegendarySynergyHandler(id, legendaryHandler{PrepareSkill: universalExtendedPrepare})
	}
	registerLegendarySynergyHandler("TY-A05", legendaryHandler{TransformSkill: universalSingleCircle, SelectTargets: universalCircleTargets})
}
func universalSingleCircle(c *legendaryContext, d *domain.SkillDef) {
	if d.LegendaryBasic || !universalSingleDamage(d) || !d.TargetEnemy {
		return
	}
	d.LegendaryModifiers.UniversalSingleCircle = true
	d.Kind = domain.SkillArea
	d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: c.RollValue("benefit", 120)}
	d.Radius = c.RollValue("benefit", 120)
}
func universalCircleTargets(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if !c.Def.LegendaryModifiers.UniversalSingleCircle || c.Def.LegendaryBasic || c.Def.Area.Shape != domain.SkillAreaCircle || c.Def.Area.Center != domain.SkillAreaCenterTarget {
		return ids
	}
	out := make([]domain.EntityID, 0, len(ids))
	if c.Primary != 0 {
		for _, id := range ids {
			if id == c.Primary {
				out = append(out, id)
				break
			}
		}
	}
	for _, id := range ids {
		if id != c.Primary {
			out = append(out, id)
		}
	}
	return out
}
func universalHasEffect(d *domain.SkillDef, kinds ...domain.EffectKind) bool {
	for _, e := range d.DirectEffects() {
		for _, k := range kinds {
			if e.Kind == k {
				return true
			}
		}
	}
	return false
}
func universalNativeStatusKind(c *legendaryContext, d *domain.SkillDef, control bool) bool {
	for _, app := range d.Statuses {
		if st, ok := c.Scene.resolveSkillStatusDef(*d, app, 0); ok {
			if control && (st.Control != domain.ControlNone || st.BlockMove) {
				return true
			}
			if !control && (st.ID == domain.StatusPoison || st.ID == 1036 || st.PoisonOnHitChance > 0) {
				return true
			}
		}
	}
	return false
}
func universalExtendedPrepare(c *legendaryContext, d *domain.SkillDef) {
	m := &d.LegendaryModifiers
	switch c.Power.ID {
	case "TY-A02":
		if d.Area.Valid() && d.Area.Shape == domain.SkillAreaRectangle {
			m.LengthBP += c.RollValue("benefit", 6000)
		}
	case "TY-A03":
		if d.Area.Valid() && d.Area.Shape == domain.SkillAreaCone {
			m.RadiusBP += c.RollValue("benefit", 5000)
		}
	case "TY-A07":
		if d.ProjectileSpeedPXPerSec > 0 {
			d.ProjectileSpeedPXPerSec = universalScaleBP(d.ProjectileSpeedPXPerSec, c.RollValue("benefit", 10000))
			d.Dist = universalScaleBP(d.Dist, c.RollValue("benefit", 4000))
		}
	case "TY-A09":
		if d.LegendaryBasic {
			universalEditEffects(d, func(e *domain.SkillEffect) {
				if e.Kind == domain.EffectDamage {
					e.GuaranteedHit = true
				}
			})
		}
	case "TY-A19":
		m.ManaDrainBP += c.RollValue("benefit", 8000)
	case "TY-A21":
		m.TrapRadiusBP += c.RollValue("benefit", 5000)
	case "TY-A40":
		if !d.LegendaryBasic && d.HasDamageEffect() && universalHasNativeShield(c) {
			m.DamageBP += c.RollValue("benefit", 4000)
		}
	case "TY-U01":
		if !d.LegendaryBasic {
			m.PrepareReductionBP += c.RollValue("benefit", 3000)
		}
	case "TY-U04":
		if !d.LegendaryBasic && universalNativeHealingPayload(c, d) {
			m.ManaReductionBP += c.RollValue("benefit", 3000)
		}
	case "TY-U06":
		m.RestoreMPBP += c.RollValue("benefit", 5000)
	case "TY-U07":
		if universalHasEffect(d, domain.EffectTeleportPoint, domain.EffectTeleportTarget) {
			d.Dist = universalScaleBP(d.Dist, c.RollValue("benefit", 4000))
		}
	case "TY-U12":
		if !d.LegendaryBasic && universalNativeStatusKind(c, d, true) {
			m.ManaReductionBP += c.RollValue("benefit", 3000)
		}
	case "TY-U14":
		if !d.LegendaryBasic && universalNativeStatusKind(c, d, false) {
			m.ManaReductionBP += c.RollValue("benefit", 3000)
		}
	case "TY-U16":
		m.HealingBP += c.RollValue("benefit", 6000)
	case "TY-U17":
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectResurrect {
				e.HealPctOfMax = min(int32(100), universalScaleBP(e.HealPctOfMax, c.RollValue("benefit", 5000)))
				e.HealFlat = universalScaleBP(e.HealFlat, c.RollValue("benefit", 5000))
			}
		})
	case "TY-U18":
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.ClearHarmful || len(e.RemoveStatusIDs) > 0 {
				e.DispelChanceBP = min(int32(10000), e.DispelChanceBP+c.RollValue("benefit", 4000))
			}
		})
	case "TY-U19":
		if universalHasEffect(d, domain.EffectDetectTrap) {
			m.RadiusBP += c.RollValue("benefit", 10000)
		}
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectRevealInvisible {
				e.RevealRadius = universalScaleBP(e.RevealRadius, c.RollValue("benefit", 10000))
			}
		})
	}
}
func universalEditEffects(d *domain.SkillDef, f func(*domain.SkillEffect)) {
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		f(&d.Effects[i])
	}
}
func universalHasNativeShield(c *legendaryContext) bool {
	found := false
	if c.Owner != nil && c.Owner.Status != nil {
		c.Owner.Status.EachEffective(func(a domain.Active) {
			if a.ShieldLeft > 0 && a.Def.LegendaryPowerID == "" {
				found = true
			}
		})
	}
	return found
}

// Native healing includes periodic HP restoration carried by a skill status.
// It does not turn unrelated buffs or mana restoration into healing spells.
func universalNativeHealingPayload(c *legendaryContext, d *domain.SkillDef) bool {
	if universalHasEffect(d, domain.EffectHeal, domain.EffectRestoreHP) {
		return true
	}
	for _, app := range d.Statuses {
		if st, ok := c.Scene.resolveSkillStatusDef(*d, app, 0); ok && st.IntervalSec > 0 && universalStatusHeals(st) {
			return true
		}
	}
	return false
}
