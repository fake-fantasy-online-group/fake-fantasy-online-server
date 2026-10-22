package domain

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These fixtures freeze the unchanged a0ede0a v4/v5 implementation. Keep
// complete persisted bytes, not just population counts, stable for saved runs.
func TestRiftLegacyCoveragePlanByteSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name     string
		version  int32
		side     int
		floor    int32
		previous int32
	}{
		{"v4-small", 4, 22, 1, 0},
		{"v5-small", 5, 22, 1, 0},
		{"v4-medium", 4, 50, 7, 1},
		{"v5-medium", 5, 50, 7, 1},
		{"v4-large", 4, 88, 19, 2},
		{"v5-large", 5, 88, 19, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := []RiftFloorTemplate{coverageFixture(2, tc.side), coverageFixture(1, tc.side)}
			planner := PlanRiftCoverageFloor
			if tc.version == 5 {
				planner = PlanRiftDenseCoverageFloor
			}
			plan, err := planner("legacy-byte-snapshot", tc.floor, pool, tc.previous)
			if err != nil || !plan.Valid() || plan.GeneratorVersion != tc.version {
				t.Fatalf("invalid legacy plan: %v", err)
			}
			raw, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "rift_floor_legacy_"+tc.name+".json"))
			if err != nil || !bytes.Equal(raw, want) {
				t.Fatalf("legacy persisted bytes changed or missing fixture: %v", err)
			}
			var restored RiftFloorPlan
			if err := json.Unmarshal(raw, &restored); err != nil || !restored.Valid() || !reflect.DeepEqual(plan, restored) {
				t.Fatalf("saved legacy plan changed: %v", err)
			}
			roundTrip, err := json.Marshal(restored)
			if err != nil || !bytes.Equal(raw, roundTrip) {
				t.Fatalf("saved legacy bytes changed on round trip: %v", err)
			}
			for i := range pool {
				p := pool[i].CombatRegions[0].Population
				for j, k := 0, len(p)-1; j < k; j, k = j+1, k-1 {
					p[j], p[k] = p[k], p[j]
				}
				pool[i].Monsters = []MonsterID{6, 9, 3}
				pool[i].Guardians = []MonsterID{11, 12}
			}
			pool[0], pool[1] = pool[1], pool[0]
			again, err := planner("legacy-byte-snapshot", tc.floor, pool, tc.previous)
			if err != nil {
				t.Fatal(err)
			}
			reordered, err := json.Marshal(again)
			if err != nil || !bytes.Equal(raw, reordered) {
				t.Fatalf("catalog iteration order changed persisted legacy bytes: %v", err)
			}
		})
	}
}
