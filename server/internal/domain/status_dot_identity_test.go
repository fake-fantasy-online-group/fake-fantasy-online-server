package domain

import "testing"

func TestGenericDOTIdentitySeparatesProducerAndPresentation(t *testing.T) {
	for _, power := range []string{"ZS-A01", "JK-A01"} {
		d := StatusDef{ID: 1900012345, LegendaryPowerID: power, LegendarySourceUID: 123, SourceSkillID: 10002, SynergyDOTNew: true, SynergyDOTVersion: 1, IntervalSec: 1, DurationSec: 6, ClassSetPoisonBonusTotal: 600}
		if GenericDOTFamily(d) != DOTBleed {
			t.Fatalf("legacy %s was not migrated", power)
		}
		name, icon := GenericDOTIdentity(GenericDOTFamily(d))
		if name != "流血" || icon != DOTBleedIcon || d.SourceSkillID != 10002 || d.LegendarySourceUID != 123 || d.ID != 1900012345 {
			t.Fatal("display changed provenance or status key")
		}
	}
	for _, source := range []int32{10002, 10102, 77777} {
		d := StatusDef{ID: 1900088888, DOTFamily: DOTBleed, SourceSkillID: source, IntervalSec: 1, DurationSec: 6, HPChanges: []ResourceChange{{Value: -50, Mode: ModeAbsolute}}}
		if GenericDOTFamily(d) != DOTBleed {
			t.Fatalf("shared bleed rejected source %d", source)
		}
	}
	for _, d := range []StatusDef{{ID: StatusPoison, IntervalSec: 2, DurationSec: 60, HPChanges: []ResourceChange{{Value: -28, Mode: ModeAbsolute}}}, {Name: "燃烧", IntervalSec: 2, DurationSec: 40, HPChanges: []ResourceChange{{Value: -54, Mode: ModeAbsolute}}}} {
		if GenericDOTFamily(d) == "" {
			t.Fatal("native DOT not classified")
		}
	}
	if GenericDOTFamily(StatusDef{Name: "流血", SourceSkillID: 10002}) != "" {
		t.Fatal("nonperiodic skill falsely classified")
	}
}
