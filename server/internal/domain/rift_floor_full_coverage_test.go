package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func fullCoverageFixture(id int32, side int) RiftFloorTemplate {
	layout := coverageFixture(id, side)
	layout.FullCombatRegions = layout.CombatRegions
	return layout
}

func TestRiftFullCoveragePopulationCountsAndSingleDensityMultiplier(t *testing.T) {
	for _, tc := range []struct{ side, want int }{{22, 36}, {50, 153}, {70, 300}, {88, 474}, {224, 3072}} {
		t.Run(fmt.Sprint(tc.side), func(t *testing.T) {
			layout := fullCoverageFixture(1, tc.side)
			plan, err := PlanRiftFullCoverageFloor("uncapped-density", 1, []RiftFloorTemplate{layout}, 0)
			if err != nil || !plan.Valid() || plan.GeneratorVersion != 6 || len(plan.Spawns) != tc.want {
				t.Fatalf("side=%d count=%d want=%d err=%v", tc.side, len(plan.Spawns), tc.want, err)
			}
			var classes [3]int
			for _, s := range plan.Spawns {
				classes[s.Class]++
			}
			if classes[1] > 24 || classes[2] > 8 || plan.Guardian.Class != 3 {
				t.Fatal("changed elite skill budget or guardian")
			}
			if tc.side == 224 && (classes[1] != 24 || classes[2] != 8) {
				t.Fatal("large population lost elite budget")
			}
			t.Logf("side=%d fullCombatArea=%.0f v6=%d ordinary=%d blue=%d gold=%d", tc.side, layout.FullCombatRegions[0].CombatArea, len(plan.Spawns), classes[0], classes[1], classes[2])
		})
	}
}
func TestRiftFullCoverageDeterminismPersistenceAndPreviousMap(t *testing.T) {
	a, b := fullCoverageFixture(1, 88), fullCoverageFixture(2, 88)
	plan, err := PlanRiftFullCoverageFloor("uncapped-persist", 3, []RiftFloorTemplate{a, b}, 1)
	if err != nil || plan.MapID != 2 {
		t.Fatal("repeated previous map", err)
	}
	pop := b.FullCombatRegions[0].Population
	for i, j := 0, len(pop)-1; i < j; i, j = i+1, j-1 {
		pop[i], pop[j] = pop[j], pop[i]
	}
	b.Monsters = []MonsterID{6, 9, 3}
	b.Guardians = []MonsterID{11, 12}
	again, err := PlanRiftFullCoverageFloor("uncapped-persist", 3, []RiftFloorTemplate{b, a}, 1)
	first, _ := json.Marshal(plan)
	second, _ := json.Marshal(again)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("input order changed v6 plan", err)
	}
	var saved RiftFloorPlan
	if err = json.Unmarshal(first, &saved); err != nil || !saved.Valid() || !reflect.DeepEqual(saved, plan) {
		t.Fatal("saved plan changed", err)
	}
	// A changed catalog cannot mutate the admitted complete plan.
	b.FullCombatRegions = nil
	if !saved.Valid() {
		t.Fatal("saved plan depends on current catalog")
	}
}
func TestRiftFullCoverageRequiresFullRegionsAndPreservesExclusions(t *testing.T) {
	layout := coverageFixture(1, 22)
	if _, err := PlanRiftFullCoverageFloor("full-only", 1, []RiftFloorTemplate{layout}, 0); err == nil {
		t.Fatal("silently reused bounded legacy regions")
	}
	for _, id := range []int32{48, 60, 21000, 23800, 23900, 24000, 24100, 24200, 24300, 20100, 28502, 28503, 28504, 28505, 24600, 24700, 24800, 36700, 933, 934, 935, 936, 937} {
		layout = fullCoverageFixture(id, 22)
		if _, err := PlanRiftFullCoverageFloor("excluded", 1, []RiftFloorTemplate{layout}, 0); err == nil {
			t.Fatalf("selected excluded map %d", id)
		}
	}
	for _, mutate := range []func(*RiftCombatRegion){
		func(r *RiftCombatRegion) { r.CombatArea = math.NaN() },
		func(r *RiftCombatRegion) { r.CombatArea = math.Inf(1) },
		func(r *RiftCombatRegion) { r.Population[1].Pos = r.Population[0].Pos },
		func(r *RiftCombatRegion) { r.Population[1].Pos = r.Population[0].Pos; r.Population[1].Pos.X += 63.99 },
		func(r *RiftCombatRegion) { r.Population[0].Pos.MapID = 99 },
	} {
		layout = fullCoverageFixture(1, 22)
		mutate(&layout.FullCombatRegions[0])
		if _, err := PlanRiftFullCoverageFloor("invalid", 1, []RiftFloorTemplate{layout}, 0); err == nil {
			t.Fatal("accepted unsafe region")
		}
	}
}
func TestRiftFullCoverageClipsOnlyToRealSafeCoordinates(t *testing.T) {
	layout := fullCoverageFixture(1, 22)
	region := RiftCombatRegion{CombatArea: math.MaxFloat64}
	for i := 0; i < 40; i++ {
		region.Population = append(region.Population, SpawnPoint{ID: int32(-i - 1), Pos: Pos{MapID: 1, X: float64(32 + 64*i), Y: 32}})
	}
	layout.FullCombatRegions = []RiftCombatRegion{region}
	plan, err := PlanRiftFullCoverageFloor("scarce", 1, []RiftFloorTemplate{layout}, 0)
	if err != nil || !plan.Valid() {
		t.Fatal(err)
	}
	safe := map[Pos]bool{}
	for _, p := range region.Population {
		if riftFloorDistance(p.Pos, plan.Entry) >= 160*160 && riftFloorDistance(p.Pos, plan.Exit) >= 80*80 {
			safe[p.Pos] = true
		}
	}
	if len(plan.Spawns) != len(safe) {
		t.Fatalf("population %d should equal actual safe capacity %d", len(plan.Spawns), len(safe))
	}
	for _, s := range plan.Spawns {
		if !safe[s.Point.Pos] {
			t.Fatal("invented or unsafe coordinate")
		}
	}
}
func TestRiftFullCoverageValidHasNoCountCapAndChecksShape(t *testing.T) {
	plan, err := PlanRiftFullCoverageFloor("validation", 1, []RiftFloorTemplate{fullCoverageFixture(1, 224)}, 0)
	if err != nil || len(plan.Spawns) <= 2048 || !plan.Valid() {
		t.Fatal("large valid plan rejected", err)
	}
	for _, mutate := range []func(*RiftFloorPlan){
		func(p *RiftFloorPlan) { p.Spawns[1].Point.Pos = p.Spawns[0].Point.Pos },
		func(p *RiftFloorPlan) {
			p.Spawns[1].Point.Pos = p.Spawns[0].Point.Pos
			p.Spawns[1].Point.Pos.X += 63.99
		},
		func(p *RiftFloorPlan) { p.Spawns[0].Point.Pos = p.Entry },
		func(p *RiftFloorPlan) { p.Spawns[0].Point.Pos = p.Exit },
		func(p *RiftFloorPlan) { p.Spawns[0].Point.Monster = 0 },
		func(p *RiftFloorPlan) { p.Spawns[0].Class = 3 },
		func(p *RiftFloorPlan) { p.Spawns[0].Point.Pos.X = math.Inf(1) },
		func(p *RiftFloorPlan) { p.Spawns[0].Point.Pos.MapID = 2 },
		func(p *RiftFloorPlan) { p.Guardian.Point.Pos = p.Entry },
		func(p *RiftFloorPlan) {
			for i := 0; i < 25; i++ {
				p.Spawns[i].Class = 1
			}
		},
		func(p *RiftFloorPlan) {
			for i := 0; i < 9; i++ {
				p.Spawns[i].Class = 2
			}
		},
	} {
		bad := plan
		bad.Spawns = append([]RiftFloorSpawn(nil), plan.Spawns...)
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("unsafe persisted plan accepted")
		}
	}
	// Exact lattice spacing and huge finite coordinates remain legal. Integer
	// conversion must not merge unrelated cells or overflow validation.
	for _, offset := range []float64{0, 1e100} {
		index := riftFullSpacingIndex{}
		step := float64(64)
		if offset > 0 {
			step = 1e86
		}
		for i := 0; i < 12; i++ {
			if !index.add(Pos{MapID: 1, X: offset + float64(i)*step, Y: offset}) {
				t.Fatal("valid spacing rejected")
			}
		}
	}
}
func TestRiftFullCoverageSpatialPacksMatchFarthestFirstPolicy(t *testing.T) {
	for _, side := range []int{22, 50, 88} {
		layout := fullCoverageFixture(1, side)
		rng := riftFloorRandom(12456)
		points := shuffledRiftFloorPoints(layout.FullCombatRegions[0].Population, &rng)
		for _, n := range []int{36, 153, 350} {
			a, b := riftFloorRandom(9087), riftFloorRandom(9087)
			old := riftCoveragePackPoints(points, n, &a)
			got := riftFullCoveragePackPoints(points, n, &b)
			if !reflect.DeepEqual(old, got) || a != b {
				t.Fatalf("side=%d n=%d optimized sampling changed exact farthest-first encounters", side, n)
			}
		}
	}
}
func TestRiftFullCoverageReachesEveryRoom(t *testing.T) {
	layout := fullCoverageFixture(1, 224)
	plan, err := PlanRiftFullCoverageFloor("full-rooms", 1, []RiftFloorTemplate{layout}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for y := 256.; y < 224*64; y += 512 {
		for x := 256.; x < 224*64; x += 512 {
			at := Pos{MapID: 1, X: x, Y: y}
			best := math.Inf(1)
			for _, s := range plan.Spawns {
				best = math.Min(best, riftFloorDistance(at, s.Point.Pos))
			}
			if best > 600*600 {
				t.Fatalf("empty room %.0f,%.0f nearest=%.0f", x, y, math.Sqrt(best))
			}
		}
	}
}
func BenchmarkRiftFullCoverageGenerateLarge(b *testing.B) {
	layout := fullCoverageFixture(1, 224)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := PlanRiftFullCoverageFloor("bench", int32(i+1), []RiftFloorTemplate{layout}, 0); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkRiftFullCoverageValidateLarge(b *testing.B) {
	plan, err := PlanRiftFullCoverageFloor("bench", 1, []RiftFloorTemplate{fullCoverageFixture(1, 224)}, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !plan.Valid() {
			b.Fatal("invalid plan")
		}
	}
}

func TestRiftFullCoverageSpacingSupportsSeveralSafePointsPerCell(t *testing.T) {
	index := riftFullSpacingIndex{}
	for _, point := range []Pos{{MapID: 1, X: 0, Y: 0}, {MapID: 1, X: 63, Y: 16}, {MapID: 1, X: 16, Y: 63}} {
		if !index.add(point) {
			t.Fatal("distinct safe points in one bucket rejected")
		}
	}
	if index.add(Pos{MapID: 1, X: 32, Y: 32}) {
		t.Fatal("missed conflict with a second point in the bucket")
	}
}
