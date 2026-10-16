package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
	"slices"
	"testing"
)

func spatialFixture(players, others int) (*Scene, *entity.Entity, []*entity.Entity) {
	s, m := targetSelectionFixture(players)
	all := make([]*entity.Entity, 0, players+others)
	for _, e := range s.entities {
		all = append(all, e)
	}
	for i := 0; i < others; i++ {
		e := &entity.Entity{ID: domain.EntityID(1000 + i), Kind: domain.KindMonster, HP: 100, MaxHP: 100, Pos: domain.Pos{X: 100 + float64(i%200), Y: 100 + float64(i%100)}, Monster: &entity.Monster{}}
		s.aoi.Enter(e)
		s.entities[e.ID] = e
		all = append(all, e)
	}
	slices.SortFunc(all, func(a, b *entity.Entity) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return s, m, all
}
func BenchmarkSpatialAcquire(b *testing.B) {
	for _, cfg := range [][2]int{{1, 20}, {8, 100}, {8, 1000}, {64, 2000}, {256, 0}} {
		b.Run(fmt.Sprintf("p%d_other%d", cfg[0], cfg[1]), func(b *testing.B) {
			s, m, _ := spatialFixture(cfg[0], cfg[1])
			p := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.Monster.Target = 0
				s.acquire(m, p)
			}
		})
	}
}
func BenchmarkSpatialMixedTick(b *testing.B) {
	for _, cfg := range [][2]int{{1, 20}, {8, 100}, {8, 1000}, {64, 2000}} {
		b.Run(fmt.Sprintf("p%d_other%d", cfg[0], cfg[1]), func(b *testing.B) {
			s, m, all := spatialFixture(cfg[0], cfg[1])
			p := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ { // A whole scene cycle: all entities move, 10% cross cells, 10% of monsters acquire.
				for j, e := range all {
					to := e.Pos
					if j%10 == 0 {
						if to.X < 300 {
							to.X += 300
						} else {
							to.X -= 300
						}
					} else {
						to.Y = 100 + float64(i%20)
					}
					s.aoi.Move(e, to)
				}
				for j := 0; j < cfg[1]/10; j++ {
					m.Monster.Target = 0
					s.acquire(m, p)
				}
			}
		})
	}
}
func BenchmarkSpatialIndexBuild(b *testing.B) {
	for _, cfg := range [][2]int{{1, 20}, {8, 100}, {64, 2000}} {
		b.Run(fmt.Sprintf("p%d_other%d", cfg[0], cfg[1]), func(b *testing.B) {
			_, _, all := spatialFixture(cfg[0], cfg[1])
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a := newAOI(300)
				for _, e := range all {
					a.Enter(e)
				}
			}
		})
	}
}
func TestSpatialLifecycle(t *testing.T) {
	s, m, all := spatialFixture(8, 100)
	p := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
	check := func() {
		t.Helper()
		*m.Monster = entity.Monster{}
		s.acquireFullSortReference(m, p)
		want := *m.Monster
		*m.Monster = entity.Monster{}
		s.acquire(m, p)
		if !reflect.DeepEqual(want, *m.Monster) {
			t.Fatalf("target state mismatch got %+v want %+v", *m.Monster, want)
		}
	}
	check()
	for _, e := range all {
		s.aoi.Move(e, domain.Pos{X: -650, Y: -650})
		check()
		s.aoi.Move(e, domain.Pos{X: 150, Y: 150})
		check()
		s.aoi.Leave(e)
		check()
		s.aoi.Enter(e)
		check()
	}
	other := newAOI(300)
	for _, e := range all {
		s.aoi.Leave(e)
		other.Enter(e)
	}
	check()
	n := 0
	s.aoi.AroundPos(m.Pos, func(*entity.Entity) { n++ })
	if n != 0 {
		t.Fatal("old scene leaked entities")
	}
}

func BenchmarkSpatialMovement(b *testing.B) {
	for _, cfg := range [][2]int{{1, 20}, {8, 100}, {8, 1000}, {64, 2000}} {
		b.Run(fmt.Sprintf("p%d_other%d", cfg[0], cfg[1]), func(b *testing.B) {
			s, _, all := spatialFixture(cfg[0], cfg[1])
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j, e := range all {
					to := e.Pos
					if j%10 == 0 {
						if to.X < 300 {
							to.X += 300
						} else {
							to.X -= 300
						}
					} else {
						to.Y = 100 + float64(i%20)
					}
					s.aoi.Move(e, to)
				}
			}
		})
	}
}

func TestSpatialIDReuse(t *testing.T) {
	s, m, _ := spatialFixture(1, 0)
	p := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
	check := func() {
		*m.Monster = entity.Monster{}
		s.acquireFullSortReference(m, p)
		want := *m.Monster
		*m.Monster = entity.Monster{}
		s.acquire(m, p)
		if !reflect.DeepEqual(want, *m.Monster) {
			t.Fatalf("got %+v want %+v", *m.Monster, want)
		}
	}
	for i := 0; i < 20; i++ {
		kind := domain.KindMonster
		if i%2 == 0 {
			kind = domain.KindPlayer
		}
		e := &entity.Entity{ID: 1, Kind: kind, HP: 100, MaxHP: 100, Pos: domain.Pos{X: 101, Y: 100}}
		s.entities[1] = e
		s.aoi.Enter(e)
		check()
		s.aoi.Leave(e)
		check()
		s.aoi.Enter(e)
		check()
	}
}

func TestPlayerIndexBoundsAndReplacement(t *testing.T) {
	a := newAOI(300)
	// Preserve the original truncation toward zero, including negative boundaries.
	for i, x := range []float64{-901, -600, -301, -300, -299, -1, 0, 299, 300, 599, 600, 901} {
		for j, y := range []float64{-600, -1, 0, 300, 600} {
			id := domain.EntityID(i*10 + j + 1)
			a.Enter(&entity.Entity{ID: id, Kind: domain.KindPlayer, Pos: domain.Pos{X: x, Y: y}})
		}
	}
	check := func() {
		t.Helper()
		for _, pos := range []domain.Pos{{}, {X: -301, Y: -301}, {X: 300, Y: 300}, {X: 900, Y: 900}} {
			want, got := map[domain.EntityID]*entity.Entity{}, map[domain.EntityID]*entity.Entity{}
			a.AroundPos(pos, func(e *entity.Entity) {
				if e.Kind == domain.KindPlayer {
					want[e.ID] = e
				}
			})
			a.AroundPlayers(pos, func(e *entity.Entity) {
				if got[e.ID] != nil {
					t.Fatalf("duplicate player %d", e.ID)
				}
				got[e.ID] = e
			})
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("player bounds mismatch at %+v", pos)
			}
		}
	}
	check()
	for _, kind := range []domain.EntityKind{domain.KindMonster, domain.KindPlayer, domain.KindMonster, domain.KindPlayer} {
		replacement := &entity.Entity{ID: 1, Kind: kind, Pos: domain.Pos{X: 900, Y: 900}}
		a.Enter(replacement)
		check()
		count := 0
		for _, bucket := range a.cells {
			if bucket[1] != nil {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("replacement occupies %d cells", count)
		}
		a.Move(replacement, domain.Pos{X: 901, Y: 901}) // Within-cell move.
		check()
		a.Move(replacement, domain.Pos{X: -301, Y: -301}) // Cross-cell move.
		check()
		replacement.Kind = domain.KindMonster // Leave must remove indexed ID regardless of current kind.
		a.Leave(replacement)
		check()
		if _, ok := a.CellOf(1); ok {
			t.Fatal("leave retained location")
		}
	}
}

func TestPlayerIndexDeathReviveAndTransfer(t *testing.T) {
	s, m := targetSelectionFixture(1)
	p := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
	player := s.entities[1]
	player.Player = &entity.Player{}
	check := func(want domain.EntityID) {
		t.Helper()
		*m.Monster = entity.Monster{}
		s.acquire(m, p)
		if m.Monster.Target != want {
			t.Fatalf("target = %d, want %d", m.Monster.Target, want)
		}
	}
	check(1)
	player.HP = 0
	check(0)
	player.HP = 100
	player.Player.Protect(s.tick + 10)
	check(0)
	s.tick += 10
	check(1)
	destination := newAOI(300)
	s.aoi.Leave(player)
	destination.Enter(player)
	check(0)
	destination.Leave(player)
	s.aoi.Enter(player)
	check(1)
	s.aoi.Leave(player)
	if len(s.aoi.players) != 0 || len(s.aoi.cells) != 0 || len(s.aoi.at) != 0 {
		t.Fatal("empty cells or IDs retained after leave")
	}
}
