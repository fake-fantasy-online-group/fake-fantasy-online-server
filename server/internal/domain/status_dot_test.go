package domain

import (
	"testing"
	"time"
)

func dotTestDef() StatusDef {
	return StatusDef{ID: 990501, Level: 1, DurationSec: 10, IntervalSec: 2,
		TypeKnown: true, Type: StatusBad, HPChanges: []ResourceChange{{Value: -10}}}
}

func TestDOTSourceDomainApplyEntrypoints(t *testing.T) {
	for name, apply := range map[string]func(*StatusSet, StatusDef, Tick, EntityID){
		"legacy": func(s *StatusSet, d StatusDef, tick Tick, src EntityID) { s.Apply(d, tick, src) },
		"empty_overlay": func(s *StatusSet, d StatusDef, tick Tick, src EntityID) {
			if r := s.ApplyWithOverlay(d, tick, src, nil); !r.Applied {
				t.Fatalf("empty overlay rejected: %+v", r)
			}
		},
		"coexist": func(s *StatusSet, d StatusDef, tick Tick, src EntityID) {
			if r := s.ApplyWithOverlay(d, tick, src, StatusOverlay{{Existing: d.ID, Incoming: d.ID}: OverlayCoexist}); !r.Applied {
				t.Fatalf("coexist rejected: %+v", r)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := NewStatusSet(), dotTestDef()
			apply(s, d, 0, 10)
			apply(s, d, Ticks(500), 10)
			apply(s, d, Ticks(500), 20)
			apply(s, d, Ticks(1000), 20)
			if s.Count() != 2 {
				t.Fatalf("wanted one instance per source, got %d", s.Count())
			}
			if due := s.DueTicks(Ticks(2000)); len(due) != 1 || due[0].Source != 10 {
				t.Fatalf("first clock changed: %+v", due)
			}
			if due := s.DueTicks(Ticks(2500)); len(due) != 1 || due[0].Source != 20 {
				t.Fatalf("second clock changed: %+v", due)
			}
		})
	}
}

func TestDOTSourceDomainStrengthAndOwnership(t *testing.T) {
	s, strong := NewStatusSet(), dotTestDef()
	strong.Level = 3
	strong.HPChanges = []ResourceChange{{Value: -30}}
	s.Apply(strong, 0, 10)
	weak := dotTestDef()
	s.Apply(weak, Ticks(1000), 10)
	s.Apply(weak, Ticks(1000), 20)
	if s.Count() != 2 {
		t.Fatalf("different strength from another source must coexist: %d", s.Count())
	}
	for _, a := range s.act[strong.ID] {
		if a.Source == 10 {
			if a.Def.Level != 3 || a.Def.HPChanges[0].Value != -30 || a.NextTickAt != Ticks(2000) || a.ExpireAt != Ticks(11000) {
				t.Fatalf("same-source weaker refresh changed payload or clock: %+v", a)
			}
		} else if a.Source != 20 || a.Def.Level != 1 || a.Def.HPChanges[0].Value != -10 {
			t.Fatalf("another source stole stronger payload: %+v", a)
		}
	}
	strong.Level = 4
	strong.HPChanges = []ResourceChange{{Value: -40}}
	s.Apply(strong, Ticks(1500), 10)
	for _, a := range s.act[strong.ID] {
		if a.Source == 10 && (a.Def.Level != 4 || a.Def.HPChanges[0].Value != -40 || a.NextTickAt != Ticks(2000)) {
			t.Fatalf("stronger refresh did not replace payload while retaining clock: %+v", a)
		}
	}
}

func TestDOTSourceDomainFractionalBalanceSurvivesRefresh(t *testing.T) {
	s := NewStatusSet()
	d := StatusDef{ID: 990502, Level: 1, DurationSec: 10, IntervalSec: 1, PeriodicDamageFraction100: 75}
	s.Apply(d, 0, 10)
	first := s.DueTicks(Ticks(1000))
	if len(first) != 1 || first[0].PeriodicDamageTickExtra != 0 || first[0].PeriodicDamageRemainder100 != 75 {
		t.Fatalf("first fractional tick: %+v", first)
	}
	s.Apply(d, Ticks(1500), 10)
	s.Apply(d, Ticks(1500), 20)
	second := s.DueTicks(Ticks(2000))
	if len(second) != 1 || second[0].Source != 10 || second[0].PeriodicDamageTickExtra != 1 || second[0].PeriodicDamageRemainder100 != 50 {
		t.Fatalf("refresh lost fractional damage or mixed source balances: %+v", second)
	}
	third := s.DueTicks(Ticks(2500))
	if len(third) != 1 || third[0].Source != 20 || third[0].PeriodicDamageTickExtra != 0 || third[0].PeriodicDamageRemainder100 != 75 {
		t.Fatalf("new source inherited another source's balance: %+v", third)
	}
}

func TestDOTSourceDomainClassSetBalanceRescales(t *testing.T) {
	s, d := NewStatusSet(), dotTestDef()
	d.ClassSetPoisonBonusTotal = 1
	s.Apply(d, 0, 10)
	s.act[d.ID][0].ClassSetPoisonRemainder = 4 // Four fifths of one point.
	d.DurationSec = 20                         // New denominator is ten authored ticks.
	s.Apply(d, Ticks(1000), 10)
	if got := s.act[d.ID][0].ClassSetPoisonRemainder; got != 8 {
		t.Fatalf("fractional class-set balance changed: got %d/10, want 8/10", got)
	}
	first := s.DueTicks(Ticks(2000))
	if len(first) != 1 || first[0].PeriodicDamageTickExtra != 0 || first[0].ClassSetPoisonRemainder != 9 {
		t.Fatalf("first rescaled tick: %+v", first)
	}
	second := s.DueTicks(Ticks(4000))
	if len(second) != 1 || second[0].PeriodicDamageTickExtra != 1 || second[0].ClassSetPoisonRemainder != 0 {
		t.Fatalf("second rescaled tick: %+v", second)
	}
}

func TestDOTSourceDomainExpiredInstanceStartsFresh(t *testing.T) {
	s, d := NewStatusSet(), dotTestDef()
	d.DurationSec, d.IntervalSec = 2, 1
	d.PeriodicDamageFraction100 = 75
	s.Apply(d, 0, 10)
	s.act[d.ID][0].PeriodicDamageRemainder100 = 75
	s.Apply(d, Ticks(3000), 10)
	a := s.act[d.ID][0]
	if s.Count() != 1 || a.NextTickAt != Ticks(4000) || a.ExpireAt != Ticks(5000) || a.PeriodicDamageRemainder100 != 0 {
		t.Fatalf("expired DoT leaked its clock or balance into fresh cast: %+v", a)
	}
	if due := s.DueTicks(Ticks(3000)); len(due) != 0 {
		t.Fatalf("fresh status inherited an overdue tick: %+v", due)
	}
}

func TestDOTSourceDomainDamageClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  StatusDef
		want bool
	}{
		{"absolute_regen_loss", StatusDef{IntervalSec: 1, Affixes: []Affix{{Attr: AttrHPRegen, Value: -1, Mode: ModeAbsolute}}}, true},
		{"percent_regen_loss", StatusDef{IntervalSec: 1, Affixes: []Affix{{Attr: AttrHPRegen, Value: -1, Mode: ModePercent}}}, true},
		{"absolute_hp_loss", StatusDef{IntervalSec: 1, HPChanges: []ResourceChange{{Value: -1, Mode: ModeAbsolute}}}, true},
		{"percent_hp_loss", StatusDef{IntervalSec: 1, HPChanges: []ResourceChange{{Value: -1, Mode: ModePercent}}}, true},
		{"fractional", StatusDef{IntervalSec: 1, PeriodicDamageFraction100: 1}, true},
		{"class_set", StatusDef{IntervalSec: 1, ClassSetPoisonBonusTotal: 1}, true},
		{"healing", StatusDef{IntervalSec: 1, HPChanges: []ResourceChange{{Value: 1}}}, false},
		{"mana_drain", StatusDef{IntervalSec: 1, MPChanges: []ResourceChange{{Value: -1}}}, false},
		{"attack_debuff", StatusDef{IntervalSec: 1, Affixes: []Affix{{Attr: AttrAtk, Value: -1}}}, false},
		{"non_periodic", StatusDef{HPChanges: []ResourceChange{{Value: -1}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.def.DamageOverTime(); got != tc.want {
				t.Fatalf("DamageOverTime=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestDOTSourceDomainRestoredUnknownCastersRemainDistinct(t *testing.T) {
	s, d := NewStatusSet(), dotTestDef()
	s.Apply(d, 0, 10)
	s.Apply(d, 0, 20)
	wall := time.Unix(1000, 0)
	saved := s.SaveForLogout(0, wall, 99)
	restored := RestoreSavedStatuses(saved, 100, wall, 99)
	if restored.Count() != 2 {
		t.Fatalf("restore merged two original casters: %d", restored.Count())
	}
	restored.Apply(d, 100, 0)
	restored.Apply(d, 101, 0)
	if restored.Count() != 3 {
		t.Fatalf("unknown restored casters merged with new source zero: %d", restored.Count())
	}
}

func TestDOTSourceDomainTransferredBalanceKeepsContinuingClock(t *testing.T) {
	s := NewStatusSet()
	d := StatusDef{ID: 990503, Level: 1, DurationSec: 10, IntervalSec: 1, PeriodicDamageFraction100: 75}
	snapshot := Active{Def: d, ExpireAt: Ticks(7000), NextTickAt: Ticks(1500), PeriodicDamageRemainder100: 60, ShieldLeft: 3}
	s.Apply(d, Ticks(1000), 10)
	s.RestoreTransferredStatusBalance(10, snapshot)
	first := s.act[d.ID][0]
	if first.ExpireAt != snapshot.ExpireAt || first.NextTickAt != snapshot.NextTickAt || first.PeriodicDamageRemainder100 != 60 || first.ShieldLeft != 3 {
		t.Fatalf("first transfer did not retain exact original balance: %+v", first)
	}
	due := s.DueTicks(Ticks(1500))
	if len(due) != 1 || due[0].PeriodicDamageTickExtra != 1 || due[0].PeriodicDamageRemainder100 != 35 {
		t.Fatalf("first transferred tick: %+v", due)
	}
	later := snapshot
	later.NextTickAt, later.ExpireAt, later.PeriodicDamageRemainder100 = Ticks(5000), Ticks(9000), 0
	s.Apply(d, Ticks(1600), 10)
	s.RestoreTransferredStatusBalance(10, later)
	refreshed := s.act[d.ID][0]
	if refreshed.NextTickAt != Ticks(2500) || refreshed.PeriodicDamageRemainder100 != 35 || refreshed.ExpireAt != Ticks(9000) {
		t.Fatalf("repeated copy restarted target clock or lost credit: %+v", refreshed)
	}
	other := snapshot
	other.NextTickAt, other.ExpireAt, other.PeriodicDamageRemainder100 = Ticks(3000), Ticks(6000), 5
	s.Apply(d, Ticks(1600), 20)
	s.RestoreTransferredStatusBalance(20, other)
	if s.Count() != 2 {
		t.Fatalf("different copied caster collapsed: %d", s.Count())
	}
	due = s.DueTicks(Ticks(2500))
	if len(due) != 1 || due[0].Source != 10 || due[0].PeriodicDamageTickExtra != 1 || due[0].PeriodicDamageRemainder100 != 10 {
		t.Fatalf("same-source copy starved continuing tick: %+v", due)
	}
	due = s.DueTicks(Ticks(3000))
	if len(due) != 1 || due[0].Source != 20 || due[0].PeriodicDamageTickExtra != 0 || due[0].PeriodicDamageRemainder100 != 80 {
		t.Fatalf("different copied source did not retain independent credit: %+v", due)
	}
}

func TestDOTSourceDomainWeakerTransferDoesNotRefillFullLifetime(t *testing.T) {
	s, strong := NewStatusSet(), dotTestDef()
	strong.Level = 3
	s.Apply(strong, 0, 10)
	weak := strong
	weak.Level = 1
	s.Apply(weak, Ticks(5000), 10)
	s.RestoreTransferredStatusBalance(10, Active{Def: weak, ExpireAt: Ticks(8000), NextTickAt: Ticks(7000)})
	a := s.act[strong.ID][0]
	if a.Def.Level != 3 || a.ExpireAt != Ticks(10000) || a.NextTickAt != Ticks(2000) {
		t.Fatalf("weaker transfer changed retained payload or refilled full lifetime: %+v", a)
	}
}

func TestDOTSourceDomainTransferDoesNotRewriteUnknownCasters(t *testing.T) {
	s, d := NewStatusSet(), dotTestDef()
	s.Apply(d, 0, 10)
	s.Apply(d, 0, 20)
	wall := time.Unix(1000, 0)
	restored := RestoreSavedStatuses(s.SaveForLogout(0, wall, 99), 100, wall, 99)
	before := append([]Active(nil), restored.act[d.ID]...)
	restored.Apply(d, 100, 0)
	restored.RestoreTransferredStatusBalance(0, Active{Def: d, ExpireAt: 500, NextTickAt: 250, PeriodicDamageRemainder100: 75})
	for i := range before {
		got := restored.act[d.ID][i]
		if got.ExpireAt != before[i].ExpireAt || got.NextTickAt != before[i].NextTickAt || got.PeriodicDamageRemainder100 != before[i].PeriodicDamageRemainder100 {
			t.Fatalf("new source zero rewrote restored unknown source %d: before=%+v after=%+v", i, before[i], got)
		}
	}
}
