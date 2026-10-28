package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

func (s *Scene) legendaryCasterHealed(src, dst *entity.Entity, def domain.SkillDef, beforeHP, effective int32) {
	if effective <= 0 {
		return
	}
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].Healed; h != nil {
			c.invoke(func(c *legendaryContext) { h(c, beforeHP, effective) })
		}
	}
}
func (s *Scene) legendaryCasterManaRestored(src, dst *entity.Entity, def domain.SkillDef, effective, overflow int32) {
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].ManaRestored; h != nil {
			c.invoke(func(c *legendaryContext) { h(c, effective, overflow) })
		}
	}
}
func (s *Scene) legendaryCasterDispelled(src, dst *entity.Entity, def domain.SkillDef, removed []domain.Active) {
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].Dispelled; h != nil {
			c.invoke(func(c *legendaryContext) { h(c, removed) })
		}
	}
}
func (s *Scene) legendaryCasterStatusApplying(source domain.EntityID, target *entity.Entity, def *domain.StatusDef) bool {
	s.legendaryAssassinControlDuration(target, def)
	skill := def.SkillSnapshotID
	if skill == 0 {
		skill = domain.SkillID(def.SourceSkillID)
	}
	for _, c := range s.legendaryContextsFor(s.entities[source], target, combat.Blow{SkillID: int32(skill)}, event.DamageDealt{}, true) {
		if child := s.legendaryState().child; child != nil {
			c.IsChild = true
			c.Def = child.Def
		}
		if h := legendaryHandlers[c.Power.ID].StatusApplied; h != nil {
			h(&c, def)
		}
	}

	return true
}
func legendaryCasterSlow(d domain.StatusDef) bool {
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
func legendaryCasterRestoreMP(c *legendaryContext, target *entity.Entity, n int32) {
	if target == nil || !target.Alive() || n <= 0 {
		return
	}
	defer c.Scene.withCombatCausalPath(c.effectPath())()
	before := target.MP
	target.MP = min(target.MaxMP, clampI32(int64(target.MP)+int64(n)))
	c.Scene.legendaryCasterManaRestored(c.Owner, target, domain.SkillDef{ID: c.Skill}, target.MP-before, max(int32(0), n-(target.MP-before)))
	c.Scene.emitAttributesIfPlayer(target)
}

func init() {

	registerLegendarySynergyHandler("YS-U03", legendaryHandler{ManaRestored: func(c *legendaryContext, n, overflow int32) {
		if c.Skill != 10302 || overflow <= 0 {
			return
		}
		var allies []*entity.Entity
		for _, p := range c.Scene.players {
			if p != c.Target && p.Alive() && p.MaxMP > 0 && sameParty(c.Target, p) && sqDist(p.Pos, c.Target.Pos) <= 350*350 {
				allies = append(allies, p)
			}
		}
		sort.Slice(allies, func(i, j int) bool {
			a, b := int64(allies[i].MP)*int64(allies[j].MaxMP), int64(allies[j].MP)*int64(allies[i].MaxMP)
			if a != b {
				return a < b
			}
			return allies[i].ID < allies[j].ID
		})
		for _, p := range allies {
			if overflow <= 0 {
				break
			}
			amount := min(overflow, specialScale(p.MaxMP, c.RollValue("restore_cap_pct", 20)), max(int32(0), p.MaxMP-p.MP))
			legendaryCasterRestoreMP(c, p, amount)
			if amount > 0 {
				c.Visual("stage-01", p)
			}
			overflow -= amount
		}
	}})

}

func legendaryCasterSteal(c *legendaryContext, removed []domain.Active) {
	if c.Skill != 10433 {
		return
	}
	for _, a := range removed {
		if !specialDefensiveStatus(a.Def.ID) || a.Def.Permanent {
			continue
		}
		if a.Def.DurationSec <= 0 || !legendaryCasterOnce(c, domain.EntityID(a.Def.ID), "stolen-native") {
			continue
		}
		// Rebuild the defensive spell from its accepted full definition, never
		// transfer the victim's remaining shield balance or multiply one kind.
		a.Def.Permanent = false
		a.Def.SkillSnapshotID, a.Def.SkillSnapshotRank = 0, 0
		a.Def.SkillSnapshotRaw, a.Def.SkillSnapshotHard = nil, nil
		a.Def.LegendarySources = nil
		a.Def.SpecialWeaponSourceBase = 0
		a.Def.ChanceExplicit, a.Def.ChanceBP = true, 10000
		a.Def.LegendaryPermanentPower = ""
		a.Def.LegendaryPowerID = ""
		a.Def.LegendaryPowerID, a.Def.LegendarySourceUID = c.Power.ID, c.SourceUID
		c.Scene.applyStatusDef(c.Owner, a.Def, c.Owner.ID)
	}
}
func legendaryCasterRegen(c *legendaryContext, d *domain.StatusDef) {
	if c.Skill == 10303 {
		d.LegendaryModifiers.HealingTickBP += 5000
	}
}
