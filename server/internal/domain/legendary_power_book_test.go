package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func powerBookNumericFixture(t *testing.T) (Stack, ItemDef) {
	t.Helper()
	for _, p := range LegendaryPowers() {
		if p.Class == "全职业" && p.Category == "攻击" && LegendaryPowerHasRoll(p.ID) {
			st, d := wingPowerDonor(p.ID)
			st.Legendary.PowerRollBP = 8000
			return st, d
		}
	}
	t.Fatal("no executable numeric power is wired")
	return Stack{}, ItemDef{}
}

func TestPowerBookRecordsOnlyHigherCoherentRoll(t *testing.T) {
	st, _ := powerBookNumericFixture(t)
	book := LegendaryPowerBook{}
	id := st.Legendary.PowerID
	for _, tc := range []struct {
		roll    int32
		changed bool
		want    int32
	}{{8000, true, 8000}, {7500, false, 8000}, {8000, false, 8000}, {9900, true, 9900}, {0, true, 0}, {10000, false, 0}} {
		st.Legendary.PowerRollBP = tc.roll
		changed, err := book.RecordSalvagedPower(st)
		if err != nil || changed != tc.changed || book[id] != tc.want {
			t.Fatalf("roll %d: changed=%v book=%v err=%v", tc.roll, changed, book, err)
		}
	}
	before := book[id]
	st.Legendary.PowerRollBP = 10001
	if _, err := book.RecordSalvagedPower(st); err == nil || book[id] != before {
		t.Fatal("invalid record changed book")
	}
}

func TestPowerBookPayloadPreservesUnrelatedProgressAndFailsClosed(t *testing.T) {
	st, _ := powerBookNumericFixture(t)
	book := LegendaryPowerBook{st.Legendary.PowerID: 8000}
	payload, err := WriteLegendaryPowerBook(`{"rift":{"tier":45},"other":[1,2]}`, book)
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]json.RawMessage
	json.Unmarshal([]byte(payload), &all)
	if string(all["rift"]) != `{"tier":45}` || string(all["other"]) != `[1,2]` {
		t.Fatal("unrelated state lost", payload)
	}
	loaded, err := ReadLegendaryPowerBook(payload)
	if err != nil || !reflect.DeepEqual(book, loaded) {
		t.Fatal("roundtrip", loaded, err)
	}
	loaded[st.Legendary.PowerID] = 9000
	if book[st.Legendary.PowerID] != 8000 {
		t.Fatal("read aliases book")
	}
	for _, bad := range []string{`null`, `[]`, `{"powerBook":null}`, `{"powerBook":[]}`, `{"powerBook":{"missing":10000}}`, `{"powerBook":42}`, `not json`} {
		if _, err := ReadLegendaryPowerBook(bad); err == nil {
			t.Fatal("accepted corrupt payload", bad)
		}
		if _, err := WriteLegendaryPowerBook(bad, book); err == nil {
			t.Fatal("silently overwrote corrupt payload", bad)
		}
	}
}

func TestPowerBookImprintPreservesEquipmentAndDoesNotAutoUpgrade(t *testing.T) {
	st, def := powerBookNumericFixture(t)
	st.RefineLevel = 15
	st.Bound = true
	st.Endgame.SourceLevel = 100
	st.Endgame.Revision = 7
	book := LegendaryPowerBook{st.Legendary.PowerID: 9500}
	before := st
	got, err := book.ImprintPower(st, def, &Character{Level: 100}, st.Legendary.PowerID)
	if err != nil {
		t.Fatal(err)
	}
	expected := before
	expected.Legendary.PowerRollBP = 9500
	if got != expected || st != before {
		t.Fatal("imprint changed unrelated equipment or aliased source")
	}
	book[st.Legendary.PowerID] = 10000
	if got.Legendary.PowerRollBP != 9500 {
		t.Fatal("record update mutated existing equipment")
	}
	if _, err := book.ImprintPower(got, def, &Character{Level: 100}, st.Legendary.PowerID); err != nil {
		t.Fatal("explicit reimprint failed", err)
	}
}

func TestPowerBookRejectsLockedOrdinaryForeignAndInvalidTargets(t *testing.T) {
	st, def := powerBookNumericFixture(t)
	book := LegendaryPowerBook{st.Legendary.PowerID: 9500}
	for _, mutate := range []func(*Stack){func(s *Stack) { s.Locked = true }, func(s *Stack) { s.Count = 2 }, func(s *Stack) { s.UID = 0 }, func(s *Stack) { s.Legendary = LegendaryInstance{} }, func(s *Stack) { s.Legendary.Bounds[0].Max = 0 }, func(s *Stack) { s.Item++ }, func(s *Stack) { s.SocketCount = 1 }} {
		bad := st
		mutate(&bad)
		got, err := book.ImprintPower(bad, def, &Character{Level: 100}, st.Legendary.PowerID)
		if err == nil || got != bad {
			t.Fatal("bad target accepted or mutated")
		}
	}
	if _, err := book.ImprintPower(st, def, &Character{}, "missing"); err == nil {
		t.Fatal("unrecorded accepted")
	}
	if _, err := book.ImprintPower(st, def, nil, st.Legendary.PowerID); err == nil {
		t.Fatal("missing character accepted")
	}
}

func TestPowerBookImprintRespectsClassLevelAndSlot(t *testing.T) {
	st, def := wingPowerDonor("TY-A01")
	RegisterLegendaryPowerExecutable("SS-A01")
	RegisterLegendaryPowerExecutable("TY-D01")
	book := LegendaryPowerBook{"SS-A01": 0, "TY-D01": 0}
	warlock := &Character{Race: Warlock, Employed: true, EmploymentKnown: true, Level: 100}
	warrior := &Character{Race: Warrior, Employed: true, EmploymentKnown: true, Level: 100}
	if _, err := book.ImprintPower(st, def, warrior, "SS-A01"); err == nil {
		t.Fatal("foreign class power imprinted")
	}
	if _, err := book.ImprintPower(st, def, warlock, "SS-A01"); err != nil {
		t.Fatal("eligible class power rejected", err)
	}
	st.Endgame.SourceLevel = 1
	if _, err := book.ImprintPower(st, def, warlock, "SS-A01"); err == nil {
		t.Fatal("high player level bypassed low item source level")
	}
	st.Endgame.SourceLevel = 100
	if _, err := book.ImprintPower(st, def, warlock, "TY-D01"); err == nil {
		t.Fatal("defense universal power imprinted onto weapon")
	}
	def.Equip.Slot = int32(SlotFace)
	if _, err := book.ImprintPower(st, def, warlock, "SS-A01"); err != nil {
		t.Fatal("all-slot class eligibility regressed", err)
	}
	def.Equip.Need.Professions = 1
	if _, err := book.ImprintPower(st, def, warlock, "SS-A01"); err == nil {
		t.Fatal("native base profession ignored")
	}
}
