package domain

import "testing"

func TestVerifiedFrequencySlowLeavesLegacyIntervalAffixesUnchanged(t *testing.T) {
	base := Stats{AtkSpeedMS: 1000, Def: 100}
	legacy := StatusDef{ID: 1009, Level: 1, DurationSec: 30, Affixes: []Affix{{Attr: AttrAtkSpeed, Mode: ModePercent, Value: 30}}}
	set := NewStatusSet()
	set.Apply(legacy, 0, 1)
	if got := ApplyMonsterStatusStats(base, set); got != ApplyStatusStats(base, legacy.Affixes) || got.AtkSpeedMS != 1300 {
		t.Fatal("unmarked legacy interval semantics changed", got)
	}
	// The frequency marker applies only to its own speed affix; other effects
	// and unrelated status rows keep the old additive attribute calculation.
	slow := StatusDef{ID: 19999, Level: 1, DurationSec: 30, AttackSpeedIsFrequency: true,
		Affixes: []Affix{{Attr: AttrAtkSpeed, Mode: ModePercent, Value: 30}, {Attr: AttrDef, Mode: ModePercent, Value: -10}}}
	set.Apply(slow, 0, 1)
	for i := 0; i < 10; i++ {
		got := ApplyMonsterStatusStats(base, set)
		if got.AtkSpeedMS != 1858 || got.Def != 90 {
			t.Fatal("double application, unrelated-affix loss or compounding", got)
		}
	}
	set.Remove(slow.ID)
	if got := ApplyMonsterStatusStats(base, set); got.AtkSpeedMS != 1300 {
		t.Fatal("removal left frequency penalty", got)
	}
}

func TestVerifiedFrequencySlowUsesExistingMinimumSpeed(t *testing.T) {
	for _, loss := range []int32{30, 35, 40, 45, 50, 55, 100, 1000000} {
		set := NewStatusSet()
		set.Apply(StatusDef{ID: 1009, Level: 11, DurationSec: 30, AttackSpeedIsFrequency: true,
			Affixes: []Affix{{Attr: AttrAtkSpeed, Mode: ModePercent, Value: loss}}}, 0, 1)
		wantSpeed := max(int32(50), 100-loss)
		got := ApplyMonsterStatusStats(Stats{AtkSpeedMS: 1000}, set)
		wantMS := (100000 + wantSpeed - 1) / wantSpeed
		if got.AtkSpeedMS != wantMS || got.AttackIntervalTicks() != Ticks(int(wantMS)) || got.PlayerAttackSpeedPercent() != wantSpeed {
			t.Fatal("frequency slow boundary mismatch", loss, got, wantMS)
		}
	}
}
