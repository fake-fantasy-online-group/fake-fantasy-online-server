package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

// Revised powers modify one native event. No old marks, movement ledgers,
// delayed detonations, independent proc damage, or independent ICDs remain.
func init() {
	for _, id := range []string{"CK-A01", "CK-A23", "CK-A02", "CK-A27", "CK-A03", "CK-A04", "CK-A05", "CK-A07", "CK-A14", "CK-A16", "CK-A17", "CK-A24", "CK-A06", "CK-A11", "CK-A12", "CK-A19", "CK-A20", "CK-A21", "CK-A29", "CK-U05"} {
		registerLegendarySynergyHandler(id, legendaryHandler{TransformSkill: legendaryAssassinTransform, PrepareSkill: legendaryAssassinPrepare, SelectTargets: legendaryAssassinTargets, TargetDefinition: legendaryAssassinTargetDefinition})
	}
	registerLegendarySynergyHandler("CK-A09", legendaryHandler{StatusApplied: legendaryAssassinPoisonStatus})
	registerLegendarySynergyHandler("CK-A10", legendaryHandler{AfterHit: legendaryAssassinNativePoisonHit})
	registerLegendarySynergyHandler("CK-A22", legendaryHandler{AfterHit: legendaryAssassinLifesteal})
	registerLegendarySynergyHandler("CK-A13", legendaryHandler{TrapPlaced: legendaryAssassinTriplePlacement})
	registerLegendarySynergyHandler("CK-U06", legendaryHandler{RootFinished: legendaryAssassinBackstabCooldown})
}
func legendaryAssassinTransform(c *legendaryContext, d *domain.SkillDef) {
	switch c.Power.ID {
	case "CK-A01":
		if d.ID == 10221 {
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: max(d.Dist, int32(c.Owner.Look.AtkDist)), Width: max(int32(40), int32(c.Owner.Look.AtkDist)/2)}
		}
	case "CK-A05":
		if d.ID == 10205 {
			// Establish the new splash before final geometry modifiers run.
			d.Radius = max(int32(100), d.Radius)
		}
	case "CK-A06":
		if d.ID == 10206 {
			d.Kind = domain.SkillArea
			d.Area.Shape = domain.SkillAreaCircle
			d.Area.Center = domain.SkillAreaCenterCaster
			d.Area.Radius = max(d.Area.Radius, d.Radius)
		}
	case "CK-A20":
		if d.ID == 10209 {
			d.RequiresInvisible = false
		}
	}
}
func legendaryAssassinPrepare(c *legendaryContext, d *domain.SkillDef) {
	switch c.Power.ID {
	case "CK-A04":
		if d.ID == 10205 {
			d.LegendaryModifiers.DamageBP += 5000
		}
	case "CK-A14":
		if d.ID == 10214 {
			d.LegendaryModifiers.RadiusBP += 3000
		}
	case "CK-A16":
		if d.ID == 10215 {
			d.LegendaryModifiers.RadiusBP += 3000
			d.LegendaryModifiers.DamageBP += 5000
		}
	case "CK-A17":
		if d.ID == 10216 {
			d.LegendaryModifiers.RadiusBP += 3000
		}
		if d.ID == 10216 || d.ID == 10209 {
			d.LegendaryModifiers.DamageBP += 5000
		}
	case "CK-A24":
		if d.ID == 10203 {
			d.LegendaryModifiers.DamageBP += 5000
		}
	case "CK-A23":
		if d.ID == 10219 {
			d.LegendaryDamageThroughControlImmunity = true
			d.LegendaryModifiers.DamageBP += 3500
			d.LegendaryModifiers.CooldownReductionBP += 4000
		}
	case "CK-A02":
		if d.ID == 10221 {
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			for i := range d.Effects {
				if d.Effects[i].Kind == domain.EffectDamage {
					d.Effects[i].GuaranteedHit = true
					d.Effects[i].GuaranteedCrit = true
				}
			}
		}
	case "CK-A27":
		if d.ID == 10205 || d.ID == 10206 {
			d.Dist = specialScale(d.Dist, 150)
			d.LegendaryModifiers.RadiusBP += 5000
			d.LegendaryModifiers.WidthBP += 5000
		}
	case "CK-A01":
		if d.ID == 10221 {
			d.LegendaryModifiers.CooldownReductionBP += 3000
		}
	case "CK-A03":
		if d.ID == 10221 {
			d.LegendaryModifiers.DamageBP += 8000
		}
	case "CK-A06":
		if d.ID == 10206 {
			d.LegendaryModifiers.RadiusBP += 2000
			d.LegendaryModifiers.DamageBP += 5000
		}
	case "CK-A11":
		if d.ID == 10213 {
			d.Dist = specialScale(d.Dist, 150)
			d.LegendaryModifiers.RadiusBP += 3000
			d.LegendaryModifiers.DamageBP += 4000
		}
	case "CK-A12":
		if d.ID == 10213 {
			d.LegendaryModifiers.RadiusBP += 3000
			d.LegendaryModifiers.DamageBP += 9000
		}
	case "CK-A20":
		if d.ID == 10209 {
			d.LegendaryModifiers.DamageBP += 2500
		}
	case "CK-A29":
		if d.ID == 10210 {
			d.LegendaryModifiers.DamageBP += 8000
		}
	case "CK-U05":
		if c.Owner.Player.Char.Skills[10222] > 0 && (d.ID == 10221 || d.ID == 10205 || d.ID == 10206) {
			d.LegendaryModifiers.ManaReductionBP += 4500
		}
	}
}
func legendaryAssassinTrapRadius(d *domain.SkillDef, pct int32) {
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		if d.Effects[i].Kind == domain.EffectPlaceTrap {
			d.Effects[i].TrapRadius = specialScale(d.Effects[i].TrapRadius, pct)
		}
	}
	if len(d.Effects) > 0 {
		d.Effect = d.Effects[0]
	}
}

// A19 keeps the legal selected target even if it lies on the cone edge. The
// engineering default cone is 120 degrees, using native melee cast range.
func legendaryAssassinTargets(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if c.Power.ID == "CK-A05" && c.Skill == 10205 {
		primary := c.Scene.entities[c.Primary]
		if primary == nil || !c.Scene.validTarget(c.Owner, primary, c.Def) {
			return ids
		}
		out := []domain.EntityID{primary.ID}
		radius := float64(max(int32(100), c.Def.Radius))
		for _, target := range c.Enemies(primary.Pos, radius, 100000) {
			if target.ID != primary.ID && c.Scene.validTarget(c.Owner, target, c.Def) {
				out = append(out, target.ID)
				if len(out) == 4 {
					break
				}
			}
		}
		return out
	}
	if c.Power.ID != "CK-A19" || c.Skill != 10209 {
		return ids
	}
	primary := c.Scene.entities[c.Primary]
	if primary == nil || !c.Scene.validTarget(c.Owner, primary, c.Def) {
		return ids
	}
	out := []domain.EntityID{primary.ID}
	radius := float64(max(c.Def.Dist, int32(c.Owner.Look.AtkDist)))
	if radius <= 0 {
		radius = 75
	}
	dx, dy := primary.Pos.X-c.Owner.Pos.X, primary.Pos.Y-c.Owner.Pos.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		dx, dy, length = 1, 0, 1
	}
	for _, target := range c.Enemies(c.Owner.Pos, radius, 100000) {
		if target.ID == primary.ID {
			continue
		}
		tx, ty := target.Pos.X-c.Owner.Pos.X, target.Pos.Y-c.Owner.Pos.Y
		dist := math.Hypot(tx, ty)
		if dist > 0 && (dx*tx+dy*ty)/(length*dist) < .5 {
			continue
		}
		if !c.Scene.validTarget(c.Owner, target, c.Def) {
			continue
		}
		out = append(out, target.ID)
		if len(out) == 5 {
			break
		}
	}
	return out
}
func legendaryAssassinTargetDefinition(c *legendaryContext, d *domain.SkillDef) {
	if c.Power.ID == "CK-A23" && d.ID == 10219 && legendaryAssassinStunImmune(c) {
		d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
		for i := range d.Effects {
			if d.Effects[i].Kind == domain.EffectDamage {
				d.Effects[i].DamagePct += 160
				if d.Effects[i].DamagePctDenominator > 0 {
					d.Effects[i].DamagePctNumerator += 160 * d.Effects[i].DamagePctDenominator
				}
			}
		}
	}
	if c.Power.ID == "CK-A07" && d.ID == 10206 && c.Target != nil && c.Target.ID == c.Primary {
		legendaryAddTargetDamage(d, 5000)
	}
	if c.Power.ID == "CK-A21" && d.ID == 10209 && c.Target != nil && c.Target.ID == c.Primary {
		legendaryAddTargetDamage(d, 7500)
	}
}
func legendaryAssassinLifesteal(c *legendaryContext) {
	if c.Skill != 10209 || c.Hit.Flag.Has(event.DamageMiss) || c.LostHP <= 0 {
		return
	}
	// Effective loss, not overkill or shield absorption, is the native damage base.
	amount := clampI32(int64(c.LostHP) * 15 / 100)
	if amount <= 0 {
		return
	}
	c.Scene.emit(combat.Restore(c.Owner, c.Owner, amount, 10209))
	c.Scene.emitAttributesIfPlayer(c.Owner)
}
func legendaryAssassinBackstabCooldown(c *legendaryContext) {
	if c.Skill != 10209 || c.Owner.Player == nil {
		return
	}
	landed, killed := false, false
	for _, h := range c.Hits {
		if h.src == c.Owner.ID && h.blow.SkillID == 10209 && !h.ev.Flag.Has(event.DamageMiss) {
			landed = true
			killed = killed || h.ev.Flag.Has(event.DamageFatal)
		}
	}
	if landed {
		pct := uint64(65)
		if killed {
			pct = 20
		}
		c.Owner.Player.ScaleRemainingSkillCooldown(10208, c.Scene.tick, pct)
	}
}
func legendaryAssassinTrapSkill(id domain.SkillID) bool { return id >= 10213 && id <= 10217 }
func legendaryAssassinHas(s *Scene, p *entity.Entity, id string, accepted ...*domain.CombatSourceSnapshotV5) bool {
	var source *domain.CombatSourceSnapshotV5
	if len(accepted) > 0 {
		source = accepted[0]
	}
	for _, c := range s.legendaryContexts(p, nil, combat.Blow{SourceOffense: source}, event.DamageDealt{}) {
		if c.Power.ID == id {
			return true
		}
	}
	return false
}

func legendaryAssassinStunImmune(c *legendaryContext) bool {
	if c.Target == nil {
		return false
	}
	if c.Target.Status != nil && (c.Target.Status.SuppressesHarmfulAt(c.Scene.tick) || c.Target.Status.BlocksStatus(domain.StatusDef{ID: domain.StatusStun, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun})) {
		return true
	}
	if c.Target.Monster != nil && c.Scene.defs != nil {
		if d, ok := c.Scene.defs.Def(c.Target.Monster.TypeID); ok {
			_, immune := d.ImmuneSkills[10219]
			return immune
		}
	}
	return false
}
