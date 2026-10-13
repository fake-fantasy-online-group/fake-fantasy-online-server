package domain

import (
	"math"
	"testing"
)

func v5ProgressionTestCurve(cap int32) map[int32]int64 {
	m := map[int32]int64{}
	for l := int32(1); l <= cap; l++ {
		m[l] = int64(l) * 100
	}
	return m
}
func TestV5ProgressionEdgesAndBudget(t *testing.T) {
	table := NewLevelTable(v5ProgressionTestCurve(LevelCap))
	for _, from := range []int32{1, 59, 60, 79, 80, 98, 99} {
		c := newChar(Warrior, from)
		c.Base = c.BaseAtLevel(from)
		before := c.Base
		gain := c.AddExp(table, table.Need(from), LevelCap)
		growth := c.ApplyLevelUps(gain.FromLevel, gain.ToLevel)
		if gain.Reason != "" || c.Level != from+1 || c.Exp != 0 || c.Base != before.Add(GrowthAt(c.Race, from+1)) || growth.SkillPoints != 1 || growth.FreePoints != 2 {
			t.Fatalf("edge %d: %+v %+v", from, gain, c)
		}
	}
	c := newChar(Warrior, 1)
	var sum int64
	for l := int32(1); l < LevelCap; l++ {
		sum += table.Need(l)
	}
	g := c.AddExp(table, sum+17, LevelCap)
	c.ApplyLevelUps(g.FromLevel, g.ToLevel)
	if c.Level != 100 || c.SkillPoints != 99 || c.FreePoints != 198 || c.Exp != 0 || g.DiscardedExp != 17 {
		t.Fatalf("budget %+v %+v", c, g)
	}
	g = c.AddExp(table, 500, LevelCap)
	if c.Level != 100 || g.AppliedExp != 0 || g.DiscardedExp != 500 {
		t.Fatal(g, c)
	}
}
func TestV5InvalidExperienceAtomic(t *testing.T) {
	for _, tc := range []struct {
		need           map[int32]int64
		progress, gain int64
	}{{map[int32]int64{1: 10, 3: 30}, 0, 40}, {map[int32]int64{1: 10}, math.MaxInt64 - 2, 3}, {map[int32]int64{1: -1}, 0, 10}} {
		c := newChar(Warrior, 1)
		c.Exp = tc.progress
		g := c.AddExp(NewLevelTable(tc.need), tc.gain, 100)
		if g.Reason == "" || c.Level != 1 || c.Exp != tc.progress {
			t.Fatal("mutated invalid gain", g, c)
		}
	}
	for _, mutate := range []func(map[int32]int64){func(m map[int32]int64) { delete(m, 61) }, func(m map[int32]int64) { m[61] = -1 }, func(m map[int32]int64) { m[61] = 1 }} {
		m := v5ProgressionTestCurve(100)
		mutate(m)
		if ValidateExperienceCurve(m, 100) == nil {
			t.Fatal("bad curve accepted")
		}
	}
	player, pet := NewLevelTable(v5ProgressionTestCurve(100)), NewPetLevelTable(v5ProgressionTestCurve(80))
	if _, err := NewProgressionRules(player, pet, experienceCurveHash(player.Need, 100), experienceCurveHash(pet.Need, 80)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProgressionRules(player, pet, "wrong", experienceCurveHash(pet.Need, 80)); err == nil {
		t.Fatal("checksum ignored")
	}
}
func TestV5PetCapGrowthAndOverflow(t *testing.T) {
	table := NewPetLevelTable(v5ProgressionTestCurve(80))
	def := 海龟
	prefix := PetPrefixDef{GrowthEven: Base{STR: 2}, FreePointsPerLevel: 2}
	p := PetInstance{Level: 79, Base: def.BaseAt(79)}
	before := p.Base
	g := p.AddExpWithPrefix(def, prefix, table, table.Need(79)+20, PetCapFor(def, 100))
	if g.Reason != "" || p.Level != 80 || p.Exp != 0 || p.Base != before.Add(def.GrowthAt(80)).Add(prefix.GrowthAt(80)) || p.FreePoints != 2 {
		t.Fatal(g, p)
	}
	if g = p.AddExpWithPrefix(def, prefix, table, math.MaxInt64, PetCapFor(def, 100)); g.AppliedExp != 0 || p.Level != 80 {
		t.Fatal(g, p)
	}
	for _, owner := range []int32{60, 61, 79, 80, 81, 99, 100} {
		want := owner
		if want > 80 {
			want = 80
		}
		if got := PetCapFor(def, owner); got != want {
			t.Fatal(owner, got)
		}
	}
	if got := PetExperienceItemCap(def, 60, 2); got != 62 {
		t.Fatal(got)
	}
	if got := PetExperienceItemCap(def, math.MaxInt32, math.MaxInt32); got != 80 {
		t.Fatal(got)
	}
	def.MaxLevel = 30
	if got := PetExperienceItemCap(def, 60, 20); got != 30 {
		t.Fatal(got)
	}
}
func TestV5PetShareNumericOrder(t *testing.T) {
	if got, err := PetRewardExperience(101, 50); err != nil || got != 75 {
		t.Fatal(got, err)
	}
	if got, err := PetRewardExperience(101, -3); err != nil || got != 50 {
		t.Fatal(got, err)
	}
	if got := SharedExp(math.MaxInt64); got != math.MaxInt64/2 {
		t.Fatal(got)
	}
	if _, err := ExperienceWithBonus(math.MaxInt64, 100); err == nil {
		t.Fatal("overflow accepted")
	}
}
func TestV5TrialRewardNormalizedRounds(t *testing.T) {
	r := &TrialRules{MinLevel: 12, MaxLevel: 100, AnchorLevel: 50, AnchorUnits: 16, FirstPct: 300, Multipliers: []int32{100, 110, 120, 130, 140, 200}}
	table := NewLevelTable(v5ProgressionTestCurve(100))
	for _, lv := range []int32{61, 80, 81, 99, 100} {
		want := (table.Need(lv) + 8) / 16
		if got := r.BaseExperience(lv, table); got != want {
			t.Fatal(lv, got, want)
		}
		if got := r.Experience(lv, lv, TrialProgress{}, 1, table); got != (table.Need(lv)*3+8)/16 {
			t.Fatal("first", lv, got)
		}
	}
	// Old <=60 line is unchanged; no cap-dependent interpolation.
	n, d := r.baseFraction(60, table)
	want := (n + d/2) / d
	if got := r.BaseExperience(60, table); got != want {
		t.Fatal(got, want)
	}
	if RiftMaxTier != 150 {
		t.Fatal("tier confused with player level")
	}
}
