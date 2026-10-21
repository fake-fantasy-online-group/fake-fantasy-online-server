package domain

import (
	"testing"
	"time"
)

func TestClassSetPoisonRemainderSurvivesSaveRestore(t *testing.T) {
	def := StatusDef{ID: StatusPoison, Level: 21, TypeKnown: true, Type: StatusBad, DurationSec: 60, IntervalSec: 2, ClassSetPoison: true, ClassSetPoisonBonusTotal: 1010}
	set := NewStatusSet()
	set.Apply(def, 0, 1)
	var total int32
	for i := 1; i <= 7; i++ {
		for _, a := range set.DueTicks(Ticks(i * 2000)) {
			total += a.PeriodicDamageTickExtra
		}
	}
	now := Ticks(14000)
	wall := time.Unix(1000000, 0)
	saved := set.SaveForLogout(now, wall, 1)
	if len(saved) != 1 || saved[0].ClassSetPoisonRemainder == 0 {
		t.Fatalf("lost remainder: %+v", saved)
	}
	set = RestoreSavedStatuses(saved, now, wall, 1)
	for i := 8; i <= 30; i++ {
		for _, a := range set.DueTicks(Ticks(i * 2000)) {
			total += a.PeriodicDamageTickExtra
		}
	}
	if total != 1010 {
		t.Fatalf("total=%d", total)
	}
}
