package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// periodicMagicBeforeShield consumes the natively defended periodic payload.
// It shares the ordinary magic incoming reduction
// buckets, without replaying an attack, outgoing modifiers, hit/crit or procs.
// Element-specific defenses require actual source metadata; unknown poison is
// still magic, but is not arbitrarily turned into fire/ice/lightning damage.
func (s *Scene) periodicMagicBeforeShield(target *entity.Entity, source *entity.Entity, blow combat.Blow, amount, scaleBP int32, critical ...bool) int32 {
	if target == nil || amount <= 0 {
		return 0
	}
	s.clearDisabledRiftMonsterStatuses(target)
	if target.Status.Invulnerable() {
		return 0
	}
	pvp := blow.PeriodicPVP || s.equipmentPVP(source, target)
	magic := blow.Magic || blow.HurtType != domain.HurtPhysical
	blow.Magic = magic
	var elemental domain.ElementCombatStats
	if index := domain.HurtElementIndex(blow.HurtType); index >= 0 {
		elemental = s.equipmentDefense(target, pvp).Elements[index]
		elemental.MitigationBP += s.classSetCardCombat(target).MitigationBP[index]
	}
	guard := domain.EvaluateUniqueTraits(s.uniqueTraits(target), domain.UniqueContext{HPPercent: uniquePercent(target.HP, target.MaxHP), Magic: magic})
	half := 0
	if chance := s.incomingMagicHalfChance(target); magic && chance > 0 && int32(s.rng.Intn(10000)) < chance {
		half++
	}
	flat := target.Status.DamageReductionFlat(magic)
	if target.Kind == domain.KindPlayer {
		kind := "physical_reduction"
		if magic {
			kind = "magic_reduction"
		}
		flat += int64(s.petProcAmount(target, kind)) + int64(s.petFlatReduction(target, magic))
	}
	hit := event.DamageDealt{}
	if len(critical) > 0 && critical[0] {
		hit.Flag |= event.DamageCrit
	}
	buckets := domain.CombatDamageBuckets{
		SelectiveReductionBP: s.synergyPoolReduction(source, target, blow, hit),
		ResistanceBP:         elemental.ResistBP, MitigationBP: int64(elemental.MitigationBP) + int64(guard.MitigationBP),
		DamageTakenIncreasePct: target.Status.DamageTakenIncreasePct(magic),
		DamageReductionPct:     target.Status.DamageReductionPct(magic), DamageReductionCapPct: target.Status.DamageReductionCapPct(),
		HalfDamageLayers: half, FlatReduction: flat, PVP: pvp,
	}
	if !magic {
		buckets.MitigationBP += int64(s.equipmentDefense(target, pvp).PhysicalMitigationBP) + int64(s.classSetCardCombat(target).PhysicalMitigationBP)
	}
	amount = buckets.IncomingDamage(amount)
	amount = s.classSetMeleeIncomingDamage(target, amount)
	amount = s.classSetAuraIncomingDamage(target, magic, amount)
	return clampI32(int64(amount) * int64(scaleBP) / 10000)
}
