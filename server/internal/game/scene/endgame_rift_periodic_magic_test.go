package scene

import (
	"encoding/json"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestRiftPeriodicDifferentCastersKeepIndependentCadence(t *testing.T) {
	for _, ability := range []int32{6, 3} {
		t.Run(map[int32]string{6: "poison", 3: "lightning"}[ability], func(t *testing.T) {
			s, m, p, now := riftEliteFixture(t, ability)
			a := s.currentRiftRules().Abilities[ability]
			s.createRiftHazard(m, p.Pos, a, *now)
			s.createRiftHazard(m, p.Pos, a, *now) // Same caster is still just one tick.
			other := *m
			other.ID = s.alloc.Monster()
			s.createRiftHazard(&other, p.Pos, a, *now)
			s.armRiftHazards(p, *now)
			*now += 1000
			s.stepRiftHazards(*now)
			want := int32(2) * 100 * a.DamagePct / 100
			if got := 10000 - p.HP; got != want {
				t.Fatalf("different casters: got %d want %d", got, want)
			}
			hp := p.HP
			*now += 100
			s.stepRiftHazards(*now)
			if p.HP != hp {
				t.Fatal("same cadence replayed damage")
			}
		})
	}
}

func TestRiftPeriodicPhysicalBudgetUsesMagicDefenseAndReduction(t *testing.T) {
	for _, mechanism := range []string{"poison_pool", "lightning_orb", "death_lava"} {
		t.Run(mechanism, func(t *testing.T) {
			s, m, p, _ := riftEliteFixture(t, 6)
			p.Stats.MDef = 100
			p.Status.Apply(domain.StatusDef{ID: 99985, Level: 1, DurationSec: 20, MagicDamageReductionPct: 50}, s.tick, p.ID)
			h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Pos: p.Pos, Attack: domain.Stats{MinAtk: 200, MaxAtk: 200, MAtk: 1}, Ability: domain.RiftAbilityRule{Mechanism: mechanism}}
			before := p.HP
			s.hitRiftHazard(h, p, 100)
			periodic := mechanism != "death_lava"
			want := int32(200)
			if periodic {
				want = 69
			} // (200 - 0.63*100) * 50%, normal bucket rounding, exactly once.
			if before-p.HP != want {
				t.Fatalf("damage=%d want %d", before-p.HP, want)
			}
			found := false
			for _, pending := range s.outbox {
				if ev, ok := pending.ev.(event.DamageDealt); ok && ev.Dst == p.ID {
					found = true
					if ev.Flag.Has(event.DamageMagic) != periodic {
						t.Fatal("wrong damage classification")
					}
				}
			}
			if !found {
				t.Fatal("no damage event")
			}
		})
	}
}

func TestRiftPeriodicLegacySaveRearmsWithoutCadenceReplay(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now)
	s.armRiftHazards(p, *now)
	*now += 1000
	s.stepRiftHazards(*now)
	x := s.eliteSnapshot()
	x.PoisonNext[p.ID] = *now + 90000 // Legacy target-only cooldown must not cross sources.
	data, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var restored endgameRiftEliteSnapshot
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.periodicSourceNext != nil {
		t.Fatal("transport-local cadence was persisted")
	}
	s.rift.data.Elite = &restored
	s.restoreRiftElite()
	if len(restored.PoisonNext) != 0 || len(restored.periodicSourceNext) != 0 {
		t.Fatal("legacy cadence survived reconstruction")
	}
	hp := p.HP
	*now += 5000
	s.stepRiftHazards(*now)
	if p.HP != hp {
		t.Fatal("replayed before full snapshot")
	}
	s.armRiftHazards(p, *now)
	s.stepRiftHazards(*now + 999)
	if p.HP != hp {
		t.Fatal("skipped rearm warning")
	}
	s.stepRiftHazards(*now + 1000)
	if hp-p.HP != 5 {
		t.Fatalf("fresh occupancy damage=%d", hp-p.HP)
	}
}

func TestRiftPeriodicPoisonDifferentCasterStaggerIsNotThrottled(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	a := s.currentRiftRules().Abilities[6]
	s.createRiftHazard(m, p.Pos, a, *now)
	s.armRiftHazards(p, *now)
	*now += 500
	other := *m
	other.ID = s.alloc.Monster()
	s.createRiftHazard(&other, p.Pos, a, *now)
	s.armRiftHazards(p, *now)
	*now += 500
	s.stepRiftHazards(*now)
	if p.HP != 9995 {
		t.Fatalf("first caster: %d", p.HP)
	}
	*now += 500
	s.stepRiftHazards(*now)
	if p.HP != 9990 {
		t.Fatalf("second caster cadence suppressed: %d", p.HP)
	}
}
