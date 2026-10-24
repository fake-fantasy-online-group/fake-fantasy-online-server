package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

func legendaryCasterControlled(target *entity.Entity) bool {
	if target == nil || target.Status == nil {
		return false
	}
	controlled := false
	target.Status.EachEffective(func(a domain.Active) {
		controlled = controlled || a.Def.ID == domain.StatusFreeze || legendaryCasterSlow(a.Def)
	})
	return controlled
}

func (s *Scene) legendaryCasterSnapshotControl(owner *entity.Entity, def *domain.SkillDef) {
	if def == nil || def.ID != legendaryIceBurst {
		return
	}
	for _, source := range def.LegendarySources {
		if source.PowerID != "SS-A15" {
			continue
		}
		def.LegendaryControlSnapshot = map[domain.EntityID]bool{}
		for id, target := range s.entities {
			if target.Alive() && s.equipmentProcEnemyTarget(owner, target) {
				def.LegendaryControlSnapshot[id] = legendaryCasterControlled(target)
			}
		}
		return
	}
}
func legendaryCasterHadControl(c *legendaryContext) bool {
	if c.Target == nil {
		return false
	}
	if c.Def.LegendaryControlSnapshot != nil {
		return c.Def.LegendaryControlSnapshot[c.Target.ID]
	}
	return legendaryCasterControlled(c.Target)
}
func (s *Scene) legendaryCasterPreservesFreeze(owner, target *entity.Entity, skill int32) bool {
	if skill != int32(legendaryIceBurst) && skill != int32(legendaryBlizzard) {
		return false
	}
	for _, c := range s.legendaryContextsFor(owner, target, combat.Blow{SkillID: skill}, event.DamageDealt{}, true) {
		if skill == int32(legendaryIceBurst) && (c.Power.ID == "SS-A12" || c.Power.ID == "SS-A15") || skill == int32(legendaryBlizzard) && c.Power.ID == "SS-A22" {
			return true
		}
	}
	return false
}

// Guaranteed application bypasses accuracy resistance only. Explicit skill,
// encounter, status-category and overlay immunities remain authoritative.
func (s *Scene) legendaryCasterGuaranteedStatus(owner, target *entity.Entity, def domain.StatusDef) bool {
	skill := def.SkillSnapshotID
	if skill == 0 {
		skill = domain.SkillID(def.SourceSkillID)
	}
	if skill != legendaryIceArrow && skill != legendaryFlame {
		return false
	}
	for _, c := range s.legendaryContextsFor(owner, target, combat.Blow{SkillID: int32(skill)}, event.DamageDealt{}, true) {
		if c.Power.ID == "SS-A09" && skill == legendaryIceArrow && def.ID == 10427 {
			return true
		}
		if c.Power.ID == "SS-A17" && skill == legendaryFlame && def.ID == 1036 {
			return true
		}
	}
	return false
}

func legendaryCasterNewBurnTotal(c *legendaryContext, def domain.StatusDef) int64 {
	if def.IntervalSec <= 0 || def.DurationSec <= 0 || c.Target == nil {
		return 0
	}
	duration := domain.Ticks(int(def.DurationSec) * 1000)
	if def.LegendaryDurationMS > 0 {
		duration = domain.Ticks(int(def.LegendaryDurationMS))
	}
	a := domain.Active{Def: def, ExpireAt: c.Scene.tick + duration, NextTickAt: c.Scene.tick + domain.Ticks(int(def.IntervalSec)*1000)}
	return classSetPoisonRemaining(a, c.Scene.tick, c.Target.MaxHP) * (10000 + int64(def.LegendaryModifiers.DamageTickBP)) / 10000
}

func (s *Scene) legendaryCasterSecondaryHeal(src, dst *entity.Entity, skill domain.SkillID) bool {
	if src == nil || dst == nil || skill != 10301 {
		return false
	}
	root := s.equipmentState().root
	if root == nil || root.src != src.ID || root.def.ID != skill || root.primary == dst.ID {
		return false
	}
	for _, c := range s.legendaryContextsFor(src, dst, combat.Blow{SkillID: int32(skill)}, event.DamageDealt{}, true) {
		if c.Power.ID == "YS-U04" {
			return true
		}
	}
	return false
}

func (s *Scene) legendaryCasterFireballLands(owner, target *entity.Entity, def domain.SkillDef) bool {
	return target != nil && target.Alive() && s.equipmentProcEnemyTarget(owner, target) &&
		s.equipmentCombatReady(owner) && s.equipmentCombatReady(target) && s.matchesSkillTargetFamilies(target, def) &&
		(!s.legendaryCasterSkillImmune(target, def.ID) || def.LegendaryDamageThroughControlImmunity)
}

func legendaryCasterPrismTargets(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if c.Skill != legendaryLightBolt || c.Def.LegendaryNextDamageBonusPct != 0 || len(ids) <= 1 {
		return ids
	}
	center := c.Scene.entities[c.Primary]
	if center == nil {
		return ids
	}
	out := append([]domain.EntityID(nil), ids...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i] == c.Primary {
			return out[j] != c.Primary
		}
		if out[j] == c.Primary {
			return false
		}
		a, b := c.Scene.entities[out[i]], c.Scene.entities[out[j]]
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		da, db := sqDist(center.Pos, a.Pos), sqDist(center.Pos, b.Pos)
		if da == db {
			return a.ID < b.ID
		}
		return da < db
	})
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}
