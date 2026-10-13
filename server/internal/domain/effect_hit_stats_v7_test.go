package domain

import (
	"math"
	"testing"
)

func TestEffectHitAffixUsesIndependentBasisPointsV7(t *testing.T) {
	src := StatSource{Base: Base{DEX: 40}, Innate: Stats{Hit: 3, EffectHitBP: 700}, Passive: []Affix{{Attr: AttrEffectHitBP, Value: 300, Mode: ModeAbsolute}}, Status: NewStatusSet()}
	src.Status.Apply(StatusDef{ID: 99001, Level: 1, Affixes: []Affix{{Attr: AttrEffectHitBP, Value: 200, Mode: ModeAbsolute}, {Attr: AttrEffectHitBP, Value: 100, Mode: ModePercent}}}, 0, 1)
	_, got := Compute(src)
	if got.EffectHitBP != 1200 || got.Hit != 23 {
		t.Fatalf("effect accuracy aliases ordinary hit or percent mode: %+v", got)
	}
	stats := ApplyStatusStats(Stats{Hit: 123, EffectHitBP: 800}, []Affix{{Attr: AttrEffectHitBP, Value: 450, Mode: ModeAbsolute}})
	if stats.Hit != 123 || stats.EffectHitBP != 1250 {
		t.Fatalf("monster/status pipeline lost effect hit: %+v", stats)
	}
	if !SupportedEndgameAffix(Affix{Attr: AttrEffectHitBP, Value: 100, Mode: ModeAbsolute}) || SupportedEndgameAffix(Affix{Attr: AttrEffectHitBP, Value: 100, Mode: ModePercent}) {
		t.Fatal("effect hit mode validation incorrect")
	}
	if AttrEffectHitBP == AttrHit || !SupportsEquipmentSuitAffix(Affix{Attr: AttrEffectHitBP, Value: 100, Mode: ModeAbsolute}) {
		t.Fatal("effect hit is not an independent recognized affix")
	}
}

func TestEffectHitAffixSaturatesV7(t *testing.T) {
	_, got := Compute(StatSource{Passive: []Affix{{Attr: AttrEffectHitBP, Value: math.MaxInt32, Mode: ModeAbsolute}, {Attr: AttrEffectHitBP, Value: math.MaxInt32, Mode: ModeAbsolute}}})
	if got.EffectHitBP != math.MaxInt32 {
		t.Fatalf("effect-hit aggregation overflowed: %d", got.EffectHitBP)
	}
}
