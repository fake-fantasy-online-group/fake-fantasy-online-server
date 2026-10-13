package domain

import (
	"math"
	"math/big"
	"testing"
)

func TestApplySkillHealingBPExactOverflowBoundary(t *testing.T) {
	// These are the actual original 10301 healing slopes. Both ranks at the
	// int64 multiplication boundary remain valid int32 healing curve outputs.
	for _, rank := range []int64{257, 65536, 1000000, 35791225, 35791226, 40000000, 70000000} {
		e := SkillEffect{Kind: EffectHeal, HealFlat: int32(30*rank + 70), HealFlatNumerator: 120*rank + 280, HealFlatDenominator: 4,
			HealPctOfMax: int32(5*rank + 5), HealPctNumerator: 20*rank + 20, HealPctDenominator: 4}
		flat, pct := big.NewRat(e.HealFlatNumerator, 4), big.NewRat(e.HealPctNumerator, 4)
		factor := big.NewRat(10000+math.MaxInt32, 10000)
		flat.Mul(flat, factor)
		pct.Mul(pct, factor)
		ApplySkillHealingBP(&e, math.MaxInt32)
		if big.NewRat(e.HealFlatNumerator, e.HealFlatDenominator).Cmp(flat) != 0 || big.NewRat(e.HealPctNumerator, e.HealPctDenominator).Cmp(pct) != 0 {
			t.Fatalf("rank %d lost exact healing: %+v", rank, e)
		}
		if e.HealFlat < 0 || e.HealPctOfMax < 0 || skillHealAmount(e, 1000) != math.MaxInt32 {
			t.Fatalf("rank %d wrapped or bypassed resource saturation: %+v", rank, e)
		}
	}
}

func TestApplySkillHealingBPPreservesSmallFractions(t *testing.T) {
	e := SkillEffect{Kind: EffectHeal, HealFlat: 1, HealFlatNumerator: 101, HealFlatDenominator: 100,
		HealPctOfMax: 0, HealPctNumerator: 1, HealPctDenominator: 100}
	ApplySkillHealingBP(&e, 300)
	if e.HealFlatNumerator != 10403 || e.HealFlatDenominator != 10000 || e.HealPctNumerator != 103 || e.HealPctDenominator != 10000 {
		t.Fatal("small percentage benefit was rounded early", e)
	}
	if got := skillHealAmount(e, 10000); got != 2 {
		t.Fatal("flat and percent channels must combine before resource rounding", got)
	}
	original := SkillEffect{Kind: EffectHeal, HealFlat: 100, HealPctOfMax: 10}
	ApplySkillHealingBP(&original, 300)
	if original.HealFlat != 103 || original.HealPctOfMax != 10 || skillHealAmount(original, 1000) != 206 {
		t.Fatal("legacy integer payload was not seeded exactly", original)
	}
}

func TestApplySkillHealingBPStorageSaturation(t *testing.T) {
	e := SkillEffect{Kind: EffectHeal, HealFlat: math.MaxInt32, HealFlatNumerator: math.MaxInt64, HealFlatDenominator: 1}
	ApplySkillHealingBP(&e, math.MaxInt32)
	if e.HealFlat != math.MaxInt32 || e.HealFlatNumerator != math.MaxInt32 || e.HealFlatDenominator != 1 || skillHealAmount(e, 1) != math.MaxInt32 {
		t.Fatal("unrepresentable rational must saturate without wrapping", e)
	}
	ApplySkillHealingBP(nil, 300)
	unchanged := SkillEffect{Kind: EffectHeal, HealFlat: 17}
	ApplySkillHealingBP(&unchanged, 0)
	if unchanged.HealFlat != 17 || unchanged.HealFlatDenominator != 0 {
		t.Fatal("zero modifier changed payload", unchanged)
	}
}
