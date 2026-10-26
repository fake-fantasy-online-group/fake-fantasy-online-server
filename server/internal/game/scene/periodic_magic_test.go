package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestPeriodicDamageUsesMagicDefenseAndReduction(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		mdef, resist, reduction, flat, shield, want int32
	}{
		{"unmitigated", 0, 0, 0, 0, 0, 1000},
		{"native magic defense", 100, 37, 0, 0, 0, 900},
		{"ordinary cap status", 0, 0, 50, 0, 0, 500},
		{"native then percentage then flat then shield", 100, 37, 50, 50, 100, 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mid, _ := statusScene(t)
			p := beefUp(s)
			p.Player.Char.Base.VIT = 10000
			s.refreshStats(p)
			p.Status = domain.NewStatusSet()
			p.Status.Apply(domain.StatusDef{ID: 990001, DurationSec: 60, MagicDamageReductionPct: tc.reduction, MagicDamageReductionFlat: tc.flat, PhysicalDamageReductionPct: 70, DamageShield: tc.shield}, s.tick, p.ID)
			p.Stats.MDef, p.Stats.MagicResist = tc.mdef, tc.resist
			p.HP = p.MaxHP
			before := p.HP
			s.outbox = nil
			s.tickStatus(p, domain.Active{Source: mid, Def: domain.StatusDef{ID: domain.StatusPoison, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
			if p.HP != before-tc.want {
				t.Fatalf("damage %d want %d", before-p.HP, tc.want)
			}
			found := false
			for _, e := range s.outbox {
				if hit, ok := e.ev.(event.DamageDealt); ok && hit.Dst == p.ID {
					found = true
					if !hit.Flag.Has(event.DamageMagic) || hit.Src != mid || hit.Origin != domain.DamageDOTOrigin {
						t.Fatalf("wrong periodic receipt: %+v", hit)
					}
				}
			}
			if !found {
				t.Fatal("missing damage receipt")
			}
		})
	}
}

func TestPeriodicDamageClassificationAndMissingSource(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	p.Status = domain.NewStatusSet()
	p.Stats.MDef, p.Stats.MagicResist = 100, 0
	p.HP, p.MaxHP = 10000, 10000
	// A physical source skill does not make its ongoing HP loss physical.
	s.skills = skillTable(domain.SkillDef{ID: 990002, Level: 1, HurtType: domain.HurtPhysical})
	a := domain.Active{Source: 999999, Def: domain.StatusDef{ID: 990003, SourceSkillID: 990002, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModePercent}}}}
	if b := s.legendaryPeriodicBlow(a); !b.Magic {
		t.Fatal("physical provenance changed periodic classification")
	}
	s.tickStatus(p, a)
	if p.HP != 10000-combat.RollMagicFixedDefense(1000, 100, 100, 0) {
		t.Fatalf("missing source bypassed magic defense: HP %d", p.HP)
	}
	// A later live defense change applies to the next tick, not the payload.
	p.Stats.MDef = 0
	before := p.HP
	s.tickStatus(p, a)
	if p.HP != before-1000 {
		t.Fatalf("live defense was not read: HP %d", p.HP)
	}
}

func TestPeriodicMagicFullReductionAndHealingIsolation(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{ID: 990010, DurationSec: 60, MagicDamageReductionPct: 100, DamageReductionCapPct: 100}, s.tick, p.ID)
	p.HP = p.MaxHP - 200
	before := p.HP
	s.tickStatus(p, domain.Active{Source: 99999, Def: domain.StatusDef{ID: 990011, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}})
	if p.HP != before {
		t.Fatalf("full magical mitigation leaked minimum damage: %d -> %d", before, p.HP)
	}
	s.tickStatus(p, domain.Active{Source: p.ID, Def: domain.StatusDef{ID: 990012, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: 100}}}})
	if p.HP != before+100 {
		t.Fatalf("healing was passed through damage mitigation: %d", p.HP)
	}
}

func TestPeriodicMagicUsesPartyAuraAndPassiveHalfDamage(t *testing.T) {
	t.Run("party magic ward", func(t *testing.T) {
		s, ps := partySetFive(t)
		s.stepEquipmentAuras()
		p := ps[2]
		p.Stats.MDef, p.Stats.MagicResist = 0, 0
		p.HP, p.MaxHP = 100000, 100000
		s.tickStatus(p, domain.Active{Source: 99999, Def: domain.StatusDef{ID: domain.StatusPoison, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
		if p.HP != 99080 {
			t.Fatalf("periodic magic bypassed 8%% party ward: %d", p.HP)
		}
	})
	t.Run("magic half damage passive", func(t *testing.T) {
		s, _, _ := statusScene(t)
		p := beefUp(s)
		p.Player.Char.Race = domain.Assassin
		p.Player.Char.EmploymentKnown, p.Player.Char.Employed = true, true
		p.Player.Char.Skills = domain.Learned{10229: 1}
		s.skills = skillTable(domain.SkillDef{ID: 10229, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveModifiers: []domain.PassiveModifier{{Kind: domain.PassiveIncomingMagicHalfChance, Value: 100}}})
		p.Stats.MDef, p.Stats.MagicResist = 0, 0
		p.HP, p.MaxHP = 10000, 10000
		s.tickStatus(p, domain.Active{Source: 99999, Def: domain.StatusDef{ID: domain.StatusPoison, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
		if p.HP != 9500 {
			t.Fatalf("periodic magic bypassed magic half-damage passive: %d", p.HP)
		}
	})
}
