package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	// Changes to the PRNG, ordering or population rules require a new version.
	RiftFloorGeneratorVersion        int32 = 3
	riftFloorBaseGeneratorVersion    int32 = 2
	RiftFloorMinAnchors                    = 18
	RiftFloorSafeRadius                    = 160
	RiftFloorMinPopulation                 = 96
	RiftFloorMaxPopulation                 = 160
	RiftFloorMaxConfiguredPopulation       = 2 * RiftFloorMaxPopulation
	RiftFloorPopulationSpacing             = 64
	RiftFloorPopulationRadius              = 160
	RiftFloorPackRadius                    = 192
	RiftFloorAreaPerMonster                = 448 * 448
)

// RiftFloorTemplate contains only anchors on the same verified walkable map
// component. The data loader excludes missing masks, cities and noncombat assets.
// Templates are immutable after startup and never accepted from a client.
type RiftFloorTemplate struct {
	MapID     int32
	MapName   string
	Anchors   []SpawnPoint
	Monsters  []MonsterID
	Guardians []MonsterID
	// Population is a collision-verified lattice near native hostile anchors.
	// Entry/exit still use Anchors. Empty Population supports anchor-only catalogs.
	Population []SpawnPoint
	// ExtraPopulation adds collision-verified positions only for density expansion.
	// Keeping it separate preserves the exact v2 base seed, composition and guardian.
	ExtraPopulation []SpawnPoint
	WalkableArea    float64
	// CombatRegions are independently connected, bounded playable areas for v4.
	// Legacy population and seed streams are deliberately left untouched.
	CombatRegions []RiftCombatRegion
}

type RiftFloorSpawn struct {
	Point SpawnPoint
	Class int32
}

// RiftFloorPlan is saved in full before admission/transfer. A seed is useful for
// diagnostics, but never regenerates an admitted floor against a changed catalog.
type RiftFloorPlan struct {
	GeneratorVersion int32
	Seed             uint64
	FloorIndex       int32
	MapID            int32
	MapName          string
	Entry, Exit      Pos
	Spawns           []RiftFloorSpawn
	Guardian         RiftFloorSpawn
}

// RiftFloorSeed retains the v2 platform-independent base stream. Version 3
// expands that frozen base population without changing its seed or guardian.
// It has no dependency on wall time, map iteration or the combat RNG stream.
func RiftFloorSeed(runID string, floor int32) uint64 {
	h := sha256.New()
	h.Write([]byte("fantasy-rift-floor\x00"))
	var b [8]byte
	binary.LittleEndian.PutUint32(b[:4], uint32(riftFloorBaseGeneratorVersion))
	binary.LittleEndian.PutUint32(b[4:], uint32(floor))
	h.Write(b[:])
	h.Write([]byte(runID))
	return binary.LittleEndian.Uint64(h.Sum(nil)[:8])
}

// riftFloorRandom is SplitMix64, unchanged since generator version 1.
type riftFloorRandom uint64

func (r *riftFloorRandom) next() uint64 {
	*r += 0x9e3779b97f4a7c15
	z := uint64(*r)
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}
func (r *riftFloorRandom) index(n int) int { return int(r.next() % uint64(n)) }

func riftFloorPointValid(p Pos, mapID int32) bool {
	return p.MapID == mapID && p.X >= 0 && p.Y >= 0 && !math.IsNaN(p.X) && !math.IsNaN(p.Y) && !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0)
}
func riftFloorDistance(a, b Pos) float64 { x, y := a.X-b.X, a.Y-b.Y; return x*x + y*y }

func (t RiftFloorTemplate) Valid() bool {
	if t.MapID <= 0 || len(t.Anchors) < RiftFloorMinAnchors || len(t.Monsters) == 0 || len(t.Guardians) == 0 {
		return false
	}
	for _, points := range [][]SpawnPoint{t.Anchors, t.Population, t.ExtraPopulation} {
		for _, p := range points {
			if !riftFloorPointValid(p.Pos, t.MapID) {
				return false
			}
		}
	}
	if t.WalkableArea < 0 || math.IsNaN(t.WalkableArea) || math.IsInf(t.WalkableArea, 0) {
		return false
	}
	for _, pool := range [][]MonsterID{t.Monsters, t.Guardians} {
		for _, id := range pool {
			if id <= 0 {
				return false
			}
		}
	}
	return true
}

// PlanRiftFloor chooses an existing map and creates a bounded randomized native
// population. Difficulty/tier is deliberately absent: floors are traversal, not
// difficulty. Adjacent floors avoid repeating a map whenever another exists.
func PlanRiftFloor(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32, density ...int32) (RiftFloorPlan, error) {
	multiplier := int32(1)
	if len(density) > 1 || (len(density) == 1 && (density[0] < 0 || density[0] > 2)) {
		return RiftFloorPlan{}, errors.New("invalid rift population multiplier")
	}
	if len(density) == 1 && density[0] > 0 {
		multiplier = density[0]
	}
	pool := append([]RiftFloorTemplate(nil), templates...)
	for {
		plan, err := planRiftFloorBase(runID, floor, pool, previousMap)
		if err != nil || multiplier == 1 {
			return plan, err
		}
		for _, layout := range pool {
			if layout.MapID == plan.MapID {
				if err = doubleRiftFloorPopulation(&plan, layout); err == nil {
					return plan, nil
				}
				break
			}
		}
		// Try another verified map deterministically before admission or floor
		// transfer commits. Never charge a key for a partial/clamped population.
		remaining := pool[:0]
		for _, layout := range pool {
			if layout.MapID != plan.MapID {
				remaining = append(remaining, layout)
			}
		}
		pool = remaining
		if len(pool) == 0 {
			return RiftFloorPlan{}, fmt.Errorf("no rift map supports doubled population: %w", err)
		}
	}
}

func planRiftFloorBase(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32) (RiftFloorPlan, error) {
	var plan RiftFloorPlan
	if runID == "" || floor < 1 {
		return plan, errors.New("invalid rift floor identity")
	}
	pool := append([]RiftFloorTemplate(nil), templates...)
	sort.Slice(pool, func(i, j int) bool { return pool[i].MapID < pool[j].MapID })
	valid := pool[:0]
	seen := map[int32]bool{}
	for _, t := range pool {
		if !RiftMapExcluded(t.MapID) && t.Valid() && !seen[t.MapID] {
			valid = append(valid, t)
			seen[t.MapID] = true
		}
	}
	pool = valid
	if len(pool) > 1 {
		valid = pool[:0]
		for _, t := range pool {
			if t.MapID != previousMap {
				valid = append(valid, t)
			}
		}
		pool = valid
	}
	if len(pool) == 0 {
		return plan, errors.New("no validated rift maps")
	}
	seed := RiftFloorSeed(runID, floor)
	rng := riftFloorRandom(seed)
	t := pool[rng.index(len(pool))]
	anchors := shuffledRiftFloorPoints(t.Anchors, &rng)
	population := anchors
	if len(t.Population) > 0 {
		population = shuffledRiftFloorPoints(t.Population, &rng)
	}
	var entry, exit SpawnPoint
	var points []SpawnPoint
	// Some native maps have dense clusters beside a long corridor. Scan the
	// seeded entry order instead of occasionally failing an otherwise valid map.
	for _, candidate := range anchors {
		entry = candidate
		exit = anchors[0]
		for _, p := range anchors {
			if riftFloorDistance(entry.Pos, p.Pos) > riftFloorDistance(entry.Pos, exit.Pos) {
				exit = p
			}
		}
		if riftFloorDistance(entry.Pos, exit.Pos) < 512*512 {
			continue
		}
		points = points[:0]
		for _, p := range population {
			if riftFloorDistance(entry.Pos, p.Pos) >= RiftFloorSafeRadius*RiftFloorSafeRadius && riftFloorDistance(exit.Pos, p.Pos) >= 80*80 {
				points = append(points, p)
			}
		}
		if len(points) >= 8 {
			break
		}
	}
	if len(points) < 8 {
		return RiftFloorPlan{}, errors.New("rift map has insufficient safe population anchors")
	}
	monsters := append([]MonsterID(nil), t.Monsters...)
	guardians := append([]MonsterID(nil), t.Guardians...)
	sort.Slice(monsters, func(i, j int) bool { return monsters[i] < monsters[j] })
	sort.Slice(guardians, func(i, j int) bool { return guardians[i] < guardians[j] })
	plan = RiftFloorPlan{GeneratorVersion: riftFloorBaseGeneratorVersion, Seed: seed, FloorIndex: floor, MapID: t.MapID, MapName: t.MapName, Entry: entry.Pos, Exit: exit.Pos}
	// Scale the target with the verified connected walkable area, with seeded
	// 90–110% variation. Small maps never force overlapping or blocked spawns.
	target := t.WalkableArea / RiftFloorAreaPerMonster
	if target < RiftFloorMinPopulation {
		target = RiftFloorMinPopulation
	}
	if target > RiftFloorMaxPopulation {
		target = RiftFloorMaxPopulation
	}
	n := int(target * float64(90+rng.index(21)) / 100)
	if n < RiftFloorMinPopulation {
		n = RiftFloorMinPopulation
	}
	if n > RiftFloorMaxPopulation {
		n = RiftFloorMaxPopulation
	}
	if n > len(points) {
		n = len(points)
	}
	points = riftFloorPackPoints(points, n, &rng)
	blue := 6 + 3*rng.index(3)
	yellow := 2 + rng.index(3)
	if blue+yellow >= n {
		blue = n / 3
		yellow = 2
	}
	// Distribute the unchanged elite counts across the chosen packs rather than
	// concentrating every elite in the first two encounters.
	classes := make([]int32, n)
	for i := 0; i < yellow+blue; i++ {
		classes[i] = 1
		if i < yellow {
			classes[i] = 2
		}
	}
	for i := len(classes) - 1; i > 0; i-- {
		j := rng.index(i + 1)
		classes[i], classes[j] = classes[j], classes[i]
	}
	for i := 0; i < n; i++ {
		p := points[i]
		p.Monster = monsters[rng.index(len(monsters))]
		plan.Spawns = append(plan.Spawns, RiftFloorSpawn{Point: p, Class: classes[i]})
	}
	exit.Monster = guardians[rng.index(len(guardians))]
	plan.Guardian = RiftFloorSpawn{Point: exit, Class: 3}
	return plan, nil
}

// Append one counterpart for each base actor. Composition, progress per kill,
// appearance, original positions, map, seed, entry/exit and guardian stay exact.
func doubleRiftFloorPopulation(plan *RiftFloorPlan, layout RiftFloorTemplate) error {
	seen := make(map[Pos]bool, len(plan.Spawns)*2)
	for _, spawn := range plan.Spawns {
		seen[spawn.Point.Pos] = true
	}
	var candidates []SpawnPoint
	population := layout.Population
	if len(population) == 0 {
		population = layout.Anchors
	}
	for _, source := range [][]SpawnPoint{population, layout.ExtraPopulation} {
		for _, p := range source {
			if seen[p.Pos] || riftFloorDistance(p.Pos, plan.Entry) < RiftFloorSafeRadius*RiftFloorSafeRadius || riftFloorDistance(p.Pos, plan.Exit) < 80*80 {
				continue
			}
			seen[p.Pos] = true
			candidates = append(candidates, p)
		}
	}
	if len(candidates) < len(plan.Spawns) {
		return fmt.Errorf("map %d has %d extra safe positions; need %d", plan.MapID, len(candidates), len(plan.Spawns))
	}
	// A separate stream cannot perturb the v2 base stream, even as extra
	// candidates change in a future catalog. Saved plans never regenerate.
	rng := riftFloorRandom(plan.Seed ^ 0x726966742d327800)
	candidates = shuffledRiftFloorPoints(candidates, &rng)
	candidates = riftFloorPackPoints(candidates, len(plan.Spawns), &rng)
	base := plan.Spawns
	plan.Spawns = make([]RiftFloorSpawn, 0, 2*len(base))
	plan.Spawns = append(plan.Spawns, base...)
	for i, original := range base {
		p := candidates[i]
		p.Monster = original.Point.Monster
		plan.Spawns = append(plan.Spawns, RiftFloorSpawn{Point: p, Class: original.Class})
	}
	plan.GeneratorVersion = RiftFloorGeneratorVersion
	return nil
}

func shuffledRiftFloorPoints(source []SpawnPoint, rng *riftFloorRandom) []SpawnPoint {
	points := append([]SpawnPoint(nil), source...)
	sort.Slice(points, func(i, j int) bool {
		a, b := points[i], points[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Pos.X != b.Pos.X {
			return a.Pos.X < b.Pos.X
		}
		return a.Pos.Y < b.Pos.Y
	})
	for i := len(points) - 1; i > 0; i-- {
		j := rng.index(i + 1)
		points[i], points[j] = points[j], points[i]
	}
	return points
}

// Seeded centers plus four-to-eight nearby positions make encounters visibly
// denser without multiplying elite rewards or inventing unchecked coordinates.
func riftFloorPackPoints(points []SpawnPoint, n int, rng *riftFloorRandom) []SpawnPoint {
	chosen := make([]SpawnPoint, 0, n)
	used := make([]bool, len(points))
	for i, center := range points {
		if len(chosen) == n {
			break
		}
		if used[i] {
			continue
		}
		used[i] = true
		chosen = append(chosen, center)
		remaining := 3 + rng.index(5)
		for j, p := range points {
			if remaining == 0 || len(chosen) == n {
				break
			}
			if !used[j] && riftFloorDistance(center.Pos, p.Pos) <= RiftFloorPackRadius*RiftFloorPackRadius {
				used[j] = true
				chosen = append(chosen, p)
				remaining--
			}
		}
	}
	return chosen
}

// Valid checks persisted plan shape. Runtime separately verifies referenced
// templates and every coordinate against its own collision mask before spawning.
func (p RiftFloorPlan) Valid() bool {
	maxPopulation := RiftFloorMaxPopulation
	if p.GeneratorVersion == 1 {
		// Already-admitted plans remain authoritative and are never regenerated.
		maxPopulation = 64
	} else if p.GeneratorVersion == RiftFloorGeneratorVersion || p.GeneratorVersion == RiftFloorCoverageVersion {
		maxPopulation = RiftFloorMaxConfiguredPopulation
	} else if p.GeneratorVersion == RiftFloorDenseCoverageVersion {
		maxPopulation = RiftDenseCoverageMaxPopulation
	} else if p.GeneratorVersion != riftFloorBaseGeneratorVersion {
		return false
	}
	if p.FloorIndex < 1 || p.MapID <= 0 || len(p.Spawns) < 8 || len(p.Spawns) > maxPopulation || !riftFloorPointValid(p.Entry, p.MapID) || !riftFloorPointValid(p.Exit, p.MapID) || riftFloorDistance(p.Entry, p.Exit) < 512*512 {
		return false
	}
	if p.Guardian.Class != 3 || p.Guardian.Point.Monster <= 0 || !riftFloorPointValid(p.Guardian.Point.Pos, p.MapID) {
		return false
	}
	seen := map[Pos]bool{}
	for _, s := range p.Spawns {
		if s.Class < 0 || s.Class > 2 || s.Point.Monster <= 0 || !riftFloorPointValid(s.Point.Pos, p.MapID) {
			return false
		}
		if p.GeneratorVersion >= 2 && (seen[s.Point.Pos] || riftFloorDistance(s.Point.Pos, p.Entry) < RiftFloorSafeRadius*RiftFloorSafeRadius || riftFloorDistance(s.Point.Pos, p.Exit) < 80*80) {
			return false
		}
		if p.GeneratorVersion == RiftFloorCoverageVersion || p.GeneratorVersion == RiftFloorDenseCoverageVersion {
			for other := range seen {
				if riftFloorDistance(other, s.Point.Pos) < RiftFloorPopulationSpacing*RiftFloorPopulationSpacing {
					return false
				}
			}
		}
		seen[s.Point.Pos] = true
	}
	return true
}
