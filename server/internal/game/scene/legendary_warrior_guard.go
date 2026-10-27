package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

func warriorHas(c *legendaryContext, id string) bool {
	found := false
	c.Scene.forEachLegendary(c.Owner, func(x *legendaryContext) { found = found || x.Power.ID == id })
	return found
}
func warriorWarcryRadius(c *legendaryContext, d *domain.SkillDef) {
	if d.ID != 10028 {
		return
	}
	for _, id := range []string{"ZS-A28", "ZS-U04", "ZS-U08", "ZS-U10"} {
		if warriorHas(c, id) {
			if c.Power.ID == id {
				pct := int32(7000)
				if id == "ZS-U10" {
					pct = 2500
				}
				d.LegendaryModifiers.RadiusBP += pct
			}
			return
		}
	}
}
func warriorShield(c *legendaryContext) {
	p := c.Owner
	id := legendaryMeleeStatusID(c, "permanent-shield")
	if p.Status != nil {
		c.Scene.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == id }))
	}
	if c.Scene.applyStatusDef(p, domain.StatusDef{ID: id, Level: 1, Permanent: true, DamageShield: legendaryMeleeScale(p.MaxHP, int64(c.RollValue("shield_pct", 25))), LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, Name: c.Power.Name, Desc: c.Power.Effect, Icon: legendaryMeleeIcon(c), IconKnown: true, TypeKnown: true, Type: domain.StatusGood}, p.ID) {
		c.Visual("stage-01", p)
	}
}
func warriorPull(c *legendaryContext, t *entity.Entity) {
	if t == nil || t.Monster == nil || !t.Alive() || c.Scene.legendaryMeleeDisplacementImmune(t) {
		return
	}
	if t.Status != nil && t.Status.SuppressesHarmfulAt(c.Scene.tick) {
		return
	}
	if c.Scene.defs != nil {
		if d, ok := c.Scene.defs.Def(t.Monster.TypeID); ok {
			if _, immune := d.ImmuneSkills[10028]; immune {
				return
			}
		}
	}
	from := t.Pos
	dx, dy := c.Owner.Pos.X-from.X, c.Owner.Pos.Y-from.Y
	length := math.Hypot(dx, dy)
	if length <= 30 {
		return
	}
	to := from
	for distance := 5.0; distance <= length-30; distance += 5 {
		next := domain.Pos{MapID: from.MapID, X: from.X + dx*distance/length, Y: from.Y + dy*distance/length}
		if !c.Scene.straightPathWalkable(to, next) {
			break
		}
		blocked := false
		for _, other := range c.Scene.entities {
			if other.ID != t.ID && other.Alive() && sqDist(next, other.Pos) < 20*20 {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		to = next
	}
	if to == from {
		return
	}
	c.Scene.aoi.Move(t, to)
	c.Scene.legendaryMeleeMoved(t, from, to, true)
	c.Scene.emit(event.EntityMoved{ID: t.ID, To: to, Dir: t.Dir, Snap: true})
	t.Monster.CombatPathActive = false
	t.Monster.Roaming = false
}
func warriorSlow(d *domain.StatusDef) bool {
	if d.ID == domain.StatusSlow {
		return true
	}
	for _, a := range d.Affixes {
		if a.Attr == domain.AttrMoveSpeed && a.Value < 0 {
			return true
		}
	}
	return false
}
func init() {
	registerWarriorPower("ZS-A28", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		warriorWarcryRadius(c, d)
		if legendaryMeleeSpearActive(*d) && d.HasDamageEffect() {
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 4000)
		}
		if d.ID == 10028 {
			warriorEffects(d, func(e *domain.SkillEffect) {
				if e.Kind == domain.EffectTaunt {
					e.TauntPower *= 2
				}
			})
		}
	}, CastReleased: func(c *legendaryContext) {
		if c.Skill == 10028 {
			for _, id := range c.Targets {
				warriorPull(c, c.Scene.entities[id])
			}
		}
	}})
	registerWarriorPower("ZS-D01", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill == 10028 {
			warriorShield(c)
		}
	}})
	registerWarriorPower("ZS-D03", legendaryHandler{ModifyIncoming: func(c *legendaryContext, n int32) int32 {
		if !warriorDirectIncoming(c) || c.Target.Status == nil {
			return n
		}
		broken := false
		c.Target.Status.EachEffective(func(a domain.Active) {
			if a.Source != c.Owner.ID || a.Def.SkillSnapshotID != 10009 && a.Def.SourceSkillID != 10009 {
				return
			}
			// The passive vulnerability shares the casting skill snapshot, but
			// does not lower defense and may land when native armor is resisted.
			for _, affix := range a.Def.Affixes {
				if affix.Attr == domain.AttrDef && affix.Value < 0 {
					broken = true
				}
			}
		})
		if broken {
			return legendaryMeleeScale(n, 100-int64(c.RollValue("reduction_pct", 50)))
		}
		return n
	}})
	registerWarriorPower("ZS-D09", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10016 && !warriorHas(c, "ZS-A26") {
			d.LegendaryModifiers.CooldownReductionBP += 5000
		}
	}, CastReleased: func(c *legendaryContext) {
		if c.Skill == 10016 {
			warriorShield(c)
		}
	}})
	h := warriorIncoming(25)
	h.EquipmentChanged = legendaryMeleeRemoveSlow
	h.StatusReceiving = func(c *legendaryContext, d *domain.StatusDef) bool { return !warriorSlow(d) }
	registerWarriorPower("ZS-D10", h)
	registerWarriorPower("ZS-U03", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10017 {
			d.Dist = specialScale(d.Dist, 150)
		}
	}, CastReleased: warriorFunctionalRelease(10017, 8000, true), PassiveStats: warriorFunctionalMove(10017, 80), Tick: warriorFunctionalTick(10017, true)})
	registerWarriorPower("ZS-U04", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		warriorWarcryRadius(c, d)
		if d.ID == 10028 {
			d.LegendaryModifiers.ManaReductionBP += 3000
		}
	}, CastReleased: warriorFunctionalRelease(10028, 12000, true), PassiveStats: warriorFunctionalMove(10028, 60), Tick: warriorFunctionalTick(10028, true)})
	registerWarriorPower("ZS-U08", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		warriorWarcryRadius(c, d)
		if d.ID == 10005 {
			d.LegendaryModifiers.WidthBP += 7000
			d.LegendaryModifiers.LengthBP += 7000
		}
		if d.ID == 10005 || d.ID == 10028 {
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 3000)
		}
	}})
	registerWarriorPower("ZS-U10", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		warriorWarcryRadius(c, d)
		if d.ID == 10028 {
			d.MPCost = 0
			d.ManaCostFractionNumerator = 0
			d.ManaCostFractionDenominator = 1
			d.RoundingPreviewReady = false
		}
		if d.ID == 10023 {
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 6000)
		}
	}})
}
