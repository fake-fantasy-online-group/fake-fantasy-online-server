package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func warriorCircle(c *legendaryContext, ids []domain.EntityID, center domain.Pos, r float64, cap int) []domain.EntityID {
	out := []domain.EntityID{}
	if c.Primary != 0 {
		out = append(out, c.Primary)
	}
	for _, e := range c.Enemies(center, r, cap) {
		out = legendaryMeleeAppendUnique(out, e.ID)
		if len(out) >= cap {
			break
		}
	}
	return out
}
func warriorEcho(c *legendaryContext, pct int64, farthest bool) {
	var best *equipmentHit
	var dist float64
	for i := range c.Hits {
		h := &c.Hits[i]
		if h.src != c.Owner.ID || h.blow.ProcDepth != 0 || h.blow.Origin != domain.DamageNormalOrigin || h.ev.Flag.Has(event.DamageMiss) {
			continue
		}
		e := c.Scene.entities[h.dst]
		if e == nil {
			continue
		}
		d := sqDist(c.CastPosition, e.Pos)
		if best == nil || (!farthest && h.dst == c.Primary) || (farthest && d > dist) || (!farthest && best.dst != c.Primary && d < dist) {
			best = h
			dist = d
		}
	}
	if best != nil && (farthest || c.Primary == 0 || best.dst == c.Primary) {
		c.Damage(c.Scene.entities[best.dst], legendaryMeleeScale(best.ev.Amount, pct), best.blow.Magic, true)
	}
}
func init() {
	registerWarriorPower("ZS-A02", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10002 {
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: int32(1.3 * legendaryMeleeReach(c))}
		}
	}, PrepareSkill: warriorSkill(10002, 15, 0, 0, 0).PrepareSkill, SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10002 {
			return warriorCircle(c, ids, c.Owner.Pos, float64(c.Def.Area.Radius), 5)
		}
		return ids
	}, TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10002 && c.Target != nil && c.Target.ID != c.Primary {
			legendaryMeleeStripControl(c, d)
		}
	}})
	registerWarriorPower("ZS-A06", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill == 10005 {
			warriorEcho(c, 110, false)
		}
	}})
	registerWarriorPower("ZS-A12", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10017 {
			d.LegendaryRequiresPrimaryLanded = true
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: int32(1.5 * legendaryMeleeReach(c))}
		}
	}, PrepareSkill: warriorSkill(10017, 10, 0, 0, 0).PrepareSkill, SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10017 {
			center := c.Center
			if t := c.Scene.entities[c.Primary]; t != nil {
				center = t.Pos
			}
			return warriorCircle(c, ids, center, float64(c.Def.Area.Radius), 6)
		}
		return ids
	}})
	registerWarriorPower("ZS-A16", legendaryHandler{SelectTargets: warriorPreservePrimary, PrepareSkill: warriorSkill(10003, 15, 0, 0, 0).PrepareSkill, TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10003 {
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: 180, Width: 120}
		}
	}})
	registerWarriorPower("ZS-A18", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill == 10021 || c.Skill == 10015 {
			warriorEcho(c, 90, true)
		}
	}})
	registerWarriorPower("ZS-A22", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10022 {
			d.Dist = int32(3 * legendaryMeleeReach(c))
			d.LegendaryModifiers.RadiusBP += 1500
			d.LegendaryModifiers.DamageBP += 2000
			if c.Target != nil {
				d.Area.Center = domain.SkillAreaCenterTarget
			}
		}
	}})
	registerWarriorPower("ZS-A23", legendaryHandler{PrepareSkill: warriorSkill(10022, 80, 0, 0, 0).PrepareSkill, RootFinished: func(c *legendaryContext) {
		if c.Skill != 10022 {
			return
		}
		var chosen domain.EntityID
		for _, h := range c.Hits {
			if h.ev.Flag.Has(event.DamageMiss) || h.blow.ProcDepth != 0 {
				continue
			}
			t := c.Scene.entities[h.dst]
			if t == nil || !t.Alive() {
				continue
			}
			if h.dst == c.Primary {
				chosen = h.dst
				break
			}
			prev := c.Scene.entities[chosen]
			if prev == nil || int64(t.HP)*int64(prev.MaxHP) > int64(prev.HP)*int64(t.MaxHP) {
				chosen = h.dst
			}
		}
		if c.Primary != 0 && chosen != c.Primary {
			return
		}
		if chosen != 0 {
			c.Damage(c.Scene.entities[chosen], legendaryMeleeScale(c.Attack(false), 160), false, false)
		}
	}})
	registerWarriorPower("ZS-A24", legendaryHandler{SelectTargets: warriorPreservePrimary, PrepareSkill: warriorSkill(10023, 85, 0, 0, 0).PrepareSkill, TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10023 {
			if w, ok := warriorLearned(c, 10022, true); ok {
				d.Kind = w.Kind
				d.Area = w.Area
				d.Radius = w.Radius
				d.LegendaryModifiers.GeometryInherited = true
			}
		}
	}})
	registerWarriorPower("ZS-A27", legendaryHandler{SelectTargets: warriorPreservePrimary, PrepareSkill: warriorDamageSkills(80, 10027, 10009).PrepareSkill, TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10027 {
			if b, ok := warriorLearned(c, 10009, true); ok {
				d.Kind = b.Kind
				d.Area = b.Area
				d.Radius = b.Radius
				d.LegendaryModifiers.GeometryInherited = true
			}
		}
	}})
}

func warriorPreservePrimary(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	skill := map[string]domain.SkillID{"ZS-A08": 10009, "ZS-A16": 10003, "ZS-A20": 10015, "ZS-A24": 10023, "ZS-A27": 10027}[c.Power.ID]
	if c.Skill == skill && c.Primary != 0 {
		return legendaryMeleeAppendUnique(ids, c.Primary)
	}
	return ids
}
