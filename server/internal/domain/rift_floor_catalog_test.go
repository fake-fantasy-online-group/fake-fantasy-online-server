package domain

import (
	"encoding/json"
	"os"
	"testing"
)

// Optional broad geometry audit, using the static SQL/MASK export produced by
// tests/rift_layout_audit.py --templates-output. It never requires live accounts
// or a database and does not replace runtime loaded-definition checks.
func TestRiftFloorStaticCatalogDensity(t *testing.T) {
	path := os.Getenv("FFO_RIFT_LAYOUT_AUDIT_TEMPLATES")
	if path == "" {
		t.Skip("set FFO_RIFT_LAYOUT_AUDIT_TEMPLATES to the static audit export")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var templates []RiftFloorTemplate
	if err := json.Unmarshal(encoded, &templates); err != nil || len(templates) == 0 {
		t.Fatal("invalid static catalog export", err)
	}
	min, max, total, plans := RiftFloorMaxConfiguredPopulation, 0, 0, 0
	for _, layout := range templates {
		verified := map[Pos]bool{}
		for _, p := range append(append([]SpawnPoint(nil), layout.Population...), layout.ExtraPopulation...) {
			verified[p.Pos] = true
		}
		for floor := int32(1); floor <= 150; floor++ {
			base, err := PlanRiftFloor("static-density-audit", floor, []RiftFloorTemplate{layout}, 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := PlanRiftFloor("static-density-audit", floor, []RiftFloorTemplate{layout}, 0, 2)
			if err != nil || !plan.Valid() {
				t.Fatalf("map %d floor %d invalid: %v", layout.MapID, floor, err)
			}
			assertDoubledRiftFloor(t, base, plan)
			count := len(plan.Spawns)
			if count < 2*RiftFloorMinPopulation || count > RiftFloorMaxConfiguredPopulation {
				t.Fatalf("map %d floor %d density outside target: %d", layout.MapID, floor, count)
			}
			seen := map[Pos]bool{}
			for _, monster := range plan.Spawns {
				p := monster.Point.Pos
				if !verified[p] || seen[p] || riftFloorDistance(p, plan.Entry) < RiftFloorSafeRadius*RiftFloorSafeRadius || riftFloorDistance(p, plan.Exit) < 80*80 {
					t.Fatalf("map %d floor %d unsafe, unverified or duplicate spawn: %+v", layout.MapID, floor, p)
				}
				for other := range seen {
					if riftFloorDistance(p, other) < 32*32 {
						t.Fatalf("map %d floor %d spacing below32px", layout.MapID, floor)
					}
				}
				seen[p] = true
			}
			if count < min {
				min = count
			}
			if count > max {
				max = count
			}
			total += count
			plans++
		}
	}
	t.Logf("maps=%d seeded plans=%d population=%d..%d mean=%.2f", len(templates), plans, min, max, float64(total)/float64(plans))
}
