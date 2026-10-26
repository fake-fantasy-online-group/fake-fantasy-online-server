package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func sourceDOTFixture(t *testing.T, player bool) (*Scene, *entity.Entity, *fakeSink, domain.StatusDef) {
	t.Helper()
	s, mid, sink := statusScene(t)
	target := s.entities[mid]
	if player {
		target = beefUp(s)
	}
	target.HP, target.MaxHP = 10000, 10000
	def := domain.StatusDef{
		ID: 990101, Level: 1, TypeKnown: true, Type: domain.StatusBad,
		DurationSec: 10, IntervalSec: 2, EffectHitBP: 1000000,
		HPChanges: []domain.ResourceChange{{Value: -100}},
	}
	s.statusOverlay = domain.StatusOverlay{
		{Existing: def.ID, Incoming: def.ID}: domain.OverlayCoexist,
	}
	return s, target, sink, def
}

func sourceDOTLayers(target *entity.Entity, id domain.StatusID) []domain.Active {
	var out []domain.Active
	target.Status.Each(func(a domain.Active) {
		if a.Def.ID == id {
			out = append(out, a)
		}
	})
	return out
}

func sourceDOTDamage(s *Scene, _ *fakeSink, target domain.EntityID) []event.DamageDealt {
	var out []event.DamageDealt
	for _, pending := range s.outbox {
		if d, ok := pending.ev.(event.DamageDealt); ok && d.Dst == target && d.Origin == domain.DamageDOTOrigin {
			out = append(out, d)
		}
	}
	s.outbox = nil
	return out
}

func TestDOTSourceRepeatedApplicationsSingleTick(t *testing.T) {
	for _, player := range []bool{true, false} {
		name := "monster"
		if player {
			name = "player"
		}
		t.Run(name, func(t *testing.T) {
			s, target, sink, def := sourceDOTFixture(t, player)
			start := s.tick
			// Multiple simultaneous area overlaps from the same caster must not
			// add separate clocks, and a later hit must not postpone the first tick.
			for i := 0; i < 6; i++ {
				if !s.applyStatusDef(target, def, 9001) {
					t.Fatal("same-caster application rejected")
				}
			}
			s.tick = start + domain.Ticks(1000)
			if !s.applyStatusDef(target, def, 9001) {
				t.Fatal("refresh rejected")
			}
			layers := sourceDOTLayers(target, def.ID)
			if len(layers) != 1 {
				t.Fatalf("same source has %d layers, want 1", len(layers))
			}
			if layers[0].NextTickAt != start+domain.Ticks(2000) {
				t.Fatalf("refresh postponed periodic clock: %+v", layers[0])
			}
			if layers[0].ExpireAt != s.tick+domain.Ticks(10000) {
				t.Fatalf("refresh did not extend expiry: %+v", layers[0])
			}
			sourceDOTDamage(s, sink, target.ID)
			before := target.HP
			s.tick = start + domain.Ticks(2000)
			s.stepStatus()
			damage := sourceDOTDamage(s, sink, target.ID)
			if len(damage) != 1 || damage[0].Src != 9001 || damage[0].Amount <= 0 {
				t.Fatalf("same-caster overlaps must deal one attributed tick: %+v", damage)
			}
			if before-target.HP != damage[0].Amount {
				t.Fatalf("HP loss %d differs from single tick %d", before-target.HP, damage[0].Amount)
			}
		})
	}
}

func TestDOTSourceDifferentCastersAndTypesCoexist(t *testing.T) {
	for _, player := range []bool{true, false} {
		name := "monster"
		if player {
			name = "player"
		}
		t.Run(name, func(t *testing.T) {
			s, target, sink, poison := sourceDOTFixture(t, player)
			burn := poison
			burn.ID++
			s.statusOverlay[domain.OverlayKey{Existing: burn.ID, Incoming: burn.ID}] = domain.OverlayCoexist
			for _, app := range []struct {
				def domain.StatusDef
				src domain.EntityID
			}{
				{poison, 9001}, {poison, 9001}, {poison, 9002}, {poison, 9002}, {burn, 9001}, {burn, 9001},
			} {
				if !s.applyStatusDef(target, app.def, app.src) {
					t.Fatalf("application rejected: source %d status %d", app.src, app.def.ID)
				}
			}
			if got := len(sourceDOTLayers(target, poison.ID)); got != 2 {
				t.Fatalf("different casters need two poison layers, got %d", got)
			}
			if got := len(sourceDOTLayers(target, burn.ID)); got != 1 {
				t.Fatalf("distinct burn needs one layer, got %d", got)
			}
			sourceDOTDamage(s, sink, target.ID)
			s.tick += domain.Ticks(2000)
			s.stepStatus()
			damage := sourceDOTDamage(s, sink, target.ID)
			counts := map[domain.EntityID]int{}
			for _, d := range damage {
				counts[d.Src]++
			}
			if len(damage) != 3 || counts[9001] != 2 || counts[9002] != 1 {
				t.Fatalf("independent source/type clocks lost or duplicated: %+v", damage)
			}
		})
	}
}

func TestDOTSourceFrequentRefreshDoesNotStarve(t *testing.T) {
	for _, player := range []bool{true, false} {
		name := "monster"
		if player {
			name = "player"
		}
		t.Run(name, func(t *testing.T) {
			s, target, sink, def := sourceDOTFixture(t, player)
			if !s.applyStatusDef(target, def, 9001) {
				t.Fatal("initial application rejected")
			}
			sourceDOTDamage(s, sink, target.ID)
			start := s.tick
			var ticks int
			for ms := 500; ms <= 14000; ms += 500 {
				s.tick = start + domain.Ticks(ms)
				if !s.applyStatusDef(target, def, 9001) {
					t.Fatalf("refresh rejected at %d ms", ms)
				}
				s.stepStatus()
				ticks += len(sourceDOTDamage(s, sink, target.ID))
			}
			if ticks != 7 {
				t.Fatalf("frequent refresh should preserve seven scheduled ticks, got %d", ticks)
			}
			if got := len(sourceDOTLayers(target, def.ID)); got != 1 {
				t.Fatalf("refresh created %d layers", got)
			}
		})
	}
}

func TestDOTSourceNonDamageStatusesKeepOverlayCoexist(t *testing.T) {
	s, target, _, base := sourceDOTFixture(t, true)
	defs := []domain.StatusDef{
		{ID: 990201, Level: 1, DurationSec: 10, IntervalSec: 2, TypeKnown: true, Type: domain.StatusGood, HPChanges: []domain.ResourceChange{{Value: 10}}},
		{ID: 990202, Level: 1, DurationSec: 10, IntervalSec: 2, TypeKnown: true, Type: domain.StatusBad, MPChanges: []domain.ResourceChange{{Value: -10}}, EffectHitBP: base.EffectHitBP},
		{ID: 990203, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlSilence, EffectHitBP: base.EffectHitBP},
	}
	for _, def := range defs {
		s.statusOverlay[domain.OverlayKey{Existing: def.ID, Incoming: def.ID}] = domain.OverlayCoexist
		for i := 0; i < 2; i++ {
			if !s.applyStatusDef(target, def, 9001) {
				t.Fatalf("non-DOT application %d rejected", def.ID)
			}
		}
		if got := len(sourceDOTLayers(target, def.ID)); got != 2 {
			t.Fatalf("non-DOT %d coexist changed: %d layers", def.ID, got)
		}
	}
}

func TestDOTSourceSameIDMatrixKeepsIndependentCasters(t *testing.T) {
	for _, rule := range []domain.OverlayRule{domain.OverlayBlock, domain.OverlayReplace, domain.OverlayMustReplace, domain.OverlayCounteract} {
		for _, player := range []bool{true, false} {
			s, target, _, def := sourceDOTFixture(t, player)
			s.statusOverlay[domain.OverlayKey{Existing: def.ID, Incoming: def.ID}] = rule
			for _, source := range []domain.EntityID{9001, 9001, 9002, 9002} {
				if !s.applyStatusDef(target, def, source) {
					t.Fatalf("same-ID rule %d rejected source %d on player=%v", rule, source, player)
				}
			}
			layers := sourceDOTLayers(target, def.ID)
			counts := map[domain.EntityID]int{}
			for _, a := range layers {
				counts[a.Source]++
			}
			if len(layers) != 2 || counts[9001] != 1 || counts[9002] != 1 {
				t.Fatalf("same-ID rule %d changed source identity on player=%v: %+v", rule, player, layers)
			}
		}
	}
}

func TestDOTSourceCrossIDMatrixStillApplies(t *testing.T) {
	for _, rule := range []domain.OverlayRule{domain.OverlayBlock, domain.OverlayCounteract} {
		s, target, _, def := sourceDOTFixture(t, true)
		old := domain.StatusDef{ID: 990301, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood}
		if !s.applyStatusDef(target, old, 9002) {
			t.Fatal("initial unrelated status rejected")
		}
		s.statusOverlay[domain.OverlayKey{Existing: old.ID, Incoming: def.ID}] = rule
		if s.applyStatusDef(target, def, 9001) {
			t.Fatalf("cross-ID rule %d unexpectedly admitted DoT", rule)
		}
		if len(sourceDOTLayers(target, def.ID)) != 0 {
			t.Fatalf("cross-ID rule %d left rejected DoT", rule)
		}
		if want := rule == domain.OverlayBlock; target.Status.Has(old.ID) != want {
			t.Fatalf("cross-ID rule %d changed existing-status removal", rule)
		}
	}
}

func TestDOTSourceFractionalSummonUsesResponsibleCaster(t *testing.T) {
	s, mid, _ := statusScene(t)
	owner := beefUp(s)
	summon := &entity.Entity{ID: 990601, Kind: domain.KindMonster, HP: 100, Monster: &entity.Monster{Summoner: owner.ID}}
	s.entities[summon.ID] = summon
	d := domain.StatusDef{ID: 990602, Level: 1, DurationSec: 10, IntervalSec: 1, PeriodicDamageFraction100: 75}
	target := s.entities[mid]
	if !s.applyStatusDef(target, d, summon.ID) || !s.applyStatusDef(target, d, owner.ID) {
		t.Fatal("fractional DoT did not apply")
	}
	if target.Status.Count() != 1 {
		t.Fatalf("summon and owner created separate same-caster DoTs: %d", target.Status.Count())
	}
	target.Status.Each(func(a domain.Active) {
		if a.Source != owner.ID {
			t.Fatalf("wrong responsible caster: %d", a.Source)
		}
	})
}
