package domain

import (
	"math"
	"testing"
)

func TestV5RankScopesNoUnlockAndSourceDedup(t *testing.T) {
	bonuses := []SkillRankBonus{{SourceID: "u:all", Scope: "all", Levels: 1}, {SourceID: "u:flame", Scope: "school", School: "flame", Levels: 2}, {SourceID: "u:single", Scope: "single", SkillID: 10402, Levels: 1}, {SourceID: "u:all", Scope: "all", Levels: 1}}
	hard := Learned{10402: 20, 10401: 1, 11005: 900}
	r, err := ResolveSkillLoadout(5, hard, bonuses, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r[10402].EffectiveRank != "24" || r[10401].EffectiveRank != "4" || r[10408].CanCast || r[10408].EffectiveRank != "0" || r[10408].SoftRank != "3" {
		t.Fatal(r)
	}
	if _, ok := r[11005]; ok {
		t.Fatal("life skill received battle bonus")
	}
	if hard[10402] != 20 {
		t.Fatal("hard map mutated")
	}
	noGear, _ := ResolveSkillLoadout(5, hard, nil, nil)
	if noGear[10402].EffectiveRank != "20" {
		t.Fatal(noGear)
	}
	bonuses[3].Levels = 2
	if _, err := ResolveSkillLoadout(5, hard, bonuses, nil); err == nil {
		t.Fatal("conflicting repeated source accepted")
	}
	for _, b := range []SkillRankBonus{{Scope: "single", SkillID: 11005, Levels: 1}, {Scope: "branch", Profession: 0, Branch: "aux", Levels: 1}, {Scope: "all", Levels: -1}} {
		if ValidateSkillRankBonuses([]SkillRankBonus{b}) == nil {
			t.Fatal(b)
		}
	}
}
func TestV5RankIntegralUnboundedRepresentation(t *testing.T) {
	bonuses := []SkillRankBonus{{SourceID: "a", Scope: "all", Levels: math.MaxInt64}, {SourceID: "b", Scope: "all", Levels: math.MaxInt64}}
	r, err := ResolveSkillLoadout(5, Learned{10401: 20}, bonuses, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r[10401].EffectiveRank != "18446744073709551634" {
		t.Fatal(r[10401])
	}
	if _, err := r[10401].EngineRank(); err == nil {
		t.Fatal("engine representation silently capped")
	}
	for _, rank := range []int64{20, 21, 99, 100, 101, 255, 256, 65535, 65536} {
		r, err := ResolveSkillLoadout(5, Learned{10401: 1}, []SkillRankBonus{{SourceID: "one", Scope: "single", SkillID: 10401, Levels: rank}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		n, err := r[10401].EngineRank()
		if err != nil || int64(n) != rank+1 {
			t.Fatal(rank, r, err)
		}
	}
}
func TestV5GrantCannotSupplyHardPrerequisite(t *testing.T) {
	r, err := ResolveSkillLoadout(5, Learned{}, []SkillRankBonus{{SourceID: "s", Scope: "all", Levels: 10}}, []SkillGrant{{SourceID: "g", SkillID: 10401, Rank: 1, Origin: "treasure_grant"}})
	if err != nil || r[10401].EffectiveRank != "1" || r[10401].HardRank != 0 {
		t.Fatal(r, err)
	}
	if err := ValidateSkillAllocation(100, 5, Learned{10402: 1}, 98); err == nil {
		t.Fatal("grant/soft prerequisite accepted")
	}
}
func TestV5AllocationBudgetsAndLearningWindows(t *testing.T) {
	for _, l := range []int32{1, 10, 60, 80, 99, 100, 101} {
		want := max(int32(0), min(l, 100)-1)
		if SkillPointBudget(l) != want {
			t.Fatal(l)
		}
	}
	if SkillHardRankLimit(80, 10413) != 1 || SkillHardRankLimit(99, 10413) != 20 || SkillHardRankLimit(100, 10413) != 20 {
		t.Fatal("80 unlock pacing")
	}
	if err := ValidateSkillAllocation(100, 5, Learned{10401: 20, 10402: 20}, 59); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		h Learned
		p int32
	}{{Learned{10401: 21}, 78}, {Learned{10401: 20}, 80}, {Learned{10002: 1}, 98}, {Learned{10401: 1}, -1}} {
		if ValidateSkillAllocation(100, 5, tc.h, tc.p) == nil {
			t.Fatal(tc)
		}
	}
	c := Character{Skills: Learned{SkillBeginnerBolt: 3, SkillAlchemy: 900}, SkillPoints: 6, SkillPointsKnown: true, SkillAllocationVersion: 5}
	c.RemoveBeginnerSkills()
	c.RemoveBeginnerSkills()
	if c.SkillPoints != 9 || c.Skills[SkillAlchemy] != 900 {
		t.Fatal(c)
	}
}
func TestV5SynergySourceCapAndCycles(t *testing.T) {
	if err := ValidateSkillSynergies(V5SkillSynergies); err != nil {
		t.Fatal(err)
	}
	graph := []SkillSynergy{{10401, 10402, "hard", "damage", 100}, {10402, 10401, "hard", "damage", 100}}
	if err := ValidateSkillSynergies(graph); err != nil {
		t.Fatal("nonrecursive bilateral rejected", err)
	}
	for _, s := range []SkillID{10424, 10426, 10408} {
		graph = append(graph, SkillSynergy{10401, s, "effective_raw", "damage", 100})
	}
	if ValidateSkillSynergies(graph) == nil {
		t.Fatal("four-source graph accepted")
	}
	if ValidateSkillSynergies([]SkillSynergy{{10401, 10401, "hard", "damage", 100}}) == nil {
		t.Fatal("self edge accepted")
	}
}
