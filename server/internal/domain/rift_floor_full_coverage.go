package domain

import (
	"container/heap"
	"errors"
	"math"
	"sort"
)

// Version 6 keeps the whole native combat component. The existing 150% density
// is applied once to its area, never a second time to v5 output.
const RiftFloorFullCoverageVersion int32 = 6

func PlanRiftFullCoverageFloor(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32) (RiftFloorPlan, error) {
	if runID == "" || floor < 1 {
		return RiftFloorPlan{}, errors.New("invalid rift floor identity")
	}
	pool := append([]RiftFloorTemplate(nil), templates...)
	sort.Slice(pool, func(i, j int) bool { return pool[i].MapID < pool[j].MapID })
	valid := pool[:0]
	seen := map[int32]bool{}
	for _, layout := range pool {
		if !RiftMapExcluded(layout.MapID) && layout.Valid() && len(layout.FullCombatRegions) > 0 && !seen[layout.MapID] {
			valid = append(valid, layout)
			seen[layout.MapID] = true
		}
	}
	pool = valid
	if len(pool) > 1 {
		valid = pool[:0]
		for _, layout := range pool {
			if layout.MapID != previousMap {
				valid = append(valid, layout)
			}
		}
		pool = valid
	}
	if len(pool) == 0 {
		return RiftFloorPlan{}, errors.New("no verified full rift combat regions")
	}
	seed := RiftFloorSeed(runID, floor)
	rng := riftFloorRandom(seed ^ 0x66756c6c636f7636)
	first := rng.index(len(pool))
	for attempt := range pool {
		layout := pool[(first+attempt)%len(pool)]
		firstRegion := rng.index(len(layout.FullCombatRegions))
		for k := range layout.FullCombatRegions {
			region := layout.FullCombatRegions[(firstRegion+k)%len(layout.FullCombatRegions)]
			if plan, ok := planRiftFullCombatRegion(seed, floor, layout, region, &rng); ok {
				return plan, nil
			}
		}
	}
	return RiftFloorPlan{}, errors.New("no safe full rift combat region")
}

func planRiftFullCombatRegion(seed uint64, floor int32, layout RiftFloorTemplate, region RiftCombatRegion, rng *riftFloorRandom) (RiftFloorPlan, bool) {
	var empty RiftFloorPlan
	if len(region.Population) < 8 || region.CombatArea <= 0 || math.IsNaN(region.CombatArea) || math.IsInf(region.CombatArea, 0) {
		return empty, false
	}
	population := shuffledRiftFloorPoints(region.Population, rng)
	spacing := riftFullSpacingIndex{points: make(map[[2]float64]Pos, len(population))}
	for _, point := range population {
		if !riftFloorPointValid(point.Pos, layout.MapID) || !spacing.add(point.Pos) {
			return empty, false
		}
	}
	entry, exit := population[0], population[0]
	for _, point := range population {
		if riftFloorDistance(entry.Pos, point.Pos) > riftFloorDistance(entry.Pos, exit.Pos) {
			exit = point
		}
	}
	if riftFloorDistance(entry.Pos, exit.Pos) < 512*512 {
		return empty, false
	}
	points := make([]SpawnPoint, 0, len(population))
	for _, point := range population {
		if riftFloorDistance(entry.Pos, point.Pos) >= RiftFloorSafeRadius*RiftFloorSafeRadius && riftFloorDistance(exit.Pos, point.Pos) >= 80*80 {
			points = append(points, point)
		}
	}
	if len(points) < 8 {
		return empty, false
	}
	base := math.Max(RiftCoverageMinPopulation, math.Round(region.CombatArea/RiftCoverageAreaPerMonster))
	// Clip only to real safe coordinates, before float-to-int conversion. No
	// population cap, overlapping fallback or overflow from custom area metadata.
	n := len(points)
	if target := math.Ceil(base * 1.5); target < float64(n) {
		n = int(target)
	}
	points = riftFullCoveragePackPoints(points, n, rng)
	monsters := append([]MonsterID(nil), layout.Monsters...)
	guardians := append([]MonsterID(nil), layout.Guardians...)
	sort.Slice(monsters, func(i, j int) bool { return monsters[i] < monsters[j] })
	sort.Slice(guardians, func(i, j int) bool { return guardians[i] < guardians[j] })
	plan := RiftFloorPlan{GeneratorVersion: RiftFloorFullCoverageVersion, Seed: seed, FloorIndex: floor, MapID: layout.MapID, MapName: layout.MapName, Entry: entry.Pos, Exit: exit.Pos}
	// Keep the existing elite skill/aura budget. Only ordinary encounters scale
	// past it; the 24 blue / 8 gold composition is not a total population limit.
	eliteBase := math.Min(base, float64(n))
	blue := int(math.Min(24, math.Floor(eliteBase*3/40)))
	gold := int(math.Min(8, math.Floor(eliteBase/40)))
	if gold == 0 && eliteBase >= 16 {
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
	plan.Spawns = make([]RiftFloorSpawn, 0, n)
	for i, point := range points {
		point.Monster = monsters[rng.index(len(monsters))]
		plan.Spawns = append(plan.Spawns, RiftFloorSpawn{Point: point, Class: classes[i]})
	}
	exit.Monster = guardians[rng.index(len(guardians))]
	plan.Guardian = RiftFloorSpawn{Point: exit, Class: 3}
	return plan, plan.Valid()
}

// Float-valued cells avoid integer overflow for finite persisted coordinates.
// Only neighboring cells can violate spacing. Legacy validation is unchanged.
type riftFullSpacingIndex struct {
	points map[[2]float64]Pos
	extras map[[2]float64][]Pos
}

func (index *riftFullSpacingIndex) add(point Pos) bool {
	key := [2]float64{math.Floor(point.X / RiftFloorPopulationSpacing), math.Floor(point.Y / RiftFloorPopulationSpacing)}
	for y := -1; y <= 1; y++ {
		for x := -1; x <= 1; x++ {
			cell := [2]float64{key[0] + float64(x), key[1] + float64(y)}
			if other, ok := index.points[cell]; ok && riftFloorDistance(point, other) < RiftFloorPopulationSpacing*RiftFloorPopulationSpacing {
				return false
			}
			for _, other := range index.extras[cell] {
				if riftFloorDistance(point, other) < RiftFloorPopulationSpacing*RiftFloorPopulationSpacing {
					return false
				}
			}
		}
	}
	if index.points == nil {
		index.points = make(map[[2]float64]Pos)
	}
	if _, exists := index.points[key]; exists {
		// Native lattice tiles need one point per bucket. Preserve general finite
		// coordinate support without allocating a slice for every native tile.
		if index.extras == nil {
			index.extras = make(map[[2]float64][]Pos)
		}
		index.extras[key] = append(index.extras[key], point)
	} else {
		index.points[key] = point
	}
	return true
}

func (plan RiftFloorPlan) validRiftFullCoverage() bool {
	if plan.FloorIndex < 1 || plan.MapID <= 0 || len(plan.Spawns) < 8 || !riftFloorPointValid(plan.Entry, plan.MapID) || !riftFloorPointValid(plan.Exit, plan.MapID) || riftFloorDistance(plan.Entry, plan.Exit) < 512*512 {
		return false
	}
	if plan.Guardian.Class != 3 || plan.Guardian.Point.Monster <= 0 || !riftFloorPointValid(plan.Guardian.Point.Pos, plan.MapID) || plan.Guardian.Point.Pos != plan.Exit {
		return false
	}
	spacing := riftFullSpacingIndex{points: make(map[[2]float64]Pos, len(plan.Spawns))}
	blue, gold := 0, 0
	for _, spawn := range plan.Spawns {
		if spawn.Class == 1 {
			blue++
		}
		if spawn.Class == 2 {
			gold++
		}
		if blue > 24 || gold > 8 {
			return false
		}
		point := spawn.Point.Pos
		if spawn.Class < 0 || spawn.Class > 2 || spawn.Point.Monster <= 0 || !riftFloorPointValid(point, plan.MapID) || riftFloorDistance(point, plan.Entry) < RiftFloorSafeRadius*RiftFloorSafeRadius || riftFloorDistance(point, plan.Exit) < 80*80 || !spacing.add(point) {
			return false
		}
	}
	return true
}

const riftFullPackBucketSize = 512

type riftFullPackBucket struct {
	points          []int
	best, heapIndex int
	distance        float64
}
type riftFullPackHeap []*riftFullPackBucket

func (h riftFullPackHeap) Len() int { return len(h) }
func (h riftFullPackHeap) Less(i, j int) bool {
	if h[i].distance != h[j].distance {
		return h[i].distance > h[j].distance
	}
	return h[i].best < h[j].best
}
func (h riftFullPackHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}
func (h *riftFullPackHeap) Push(x any) {
	b := x.(*riftFullPackBucket)
	b.heapIndex = len(*h)
	*h = append(*h, b)
}
func (h *riftFullPackHeap) Pop() any {
	old := *h
	b := old[len(old)-1]
	*h = old[:len(old)-1]
	return b
}

// Exact farthest-first encounter centers, without a candidate-wide scan per
// pack. Spatial buckets and a maximum-distance heap only update rooms that a
// new center can improve. Every pack recruits 3-5 nearest safe neighbors.
func riftFullCoveragePackPoints(points []SpawnPoint, n int, rng *riftFloorRandom) []SpawnPoint {
	byCell := map[[2]float64]*riftFullPackBucket{}
	buckets := make([]*riftFullPackBucket, 0)
	pointBucket := make([]*riftFullPackBucket, len(points))
	distances := make([]float64, len(points))
	used := make([]bool, len(points))
	for i, point := range points {
		key := [2]float64{math.Floor(point.Pos.X / riftFullPackBucketSize), math.Floor(point.Pos.Y / riftFullPackBucketSize)}
		bucket := byCell[key]
		if bucket == nil {
			bucket = &riftFullPackBucket{best: i, distance: math.Inf(1)}
			byCell[key] = bucket
			buckets = append(buckets, bucket)
		}
		bucket.points = append(bucket.points, i)
		pointBucket[i] = bucket
		distances[i] = math.Inf(1)
	}
	h := riftFullPackHeap(append([]*riftFullPackBucket(nil), buckets...))
	for i, bucket := range h {
		bucket.heapIndex = i
	}
	heap.Init(&h)
	recompute := func(bucket *riftFullPackBucket) {
		bucket.best, bucket.distance = -1, -1
		for _, i := range bucket.points {
			if !used[i] && (bucket.best < 0 || distances[i] > bucket.distance || distances[i] == bucket.distance && i < bucket.best) {
				bucket.best, bucket.distance = i, distances[i]
			}
		}
		heap.Fix(&h, bucket.heapIndex)
	}
	visit := func(point Pos, radius float64, fn func(*riftFullPackBucket)) {
		minX, maxX := math.Floor((point.X-radius)/riftFullPackBucketSize), math.Floor((point.X+radius)/riftFullPackBucketSize)
		minY, maxY := math.Floor((point.Y-radius)/riftFullPackBucketSize), math.Floor((point.Y+radius)/riftFullPackBucketSize)
		nx, ny := maxX-minX+1, maxY-minY+1
		// Sparse or extreme coordinates cannot trigger an enormous grid loop.
		if math.IsInf(radius, 0) || math.IsNaN(nx*ny) || nx*ny > float64(len(buckets))*4 {
			for _, bucket := range buckets {
				fn(bucket)
			}
			return
		}
		for y := 0; y < int(ny); y++ {
			for x := 0; x < int(nx); x++ {
				if bucket := byCell[[2]float64{minX + float64(x), minY + float64(y)}]; bucket != nil {
					fn(bucket)
				}
			}
		}
	}
	chosen := make([]SpawnPoint, 0, n)
	for len(chosen) < n && h[0].best >= 0 {
		center, radius := h[0].best, math.Sqrt(h[0].distance)
		used[center] = true
		chosen = append(chosen, points[center])
		neighbors := make([]int, 0, 32)
		visit(points[center].Pos, 160, func(bucket *riftFullPackBucket) {
			for _, i := range bucket.points {
				if !used[i] && riftFloorDistance(points[i].Pos, points[center].Pos) <= 160*160 {
					neighbors = append(neighbors, i)
				}
			}
		})
		sort.Slice(neighbors, func(i, j int) bool {
			a, b := riftFloorDistance(points[neighbors[i]].Pos, points[center].Pos), riftFloorDistance(points[neighbors[j]].Pos, points[center].Pos)
			if a != b {
				return a < b
			}
			return neighbors[i] < neighbors[j]
		})
		count := 3 + rng.index(3)
		touched := map[*riftFullPackBucket]bool{pointBucket[center]: true}
		for _, i := range neighbors {
			if count == 0 || len(chosen) == n {
				break
			}
			used[i] = true
			chosen = append(chosen, points[i])
			touched[pointBucket[i]] = true
			count--
		}
		visit(points[center].Pos, radius, func(bucket *riftFullPackBucket) {
			changed := touched[bucket]
			for _, i := range bucket.points {
				if !used[i] {
					if d := riftFloorDistance(points[i].Pos, points[center].Pos); d < distances[i] {
						distances[i] = d
						changed = true
					}
				}
			}
			if changed {
				recompute(bucket)
			}
			delete(touched, bucket)
		})
		// When most candidates are occupied, distance may be below the pack radius.
		for bucket := range touched {
			recompute(bucket)
		}
	}
	return chosen
}
