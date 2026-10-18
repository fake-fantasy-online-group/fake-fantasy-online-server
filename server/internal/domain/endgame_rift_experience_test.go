package domain

import (
	"math"
	"math/big"
	"testing"
)

func TestRiftBossExperienceTierScaling(t *testing.T) {
	rules := &TrialRules{MinLevel: 12, MaxLevel: 100, AnchorLevel: 50, AnchorUnits: 16}
	curve := map[int32]int64{}
	for level := int32(1); level <= 100; level++ {
		curve[level] = int64(level)*int64(level)*int64(level)*100003 + 7
	}
	levels := NewLevelTable(curve)
	for _, level := range []int32{12, 30, 50, 60, 61, 80, 99, 100} {
		base := rules.BaseExperience(level, levels)
		for _, tier := range []int32{1, 10, 150} {
			want := base * (10 + int64(tier)) / 10
			if got := RiftBossExperience(base, tier, riftRulesFixture()); got != want {
				t.Fatalf("level %d tier %d: %d != %d", level, tier, got, want)
			}
		}
	}
	if got := RiftBossExperience(19, 150, riftRulesFixture()); got != 304 {
		t.Fatalf("early rounding: %d", got)
	}
	for _, base := range []int64{0, -1, 1, 9, 10, 19, math.MaxInt64 / 15, math.MaxInt64} {
		for _, tier := range []int32{1, 10, 150} {
			want := new(big.Int).Mul(big.NewInt(base), big.NewInt(10+int64(tier)))
			want.Quo(want, big.NewInt(10))
			if base <= 0 {
				want.SetInt64(0)
			}
			if !want.IsInt64() {
				want.SetInt64(math.MaxInt64)
			}
			if got := RiftBossExperience(base, tier, riftRulesFixture()); got != want.Int64() {
				t.Fatalf("overflow %d/%d: %d != %s", base, tier, got, want)
			}
		}
	}
	for _, tier := range []int32{-1, 0, 151} {
		if RiftBossExperience(100, tier, riftRulesFixture()) != 0 {
			t.Fatal("invalid tier")
		}
	}
}

func TestRiftBossExperienceRejectsInvalidDivisor(t *testing.T) {
	for _, divisor := range []int32{0, -1} {
		rule := *riftRulesFixture()
		rule.ExperienceDivisor = divisor
		if got := RiftBossExperience(100, 1, &rule); got != 0 {
			t.Fatalf("invalid divisor %d produced %d", divisor, got)
		}
	}
	if RiftBossExperience(100, 1, nil) != 0 {
		t.Fatal("missing rules produced experience")
	}
}
