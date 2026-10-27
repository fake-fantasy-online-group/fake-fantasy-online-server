package domain

import (
	"testing"
	"time"
)

func TestLegendaryPermanentNativeModifierConsumptionAndSave(t *testing.T) {
	s := NewStatusSet()
	d := StatusDef{ID: 1027, Permanent: true, NextSkillDamagePct: 55, LegendaryPermanentPower: "SS-D10"}
	s.Apply(d, 0, 1)
	for i := 0; i < 3; i++ {
		damage, _, _, removed := s.ConsumeAttackModifiers(true, true, false, false)
		if damage != 55 || len(removed) != 0 || !s.Has(d.ID) {
			t.Fatalf("permanent consumed: %d %v", damage, removed)
		}
	}
	if len(s.SaveForLogout(0, time.Now(), 1)) != 0 {
		t.Fatal("equipment passive persisted")
	}
	saved := []SavedStatus{{Def: d, RemainingTicks: -1, SourceSelf: true}}
	if RestoreSavedStatuses(saved, 0, time.Now(), 1).Has(d.ID) {
		t.Fatal("restored free equipment passive")
	}
	native := NewStatusSet()
	d.LegendaryPermanentPower = ""
	native.Apply(d, 0, 1)
	native.ConsumeAttackModifiers(true, true, false, false)
	if native.Has(d.ID) {
		t.Fatal("unrelated native Permanent flag bypasses one-shot consumption")
	}
}
