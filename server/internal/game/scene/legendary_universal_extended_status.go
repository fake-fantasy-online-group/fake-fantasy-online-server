package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
)

func init() {
	registerLegendarySynergyHandler("TY-A15", legendaryHandler{StatusAccuracy: func(c *legendaryContext, d *domain.StatusDef) {
		if d != nil && !d.LegendaryModifiers.Applied && d.Harmful() && d.LegendaryPowerID == "" && (d.SkillApplied || d.SourceSkillID != 0 || d.SkillSnapshotID != 0) {
			d.EffectHitBP += 3000
		}
	}})
	registerLegendarySynergyHandler("TY-D12", legendaryHandler{IncomingHeal: func(c *legendaryContext, n int32) int32 { return universalScaleBP(n, 4000) }})
	registerLegendarySynergyHandler("TY-A17", legendaryHandler{StatusApplied: universalExtendedStatus, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) { d.LegendaryModifiers.LifeStealBP += 6000 }})

	for _, id := range []string{"TY-A13", "TY-A14", "TY-A22", "TY-A23", "TY-A28", "TY-A29", "TY-D18", "TY-U11"} {
		registerLegendarySynergyHandler(id, legendaryHandler{StatusApplied: universalExtendedStatus})
	}
	for _, id := range []string{"TY-D13", "TY-D14", "TY-D15", "TY-D20"} {
		registerLegendarySynergyHandler(id, legendaryHandler{StatusReceiving: universalExtendedReceiving})
	}
	for _, id := range []string{"TY-A11", "TY-A12", "TY-A33"} {
		registerLegendarySynergyHandler(id, legendaryHandler{ModifyBlow: universalExtendedBlow})
	}
}
func universalExtendedBlow(c *legendaryContext, b *combat.Blow) {
	if b.Origin == domain.DamageDOTOrigin || b.LegendaryPayload {
		return
	}
	switch c.Power.ID {
	case "TY-A11":
		b.CritChanceBonusBP += 2000
	case "TY-A12":
		b.FixedDefensePenetrationBP += 3000
	case "TY-A33":
		b.CriticalBonusBP += 5000
	}
}
func universalExtendedStatus(c *legendaryContext, d *domain.StatusDef) {
	if d == nil || d.LegendaryModifiers.Applied || d.LegendaryPowerID != "" || (!d.SkillApplied && d.SkillSnapshotID == 0 && d.SourceSkillID == 0) {
		return
	}
	m := &d.LegendaryModifiers
	switch c.Power.ID {
	case "TY-A13":
		if d.Harmful() && (d.Control != domain.ControlNone || d.BlockMove) {
			m.DurationBP += 4000
		}
	case "TY-A14":
		if d.Harmful() {
			m.SlowMagnitudeBP += 5000
		}
	case "TY-A17":
		if d.LifeStealPct > 0 {
			m.LifeStealBP += 6000
		}
	case "TY-A22":
		if d.ID == domain.StatusPoison || d.ID == 1036 {
			m.DurationBP += 5000
		}
	case "TY-A23":
		if d.PoisonOnHitChance > 0 {
			m.PoisonChancePct += 40
		}
	case "TY-A28", "TY-A29":
		if (c.Power.ID == "TY-A28" && d.ID == domain.StatusPoison) || (c.Power.ID == "TY-A29" && d.ID == 1036) {
			stats := c.Scene.equipmentAttackStats(c.Owner)
			m.PeriodicCritChanceBP = stats.CritRate
			if d.ID == 1036 {
				m.PeriodicCritChanceBP = stats.MCritRate
			}
			m.PeriodicCritChanceBP = max(int32(0), m.PeriodicCritChanceBP)
			m.PeriodicCanCrit = true
			c.Scene.forEachLegendary(c.Owner, func(other *legendaryContext) {
				if other.Power.ID == "TY-A11" {
					m.PeriodicCritChanceBP += 2000
				}
				if other.Power.ID == "TY-A33" {
					m.PeriodicCritBonusBP += 5000
				}
			})
		}
	case "TY-D18":
		if d.DamageShield > 0 {
			m.DurationBP += 4000
		}
	case "TY-U11":
		if !d.Harmful() && d.DamageShield <= 0 && !universalStatusHeals(*d) {
			m.DurationBP += 5000
		}
	}
}
func universalExtendedReceiving(c *legendaryContext, d *domain.StatusDef) bool {
	if d == nil || c.Owner == nil || c.Target == nil || d.LegendaryPowerID != "" {
		return true
	}
	if c.Power.ID == "TY-D13" {
		if c.Owner.ID != c.Target.ID && !d.Harmful() && d.DamageShield > 0 {
			d.LegendaryModifiers.ShieldBP += 4000
		}
		return true
	}
	if c.Owner.ID == c.Target.ID || sameParty(c.Owner, c.Target) || !d.Harmful() {
		return true
	}
	switch c.Power.ID {
	case "TY-D14":
		d.LegendaryModifiers.DefenseLossReductionBP += 4000
	case "TY-D15":
		d.LegendaryModifiers.AttackSlowReductionBP += 4000
	case "TY-D20":
		if d.ID == domain.StatusPoison || d.ID == 1036 {
			d.LegendaryModifiers.ControlDurationReductionBP += 4000
		}
	}
	return true
}
