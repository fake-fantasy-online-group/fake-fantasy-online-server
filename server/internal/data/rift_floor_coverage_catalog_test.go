package data

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Reuses the static source export, then calls the ACTUAL data loader and v4
// planner against checked-in MASK bytes. Run after tests/rift_layout_audit.py
// --templates-output and set FFO_RIFT_COVERAGE_AUDIT_TEMPLATES to that file.
// This does not claim live DB or client/asset rendering verification.
func TestRiftCoverageNativeCatalog(t *testing.T) {
	path := os.Getenv("FFO_RIFT_COVERAGE_AUDIT_TEMPLATES")
	if path == "" {
		t.Skip("set FFO_RIFT_COVERAGE_AUDIT_TEMPLATES")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var source []domain.RiftFloorTemplate
	if err = json.Unmarshal(raw, &source); err != nil || len(source) == 0 {
		t.Fatal("invalid template export", err)
	}
	migration := func(n int) []byte {
		b, e := os.ReadFile(filepath.Join("../../migrations", fmt.Sprintf("000000_init-%d.sql", n)))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	resources := map[int32]string{}
	blocks := regexp.MustCompile(`(?s)INSERT INTO "map_defs".*?VALUES\s*(.*?);`).FindAllSubmatch(migration(0), -1)
	for _, block := range blocks {
		for _, m := range regexp.MustCompile(`\((\d+), '([^']+)', '[^']*', (?:TRUE|FALSE)\)`).FindAllSubmatch(block[1], -1) {
			id, _ := strconv.Atoi(string(m[1]))
			resources[int32(id)] = string(m[2])
		}
	}
	for _, m := range regexp.MustCompile(`\((\d+),'([^']+)','[^']*'\)`).FindAllSubmatch(migration(46), -1) {
		id, _ := strconv.Atoi(string(m[1]))
		resources[int32(id)] = string(m[2])
	}
	compressed := map[string][]byte{}
	maskRE := regexp.MustCompile(`\(convert_from\(decode\('([0-9a-f]+)','hex'\),'UTF8'\),decode\('([0-9a-f]+)','hex'\),\d+\)`)
	for _, n := range []int{47, 48, 49} {
		for _, m := range maskRE.FindAllSubmatch(migration(n), -1) {
			name, _ := hex.DecodeString(string(m[1]))
			data, _ := hex.DecodeString(string(m[2]))
			compressed[string(name)] = data
		}
	}
	minimum, maximum, total, plans := 320, 0, 0, 0
	for _, original := range source {
		packed, ok := compressed[resources[original.MapID]]
		if !ok {
			t.Fatalf("map %d no mask", original.MapID)
		}
		zr, e := gzip.NewReader(bytes.NewReader(packed))
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(zr)
		zr.Close()
		if e != nil {
			t.Fatal(e)
		}
		mask, e := loadCollisionMask(resources[original.MapID], data)
		if e != nil {
			t.Fatal(e)
		}
		defs := riftFloorTestDefs{}
		for _, id := range original.Monsters {
			defs[id] = domain.MonsterDef{ID: id, HP: 100, Sprite: 1, Kind: domain.MonsterNormal}
		}
		for _, id := range original.Guardians {
			if _, exists := defs[id]; !exists {
				defs[id] = domain.MonsterDef{ID: id, HP: 100, Sprite: 1, Kind: domain.MonsterBoss}
			}
		}
		layout, ok := BuildRiftFloorTemplate(original.MapID, original.MapName, original.Anchors, defs, mask)
		if !ok {
			t.Fatalf("map %d loader rejected", original.MapID)
		}
		if len(layout.CombatRegions) == 0 {
			t.Fatalf("map %d no coverage regions", original.MapID)
		}
		for floor := int32(1); floor <= 20; floor++ {
			p, e := domain.PlanRiftCoverageFloor("native-coverage-audit", floor, []domain.RiftFloorTemplate{layout}, 0)
			if e != nil || !p.Valid() {
				t.Fatalf("map %d floor %d invalid: %v", layout.MapID, floor, e)
			}
			n := len(p.Spawns)
			minimum = min(minimum, n)
			maximum = max(maximum, n)
			total += n
			plans++
			for _, s := range p.Spawns {
				if !mask.Walkable(s.Point.Pos) {
					t.Fatal("blocked generated spawn")
				}
			}
			if floor == 1 && (layout.MapID == 28402 || layout.MapID == 28602 || layout.MapID == 2) {
				old, e := domain.PlanRiftFloor("native-coverage-audit", floor, []domain.RiftFloorTemplate{layout}, 0, 2)
				if e != nil {
					t.Fatal(e)
				}
				// Measure actual shortest 4-neighbor MASK path distance to the nearest
				// monster, restricted to the chosen playable region's safe tile centers.
				region := coveragePlanRegion(t, layout, p)
				oldGap := coveragePathGap(mask, region.Population, old.Spawns)
				newGap := coveragePathGap(mask, region.Population, p.Spawns)
				t.Logf("map=%d %s old/new mobs=%d/%d selectedCombatArea=%.0f old/new max walk-to-mob=%d/%d px old/new localPeak(192px)=%d/%d regions=%d", layout.MapID, layout.MapName, len(old.Spawns), n, region.CombatArea, oldGap, newGap, coverageLocalPeak(old.Spawns), coverageLocalPeak(p.Spawns), len(layout.CombatRegions))
			}
		}
	}
	t.Logf("maps=%d plans=%d population=%d..%d mean=%.2f", len(source), plans, minimum, maximum, float64(total)/float64(plans))
}
func coveragePlanRegion(t *testing.T, layout domain.RiftFloorTemplate, p domain.RiftFloorPlan) domain.RiftCombatRegion {
	for _, r := range layout.CombatRegions {
		for _, point := range r.Population {
			if point.Pos == p.Entry {
				return r
			}
		}
	}
	t.Fatal("entry outside verified regions")
	return domain.RiftCombatRegion{}
}
func coveragePathGap(mask *CollisionMask, points []domain.SpawnPoint, spawns []domain.RiftFloorSpawn) int {
	// Source masks currently use square cells. Refuse misleading non-square metrics.
	if mask.cellWidth != mask.cellHeight {
		return -1
	}
	width := int(mask.width)
	distance := make([]int32, len(mask.cells))
	for i := range distance {
		distance[i] = -1
	}
	queue := make([]int, 0, len(mask.cells))
	for _, s := range spawns {
		i := int(uint32(s.Point.Pos.Y)/mask.cellHeight)*width + int(uint32(s.Point.Pos.X)/mask.cellWidth)
		if distance[i] < 0 {
			distance[i] = 0
			queue = append(queue, i)
		}
	}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		visit := func(j int) {
			if distance[j] < 0 && mask.cells[j]&maskBlockedBit == 0 {
				distance[j] = distance[i] + 1
				queue = append(queue, j)
			}
		}
		if i%width > 0 {
			visit(i - 1)
		}
		if i%width < width-1 {
			visit(i + 1)
		}
		if i >= width {
			visit(i - width)
		}
		if i+width < len(distance) {
			visit(i + width)
		}
	}
	gap := int32(0)
	for _, p := range points {
		i := int(uint32(p.Pos.Y)/mask.cellHeight)*width + int(uint32(p.Pos.X)/mask.cellWidth)
		if distance[i] < 0 {
			return -1
		}
		if distance[i] > gap {
			gap = distance[i]
		}
	}
	return int(gap) * int(mask.cellWidth)
}
func coverageLocalPeak(spawns []domain.RiftFloorSpawn) int {
	peak := 0
	for _, a := range spawns {
		n := 0
		for _, b := range spawns {
			dx, dy := a.Point.Pos.X-b.Point.Pos.X, a.Point.Pos.Y-b.Point.Pos.Y
			if dx*dx+dy*dy <= 192*192 {
				n++
			}
		}
		peak = max(peak, n)
	}
	return peak
}
