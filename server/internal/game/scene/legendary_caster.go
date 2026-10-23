package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	legendaryFireBolt  domain.SkillID = 10401
	legendaryFireball  domain.SkillID = 10402
	legendaryFlame     domain.SkillID = 10405
	legendaryIceArrow  domain.SkillID = 10409
	legendaryIceBurst  domain.SkillID = 10410
	legendaryBlizzard  domain.SkillID = 10412
	legendaryFreeze    domain.SkillID = 10428
	legendaryLightBolt domain.SkillID = 10319
	legendaryHolyLight domain.SkillID = 10317
	legendaryExorcism  domain.SkillID = 10312
	legendaryDivine    domain.SkillID = 10326
)

func legendaryCasterLanded(c *legendaryContext) bool {
	return c.Owner != nil && c.Target != nil && !c.Hit.Flag.Has(event.DamageMiss) && c.Blow.ProcDepth == 0 && !c.Blow.Trap && c.Scene.equipmentState().classSetTriggered == 0 && c.Blow.Origin == domain.DamageNormalOrigin
}
func legendaryCasterOnce(c *legendaryContext, target domain.EntityID, phase string) bool {
	st := c.State(target, phase)
	if st.RootID == c.RootID && st.Stacks != 0 {
		return false
	}
	st.RootID, st.Stacks = c.RootID, 1
	return true
}
func legendaryCasterBurn(c *legendaryContext, target *entity.Entity) int64 {
	if target == nil || target.Status == nil {
		return 0
	}
	var value int64
	target.Status.EachEffective(func(a domain.Active) {
		if a.Source == c.Owner.ID && a.Def.ID == 1036 {
			value += classSetPoisonRemaining(a, c.Scene.tick, target.MaxHP) * (10000 + int64(a.Def.LegendaryModifiers.DamageTickBP)) / 10000
		}
	})
	return value
}
func legendaryCasterSkill(c *legendaryContext, skill domain.SkillID) (domain.SkillDef, bool) {
	if c.Scene.skills == nil || c.Owner == nil || c.Owner.Player == nil || c.Owner.Player.Char == nil || c.Owner.Player.Char.Skills[skill] <= 0 {
		return domain.SkillDef{}, false
	}
	rank, _ := c.Scene.skillLevelForUse(c.Owner, skill)
	return c.Scene.skills.Get(skill, rank)
}
func legendaryCasterBase(c *legendaryContext, skill domain.SkillID) int32 {
	d, ok := legendaryCasterSkill(c, skill)
	if !ok {
		return 0
	}
	for _, e := range d.DirectEffects() {
		if e.Kind == domain.EffectDamage {
			if e.DamagePctDenominator > 0 {
				return clampI32(int64(c.Attack(true))*e.DamagePctNumerator/(100*e.DamagePctDenominator) + int64(e.DamageFlat))
			}
			return clampI32(int64(c.Attack(true))*int64(e.DamagePct)/100 + int64(e.DamageFlat))
		}
	}
	return 0
}
func legendaryCasterBurnBurst(skill domain.SkillID, percent int32) legendaryHandler {
	return legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill != skill || c.Target == nil || c.Blow.ProcDepth > 0 && !c.IsChild || !legendaryCasterOnce(c, c.Target.ID, "burn-burst") {
			return
		}
		c.Damage(c.Target, clampI32(legendaryCasterBurn(c, c.Target)*int64(percent)/100), true, false)
	}}
}
func legendaryCasterFlatSkill(skill domain.SkillID, percent int32) legendaryHandler {
	return legendaryHandler{TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, amount int32) int32 {
		if c.Skill == skill {
			return specialScale(amount, percent)
		}
		return amount
	})}
}
func legendaryCasterHas(t *entity.Entity, id domain.StatusID) bool {
	return t != nil && t.Status != nil && t.Status.HasEffective(id)
}

func init() {
	registerLegendarySynergyHandler("SS-A02", legendaryCasterBurnBurst(legendaryFireBolt, 80))
	registerLegendarySynergyHandler("SS-A11", legendaryCasterBurnBurst(legendaryIceArrow, 100))
	registerLegendarySynergyHandler("SS-A16", legendaryCasterBurnBurst(legendaryFlame, 160))
	registerLegendarySynergyHandler("SS-A08", legendaryHandler{TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill != legendaryIceArrow || c.Target == nil {
			return n
		}
		d, ok := legendaryCasterSkill(c, c.Skill)
		if !ok {
			return n
		}
		if sqDist(c.Owner.Pos, c.Target.Pos) >= float64(d.Dist)*float64(d.Dist)/4 {
			return specialScale(n, 155)
		}
		return specialScale(n, 120)
	})})
	registerLegendarySynergyHandler("SS-A15", legendaryHandler{TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill == legendaryIceBurst && (legendaryCasterHas(c.Target, domain.StatusFreeze) || legendaryCasterHas(c.Target, domain.StatusSlow)) {
			return specialScale(n, 160)
		}
		return n
	})})

	registerLegendarySynergyHandler("YS-A02", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if c.Skill == legendaryLightBolt && legendaryCasterLanded(c) && legendaryCasterOnce(c, c.Target.ID, "second-light") {
			c.Damage(c.Target, specialScale(c.Hit.Amount, 85), true, true)
		}
	}})
	registerLegendarySynergyHandler("YS-A01", legendaryHandler{
		TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if d.ID == legendaryLightBolt {
				specialCircle(d, 180, domain.SkillAreaCenterTarget, 4)
			}
		},
		TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
			if d.ID == legendaryLightBolt && c.Target != nil && c.Target.ID != c.Primary && d.LegendaryNextDamageBonusPct == 0 {
				d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
				for i := range d.Effects {
					if d.Effects[i].Kind == domain.EffectDamage {
						domain.ScaleSkillDamageFraction(&d.Effects[i], 60, 100)
					}
				}
				d.Statuses = nil
			}
		},
	})

	registerLegendarySynergyHandler("YS-A15", legendaryHandler{
		AfterHit: func(c *legendaryContext) {
			if c.Skill != legendaryDivine || !legendaryCasterLanded(c) {
				return
			}
			st := c.State(0, "divine-pool")
			if st.RootID != c.RootID {
				st.RootID = c.RootID
				st.Value = 0
				st.Stacks = 0
			}
			if c.Target.ID == c.Primary {
				st.Stacks = c.Hit.Amount
			} else {
				st.Value += int64(c.LostHP)
			}
		},
		RootFinished: func(c *legendaryContext) {
			if c.Skill != legendaryDivine || c.Target == nil {
				return
			}
			st := c.State(0, "divine-pool")
			if st.RootID == c.RootID {
				c.Damage(c.Target, min(st.Stacks, clampI32(st.Value*35/100)), true, true)
			}
		}})

}
