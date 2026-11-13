package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestPeriodicMonsterDamageRejoinKeepsHPAndExpiryConsistent(t *testing.T) {
	for _, legendary := range []bool{false, true} {
		name := "native"
		if legendary {
			name = "legendary-proc-origin"
		}
		t.Run(name, func(t *testing.T) {
			s, mid, _ := statusScene(t)
			monster := s.entities[mid]
			monster.HP = 10000
			monster.MaxHP = 10000
			d := domain.StatusDef{ID: 1900019221, Level: 1, Name: "流血", TypeKnown: true, Type: domain.StatusBad, DurationSec: 6, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -100}}, SynergyDOTVersion: 1, SynergyDOTPhysical: true}
			if legendary {
				d.LegendaryPowerID = "ZS-A01"
				d.SynergyDOTNew = true
			}
			if !s.applyStatusDef(monster, d, 1) {
				t.Fatal("status admission failed")
			}
			var admitted domain.Active
			monster.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == d.ID {
					admitted = a
				}
			})
			s.onLeave(Leave{ID: 1})
			rejoined := join(t, s, 22, 100, "rejoined", 100, 100)
			s.flush()
			rejoined.take()
			s.tick = admitted.NextTickAt
			s.stepStatus()
			s.flush()
			got, ok := firstOf[event.DamageDealt](rejoined.take())
			if !ok {
				t.Fatalf("server HP=%d but rejoined observer received no periodic HP update", monster.HP)
			}
			if !got.Periodic || got.Src != 1 || got.Dst != mid || got.DstHP != monster.HP || got.Amount <= 0 {
				t.Fatalf("attribution or authoritative HP changed: %+v", got)
			}
			var current domain.Active
			monster.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == d.ID {
					current = a
				}
			})
			if current.Source != 1 || current.ExpireAt != admitted.ExpireAt || current.NextTickAt <= admitted.NextTickAt {
				t.Fatalf("rejoin rebound/refreshed the frozen source: %+v", current)
			}
			s.tick = admitted.ExpireAt + 1
			s.stepStatus()
			s.flush()
			if monster.Status.Has(d.ID) {
				t.Fatal("periodic status survived original deadline")
			}
		})
	}
}

func TestPeriodicLiveCasterRetainsAttackerOnlyMonsterDelivery(t *testing.T) {
	s, mid, caster := statusScene(t)
	observer := join(t, s, 2, 200, "observer", 105, 100)
	s.flush()
	caster.take()
	observer.take()
	m := s.entities[mid]
	m.HP = 10000
	m.MaxHP = 10000
	d := domain.StatusDef{ID: 1900019222, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 2, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}
	if !s.applyStatusDef(m, d, 1) {
		t.Fatal("status rejected")
	}
	s.flush()
	caster.take()
	observer.take()
	s.tick += domain.Ticks(1000)
	s.stepStatus()
	s.flush()
	if _, ok := firstOf[event.DamageDealt](caster.take()); !ok {
		t.Fatal("live caster lost periodic update")
	}
	if _, ok := firstOf[event.DamageDealt](observer.take()); ok {
		t.Fatal("live-caster optimization broadened to unrelated observer")
	}
}
