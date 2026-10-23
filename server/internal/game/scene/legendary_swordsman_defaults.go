package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

// Exact coefficients and replacement directions are recorded separately in the
// reviewed swordsman implementation specification, never inferred at runtime.
func init() {
	registerLegendarySynergyHandler("JK-A01", legendaryHandler{BeforeDamage: legendaryMeleeSkillDamage(10102, 200)})
	registerLegendarySynergyHandler("JK-A03", legendaryHandler{AfterHit: swordImmediatePrimary(10102, 160)})
	registerLegendarySynergyHandler("JK-A06", legendaryHandler{StatusCommitted: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Def.ID == 10111 && d.BlockMove && d.Control != domain.ControlStun && swordRooted(c.Target, c.Owner.ID) && swordNativeOnce(c, c.Target.ID) {
			c.Damage(c.Target, legendaryMeleeScale(c.Attack(false), 200), false, false)
		}
	}})
	registerLegendarySynergyHandler("JK-A12", legendaryHandler{NativeRestoreResolved: func(c *legendaryContext, effective, overflow int32) {
		if c.Skill == 10119 && effective > 0 {
			c.Damage(c.Scene.entities[c.Primary], legendaryMeleeScale(effective, 280), false, false)
		}
	}})
	registerLegendarySynergyHandler("JK-A17", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10121 {
			d.Radius = max(d.Radius, 150)
			if d.Area.Valid() {
				d.Area.Radius = max(d.Area.Radius, 150)
			}
		}
	}})
	registerLegendarySynergyHandler("JK-A19", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if !legendaryMeleeDirect(c) {
			return n
		}
		switch c.Skill {
		case 10122, 10102:
			return legendaryMeleeScale(n, 125)
		case 10117:
			return legendaryMeleeScale(n, 200)
		}
		return n
	}, ImpactTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill != 10102 || c.Target == nil {
			return ids
		}
		for _, e := range legendaryMeleeNearbyOthers(c, 150, 4) {
			ids = legendaryMeleeAppendUnique(ids, e.ID)
		}
		return ids
	}})
	registerLegendarySynergyHandler("JK-A21", legendaryHandler{AfterHit: swordImmediatePrimary(10116, 320)})
	registerLegendarySynergyHandler("JK-A25", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10127 && c.Target != nil && c.Target.Monster != nil && c.Target.Monster.Target == c.Owner.ID && c.Target.Monster.TauntPower > 0 && swordNativeOnce(c, c.Target.ID) {
			c.Damage(c.Target, legendaryMeleeScale(c.Attack(false), 160), false, false)
		}
	}})
	registerLegendarySynergyHandler("JK-A26", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && (swordDual(c.Skill) || c.Skill == 10102 || c.Skill == 10117) {
			return legendaryMeleeScale(n, 200)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-A27", legendaryHandler{RootFinished: swordSingleCastArray})
	registerLegendarySynergyHandler("JK-A28", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if swordDual(c.Skill) {
			swordImmediatePrimary(c.Skill, 260)(c)
		}
	}})
	registerLegendarySynergyHandler("JK-A29", legendaryHandler{AfterIncoming: func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && c.Owner != c.Target && !sameParty(c.Owner, c.Target) {
			c.Damage(c.Target, legendaryMeleeScale(c.Attack(false), 180), false, false)
		}
	}})
	registerLegendarySynergyHandler("JK-A30", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && legendaryMeleePrimary(c) && (c.Skill == 10116 || c.Skill == 10123) {
			return legendaryMeleeScale(n, 210)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-D01", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && c.Skill == 10102 && swordNativeOnce(c, 0) {
			swordPermanentShield(c, c.Owner, specialScale(c.Owner.MaxHP, 12))
		}
	}})
	registerLegendarySynergyHandler("JK-D02", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if c.Blow.Origin == domain.DamageDOTOrigin {
			return legendaryMeleeScale(n, 45)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-D04", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) && c.Target.Monster != nil && c.Target.Monster.Target == c.Owner.ID && c.Target.Monster.TauntPower > 0 {
			return legendaryMeleeScale(n, 50)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-D07", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) && swordSilencedBy(c.Target, c.Owner.ID) {
			return legendaryMeleeScale(n, 65)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-D08", legendaryHandler{AfterHit: swordLionShields})
	registerLegendarySynergyHandler("JK-D09", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) && int64(n)*100 >= int64(c.Owner.MaxHP)*8 {
			return legendaryMeleeScale(n, 55)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-D10", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) && sqDist(c.Owner.Pos, c.Target.Pos) > 75*75 {
			return legendaryMeleeScale(n, 65)
		}
		return n
	}})
	registerLegendarySynergyHandler("JK-U01", legendaryHandler{PassiveStats: func(c *legendaryContext, stats *domain.Stats) {
		stats.MoveSpeed = legendaryMeleeScale(stats.MoveSpeed, 135)
	}})
	registerLegendarySynergyHandler("JK-U04", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10127 {
			d.Dist = legendaryMeleeScale(d.Dist, 135)
			d.Radius = legendaryMeleeScale(d.Radius, 135)
			d.Area.Radius = legendaryMeleeScale(d.Area.Radius, 135)
		}
	}})
	registerLegendarySynergyHandler("JK-U06", legendaryHandler{StatusCommitted: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Def.ID == 10111 && d.BlockMove && d.Control != domain.ControlStun && c.Target != nil {
			c.State(c.Target.ID, "bound-this-cast").RootID = c.RootID
		}
	}, RootFinished: func(c *legendaryContext) {
		bound := false
		if c.Target != nil {
			st := c.State(c.Target.ID, "bound-this-cast")
			bound = st.RootID == c.RootID
			c.DeleteState(c.Target.ID, "bound-this-cast")
		}
		if c.Skill != 10111 || c.Target == nil || bound {
			return
		}
		for _, h := range c.Hits {
			if h.dst == c.Primary && swordNativeHit(c, h) {
				c.RefundMP(specialScale(c.PaidMP, 60))
				return
			}
		}
	}})
	registerLegendarySynergyHandler("JK-U07", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if (c.Skill == 10102 || c.Skill == 10117 || c.Skill == 10103) && swordLandedCount(c.Owner.ID, c.Skill, c.Hits) > 0 {
			legendaryMeleeRestoreMP(c, specialScale(c.Owner.MaxMP, 12))
		}
	}})
	registerLegendarySynergyHandler("JK-U10", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10113 {
			d.Dist = legendaryMeleeScale(d.Dist, 145)
		}
	}})
}
func swordNativeHit(c *legendaryContext, h equipmentHit) bool {
	return h.src == c.Owner.ID && h.blow.SkillID == int32(c.Skill) && h.blow.Origin == domain.DamageNormalOrigin && h.blow.ProcDepth == 0 && !h.ev.Flag.Has(event.DamageMiss)
}
func swordNativeOnce(c *legendaryContext, target domain.EntityID) bool {
	root := c.Scene.equipmentState().root
	if root == nil || root.id != c.RootID {
		return false
	}
	key := c.key(target, "native-root")
	if root.swordTriggers == nil {
		root.swordTriggers = map[legendaryStateKey]bool{}
	}
	if root.swordTriggers[key] {
		return false
	}
	root.swordTriggers[key] = true
	return true
}
func swordImmediatePrimary(skill domain.SkillID, pct int64) func(*legendaryContext) {
	return func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && legendaryMeleePrimary(c) && c.Skill == skill && swordNativeOnce(c, c.Target.ID) {
			c.Damage(c.Target, legendaryMeleeScale(c.Attack(false), pct), false, false)
		}
	}
}
func swordSingleCastArray(c *legendaryContext) {
	if c.Skill != 10102 && c.Skill != 10103 && c.Skill != 10121 {
		return
	}
	var center *entity.Entity
	for _, h := range c.Hits {
		if !swordNativeHit(c, h) {
			continue
		}
		e := c.Scene.entities[h.dst]
		if e != nil && (center == nil || sqDist(c.Owner.Pos, e.Pos) < sqDist(c.Owner.Pos, center.Pos) || sqDist(c.Owner.Pos, e.Pos) == sqDist(c.Owner.Pos, center.Pos) && e.ID < center.ID) {
			center = e
		}
	}
	if center == nil {
		return
	}
	for _, e := range c.Enemies(center.Pos, 150, 8) {
		c.Damage(e, legendaryMeleeScale(c.Attack(false), 320), false, false)
	}
}
func swordSilencedBy(e *entity.Entity, source domain.EntityID) bool {
	if e == nil || e.Status == nil {
		return false
	}
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if a.Source == source && a.Def.Control == domain.ControlSilence {
			found = true
		}
	})
	return found
}
func swordPermanentShield(c *legendaryContext, target *entity.Entity, amount int32) {
	if target == nil || !target.Alive() || amount <= 0 {
		return
	}
	id := legendaryMeleeStatusID(c, fmt.Sprintf("shield:%d:%d", c.Owner.ID, c.SourceUID))
	if target.Status != nil {
		target.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == id {
				amount = max(amount, a.ShieldLeft)
			}
		})
		c.Scene.statusesRemoved(target, target.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == id }))
	}
	c.Scene.applyStatusDef(target, domain.StatusDef{ID: id, Level: 1, Name: c.Power.Name, Desc: c.Power.Effect, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, TypeKnown: true, Type: domain.StatusGood, Permanent: true, DamageShield: amount, Icon: legendaryMeleeIcon(c), IconKnown: true}, c.Owner.ID)
}
func swordLionShields(c *legendaryContext) {
	if !legendaryMeleeLanded(c) || !legendaryMeleePrimary(c) || c.Skill != 10116 || !swordNativeOnce(c, 0) {
		return
	}
	amount := specialScale(c.Owner.MaxHP, 14)
	allies := []*entity.Entity{}
	for _, p := range c.Scene.players {
		if p.ID != c.Owner.ID && p.Alive() && sameParty(c.Owner, p) && p.Pos.MapID == c.Owner.Pos.MapID && sqDist(c.Owner.Pos, p.Pos) <= 180*180 {
			allies = append(allies, p)
		}
	}
	sort.Slice(allies, func(i, j int) bool {
		a, b := int64(allies[i].HP)*int64(allies[j].MaxHP), int64(allies[j].HP)*int64(allies[i].MaxHP)
		if a != b {
			return a < b
		}
		return allies[i].ID < allies[j].ID
	})
	swordPermanentShield(c, c.Owner, amount)
	for i, p := range allies {
		if i == 2 {
			break
		}
		swordPermanentShield(c, p, amount)
	}
}
