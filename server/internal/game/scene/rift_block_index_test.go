package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
	"testing"
)

func TestRiftVisibilityBlockBoundaries(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		want int64
	}{{0, 0}, {299.999, 0}, {300, 1}, {-0.001, -1}, {-300, -1}, {-300.001, -2}, {math.MaxInt32, 7158278}, {math.MinInt32, -7158279}} {
		if got := visibilityBlock(domain.Pos{X: tc.v}).x; got != tc.want {
			t.Fatalf("%v: got %d want %d", tc.v, got, tc.want)
		}
	}
	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		b := visibilityBlock(domain.Pos{X: v})
		if b.x < -7158279 || b.x > 7158278 {
			t.Fatal(b)
		}
	}
}
func TestRiftBlockIndexViewportClampedCoverage(t *testing.T) {
	for _, size := range [][2]float64{{12000, 9000}, {1000, 500}, {2560, 1440}, {2700, 1600}} {
		idx := newRiftBlockIndex(size[0], size[1], nil)
		for _, p := range []domain.Pos{{}, {X: size[0], Y: size[1]}, {X: size[0] / 2, Y: size[1] / 2}, {X: 299.99, Y: 299.99}} {
			idx.subscribe(1, p)
			w := idx.windows[1]
			clamp := func(v, extent, half float64) float64 {
				if extent <= 2*half {
					return extent / 2
				}
				return math.Max(half, math.Min(extent-half, v))
			}
			cx, cy := clamp(p.X, size[0], 1280), clamp(p.Y, size[1], 720)
			for _, x := range []float64{math.Max(0, cx-1880), math.Min(size[0], cx+1880)} {
				for _, y := range []float64{math.Max(0, cy-1320), math.Min(size[1], cy+1320)} {
					if !w.contains(visibilityBlock(domain.Pos{X: x, Y: y})) {
						t.Fatalf("size=%v p=%+v misses %v,%v window=%+v", size, p, x, y, w)
					}
				}
			}
			if w.max.x-w.min.x != 14 || w.max.y-w.min.y != 10 {
				t.Fatalf("wrong fixed block window %+v", w)
			}
		}
	}
}
func TestRiftBlockIndexStationaryObserverAndIncarnation(t *testing.T) {
	type change struct {
		id      domain.EntityID
		e       *entity.Entity
		visible bool
	}
	var changes []change
	idx := newRiftBlockIndex(20000, 16000, func(id domain.EntityID, e *entity.Entity, v bool) { changes = append(changes, change{id, e, v}) })
	p := domain.Pos{X: 6000, Y: 6000}
	idx.subscribe(1, p)
	m := &entity.Entity{ID: 9, Pos: domain.Pos{X: 8399, Y: 6000}}
	idx.updateEntity(m, true)
	if len(changes) != 1 || !changes[0].visible {
		t.Fatal(changes)
	}
	changes = nil
	m.Pos.X = 8398
	idx.updateEntity(m, true)
	if len(changes) != 0 {
		t.Fatal("same block re-evaluated")
	}
	m.Pos.X = 8400
	idx.updateEntity(m, true)
	if len(changes) != 1 || changes[0].visible {
		t.Fatal(changes)
	}
	changes = nil
	m.Pos.X = 8399
	idx.updateEntity(m, true)
	if len(changes) != 1 || !changes[0].visible {
		t.Fatal(changes)
	}
	changes = nil
	replacement := &entity.Entity{ID: m.ID, Pos: m.Pos}
	idx.updateEntity(replacement, true)
	if len(changes) != 2 || changes[0].e != m || changes[0].visible || changes[1].e != replacement || !changes[1].visible {
		t.Fatal(changes)
	}
	changes = nil
	idx.updateEntity(m, false)
	if len(changes) != 0 || idx.entities[m.ID].entity != replacement {
		t.Fatal("stale removal erased replacement")
	}
	idx.updateEntity(replacement, false)
	if len(changes) != 1 || changes[0].visible {
		t.Fatal(changes)
	}
	idx.unsubscribe(1)
	if len(idx.entities) != 0 || len(idx.blocks) != 0 || len(idx.windows) != 0 || len(idx.subscribers) != 0 {
		t.Fatal("retained empty index")
	}
}
func TestRiftBlockIndexFivePlayersMovementAndTeleport(t *testing.T) {
	views := map[domain.EntityID]map[domain.EntityID]bool{}
	idx := newRiftBlockIndex(20000, 16000, func(p domain.EntityID, e *entity.Entity, v bool) {
		if views[p] == nil {
			views[p] = map[domain.EntityID]bool{}
		}
		if v {
			views[p][e.ID] = true
		} else {
			delete(views[p], e.ID)
		}
	})
	for n := 1; n <= 500; n++ {
		idx.updateEntity(&entity.Entity{ID: domain.EntityID(100 + n), Pos: domain.Pos{X: float64((n * 173) % 20000), Y: float64((n * 359) % 16000)}}, true)
	}
	for step := 0; step < 50; step++ {
		for p := 1; p <= 5; p++ {
			id := domain.EntityID(p)
			pos := domain.Pos{X: float64((step*317 + p*1300) % 20000), Y: float64((step*173 + p*1700) % 16000)}
			idx.subscribe(id, pos)
			expected := 0
			for eid, e := range idx.entities {
				want := idx.visible(id, e.entity)
				if views[id][eid] != want {
					t.Fatalf("step %d player %d entity %d", step, p, eid)
				}
				if want {
					expected++
				}
			}
			enumerated := 0
			idx.eachVisible(id, func(*entity.Entity) { enumerated++ })
			if expected != enumerated {
				t.Fatal("bad enumeration")
			}
		}
	}
	for p := 1; p <= 5; p++ {
		idx.unsubscribe(domain.EntityID(p))
	}
	if len(idx.subscribers) != 0 {
		t.Fatal("subscription leak")
	}
}
func TestRiftBlockAOILifecycleHook(t *testing.T) {
	var changes int
	idx := newRiftBlockIndex(20000, 16000, func(domain.EntityID, *entity.Entity, bool) { changes++ })
	idx.subscribe(1, domain.Pos{X: 6000, Y: 6000})
	a := newAOI(10000)
	a.onChange = idx.updateEntity
	m := &entity.Entity{ID: 2, Pos: domain.Pos{X: 8399, Y: 6000}}
	a.Enter(m)
	a.Move(m, domain.Pos{X: 8400, Y: 6000})
	a.Move(m, domain.Pos{X: 8399, Y: 6000})
	a.Leave(m)
	if changes != 4 {
		t.Fatalf("got %d transitions, want 4", changes)
	}
}

func TestRiftBlockUnknownBoundsCameraEnvelope(t *testing.T) {
	idx := newRiftBlockIndex(0, 0, nil)
	for _, p := range []domain.Pos{{}, {X: 299.999, Y: 299.999}, {X: 300, Y: 300}, {X: 10000, Y: 10000}, {X: math.MaxInt32 - 4000, Y: math.MaxInt32 - 4000}} {
		idx.subscribe(1, p)
		w := idx.windows[1]
		if w.max.x-w.min.x != 22 || w.max.y-w.min.y != 14 {
			t.Fatalf("unknown bounds window %+v", w)
		}
		for _, dx := range []float64{-3160, 3160} {
			for _, dy := range []float64{-2040, 2040} {
				if !w.contains(visibilityBlock(domain.Pos{X: p.X + dx, Y: p.Y + dy})) {
					t.Fatalf("unknown camera envelope misses p=%+v delta=%v,%v", p, dx, dy)
				}
			}
		}
	}
}

func TestRiftBlockIndexBoundedGlobalFallback(t *testing.T) {
	views := map[domain.EntityID]bool{}
	changes := 0
	idx := newRiftBlockIndex(0, 0, func(_ domain.EntityID, e *entity.Entity, v bool) {
		changes++
		if v {
			views[e.ID] = true
		} else {
			delete(views, e.ID)
		}
	})
	near := &entity.Entity{ID: 2, Pos: domain.Pos{X: 6000, Y: 6000}}
	far := &entity.Entity{ID: 3, Pos: domain.Pos{X: 90000, Y: 90000}}
	idx.updateEntity(near, true)
	idx.updateEntity(far, true)
	idx.subscribe(1, near.Pos)
	if len(views) != 1 {
		t.Fatal(views)
	}
	idx.subscribe(1, near.Pos, math.MaxFloat64)
	if !idx.windows[1].global || len(idx.subscribers) != 0 || len(views) != 2 {
		t.Fatal("not bounded/global", idx.windows[1], len(idx.subscribers), views)
	}
	changes = 0
	idx.subscribe(1, far.Pos, math.Inf(1))
	if changes != 0 {
		t.Fatal("full-mode movement re-enumerated")
	}
	far.Pos.X = 100000
	idx.updateEntity(far, true)
	if changes != 0 {
		t.Fatal("full-mode monster movement changed membership")
	}
	count := 0
	idx.eachVisible(1, func(*entity.Entity) { count++ })
	if count != 2 {
		t.Fatal(count)
	}
	idx.updateEntity(far, false)
	if len(views) != 1 {
		t.Fatal(views)
	}
	idx.updateEntity(far, true)
	if len(views) != 2 {
		t.Fatal(views)
	}
	idx.subscribe(1, near.Pos)
	if len(views) != 1 || !views[near.ID] || len(idx.global) != 0 {
		t.Fatal("failed full-to-local reconciliation", views)
	}
	idx.subscribe(1, near.Pos, math.MaxFloat64)
	idx.unsubscribe(1)
	if len(views) != 0 || len(idx.global) != 0 || len(idx.windows) != 0 || len(idx.subscribers) != 0 {
		t.Fatal("retained observer")
	}
}
func TestRiftReplicationMovementBuffer(t *testing.T) {
	s := New(Config{})
	if got := s.replicationMovementBuffer(nil); got != 0 {
		t.Fatal(got)
	}
	p := &entity.Entity{Player: &entity.Player{}}
	p.Stats.MoveSpeed = 100
	if got := s.replicationMovementBuffer(p); got != 0 {
		t.Fatalf("normal speed should fit nominal600: %v", got)
	}
	p.Stats.MoveSpeed = 1000
	speed := float64(s.playerMoveSpeedPX(p))
	want := speed*((clientMoveReportInterval*clientMoveWindowPeriods).Seconds()*clientMoveCatchUpMul+2*float64(domain.TickMS)/1000) + clientMoveLagTolerancePX - 600
	if got := s.replicationMovementBuffer(p); got != want {
		t.Fatalf("got %v want %v", got, want)
	}
	idx := newRiftBlockIndex(0, 0, nil)
	idx.subscribe(1, domain.Pos{}, want)
	w := idx.windows[1]
	if !w.contains(visibilityBlock(domain.Pos{X: 2560 + 600 + want, Y: 1440 + 600 + want})) {
		t.Fatal("high-speed margin omitted")
	}
	p.Stats.MoveSpeed = math.MaxInt32
	idx.subscribe(1, domain.Pos{}, s.replicationMovementBuffer(p))
	if !idx.windows[1].global {
		t.Fatal("extreme speed not bounded")
	}
}
