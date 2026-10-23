package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func init() {
	registerLegendaryHandler("JK-A10", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && c.Skill == 10104 && legendaryMeleePrimary(c) {
			swordDebuff(c, "antiheal", domain.StatusDef{HealingReceivedReductionPct: 65})
		}
	}})
	registerLegendaryHandler("JK-A18", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && c.Skill == 10122 {
			swordDebuff(c, "armor", domain.StatusDef{Affixes: []domain.Affix{{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: -30}}})
		}
	}})
	registerLegendaryHandler("JK-A14", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && c.Skill == 10113 && !swordHasShield(c.Target) {
			return legendaryMeleeScale(n, 150)
		}
		return n
	}, AfterHit: func(c *legendaryContext) {
		if !legendaryMeleeLanded(c) || c.Skill != 10113 || !legendaryMeleePrimary(c) || c.Target.Status == nil || c.Absorbed <= 0 {
			return
		}
		amount := min(domain.SaturateInt32((int64(c.LostHP)+int64(c.Absorbed))*120/100), legendaryMeleeScale(c.Attack(false), 400))
		_, removed := c.Target.Status.AbsorbDamage(amount)
		c.Scene.statusesRemoved(c.Target, removed)
		c.Scene.pushStatusSnapshots(c.Target)
	}})
}
func swordHasShield(target *entity.Entity) bool {
	if target == nil || target.Status == nil {
		return false
	}
	found := false
	target.Status.EachEffective(func(a domain.Active) {
		if a.ShieldLeft > 0 {
			found = true
		}
	})
	return found
}
func swordDebuff(c *legendaryContext, phase string, d domain.StatusDef) {
	if c.Target == nil || !c.Target.Alive() {
		return
	}
	d.ID = legendaryMeleeStatusID(c, phase)
	d.Level = 1
	d.Name = c.Power.Name
	d.Desc = c.Power.Effect
	d.TypeKnown = true
	d.Type = domain.StatusBad
	d.DurationSec = 8
	d.LegendaryPowerID = c.Power.ID
	d.LegendarySourceUID = c.SourceUID
	d.Icon = legendaryMeleeIcon(c)
	d.IconKnown = true
	// Native status overlay refresh semantics retain one same-name instance.
	c.Scene.applyStatusDef(c.Target, d, c.Owner.ID)
}
