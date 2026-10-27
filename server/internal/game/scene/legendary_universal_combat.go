package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

func init() {
	registerLegendarySynergyHandler("TY-A01", legendaryHandler{TransformSkill: universalBasicFan, SelectTargets: universalBasicTargets, TargetDefinition: universalBasicSide})
	registerLegendarySynergyHandler("TY-A37", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if c.IsBasic || d.LegendaryBasic {
			d.LegendaryModifiers.DamageBP += c.RollValue("benefit", universalTuning.BasicDamageBP)
		}
	}})
	registerLegendarySynergyHandler("TY-A16", legendaryHandler{ShieldDamageBP: func(c *legendaryContext) int32 { return c.RollValue("benefit", universalTuning.ShieldDamageBP) }})
	for _, id := range []string{"TY-D01", "TY-U20"} {
		registerLegendarySynergyHandler(id, legendaryHandler{PassiveStats: universalEquipmentStats})
	}
}
func universalBasicFan(c *legendaryContext, d *domain.SkillDef) {
	if !(c.IsBasic || d.LegendaryBasic) || c.IsChild || d.Area.Valid() {
		return
	}
	d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: max(int32(1), d.Dist), Angle: 120}
	d.TargetEnemy = true
	d.Kind = domain.SkillArea
}
func universalBasicTargets(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if !c.Def.LegendaryBasic || c.IsChild {
		return ids
	}
	ids = append([]domain.EntityID(nil), ids...)
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := c.Scene.entities[ids[i]], c.Scene.entities[ids[j]]
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		dx, dy := int64(a.Pos.X-c.Owner.Pos.X), int64(a.Pos.Y-c.Owner.Pos.Y)
		ex, ey := int64(b.Pos.X-c.Owner.Pos.X), int64(b.Pos.Y-c.Owner.Pos.Y)
		return dx*dx+dy*dy < ex*ex+ey*ey
	})
	out := make([]domain.EntityID, 0, len(ids))
	if c.Primary != 0 {
		out = append(out, c.Primary)
	}
	for _, id := range ids {
		if id != c.Primary {
			out = append(out, id)
		}
	}
	return out
}
func universalBasicSide(c *legendaryContext, d *domain.SkillDef) {
	if !d.LegendaryBasic || c.IsChild || c.Target == nil || c.Target.ID == c.Primary {
		return
	}
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		e := &d.Effects[i]
		if e.Kind == domain.EffectDamage {
			domain.ScaleSkillDamageFraction(e, int64(c.RollValue("benefit", 70)), 100)
			e.DamageFlat = universalPct(e.DamageFlat, int64(c.RollValue("benefit", 70)))
		}
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
