package domain

import "testing"

func TestLegendaryHealingReductionStrongestEffectiveOnly(t *testing.T) {
	var none *StatusSet
	if none.HealingReceivedAmount(100) != 100 {
		t.Fatal("nil changed heal")
	}
	s := NewStatusSet()
	s.Apply(StatusDef{ID: 90001, Level: 1, DurationSec: 8, TypeKnown: true, Type: StatusBad, HealingReceivedReductionPct: 65}, 0, 1)
	s.Apply(StatusDef{ID: 90002, Level: 1, DurationSec: 8, TypeKnown: true, Type: StatusBad, HealingReceivedReductionPct: 20}, 0, 2)
	if n := s.HealingReceivedAmount(100); n != 35 {
		t.Fatal("reductions stacked", n)
	}
	s.Apply(StatusDef{ID: 90003, Level: 1, DurationSec: 2, TypeKnown: true, Type: StatusGood, SuppressHarmful: true}, 0, 3)
	if n := s.HealingReceivedAmount(100); n != 100 {
		t.Fatal("suppressed harmful reduction still applied", n)
	}
	s.Expire(Ticks(2000))
	if n := s.HealingReceivedAmount(100); n != 35 {
		t.Fatal(n)
	}
	s.Expire(Ticks(8000))
	if n := s.HealingReceivedAmount(100); n != 100 {
		t.Fatal(n)
	}
}
