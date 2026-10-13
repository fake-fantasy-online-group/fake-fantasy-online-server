package domain

import "testing"

func TestLegacyPassiveOverflowCarriersRemoved(t *testing.T) {
	table := realV5CurveAnchors(t)
	for _, id := range V5SkillIDs(testSkillProgressionCatalog()) {
		for _, rank := range []int32{6, 20, 50, 100} {
			got, err := EvaluateV5Skill(table[SkillKey{id, 1}], table[SkillKey{id, 5}], rank)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range got.PassiveModifiers {
				if m.ProgressionOverflow != 0 {
					t.Fatal(id, rank, m)
				}
			}
			for _, m := range got.PassiveResists {
				if m.ProgressionRestoreMP != 0 {
					t.Fatal(id, rank, m)
				}
			}
			for _, m := range got.PassiveTriggers {
				if m.ManaCostReductionRanks != 0 {
					t.Fatal(id, rank, m)
				}
			}
			for _, m := range got.PassiveTriggerEffects {
				if m.ManaCostReductionRanks != 0 {
					t.Fatal(id, rank, m)
				}
			}
			for _, m := range got.PassiveStatusAffixes {
				if m.ManaCostReductionRanks != 0 {
					t.Fatal(id, rank, m)
				}
			}
		}
	}
}

func TestDedicatedResistanceAndVulnerabilityContinuePast100(t *testing.T) {
	table := realV5CurveAnchors(t)
	for _, id := range []SkillID{10008, 10025, 10026, 10108} {
		a, err := EvaluateV5Skill(table[SkillKey{id, 1}], table[SkillKey{id, 5}], 20)
		if err != nil {
			t.Fatal(err)
		}
		b, err := EvaluateV5Skill(table[SkillKey{id, 1}], table[SkillKey{id, 5}], 21)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range a.PassiveResists {
			if v.Pct <= 100 || b.PassiveResists[i].Pct <= v.Pct {
				t.Fatal(id, a, b)
			}
		}
	}
	a, err := EvaluateV5Skill(table[SkillKey{10429, 1}], table[SkillKey{10429, 5}], 50)
	if err != nil {
		t.Fatal(err)
	}
	if a.PassiveModifiers[0].Value <= 100 {
		t.Fatal("frozen vulnerability", a)
	}
}
