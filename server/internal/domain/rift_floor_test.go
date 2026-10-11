package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func testRiftFloorTemplate(mapID int32) RiftFloorTemplate {
	t := RiftFloorTemplate{MapID: mapID, MapName: "测试原生地图", Monsters: []MonsterID{9, 3, 6}, Guardians: []MonsterID{12, 11}}
	for i := 0; i < 100; i++ {
		t.Anchors = append(t.Anchors, SpawnPoint{ID: int32(i + 1), Monster: 3, Pos: Pos{MapID: mapID, X: float64(i%10)*160 + 100, Y: float64(i/10)*160 + 100}, Dir: 67})
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
	if a.MapID == 7 || !a.Valid() || a.GeneratorVersion != RiftFloorGeneratorVersion || a.FloorIndex != 3 {
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
	const expected uint64 = 5406192697100663770
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
	}
	if len(maps) != 3 || len(counts) < 10 {
		t.Fatalf("insufficient seeded variation maps=%d counts=%d", len(maps), len(counts))
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
	p.GeneratorVersion++
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
