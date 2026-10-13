package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func skillProgressionInput(t *testing.T) SkillProgressionData {
	t.Helper()
	raw, err := os.ReadFile("../testfixture/catalog/skill_progression.json")
	if err != nil {
		t.Fatal(err)
	}
	var in SkillProgressionData
	if err = json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	return in
}
func TestSkillProgressionCatalogRejectsIncompleteOrInvalidConfiguration(t *testing.T) {
	tests := map[string]func(*SkillProgressionData){
		"unknown native effect": func(in *SkillProgressionData) {
			for i, e := range in.Synergies {
				if e.RankMode == "effective_raw" {
					in.Synergies[i].Effect = "unsupported"
					break
				}
			}
		},
		"invented native coefficient": func(in *SkillProgressionData) {
			for i, e := range in.Synergies {
				if e.RankMode == "effective_raw" {
					in.Synergies[i].PerPointBP = 1
					break
				}
			}
		},
		"duplicate edge differing coefficient": func(in *SkillProgressionData) {
			for _, e := range in.Synergies {
				if e.RankMode == "hard" && e.Effect == "damage" {
					e.PerPointBP++
					in.Synergies = append(in.Synergies, e)
					in.ExpectedSynergies++
					break
				}
			}
		},
		"missing membership": func(in *SkillProgressionData) {
			id := in.Identities[0].ID
			in.Identities = in.Identities[1:]
			delete(in.Policies, id)
		},
		"missing policy":       func(in *SkillProgressionData) { delete(in.Policies, in.Identities[0].ID) },
		"missing synergy":      func(in *SkillProgressionData) { in.Synergies = in.Synergies[1:] },
		"missing manifest":     func(in *SkillProgressionData) { in.ExpectedSkills = 0 },
		"duplicate identity":   func(in *SkillProgressionData) { in.Identities[1] = in.Identities[0] },
		"missing prerequisite": func(in *SkillProgressionData) { in.Identities[0].Prerequisites = []SkillID{9999999} },
		"cyclic prerequisite": func(in *SkillProgressionData) {
			a, b := in.Identities[0].ID, in.Identities[1].ID
			in.Identities[0].Prerequisites = []SkillID{b}
			in.Identities[1].Prerequisites = []SkillID{a}
		},
		"invalid cap": func(in *SkillProgressionData) {
			id := in.Identities[0].ID
			p := in.Policies[id]
			p.ProtectionCapRank = 4
			in.Policies[id] = p
		},
		"invalid attribute": func(in *SkillProgressionData) {
			id := in.Identities[0].ID
			p := in.Policies[id]
			p.Attribute = 999
			p.AttributeCapRank = 4
			in.Policies[id] = p
		},
		"missing attack target": func(in *SkillProgressionData) {
			id := in.Identities[0].ID
			p := in.Policies[id]
			p.AttackCapRank = 20
			p.AttackTargets = []SkillID{9999999}
			in.Policies[id] = p
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			in := skillProgressionInput(t)
			mutate(&in)
			if _, err := NewSkillProgressionCatalog(in); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestSkillProgressionSnapshotIsImmutableAndNotSerialized(t *testing.T) {
	in := skillProgressionInput(t)
	c, err := NewSkillProgressionCatalog(in)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range in.Identities {
		if len(d.Prerequisites) > 0 {
			before, _ := c.Identity(d.ID)
			in.Identities[i].Prerequisites[0] = 999
			got, _ := c.Identity(d.ID)
			got.Prerequisites[0] = 888
			after, _ := c.Identity(d.ID)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("identity alias")
			}
			break
		}
	}
	for id, p := range in.Policies {
		if len(p.AttackTargets) > 0 {
			before, _ := c.Policy(id)
			p.AttackTargets[0] = 999
			got, _ := c.Policy(id)
			got.AttackTargets[0] = 888
			after, _ := c.Policy(id)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("policy alias")
			}
			break
		}
	}
	before := c.Synergies()
	in.Synergies[0].Effect = "bad"
	got := c.Synergies()
	got[0].Effect = "bad"
	if !reflect.DeepEqual(before, c.Synergies()) {
		t.Fatal("synergy alias")
	}
	original := []StatusApplication{{ID: 10009, Level: 1}}
	table := SkillTable{{ID: 10009, Level: 1}: {ID: 10009, Level: 1, Statuses: original}}
	AttachSkillProgression(table, c)
	d := table[SkillKey{ID: 10009, Level: 1}]
	if table.ProgressionCatalog() != c || d.Progression != c || d.Statuses[0].Progression != c || original[0].Progression != nil {
		t.Fatal("attachment/cloning failed")
	}
	encoded, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "Progression\"") {
		t.Fatal("runtime pointer leaked into serialization")
	}
	first := SkillDef{ID: 10018, Level: 1}
	fifth := first
	fifth.Level = 5
	if _, err := EvaluateV5Skill(first, fifth, 20); err == nil {
		t.Fatal("missing catalog silently synthesized")
	}
}
