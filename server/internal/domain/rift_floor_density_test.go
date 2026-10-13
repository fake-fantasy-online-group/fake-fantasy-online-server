package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func assertDoubledRiftFloor(t *testing.T, base, doubled RiftFloorPlan) {
	t.Helper()
	if !base.Valid() || !doubled.Valid() || doubled.GeneratorVersion != 3 || len(doubled.Spawns) != 2*len(base.Spawns) {
		t.Fatalf("bad density: base=%d doubled=%d", len(base.Spawns), len(doubled.Spawns))
	}
	if base.Seed != doubled.Seed || base.MapID != doubled.MapID || base.Entry != doubled.Entry || base.Exit != doubled.Exit || base.Guardian != doubled.Guardian || !reflect.DeepEqual(base.Spawns, doubled.Spawns[:len(base.Spawns)]) {
		t.Fatal("v2 seed, base actors or guardian changed")
	}
	before, after := [3]int{}, [3]int{}
	for _, s := range base.Spawns {
		before[s.Class]++
	}
	for _, s := range doubled.Spawns {
		after[s.Class]++
	}
	for i := range before {
		if after[i] != 2*before[i] {
			t.Fatal("class not exactly doubled", before, after)
		}
	}
	for i, s := range base.Spawns {
		twin := doubled.Spawns[len(base.Spawns)+i]
		if twin.Class != s.Class || twin.Point.Monster != s.Point.Monster {
			t.Fatal("counterpart composition changed")
		}
	}
}

func TestRiftFloorDoublesCurrentCompositionAndPreservesBase(t *testing.T) {
	for _, area := range []float64{2, 128, 10000} {
		for _, floor := range []int32{1, 2, 150, 1000} {
			layout := testRiftFloorTemplate(7)
			layout.WalkableArea = area * RiftFloorAreaPerMonster
			base, err := PlanRiftFloor("density-76", floor, []RiftFloorTemplate{layout}, 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			doubled, err := PlanRiftFloor("density-76", floor, []RiftFloorTemplate{layout}, 0, 2)
			if err != nil {
				t.Fatal(err)
			}
			assertDoubledRiftFloor(t, base, doubled)
			raw, _ := json.Marshal(doubled)
			var saved RiftFloorPlan
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			again, _ := json.Marshal(saved)
			if !saved.Valid() || string(raw) != string(again) {
				t.Fatal("saved plan is not byte-identical after reload")
			}
			for i, j := 0, len(layout.Population)-1; i < j; i, j = i+1, j-1 {
				layout.Population[i], layout.Population[j] = layout.Population[j], layout.Population[i]
			}
			reordered, err := PlanRiftFloor("density-76", floor, []RiftFloorTemplate{layout}, 0, 2)
			if err != nil || !reflect.DeepEqual(doubled, reordered) {
				t.Fatal("density expansion depends on candidate order", err)
			}
		}
	}
}

func TestRiftFloorDoubleDensityFallsBackBeforeAdmission(t *testing.T) {
	crowded := testRiftFloorTemplate(1)
	crowded.Population = nil
	usable := testRiftFloorTemplate(2)
	for floor := int32(1); floor <= 100; floor++ {
		if _, err := PlanRiftFloor("crowded", floor, []RiftFloorTemplate{crowded}, 0, 2); err == nil {
			t.Fatal("silently clamped doubled request")
		}
		p, err := PlanRiftFloor("crowded", floor, []RiftFloorTemplate{crowded, usable}, 0, 2)
		if err != nil || p.MapID != 2 || len(p.Spawns) < 192 || !p.Valid() {
			t.Fatal("deterministic fallback failed", err)
		}
	}
	for _, density := range []int32{-1, 3, 100000} {
		if _, err := PlanRiftFloor("invalid", 1, []RiftFloorTemplate{usable}, 0, density); err == nil {
			t.Fatal("invalid multiplier accepted")
		}
	}
}

func TestRiftDensityRulesPinOldRunsAndNewRuns(t *testing.T) {
	base := *riftRulesFixture()
	doubled := cloneRiftRules(base)
	doubled.Version = 2
	doubled.FloorPopulationMultiplier = 2
	table, err := NewRiftRuleTable(2, []RiftRules{base, doubled}, lootRulesTableFixture(), balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int32{1, 2} {
		rules, _ := table.Version(version)
		run, err := NewRiftRun(fmt.Sprint("density-", version), 1, 1, []TrialKeyMember{{CharID: 1, Level: 60}}, 1000, rules)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(run)
		var restored RiftRun
		if err = json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		if err = restored.BindRules(table); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(run.Rules, restored.Rules) || restored.Rules.FloorPopulationMultiplier != rules.FloorPopulationMultiplier {
			t.Fatal("reload rebound density to active rules")
		}
	}
	legacy := &RiftRun{RulesVersion: 0}
	if err = legacy.BindRules(table); err != nil || legacy.Rules.FloorPopulationMultiplier != 0 {
		t.Fatal("missing legacy density did not retain original rules", err)
	}
	if table.Active().FloorPopulationMultiplier != 2 {
		t.Fatal("new admissions did not select doubled density")
	}
}

func BenchmarkRiftFloorDensity(b *testing.B) {
	for _, density := range []int32{1, 2} {
		b.Run(fmt.Sprintf("x%d", density), func(b *testing.B) {
			layout := testRiftFloorTemplate(7)
			layout.WalkableArea = 1000 * RiftFloorAreaPerMonster
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := PlanRiftFloor("density-benchmark", 1, []RiftFloorTemplate{layout}, 0, density); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
