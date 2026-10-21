package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// prepareClassSetPoison applies only to the assassin's native poison-on-hit
// carrier, never monster poison, traps, or an arbitrary status sharing its ID.
func (s *Scene) prepareClassSetPoison(source *entity.Entity, carrier domain.StatusDef, def *domain.StatusDef) {
	if source == nil || !source.Alive() || carrier.ID != 1018 || def == nil || def.ID != domain.StatusPoison || def.IntervalSec <= 0 || def.DurationSec < def.IntervalSec {
		return
	}
	def.ClassSetPoison = true
	pieces := s.classSetPieces(source, 1)
	if pieces < 4 {
		return
	}
	// Physical attack is a cast-time average of the actual panel range. It is
	// not rolled again per tick and does not multiply the original fixed poison.
	attack := (int64(source.Stats.MinAtk) + int64(source.Stats.MaxAtk)) / 2
	if attack > 0 {
		def.ClassSetPoisonBonusTotal = attack * 10
	}
}

func (s *Scene) classSetPoisonBurst(target *entity.Entity, incoming domain.StatusDef, source domain.EntityID) int32 {
	owner := s.entities[source]
	if !incoming.ClassSetPoison || owner == nil || !owner.Alive() || s.classSetPieces(owner, 1) < 6 || target == nil || target.Status == nil {
		return 0
	}
	var remaining int64
	target.Status.EachEffective(func(a domain.Active) {
		// Only native Quenching Poison counts, including another assassin's
		// snapshot. The extra burst belongs to the new applying assassin.
		if a.Def.ID == domain.StatusPoison && a.Def.ClassSetPoison {
			remaining += classSetPoisonRemaining(a, s.tick, target.MaxHP)
		}
	})
	return domain.SaturateInt32(remaining / 5)
}

// classSetPoisonRemaining uses the actual next tick and fractional remainders,
// not a percentage of original duration. An expired poison has no balance.
func classSetPoisonRemaining(a domain.Active, now domain.Tick, maxHP int32) int64 {
	if a.Def.IntervalSec <= 0 || a.NextTickAt == 0 || a.ExpireAt == 0 || a.ExpireAt <= now || a.NextTickAt > a.ExpireAt {
		return 0
	}
	interval := domain.Ticks(int(a.Def.IntervalSec) * 1000)
	if interval == 0 {
		return 0
	}
	// DueTicks does not catch up missed deadlines: it emits one tick now,
	// then schedules now+interval. Only that future schedule is cashable.
	next := max(a.NextTickAt, now)
	ticks := int64((a.ExpireAt-next)/interval) + 1
	var perTick int64
	for _, af := range a.Def.Affixes {
		if af.Attr == domain.AttrHPRegen {
			perTick -= int64(resourceDelta(af.Value, af.Mode, maxHP))
		}
	}
	for _, change := range a.Def.HPChanges {
		perTick -= int64(resourceDelta(change.Value, change.Mode, maxHP))
	}
	total := max(int64(0), perTick) * ticks
	if a.Def.PeriodicDamageFraction100 > 0 {
		total += (a.PeriodicDamageRemainder100 + a.Def.PeriodicDamageFraction100*ticks) / 100
	}
	if count := int64(a.Def.DurationSec / a.Def.IntervalSec); count > 0 && a.Def.ClassSetPoisonBonusTotal > 0 {
		total += (a.ClassSetPoisonRemainder + a.Def.ClassSetPoisonBonusTotal*ticks) / count
	}
	return total
}

func (s *Scene) dealClassSetPoisonBurst(target *entity.Entity, source domain.EntityID, damage int32) {
	if target == nil || damage <= 0 || !target.Alive() {
		return
	}
	// Reuse DOT mitigation, attribution, death, and event paths without running
	// a basic attack or applying another status (thus no recursive poison).
	s.tickStatus(target, domain.Active{Source: source, Def: domain.StatusDef{
		ID: domain.StatusPoison, TypeKnown: true, Type: domain.StatusBad,
		HPChanges: []domain.ResourceChange{{Value: -damage, Mode: domain.ModeAbsolute}},
	}})
}
