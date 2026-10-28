package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Compatibility boundaries for shared native call sites. Revised pending
// defenses do not silently execute the retired timed-shield/charge mechanics.
func (s *Scene) legendaryAssassinVisibility(owner *entity.Entity, was, now bool) {
	if !was || now || owner == nil || !owner.Alive() {
		return
	}
	for _, c := range s.legendaryContexts(owner, nil, combat.Blow{}, event.DamageDealt{}) {
		if c.Power.ID == "CK-D01" && legendaryAssassinLearned(&c, 10208) && c.State(0, "stealth-armed").Stacks > 0 {
			c.State(0, "exit-guard").ExpiresMS = c.NowMS + 8000
			c.DeleteState(0, "stealth-armed")
		}
	}
}
func (s *Scene) legendaryAssassinControlDuration(target *entity.Entity, def *domain.StatusDef) {}
func (s *Scene) legendaryAssassinPoisonIncoming(target *entity.Entity, amount int32) int32 {
	if c, ok := legendaryAssassinPowerContext(s, target, "CK-D08"); ok {
		return clampI32(int64(amount) * int64(100-c.RollValue("reduction_pct", 30)) / 100)
	}
	return amount
}
func (s *Scene) legendaryAssassinPeriodicDamage(src, target *entity.Entity, ev event.DamageDealt) {}

// Balance defaults for rewritten static defenses:20% damage mitigation,
// 30% control-duration reduction,15% native counter lifesteal,20% maximum HP.
// These values are engineering tuning, not additional user-confirmed numbers.
func init() {
	for _, id := range []string{"CK-D01", "CK-D02", "CK-D06", "CK-D04", "CK-D07", "CK-D10"} {
		registerLegendarySynergyHandler(id, legendaryHandler{AllDamageIncoming: id != "CK-D04", BeforeIncoming: legendaryAssassinNativeDefense, PrepareSkill: legendaryAssassinDefensePrepare, TransformSkill: legendaryAssassinSmokeTransform, CastReleased: legendaryAssassinDefenseRelease, StatusCommitted: legendaryAssassinDefenseCommitted})
	}
	registerLegendarySynergyHandler("CK-D03", legendaryHandler{AfterHit: legendaryAssassinCounterHeal})
	registerLegendarySynergyHandler("CK-D05", legendaryHandler{StatusReceiving: legendaryAssassinReceiveControl})
	registerLegendarySynergyHandler("CK-D09", legendaryHandler{PrepareSkill: legendaryAssassinDefensePrepare, PassiveStats: legendaryAssassinDefenseStats})
}
func legendaryAssassinNativeDefense(c *legendaryContext, n int32) int32 {
	active := false
	switch c.Power.ID {
	case "CK-D06":
		active = c.Blow.SourceSkillRange > 100 || c.IsBasic && c.Target != nil && c.Target.Look.AtkDist > 100
	case "CK-D02":
		if c.Target != nil && c.Target.Status != nil {
			c.Target.Status.EachEffective(func(a domain.Active) {
				if a.Source == c.Owner.ID && (a.Def.SourceSkillID == 10224 || a.Def.SkillSnapshotID == 10224) {
					for _, af := range a.Def.Affixes {
						if af.Attr == domain.AttrHit && af.Value < 0 {
							active = true
						}
					}
				}
			})
		}
	case "CK-D01":
		active = legendaryAssassinLearned(c, 10208) && c.State(0, "exit-guard").ExpiresMS > c.NowMS
		if c.Owner.Status != nil {
			c.Owner.Status.EachEffective(func(a domain.Active) {
				if legendaryAssassinLearned(c, 10208) && a.Def.Invisible && (a.Def.SourceSkillID == 10208 || a.Def.SkillSnapshotID == 10208) {
					active = true
				}
			})
		}
	case "CK-D07":
		active = legendaryAssassinLearned(c, 10219) && c.State(0, "stun-guard").ExpiresMS > c.NowMS
	case "CK-D04":
		if c.Target != nil && c.Target.Status != nil {
			c.Target.Status.EachEffective(func(a domain.Active) {
				if a.Source != c.Owner.ID {
					return
				}
				if c.Power.ID == "CK-D04" && a.Def.ID == domain.StatusPoison && a.Def.ClassSetPoison {
					active = true
				}
			})
		}
	case "CK-D10":
		active = (c.Blow.Trap || c.Blow.SourceTrap) && c.Owner.Player != nil && c.Owner.Player.Char.Skills[10229] > 0
	}
	if active {
		if c.Power.ID == "CK-D01" || c.Power.ID == "CK-D07" {
			return clampI32(int64(n) * int64(100-c.RollValue("reduction_pct", 35)) / 100)
		}
		return clampI32(int64(n) * int64(100-c.RollValue("reduction_pct", 20)) / 100)
	}
	return n
}
func legendaryAssassinCounterHeal(c *legendaryContext) {
	if c.Skill == 10203 && !c.Hit.Flag.Has(event.DamageMiss) && c.LostHP > 0 {
		c.RestoreHealth(c.Owner, clampI32(int64(c.LostHP)*int64(c.RollValue("restore_pct", 15))/100), 10203)
		c.Scene.emitAttributesIfPlayer(c.Owner)
	}
}
func legendaryAssassinReceiveControl(c *legendaryContext, d *domain.StatusDef) bool {
	if d.ID == domain.StatusStun || d.ID == domain.StatusFreeze || d.BlockMove {
		d.LegendaryModifiers.ControlDurationReductionBP += c.RollValue("control_bp", 3000)
	}
	return true
}
func legendaryAssassinDefensePrepare(c *legendaryContext, d *domain.SkillDef) {
	if c.Power.ID == "CK-D06" && d.ID == 10213 {
		d.LegendaryModifiers.ManaReductionBP += 3000
	}
	if c.Power.ID == "CK-D07" && d.ID == 10219 {
		d.LegendaryModifiers.CooldownReductionBP += 3000
	}
	if c.Power.ID == "CK-D09" && d.ID == 10208 {
		d.LegendaryModifiers.ManaReductionBP += 3000
	}
}
func legendaryAssassinDefenseStats(c *legendaryContext, stats *domain.Stats) {
	if c.Power.ID == "CK-D09" {
		stats.MaxHP = clampI32(int64(stats.MaxHP) * int64(100+c.RollValue("life_pct", 20)) / 100)
	}
}

func legendaryAssassinSmokeTransform(c *legendaryContext, d *domain.SkillDef) {
	if c.Power.ID == "CK-D02" && d.ID == 10224 {
		d.Kind = domain.SkillArea
		d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: max(int32(100), d.Radius)}
	}
}
func init() {
	registerLegendarySynergyHandler("CK-D08", legendaryHandler{CastReleased: legendaryAssassinCleanPoison})
}
func legendaryAssassinCleanPoison(c *legendaryContext) {
	if c.Skill != 10211 || c.Owner.Status == nil {
		return
	}
	count := 0
	removed := c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		if count >= 2 || d.ID != domain.StatusPoison || d.Permanent {
			return false
		}
		count++
		return true
	})
	c.Scene.statusesRemoved(c.Owner, removed)
	if len(removed) > 0 {
		c.Visual("stage-01", c.Owner)
	}
}

func legendaryAssassinDefenseRelease(c *legendaryContext) {
	if c.Power.ID == "CK-D07" && c.Skill == 10219 && legendaryAssassinLearned(c, 10219) {
		c.State(0, "stun-guard").ExpiresMS = c.NowMS + 8000
	}
}

func legendaryAssassinDefenseCommitted(c *legendaryContext, d *domain.StatusDef) {
	if c.Power.ID == "CK-D01" && legendaryAssassinLearned(c, 10208) && d.Invisible && (d.SourceSkillID == 10208 || d.SkillSnapshotID == 10208) {
		c.State(0, "stealth-armed").Stacks = 1
	}
}
