package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Direction defaults are explicit numerical specifications, not prose-driven handlers.
func init() {
	registerLegendarySynergyHandler("SS-A10", legendaryHandler{ModifyBlow: func(c *legendaryContext, b *combat.Blow) {
		if c.Skill == legendaryIceArrow {
			b.ElementReductionPenetrationBP += 5000
		}
	}})
	registerLegendarySynergyHandler("YS-A03", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryLightBolt {
			d.LegendaryModifiers.DamageBP += 6000
		}
	}, SkillResolved: func(c *legendaryContext) {
		if c.Skill != legendaryLightBolt || c.Target == nil || c.Target.Status == nil {
			return
		}
		removed := false
		c.Scene.statusesRemoved(c.Target, c.Target.Status.RemoveWhere(func(d domain.StatusDef) bool {
			if removed || d.Permanent || d.Harmful() || d.IntervalSec <= 0 {
				return false
			}
			for _, r := range d.HPChanges {
				if r.Value > 0 {
					removed = true
					return true
				}
			}
			return false
		}))
	}})
	registerLegendarySynergyHandler("YS-A08", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryHolyLight {
			d.Kind = domain.SkillArea
			d.Radius = 0
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: 600, Width: 160}
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryHolyLight {
			d.LegendaryModifiers.DamageBP += 8000
		}
	}})
	registerLegendarySynergyHandler("YS-A12", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryExorcism {
			d.LegendaryModifiers.DamageBP += 5000
		}
	}, SkillResolved: func(c *legendaryContext) {
		if c.Skill == legendaryExorcism && c.Target != nil && !c.Scene.legendaryCasterSkillImmune(c.Target, c.Skill) {
			c.Scene.clearCastWindup(c.Target.ID)
		}
	}})
	registerLegendarySynergyHandler("YS-A14", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryDivine {
			d.LegendaryModifiers.CooldownReductionBP += 4500
		}
	}})
	registerLegendarySynergyHandler("YS-A22", legendaryCasterSupportWave(10306, 220, 220))
	registerLegendarySynergyHandler("YS-A24", legendaryCasterSupportWave(10304, 250, 250))
	registerLegendarySynergyHandler("YS-D01", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10301 {
			d.LegendaryModifiers.HealingBP += 6000
		}
	}})
	registerLegendarySynergyHandler("YS-D02", legendaryCasterGroupCleanse(10306, true))
	registerLegendarySynergyHandler("YS-D07", legendaryCasterGroupCleanse(10316, false))
	registerLegendarySynergyHandler("YS-U10", legendaryCasterGroupCleanse(10318, true))
	registerLegendarySynergyHandler("YS-D04", legendaryHandler{StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10321 {
			d.PhysicalDamageReductionPct += 35
			d.MagicDamageReductionPct += 35
		}
	}})
	registerLegendarySynergyHandler("YS-D06", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if c.Scene.legendaryCasterWinding(c.Owner, 10304) {
			return specialScale(n, 60)
		}
		return n
	}})
	registerLegendarySynergyHandler("YS-D08", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if c.Scene.legendaryCasterWinding(c.Owner, 10305) {
			return specialScale(n, 40)
		}
		return n
	}})
	registerLegendarySynergyHandler("YS-D09", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if c.Skill == 0 && legendaryCasterHasSkill(c.Owner, 10330) {
			return specialScale(n, 65)
		}
		return n
	}, StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10330 {
			d.PhysicalReflectPct = specialScale(d.PhysicalReflectPct, 175)
		}
	}})
	registerLegendarySynergyHandler("YS-D10", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryCasterOwnSkill(c.Target, c.Owner.ID, 10325) {
			return specialScale(n, 60)
		}
		return n
	}})
	registerLegendarySynergyHandler("YS-U08", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if (d.TargetSelf || d.TargetTeam) && !d.TargetEnemy && d.Area.Shape == domain.SkillAreaNone {
			d.Dist += 200
			d.LegendaryModifiers.ManaReductionBP += 2500
		}
		if d.ID == 10327 {
			d.LegendaryRevealPoint = true
			d.Dist = 600
		}
	}})
	registerLegendarySynergyHandler("YS-U09", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10305 {
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			for i := range d.Effects {
				if d.Effects[i].Kind == domain.EffectResurrect {
					d.Effects[i].HealPctOfMax = max(d.Effects[i].HealPctOfMax, 70)
				}
			}
		}
	}, Resurrected: func(c *legendaryContext) {
		if c.Skill == 10305 && legendaryCasterOnce(c, 0, "revive-refund") {
			c.RefundMP(specialScale(c.PaidMP, 50))
		}
	}})
}
func legendaryCasterSupportWave(skill domain.SkillID, radius float64, pct int32) legendaryHandler {
	return legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill != skill || !legendaryCasterOnce(c, 0, "support-wave") {
			return
		}
		center := legendaryCasterCenter(c)
		if (skill == 10306 || skill == 10302) && c.Target != nil {
			center = c.Target.Pos
		}
		d := domain.SkillDef{ID: skill, Level: c.Def.Level, Kind: domain.SkillSingle, TargetEnemy: true, HurtType: domain.HurtHoly, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		for _, target := range c.Enemies(center, radius, 256) {
			c.ChildSkill(target.ID, d, legendaryChildOptions{BaseDamage: specialScale(c.Attack(true), pct), TargetOnly: true, SuppressGeneration: true})
		}
	}}
}
func legendaryCasterGroupCleanse(skill domain.SkillID, guaranteed bool) legendaryHandler {
	return legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != skill {
			return
		}
		specialCircle(d, 250, domain.SkillAreaCenterTarget, 0)
		d.TargetSelf = true
		d.TargetTeam = true
		d.TargetEnemy = false
		if guaranteed {
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			for i := range d.Effects {
				if len(d.Effects[i].RemoveStatusIDs) > 0 || d.Effects[i].ClearHarmful {
					d.Effects[i].DispelChanceBP = 10000
				}
			}
		}
	}}
}
func legendaryCasterOwnSkill(e *entity.Entity, owner domain.EntityID, skill domain.SkillID) bool {
	if e == nil || e.Status == nil {
		return false
	}
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if a.Source == owner && (a.Def.SkillSnapshotID == skill || a.Def.SourceSkillID == int32(skill)) {
			found = true
		}
	})
	return found
}
func (s *Scene) legendaryCasterWinding(e *entity.Entity, skill domain.SkillID) bool {
	if e == nil {
		return false
	}
	for _, cast := range s.casts {
		if cast.src == e.ID && cast.def.ID == skill && !cast.released && !cast.releaseAtWall.IsZero() && (cast.windup == nil || !cast.windup.finished) {
			return true
		}
	}
	return false
}

func (s *Scene) legendaryCasterSkillImmune(e *entity.Entity, skill domain.SkillID) bool {
	if e == nil || e.Monster == nil || s.defs == nil {
		return false
	}
	if d, ok := s.defs.Def(e.Monster.TypeID); ok {
		_, immune := d.ImmuneSkills[skill]
		return immune
	}
	return false
}
