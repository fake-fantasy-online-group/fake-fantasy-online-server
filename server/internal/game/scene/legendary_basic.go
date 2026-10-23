package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Basic transformations modify the already admitted swing. They neither spend
// skill resources nor create a cast, and retain wire SkillID0/native cadence.
func (s *Scene) legendaryBasicAttack(src, dst *entity.Entity, tr *combat.HitTracker, original combat.Blow) bool {
	if src.Player == nil || original.SkillID != 0 || original.ProcDepth != 0 {
		return false
	}
	def := domain.SkillDef{LegendaryBasic: true, TargetEnemy: true, HurtType: domain.HurtPhysical, Dist: int32(src.Look.AtkDist), Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: original.SkillPct, GuaranteedHit: original.GuaranteedHit}}}
	if def.Effects[0].DamagePct == 0 {
		def.Effects[0].DamagePct = 100
	}
	s.legendaryPrepareSkill(src, &def, dst.ID)
	if !def.LegendaryBasic {
		def.LegendaryBasic = true
	}
	if def.ID == 0 && !def.Area.Valid() && def.Radius == 0 && len(def.Effects) == 1 && def.Effects[0].DamagePct == max(int32(100), original.SkillPct) && def.Effects[0].BaseDamageFlat == 0 && def.Effects[0].GuaranteedHit == original.GuaranteedHit {
		return false
	}
	targets := []domain.EntityID{dst.ID}
	if def.Area.Valid() || def.Radius > 0 {
		targets = s.resolveTargets(src, def, dst.ID, nil)
		found := false
		for _, id := range targets {
			found = found || id == dst.ID
		}
		if !found {
			targets = append([]domain.EntityID{dst.ID}, targets...)
		}
	}
	targets = s.legendarySelectTargets(src, def, dst.ID, targets)
	root := s.equipmentState().root
	root.def = def
	root.targets = targets
	root.originalTargets = []domain.EntityID{dst.ID}
	root.center = src.Pos
	for _, id := range targets {
		target := s.entities[id]
		if target == nil || !target.Alive() || !s.equipmentProcEnemyTarget(src, target) {
			continue
		}
		local := s.legendaryTargetDefinition(src, def, target, dst.ID, targets)
		for _, effect := range local.DirectEffects() {
			if effect.Kind != domain.EffectDamage {
				continue
			}
			blow := original
			blow.EffectiveSkillID = int32(local.ID)
			blow.SkillID = 0
			blow.SkillPct = effect.DamagePct
			blow.SkillPctNumerator = effect.DamagePctNumerator
			blow.SkillPctDenominator = effect.DamagePctDenominator
			blow.BaseDamageFlat = effect.BaseDamageFlat
			blow.FlatDamage = effect.DamageFlat
			blow.GuaranteedHit = effect.GuaranteedHit
			blow.GuaranteedCrit = original.GuaranteedCrit || effect.GuaranteedCrit
			blow.Area = len(targets) > 1
			ev, raw, reflect := s.resolveStrikeStatuses(src, target, tr, blow)
			s.emit(ev)
			s.afterStrikeStatuses(src, target, ev, raw, reflect)
			s.consumeHitDurability(target, ev)
			s.emitAttributesIfPlayer(target)
			s.aggroAfterHit(target, src.ID, ev)
			if ev.Flag.Has(event.DamageMiss) {
				if counter, ok := s.passiveCounterPending(target, src); ok {
					s.attacks = append(s.attacks, counter)
				}
			}
			if ev.Flag.Has(event.DamageFatal) {
				s.onDeath(target, src.ID)
			}
		}
	}
	s.consumeAttackDurability(src)
	return true
}

func (s *Scene) deferLegendaryBasicLeech(owner *entity.Entity, hit event.DamageDealt) bool {
	root := s.equipmentState().root
	if root == nil || root.src != owner.ID || !root.def.LegendaryBasic || len(root.targets) < 2 || hit.SkillID != 0 || hit.ProcDepth != 0 {
		return false
	}
	root.legendaryBasicLeech += int64(hit.Amount)
	return true
}
func (s *Scene) flushLegendaryBasicLeech(root *equipmentRoot) {
	owner := s.entities[root.src]
	if root.legendaryBasicLeech <= 0 || owner == nil || !owner.Alive() || owner.Status == nil {
		return
	}
	damage := root.legendaryBasicLeech
	root.legendaryBasicLeech = 0
	_, _, _, lifePct, lifeChance, manaPct, manaChance := owner.Status.CombatTriggers()
	if lifePct > 0 && int32(s.rng.Intn(100)) < lifeChance {
		amount := clampI32(damage * int64(lifePct) / 100)
		if amount > 0 {
			s.emit(combat.Restore(owner, owner, s.legendaryIncomingHeal(owner, amount), 0))
		}
	}
	if manaPct > 0 && int32(s.rng.Intn(100)) < manaChance {
		owner.MP = min(owner.MaxMP, clampI32(int64(owner.MP)+damage*int64(manaPct)/100))
	}
	s.emitAttributesIfPlayer(owner)
}
