package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 2026-11 buff pass: class survival identities layered onto the existing
// defensive powers. Warrior = max HP + lifesteal, swordsman = flat damage
// reduction, assassin = dodge, sorcerer = shields + spell lifesteal,
// healer = recovery.
func init() {
	// ---- warrior ----
	legendaryAugment("ZS-D01", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if bpOwnShieldActive(c) {
			bpLifesteal(c, 3)
		}
	}})
	legendaryAugment("ZS-D02", legendaryHandler{AfterHit: func(c *legendaryContext) { bpLifesteal(c, 2) }})
	legendaryAugment("ZS-D03", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if bpWarriorArmorBroken(c, c.Target) {
			bpLifesteal(c, 6)
		}
	}})
	legendaryAugment("ZS-D04", legendaryHandler{PassiveStats: func(c *legendaryContext, st *domain.Stats) { bpMaxHP(st, 20) }})
	for _, id := range []string{"ZS-D05", "ZS-D07"} {
		legendaryAugment(id, legendaryHandler{AfterHit: func(c *legendaryContext) {
			if bpOwnerHPBelow(c, 50) {
				bpLifesteal(c, 5)
			}
		}})
	}
	legendaryAugment("ZS-D06", legendaryHandler{PassiveStats: func(c *legendaryContext, st *domain.Stats) { bpMaxHP(st, 20) }, AfterHit: func(c *legendaryContext) {
		reach := 1.5 * legendaryMeleeReach(c)
		if len(bpHostilesInRange(c, c.Owner.Pos, reach)) > 0 {
			bpLifesteal(c, 3)
		}
	}})
	legendaryAugment("ZS-D08", legendaryHandler{AfterHit: bpWarriorWallDrain})
	legendaryAugment("ZS-D09", legendaryHandler{PassiveStats: func(c *legendaryContext, st *domain.Stats) { bpMaxHP(st, 25) }})
	legendaryAugment("ZS-D10", legendaryHandler{PassiveStats: func(c *legendaryContext, st *domain.Stats) { bpMaxHP(st, 30) }})

	// ---- swordsman ----
	legendaryAugment("JK-D01", legendaryHandler{AllDamageIncoming: true, BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if bpOwnShieldActive(c) {
			return legendaryMeleeScale(n, 80)
		}
		return n
	}})
	legendaryAugment("JK-D02", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeIncoming(c) {
			return legendaryMeleeScale(n, 85)
		}
		return n
	}})

	// ---- assassin: dodge ----
	legendaryDodgeProviders["CK-D01"] = func(c *legendaryContext) int32 {
		if legendaryAssassinDefenseActive(c) {
			return 2500
		}
		return 0
	}
	legendaryDodgeProviders["CK-D02"] = func(c *legendaryContext) int32 {
		if legendaryAssassinDefenseActive(c) {
			return 3000
		}
		return 0
	}
	legendaryDodgeProviders["CK-D04"] = func(c *legendaryContext) int32 {
		if legendaryAssassinDefenseActive(c) {
			return 2500
		}
		return 0
	}
	legendaryDodgeProviders["CK-D05"] = func(c *legendaryContext) int32 {
		if c.Owner.Status != nil && universalNativeControlled(c.Owner) {
			return 3000
		}
		return 0
	}
	legendaryDodgeProviders["CK-D06"] = func(c *legendaryContext) int32 {
		if legendaryAssassinDefenseActive(c) {
			return 4000
		}
		return 0
	}
	legendaryDodgeProviders["CK-D07"] = func(c *legendaryContext) int32 {
		if legendaryAssassinDefenseActive(c) {
			return 2500
		}
		return 0
	}
	legendaryDodgeProviders["CK-D10"] = func(c *legendaryContext) int32 {
		if c.Owner.Player != nil && c.Owner.Player.Char != nil && c.Owner.Player.Char.Skills[10229] > 0 {
			return c.RollValue("dodge_pct", 15) * 100
		}
		return 0
	}
	legendaryDodgeReactions["CK-D03"] = func(c *legendaryContext) { bpHealPctMax(c, c.Owner, 20) }
	legendaryAugment("CK-D08", legendaryHandler{Tick: func(c *legendaryContext) { bpRegenTick(c, "bp-regen", 10, true) }})
	legendaryAugment("CK-D09", legendaryHandler{AfterIncoming: bpAssassinEmergencyStealth})

	// ---- sorcerer: shields + spell lifesteal ----
	legendaryAugment("SS-D01", legendaryHandler{Tick: func(c *legendaryContext) {
		if c.Owner.Alive() && legendaryCasterHasSkill(c.Owner, 10425) && c.Ready(12000) {
			bpShield(c, c.Owner, "fire-ward", 10, 12)
		}
	}})
	legendaryAugment("SS-D04", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill == 10432 {
			bpShield(c, c.Owner, "phantom-ward", 15, 0)
		}
	}})
	legendaryAugment("SS-D06", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill == legendaryFreeze && c.Target != nil && c.Target.ID != c.Owner.ID && !sameParty(c.Owner, c.Target) && legendaryCasterOnce(c, 0, "bp-freeze-ward") {
			bpShield(c, c.Owner, "freeze-ward", 20, 0)
		}
	}})
	legendaryAugment("SS-D07", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill == 10425 || c.Skill == 10435 || c.Skill == 10417 {
			bpShield(c, c.Owner, "cast-ward", 10, 0)
		}
	}})
	legendaryAugment("SS-D09", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if c.Skill != 0 && !c.IsBasic && c.Target != nil {
			if _, ok := legendaryCasterSourceStatus(c.Target, 1036, c.Owner.ID); ok {
				bpLifesteal(c, 6)
			}
		}
	}})

	// ---- healer: recovery ----
	legendaryAugment("YS-D04", legendaryHandler{Tick: func(c *legendaryContext) {
		bpRegenTick(c, "bp-regen", 10, legendaryCasterHasSkill(c.Owner, 10321))
	}})
	legendaryAugment("YS-D05", legendaryHandler{StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10303 && c.Target != nil && c.Target.ID == c.Owner.ID && d.IntervalSec > 0 {
			d.LegendaryModifiers.HealingTickBP += 10000
		}
	}})
	legendaryAugment("YS-D06", legendaryHandler{Healed: func(c *legendaryContext, before, n int32) {
		if c.Skill == 10304 && n > 0 && c.Target != nil && c.Target.ID == c.Owner.ID {
			c.RestoreHealth(c.Owner, specialScale(n, 50), 0)
		}
	}})
	legendaryAugment("YS-D08", legendaryHandler{Tick: func(c *legendaryContext) {
		bpRegenTick(c, "bp-regen", 50, c.Scene.legendaryCasterWinding(c.Owner, 10305))
	}})
	legendaryAugment("YS-D09", legendaryHandler{Tick: func(c *legendaryContext) {
		bpRegenTick(c, "bp-regen", 10, legendaryCasterHasSkill(c.Owner, 10330))
	}})
	legendaryAugment("YS-D10", legendaryHandler{AfterIncoming: func(c *legendaryContext) {
		if c.Target != nil && legendaryCasterOwnSkill(c.Target, c.Owner.ID, 10325) && !c.Hit.Flag.Has(event.DamageMiss) && c.Hit.Amount > 0 {
			bpHealPctMax(c, c.Owner, 10)
		}
	}})
	legendaryPartyGuards["YS-D10"] = func(c *legendaryContext, victim *entity.Entity, amount int32) int32 {
		if c.Target != nil && legendaryCasterOwnSkill(c.Target, c.Owner.ID, 10325) {
			return specialScale(amount, 75)
		}
		return amount
	}

}

// bpWarriorArmorBroken: target carries the wearer's 破防之枪 defense loss.
func bpWarriorArmorBroken(c *legendaryContext, t *entity.Entity) bool {
	if t == nil || t.Status == nil {
		return false
	}
	broken := false
	t.Status.EachEffective(func(a domain.Active) {
		if a.Source != c.Owner.ID || a.Def.SkillSnapshotID != 10009 && a.Def.SourceSkillID != 10009 {
			return
		}
		for _, affix := range a.Def.Affixes {
			if affix.Attr == domain.AttrDef && affix.Value < 0 {
				broken = true
			}
		}
	})
	return broken
}

// bpWarriorWallDrain: each enemy landed by one cast drains 0.5% maximum HP,
// at most 10% per cast.
func bpWarriorWallDrain(c *legendaryContext) {
	if !bpLanded(c) || !c.Owner.Alive() {
		return
	}
	if c.RootID != 0 {
		per := c.State(c.Target.ID, "wall-drain-hit")
		if per.RootID == c.RootID {
			return
		}
		per.RootID = c.RootID
		per.ExpiresMS = c.NowMS + 10000
	}
	total := c.State(0, "wall-drain")
	if total.RootID != c.RootID || c.RootID == 0 {
		total.RootID, total.Stacks = c.RootID, 0
	}
	if total.Stacks >= 20 {
		return
	}
	total.Stacks++
	bpHealPctMax(c, c.Owner, 5)
}

// bpAssassinEmergencyStealth: when a hit first takes the wearer below 30%
// HP, it turns invisible for 3 seconds (60s internal cooldown).
func bpAssassinEmergencyStealth(c *legendaryContext) {
	if c.Owner == nil || !c.Owner.Alive() || c.Owner.MaxHP <= 0 || c.Hit.Amount <= 0 {
		return
	}
	threshold := int64(c.Owner.MaxHP) * 30
	if int64(c.Owner.HP)*100 >= threshold || int64(c.BeforeHP)*100 < threshold {
		return
	}
	if !c.Ready(60000) {
		return
	}
	bpTimed(c, c.Owner, "emergency-stealth", 3, false, func(d *domain.StatusDef) {
		d.Invisible = true
		d.BreakOnAttack = true
	})
}

// typifiedDrinkBlood (TY-D04 饮血): 8% of all direct damage dealt returns as
// life, 15% while the wearer is below 35% HP.
func typifiedDrinkBlood(c *legendaryContext) {
	pct := c.RollValue("lifesteal_pct", 8)
	if bpOwnerHPBelow(c, 35) {
		pct = c.RollValue("low_pct", 15)
	}
	bpLifesteal(c, pct)
}
