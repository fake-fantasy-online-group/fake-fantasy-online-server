package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func coverageFixture(id int32, side int) RiftFloorTemplate {
	t := testRiftFloorTemplate(id)
	r := RiftCombatRegion{}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			r.Population = append(r.Population, SpawnPoint{ID: -int32(len(r.Population) + 1), Pos: Pos{MapID: id, X: float64(32 + x*64), Y: float64(32 + y*64)}})
		}
	}
	r.CombatArea = float64(side * side * 64 * 64)
	t.CombatRegions = []RiftCombatRegion{r}
	return t
}
func TestRiftCoverageDensityAndBudget(t *testing.T) {
	small, large := coverageFixture(1, 22), coverageFixture(2, 88)
	for floor := int32(1); floor <= 100; floor++ {
		a, err := PlanRiftCoverageFloor("coverage", floor, []RiftFloorTemplate{small}, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := PlanRiftCoverageFloor("coverage", floor, []RiftFloorTemplate{large}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(a.Spawns) != 24 || len(b.Spawns) < 300 || len(b.Spawns) > 320 {
			t.Fatalf("area-based populations %d,%d", len(a.Spawns), len(b.Spawns))
		}
		for _, p := range []RiftFloorPlan{a, b} {
			if !p.Valid() || p.GeneratorVersion != 4 {
				t.Fatal("invalid plan")
			}
			blue, gold := 0, 0
			for i, s := range p.Spawns {
				if s.Class == 1 {
					blue++
				}
				if s.Class == 2 {
					gold++
				}
				for _, other := range p.Spawns[:i] {
					if riftFloorDistance(s.Point.Pos, other.Point.Pos) < 64*64 {
						t.Fatal("crowded lattice")
					}
				}
			}
			if blue > 24 || gold > 8 {
				t.Fatal("elite wire budget exceeded")
			}
		}
	}
}
func TestRiftCoverageDeterminismPersistenceAndLegacy(t *testing.T) {
	a, b := coverageFixture(1, 40), coverageFixture(2, 40)
	before, _ := PlanRiftFloor("legacy", 1, []RiftFloorTemplate{a, b}, 0, 2)
	plan, err := PlanRiftCoverageFloor("coverage", 5, []RiftFloorTemplate{a, b}, 1)
	if err != nil || plan.MapID != 2 {
		t.Fatal("adjacent map repeated", err)
	}
	for i, j := 0, len(b.CombatRegions[0].Population)-1; i < j; i, j = i+1, j-1 {
		b.CombatRegions[0].Population[i], b.CombatRegions[0].Population[j] = b.CombatRegions[0].Population[j], b.CombatRegions[0].Population[i]
	}
	again, err := PlanRiftCoverageFloor("coverage", 5, []RiftFloorTemplate{b, a}, 1)
	if err != nil || !reflect.DeepEqual(plan, again) {
		t.Fatal("input order changed plan", err)
	}
	data, _ := json.Marshal(plan)
	var restored RiftFloorPlan
	if json.Unmarshal(data, &restored) != nil || !reflect.DeepEqual(plan, restored) || !restored.Valid() {
		t.Fatal("persisted v4 plan changed")
	}
	after, _ := PlanRiftFloor("legacy", 1, []RiftFloorTemplate{a, b}, 0, 2)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("v4 metadata perturbed legacy generation")
	}
}
func TestRiftCoverageRejectsUnverifiedRegions(t *testing.T) {
	layout := coverageFixture(1, 20)
	layout.CombatRegions[0].CombatArea = RiftCoverageMaxArea + 1
	if _, err := PlanRiftCoverageFloor("bad", 1, []RiftFloorTemplate{layout}, 0); err == nil {
		t.Fatal("accepted unbounded region")
	}
	layout.CombatRegions = nil
	if _, err := PlanRiftCoverageFloor("bad", 1, []RiftFloorTemplate{layout}, 0); err == nil {
		t.Fatal("invented coverage on legacy anchors")
	}
}
func TestRiftCoverageCentersCoverRooms(t *testing.T) {
	layout := coverageFixture(1, 64)
	p, err := PlanRiftCoverageFloor("spread", 1, []RiftFloorTemplate{layout}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Every 512px room-sized bin receives an encounter or lies near the safe entry/exit.
	for y := 256.; y < 4096; y += 512 {
		for x := 256.; x < 4096; x += 512 {
			at := Pos{MapID: 1, X: x, Y: y}
			best := 1e100
			for _, s := range p.Spawns {
				if d := riftFloorDistance(at, s.Point.Pos); d < best {
					best = d
				}
			}
			if best > 600*600 {
				t.Fatalf("empty combat room at %.0f,%.0f nearest squared %.0f", x, y, best)
			}
		}
	}
}

func TestRiftCoverageRuleVersionAndWireBudget(t *testing.T) {
	rules := eliteRulesFixture(t)
	old, _ := json.Marshal(rules)
	rules.EquipmentPools = []RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []MonsterID{1011}, Items: []ItemID{1234}}}
	rules.Version = 8
	rules.FloorLayoutVersion = 4
	rules.GuardianRecentWindow = 3
	if err := rules.Validate(); err != nil {
		t.Fatal(err)
	}
	// The new policy does not change existing timing/progress/reward/skill fields.
	rules.EquipmentPools = nil
	rules.Version = 6
	rules.FloorLayoutVersion = 0
	rules.GuardianRecentWindow = 0
	restored, _ := json.Marshal(rules)
	if string(old) != string(restored) {
		t.Fatal("legacy rule snapshot changed")
	}
	for _, mutate := range []func(*RiftRules){
		func(r *RiftRules) { r.FloorLayoutVersion = 4 },
		func(r *RiftRules) { r.Version = 8; r.FloorLayoutVersion = 3 },
		func(r *RiftRules) { r.Version = 8; r.GuardianRecentWindow = 4 },
		func(r *RiftRules) { r.Version = 8; r.FloorLayoutVersion = 4; r.Elite = nil },
	} {
		r := eliteRulesFixture(t)
		mutate(r)
		if r.Validate() == nil {
			t.Fatal("invalid new rule policy accepted")
		}
	}
}
