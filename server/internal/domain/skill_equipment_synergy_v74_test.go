package domain

import "testing"

func TestEquipmentTrainingSynergyConfigurationBounds(t *testing.T) {
	for _, bp := range []int32{299, 300, 400, 500, 600, 601} {
		in := skillProgressionInput(t)
		changed := false
		for i := range in.Synergies {
			if in.Synergies[i].RankMode == "hard" && in.Synergies[i].Effect == "damage" {
				in.Synergies[i].RankMode = "effective_training"
				in.Synergies[i].PerPointBP = bp
				changed = true
				break
			}
		}
		if !changed {
			t.Fatal("no baseline training edge")
		}
		_, err := NewSkillProgressionCatalog(in)
		if (err == nil) != (bp >= 300 && bp <= 600) {
			t.Fatal(bp, err)
		}
	}
	in := skillProgressionInput(t)
	for i := range in.Synergies {
		if in.Synergies[i].Effect == "heal_flat" {
			in.Synergies[i].RankMode = "effective_training"
			in.Synergies[i].PerPointBP = 300
			break
		}
	}
	if _, err := NewSkillProgressionCatalog(in); err == nil {
		t.Fatal("flat points mislabeled as3%")
	}
	e := SkillSynergy{Target: 10402, Source: 10401, RankMode: "effective_training", Effect: "damage", PerPointBP: 300}
	if !e.IsTraining() || e.SourceRank(Learned{10401: 3}, Learned{10401: 25}) != 25 || e.SourceRank(Learned{10401: 3}, nil) != 0 {
		t.Fatal("raw rank resolution")
	}
	e.RankMode = "hard"
	if e.SourceRank(Learned{10401: 3}, Learned{10401: 25}) != 3 {
		t.Fatal("legacy mode changed")
	}
}
