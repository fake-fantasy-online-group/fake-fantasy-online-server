package domain

import (
	"reflect"
	"testing"
)

func TestMovementImmunityPreservesMixedPayloadAndSource(t *testing.T) {
	slow := Affix{Attr: AttrMoveSpeed, Mode: ModePercent, Value: -40}
	for _, tc := range []struct {
		name  string
		def   StatusDef
		empty bool
	}{
		{"slow", StatusDef{ID: StatusSlow, Affixes: []Affix{slow}}, true},
		{"root", StatusDef{ID: 10111, BlockMove: true, Immobile: 1}, true},
		{"stun", StatusDef{ID: StatusStun, Control: ControlStun, Immobile: 1, Affixes: []Affix{slow}}, false},
		{"freeze", StatusDef{ID: StatusFreeze, Control: ControlStun, Immobile: 1, BreakOnDamage: true, Affixes: []Affix{slow}}, false},
		{"petrify", StatusDef{ID: StatusPetrify, Control: ControlStun, Immobile: 1, Affixes: []Affix{slow}}, false},
		{"root-poison", StatusDef{ID: 81001, BlockMove: true, Immobile: 1, IntervalSec: 2, HPChanges: []ResourceChange{{Value: -20}}}, false},
		{"root-shield", StatusDef{ID: 81002, BlockMove: true, Immobile: 1, DamageShield: 200}, false},
		{"slow-vulnerability", StatusDef{ID: 81003, Affixes: []Affix{slow}, PhysicalDamageTakenPct: 30}, false},
		{"slow-attack-speed", StatusDef{ID: StatusSlow, AttackSpeedIsFrequency: true, Affixes: []Affix{slow, {Attr: AttrAtkSpeed, Value: 30, Mode: ModePercent}}}, false},
		{"slow-positive-speed", StatusDef{ID: 81004, Affixes: []Affix{slow, {Attr: AttrMoveSpeed, Value: 10}}}, false},
		{"slow-unrelated-floor", StatusDef{ID: 81005, Affixes: []Affix{slow}, MaxHPFloor: 77}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.def
			d.Level, d.DurationSec, d.Type, d.TypeKnown = 1, 9, StatusBad, true
			d.SkillApplied, d.SkillSnapshotID, d.SourceSkillID = true, 10111, 10111
			d.LegendaryModifiers = LegendaryStatusModifiers{Applied: true, DurationBP: 5000}
			before := d
			before.Affixes = append([]Affix(nil), d.Affixes...)
			changed, empty := d.StripMovementImpairments()
			if !changed || empty != tc.empty {
				t.Fatal("wrong component filtering", changed, empty, d)
			}
			if !reflect.DeepEqual(tc.def.Affixes, before.Affixes) {
				t.Fatal("source affix slice mutated")
			}
			want := before
			want.Affixes = nil
			for _, a := range before.Affixes {
				if a.Attr != AttrMoveSpeed || a.Value >= 0 {
					want.Affixes = append(want.Affixes, a)
				}
			}
			if before.Control != ControlStun {
				want.BlockMove, want.Immobile = false, 0
			}
			if len(d.Affixes) == 0 {
				d.Affixes = nil
			}
			if !reflect.DeepEqual(d, want) {
				t.Fatalf("unrelated payload changed\ngot %+v\nwant %+v", d, want)
			}
		})
	}
}

func TestMovementImmunityKeepsActiveBalancesAndCoexistingLayers(t *testing.T) {
	s := NewStatusSet()
	root := StatusDef{ID: 10111, Level: 1, DurationSec: 10, BlockMove: true, Immobile: 1}
	mixed := root
	mixed.IntervalSec, mixed.DamageShield = 2, 500
	mixed.HPChanges = []ResourceChange{{Value: -7}}
	s.Apply(root, 10, 1)
	s.ApplyWithOverlay(mixed, 20, 2, StatusOverlay{{Existing: root.ID, Incoming: root.ID}: OverlayCoexist})
	s.act[root.ID][1].ShieldLeft = 73
	s.act[root.ID][1].PeriodicDamageRemainder100 = 37
	s.act[root.ID][1].ClassSetPoisonRemainder = 2
	before := s.act[root.ID][1]
	// Suppression is not permission to let an old root reappear later.
	s.Apply(StatusDef{ID: 81010, SuppressHarmful: true, DurationSec: 30}, 20, 1)
	changed, removed := s.StripMovementImpairments()
	if !changed || len(removed) != 0 || len(s.act[root.ID]) != 1 {
		t.Fatal(changed, removed, s.act[root.ID])
	}
	want := before
	want.Def.BlockMove, want.Def.Immobile, want.Def.Affixes = false, 0, []Affix{}
	if got := s.act[root.ID][0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("active balance changed: %+v != %+v", got, want)
	}
	if changed, removed := s.StripMovementImpairments(); changed || len(removed) != 0 {
		t.Fatal("cleanse not idempotent", changed, removed)
	}
}
