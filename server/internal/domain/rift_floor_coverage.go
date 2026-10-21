package domain

import (
	"errors"
	"math"
	"sort"
)

const (
	// Version 4 uses bounded connected combat regions, not native spawn circles.
	// Keep the v1-v3 constants and random streams unchanged for admitted runs.
	RiftFloorCoverageVersion       int32 = 4
	RiftFloorDenseCoverageVersion  int32 = 5
	RiftDenseCoverageMaxPopulation       = RiftFloorMaxConfiguredPopulation
	RiftCoverageAreaPerMonster           = RiftFloorAreaPerMonster / 2
	RiftCoverageMaxArea                  = RiftCoverageAreaPerMonster * RiftFloorMaxConfiguredPopulation
	RiftCoverageMinPopulation            = 24
)

type RiftCombatRegion struct {
	Population []SpawnPoint
	// CombatArea counts cleared 64px lattice tiles, not walls or narrow corridors.
	CombatArea float64
}

// PlanRiftCoverageFloor samples one connected playable region. Very large maps
// deliberately use a bounded region rather than spreading 320 actors over the
// entire map or silently increasing the wire/AI budget. The exit is in that same
// region. Other native map areas remain traversable but are not promised mobs.
func PlanRiftCoverageFloor(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32) (RiftFloorPlan, error) {
	return planRiftCoverageFloor(runID, floor, templates, previousMap, RiftFloorCoverageVersion)
}

// PlanRiftDenseCoverageFloor keeps v4 geometry and adds ordinary encounters to
// min(320, ceil(v4 population * 3/2)), within the existing client aura budget.
func PlanRiftDenseCoverageFloor(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32) (RiftFloorPlan, error) {
	return planRiftCoverageFloor(runID, floor, templates, previousMap, RiftFloorDenseCoverageVersion)
}
func planRiftCoverageFloor(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32, version int32) (RiftFloorPlan, error) {
	if runID == "" || floor < 1 {
		return RiftFloorPlan{}, errors.New("invalid rift floor identity")
	}
	pool := append([]RiftFloorTemplate(nil), templates...)
	sort.Slice(pool, func(i, j int) bool { return pool[i].MapID < pool[j].MapID })
	valid := pool[:0]
	seen := map[int32]bool{}
	for _, t := range pool {
		if !RiftMapExcluded(t.MapID) && t.Valid() && len(t.CombatRegions) > 0 && !seen[t.MapID] {
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
		return RiftFloorPlan{}, errors.New("no verified rift combat regions")
	}
	seed := RiftFloorSeed(runID, floor)
	rng := riftFloorRandom(seed ^ 0x636f766572616734)
	// Invalid custom regions are skipped before admission, deterministically.
	first := rng.index(len(pool))
	for attempt := 0; attempt < len(pool); attempt++ {
		t := pool[(first+attempt)%len(pool)]
		firstRegion := rng.index(len(t.CombatRegions))
		for k := 0; k < len(t.CombatRegions); k++ {
			region := t.CombatRegions[(firstRegion+k)%len(t.CombatRegions)]
			if plan, ok := planRiftCombatRegionVersion(seed, floor, t, region, &rng, version); ok {
				return plan, nil
			}
		}
	}
	return RiftFloorPlan{}, errors.New("no safe rift combat region")
}

func planRiftCombatRegion(seed uint64, floor int32, t RiftFloorTemplate, region RiftCombatRegion, rng *riftFloorRandom) (RiftFloorPlan, bool) {
	return planRiftCombatRegionVersion(seed, floor, t, region, rng, RiftFloorCoverageVersion)
}
func planRiftCombatRegionVersion(seed uint64, floor int32, t RiftFloorTemplate, region RiftCombatRegion, rng *riftFloorRandom, version int32) (RiftFloorPlan, bool) {
	var empty RiftFloorPlan
	if len(region.Population) < 8 || region.CombatArea <= 0 || math.IsNaN(region.CombatArea) || math.IsInf(region.CombatArea, 0) || region.CombatArea > RiftCoverageMaxArea {
		return empty, false
	}
	population := shuffledRiftFloorPoints(region.Population, rng)
	seen := map[Pos]bool{}
	for _, p := range population {
		if !riftFloorPointValid(p.Pos, t.MapID) || seen[p.Pos] {
			return empty, false
		}
		seen[p.Pos] = true
	}
	entry := population[0]
	exit := entry
	for _, p := range population {
		if riftFloorDistance(entry.Pos, p.Pos) > riftFloorDistance(entry.Pos, exit.Pos) {
			exit = p
		}
	}
	if riftFloorDistance(entry.Pos, exit.Pos) < 512*512 {
		return empty, false
	}
	points := make([]SpawnPoint, 0, len(population))
	for _, p := range population {
		if riftFloorDistance(entry.Pos, p.Pos) >= RiftFloorSafeRadius*RiftFloorSafeRadius && riftFloorDistance(exit.Pos, p.Pos) >= 80*80 {
			points = append(points, p)
		}
	}
	if len(points) < 8 {
		return empty, false
	}
	n := int(math.Round(region.CombatArea / RiftCoverageAreaPerMonster))
	if n < RiftCoverageMinPopulation {
		n = RiftCoverageMinPopulation
	}
	if n > RiftFloorMaxConfiguredPopulation {
		n = RiftFloorMaxConfiguredPopulation
	}
	if n > len(points) {
		n = len(points)
	}
	baseN := n
	if version == RiftFloorDenseCoverageVersion {
		n = min(RiftDenseCoverageMaxPopulation, (baseN*3+1)/2)
		// The explicit 320 cap is part of v5; never use an overlapping fallback.
		if n > RiftDenseCoverageMaxPopulation || n > len(points) {
			return empty, false
		}
	}
	points = riftCoveragePackPoints(points, n, rng)
	monsters := append([]MonsterID(nil), t.Monsters...)
	sort.Slice(monsters, func(i, j int) bool { return monsters[i] < monsters[j] })
	guardians := append([]MonsterID(nil), t.Guardians...)
	sort.Slice(guardians, func(i, j int) bool { return guardians[i] < guardians[j] })
	plan := RiftFloorPlan{GeneratorVersion: version, Seed: seed, FloorIndex: floor, MapID: t.MapID, MapName: t.MapName, Entry: entry.Pos, Exit: exit.Pos}
	// Preserve v4 elite counts (at most 24 blue / 8 gold); v5 adds only ordinary
	// monsters. Per-kill progress, rewards and difficulty stats are unchanged.
	blue, gold := baseN*3/40, baseN/40
	if gold == 0 && baseN >= 16 {
		gold = 1
	}
	classes := make([]int32, n)
	for i := 0; i < blue+gold; i++ {
		classes[i] = 1
		if i < gold {
			classes[i] = 2
		}
	}
	for i := n - 1; i > 0; i-- {
		j := rng.index(i + 1)
		classes[i], classes[j] = classes[j], classes[i]
	}
	for i, p := range points {
		p.Monster = monsters[rng.index(len(monsters))]
		plan.Spawns = append(plan.Spawns, RiftFloorSpawn{Point: p, Class: classes[i]})
	}
	exit.Monster = guardians[rng.index(len(guardians))]
	plan.Guardian = RiftFloorSpawn{Point: exit, Class: 3}
	return plan, plan.Valid()
}

// Farthest-first encounter centers cover rooms instead of repeatedly sampling
// the same native cluster. Each center recruits 3-5 nearest safe neighbors.
// Every coordinate is preverified; no jitter or denser fallback grid is used.
func riftCoveragePackPoints(points []SpawnPoint, n int, rng *riftFloorRandom) []SpawnPoint {
	used := make([]bool, len(points))
	distance := make([]float64, len(points))
	for i := range distance {
		distance[i] = math.Inf(1)
	}
	chosen := make([]SpawnPoint, 0, n)
	center := 0
	for len(chosen) < n {
		if len(chosen) > 0 {
			center = -1
			for i := range points {
				if !used[i] && (center < 0 || distance[i] > distance[center]) {
					center = i
				}
			}
		}
		if center < 0 {
			break
		}
		used[center] = true
		chosen = append(chosen, points[center])
		for i, p := range points {
			d := riftFloorDistance(p.Pos, points[center].Pos)
			if d < distance[i] {
				distance[i] = d
			}
		}
		neighbors := make([]int, 0, 32)
		for i, p := range points {
			if !used[i] && riftFloorDistance(p.Pos, points[center].Pos) <= 160*160 {
				neighbors = append(neighbors, i)
			}
		}
		sort.SliceStable(neighbors, func(i, j int) bool {
			return riftFloorDistance(points[neighbors[i]].Pos, points[center].Pos) < riftFloorDistance(points[neighbors[j]].Pos, points[center].Pos)
		})
		count := 3 + rng.index(3)
		for _, i := range neighbors {
			if count == 0 || len(chosen) == n {
				break
			}
			used[i] = true
			chosen = append(chosen, points[i])
			count--
		}
	}
	return chosen
}
