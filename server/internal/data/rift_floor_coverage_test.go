package data

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
)

func TestRiftCoverageUsesRoomsBeyondNativeAnchors(t *testing.T) {
	mask, defs, points := riftGeometryFixture()
	// All native anchors remain on the left, but the connected right room is usable.
	points = points[:50]
	layout, ok := BuildRiftFloorTemplate(7, "rooms", points, defs, mask)
	if !ok || len(layout.CombatRegions) == 0 {
		t.Fatal("no verified combat regions")
	}
	far := false
	for _, r := range layout.CombatRegions {
		for _, p := range r.Population {
			if p.Pos.X > 1500 {
				far = true
			}
			if !mask.Walkable(p.Pos) {
				t.Fatal("blocked coverage")
			}
		}
	}
	if !far {
		t.Fatal("coverage still limited to native spawn circles")
	}
	plan, err := domain.PlanRiftCoverageFloor("rooms", 1, []domain.RiftFloorTemplate{layout}, 0)
	if err != nil {
		t.Fatal(err)
	}
	far = false
	for _, s := range plan.Spawns {
		far = far || s.Point.Pos.X > 1500
	}
	if !far {
		t.Fatal("empty connected room")
	}
	if len(plan.Spawns) >= 192 {
		t.Fatal("small map still forced to old minimum")
	}
}
func TestRiftCoverageExcludesWallsIslandsAndNarrowCorridors(t *testing.T) {
	mask, defs, points := riftGeometryFixture()
	for y := 0; y < int(mask.height); y++ {
		mask.cells[y*int(mask.width)+65] = maskBlockedBit
	}
	layout, ok := BuildRiftFloorTemplate(7, "separated", points, defs, mask)
	if !ok {
		t.Fatal("invalid fixture")
	}
	for _, r := range layout.CombatRegions {
		for _, p := range r.Population {
			if p.Pos.X >= 1040 {
				t.Fatal("crossed disconnected wall")
			}
		}
	}
	// A one-cell corridor connects the two sides but is not counted as combat area.
	mask.cells[50*int(mask.width)+65] = 0
	layout, ok = BuildRiftFloorTemplate(7, "corridor", points, defs, mask)
	if !ok {
		t.Fatal("invalid connected fixture")
	}
	for _, r := range layout.CombatRegions {
		for _, p := range r.Population {
			if p.Pos.X >= 1024 && p.Pos.X <= 1088 {
				t.Fatal("spawn placed in narrow corridor")
			}
		}
	}
}
func TestRiftCoverageLargeRegionsBoundedConnectedAndStable(t *testing.T) {
	mask := &CollisionMask{width: 500, height: 300, cellWidth: 32, cellHeight: 32, cells: make([]byte, 500*300)}
	labels := make([]int32, len(mask.cells))
	for i := range labels {
		labels[i] = 1
	}
	regions := buildRiftCombatRegions(9, mask, labels, 1)
	if len(regions) < 4 {
		t.Fatal("large map not partitioned")
	}
	if !reflect.DeepEqual(regions, buildRiftCombatRegions(9, mask, labels, 1)) {
		t.Fatal("nondeterministic partition")
	}
	for _, r := range regions {
		if r.CombatArea > domain.RiftCoverageMaxArea || r.CombatArea != float64(len(r.Population)*64*64) {
			t.Fatal("unbounded or inflated combat area")
		}
	}
}
