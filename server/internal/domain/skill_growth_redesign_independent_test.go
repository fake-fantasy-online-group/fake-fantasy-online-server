package domain

import (
	"math"
	"testing"
)

// Review-authored edge cases: tiny hits must use a ceiling so a nominal 75%
// ceiling cannot become 100% through integer rounding. Shields and categorical
// invulnerability remain permitted to absorb the remaining damage.
func TestRedesignIndependentAllReductionBudgetAndShieldBoundary(t *testing.T) {
	for _, magic := range []bool{false, true} {
		for damage := int32(1); damage <= 101; damage++ {
			statuses := NewStatusSet()
			statuses.Apply(StatusDef{ID: 77001, Level: 1, DurationSec: 30,
				PhysicalDamageReductionPct: 100, MagicDamageReductionPct: 100,
				MagicDamageReductionFlat: math.MaxInt32}, 0, 1)
			want := (damage + 3) / 4
			got, _ := statuses.MitigateDamage(magic, damage)
			if got != want {
				t.Fatalf("magic=%t hit=%d final=%d, want ceiling quarter=%d", magic, damage, got, want)
			}
			statuses.Apply(StatusDef{ID: 77002, Level: 1, DurationSec: 30, DamageShield: want}, 0, 1)
			got, removed := statuses.MitigateDamage(magic, damage)
			if got != 0 || len(removed) != 1 || removed[0] != 77002 {
				t.Fatalf("shield must consume remaining quarter: %d %v", got, removed)
			}
		}
	}
	statuses := NewStatusSet()
	statuses.Apply(StatusDef{ID: 77003, Level: 1, DurationSec: 30, Invulnerable: true}, 0, 1)
	if got, _ := statuses.MitigateDamage(true, math.MaxInt32); got != 0 {
		t.Fatal("true immunity treated as mitigation", got)
	}
}

func TestRedesignIndependentVulnerabilityStackSafeBeyondHundred(t *testing.T) {
	statuses := NewStatusSet()
	statuses.Apply(StatusDef{ID: 77001, Level: 1, DurationSec: 30, PhysicalDamageTakenPct: 200, MagicDamageTakenPct: 200}, 0, 1)
	if got := statuses.DamageTakenAmount(false, 100); got != 300 {
		t.Fatal("200% vulnerability capped", got)
	}
	statuses.Apply(StatusDef{ID: 77002, Level: 1, DurationSec: 30, PhysicalDamageTakenPct: math.MaxInt32, MagicDamageTakenPct: math.MaxInt32}, 0, 1)
	amount := statuses.DamageTakenAmount(true, math.MaxInt32)
	if amount <= math.MaxInt32 {
		t.Fatal("wide amplification wrapped or saturated before mitigation", amount)
	}
	for _, pct := range []int32{0, 25, 75, 100, math.MaxInt32} {
		got := MitigatePercentDamageV7(amount, 7500, 0, 4500, pct, 2, false)
		if got < (amount+3)/4 || got > amount {
			t.Fatal("wide budget invalid", amount, pct, got)
		}
	}
}

func TestRedesignIndependentEffectContestHasNoHiddenRatingCap(t *testing.T) {
	cases := []struct {
		base        int32
		hit, resist int64
		want        float64
	}{
		{1300, 0, 0, .13}, {1300, 10000, 10000, .13}, {5000, 0, 10000, .25},
		{5000, 20000, 10000, .75}, {5000, 20000, 20000, .5}, {5000, 0, 20000, 1.0 / 6},
		{10000, 0, 1000000, 1.0 / 101}, {0, math.MaxInt32, 0, 0},
		{10000, -10000, -10000, 1}, {10000, math.MaxInt32, math.MaxInt32, 1},
	}
	for _, c := range cases {
		got := EffectSuccessProbability(c.base, c.hit, c.resist)
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%+v probability=%v", c, got)
		}
	}
	if EffectiveStatusChanceBP(0, false) != 10000 || EffectiveStatusChanceBP(0, true) != 0 {
		t.Fatal("legacy omission and explicit zero were merged")
	}
}

func TestRedesignIndependentEfficiencyKeepsAbsentCostsAbsent(t *testing.T) {
	// Original zero means the channel is absent, not a hidden value to grow.
	for _, id := range []SkillID{10018, 10227, 10301, 10305, 10309, 10432} {
		first := SkillDef{ID: id, Kind: SkillSingle, Level: 1, Progression: testSkillProgressionCatalog()}
		fifth := first
		fifth.Level = 5
		for _, rank := range []int32{6, 20, 100, 1000000} {
			got, err := EvaluateV5Skill(first, fifth, rank)
			if err != nil {
				t.Fatal(err)
			}
			FinalizeSkillManaCost(&got)
			if got.MPCost != 0 || got.CooldownMS != 0 || got.PrepareMS != 0 {
				t.Fatal("zero cost/timing invented", id, rank, got.MPCost, got.CooldownMS, got.PrepareMS)
			}
		}
	}
}
