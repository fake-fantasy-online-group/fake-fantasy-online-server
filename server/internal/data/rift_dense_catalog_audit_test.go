package data

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Audits every retained region and every possible v4 entry/exit on native MASKs.
// Each map also samples 20 independent seeds, reconstructs a real collision-grid
// path, and checks each compressed route at one-pixel resolution. Optional
// FFO_RIFT_EXIT_AUDIT_OUTPUT writes one JSON record per map (including MASK bytes
// and plans) for scene-state-machine consumption; output is streamed to bound RAM.
// Use tests/rift_layout_audit.py --templates-output for the input.
// This does not claim live DB, temporary combat hazards, or Unity verification.
func TestRiftDenseNativeCatalog(t *testing.T) {
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
	candidates, regions, plans, maps := 0, 0, 0, 0
	var output *os.File
	if out := os.Getenv("FFO_RIFT_EXIT_AUDIT_OUTPUT"); out != "" {
		var e error
		output, e = os.Create(out)
		if e != nil {
			t.Fatal(e)
		}
		defer output.Close()
	}

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

		record := denseAuditMap{MapID: layout.MapID, MapName: layout.MapName, Width: mask.width, Height: mask.height, CellWidth: mask.cellWidth, CellHeight: mask.cellHeight, Cells: mask.cells}
		// Independently flood the real collision map from each region's first point.
		// Check every possible entry/exit candidate, not just sampled RNG outcomes.
		for ri, region := range layout.CombatRegions {
			prev := denseAuditBFS(mask, region.Population[0].Pos)
			for _, candidate := range region.Population {
				if prev[denseAuditCell(mask, candidate.Pos)] == -1 {
					t.Fatalf("map=%d region=%d unreachable candidate=%+v", layout.MapID, ri, candidate.Pos)
				}
				if !denseAuditSafe(mask, candidate.Pos) {
					t.Fatalf("map=%d region=%d unsafe candidate=%+v", layout.MapID, ri, candidate.Pos)
				}
				// Every possible entry must admit an exit at the planner's 512px minimum.
				foundFar := false
				for _, other := range region.Population {
					dx, dy := other.Pos.X-candidate.Pos.X, other.Pos.Y-candidate.Pos.Y
					if dx*dx+dy*dy >= 512*512 {
						foundFar = true
						break
					}
				}
				if !foundFar {
					t.Fatalf("map=%d region=%d entry=%+v has no legal distant exit", layout.MapID, ri, candidate.Pos)
				}
				record.Candidates++
			}
		}
		record.Regions = len(layout.CombatRegions)
		for seedIndex := 0; seedIndex < 20; seedIndex++ {
			seed := fmt.Sprintf("exit-native-audit-%02d", seedIndex)
			floor := int32(seedIndex%5 + 1)
			old, e := domain.PlanRiftCoverageFloor(seed, floor, []domain.RiftFloorTemplate{layout}, 0)
			if e != nil {
				t.Fatal(e)
			}
			p, e := domain.PlanRiftDenseCoverageFloor(seed, floor, []domain.RiftFloorTemplate{layout}, 0)
			if e == nil {
				want := min(320, (len(old.Spawns)*3+1)/2)
				if len(p.Spawns) != want || p.Entry != old.Entry || p.Exit != old.Exit {
					t.Fatalf("map=%d seed=%s changed geometry/count old=%d new=%d want=%d", layout.MapID, seed, len(old.Spawns), len(p.Spawns), want)
				}
				if seedIndex == 0 {
					t.Logf("DENSITY map=%d %s old=%d new=%d localPeak192=%d/%d", layout.MapID, layout.MapName, len(old.Spawns), len(p.Spawns), coverageLocalPeak(old.Spawns), coverageLocalPeak(p.Spawns))
				}
			}
			if e != nil || !p.Valid() {
				t.Fatalf("map=%d seed=%s floor=%d plan error=%v", layout.MapID, seed, floor, e)
			}
			prev := denseAuditBFS(mask, p.Entry)
			at := denseAuditCell(mask, p.Exit)
			if prev[at] == -1 {
				t.Fatalf("map=%d seed=%s floor=%d exit unreachable", layout.MapID, seed, floor)
			}
			path := []domain.Pos{p.Exit}
			for prev[at] != at {
				at = prev[at]
				path = append(path, denseAuditPos(mask, layout.MapID, at))
			}
			path = append(path, p.Entry)
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			compact := []domain.Pos{path[0]}
			for i := 1; i < len(path)-1; i++ {
				a, b, c := path[i-1], path[i], path[i+1]
				if (b.X-a.X)*(c.Y-b.Y) != (b.Y-a.Y)*(c.X-b.X) {
					compact = append(compact, b)
				}
			}
			compact = append(compact, p.Exit)
			for i := 1; i < len(compact); i++ {
				a, b := compact[i-1], compact[i]
				steps := int(math.Ceil(math.Hypot(b.X-a.X, b.Y-a.Y)))
				for j := 0; j <= steps; j++ {
					q := a
					if steps > 0 {
						q.X += (b.X - a.X) * float64(j) / float64(steps)
						q.Y += (b.Y - a.Y) * float64(j) / float64(steps)
					}
					if !mask.Walkable(q) {
						t.Fatalf("map=%d seed=%s route crosses blocked pixel %+v", layout.MapID, seed, q)
					}
				}
			}
			record.Plans = append(record.Plans, denseAuditPlan{Seed: seed, Plan: p, Path: compact})
		}
		if output != nil {
			if e := json.NewEncoder(output).Encode(record); e != nil {
				t.Fatal(e)
			}
		}
		candidates += record.Candidates
		regions += record.Regions
		plans += len(record.Plans)
		maps++
		t.Logf("map=%d %s regions=%d all-candidates=%d sampled-plans=%d PASS", record.MapID, record.MapName, record.Regions, record.Candidates, len(record.Plans))
	}
	t.Logf("SUMMARY maps=%d regions=%d all-candidates=%d sampled-plans=%d failures=0", maps, regions, candidates, plans)
}

type denseAuditMap struct {
	MapID                                int32
	MapName                              string
	Width, Height, CellWidth, CellHeight uint32
	Cells                                []byte
	Regions, Candidates                  int
	Plans                                []denseAuditPlan
}
type denseAuditPlan struct {
	Seed string
	Plan domain.RiftFloorPlan
	Path []domain.Pos
}

func denseAuditCell(m *CollisionMask, p domain.Pos) int {
	return int(uint32(p.Y)/m.cellHeight)*int(m.width) + int(uint32(p.X)/m.cellWidth)
}
func denseAuditPos(m *CollisionMask, id int32, i int) domain.Pos {
	return domain.Pos{MapID: id, X: (float64(i%int(m.width)) + .5) * float64(m.cellWidth), Y: (float64(i/int(m.width)) + .5) * float64(m.cellHeight)}
}
func denseAuditSafe(m *CollisionMask, p domain.Pos) bool {
	for y := -32; y < 32; y++ {
		for x := -32; x < 32; x++ {
			q := p
			q.X += float64(x)
			q.Y += float64(y)
			if !m.Walkable(q) {
				return false
			}
		}
	}
	return true
}
func denseAuditBFS(m *CollisionMask, p domain.Pos) []int {
	prev := make([]int, len(m.cells))
	for i := range prev {
		prev[i] = -1
	}
	start := denseAuditCell(m, p)
	prev[start] = start
	q := []int{start}
	w := int(m.width)
	for head := 0; head < len(q); head++ {
		at := q[head]
		visit := func(n int) {
			if prev[n] == -1 && m.cells[n]&maskBlockedBit == 0 {
				prev[n] = at
				q = append(q, n)
			}
		}
		if at%w > 0 {
			visit(at - 1)
		}
		if at%w < w-1 {
			visit(at + 1)
		}
		if at >= w {
			visit(at - w)
		}
		if at+w < len(prev) {
			visit(at + w)
		}
	}
	return prev
}
