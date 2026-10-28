package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestLegendarySourceReuseNativeTriplePlacement(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "CK-A13", "CK-A26")
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10213, At: &m.Pos})
	if len(s.casts) != 1 {
		t.Fatal("native trap was not admitted")
	}
	accepted := s.casts[0].sourceOffense
	def := s.casts[0].def
	if accepted == nil || accepted.ChildPreparation == nil || accepted.ChildPreparation.Hard[10213] != 1 {
		t.Fatal("accepted derived-source preparation missing")
	}
	p.Player.Char.Skills[10213] = 5
	p.Stats.MinAtk, p.Stats.MaxAtk = 900, 900
	sharedTargetFinishCasts(t, s)
	if len(s.traps) != 3 {
		t.Fatalf("native triple placement produced %d traps", len(s.traps))
	}
	for _, trap := range s.traps {
		if trap.Trap.SourceOffense.ChildPreparation != accepted.ChildPreparation {
			t.Fatal("root/side trap did not reuse the accepted immutable preparation")
		}
		if trap.Trap.LegendaryChildPlacement && len(trap.Trap.SourceOffense.CombatCausalPath) == 0 {
			t.Fatal("generated side trap lost producer ancestry")
		}
		if !trap.Trap.LegendaryChildPlacement && trap.Trap.SourceOffense != accepted {
			t.Fatal("native root trap discarded accepted source")
		}
		if trap.Trap.SourceOffense.ChildPreparation.Hard[10213] != 1 || trap.Trap.AttackStats != accepted.Stats {
			t.Fatal("live source update changed an accepted trap")
		}
	}
	for id := range s.traps {
		s.removeTrap(id)
	}
	// A placement without an accepted source still captures a new, independent
	// source. Nil in the optional slot has the same fallback meaning.
	s.placeAssassinTrap(p, def, m.Pos, 100, 100, false, nil)
	if len(s.traps) != 3 {
		t.Fatal("fresh placement should also receive equipped triple placement")
	}
	for _, trap := range s.traps {
		fresh := trap.Trap.SourceOffense
		if fresh == accepted || fresh.ChildPreparation == accepted.ChildPreparation || fresh.ChildPreparation.Hard[10213] != 5 || fresh.Stats.MinAtk != 900 {
			t.Fatal("fresh placement reused stale source inputs")
		}
	}
	if accepted.ChildPreparation.Hard[10213] != 1 {
		t.Fatal("fresh source aliased the accepted hard-rank map")
	}
}

func TestLegendarySourceReuseAvoidsDiscardedCapture(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "CK-A26")
	source := s.captureCombatSourceV5(p, false)
	if source.ChildPreparation == nil {
		t.Fatal("fixture must exercise the derived preparation allocation")
	}
	def := domain.SkillDef{ID: 10213}
	place := func(reuse bool) {
		if reuse {
			s.placeAssassinTrap(p, def, m.Pos, 100, 100, false, source)
		} else {
			s.placeAssassinTrap(p, def, m.Pos, 100, 100, false)
		}
		for id := range s.traps {
			s.removeTrap(id)
		}
		s.outbox = s.outbox[:0]
	}
	// A bounded relative allocation guard, not a throughput/latency claim.
	// Both sides execute the same trap consumer; only source capture differs.
	reused := testing.AllocsPerRun(10, func() { place(true) })
	fresh := testing.AllocsPerRun(10, func() { place(false) })
	if reused >= fresh {
		t.Fatalf("accepted placement still pays fresh-source allocations: reused=%g fresh=%g", reused, fresh)
	}
	t.Logf("trap placement allocations per run: accepted=%g fresh=%g", reused, fresh)
}
