package domain

import (
	"math"
	"testing"
)

func TestEffectContestRatioUnboundedRatingsAndExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		base        int32
		hit, resist int64
		want        float64
	}{
		{3000, 0, 0, .3}, {10000, 10000 - 4000, 10000 - 4000, 1}, {10000, 0, 10000 - 4000, .625}, {3000, -5000, -5000, .3}, {3000, 10000, 10000, .3}, {3000, 20000, 10000, .45},
		{10000, 0, 10000, .5}, {10000, 0, 30000, .25}, {10000, 100000, 0, 1},
		{0, 100000, 0, 0}, {1, 0, 1_000_000_000, 1.0 / 10000 / 100001},
	} {
		got := EffectSuccessProbability(tc.base, tc.hit, tc.resist)
		if math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("%+v: got %.15g", tc, got)
		}
	}
	if EffectiveStatusChanceBP(0, false) != 10000 || EffectiveStatusChanceBP(-1, false) != 10000 || EffectiveStatusChanceBP(0, true) != 0 {
		t.Fatal("legacy sentinel confused with explicit zero")
	}
	for r := int64(0); r < 100000; r += 100 {
		if EffectSuccessProbability(3000, 20000, r+100) >= EffectSuccessProbability(3000, 20000, r) {
			t.Fatal("resistance stopped growing", r)
		}
	}
}

func TestPercentMitigationTotalCapAfterVulnerability(t *testing.T) {
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 1, DurationSec: 20, PhysicalDamageTakenPct: 200, PhysicalDamageReductionPct: 90}, 0, 0)
	base := s.DamageTakenAmount(false, 1000)
	if base != 3000 {
		t.Fatal(base)
	}
	if got := MitigatePercentDamageV7(base, 7500, 0, 4500, s.DamageReductionPct(false), 2, false); got != 750 {
		t.Fatal("all reductions must retain 25% of amplified damage", got)
	}
	if got, _ := s.MitigateDamage(false, 1000); got != 750 {
		t.Fatal("standalone status mitigation", got)
	}
	s.Apply(StatusDef{ID: 2, DurationSec: 20, Invulnerable: true}, 0, 0)
	if got, _ := s.MitigateDamage(false, 1000); got != 0 {
		t.Fatal("true immunity nerfed", got)
	}
	var empty *StatusSet
	if got := empty.DamageTakenAmount(false, 800); got != 800 {
		t.Fatal(got)
	}
	if got := MitigatePercentDamageV7(1000, -5000, 0, 4500, 90, 2, false); got != 375 {
		t.Fatal("negative resistance amplification baseline", got)
	}
}

func TestPercentMitigationWideVulnerability(t *testing.T) {
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 1, DurationSec: 20, PhysicalDamageTakenPct: 200, PhysicalDamageReductionPct: 75}, 0, 0)
	if got, _ := s.MitigateDamage(false, math.MaxInt32); got != 1610612736 {
		t.Fatal("premature int32 saturation", got)
	}
	s.Apply(StatusDef{ID: 2, DurationSec: 20, PhysicalDamageTakenPct: math.MaxInt32}, 0, 0)
	if got, _ := s.MitigateDamage(false, math.MaxInt32); got != math.MaxInt32 {
		t.Fatal("amplification overflow", got)
	}
}

func TestFlatReductionsShareFinalMitigationBudget(t *testing.T) {
	for _, amount := range []int64{1, 2, 3, 4, 5, 1000} {
		if got := MitigatePercentDamageV7(amount, 0, 0, 0, 0, 0, false, 999999); got != (amount+3)/4 {
			t.Fatalf("amount=%d got=%d", amount, got)
		}
	}
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 1, DurationSec: 30, MagicDamageReductionFlat: 1000000}, 0, 0)
	if got, _ := s.MitigateDamage(true, 100); got != 25 {
		t.Fatal("flat reduction bypassed cap", got)
	}
}
