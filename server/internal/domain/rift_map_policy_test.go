package domain

import "testing"

func TestRiftExcludedMapsNeverSelected(t *testing.T) {
	excluded := []int32{60, 21000, 23800, 23900, 24000, 24100, 24200, 24300, 20100, 28502, 28503, 28504, 28505, 24600, 24700, 24800, 36700, 933, 934, 935, 936, 937}
	var bad []RiftFloorTemplate
	for _, id := range excluded {
		if !RiftMapExcluded(id) {
			t.Fatalf("missing exclusion %d", id)
		}
		bad = append(bad, coverageFixture(id, 40))
	}
	good := coverageFixture(7, 40)
	planners := map[string]func([]RiftFloorTemplate) (RiftFloorPlan, error){
		"base": func(p []RiftFloorTemplate) (RiftFloorPlan, error) { return planRiftFloorBase("excluded", 2, p, 23800) },
		"v2":   func(p []RiftFloorTemplate) (RiftFloorPlan, error) { return PlanRiftFloor("excluded", 2, p, 23800, 1) },
		"v3":   func(p []RiftFloorTemplate) (RiftFloorPlan, error) { return PlanRiftFloor("excluded", 2, p, 23800, 2) },
		"v4": func(p []RiftFloorTemplate) (RiftFloorPlan, error) {
			return PlanRiftCoverageFloor("excluded", 2, p, 23800)
		},
		"v5": func(p []RiftFloorTemplate) (RiftFloorPlan, error) {
			return PlanRiftDenseCoverageFloor("excluded", 2, p, 23800)
		},
	}
	for name, plan := range planners {
		t.Run(name, func(t *testing.T) {
			if _, err := plan(bad); err == nil {
				t.Fatal("excluded-only pool admitted")
			}
			p, err := plan(append(append([]RiftFloorTemplate(nil), bad...), good))
			if err != nil || p.MapID != 7 {
				t.Fatalf("safe pool failed: %d %v", p.MapID, err)
			}
		})
	}
	for _, id := range []int32{7, 932, 938, 23700, 24400} {
		if RiftMapExcluded(id) {
			t.Fatalf("ordinary adjacent map wrongly removed %d", id)
		}
	}
}
