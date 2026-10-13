package domain

import "testing"

func TestMonsterFamilyPredicateAndClone(t *testing.T) {
	d := SkillDef{TargetFamilies: []MonsterFamily{MonsterFamilyUndead, MonsterFamilyDemon}}
	for _, f := range []MonsterFamily{"", MonsterFamilyUnknown, MonsterFamilyOther, "invalid", MonsterFamilyUndead, MonsterFamilyDemon} {
		want := f == MonsterFamilyUndead || f == MonsterFamilyDemon
		if d.MatchesMonsterFamily(f) != want {
			t.Fatalf("family %q want %v", f, want)
		}
		if !(SkillDef{}).MatchesMonsterFamily(f) {
			t.Fatal("unrestricted existing skill changed")
		}
	}
	clone := cloneOriginalSkill(d)
	clone.TargetFamilies[0] = MonsterFamilyOther
	if d.TargetFamilies[0] != MonsterFamilyUndead {
		t.Fatal("family allow-list aliases original skill")
	}
}
