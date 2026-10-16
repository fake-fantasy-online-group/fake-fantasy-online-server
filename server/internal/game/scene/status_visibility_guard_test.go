package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestStatusExpiryVisibilityEntityKinds(t *testing.T) {
	for _, kind := range []domain.EntityKind{domain.KindPlayer, domain.KindMonster, domain.KindPet} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			s := newCombatScene(t)
			var e *entity.Entity
			if kind == domain.KindPlayer {
				join(t, s, 1, 100, "expiry", 100, 100)
				e = s.entities[1]
			} else {
				e = putMonster(s, 1, 1000, sureHit, 100, 100)
				e.Kind = kind
			}
			e.Status = domain.NewStatusSet()
			e.Status.Apply(domain.StatusDef{ID: 91001, Level: 1, Invisible: true, DurationSec: 1}, 0, 1)
			e.Status.Apply(domain.StatusDef{ID: 91002, Level: 1, DurationSec: 1, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -1}}}, 0, 1)
			hp := e.HP
			s.outbox = nil
			s.tick = domain.Ticks(1000) - 1
			s.stepStatus()
			if !entityInvisible(e) || e.Status.Count() != 2 || e.HP != hp {
				t.Fatalf("status changed before expiry: invisible=%v count=%d hp=%d want=%d", entityInvisible(e), e.Status.Count(), e.HP, hp)
			}
			s.outbox = nil
			s.tick++
			s.stepStatus()
			if entityInvisible(e) || e.Status.Count() != 0 || e.HP != hp-1 {
				t.Fatalf("final tick or expiry changed: invisible=%v count=%d hp=%d want=%d", entityInvisible(e), e.Status.Count(), e.HP, hp-1)
			}
			visibility, damage, removed := 0, 0, 0
			for _, p := range s.outbox {
				switch ev := p.ev.(type) {
				case event.PlayerVisibilityChanged:
					visibility++
					if ev.Who != e.ID || ev.Invisible {
						t.Fatalf("wrong visibility event: %+v", ev)
					}
				case event.DamageDealt:
					damage++
				case event.StatusRemoved:
					removed++
				}
			}
			want := 0
			if kind == domain.KindPlayer {
				want = 1
			}
			if visibility != want || damage != 1 || removed != 2 {
				t.Fatalf("events visibility=%d want=%d damage=%d removed=%d", visibility, want, damage, removed)
			}
		})
	}
}

// This isolates the common between-tick status stage, with six monsters per
// player. Full-frame measurements also include combat, aura, and event delivery.
func BenchmarkStatusStageMixedVisibility(b *testing.B) {
	for _, n := range []int{30, 300} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := &Scene{entities: make(map[domain.EntityID]*entity.Entity), tick: 1}
			for i := 0; i < n+n/6; i++ {
				e := &entity.Entity{ID: domain.EntityID(i + 1), Kind: domain.KindMonster, Status: domain.NewStatusSet()}
				if i >= n {
					e.Kind = domain.KindPlayer
					e.Player = &entity.Player{}
				}
				e.Status.Apply(domain.StatusDef{ID: domain.StatusPoison, DurationSec: 10000, IntervalSec: 2, HPChanges: []domain.ResourceChange{{Value: -1}}}, 0, 0)
				s.entities[e.ID] = e
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.stepStatus()
			}
		})
	}
}
