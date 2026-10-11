package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func uniqueFixture(d UniqueDefinition) ItemDef {
	return ItemDef{ID: d.BaseID, Name: d.BaseName, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{Slot: d.Slot, LevelReq: d.LevelRequirement, ResourceLevel: d.ResourceLevel, Durable: 100,
		Need: Requirement{Professions: d.Professions, BeginnerAllowed: d.BeginnerAllowed, ProfessionRestricted: !d.Unrestricted, Sex: d.Sex}}}
}
func TestUniqueCatalogCompleteDistinctAndValid(t *testing.T) {
	all := UniqueCatalog()
	if len(all) != 1619 {
		t.Fatal(len(all))
	}
	names, packages, traits := map[string]bool{}, map[string]bool{}, map[string]bool{}
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
		if d.Slot < 1 || d.Slot > 25 || len(d.Affixes) != 3 || len(d.Traits) != 2 {
			t.Fatal(d.ID)
		}
		b, _ := json.Marshal(struct {
			A []Affix
			T []UniqueTrait
		}{d.Affixes, d.Traits})
		if packages[string(b)] {
			t.Fatalf("identical package %d", d.ID)
		}
		packages[string(b)] = true
		for _, tr := range d.Traits {
			if !SupportedUniqueTrait(tr.Kind) || tr.Describe(10000) == "" {
				t.Fatal(tr)
			}
			traits[tr.Kind] = true
		}
		defs[d.BaseID] = uniqueFixture(d)
	}
	if native != 885 || ready != 298 || len(traits) != 28 {
		t.Fatalf("native=%d ready=%d traits=%d", native, ready, len(traits))
	}
	if err := ValidateUniqueCatalog(defs); err != nil {
		t.Fatal(err)
	}
	delete(defs, all[0].BaseID)
	if ValidateUniqueCatalog(defs) == nil {
		t.Fatal("accepted coverage drift")
	}
	all[0].Affixes[0].Value = -999
	if again, _ := UniqueForBase(all[0].BaseID); again.Affixes[0].Value < 0 {
		t.Fatal("catalogue escaped mutable slice")
	}
}
func TestUniqueApplicationPreservesAllLegacyState(t *testing.T) {
	d := UniqueCatalog()[0]
	def := uniqueFixture(d)
	st := Stack{UID: 77, Item: d.BaseID, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 70, MaxDurability: 100, DurabilityWearRaw: 12,
		RefineLevel: 3, SocketCount: 1, Sockets: [5]ItemID{123}, SocketUIDs: [5]int64{987}, Bound: true, Locked: true, FusedSoul: 5, WashCount: 1}
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
	if _, ok := UniqueOf(st); ok {
		t.Fatal("accepted malformed potency")
	}
	st = before
	if ApplyUnique(&st, def, func(int) int { return -1 }) || st != before {
		t.Fatal("bad random changed item")
	}
}
func TestUniqueStatsAndAllExtendedSlots(t *testing.T) {
	d := UniqueCatalog()[0]
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
	for _, d := range UniqueCatalog() {
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
