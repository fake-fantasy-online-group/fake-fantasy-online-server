package domain

import (
	"math"
	"reflect"
	"testing"
)

func TestChallengeMonsterBehaviorPreservesNativeAndAppliesOnce(t *testing.T) {
	for _, speed := range []int32{0, 1, 77, 100, math.MaxInt32} {
		native := MonsterDef{ID: 7, Kind: MonsterNormal, Stats: Stats{MoveSpeed: speed, MinAtk: 23}, AI: MonsterAIProfile{ID: "caster", ViewDist: 400, TraceDist: 600, BasicAttackDist: 75, HelpRadius: 123}, NoBasicAttack: true, Skills: []MonsterSkillRule{{Skill: SkillDef{ID: 123}, InitialDelay: 7}}, ImmuneSkills: map[SkillID]struct{}{123: {}}}
		got := ChallengeMonsterBehavior(native)
		if got.Stats.MoveSpeed != SaturateInt32(int64(speed)*3/2) || got.TraceDist != 1800 || got.AI.TraceDist != 1800 {
			t.Fatal("wrong challenge scaling", got)
		}
		if got.AI.ViewDist != 400 || got.AI.BasicAttackDist != 75 || got.AI.HelpRadius != 123 || got.Stats.MinAtk != 23 || !got.NoBasicAttack || !reflect.DeepEqual(got.Skills, native.Skills) || !reflect.DeepEqual(got.ImmuneSkills, native.ImmuneSkills) {
			t.Fatal("native behavior changed", got)
		}
		if !reflect.DeepEqual(got, ChallengeMonsterBehavior(got)) {
			t.Fatal("challenge multiplier compounded")
		}
		if native.Stats.MoveSpeed != speed || native.AI.TraceDist != 600 {
			t.Fatal("shared source mutated")
		}
	}
}
func TestChallengeMonsterTraceUsesEffectiveFallback(t *testing.T) {
	for _, tc := range []struct {
		ai          MonsterAIProfile
		trace, want int32
	}{
		{MonsterAIProfile{}, 700, 2100}, {MonsterAIProfile{}, 0, 1800},
		{MonsterAIProfile{ID: "native", TraceDist: 900}, 700, 2700},
		{MonsterAIProfile{ID: "native"}, 700, 1800},
	} {
		d := ChallengeMonsterBehavior(MonsterDef{Kind: MonsterNormal, AI: tc.ai, TraceDist: tc.trace})
		if d.TraceDist != tc.want || d.AI.TraceDist != tc.want {
			t.Fatal(tc, d)
		}
	}
}
