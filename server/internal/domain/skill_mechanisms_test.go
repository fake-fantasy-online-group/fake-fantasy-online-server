package domain

import (
	"reflect"
	"testing"
)

func TestSkillMechanismsStatusTargetUsesEntityKind(t *testing.T) {
	for _, kind := range []EntityKind{KindPlayer, KindMonster, KindNPC, KindPet, KindTrap, KindDrop, KindUnknown} {
		if !SkillStatusTargetAny.Matches(kind) {
			t.Fatal("legacy any target changed", kind)
		}
		if got := SkillStatusTargetPlayer.Matches(kind); got != (kind == KindPlayer) {
			t.Fatal("player filter", kind, got)
		}
		if got := SkillStatusTargetNonPlayer.Matches(kind); got != (kind != KindPlayer && kind != KindUnknown) {
			t.Fatal("non-player filter", kind, got)
		}
		if SkillStatusTarget("unknown").Matches(kind) {
			t.Fatal("invalid filter did not fail closed")
		}
	}
}

func TestSkillMechanismsOverlayPriorityPreservesLegacyOrdering(t *testing.T) {
	for _, a := range []int32{1, 2, 17, 100, 2147483647} {
		for _, b := range []int32{1, 2, 17, 100, 2147483647} {
			pa, va := (StatusDef{Level: a}).OverlayPriority()
			pb, vb := (StatusDef{Level: b}).OverlayPriority()
			if !va || !vb || (pa > pb) != (a > b) {
				t.Fatal("legacy ordering changed", a, b)
			}
		}
	}
	for _, value := range []int32{-2147483648, 2147483647} {
		d := mechanismBlind(25, value)
		if got, valid := d.OverlayPriority(); !valid || got != -int64(value) {
			t.Fatal("strength overflow", got)
		}
	}
}
func mechanismBlind(level, value int32) StatusDef {
	return StatusDef{ID: 1008, Level: level, DurationSec: 8, OverlayStrength: &StatusOverlayStrength{Attr: AttrHit, Mode: ModePercent, ValueOrder: -1}, Affixes: []Affix{{Attr: AttrHit, Mode: ModePercent, Value: value}}}
}
func TestSkillMechanismsOverlayPriorityWeakStrongAndEqual(t *testing.T) {
	overlay := StatusOverlay{{Existing: 1008, Incoming: 1008}: OverlayReplace}
	for _, tc := range []struct {
		old, new StatusDef
		blocked  bool
	}{
		{mechanismBlind(6, -30), mechanismBlind(25, -18), true},
		{mechanismBlind(3, -15), mechanismBlind(25, -18), false},
		{mechanismBlind(2, -10), mechanismBlind(21, -10), false},
		{mechanismBlind(25, -18), mechanismBlind(6, -30), false},
		{mechanismBlind(6, -30), mechanismBlind(25, -32), false},
		{mechanismBlind(25, -100), mechanismBlind(6, -100), false},
	} {
		states := NewStatusSet()
		states.ApplyWithOverlay(tc.old, 0, 1, overlay)
		got := states.ApplyWithOverlay(tc.new, 1, 2, overlay)
		if got.Blocked != tc.blocked || got.Applied == tc.blocked {
			t.Fatalf("priority outcome: %+v", got)
		}
		var active Active
		states.Each(func(a Active) { active = a })
		if tc.blocked && active.Source != 1 || !tc.blocked && active.Source != 2 {
			t.Fatal("replacement ownership")
		}
		legacy := NewStatusSet()
		legacy.Apply(tc.old, 0, 1)
		legacy.Apply(tc.new, 1, 2)
		legacy.Each(func(a Active) { active = a })
		if tc.blocked && active.Def.Level != tc.old.Level {
			t.Fatal("fallback Apply ignored precedence")
		}
	}
}
func TestSkillMechanismsOverlayStrengthFailsClosed(t *testing.T) {
	good := mechanismBlind(25, -18)
	noRule := good
	noRule.OverlayStrength = nil
	missing := good
	missing.Affixes = nil
	duplicate := good
	duplicate.Affixes = append(append([]Affix(nil), good.Affixes...), good.Affixes[0])
	mixed := good
	mixed.Affixes = []Affix{{Attr: AttrHit, Mode: ModeAbsolute, Value: -18}}
	invalid := good
	invalid.OverlayStrength = &StatusOverlayStrength{Attr: AttrHit, Mode: ModePercent, ValueOrder: 0}
	overlay := StatusOverlay{{Existing: 1008, Incoming: 1008}: OverlayReplace}
	for _, bad := range []StatusDef{noRule, missing, duplicate, mixed, invalid} {
		if !overlayReplacementBlocked(good, bad) || !overlayReplacementBlocked(bad, good) {
			t.Fatal("incomparable strength accepted")
		}
		set := NewStatusSet()
		set.ApplyWithOverlay(good, 0, 1, overlay)
		if !set.ApplyWithOverlay(bad, 1, 2, overlay).Blocked {
			t.Fatal("malformed attempt replaced good state")
		}
	}
	for _, bad := range []StatusDef{missing, duplicate, mixed, invalid} {
		set := NewStatusSet()
		if !set.ApplyWithOverlay(bad, 0, 1, overlay).Blocked || set.Count() != 0 {
			t.Fatal("malformed state inserted")
		}
	}
}
func TestSkillMechanismsLegacyOverlayLevelOnlyAffectsCrossID(t *testing.T) {
	old := mechanismBlind(21, -10)
	old.LegacyOverlayLevelBP = 20000
	incoming := StatusDef{ID: 1000, Level: 3}
	if overlayReplacementBlocked(old, incoming) {
		t.Fatal("storage 21 replaced logical 2")
	}
	if !overlayReplacementBlocked(incoming, old) {
		t.Fatal("reverse legacy level lost")
	}
	old.LegacyOverlayLevelBP = 9223372036854775807
	if !overlayReplacementBlocked(old, incoming) {
		t.Fatal("wide logical level overflow")
	}
	if overlayReplacementBlocked(old, mechanismBlind(25, -18)) {
		t.Fatal("cross-ID alias affected same-ID affix comparison")
	}
}

func TestSkillMechanismsMetadataRefreshOnlyChangesRules(t *testing.T) {
	canonical := mechanismBlind(25, -18)
	canonical.LegacyOverlayLevelBP = 36000
	native := mechanismBlind(6, -30)
	table := StatusTable{{ID: 1008, Level: 25}: canonical, {ID: 1008, Level: 6}: native}
	set := NewStatusSet()
	for _, def := range []StatusDef{canonical, native} {
		def.OverlayStrength = nil
		def.LegacyOverlayLevelBP = 0
		// Preserve a grown instance rather than replacing it with the canonical row.
		def.Affixes = []Affix{{Attr: AttrHit, Mode: ModePercent, Value: -47}}
		set.act[1008] = append(set.act[1008], Active{Def: def, ExpireAt: 989, NextTickAt: 17, Source: 42, ShieldLeft: 345, PeriodicDamageRemainder100: 37, PeriodicDamageTickExtra: 2})
	}
	before := append([]Active(nil), set.act[1008]...)
	set.RefreshOverlayStrengths(table)
	for i, got := range set.act[1008] {
		want := before[i]
		metadata := table[StatusKey{ID: got.Def.ID, Level: got.Def.Level}]
		want.Def.OverlayStrength = metadata.OverlayStrength
		want.Def.LegacyOverlayLevelBP = metadata.LegacyOverlayLevelBP
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("metadata refresh changed active fields:\ngot%+v\nwant%+v", got, want)
		}
	}
}
