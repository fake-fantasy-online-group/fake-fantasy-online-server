package domain

import (
	"math"
	"testing"
)

func TestSkillManaCostIntegerFloorAndSingleApplication(t *testing.T) {
	for _, base := range []int32{0, 1, 2, 64, 729, math.MaxInt32} {
		for _, ranks := range []int32{0, 1, 15, 16, 95, 1000000, math.MaxInt32} {
			d := SkillDef{MPCost: base, ManaCostReductionRanks: ranks}
			FinalizeSkillManaCost(&d)
			want := base
			if base > 0 && ranks > 0 {
				want = int32((int64(base)*50 + 49 + int64(ranks)) / (50 + int64(ranks)))
			}
			if d.MPCost != want || d.MPCost < 0 || d.MPCost > base || base > 0 && d.MPCost < 1 {
				t.Fatal(base, ranks, d.MPCost, want)
			}
			once := d
			FinalizeSkillManaCost(&d)
			if d.MPCost != once.MPCost {
				t.Fatal("repeated finalization reduced cost twice")
			}
		}
	}
}

func TestSkillManaCostIntegerPlateauIsHonest(t *testing.T) {
	a := SkillDef{MPCost: 64, ManaCostReductionRanks: 15}
	b := SkillDef{MPCost: 64, ManaCostReductionRanks: 16}
	FinalizeSkillManaCost(&a)
	FinalizeSkillManaCost(&b)
	if a.MPCost != 50 || b.MPCost != 49 {
		t.Fatal(a.MPCost, b.MPCost)
	}
	a = SkillDef{MPCost: 1, ManaCostReductionRanks: 15}
	b = SkillDef{MPCost: 1, ManaCostReductionRanks: 1000000}
	FinalizeSkillManaCost(&a)
	FinalizeSkillManaCost(&b)
	if a.MPCost != 1 || b.MPCost != 1 {
		t.Fatal("fabricated growth below one mana")
	}
}
