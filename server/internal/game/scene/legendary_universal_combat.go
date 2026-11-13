package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func init() {
	registerLegendarySynergyHandler("TY-A01", legendaryHandler{AfterHit: synergyCashout})
	registerLegendarySynergyHandler("TY-A37", legendaryHandler{AfterHit: synergySkillProc})
	registerLegendarySynergyHandler("TY-A39", legendaryHandler{AfterHit: synergySkillShield})
	registerLegendarySynergyHandler("TY-A10", legendaryHandler{PeriodicResolved: synergySpread})
	registerLegendarySynergyHandler("TY-A16", legendaryHandler{ShieldDamageBP: func(c *legendaryContext) int32 { return c.RollValue("benefit", universalTuning.ShieldDamageBP) }})
	for _, id := range []string{"TY-D01", "TY-U20"} {
		registerLegendarySynergyHandler(id, legendaryHandler{PassiveStats: universalEquipmentStats})
	}
}
func universalEquipmentStats(c *legendaryContext, stats *domain.Stats) {
	if c.Owner == nil || c.Owner.Player == nil || c.Owner.Player.Char == nil {
		return
	}
	ch := c.Owner.Player.Char
	src := domain.StatSource{Base: ch.EffectiveBase(), Level: ch.Level, Innate: characterInnateStats(ch), Worn: c.Scene.functionalWorn(c.Owner), Defs: c.Scene.itemDef}
	_, equipped := domain.Compute(src)
	src.Worn = nil
	_, bare := domain.Compute(src)
	switch c.Power.ID {
	case "TY-D01":
		stats.Def = domain.SaturateInt32(int64(stats.Def) + int64(max(int32(0), equipped.Def-bare.Def))*int64(c.RollValue("benefit", universalTuning.EquipmentDefenseBP))/10000)
	case "TY-U20":
		stats.MoveSpeed = domain.SaturateInt32(int64(stats.MoveSpeed) + int64(max(int32(0), equipped.MoveSpeed-bare.MoveSpeed))*int64(c.RollValue("benefit", universalTuning.EquipmentMoveBP))/10000)
	}
}

// Shield efficiency consumes amplified shield units, but converts only the
// absorbed portion back to base damage units. HP spillover is never amplified.
func (s *Scene) legendaryAbsorbDamage(src, dst *entity.Entity, blow combat.Blow, n int32) (int32, []domain.StatusID) {
	if dst == nil || dst.Status == nil || n <= 0 {
		return n, nil
	}
	bp := int32(0)
	if src != nil && src != dst && !sameParty(src, dst) && blow.Origin != domain.DamageDOTOrigin {
		for _, c := range s.legendaryContextsFor(src, dst, blow, event.DamageDealt{}, blow.LegendaryChild) {
			if h := legendaryHandlers[c.Power.ID].ShieldDamageBP; h != nil {
				bp += h(&c)
			}
		}
	}
	if bp <= 0 {
		return dst.Status.AbsorbDamage(n)
	}
	scaled := universalScaleBP(n, bp)
	remaining, removed := dst.Status.AbsorbDamage(scaled)
	absorbed := scaled - remaining
	used := domain.SaturateInt32((int64(absorbed)*10000 + int64(9999+bp)) / int64(10000+bp))
	return max(int32(0), n-used), removed
}
