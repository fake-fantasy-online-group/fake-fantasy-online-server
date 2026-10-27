package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Preparation must never consume a charge: an invalid target, insufficient mana
// or an interrupted windup must not spend a legendary grant.
func init() {
	registerLegendarySynergyHandler("SS-A20", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryBlizzard {
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 5500)
			d.LegendaryModifiers.RadiusBP += 4000
			d.LegendaryModifiers.CooldownReductionBP += 2000
		}
	}})
	registerLegendarySynergyHandler("SS-A06", legendaryHandler{
		PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if d.ID == legendaryFireball {
				d.LegendaryModifiers.RadiusBP += c.RollValue("radius_bp", 6500)
			}
		},
		TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
			if c.Skill != legendaryFireball || c.Target == nil {
				return n
			}
			d, ok := legendaryCasterSkill(c, c.Skill)
			if !ok {
				return n
			}
			center := legendaryCasterCenter(c)
			r := max(d.Radius, d.Area.Radius)
			if sqDist(center, c.Target.Pos) > float64(r)*float64(r)/4 {
				return specialScale(n, 145)
			}
			return specialScale(n, 120)
		}),
	})
	registerLegendarySynergyHandler("SS-A13", legendaryHandler{TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill != legendaryIceBurst || c.Target == nil {
			return n
		}
		d, ok := legendaryCasterSkill(c, c.Skill)
		if !ok {
			return n
		}
		r := max(d.Radius, d.Area.Radius)
		if sqDist(legendaryCasterCenter(c), c.Target.Pos) <= float64(r)*float64(r)/4 {
			return specialScale(n, 100+c.RollValue("damage_pct", 85))
		}
		return specialScale(n, 100+c.RollValue("damage_pct", 65))
	})})
	registerLegendarySynergyHandler("SS-A21", legendaryHandler{TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill != legendaryBlizzard || c.Target == nil {
			return n
		}
		d, ok := legendaryCasterSkill(c, c.Skill)
		if !ok {
			return n
		}
		r := float64(max(d.Radius, d.Area.Radius)) * 2 / 3
		if sqDist(legendaryCasterCenter(c), c.Target.Pos) <= r*r {
			return specialScale(n, 100+c.RollValue("damage_pct", 80))
		}
		return specialScale(n, 100+c.RollValue("damage_pct", 20))
	})})
	registerLegendarySynergyHandler("SS-U01", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != 10432 {
			return
		}
		d.CooldownMS = 0
		d.CooldownFractionNumeratorMS = 0
		d.CooldownFractionDenominator = 0
		d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 1500)
		d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
		for i := range d.Effects {
			if d.Effects[i].Kind == domain.EffectTeleportPoint || d.Effects[i].Kind == domain.EffectTeleportTarget {
				d.Effects[i].ChanceBP = 10000
			}
		}
	}})
	registerLegendarySynergyHandler("SS-U02", legendaryHandler{Dispelled: legendaryCasterSteal, TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10433 {
			specialCircle(d, c.RollValue("radius", 250), domain.SkillAreaCenterTarget, 0)
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != 10433 {
			return
		}
		d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
		for i := range d.Effects {
			if len(d.Effects[i].RemoveStatusIDs) > 0 {
				d.Effects[i].DispelChanceBP = 10000
			}
		}
	}})
	registerLegendarySynergyHandler("YS-A06", legendaryHandler{
		PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if d.ID == legendaryHolyLight {
				specialCircle(d, 200, domain.SkillAreaCenterTarget, 0)
			}
		},
		TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
			if c.Skill != legendaryHolyLight || c.Target == nil {
				return n
			}
			if sqDist(legendaryCasterCenter(c), c.Target.Pos) <= 100*100 {
				return specialScale(n, 100+c.RollValue("damage_pct", 60))
			}
			return specialScale(n, 100+c.RollValue("damage_pct", 20))
		}),
	})
	registerLegendarySynergyHandler("YS-U01", legendaryHandler{StatusApplied: legendaryCasterRegen, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10303 {
			d.CooldownMS = 0
			d.CooldownFractionNumeratorMS = 0
			d.CooldownFractionDenominator = 0
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 2000)
		}
	}})
	registerLegendarySynergyHandler("YS-U02", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10322 {
			d.LegendaryModifiers.ManaReductionBP += 3000
		}
		if len(d.Statuses) > 0 && legendaryCasterNativeReady(c.Owner, true, false, false) {
			d.LegendaryModifiers.StatusPowerBP += c.RollValue("buff_bp", 5000)
		}
	}})

}
func legendaryCasterCenter(c *legendaryContext) domain.Pos {
	if c.IsChild {
		return c.Center
	}
	if root := c.Scene.equipmentState().root; root != nil {
		return root.center
	}
	return c.Center
}
