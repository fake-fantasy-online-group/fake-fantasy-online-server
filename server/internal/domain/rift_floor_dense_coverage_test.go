package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRiftDenseCoveragePopulationGeometryAndFrozenPlans(t *testing.T) {
	for _, side := range []int{22, 50, 70, 88} {
		layout := coverageFixture(1, side)
		for floor := int32(1); floor <= 20; floor++ {
			old, e := PlanRiftCoverageFloor("density-150", floor, []RiftFloorTemplate{layout}, 0)
			if e != nil {
				t.Fatal(e)
			}
			plan, e := PlanRiftDenseCoverageFloor("density-150", floor, []RiftFloorTemplate{layout}, 0)
			if e != nil {
				t.Fatal(e)
			}
			want := min(320, (len(old.Spawns)*3+1)/2)
			if len(plan.Spawns) != want || plan.GeneratorVersion != 5 || !plan.Valid() {
				t.Fatalf("side=%d old=%d new=%d want=%d", side, len(old.Spawns), len(plan.Spawns), want)
			}
			if plan.MapID != old.MapID || plan.Entry != old.Entry || plan.Exit != old.Exit || plan.Guardian.Point.Pos != old.Guardian.Point.Pos {
				t.Fatal("density changed geometry")
			}
			classes := func(p RiftFloorPlan) [3]int {
				var c [3]int
				for _, s := range p.Spawns {
					c[s.Class]++
				}
				return c
			}
			a, b := classes(old), classes(plan)
			if a[1] != b[1] || a[2] != b[2] || b[0]-a[0] != want-len(old.Spawns) {
				t.Fatal("added elites or lost ordinary actors")
			}
			for i, s := range old.Spawns {
				if s.Point.Pos != plan.Spawns[i].Point.Pos {
					t.Fatal("moved existing encounter point")
				}
			}
			again, e := PlanRiftCoverageFloor("density-150", floor, []RiftFloorTemplate{layout}, 0)
			if e != nil || !reflect.DeepEqual(old, again) {
				t.Fatal("changed old run")
			}
			for _, p := range []RiftFloorPlan{old, plan} {
				raw, _ := json.Marshal(p)
				var saved RiftFloorPlan
				if json.Unmarshal(raw, &saved) != nil || !saved.Valid() || !reflect.DeepEqual(p, saved) {
					t.Fatal("saved plan changed")
				}
			}
		}
	}
}
func TestRiftDenseCoverageRequiresNewFrozenRuleVersion(t *testing.T) {
	r := eliteRulesFixture(t)
	r.EquipmentPools = []RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []MonsterID{1011}, Items: []ItemID{1234}}}
	r.Version = 8
	r.FloorLayoutVersion = 5
	r.GuardianRecentWindow = 3
	if r.Validate() == nil {
		t.Fatal("v8 accepted v5 generation")
	}
	r.Version = 9
	if e := r.Validate(); e != nil {
		t.Fatal(e)
	}
	r.FloorLayoutVersion = 4
	if e := r.Validate(); e != nil {
		t.Fatal(e)
	}
}
