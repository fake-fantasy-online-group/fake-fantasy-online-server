package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Replacement identities reuse native skills and statuses; none introduces an
// equipment timer, delayed condition, charge ledger or internal cooldown.
func init() {
	registerLegendarySynergyHandler("SS-A01", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFireball {
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 7000)
		}
	}, TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Skill == legendaryFireball && legendaryCasterBurn(c, c.Target) > 0 {
			legendaryAddTargetDamage(d, 4000)
		}
	}})
	registerLegendarySynergyHandler("SS-A18", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFlame {
			// Native Flame is circular: its width is its diameter. Keep
			// that native shape and apply the authored 80% to its extent.
			if d.Area.Shape == domain.SkillAreaCircle {
				d.LegendaryModifiers.RadiusBP += 8000
			} else {
				d.LegendaryModifiers.WidthBP += 8000
			}
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 4000)
		}
	}})
	registerLegendarySynergyHandler("SS-A22", legendaryHandler{TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Skill == legendaryBlizzard && legendaryCasterControlled(c.Target) {
			legendaryAddTargetDamage(d, c.RollValue("damage_bp", 9000))
		}
	}})
	registerLegendarySynergyHandler("SS-A23", legendaryHandler{BaseSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFreeze {
			specialCircle(d, max(d.Radius, d.Area.Radius)+c.RollValue("radius", 150), domain.SkillAreaCenterTarget, 5)
		}
	}})
	registerLegendarySynergyHandler("SS-U09", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFireball || d.ID == legendaryIceBurst || d.ID == legendaryFlame || d.ID == legendaryBlizzard {
			d.LegendaryModifiers.CooldownReductionBP += c.RollValue("cooldown_bp", 3000)
			d.LegendaryModifiers.ManaReductionBP += 2000
		}
	}})
	registerLegendarySynergyHandler("YS-A04", legendaryHandler{TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Skill == legendaryLightBolt && (legendaryCasterOwnSkill(c.Target, c.Owner.ID, 10324) || legendaryCasterOwnSkill(c.Target, c.Owner.ID, 10325)) {
			legendaryAddTargetDamage(d, c.RollValue("damage_bp", 9000))
		}
	}})
	registerLegendarySynergyHandler("YS-A10", legendaryHandler{BaseSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10324 {
			specialCircle(d, max(d.Radius, d.Area.Radius)+100, domain.SkillAreaCenterTarget, 0)
		}
	}, StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10324 {
			d.Affixes = append([]domain.Affix(nil), d.Affixes...)
			for i := range d.Affixes {
				if d.Affixes[i].Value < 0 {
					d.Affixes[i].Value = specialScale(d.Affixes[i].Value, 100+c.RollValue("weaken_pct", 60))
				}
			}
		}
	}})
	registerLegendarySynergyHandler("YS-A11", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryExorcism {
			d.Radius += 120
			d.Area.Radius += 120
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 4500)
		}
	}})
	registerLegendarySynergyHandler("YS-A13", legendaryHandler{TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Skill == legendaryDivine && legendaryCasterOwnSkill(c.Target, c.Owner.ID, 10325) {
			legendaryAddTargetDamage(d, c.RollValue("damage_bp", 8000))
		}
	}})
	registerLegendarySynergyHandler("YS-A16", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryDivine {
			d.Radius += 100
			d.Area.Radius += 100
			d.Dist += 200
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 3500)
		}
	}})
	registerLegendarySynergyHandler("YS-A18", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10325 {
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 6000)
		}
	}, StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10325 {
			d.Affixes = append([]domain.Affix(nil), d.Affixes...)
			for i := range d.Affixes {
				if d.Affixes[i].Attr == domain.AttrAtkSpeed {
					d.Affixes[i].Value = specialScale(d.Affixes[i].Value, 150)
				}
			}
		}
	}})
	registerLegendarySynergyHandler("YS-A25", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10307 {
			specialCircle(d, 250, domain.SkillAreaCenterTarget, 0)
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10307 {
			d.LegendaryModifiers.StatusPowerBP += c.RollValue("buff_bp", 6000)
		}
	}})
	registerLegendarySynergyHandler("YS-A26", legendaryHandler{StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10321 {
			d.HolyActiveDamageBonusBP += 4000
		}
	}, SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10321 {
			return legendaryCasterAdditionalAllies(c, ids, 250, 2)
		}
		return ids
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10321 {
			d.LegendaryModifiers.StatusPowerBP += c.RollValue("buff_bp", 7000)
		}
	}})
	registerLegendarySynergyHandler("YS-A27", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if legendaryCasterHasSkill(c.Owner, 10311) && (d.ID == legendaryLightBolt || d.ID == legendaryHolyLight || d.ID == legendaryExorcism) {
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 5000)
		}
	}})
	h := legendaryCasterSupportWave(10302, 180, 220)
	h.PrepareSkill = func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10302 {
			d.LegendaryModifiers.RestoreMPBP += 7000
		}
	}
	registerLegendarySynergyHandler("YS-A28", h)
}

func legendaryCasterHolyStatusBonus(e *entity.Entity, d *domain.SkillDef) {
	if e == nil || e.Status == nil || d.HurtType != domain.HurtHoly || d.LegendaryBasic || !d.Kind.Active() {
		return
	}
	e.Status.EachEffective(func(a domain.Active) { d.LegendaryModifiers.DamageBP += a.Def.HolyActiveDamageBonusBP })
}
