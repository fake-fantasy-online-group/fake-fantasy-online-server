package combat

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
)

func TestLegendaryAddedBaseDefendedExactlyOnce(t *testing.T) {
	attacker := &entity.Entity{ID: 1, HP: 1000, Stats: domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}}
	target := &entity.Entity{ID: 2, HP: 1000, Stats: domain.Stats{PhysResist: 30, MagicResist: 30, MDef: 100}}
	for _, tc := range []struct {
		magic bool
		want  int32
	}{{false, 170}, {true, 107}} {
		b := Blow{SkillPct: 100, BaseDamageFlat: 100, GuaranteedHit: true, Magic: tc.magic}
		ev := CalculateStrike(attacker, target, &HitTracker{}, b, nil)
		if ev.Amount != tc.want {
			t.Fatalf("magic=%v got%d want%d", tc.magic, ev.Amount, tc.want)
		}
	}
}
func TestLegendaryFlatOnlyNativeBase(t *testing.T) {
	attacker := &entity.Entity{ID: 1, HP: 1000, Stats: domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}}
	target := &entity.Entity{ID: 2, HP: 1000, Stats: domain.Stats{PhysResist: 20}}
	b := Blow{SkillPctDenominator: 1, BaseDamageFlat: 70, GuaranteedHit: true}
	ev := CalculateStrike(attacker, target, &HitTracker{}, b, nil)
	if ev.Amount != 50 {
		t.Fatalf("flat-only inherited attack: %d", ev.Amount)
	}
}

func TestLegendaryStrikeModifiersAreLocal(t *testing.T) {
	a := &entity.Entity{ID: 1, HP: 1000, Stats: domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}}
	d := &entity.Entity{ID: 2, HP: 1000, Stats: domain.Stats{PhysResist: 40, MagicResist: 40}}
	got := CalculateStrike(a, d, &HitTracker{}, Blow{GuaranteedHit: true, FixedDefensePenetrationBP: 5000}, nil)
	if got.Amount != 80 || d.Stats.PhysResist != 40 {
		t.Fatalf("local penetration: %+v target=%+v", got, d.Stats)
	}
	got = CalculateStrike(a, d, &HitTracker{}, Blow{GuaranteedHit: true, CritChanceBonusBP: 10000, CriticalBonusBP: 5000}, upperEndpointRand{})
	if got.Amount != 150 || a.Stats.CritRate != 0 {
		t.Fatalf("local crit: %+v source=%+v", got, a.Stats)
	}
}
func TestLegendaryNativeSnapshotDefendedWithoutReroll(t *testing.T) {
	a := &entity.Entity{ID: 1, HP: 1000, Stats: domain.Stats{MinAtk: 999, MaxAtk: 999, CritRate: 10000}}
	d := &entity.Entity{ID: 2, HP: 1000, Stats: domain.Stats{PhysResist: 40}}
	got := CalculateStrike(a, d, &HitTracker{}, Blow{GuaranteedHit: true, BaseDamageFlat: 999, NativeDamageSnapshot: &NativeDamageSnapshot{Amount: 250, Critical: true}}, nil)
	if got.Amount != 210 {
		t.Fatalf("snapshot repeated offense/crit: %+v", got)
	}
}

func TestLegendaryAttackBaseUsesNativeFractionOnce(t *testing.T) {
	a := &entity.Entity{ID: 1, HP: 1000, Stats: domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}}
	d := &entity.Entity{ID: 2, HP: 10000, Stats: domain.Stats{PhysResist: 10, MagicResist: 10, MDef: 100}}
	for _, tc := range []struct {
		magic, crit bool
		want        int32
	}{{false, false, 215}, {false, true, 430}, {true, false, 152}, {true, true, 304}} {
		b := Blow{Magic: tc.magic, GuaranteedHit: true, GuaranteedCrit: tc.crit, AttackBaseFlat: 50, SkillPctNumerator: 150, SkillPctDenominator: 1}
		got := CalculateStrike(a, d, &HitTracker{}, b, nil)
		if got.Amount != tc.want {
			t.Fatalf("magic=%v crit=%v got%d want%d", tc.magic, tc.crit, got.Amount, tc.want)
		}
	}
	// Flat native replacement bases remain independent of the attack coefficient.
	b := Blow{GuaranteedHit: true, AttackBaseFlat: 50, BaseDamageFlat: 70, SkillPctNumerator: 0, SkillPctDenominator: 1}
	if got := CalculateStrike(a, d, &HitTracker{}, b, nil); got.Amount != 60 {
		t.Fatalf("zero native coefficient inherited attack base: %+v", got)
	}
}
