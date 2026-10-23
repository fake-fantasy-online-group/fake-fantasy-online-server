package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func warriorDrain(c *legendaryContext, d *domain.SkillDef) {
	if d.ID != 10013 {
		return
	}
	if c.Power.ID == "ZS-U02" {
		d.LegendaryModifiers.ManaDrainBP += 6000
	}
	if c.Power.ID == "ZS-D04" {
		d.LegendaryModifiers.ManaDrainBP += 2500
	}
}

func warriorArmor(c *legendaryContext, n int32) int32 {
	if !legendaryMeleeDirect(c) || c.Target == nil {
		return n
	}
	if c.Target.Monster != nil && c.Scene.defs != nil {
		if d, ok := c.Scene.defs.Def(c.Target.Monster.TypeID); ok {
			if _, immune := d.ImmuneSkills[10009]; immune {
				return n
			}
		}
	}
	d, ok := warriorLearned(c, 10009, false)
	if !ok {
		return n
	}
	for _, a := range d.Statuses {
		if !a.TargetType.Matches(c.Target.Kind) {
			continue
		}
		st, ok := c.Scene.resolveSkillStatusDef(d, a, 0)
		if !ok {
			continue
		}
		armor := st.PhysicalDamageTakenPct > 0
		for _, af := range st.Affixes {
			armor = armor || af.Attr == domain.AttrDef && af.Value < 0
		}
		if armor {
			c.Scene.applyStatusDef(c.Target, st, c.Owner.ID)
		}
	}
	return n
}
func warriorArmorStrength(c *legendaryContext, d *domain.StatusDef) {
	if c.Skill != 10009 && c.Skill != 10005 {
		return
	}
	pct := int32(0)
	if warriorHas(c, "ZS-A08") {
		pct += 50
		if c.Power.ID != "ZS-A08" {
			return
		}
	}
	if warriorHas(c, "ZS-D02") {
		pct += 25
	}
	d.PhysicalDamageTakenPct = specialScale(d.PhysicalDamageTakenPct, 100+pct)
	d.Affixes = append([]domain.Affix(nil), d.Affixes...)
	for i := range d.Affixes {
		if d.Affixes[i].Attr == domain.AttrDef && d.Affixes[i].Value < 0 {
			d.Affixes[i].Value = specialScale(d.Affixes[i].Value, 100+pct)
		}
	}
}

func init() {
	h := warriorDamageSkills(50, 10005, 10009)
	h.BeforeDamage = func(c *legendaryContext, n int32) int32 {
		if c.Skill == 10005 {
			return warriorArmor(c, n)
		}
		return n
	}
	registerWarriorPower("ZS-A07", h)
	registerWarriorPower("ZS-A08", legendaryHandler{SelectTargets: warriorPreservePrimary, TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10009 {
			if p, ok := warriorLearned(c, 10005, true); ok {
				d.Kind = domain.SkillArea
				d.Area = p.Area
				d.Radius = p.Radius
				d.LegendaryModifiers.GeometryInherited = true
			}
		}
	}, StatusApplied: warriorArmorStrength})
	registerWarriorPower("ZS-A09", legendaryHandler{PrepareSkill: warriorSkill(10013, 25, 0, 0, 0).PrepareSkill, TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10013 && legendaryMeleeShieldRemaining(c) > 0 {
			legendaryAddTargetDamage(d, 9500)
		}
	}})
	registerWarriorPower("ZS-A10", legendaryHandler{BaseSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != 10013 {
			return
		}
		var drain int32
		for _, e := range d.DirectEffects() {
			if e.Kind == domain.EffectDamageMP {
				native := int64(e.DamageFlat) + int64(c.Scene.equipmentAttackStats(c.Owner).MaxAtk)*int64(e.SourceAttackPct)/100
				drain += domain.SaturateInt32(native * (10000 + int64(d.LegendaryModifiers.ManaDrainBP)) / 10000)
			}
		}
		warriorEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectDamage {
				e.BaseDamageFlat += 2 * drain
			}
		})
	}})
	registerWarriorPower("ZS-A20", legendaryHandler{SelectTargets: warriorPreservePrimary, BaseSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10015 {
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: 180, Width: 120}
			warriorEffects(d, func(e *domain.SkillEffect) {
				if e.Kind == domain.EffectDamage {
					e.GuaranteedHit = true
					e.BaseDamageFlat += legendaryMeleeScale(c.Attack(false), 240)
				}
			})
		}
	}})
	registerWarriorPower("ZS-A21", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if !d.LegendaryBasic {
			return
		}
		if eye, ok := warriorLearned(c, 10015, false); ok {
			d.ID = eye.ID
			d.Effects = append([]domain.SkillEffect(nil), eye.DirectEffects()...)
			warriorEffects(d, func(e *domain.SkillEffect) {
				if e.Kind == domain.EffectDamage {
					e.GuaranteedHit = true
				}
			})
		} else {
			warriorEffects(d, func(e *domain.SkillEffect) {
				if e.Kind == domain.EffectDamage {
					e.GuaranteedHit = true
				}
			})
			d.LegendaryModifiers.DamageBP += 10000
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10015 && !d.LegendaryBasic {
			d.LegendaryModifiers.DamageBP += 10000
		}
	}})
	h = warriorDamageSkills(35, 10023, 10022)
	h.BeforeDamage = func(c *legendaryContext, n int32) int32 {
		if c.Skill == 10023 || c.Skill == 10022 {
			return warriorArmor(c, n)
		}
		return n
	}
	registerWarriorPower("ZS-A25", h)
	h = warriorIncoming(16)
	h.StatusApplied = warriorArmorStrength
	registerWarriorPower("ZS-D02", h)
	registerWarriorPower("ZS-D04", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) { warriorDrain(c, d) }, ModifyIncoming: func(c *legendaryContext, n int32) int32 {
		if warriorDirectIncoming(c) && c.Blow.Magic {
			return legendaryMeleeScale(n, 40)
		}
		return n
	}})
	registerWarriorPower("ZS-U02", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) { warriorDrain(c, d) }, PrepareSkill: warriorSkill(10013, 0, 60, 0, 0).PrepareSkill})
}
