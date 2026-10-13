package domain

import (
	"math"
	"testing"
)

func TestRiftSaturatingArithmetic(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want int32
	}{
		{math.MaxInt64, math.MaxInt32}, {math.MinInt64, math.MinInt32},
		{math.MaxInt32, math.MaxInt32}, {math.MinInt32, math.MinInt32}, {0, 0}, {-100, -100},
	} {
		if got := SaturateInt32(tc.in); got != tc.want {
			t.Fatalf("%d: got %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestRiftHighStatsSurviveStatusRecomputation(t *testing.T) {
	baseline := Stats{MaxHP: math.MaxInt32, MinAtk: math.MaxInt32, MaxAtk: math.MaxInt32, MAtk: math.MaxInt32}
	got := ApplyStatusStats(baseline, []Affix{
		{Attr: AttrMaxHP, Mode: ModePercent, Value: 10},
		{Attr: AttrAtk, Mode: ModePercent, Value: -10},
		{Attr: AttrMAtk, Value: 100},
	})
	wantAttack := int32(int64(math.MaxInt32) - int64(math.MaxInt32)*10/100)
	if got.MaxHP != math.MaxInt32 || got.MAtk != math.MaxInt32 || got.MinAtk != wantAttack || got.MaxAtk != wantAttack {
		t.Fatalf("scaled stats wrapped during buff/debuff: %+v", got)
	}
}

func TestRiftHighHPHealing(t *testing.T) {
	e := SkillEffect{Kind: EffectHeal, HealPctOfMax: 10, HealFlat: 100}
	if got, want := e.HealAmount(math.MaxInt32), int32(int64(math.MaxInt32)*10/100+100); got != want {
		t.Fatalf("heal=%d want=%d", got, want)
	}
	e.HealPctOfMax = 200
	if got := e.HealAmount(math.MaxInt32); got != math.MaxInt32 {
		t.Fatalf("healing must saturate: %d", got)
	}
}

func TestRiftHighDamageStatusMitigation(t *testing.T) {
	for _, magic := range []bool{false, true} {
		set := NewStatusSet()
		set.Apply(StatusDef{ID: 9001, DurationSec: 60,
			PhysicalDamageTakenPct: 100, PhysicalDamageReductionPct: 75,
			MagicDamageTakenPct: 100, MagicDamageReductionPct: 75}, 0, 1)
		if got, _ := set.MitigateDamage(magic, math.MaxInt32); got != 1073741824 {
			t.Fatalf("magic=%v increased/reduced damage=%d", magic, got)
		}
	}
	set := NewStatusSet()
	set.Apply(StatusDef{ID: 9001, DurationSec: 60, PhysicalDamageTakenPct: 100}, 0, 1)
	if got, _ := set.MitigateDamage(false, math.MaxInt32); got != math.MaxInt32 {
		t.Fatalf("vulnerability must saturate: %d", got)
	}
}
