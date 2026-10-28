package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func init() {
	for _, id := range []string{"TY-A10", "TY-D02", "TY-D03", "TY-D04", "TY-U09"} {
		registerLegendarySynergyHandler(id, legendaryHandler{PassiveStats: universalExtendedStats})
	}
	for _, id := range []string{"TY-A25", "TY-A26", "TY-A38", "TY-A39"} {
		registerLegendarySynergyHandler(id, legendaryHandler{BaseSkill: universalExtendedBase})
	}
	for _, id := range []string{"TY-A30", "TY-A31", "TY-A32", "TY-A36"} {
		registerLegendarySynergyHandler(id, legendaryHandler{TargetDefinition: universalExtendedTarget})
	}
	for _, id := range []string{"TY-D06", "TY-D07", "TY-D08", "TY-D10", "TY-D17"} {
		registerLegendarySynergyHandler(id, legendaryHandler{ModifyIncoming: universalExtendedIncoming})
	}
}
func universalEquipmentContribution(c *legendaryContext) (domain.Stats, domain.Stats) {
	if c.SourceSnapshot != nil && c.SourceSnapshot.ChildPreparation != nil {
		p := c.SourceSnapshot.ChildPreparation
		return p.Equipped, p.Bare
	}
	if c.Owner == nil || c.Owner.Player == nil || c.Owner.Player.Char == nil {
		return domain.Stats{}, domain.Stats{}
	}
	ch := c.Owner.Player.Char
	src := domain.StatSource{Base: ch.EffectiveBase(), Level: ch.Level, Innate: characterInnateStats(ch), Worn: c.Scene.functionalWorn(c.Owner), Defs: c.Scene.itemDef}
	_, eq := domain.Compute(src)
	src.Worn = nil
	_, bare := domain.Compute(src)
	return eq, bare
}
func universalExtendedStats(c *legendaryContext, st *domain.Stats) {
	eq, bare := universalEquipmentContribution(c)
	switch c.Power.ID {
	case "TY-A10":
		ms := st.AtkSpeedMS
		if ms <= 0 {
			ms = 1500
		}
		st.AtkSpeedMS = max(int32(100), universalScaleBP(ms, -c.RollValue("benefit", 2000)))
		st.AtkSpeedPct = domain.SaturateInt32((100+int64(st.AtkSpeedPct))*10000/int64(10000-c.RollValue("benefit", 2000)) - 100)
	case "TY-D02":
		st.MDef = domain.SaturateInt32(int64(st.MDef) + int64(universalPct(max(int32(0), eq.MDef-bare.MDef), int64(c.RollValue("benefit", 40)))))
	case "TY-D03":
		st.MaxHP = domain.SaturateInt32(int64(st.MaxHP) + int64(universalPct(max(int32(0), eq.MaxHP-bare.MaxHP), int64(c.RollValue("benefit", 30)))))
	case "TY-D04":
		st.MaxMP = domain.SaturateInt32(int64(st.MaxMP) + int64(universalPct(max(int32(0), eq.MaxMP-bare.MaxMP), int64(c.RollValue("benefit", 40)))))
	case "TY-U09":
		st.MoveSpeed = domain.SaturateInt32(int64(st.MoveSpeed) + int64(universalPct(max(int32(0), bare.MoveSpeed), int64(c.RollValue("benefit", 20)))))
	}
}
func universalExtendedBase(c *legendaryContext, d *domain.SkillDef) {
	if !d.HasDamageEffect() {
		return
	}
	eq, bare := universalEquipmentContribution(c)
	var base int32
	switch c.Power.ID {
	case "TY-A25":
		if !d.LegendaryBasic && (d.HurtType == domain.HurtPhysical || d.HurtType == domain.HurtNone) {
			base = universalPct(max(int32(0), eq.MaxAtk-bare.MaxAtk), int64(c.RollValue("benefit", 50)))
		}
	case "TY-A26":
		if !d.LegendaryBasic && d.HurtType != domain.HurtPhysical && d.HurtType != domain.HurtNone {
			base = universalPct(max(int32(0), eq.MAtk-bare.MAtk), int64(c.RollValue("benefit", 50)))
		}
	case "TY-A38":
		if !d.LegendaryBasic {
			maximum := c.Owner.MaxMP
			if c.SourceSnapshot != nil && c.SourceSnapshot.ChildPreparation != nil {
				maximum = c.SourceSnapshot.ChildPreparation.MaximumMP
			}
			base = universalPct(maximum, int64(c.RollValue("benefit", 4)))
		}
	case "TY-A39":
		if d.LegendaryBasic {
			contribution := max(int32(0), eq.Def-bare.Def)
			c.Scene.forEachLegendary(c.Owner, func(other *legendaryContext) {
				if other.Power.ID == "TY-D01" {
					contribution = universalScaleBP(contribution, other.RollValue("benefit", universalTuning.EquipmentDefenseBP))
				}
			})
			base = universalPct(contribution, int64(c.RollValue("benefit", 20)))
		}
	}
	if base > 0 {
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectDamage {
				if c.Power.ID == "TY-A25" || c.Power.ID == "TY-A26" {
					e.AttackBaseFlat = domain.SaturateInt32(int64(e.AttackBaseFlat) + int64(base))
				} else {
					e.BaseDamageFlat = domain.SaturateInt32(int64(e.BaseDamageFlat) + int64(base))
				}
			}
		})
	}
}
func universalNativeControlled(e *entity.Entity) bool {
	found := false
	if e != nil && e.Status != nil {
		e.Status.EachEffective(func(a domain.Active) {
			if a.Def.LegendaryPowerID == "" && (a.Def.Control != domain.ControlNone || a.Def.BlockMove) {
				found = true
			}
		})
	}
	return found
}
func universalOwnedNativeStatus(c *legendaryContext, id domain.StatusID) bool {
	found := false
	if c.Target != nil && c.Target.Status != nil {
		c.Target.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == id && a.Source == c.Owner.ID && a.Def.LegendaryPowerID == "" {
				found = true
			}
		})
	}
	return found
}
func universalExtendedTarget(c *legendaryContext, d *domain.SkillDef) {
	if c.Target == nil || !d.HasDamageEffect() {
		return
	}
	var bp int32
	switch c.Power.ID {
	case "TY-A30":
		if universalNativeControlled(c.Target) {
			bp = c.RollValue("benefit", 5000)
		}
	case "TY-A31":
		if universalOwnedNativeStatus(c, domain.StatusPoison) {
			bp = c.RollValue("benefit", 4000)
		}
	case "TY-A32":
		if universalOwnedNativeStatus(c, 1036) {
			bp = c.RollValue("benefit", 4000)
		}
	case "TY-A36":
		if universalArea(d) && c.Primary != 0 && c.Target.ID != c.Primary {
			bp = c.RollValue("benefit", 4000)
		}
	}
	legendaryAddTargetDamage(d, bp)
}
func universalExtendedIncoming(c *legendaryContext, n int32) int32 {
	if c.Owner == nil || c.Target == nil || c.Owner.ID == c.Target.ID || sameParty(c.Owner, c.Target) {
		return n
	}
	var bp int32
	switch c.Power.ID {
	case "TY-D06":
		if c.IsBasic {
			bp = c.RollValue("benefit", 2500)
		}
	case "TY-D07":
		if !c.Blow.Magic && !c.Hit.Flag.Has(event.DamageMagic) {
			bp = c.RollValue("benefit", 2000)
		}
	case "TY-D08":
		if c.Blow.Magic || c.Hit.Flag.Has(event.DamageMagic) {
			bp = c.RollValue("benefit", 2000)
		}
	case "TY-D10":
		if c.Hit.Flag.Has(event.DamageCrit) {
			bp = c.RollValue("benefit", 3000)
		}
	case "TY-D17":
		if universalNativeControlled(c.Owner) {
			bp = c.RollValue("benefit", 2500)
		}
	}
	return universalScaleBP(n, -bp)
}
