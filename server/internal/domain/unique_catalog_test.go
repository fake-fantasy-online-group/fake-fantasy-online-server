package domain

import (
	"reflect"
	"testing"
)

func uniqueFixture(d UniqueDefinition) ItemDef {
	return ItemDef{EndgameCatalog: testEndgameCatalog(), ID: d.BaseID, Name: d.BaseName, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{NativeDropEligible: d.NativeDropEligible, Slot: d.Slot, LevelReq: d.LevelRequirement, ResourceLevel: d.ResourceLevel, Durable: 100,
		Need: Requirement{Professions: d.Professions, BeginnerAllowed: d.BeginnerAllowed, ProfessionRestricted: !d.Unrestricted, Sex: d.Sex}}}
}
func TestUniqueCatalogCompleteDistinctAndValid(t *testing.T) {
	all := UniqueCatalog(testEndgameCatalog())
	if len(all) != UniqueCatalogCount {
		t.Fatal(len(all))
	}
	names, traits := map[string]bool{}, map[string]bool{}
	defs := map[ItemID]ItemDef{}
	native, ready := 0, 0
	for _, d := range all {
		if names[d.Name] || d.Name == d.BaseName || d.Lore == "" || d.SourceArm == "" || d.SourceDesc == "" {
			t.Fatalf("invalid name/provenance %d", d.ID)
		}
		names[d.Name] = true
		if d.NativeDropEligible {
			native++
		}
		if d.LegacyAffixPoolReady {
			ready++
		}
		if d.Slot < 1 || d.Slot > 25 || validateUniqueDefinition(d, testEndgameCatalog()) != nil {
			t.Fatal(d.ID)
		}
		for _, tr := range d.Traits {
			if !SupportedUniqueTrait(tr.Kind) || tr.Describe(10000) == "" {
				t.Fatal(tr)
			}
			traits[tr.Kind] = true
		}
		defs[d.BaseID] = uniqueFixture(d)
		// This structural fixture needs nonzero white bases for explicit local rows.
		defs[d.BaseID].Equip.Bonus = Stats{MinAtk: 100, MaxAtk: 200, MAtk: 100, Def: 100, MDef: 100}
	}
	if native != 259 || ready != 259 {
		t.Fatalf("native=%d ready=%d traits=%d", native, ready, len(traits))
	}
	if err := ValidateUniqueCatalog(defs, testEndgameCatalog()); err != nil {
		t.Fatal(err)
	}
	delete(defs, all[0].BaseID)
	if ValidateUniqueCatalog(defs, testEndgameCatalog()) == nil {
		t.Fatal("accepted coverage drift")
	}
	all[0].Affixes[0].Value = -999
	if again, _ := UniqueForBase(all[0].BaseID, testEndgameCatalog()); again.Affixes[0].Value < 0 {
		t.Fatal("catalogue escaped mutable slice")
	}
}
func TestUniqueApplicationPreservesAllLegacyState(t *testing.T) {
	d := UniqueCatalog(testEndgameCatalog())[0]
	def := uniqueFixture(d)
	st := Stack{UID: 77, Item: d.BaseID, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 70, MaxDurability: 100, DurabilityWearRaw: 12,
		RefineLevel: 3, RolledAffixCount: 2, RolledAffixes: [5]InstanceAffix{{Affix: Affix{Attr: AttrAtk, Value: 37, Mode: ModePercent}}, {Affix: Affix{Attr: AttrMaxAtk, Value: 57, Mode: ModeAbsolute}}}, Bound: true, Locked: true, FusedSoul: 5, WashCount: 1}
	before := st
	if !ApplyUnique(&st, def, func(n int) int { return n - 1 }) || st.Endgame.UniqueRollBP != 12000 || st.Endgame.UniqueID != d.ID {
		t.Fatal(st)
	}
	metadata := st.Endgame
	st.Endgame = before.Endgame
	if !reflect.DeepEqual(st, before) {
		t.Fatal("legacy progression changed")
	}
	st.Endgame = metadata
	if ApplyUnique(&st, def, func(int) int { return 0 }) {
		t.Fatal("awakened twice")
	}
	st.Endgame.UniqueRollBP = 7999
	if _, ok := UniqueOf(st, testEndgameCatalog()); ok {
		t.Fatal("accepted malformed potency")
	}
	st = before
	if ApplyUnique(&st, def, func(int) int { return -1 }) || st != before {
		t.Fatal("bad random changed item")
	}
}
func TestUniqueStatsAndAllExtendedSlots(t *testing.T) {
	d := UniqueCatalog(testEndgameCatalog())[0]
	def := uniqueFixture(d)
	defs := func(id ItemID) (ItemDef, bool) { return def, id == def.ID }
	st := Stack{UID: 77, Item: def.ID, Count: 1, MaxDurability: 100, Durability: 100}
	ApplyUnique(&st, def, func(int) int { return 2000 })
	for slot := EquipSlot(14); slot <= 25; slot++ {
		w := NewEquipSet()
		w.Set(slot, st)
		seen := 0
		w.Each(func(got EquipSlot, _ Stack) {
			if got != slot {
				t.Fatal(got)
			}
			seen++
		})
		if seen != 1 {
			t.Fatal("lost extended slot", slot)
		}
		base := Base{STR: 10, VIT: 10, INT: 10, AGI: 10, DEX: 10, SPI: 10}
		_, with := Compute(StatSource{Base: base, Worn: w, Defs: defs})
		plain := st
		plain.Endgame = EndgameItemState{}
		w.Set(slot, plain)
		_, without := Compute(StatSource{Base: base, Worn: w, Defs: defs})
		if with == without {
			t.Fatal("unique affixes did not execute", slot)
		}
	}
}
func TestUniqueDropsRespectNativeMembership(t *testing.T) {
	for _, d := range UniqueCatalog(testEndgameCatalog()) {
		def := uniqueFixture(d)
		st := Stack{UID: 77, Item: d.BaseID, Count: 1}
		got := RollUniqueDrop(&st, def, func(int) int { return 0 })
		if got != d.NativeDropEligible {
			t.Fatal(d.ID)
		}
		st.Endgame = EndgameItemState{}
		def.Equip.NoTypeDrop = true
		if RollUniqueDrop(&st, def, func(int) int { return 0 }) {
			t.Fatal("changed native drop exclusion")
		}
	}
}
