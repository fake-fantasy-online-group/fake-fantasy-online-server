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

// Opt-in native MASK audit: all retained maps, all safe tiles, twenty seeds per
// map. Reuses the source export, but runs the real Go loader and v6 planner.
// No production database, client, source hashing or external mutation is used.
func TestRiftFullNativeCatalog(t *testing.T) {
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
	maps, excluded, plans, total, over320, over2048 := 0, 0, 0, 0, 0, 0
	minimum, maximum := int(^uint(0)>>1), 0
	var maxPlan domain.RiftFloorPlan
	var summary []map[string]any
	for _, original := range source {
		if domain.RiftMapExcluded(original.MapID) {
			excluded++
			continue
		}
		packed, ok := compressed[resources[original.MapID]]
		if !ok {
			t.Fatalf("map %d has no mask", original.MapID)
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
			if _, ok := defs[id]; !ok {
				defs[id] = domain.MonsterDef{ID: id, HP: 100, Sprite: 1, Kind: domain.MonsterBoss}
			}
		}
		layout, ok := BuildRiftFloorTemplate(original.MapID, original.MapName, original.Anchors, defs, mask)
		if !ok || len(layout.FullCombatRegions) != 1 {
			t.Fatalf("map %d missing whole verified region", original.MapID)
		}
		region := layout.FullCombatRegions[0]
		if region.CombatArea != float64(len(region.Population))*64*64 {
			t.Fatalf("map %d combat area inflated", layout.MapID)
		}
		prev := denseAuditBFS(mask, region.Population[0].Pos)
		eligible := make(map[domain.Pos]bool, len(region.Population))
		for _, p := range region.Population {
			if prev[denseAuditCell(mask, p.Pos)] < 0 || !fullAuditSafeTile(mask, p.Pos) {
				t.Fatalf("map %d unsafe or disconnected full candidate %+v", layout.MapID, p.Pos)
			}
			eligible[p.Pos] = true
		}
		base := math.Max(24, math.Round(region.CombatArea/domain.RiftCoverageAreaPerMonster))
		wanted := int(math.Ceil(base * 1.5))
		var firstPlan domain.RiftFloorPlan
		oldCount := 0
		for seedIndex := 0; seedIndex < 20; seedIndex++ {
			seed := fmt.Sprintf("full-native-audit-%02d", seedIndex)
			floor := int32(seedIndex%5 + 1)
			p, e := domain.PlanRiftFullCoverageFloor(seed, floor, []domain.RiftFloorTemplate{layout}, 0)
			if e != nil || !p.Valid() {
				t.Fatalf("map=%d seed=%s invalid: %v", layout.MapID, seed, e)
			}
			if !eligible[p.Entry] || !eligible[p.Exit] || prev[denseAuditCell(mask, p.Entry)] < 0 || prev[denseAuditCell(mask, p.Exit)] < 0 {
				t.Fatalf("map=%d unreachable entry/exit", layout.MapID)
			}
			safeCapacity := 0
			for _, point := range region.Population {
				dx, dy := point.Pos.X-p.Entry.X, point.Pos.Y-p.Entry.Y
				ex, ey := point.Pos.X-p.Exit.X, point.Pos.Y-p.Exit.Y
				if dx*dx+dy*dy >= 160*160 && ex*ex+ey*ey >= 80*80 {
					safeCapacity++
				}
			}
			if len(p.Spawns) != min(wanted, safeCapacity) {
				t.Fatalf("map=%d wrong population %d want=%d safe=%d", layout.MapID, len(p.Spawns), wanted, safeCapacity)
			}
			for _, spawn := range p.Spawns {
				if !eligible[spawn.Point.Pos] {
					t.Fatalf("map=%d spawn outside verified native component", layout.MapID)
				}
			}
			if seedIndex == 0 {
				firstPlan = p
				old, e := domain.PlanRiftDenseCoverageFloor(seed, floor, []domain.RiftFloorTemplate{layout}, 0)
				if e != nil {
					t.Fatal(e)
				}
				oldCount = len(old.Spawns)
			}
			n := len(p.Spawns)
			minimum = min(minimum, n)
			total += n
			plans++
			if n > maximum {
				maximum = n
				maxPlan = p
			}
		}
		if len(firstPlan.Spawns) > 320 {
			over320++
		}
		if len(firstPlan.Spawns) > 2048 {
			over2048++
		}
		maps++
		record := map[string]any{"mapId": layout.MapID, "mapName": layout.MapName, "legacyRegions": len(layout.CombatRegions), "safeTiles": len(region.Population), "combatArea": region.CombatArea, "v5Population": oldCount, "v6Population": len(firstPlan.Spawns)}
		summary = append(summary, record)
		t.Logf("FULL map=%d %s safeTiles=%d fullArea=%.0f v5=%d v6=%d legacyRegions=%d seeds=20 PASS", layout.MapID, layout.MapName, len(region.Population), region.CombatArea, oldCount, len(firstPlan.Spawns), len(layout.CombatRegions))
	}
	if maps == 0 {
		t.Fatal("no native maps audited")
	}
	if out := os.Getenv("FFO_RIFT_FULL_AUDIT_OUTPUT"); out != "" {
		result := map[string]any{"maps": maps, "excluded": excluded, "plans": plans, "minPopulation": minimum, "maxPopulation": maximum, "totalPopulation": total, "mapsAbove320": over320, "mapsAbove2048": over2048, "largestPlan": maxPlan, "mapCounts": summary}
		raw, e := json.MarshalIndent(result, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(out, raw, 0644); e != nil {
			t.Fatal(e)
		}
		planJSON, e := json.Marshal(maxPlan)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(out+".max-plan.json", planJSON, 0644); e != nil {
			t.Fatal(e)
		}
		// This is the real ordinary-aura wire shape and actual native population,
		// not a claim that this synthetic payload includes every elite/hazard field.
		view := &domain.EndgameRiftView{RunID: "full-native-audit", GeneratorVersion: 6, MapID: maxPlan.MapID, MapName: maxPlan.MapName, FloorIndex: maxPlan.FloorIndex, Phase: "active"}
		for i, s := range maxPlan.Spawns {
			view.Auras = append(view.Auras, domain.EndgameRiftAuraView{EntityID: strconv.Itoa(4000000000 + i), Class: s.Class})
		}
		wire, e := json.Marshal(domain.EndgameResponse{Kind: "rift.advance", OK: true, Rift: view})
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(out+".max-aura.json", wire, 0644); e != nil {
			t.Fatal(e)
		}
		t.Logf("largest map=%d %s population=%d ordinary-aura-shape bytes=%d saved-plan bytes=%d", maxPlan.MapID, maxPlan.MapName, len(maxPlan.Spawns), len(wire), len(planJSON))
	}
	t.Logf("SUMMARY maps=%d excluded=%d sampledPlans=%d population=%d..%d mean=%.2f mapsAbove320=%d mapsAbove2048=%d all-safe failures=0", maps, excluded, plans, minimum, maximum, float64(total)/float64(plans), over320, over2048)
}

// Independent cell-exact equivalent of denseAuditSafe's 4096 pixel checks.
func fullAuditSafeTile(mask *CollisionMask, p domain.Pos) bool {
	if p.X < 32 || p.Y < 32 || p.X+32 > float64(mask.width)*float64(mask.cellWidth) || p.Y+32 > float64(mask.height)*float64(mask.cellHeight) {
		return false
	}
	for y := int(p.Y-32) / int(mask.cellHeight); y <= int(p.Y+31)/int(mask.cellHeight); y++ {
		for x := int(p.X-32) / int(mask.cellWidth); x <= int(p.X+31)/int(mask.cellWidth); x++ {
			if mask.cells[y*int(mask.width)+x]&maskBlockedBit != 0 {
				return false
			}
		}
	}
	return true
}
