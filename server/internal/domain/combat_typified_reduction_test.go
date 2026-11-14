package domain

import "testing"

func TestTypifiedReductionEscapesOrdinaryFloor(t *testing.T) {
	ordinary := CombatDamageBuckets{DamageReductionPct: 60, SelectiveReductionBP: 7000}
	if n := ordinary.IncomingDamage(1000); n != 250 {
		t.Fatal("selective floor changed", n)
	}
	typified := CombatDamageBuckets{DamageReductionPct: 60, TypifiedReductionBP: 7000}
	if n := typified.IncomingDamage(1000); n != 120 {
		t.Fatal("typified reduction must escape the 25% floor", n)
	}
	if n := (CombatDamageBuckets{TypifiedReductionBP: 9000}).IncomingDamage(1000); n != 300 {
		t.Fatal("typified reduction must still cap at 70%", n)
	}
}
