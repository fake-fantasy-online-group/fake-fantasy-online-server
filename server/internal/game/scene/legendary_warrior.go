package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Warrior synergy modifiers join the shared additive equipment bucket. They
// modify the accepted native attack, rather than manufacturing timed procs.
func warriorSkill(id domain.SkillID, damage, mana, radius, cooldown int64) legendaryHandler {
	return legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != id {
			return
		}
		d.LegendaryModifiers.DamageBP += int32(damage * 100)
		d.LegendaryModifiers.ManaReductionBP += int32(mana * 100)
		d.LegendaryModifiers.RadiusBP += int32(radius * 100)
		d.LegendaryModifiers.CooldownReductionBP += int32(cooldown * 100)
	}}
}
func warriorDamageSkills(pct int64, ids ...domain.SkillID) legendaryHandler {
	return legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		for _, id := range ids {
			if d.ID == id {
				d.LegendaryModifiers.DamageBP += int32(pct * 100)
				return
			}
		}
	}}
}
func warriorIncoming(pct int64) legendaryHandler {
	return legendaryHandler{ModifyIncoming: func(c *legendaryContext, n int32) int32 {
		if warriorDirectIncoming(c) {
			return legendaryMeleeScale(n, 100-pct)
		}
		return n
	}}
}
func warriorEffects(d *domain.SkillDef, f func(*domain.SkillEffect)) {
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		f(&d.Effects[i])
	}
}
func warriorLearned(c *legendaryContext, id domain.SkillID, composed bool) (domain.SkillDef, bool) {
	if c.Owner == nil || c.Owner.Player == nil || c.Owner.Player.Char.Skills.LevelOf(id) <= 0 || c.Scene.skills == nil {
		return domain.SkillDef{}, false
	}
	lv, _ := c.Scene.skillLevelForUse(c.Owner, id)
	d, ok := c.Scene.skills.Get(id, lv)
	if !ok {
		return d, false
	}
	c.Scene.applyResolvedSkillModifiers(c.Owner, &d, c.Owner.Player.Char.Skills, c.Scene.rawSkillLevels(c.Owner))
	if composed {
		c.Scene.legendaryPrepareSkill(c.Owner, &d, c.Primary)
	}
	return d, true
}
func init() {
	registerWarriorPower("ZS-A01", warriorSkill(10002, 80, 0, 0, 0))
	registerWarriorPower("ZS-A04", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10005 {
			d.LegendaryModifiers.DamageBP += 1500
			d.LegendaryModifiers.WidthBP += 9000
			d.LegendaryModifiers.LengthBP += 2000
		}
	}})
	registerWarriorPower("ZS-A05", warriorSkill(10005, 60, 0, 0, 0))
	registerWarriorPower("ZS-A11", warriorSkill(10017, 60, 0, 0, 0))
	registerWarriorPower("ZS-A13", warriorSkill(10006, 45, 0, 20, 0))
	registerWarriorPower("ZS-A15", warriorSkill(10006, 60, 0, 0, 0))
	registerWarriorPower("ZS-A17", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10003 {
			d.LegendaryModifiers.DamageBP += 3000
			d.WeaponDurabilityReductionPct = 100
			warriorEffects(d, func(e *domain.SkillEffect) { e.WeaponDurabilityCostPct = 0 })
		}
	}})
	registerWarriorPower("ZS-A19", warriorSkill(10021, 70, 0, 0, 0))
	registerWarriorPower("ZS-A29", legendaryHandler{BaseSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if legendaryMeleeSpearActive(*d) && !d.LegendaryBasic {
			warriorEffects(d, func(e *domain.SkillEffect) {
				if e.Kind == domain.EffectDamage {
					e.BaseDamageFlat += legendaryMeleeScale(c.Attack(false), 180)
				}
			})
		}
	}})
	registerWarriorPower("ZS-A30", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10002 || d.ID == 10017 {
			d.LegendaryModifiers.DamageBP += 6500
		}
		if d.ID == 10002 {
			if fly, ok := warriorLearned(c, 10017, true); ok {
				d.Dist = max(d.Dist, fly.Dist)
			}
		}
	}})
	registerWarriorPower("ZS-D05", warriorIncoming(40))
	registerWarriorPower("ZS-D07", warriorIncoming(45))
	registerWarriorPower("ZS-D06", legendaryHandler{ModifyIncoming: func(c *legendaryContext, n int32) int32 {
		if warriorDirectIncoming(c) && sqDist(c.Owner.Pos, c.Target.Pos) <= 2.25*legendaryMeleeReach(c)*legendaryMeleeReach(c) {
			return legendaryMeleeScale(n, 65)
		}
		return n
	}})
	h := warriorIncoming(30)
	h.PrepareSkill = warriorSkill(10022, 0, 0, 20, 0).PrepareSkill
	registerWarriorPower("ZS-D08", h)
	registerWarriorPower("ZS-U01", warriorSkill(10002, 0, 40, 0, 0))
	registerWarriorPower("ZS-U05", warriorSkill(10006, 0, 45, 45, 0))
	registerWarriorPower("ZS-U06", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if legendaryMeleeSpearActive(*d) && !d.LegendaryBasic {
			d.LegendaryModifiers.ManaReductionBP += 6000
		}
	}})
	registerWarriorPower("ZS-U07", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if c.IsBasic && legendaryMeleeLanded(c) && legendaryMeleeOnce(c, 0, "basic-mana") {
			legendaryMeleeRestoreMP(c, legendaryMeleeScale(c.Owner.MaxMP, 5))
		}
	}})
	registerWarriorPower("ZS-U09", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10017 {
			d.PrepareMS /= 2
		}
		if d.ID == 10017 || d.ID == 10027 {
			d.LegendaryModifiers.CooldownReductionBP += 5000
		}
	}})
}

func warriorDirectIncoming(c *legendaryContext) bool {
	return c.Owner != nil && c.Target != nil && (c.Blow.Origin == domain.DamageNormalOrigin || c.Blow.LegendaryChild)
}

// The contract and synergy suites exercise these complete replacement powers.
func registerWarriorPower(id string, h legendaryHandler) {
	registerLegendarySynergyHandler(id, h)
	certifyLegendaryPower(id)
}
