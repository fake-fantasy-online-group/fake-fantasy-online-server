package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestLegendaryNaturalShieldExpiryNotDispelOrRecursive(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "TY-D16")
	p.Status = domain.NewStatusSet()
	shield := domain.StatusDef{ID: 98765, DurationSec: 1, DamageShield: 100, TypeKnown: true, Type: domain.StatusGood}
	p.Status.Apply(shield, s.tick, p.ID)
	s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == shield.ID }))
	n := int32(0)
	p.Status.Each(func(a domain.Active) { n += a.ShieldLeft })
	if n != 0 {
		t.Fatal("dispel generated shield", n)
	}
	p.Status.Apply(shield, s.tick, p.ID)
	s.tick += domain.Ticks(1000)
	s.stepStatus()
	n = 0
	p.Status.Each(func(a domain.Active) { n += a.ShieldLeft })
	if n != 70 {
		t.Fatal("natural expiry shield", n)
	}
	s.tick += domain.Ticks(9000)
	s.stepStatus()
	n = 0
	p.Status.Each(func(a domain.Active) { n += a.ShieldLeft })
	if n != 0 {
		t.Fatal("returned shield recursed", n)
	}
}
func TestLegendaryControlEndsOnExpiryAndExplicitRemoval(t *testing.T) {
	for _, natural := range []bool{true, false} {
		s, p, _ := legendaryMeleeFixture(t, "TY-D09")
		p.Status = domain.NewStatusSet()
		p.Status.Apply(domain.StatusDef{ID: domain.StatusStun, DurationSec: 1, Control: domain.ControlStun}, s.tick, p.ID)
		if natural {
			s.tick += domain.Ticks(1000)
			s.stepStatus()
		} else {
			s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == domain.StatusStun }))
		}
		cs := s.legendaryContexts(p, nil, combat.Blow{}, event.DamageDealt{})
		if len(cs) != 1 {
			t.Fatal("power inactive")
		}
		if !universalLive(&cs[0], 0, "control-buffer") {
			t.Fatal("control ended callback missing", natural)
		}
	}
}
