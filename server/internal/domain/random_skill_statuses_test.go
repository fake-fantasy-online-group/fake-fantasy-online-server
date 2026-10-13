package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRandomStatusGroupValidation(t *testing.T) {
	base := []StatusApplication{
		{ID: 1006, Level: 1, ChanceBP: 4000, ChanceExplicit: true, RandomGroup: 1, RandomWeight: 1},
		{ID: 1008, Level: 2, ChanceBP: 4000, ChanceExplicit: true, RandomGroup: 1, RandomWeight: 3},
	}
	for _, tc := range []struct {
		name  string
		edit  func([]StatusApplication) []StatusApplication
		valid bool
	}{
		{"weighted", func(a []StatusApplication) []StatusApplication { return a }, true},
		{"single remaining candidate", func(a []StatusApplication) []StatusApplication { return a[:1] }, true},
		{"ordinary alongside", func(a []StatusApplication) []StatusApplication { return append(a, StatusApplication{ID: 1014}) }, true},
		{"zero chance", func(a []StatusApplication) []StatusApplication { a[0].ChanceBP, a[1].ChanceBP = 0, 0; return a }, true},
		{"negative group", func(a []StatusApplication) []StatusApplication { a[0].RandomGroup = -1; return a }, false},
		{"zero weight", func(a []StatusApplication) []StatusApplication { a[0].RandomWeight = 0; return a }, false},
		{"ungrouped weight", func(a []StatusApplication) []StatusApplication { a[0].RandomGroup = 0; return a }, false},
		{"implicit chance", func(a []StatusApplication) []StatusApplication { a[0].ChanceExplicit = false; return a }, false},
		{"invalid chance", func(a []StatusApplication) []StatusApplication { a[0].ChanceBP = 10001; return a }, false},
		{"mixed chance", func(a []StatusApplication) []StatusApplication { a[0].ChanceBP = 3000; return a }, false},
		{"mixed scope", func(a []StatusApplication) []StatusApplication { a[0].TargetType = SkillStatusTargetPlayer; return a }, false},
		{"duplicate ID", func(a []StatusApplication) []StatusApplication { a[0].ID = a[1].ID; return a }, false},
		{"ambiguous persistent identity", func(a []StatusApplication) []StatusApplication { return append(a, StatusApplication{ID: 1006}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRandomStatusGroups(tc.edit(append([]StatusApplication(nil), base...)))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
	data, _ := json.Marshal(StatusApplication{ID: 1006})
	if strings.Contains(string(data), "Random") {
		t.Fatal("zero grouping changed historical serialized payload")
	}
}

func TestRandomStatusGrowthKeepsPoolIdentity(t *testing.T) {
	table := realV5CurveAnchors(t)
	first, fifth := table[SkillKey{ID: 10313, Level: 1}], table[SkillKey{ID: 10313, Level: 5}]
	first.Statuses = []StatusApplication{{ID: 1006, Level: 1, ChanceBP: 4000, ChanceExplicit: true, RandomGroup: 1, RandomWeight: 1, DurationSec: 6}}
	fifth.Statuses = []StatusApplication{{ID: 1006, Level: 5, ChanceBP: 4000, ChanceExplicit: true, RandomGroup: 1, RandomWeight: 1, DurationSec: 10}}
	for _, rank := range []int32{6, 10, 11, 20, 100, 1000} {
		got, err := EvaluateV5Skill(first, fifth, rank)
		if err != nil || len(got.Statuses) != 1 {
			t.Fatal(rank, err)
		}
		app := got.Statuses[0]
		if app.RandomGroup != 1 || app.RandomWeight != 1 || app.ChanceBP != 4000 || app.ProgressionRanks != rank-5 {
			t.Fatal(rank, app)
		}
		app.RandomWeight = 7
		if fifth.Statuses[0].RandomWeight != 1 {
			t.Fatal("resolved pool aliases original")
		}
	}
	fifth.Statuses[0].RandomWeight = 2
	if _, err := EvaluateV5Skill(first, fifth, 6); err == nil {
		t.Fatal("mismatched pool weights accepted")
	}
	fifth.Statuses[0].RandomWeight, fifth.Statuses[0].RandomGroup = 1, 2
	if _, err := EvaluateV5Skill(first, fifth, 6); err == nil {
		t.Fatal("mismatched pool groups accepted")
	}
}
