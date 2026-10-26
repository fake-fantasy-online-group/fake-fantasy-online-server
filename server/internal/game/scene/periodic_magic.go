package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// periodicMagicBeforeShield consumes the natively defended periodic payload.
// It shares the ordinary magic incoming reduction
// buckets, without replaying an attack, outgoing modifiers, hit/crit or procs.
// Element-specific defenses require actual source metadata; unknown poison is
// still magic, but is not arbitrarily turned into fire/ice/lightning damage.
func (s *Scene) periodicMagicBeforeShield(target *entity.Entity, source *entity.Entity, blow combat.Blow, amount, scaleBP int32) int32 {
	if target == nil || amount <= 0 {
		return 0
	}
	s.clearDisabledRiftMonsterStatuses(target)
	if target.Status.Invulnerable() {
		return 0
	}
	pvp := s.equipmentPVP(source, target)
	var elemental domain.ElementCombatStats
	if index := domain.HurtElementIndex(blow.HurtType); index >= 0 {
		elemental = s.equipmentDefense(target, pvp).Elements[index]
		elemental.MitigationBP += s.classSetCardCombat(target).MitigationBP[index]
	}
	guard := domain.EvaluateUniqueTraits(s.uniqueTraits(target), domain.UniqueContext{HPPercent: uniquePercent(target.HP, target.MaxHP), Magic: true})
	half := 0
	if chance := s.incomingMagicHalfChance(target); chance > 0 && int32(s.rng.Intn(10000)) < chance {
		half++
	}
	flat := target.Status.DamageReductionFlat(true)
	if target.Kind == domain.KindPlayer {
		flat += int64(s.petProcAmount(target, "magic_reduction")) + int64(s.petFlatReduction(target, true))
	}
	buckets := domain.CombatDamageBuckets{
		ResistanceBP: elemental.ResistBP, MitigationBP: int64(elemental.MitigationBP) + int64(guard.MitigationBP),
		DamageTakenIncreasePct: target.Status.DamageTakenIncreasePct(true),
		DamageReductionPct:     target.Status.DamageReductionPct(true), DamageReductionCapPct: target.Status.DamageReductionCapPct(),
		HalfDamageLayers: half, FlatReduction: flat, PVP: pvp,
	}
	amount = buckets.IncomingDamage(amount)
	amount = s.classSetMeleeIncomingDamage(target, amount)
	amount = s.classSetAuraIncomingDamage(target, true, amount)
	return clampI32(int64(amount) * int64(scaleBP) / 10000)
}
