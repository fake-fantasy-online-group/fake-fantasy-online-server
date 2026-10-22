package data

import (
	"math"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestRiftFullCoverageCrossesLegacyCutsAndRestoresBoundaryTiles(t *testing.T) {
	_, defs, points := riftGeometryFixture()
	mask := &CollisionMask{width: 500, height: 300, cellWidth: 32, cellHeight: 32, cells: make([]byte, 500*300)}
	layout, ok := BuildRiftFloorTemplate(7, "large full component", points, defs, mask)
	if !ok || len(layout.FullCombatRegions) != 1 || len(layout.CombatRegions) < 4 {
		t.Fatal("invalid large geometry")
	}
	full := layout.FullCombatRegions[0]
	if len(full.Population) != 250*150 || full.CombatArea != 16000*9600 {
		t.Fatalf("full component missing tiles: %d area=%.0f", len(full.Population), full.CombatArea)
	}
	again, ok := BuildRiftFloorTemplate(7, "large full component", points, defs, mask)
	if !ok || !reflect.DeepEqual(layout, again) {
		t.Fatal("nondeterministic full component")
	}
	plan, err := domain.PlanRiftFullCoverageFloor("across-cuts", 1, []domain.RiftFloorTemplate{layout}, 0)
	if err != nil || !plan.Valid() || len(plan.Spawns) != 2297 {
		t.Fatalf("full population=%d want=2297 err=%v", len(plan.Spawns), err)
	}
	old, err := domain.PlanRiftDenseCoverageFloor("across-cuts", 1, []domain.RiftFloorTemplate{layout}, 0)
	if err != nil || len(old.Spawns) > 320 {
		t.Fatal("legacy bounded planner changed", err)
	}
	occupied := make(map[[2]int]bool)
	for _, s := range plan.Spawns {
		if !mask.Walkable(s.Point.Pos) {
			t.Fatal("blocked spawn")
		}
		occupied[[2]int{int(s.Point.Pos.X) / 4000, int(s.Point.Pos.Y) / 3200}] = true
	}
	if len(occupied) != 12 {
		t.Fatalf("full-map rooms empty: occupied=%d want=12", len(occupied))
	}
	for _, legacy := range layout.CombatRegions {
		positions := map[domain.Pos]bool{}
		for _, p := range legacy.Population {
			positions[p.Pos] = true
		}
		found := false
		for _, s := range plan.Spawns {
			if positions[s.Point.Pos] {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("full plan omitted a former bounded region")
		}
	}
	t.Logf("old bounded regions=%d old v5 population=%d full safe tiles=%d full combat area=%.0f v6 population=%d", len(layout.CombatRegions), len(old.Spawns), len(full.Population), full.CombatArea, len(plan.Spawns))
}
func TestRiftFullCoverageKeepsWallsIslandsAndNarrowCorridorsSafe(t *testing.T) {
	mask, defs, points := riftGeometryFixture()
	for y := 0; y < int(mask.height); y++ {
		mask.cells[y*int(mask.width)+65] = maskBlockedBit
	}
	layout, ok := BuildRiftFloorTemplate(7, "separated", points, defs, mask)
	if !ok {
		t.Fatal("fixture rejected")
	}
	for _, p := range layout.FullCombatRegions[0].Population {
		if p.Pos.X >= 1040 {
			t.Fatal("crossed disconnected wall")
		}
	}
	mask.cells[50*int(mask.width)+65] = 0
	layout, ok = BuildRiftFloorTemplate(7, "narrow connection", points, defs, mask)
	if !ok {
		t.Fatal("connected fixture rejected")
	}
	far := false
	for _, p := range layout.FullCombatRegions[0].Population {
		far = far || p.Pos.X > 1500
		if p.Pos.X >= 1024 && p.Pos.X <= 1088 {
			t.Fatal("spawn in narrow connection")
		}
		for dy := -32; dy < 32; dy++ {
			for dx := -32; dx < 32; dx++ {
				if !mask.Walkable(domain.Pos{MapID: 7, X: p.Pos.X + float64(dx), Y: p.Pos.Y + float64(dy)}) {
					t.Fatal("tile not wholly walkable")
				}
			}
		}
	}
	if !far {
		t.Fatal("lost reachable far room")
	}
	plan, err := domain.PlanRiftFullCoverageFloor("narrow", 1, []domain.RiftFloorTemplate{layout}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range plan.Spawns {
		for _, other := range plan.Spawns[:i] {
			if math.Hypot(s.Point.Pos.X-other.Point.Pos.X, s.Point.Pos.Y-other.Point.Pos.Y) < 64 {
				t.Fatal("unsafe spacing")
			}
		}
	}
}
