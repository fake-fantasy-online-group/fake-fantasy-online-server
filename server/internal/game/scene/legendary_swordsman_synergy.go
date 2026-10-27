package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

// These handlers implement the approved event-local sword builds. No equipment
// timer, next-hit marker, skill history or independent cooldown is introduced.
func init() {
	registerLegendarySynergyHandler("JK-A09", legendaryHandler{BeforeDamage: swordDualDamage, ImpactTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if swordDual(c.Skill) {
			return swordExpand(c, ids, 100, false)
		}
		return ids
	}})
	registerLegendarySynergyHandler("JK-A11", legendaryHandler{NativeRestoreAmount: func(c *legendaryContext, n int32, hits []equipmentHit) int32 {
		if !swordDual(c.Skill) {
			return n
		}
		return legendaryMeleeScale(n, 100+int64(c.RollValue("restore_pct", 50))*(1+int64(min(3, max(0, swordLandedCount(c.Owner.ID, c.Skill, hits)-1)))))
	}})
	registerLegendarySynergyHandler("JK-D03", legendaryHandler{NativeRestoreResolved: swordOverflowShield})
	registerLegendarySynergyHandler("JK-A05", legendaryHandler{PrepareSkill: swordCooldown(10111, 24000), BeforeDamage: legendaryMeleeSkillDamage(10111, 140), ImpactTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10111 {
			return swordExpand(c, ids, 150, false)
		}
		return ids
	}})
	registerLegendarySynergyHandler("JK-A13", legendaryHandler{PrepareSkill: swordCooldown(10113, 24000), BeforeDamage: legendaryMeleeSkillDamage(10113, 170), ImpactTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10113 {
			return swordExpand(c, ids, 100, false)
		}
		return ids
	}})
	registerLegendarySynergyHandler("JK-A24", legendaryHandler{PrepareSkill: swordCooldown(10126, 18000), BeforeDamage: legendaryMeleeSkillDamage(10126, 180), ImpactTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10126 && swordRooted(c.Target, 0) {
			return swordExpand(c, ids, 150, true)
		}
		return ids
	}, TargetDefinition: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Skill == 10126 && c.Target != nil && c.Primary != 0 && c.Target.ID != c.Primary {
			legendaryMeleeStripControl(c, d)
		}
	}})
	registerLegendarySynergyHandler("JK-D06", legendaryHandler{CastReleased: warriorFunctionalRelease(10111, 8000, false), Tick: warriorFunctionalTick(10111, false), BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) && warriorFunctionalActive(c, 10111) {
			return legendaryMeleeScale(n, 100-int64(c.RollValue("reduction_pct", 35)))
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-A16", legendaryHandler{BeforeDamage: legendaryMeleeSkillDamage(10121, 130), SelectTargets: swordGatherAdmission, ImpactTargets: swordGather})
	registerLegendarySynergyHandler("JK-A15", legendaryHandler{BeforeDamage: legendaryMeleeSkillDamage(10121, 160), AfterHit: swordRecordActualDamage, RootFinished: swordConcentrate})
	registerLegendarySynergyHandler("JK-D05", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) && !c.Blow.Magic && c.Owner.Pos.MapID == c.Target.Pos.MapID && sqDist(c.Owner.Pos, c.Target.Pos) <= 75*75 {
			return legendaryMeleeScale(n, 100-int64(c.RollValue("reduction_pct", 40)))
		}
		return n
	}})
}
func swordDual(id domain.SkillID) bool { return id == 10104 || id == 10119 }
func swordDualDamage(c *legendaryContext, n int32) int32 {
	if legendaryMeleeDirect(c) && swordDual(c.Skill) {
		return legendaryMeleeScale(n, 100+int64(c.RollValue("damage_pct", 15)))
	}
	return n
}
func swordCooldown(skill domain.SkillID, ms int32) func(*legendaryContext, *domain.SkillDef) {
	return func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == skill {
			d.CooldownMS = ms
		}
	}
}
func swordRooted(e *entity.Entity, source domain.EntityID) bool {
	if e == nil || e.Status == nil {
		return false
	}
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if (source == 0 || a.Source == source) && a.Def.BlockMove && a.Def.Control != domain.ControlStun && a.Def.ID != domain.StatusFreeze && a.Def.ID != domain.StatusPetrify {
			found = true
		}
	})
	return found
}
func swordControlledBy(e *entity.Entity, source domain.EntityID) bool {
	if swordRooted(e, source) {
		return true
	}
	if e == nil || e.Status == nil {
		return false
	}
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if a.Source == source && a.Def.Control == domain.ControlStun {
			found = true
		}
	})
	return found
}
func swordExpand(c *legendaryContext, ids []domain.EntityID, radius float64, roots bool) []domain.EntityID {
	if c.Target == nil || !c.Target.Alive() {
		return ids
	}
	out := []domain.EntityID{c.Target.ID}
	for _, e := range c.Enemies(c.Target.Pos, radius, len(c.Scene.entities)) {
		if e.ID == c.Target.ID || roots && !swordRooted(e, 0) {
			continue
		}
		out = append(out, e.ID)
	}
	return out
}
func swordLandedCount(owner domain.EntityID, skill domain.SkillID, hits []equipmentHit) int {
	seen := map[domain.EntityID]bool{}
	for _, h := range hits {
		if h.src == owner && h.blow.SkillID == int32(skill) && h.blow.Origin == domain.DamageNormalOrigin && h.blow.ProcDepth == 0 && !h.ev.Flag.Has(event.DamageMiss) {
			seen[h.dst] = true
		}
	}
	return len(seen)
}
func swordOverflowShield(c *legendaryContext, effective, overflow int32) {
	if !swordDual(c.Skill) || overflow <= 0 || !c.Owner.Alive() {
		return
	}
	amount := min(overflow, specialScale(c.Owner.MaxHP, c.RollValue("shield_pct", 16)))
	id := legendaryMeleeStatusID(c, "shield")
	if c.Owner.Status != nil {
		c.Owner.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == id {
				amount = max(amount, a.ShieldLeft)
			}
		})
		c.Scene.statusesRemoved(c.Owner, c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == id }))
	}
	c.Scene.applyStatusDef(c.Owner, domain.StatusDef{ID: id, Level: 1, Name: c.Power.Name, Desc: c.Power.Effect, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, TypeKnown: true, Type: domain.StatusGood, Permanent: true, DamageShield: amount, Icon: legendaryMeleeIcon(c), IconKnown: true}, c.Owner.ID)
}
func swordGather(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if c.Skill != 10121 {
		return ids
	}
	for _, e := range c.Enemies(c.Owner.Pos, 150, len(c.Scene.entities)) {
		if e.Monster == nil || e.Status != nil && e.Status.SuppressesHarmfulAt(c.Scene.tick) || c.Scene.legendaryMeleeDisplacementImmune(e) {
			continue
		}
		distance := math.Sqrt(sqDist(c.Owner.Pos, e.Pos))
		if distance <= 50 {
			continue
		}
		source := *c.Owner
		source.Pos = e.Pos
		source.Pos.X += (e.Pos.X - c.Owner.Pos.X) / distance
		source.Pos.Y += (e.Pos.Y - c.Owner.Pos.Y) / distance
		c.Scene.knockbackMonster(&source, e, int32(math.Ceil(distance-50)))
	}
	out := []domain.EntityID{}
	for _, e := range c.Enemies(c.Owner.Pos, float64(max(int32(100), max(c.Def.Radius, c.Def.Area.Radius))), len(c.Scene.entities)) {
		out = append(out, e.ID)
	}
	return out
}
func swordConcentrate(c *legendaryContext) {
	if c.Skill != 10121 {
		return
	}
	st := c.State(0, "cast-actual-damage")
	sum := int64(0)
	if st.RootID == c.RootID {
		sum = st.Value
	}
	c.DeleteState(0, "cast-actual-damage")
	var target *entity.Entity
	for _, h := range c.Hits {
		if h.src != c.Owner.ID || h.blow.SkillID != 10121 || h.blow.Origin != domain.DamageNormalOrigin || h.blow.ProcDepth != 0 || h.ev.Flag.Has(event.DamageMiss) {
			continue
		}
		e := c.Scene.entities[h.dst]
		if e != nil && e.Alive() && (target == nil || e.HP > target.HP || e.HP == target.HP && e.ID < target.ID) {
			target = e
		}
	}
	if target != nil {
		c.Damage(target, domain.SaturateInt32(sum*35/100), false, true)
	}
}

func (s *Scene) legendarySwordsmanNativeRestoreAmount(src *entity.Entity, def domain.SkillDef, amount int32, hits []equipmentHit) int32 {
	if src == nil || !swordDual(def.ID) || s.legendaryState().depth != 0 || s.equipmentState().classSetTriggered != 0 {
		return amount
	}
	s.forEachLegendary(src, func(c *legendaryContext) {
		c.Skill = def.ID
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].NativeRestoreAmount; h != nil {
			amount = h(c, amount, hits)
		}
	})
	return amount
}
func (s *Scene) legendarySwordsmanNativeRestoreResolved(src *entity.Entity, def domain.SkillDef, effective, overflow int32) {
	if src == nil || !swordDual(def.ID) || s.legendaryState().depth != 0 || s.equipmentState().classSetTriggered != 0 {
		return
	}
	s.forEachLegendary(src, func(c *legendaryContext) {
		c.Skill = def.ID
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].NativeRestoreResolved; h != nil {
			h(c, effective, overflow)
		}
	})
}

func swordRecordActualDamage(c *legendaryContext) {
	if !legendaryMeleeLanded(c) || c.Skill != 10121 {
		return
	}
	st := c.State(0, "cast-actual-damage")
	if st.RootID != c.RootID {
		st.Value = 0
		st.RootID = c.RootID
	}
	st.Value += int64(c.LostHP)
}

// Admission may see an empty native100 circle while an enemy is pullable at150.
// Merely admit candidate IDs here; movement and final damage geometry wait for impact.
func swordGatherAdmission(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if c.Skill != 10121 {
		return ids
	}
	for _, e := range c.Enemies(c.Owner.Pos, 150, len(c.Scene.entities)) {
		ids = legendaryMeleeAppendUnique(ids, e.ID)
	}
	return ids
}
