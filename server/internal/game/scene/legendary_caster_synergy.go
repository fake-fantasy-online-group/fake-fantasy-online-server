package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
	"sort"
)

// These modifiers read native states only. They never create equipment timers.
func legendaryCasterNativeReady(e *entity.Entity, duration, cast, damage bool) bool {
	if e == nil || e.Status == nil {
		return false
	}
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if duration && a.Def.NextStatusDurationPct > 0 || cast && a.Def.NextCastTimeReductionPct > 0 || damage && a.Def.NextSkillDamagePct > 0 {
			found = true
		}
	})
	return found
}
func legendaryCasterAdditionalAllies(c *legendaryContext, ids []domain.EntityID, radius float64, count int) []domain.EntityID {
	center := c.Scene.entities[c.Primary]
	if center == nil {
		return ids
	}
	seen := map[domain.EntityID]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	var allies []*entity.Entity
	for _, p := range c.Scene.players {
		if !seen[p.ID] && p.Alive() && (p == c.Owner || sameParty(p, c.Owner)) && sqDist(center.Pos, p.Pos) <= radius*radius {
			allies = append(allies, p)
		}
	}
	sort.Slice(allies, func(i, j int) bool {
		a, b := allies[i], allies[j]
		left, right := int64(a.HP)*int64(b.MaxHP), int64(b.HP)*int64(a.MaxHP)
		if left != right {
			return left < right
		}
		return a.ID < b.ID
	})
	for i := 0; i < len(allies) && i < count; i++ {
		ids = append(ids, allies[i].ID)
	}
	return ids
}
func init() {
	registerLegendarySynergyHandler("SS-A12", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != legendaryIceBurst {
			return
		}
		d.LegendaryEntityCenter = true
		if c.Preview || c.Target != nil && c.Target != c.Owner {
			d.Area.Center = domain.SkillAreaCenterTarget
			if freeze, ok := c.Scene.skills.Get(legendaryFreeze, 1); ok {
				d.Dist = freeze.Dist
			}
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryIceBurst {
			d.LegendaryModifiers.DamageBP += 2000
		}
	}})
	registerLegendarySynergyHandler("SS-D01", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryCasterHasSkill(c.Owner, 10425) && c.Target != nil && sqDist(c.Owner.Pos, c.Target.Pos) <= 180*180 {
			return specialScale(n, 72)
		}
		return n
	}})
	registerLegendarySynergyHandler("SS-D02", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryCasterHasSkill(c.Owner, 10435) && int64(n)*100 > int64(c.Owner.MaxHP)*8 {
			return specialScale(n, 45)
		}
		return n
	}})
	registerLegendarySynergyHandler("SS-D05", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryCasterNativeReady(c.Owner, true, false, false) {
			return n - min(n/2, specialScale(c.Owner.MaxHP, 22))
		}
		return n
	}})
	registerLegendarySynergyHandler("SS-D09", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if _, ok := legendaryCasterSourceStatus(c.Target, 1036, c.Owner.ID); ok {
			return specialScale(n, 60)
		}
		return n
	}})
	registerLegendarySynergyHandler("SS-D10", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 { return specialScale(n, 65) }, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.HasDamageEffect() && legendaryCasterNativeReady(c.Owner, false, false, true) {
			d.LegendaryModifiers.DamageBP += 3500
		}
	}})
	registerLegendarySynergyHandler("SS-U03", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if len(d.Statuses) > 0 && legendaryCasterNativeReady(c.Owner, true, false, false) {
			d.LegendaryModifiers.ManaReductionBP += 5500
		}
	}})
	registerLegendarySynergyHandler("SS-U07", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) { d.LegendaryModifiers.ManaReductionBP += 2200 }})
	registerLegendarySynergyHandler("SS-U10", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.TargetEnemy && d.Dist > 0 && d.Area.Center != domain.SkillAreaCenterCaster {
			d.Dist = specialScale(d.Dist, 160)
		}
	}})
	registerLegendarySynergyHandler("SS-U06", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if (c.Skill == legendaryFireball || c.Skill == legendaryIceBurst) && len(c.Hits) == 0 {
			c.RefundMP(c.PaidMP)
			c.Owner.Player.ScaleRemainingSkillCooldown(c.Skill, c.Scene.tick, 20)
		}
	}})
	registerLegendarySynergyHandler("YS-A09", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryExorcism {
			d.TargetFamilies = nil
			specialCircle(d, 220, domain.SkillAreaCenterTarget, 0)
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryExorcism {
			d.LegendaryModifiers.DamageBP += 4000
		}
	}, SkillResolved: func(c *legendaryContext) {
		if c.Skill != legendaryExorcism || c.Target == nil || c.Target.Status == nil {
			return
		}
		n := 0
		c.Scene.statusesRemoved(c.Target, c.Target.Status.RemoveWhere(func(d domain.StatusDef) bool {
			if n >= 2 || d.Harmful() || d.Permanent {
				return false
			}
			n++
			return true
		}))
	}})
	registerLegendarySynergyHandler("YS-A17", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10324 {
			d.LegendaryDamageThroughControlImmunity = true
			specialCircle(d, 250, domain.SkillAreaCenterTarget, 0)
			d.PrepareMS = 0
			d.HurtType = domain.HurtHoly
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			d.Effects = append(d.Effects, domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 180})
		}
	}, TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill == legendaryExorcism && legendaryCasterHasSkill(c.Target, 10324) {
			return specialScale(n, 160)
		}
		return n
	})})
	registerLegendarySynergyHandler("YS-A19", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10325 {
			d.LegendaryDamageThroughControlImmunity = true
			specialCircle(d, 250, domain.SkillAreaCenterTarget, 0)
			d.HurtType = domain.HurtHoly
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			d.Effects = append(d.Effects, domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 200})
		}
	}, SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10325 && c.Target != nil && (c.Target.Monster == nil || c.Target.Monster.Kind != domain.MonsterBoss) {
			distance := math.Sqrt(sqDist(c.Center, c.Target.Pos))
			if distance > 80 {
				legendaryCasterPull(c, c.Target, c.Center, distance-80)
			}
		}
	}})
	registerLegendarySynergyHandler("YS-D03", legendaryHandler{SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10320 {
			return legendaryCasterAdditionalAllies(c, ids, 250, 2)
		}
		return ids
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10320 {
			d.LegendaryModifiers.StatusPowerBP += 6000
		}
	}})
	registerLegendarySynergyHandler("YS-U04", legendaryHandler{SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill == 10301 {
			return legendaryCasterAdditionalAllies(c, ids, 250, 2)
		}
		if c.Skill == 10304 {
			sort.SliceStable(ids, func(i, j int) bool {
				a, b := c.Scene.entities[ids[i]], c.Scene.entities[ids[j]]
				return int64(a.HP)*int64(b.MaxHP) < int64(b.HP)*int64(a.MaxHP)
			})
		}
		return ids
	}, Healed: func(c *legendaryContext, before, n int32) {
		if c.Skill == 10304 && legendaryCasterOnce(c, 0, "triage-refund") {
			c.RefundMP(specialScale(c.PaidMP, 35))
		}
	}})
	registerLegendarySynergyHandler("YS-D05", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryCasterHasSkill(c.Owner, 10303) && c.Scene.legendaryCasterHealingWindup(c.Owner) {
			return specialScale(n, 75)
		}
		return n
	}})
}

func init() {
	registerLegendarySynergyHandler("SS-A17", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != legendaryFlame {
			return
		}
		burn, ok := legendaryCasterSkill(c, 10421)
		if !ok {
			return
		}
		c.Scene.applyResolvedSkillModifiers(c.Owner, &burn, c.Owner.Player.Char.Skills, c.Scene.rawSkillLevels(c.Owner))
		var chosen domain.StatusApplication
		found := false
		for _, app := range burn.Statuses {
			if app.ID == 1036 {
				chosen = app
				found = true
				break
			}
		}
		if !found {
			return
		}
		for _, app := range d.Statuses {
			if app.ID == 1036 && legendaryCasterBurnApplicationTotal(c, *d, app) > legendaryCasterBurnApplicationTotal(c, burn, chosen) {
				chosen = app
			}
		}
		chosen.ChanceBP = 10000
		chosen.ChanceExplicit = true

		chosen.SourceSkill = 10421
		out := append([]domain.StatusApplication(nil), d.Statuses...)
		j := 0
		for _, app := range out {
			if app.ID != 1036 {
				out[j] = app
				j++
			}
		}
		d.Statuses = append(out[:j], chosen)
	}, StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == legendaryFlame && d.ID == 1036 {
			if _, ok := legendaryCasterSkill(c, 10421); !ok {
				return
			}
			d.LegendaryModifiers.DamageTickBP += 8200
		}
	},
	})
	registerLegendarySynergyHandler("SS-A24", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFreeze {
			d.LegendaryDamageThroughControlImmunity = true
		}
	}, SkillResolved: func(c *legendaryContext) {
		if c.Skill != legendaryFreeze || c.Target == nil || !legendaryCasterOnce(c, c.Target.ID, "freeze-arrow") {
			return
		}
		d, ok := legendaryCasterSkill(c, legendaryIceArrow)
		if ok {
			c.ChildSkill(c.Target.ID, d, legendaryChildOptions{ScaleBP: 15000, ApplyStatuses: true, PreserveControl: true})
		}
	}})
	registerLegendarySynergyHandler("SS-A09", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != legendaryIceArrow {
			return
		}
		if _, ok := legendaryCasterSkill(c, 10427); !ok {
			return
		}
		d.Statuses = append([]domain.StatusApplication(nil), d.Statuses...)
		for i := range d.Statuses {
			if d.Statuses[i].ID == 10427 {
				d.Statuses[i].ChanceBP = 10000
				d.Statuses[i].ChanceExplicit = true
			}
		}
	}, TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if _, ok := legendaryCasterSkill(c, 10427); !ok {
			return n
		}
		if c.Skill == legendaryIceArrow && (legendaryCasterHas(c.Target, domain.StatusFreeze) || legendaryCasterHas(c.Target, domain.StatusSlow) || legendaryCasterHas(c.Target, 10427)) {
			return specialScale(n, 200)
		}
		return n
	})})
	registerLegendarySynergyHandler("SS-A28", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill != 10433 || c.Target == nil || !legendaryCasterOnce(c, c.Target.ID, "dispel-strike") {
			return
		}
		kinds := map[domain.StatusID]bool{}
		if c.Owner.Status != nil {
			c.Owner.Status.EachEffective(func(a domain.Active) {
				if specialDefensiveStatus(a.Def.ID) {
					kinds[a.Def.ID] = true
				}
			})
		}
		n := specialScale(legendaryCasterBase(c, legendaryFireBolt), int32(220+50*min(3, len(kinds))))
		c.Damage(c.Target, n, true, false)
	}})
	registerLegendarySynergyHandler("YS-A23", legendaryHandler{HealResolved: func(c *legendaryContext, raw, effective, overflow int32) {
		if c.Skill != 10301 || c.Target == nil || c.Target.ID != c.Primary || raw <= 0 || !legendaryCasterOnce(c, 0, "heal-light") {
			return
		}
		enemies := c.Enemies(c.Target.Pos, 250, 1)
		if len(enemies) == 0 {
			return
		}
		d, ok := legendaryCasterSkill(c, legendaryLightBolt)
		if ok {
			c.ChildSkill(enemies[0].ID, d, legendaryChildOptions{BaseDamage: raw})
		}
	}})
	registerLegendarySynergyHandler("YS-A21", legendaryHandler{StatusCommitted: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10320 && c.Target != nil && d.DamageShield > 0 {
			st := c.State(c.Target.ID, "shield-created")
			st.RootID = c.RootID
			st.Value = int64(d.DamageShield)
			st.Stacks = 1
		}
	}, RootFinished: func(c *legendaryContext) {
		if c.Skill != 10320 {
			return
		}
		strongest := map[domain.EntityID]int32{}
		for _, id := range c.Targets {
			ally := c.Scene.entities[id]
			if ally == nil || ally.Status == nil {
				continue
			}
			st := c.State(id, "shield-created")
			if st.RootID != c.RootID || st.Stacks != 1 {
				continue
			}
			shield := clampI32(st.Value)
			c.DeleteState(id, "shield-created")
			if shield <= 0 {
				continue
			}
			for _, t := range c.Enemies(ally.Pos, 220, 256) {
				strongest[t.ID] = max(strongest[t.ID], specialScale(shield, 80))
			}
		}
		ids := make([]domain.EntityID, 0, len(strongest))
		for id := range strongest {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			c.Damage(c.Scene.entities[id], strongest[id], true, false)
		}
	}})
}

func init() {
	registerLegendarySynergyHandler("SS-D03", legendaryHandler{BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if c.Blow.Magic && legendaryCasterHasSkill(c.Owner, 10417) {
			return specialScale(n, 25)
		}
		return n
	}, StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Target != c.Owner || !specialDefensiveStatus(d.ID) {
			return
		}
		d.Affixes = append([]domain.Affix(nil), d.Affixes...)
		for i := range d.Affixes {
			if d.Affixes[i].Attr == domain.AttrPhysRes || d.Affixes[i].Attr == domain.AttrMagicRes {
				d.Affixes[i].Value = specialScale(d.Affixes[i].Value, 150)
			}
		}
		d.PhysicalReflectPct = specialScale(d.PhysicalReflectPct, 150)
		d.MagicReflectPct = specialScale(d.MagicReflectPct, 150)
		d.PhysicalReflectFlat = specialScale(d.PhysicalReflectFlat, 150)
		d.MagicReflectFlat = specialScale(d.MagicReflectFlat, 150)
		if d.FreezeAttackerChance > 0 {
			d.FreezeAttackerChance = min(int32(100), d.FreezeAttackerChance+30)
		}
		if d.MagicFullReflectChancePct > 0 {
			d.MagicFullReflectChancePct = min(int32(100), d.MagicFullReflectChancePct+30)
		}
	}})
	registerLegendarySynergyHandler("SS-U08", legendaryHandler{StatusReceiving: func(c *legendaryContext, d *domain.StatusDef) bool {
		return !legendaryCasterSlow(*d) || !legendaryCasterHasSkill(c.Owner, 10417)
	}, SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10417 && c.Owner.Status != nil && legendaryCasterHasSkill(c.Owner, 10417) {
			c.Scene.statusesRemoved(c.Owner, c.Owner.Status.RemoveWhere(legendaryCasterSlow))
		}
	}})
	registerLegendarySynergyHandler("YS-A30", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.PrepareMS <= 0 || !legendaryCasterNativeReady(c.Owner, false, true, false) {
			return
		}
		d.LegendaryInstantCast = true
		d.LegendaryModifiers.DamageBP += 4000
		d.LegendaryModifiers.HealingBP += 4000
	}})
}

func init() {
	registerLegendarySynergyHandler("SS-A03", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFireBolt {
			specialCircle(d, 150, domain.SkillAreaCenterTarget, 5)
		}
	}})
	registerLegendarySynergyHandler("SS-A07", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill != legendaryFireball {
			return
		}
		var best int64
		var original domain.EntityID
		for _, h := range c.Hits {
			if n := legendaryCasterBurn(c, c.Scene.entities[h.dst]); n > best {
				best = n
				original = h.dst
			}
		}
		if best <= 0 {
			return
		}
		n := 0
		for _, t := range c.Enemies(c.Center, float64(max(c.Def.Radius, c.Def.Area.Radius)), 256) {
			if t.ID == original {
				continue
			}
			c.Damage(t, clampI32(best*90/100), true, false)
			n++
			if n == 4 {
				break
			}
		}
	}})
	registerLegendarySynergyHandler("SS-A27", legendaryHandler{Teleported: func(c *legendaryContext, from domain.Pos) {
		if c.Skill != 10432 {
			return
		}
		amount := specialScale(legendaryCasterBase(c, legendaryFireBolt), 180)
		if amount <= 0 {
			return
		}
		d := domain.SkillDef{ID: 10432, Level: 1, Prof: 5, Kind: domain.SkillSingle, HurtType: domain.HurtFire, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		for _, t := range c.Enemies(from, 180, 256) {
			c.ChildSkill(t.ID, d, legendaryChildOptions{BaseDamage: amount, TargetOnly: true, SuppressGeneration: true})
		}
	}})

	registerLegendarySynergyHandler("SS-A29", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.HasDamageEffect() && (legendaryCasterNativeReady(c.Owner, false, false, true) || d.PrepareMS > 0 && legendaryCasterNativeReady(c.Owner, false, true, false) || len(d.Statuses) > 0 && legendaryCasterNativeReady(c.Owner, true, false, false)) {
			d.LegendaryModifiers.DamageBP += 6600
		}
	}})
	registerLegendarySynergyHandler("SS-D06", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == legendaryFreeze {
			specialCircle(d, 100, domain.SkillAreaCenterTarget, 5)
		}
	}, BeforeIncoming: func(c *legendaryContext, n int32) int32 { return specialScale(n, 65) }})
	registerLegendarySynergyHandler("YS-U06", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill != 10307 || c.Target == nil {
			return
		}
		for _, id := range []domain.SkillID{10321, 10311} {
			d, ok := legendaryCasterSkill(c, id)
			if !ok {
				continue
			}
			c.Scene.applyResolvedSkillModifiers(c.Owner, &d, c.Owner.Player.Char.Skills, c.Scene.rawSkillLevels(c.Owner))
			c.Scene.legendaryPrepareSkill(c.Owner, &d, c.Target.ID)
			for _, app := range d.Statuses {
				if def, ok := c.Scene.resolveSkillStatusDef(d, app, 0); ok {
					c.Scene.applyStatusDef(c.Target, def, c.Owner.ID)
				}
			}
		}
	}})
	registerLegendarySynergyHandler("YS-A20", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if !legendaryCasterHasSkill(c.Owner, 10330) || d.ID != legendaryLightBolt && d.ID != legendaryHolyLight && d.ID != legendaryExorcism {
			return
		}
		if d.Area.Center == domain.SkillAreaCenterCaster {
			specialCircle(d, 220, domain.SkillAreaCenterCaster, 0)
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if legendaryCasterHasSkill(c.Owner, 10330) && (d.ID == legendaryLightBolt || d.ID == legendaryHolyLight || d.ID == legendaryExorcism) {
			d.LegendaryModifiers.DamageBP += 4000
		}
	}})
	registerLegendarySynergyHandler("YS-A29", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if !legendaryCasterNativeReady(c.Owner, false, false, true) {
			return
		}
		switch d.ID {
		case legendaryLightBolt:
			specialCircle(d, 220, domain.SkillAreaCenterTarget, 0)
		case legendaryHolyLight, legendaryExorcism, legendaryDivine:
			d.Radius += 100
			d.Area.Radius += 100
		}
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.HasDamageEffect() && legendaryCasterNativeReady(c.Owner, false, false, true) {
			d.LegendaryNextDamageBonusPct += 50
		}
	}})
}

func init() {
	registerLegendarySynergyHandler("SS-A19", legendaryHandler{StatusCommitted: func(c *legendaryContext, d *domain.StatusDef) {
		if d.ID != 1036 || c.Target == nil || !legendaryCasterOnce(c, c.Target.ID, "burn-opening") {
			return
		}
		c.DamageTyped(c.Target, clampI32(legendaryCasterBurn(c, c.Target)*65/100), domain.HurtFire, false)
	}})
}

func init() {
	registerLegendarySynergyHandler("SS-D04", legendaryHandler{PassiveStats: func(c *legendaryContext, s *domain.Stats) { s.MaxHP = specialScale(s.MaxHP, 130) }, TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill == 10432 {
			return specialScale(n, 130)
		}
		return n
	})})
	registerLegendarySynergyHandler("SS-D07", legendaryHandler{PassiveStats: func(c *legendaryContext, s *domain.Stats) { s.MaxHP = specialScale(s.MaxHP, 128) }})
	registerLegendarySynergyHandler("SS-D08", legendaryHandler{PassiveStats: func(c *legendaryContext, s *domain.Stats) { s.MaxHP = specialScale(s.MaxHP, 130) }, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) { d.LegendaryModifiers.ManaReductionBP += 2000 }})
	registerLegendarySynergyHandler("SS-U04", legendaryHandler{PassiveStats: func(c *legendaryContext, s *domain.Stats) { s.MoveSpeed = specialScale(s.MoveSpeed, 140) }})
}

func (s *Scene) legendaryCasterOverlay(target *entity.Entity, d domain.StatusDef, base domain.StatusOverlay) domain.StatusOverlay {
	if d.ID != 1037 && d.ID != 1038 {
		return base
	}
	enabled := false
	s.forEachLegendary(target, func(c *legendaryContext) {
		if c.Power.ID == "SS-D07" {
			enabled = true
		}
	})
	if !enabled {
		return base
	}
	out := make(domain.StatusOverlay, len(base)+2)
	for k, v := range base {
		out[k] = v
	}
	out[domain.OverlayKey{Existing: 1037, Incoming: 1038}] = domain.OverlayCoexist
	out[domain.OverlayKey{Existing: 1038, Incoming: 1037}] = domain.OverlayCoexist
	return out
}

func init() {
	registerLegendarySynergyHandler("SS-A25", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		var passive domain.SkillID
		switch d.HurtType {
		case domain.HurtFire:
			passive = 10431
		case domain.HurtIce:
			passive = 10424
		default:
			return
		}
		p, ok := legendaryCasterSkill(c, passive)
		if !ok {
			return
		}
		var bonus int32
		for _, m := range p.PassiveModifiers {
			if m.Kind == domain.PassiveSkillDamagePct {
				bonus = max(bonus, m.Value)
			}
		}
		d.LegendaryModifiers.DamageBP += bonus * 100
	}})
}

// Conditional skill multipliers share the same additive damage bucket as
// unconditional equipment modifiers, including inherited child skill modifiers.
func legendaryCasterTargetBonus(fn func(*legendaryContext, int32) int32) func(*legendaryContext, *domain.SkillDef) {
	return func(c *legendaryContext, d *domain.SkillDef) { legendaryAddTargetDamage(d, fn(c, 10000)-10000) }
}

func init() {
	registerLegendarySynergyHandler("SS-U05", legendaryHandler{PeriodicResolved: func(c *legendaryContext, a domain.Active) {
		if a.Def.ID != 1036 || a.Source != c.Owner.ID || c.Target == nil || c.Target.Alive() || !c.Hit.Flag.Has(event.DamageFatal) {
			return
		}
		legendaryCasterBurnDeathReward(c)
	}})
	registerLegendarySynergyHandler("SS-A26", legendaryHandler{MagicResisted: func(c *legendaryContext, n int32) int32 {
		pct := int32(120)
		c.Scene.forEachLegendary(c.Owner, func(other *legendaryContext) {
			if other.Power.ID == "SS-D03" {
				pct = 180
			}
		})
		return min(specialScale(n, pct), specialScale(c.Owner.MaxHP, 18))
	}})
}

func (s *Scene) legendaryCasterHealingWindup(e *entity.Entity) bool {
	for _, cast := range s.casts {
		if cast.src == e.ID && !cast.released && !cast.releaseAtWall.IsZero() && (cast.windup == nil || !cast.windup.finished) && (cast.def.ID == 10301 || cast.def.ID == 10304 || cast.def.ID == 10305) {
			return true
		}
	}
	return false
}
func legendaryCasterBurnApplicationTotal(c *legendaryContext, d domain.SkillDef, app domain.StatusApplication) int64 {
	status, ok := c.Scene.resolveSkillStatusDef(d, app, 0)
	if !ok || status.IntervalSec <= 0 {
		return 0
	}
	maxHP := int32(1)
	if c.Target != nil {
		maxHP = c.Target.MaxHP
	}
	a := domain.Active{Def: status, ExpireAt: c.Scene.tick + domain.Ticks(int(status.DurationSec)*1000), NextTickAt: c.Scene.tick + domain.Ticks(int(status.IntervalSec)*1000)}
	return classSetPoisonRemaining(a, c.Scene.tick, maxHP)
}

// Burn settlements already contain the remaining native burn amount. Only these
// explicit producers share the native burn kill reward; other fire damage does not.
func (s *Scene) legendaryCasterDamageResolved(source *legendaryContext, target *entity.Entity, hit event.DamageDealt) {
	if source == nil || target == nil || target.Alive() || !hit.Flag.Has(event.DamageFatal) {
		return
	}
	switch source.Power.ID {
	case "SS-A02", "SS-A07", "SS-A11", "SS-A16", "SS-A19":
	default:
		return
	}
	for _, c := range s.legendaryContextsFor(source.Owner, target, combat.Blow{}, hit, true) {
		if c.Power.ID == "SS-U05" {
			legendaryCasterBurnDeathReward(&c)
		}
	}
}
func legendaryCasterBurnDeathReward(c *legendaryContext) {
	if c.Target == nil || c.Target.Alive() || !c.Hit.Flag.Has(event.DamageFatal) {
		return
	}
	st := c.State(0, fmt.Sprintf("burn-death-%d-%d", c.Scene.id.MapID, c.Target.ID))
	life := int64(c.Target.LegendaryLifeSequence)
	if st.Stacks > 0 && st.Value == life {
		return
	}
	st.Stacks = 1
	st.Value = life
	st.ExpiresMS = c.NowMS + 1
	legendaryCasterRestoreMP(c, c.Owner, specialScale(c.Owner.MaxMP, 6))
}
