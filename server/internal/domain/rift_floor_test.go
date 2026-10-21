package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func testRiftFloorTemplate(mapID int32) RiftFloorTemplate {
	t := RiftFloorTemplate{MapID: mapID, MapName: "测试原生地图", Monsters: []MonsterID{9, 3, 6}, Guardians: []MonsterID{12, 11}, WalkableArea: 128 * RiftFloorAreaPerMonster}
	for i := 0; i < 100; i++ {
		t.Anchors = append(t.Anchors, SpawnPoint{ID: int32(i + 1), Monster: 3, Pos: Pos{MapID: mapID, X: float64(i%10)*160 + 100, Y: float64(i/10)*160 + 100}, Dir: 67})
	}
	for i := 0; i < 1600; i++ {
		t.Population = append(t.Population, SpawnPoint{ID: -int32(i + 1), Pos: Pos{MapID: mapID, X: float64(i%40)*RiftFloorPopulationSpacing + 32, Y: float64(i/40)*RiftFloorPopulationSpacing + 32}, Dir: 67})
	}
	return t
}
func TestRiftFloorDeterministicCanonicalPlan(t *testing.T) {
	pool := []RiftFloorTemplate{testRiftFloorTemplate(7), testRiftFloorTemplate(12), testRiftFloorTemplate(4)}
	a, err := PlanRiftFloor("frozen-run", 3, pool, 7)
	if err != nil {
		t.Fatal(err)
	}
	for i := range pool {
		for j, k := 0, len(pool[i].Anchors)-1; j < k; j, k = j+1, k-1 {
			pool[i].Anchors[j], pool[i].Anchors[k] = pool[i].Anchors[k], pool[i].Anchors[j]
		}
		for j, k := 0, len(pool[i].Population)-1; j < k; j, k = j+1, k-1 {
			pool[i].Population[j], pool[i].Population[k] = pool[i].Population[k], pool[i].Population[j]
		}
		pool[i].Monsters = []MonsterID{6, 9, 3}
		pool[i].Guardians = []MonsterID{11, 12}
	}
	pool[0], pool[2] = pool[2], pool[0]
	b, err := PlanRiftFloor("frozen-run", 3, pool, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("input iteration order changed seeded plan")
	}
	if a.MapID == 7 || !a.Valid() || a.GeneratorVersion != riftFloorBaseGeneratorVersion || a.FloorIndex != 3 {
		t.Fatalf("invalid plan %+v", a)
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var restored RiftFloorPlan
	if json.Unmarshal(data, &restored) != nil || !reflect.DeepEqual(a, restored) {
		t.Fatal("plan was not losslessly persisted")
	}
}
func TestRiftFloorSeedGolden(t *testing.T) {
	const expected uint64 = 10881727492265092799
	if got := RiftFloorSeed("frozen-run", 3); got != expected {
		t.Fatalf("seed %d; want %d", got, expected)
	}
}
func TestRiftFloorPopulationVariesAndUsesOnlySafeAnchors(t *testing.T) {
	pool := []RiftFloorTemplate{testRiftFloorTemplate(7), testRiftFloorTemplate(12), testRiftFloorTemplate(4)}
	counts := map[int]bool{}
	maps := map[int32]bool{}
	last := int32(0)
	for floor := int32(1); floor <= 100; floor++ {
		p, err := PlanRiftFloor("varied-run", floor, pool, last)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Valid() || p.MapID == last {
			t.Fatal("bad or repeated floor")
		}
		if len(p.Spawns) < RiftFloorMinPopulation || len(p.Spawns) > RiftFloorMaxPopulation {
			t.Fatalf("population failed density bounds: %d", len(p.Spawns))
		}
		last = p.MapID
		maps[p.MapID] = true
		counts[len(p.Spawns)] = true
		seen := map[int32]bool{}
		classes := map[int32]int{}
		for _, s := range p.Spawns {
			if seen[s.Point.ID] {
				t.Fatal("overlapping native spawn anchor")
			}
			seen[s.Point.ID] = true
			classes[s.Class]++
			if riftFloorDistance(s.Point.Pos, p.Entry) < RiftFloorSafeRadius*RiftFloorSafeRadius || riftFloorDistance(s.Point.Pos, p.Exit) < 80*80 {
				t.Fatal("unsafe spawn")
			}
			if s.Point.Monster != 3 && s.Point.Monster != 6 && s.Point.Monster != 9 {
				t.Fatal("invented asset")
			}
		}
		if classes[0] == 0 || classes[1] < 6 || classes[2] < 2 || p.Guardian.Class != 3 {
			t.Fatal(classes)
		}
		if (classes[1] != 6 && classes[1] != 9 && classes[1] != 12) || classes[2] > 4 {
			t.Fatalf("changed elite population rules: %v", classes)
		}
		nearby := 0
		for i, s := range p.Spawns {
			neighbors := 0
			for j, other := range p.Spawns {
				if i != j && riftFloorDistance(s.Point.Pos, other.Point.Pos) <= 2*RiftFloorPackRadius*2*RiftFloorPackRadius {
					neighbors++
				}
			}
			if neighbors >= 3 {
				nearby++
			}
		}
		if nearby*10 < len(p.Spawns)*9 {
			t.Fatalf("isolated population rather than dense packs: %d/%d", nearby, len(p.Spawns))
		}
	}
	if len(maps) != 3 || len(counts) < 10 {
		t.Fatalf("insufficient seeded variation maps=%d counts=%d", len(maps), len(counts))
	}
}

func TestRiftFloorDensityScalesWithAreaWithoutProgressCap(t *testing.T) {
	small, large := testRiftFloorTemplate(7), testRiftFloorTemplate(7)
	small.WalkableArea = 2 * RiftFloorAreaPerMonster
	large.WalkableArea = 10000 * RiftFloorAreaPerMonster
	var crossedProgressTarget bool
	for floor := int32(1); floor <= 100; floor++ {
		a, err := PlanRiftFloor("density-area", floor, []RiftFloorTemplate{small}, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := PlanRiftFloor("density-area", floor, []RiftFloorTemplate{large}, 0)
		if err != nil || len(b.Spawns) <= len(a.Spawns) {
			t.Fatal("larger playable area did not raise population", err)
		}
		progress := int32(0)
		for _, s := range b.Spawns {
			switch s.Class {
			case 0:
				progress += RiftNormalProgress
			case 1:
				progress += RiftGlobeProgress
			case 2:
				progress += 4 * RiftGlobeProgress
			}
		}
		crossedProgressTarget = crossedProgressTarget || progress >= RiftProgressMax
	}
	if !crossedProgressTarget {
		t.Fatal("density must not be reduced to impose a mandatory floor count")
	}
}

func TestRiftFloorSavedVersionOnePlanRemainsValid(t *testing.T) {
	p, err := PlanRiftFloor("legacy-saved", 1, []RiftFloorTemplate{testRiftFloorTemplate(7)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.GeneratorVersion, p.Seed, p.Spawns = 1, 5406192697100663770, p.Spawns[:64]
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored RiftFloorPlan
	if json.Unmarshal(encoded, &restored) != nil || !restored.Valid() || !reflect.DeepEqual(p, restored) {
		t.Fatal("saved v1 plan changed or became invalid")
	}
	restored.Spawns = append(restored.Spawns, p.Spawns[0])
	if restored.Valid() {
		t.Fatal("v1 population limit was silently changed")
	}
}

func TestRiftFloorDensePlanRejectsUnsafeSavedPositions(t *testing.T) {
	for _, bad := range []string{"duplicate", "entry", "exit"} {
		t.Run(bad, func(t *testing.T) {
			p, err := PlanRiftFloor("invalid-dense", 1, []RiftFloorTemplate{testRiftFloorTemplate(7)}, 0)
			if err != nil {
				t.Fatal(err)
			}
			switch bad {
			case "duplicate":
				p.Spawns[0].Point.Pos = p.Spawns[1].Point.Pos
			case "entry":
				p.Spawns[0].Point.Pos = p.Entry
			case "exit":
				p.Spawns[0].Point.Pos = p.Exit
			}
			if p.Valid() {
				t.Fatal("unsafe dense plan accepted")
			}
		})
	}
}
func TestRiftFloorInvalidInputsFailClosed(t *testing.T) {
	for _, test := range []struct {
		run   string
		floor int32
		pool  []RiftFloorTemplate
	}{{"", 1, []RiftFloorTemplate{testRiftFloorTemplate(1)}}, {"r", 0, []RiftFloorTemplate{testRiftFloorTemplate(1)}}, {"r", 1, nil}, {"r", 1, []RiftFloorTemplate{{MapID: 1}}}} {
		if _, err := PlanRiftFloor(test.run, test.floor, test.pool, 0); err == nil {
			t.Fatal("accepted invalid floor")
		}
	}
	p, err := PlanRiftFloor("solo-map", 1, []RiftFloorTemplate{testRiftFloorTemplate(7)}, 7)
	if err != nil || p.MapID != 7 {
		t.Fatal("single validated map must still be usable")
	}
	p.GeneratorVersion = RiftFloorCoverageVersion + 1
	if p.Valid() {
		t.Fatal("accepted unknown generation version")
	}
}

func TestRiftFloorRetriesDenseEntryClustersDeterministically(t *testing.T) {
	layout := RiftFloorTemplate{MapID: 7, Monsters: []MonsterID{3}, Guardians: []MonsterID{3}}
	// A dense local cluster cannot itself be used as the entry. Spread out
	// points down the corridor remain usable for every seed.
	for i := 0; i < 18; i++ {
		layout.Anchors = append(layout.Anchors, SpawnPoint{ID: int32(i + 1), Pos: Pos{MapID: 7, X: 100 + float64(i), Y: 100}})
	}
	for i := 0; i < 10; i++ {
		layout.Anchors = append(layout.Anchors, SpawnPoint{ID: int32(i + 100), Pos: Pos{MapID: 7, X: 400 + float64(i)*100, Y: 100}})
	}
	for floor := int32(1); floor <= 1000; floor++ {
		plan, err := PlanRiftFloor("dense-entry", floor, []RiftFloorTemplate{layout}, 0)
		if err != nil || !plan.Valid() {
			t.Fatal("validated native cluster intermittently failed generation", floor, err)
		}
	}
}
