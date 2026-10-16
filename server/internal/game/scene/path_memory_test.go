package scene

import (
	"container/heap"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Reference implementation predates the typed heap. Keep it independent so
// exact waypoint ordering and failed searches can be compared after changes.
type referenceMemoryPathQueue []monsterPathQueueItem

func (q referenceMemoryPathQueue) Len() int { return len(q) }
func (q referenceMemoryPathQueue) Less(i, j int) bool {
	if q[i].f != q[j].f {
		return q[i].f < q[j].f
	}
	return q[i].seq < q[j].seq
}
func (q referenceMemoryPathQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *referenceMemoryPathQueue) Push(v any)   { *q = append(*q, v.(monsterPathQueueItem)) }
func (q *referenceMemoryPathQueue) Pop() any {
	old := *q
	n := len(old) - 1
	v := old[n]
	*q = old[:n]
	return v
}

func (s *Scene) referenceMemoryMonsterPath(from, target domain.Pos, radius float64) ([]domain.Pos, bool) {
	if !s.spawnPositionWalkable(from) || !s.spawnPositionWalkable(target) || radius <= 0 {
		return nil, false
	}
	start := monsterPathCell{}
	world := func(c monsterPathCell) domain.Pos {
		return domain.Pos{MapID: from.MapID,
			X: from.X + float64(c.x)*monsterPathGrid,
			Y: from.Y + float64(c.y)*monsterPathGrid}
	}
	heuristic := func(p domain.Pos) float64 { return math.Hypot(target.X-p.X, target.Y-p.Y) }

	open := referenceMemoryPathQueue{{cell: start, f: heuristic(from)}}
	heap.Init(&open)
	gScore := map[monsterPathCell]float64{start: 0}
	cameFrom := make(map[monsterPathCell]monsterPathCell)
	closed := make(map[monsterPathCell]bool)
	radiusSq := radius * radius
	sequence, expanded := 1, 0
	var goal monsterPathCell
	found := false

	for open.Len() > 0 && expanded < monsterPathMaxExpanded {
		item := heap.Pop(&open).(monsterPathQueueItem)
		best, ok := gScore[item.cell]
		if !ok || item.g != best || closed[item.cell] {
			continue
		}
		closed[item.cell] = true
		expanded++
		at := world(item.cell)
		if heuristic(at) <= monsterPathGrid*1.5 && s.straightPathWalkable(at, target) {
			goal, found = item.cell, true
			break
		}

		for _, delta := range monsterPathNeighbours {
			next := monsterPathCell{x: item.cell.x + delta.x, y: item.cell.y + delta.y}
			if closed[next] {
				continue
			}
			nextAt := world(next)
			dx, dy := nextAt.X-from.X, nextAt.Y-from.Y
			if dx*dx+dy*dy > radiusSq || !s.straightPathWalkable(at, nextAt) {
				continue
			}
			step := monsterPathGrid
			if delta.x != 0 && delta.y != 0 {
				step *= math.Sqrt2
			}
			candidate := item.g + step
			if previous, seen := gScore[next]; seen && candidate >= previous {
				continue
			}
			gScore[next] = candidate
			cameFrom[next] = item.cell
			heap.Push(&open, monsterPathQueueItem{
				cell: next, g: candidate, f: candidate + heuristic(nextAt), seq: sequence,
			})
			sequence++
		}
	}
	if !found {
		return nil, false
	}

	cells := []monsterPathCell{goal}
	for cells[len(cells)-1] != start {
		previous, ok := cameFrom[cells[len(cells)-1]]
		if !ok {
			return nil, false
		}
		cells = append(cells, previous)
	}
	points := make([]domain.Pos, 0, len(cells))
	for i := len(cells) - 2; i >= 0; i-- { // 去掉起点，按正向恢复
		points = append(points, world(cells[i]))
	}
	points = append(points, target)
	return s.simplifyMonsterPath(from, points), true
}

func TestMemoryPathQueueMatchesContainerHeap(t *testing.T) {
	for seed := int64(0); seed < 30; seed++ {
		rng := rand.New(rand.NewSource(seed))
		var got monsterPathQueue
		var want referenceMemoryPathQueue
		pop := func() {
			t.Helper()
			g, w := got.pop(), heap.Pop(&want).(monsterPathQueueItem)
			if g != w || !reflect.DeepEqual([]monsterPathQueueItem(got), []monsterPathQueueItem(want)) {
				t.Fatalf("seed %d: heap differs: %v != %v", seed, g, w)
			}
		}
		for i := 0; i < 3000; i++ {
			if len(got) > 0 && rng.Intn(3) == 0 {
				pop()
				continue
			}
			// Duplicate f and sequence values also exercise exact tie behavior.
			v := monsterPathQueueItem{cell: monsterPathCell{x: i}, g: float64(i), f: float64(rng.Intn(20)), seq: rng.Intn(30)}
			got.push(v)
			heap.Push(&want, v)
		}
		for len(got) > 0 {
			pop()
		}
	}
}

func memoryPathScene(blocked bool) *Scene {
	s := &Scene{}
	s.now = func() time.Time { return time.UnixMilli(100000) }
	endY := 140.0
	if blocked {
		endY = 1000
	}
	s.temporaryWalls = map[string]temporaryWall{"a": temporaryTestWall("a", 100, 60, 100, endY)}
	if blocked {
		w := s.temporaryWalls["a"]
		w.Y = -1000
		s.temporaryWalls["a"] = w
	}
	return s
}

func TestMemoryMonsterPathMatchesReference(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		s := memoryPathScene(blocked)
		for x := -20.; x <= 60; x += 20 {
			for y := 0.; y <= 200; y += 20 {
				from, to := temporaryTestPos(x, y), temporaryTestPos(180, 100)
				got, gok := s.localMonsterPath(from, to, 200)
				want, wok := s.referenceMemoryMonsterPath(from, to, 200)
				if gok != wok || !reflect.DeepEqual(got, want) {
					t.Fatalf("blocked=%v from=%v: got %v/%v want %v/%v", blocked, from, got, gok, want, wok)
				}
			}
		}
	}
}

func BenchmarkMemoryMonsterPath(b *testing.B) {
	for _, scenario := range []struct {
		name    string
		blocked bool
	}{{"Detour", false}, {"Unreachable", true}} {
		b.Run(scenario.name, func(b *testing.B) {
			for _, reference := range []bool{true, false} {
				name := "Typed"
				if reference {
					name = "ContainerHeap"
				}
				b.Run(name, func(b *testing.B) {
					s := memoryPathScene(scenario.blocked)
					from, to := temporaryTestPos(0, 100), temporaryTestPos(180, 100)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var path []domain.Pos
						var ok bool
						if reference {
							path, ok = s.referenceMemoryMonsterPath(from, to, 200)
						} else {
							path, ok = s.localMonsterPath(from, to, 200)
						}
						if ok == scenario.blocked || ok && len(path) < 2 {
							b.Fatal("unexpected search result")
						}
					}
				})
			}
		})
	}
}
