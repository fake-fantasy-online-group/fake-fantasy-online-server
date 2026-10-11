package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sort"
)

const (
	// Changes to the PRNG, ordering or population rules require a new version.
	RiftFloorGeneratorVersion int32 = 1
	RiftFloorMinAnchors             = 18
	RiftFloorSafeRadius             = 160
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

// RiftFloorSeed uses an explicitly versioned, platform-independent byte format.
// It has no dependency on wall time, map iteration or the combat RNG stream.
func RiftFloorSeed(runID string, floor int32) uint64 {
	h := sha256.New()
	h.Write([]byte("fantasy-rift-floor\x00"))
	var b [8]byte
	binary.LittleEndian.PutUint32(b[:4], uint32(RiftFloorGeneratorVersion))
	binary.LittleEndian.PutUint32(b[4:], uint32(floor))
	h.Write(b[:])
	h.Write([]byte(runID))
	return binary.LittleEndian.Uint64(h.Sum(nil)[:8])
}

// riftFloorRandom is SplitMix64, frozen as part of generator version 1.
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
	for _, p := range t.Anchors {
		if !riftFloorPointValid(p.Pos, t.MapID) {
			return false
		}
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
func PlanRiftFloor(runID string, floor int32, templates []RiftFloorTemplate, previousMap int32) (RiftFloorPlan, error) {
	var plan RiftFloorPlan
	if runID == "" || floor < 1 {
		return plan, errors.New("invalid rift floor identity")
	}
	pool := append([]RiftFloorTemplate(nil), templates...)
	sort.Slice(pool, func(i, j int) bool { return pool[i].MapID < pool[j].MapID })
	valid := pool[:0]
	seen := map[int32]bool{}
	for _, t := range pool {
		if t.Valid() && !seen[t.MapID] {
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
	anchors := append([]SpawnPoint(nil), t.Anchors...)
	sort.Slice(anchors, func(i, j int) bool {
		a, b := anchors[i], anchors[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Pos.X != b.Pos.X {
			return a.Pos.X < b.Pos.X
		}
		return a.Pos.Y < b.Pos.Y
	})
	for i := len(anchors) - 1; i > 0; i-- {
		j := rng.index(i + 1)
		anchors[i], anchors[j] = anchors[j], anchors[i]
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
		for _, p := range anchors {
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
	plan = RiftFloorPlan{GeneratorVersion: RiftFloorGeneratorVersion, Seed: seed, FloorIndex: floor, MapID: t.MapID, MapName: t.MapName, Entry: entry.Pos, Exit: exit.Pos}
	n := 40 + rng.index(25)
	if n > len(points) {
		n = len(points)
	}
	blue := 6 + 3*rng.index(3)
	yellow := 2 + rng.index(3)
	if blue+yellow >= n {
		blue = n / 3
		yellow = 2
	}
	for i := 0; i < n; i++ {
		p := points[i]
		p.Monster = monsters[rng.index(len(monsters))]
		class := int32(0)
		if i < yellow {
			class = 2
		} else if i < yellow+blue {
			class = 1
		}
		plan.Spawns = append(plan.Spawns, RiftFloorSpawn{Point: p, Class: class})
	}
	exit.Monster = guardians[rng.index(len(guardians))]
	plan.Guardian = RiftFloorSpawn{Point: exit, Class: 3}
	return plan, nil
}

// Valid checks persisted plan shape. Runtime separately verifies referenced
// templates and every coordinate against its own collision mask before spawning.
func (p RiftFloorPlan) Valid() bool {
	if p.GeneratorVersion != RiftFloorGeneratorVersion || p.FloorIndex < 1 || p.MapID <= 0 || len(p.Spawns) < 8 || len(p.Spawns) > 64 || !riftFloorPointValid(p.Entry, p.MapID) || !riftFloorPointValid(p.Exit, p.MapID) || riftFloorDistance(p.Entry, p.Exit) < 512*512 {
		return false
	}
	if p.Guardian.Class != 3 || p.Guardian.Point.Monster <= 0 || !riftFloorPointValid(p.Guardian.Point.Pos, p.MapID) {
		return false
	}
	for _, s := range p.Spawns {
		if s.Class < 0 || s.Class > 2 || s.Point.Monster <= 0 || !riftFloorPointValid(s.Point.Pos, p.MapID) {
			return false
		}
	}
	return true
}
