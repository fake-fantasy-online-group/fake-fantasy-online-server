package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLegendaryPowerRollBoundsLegacyAndIsolation(t *testing.T) {
	for _, spec := range LegendaryPowerRollSpecs() {
		id := spec.ID
		original, _ := LegendaryPower(id)
		legacy, ok := LegendaryPowerForRoll(id, 0)
		if !ok || legacy.Effect != original.Effect || EffectiveLegendaryPowerRollBP(id, 0) != 10000 {
			t.Fatal(id, "legacy changed")
		}
		if len(spec.Parameters) == 0 {
			if spec.FixedReason == "" || LegendaryPowerRollValid(id, 7500) {
				t.Fatal(id, "fixed mechanic accepts numerical roll")
			}
			continue
		}
		for _, invalid := range []int32{-1, 1, 7499, 10001, 2147483647} {
			if LegendaryPowerRollValid(id, invalid) {
				t.Fatal(id, "invalid accepted", invalid)
			}
			if _, ok := LegendaryPowerForRoll(id, invalid); ok {
				t.Fatal(id, "invalid description")
			}
		}
		for _, roll := range []int32{7500, 8123, 9999, 10000} {
			p, ok := LegendaryPowerForRoll(id, roll)
			if !ok || strings.Contains(p.Effect, "～") {
				t.Fatal(id, "equipment description contains range", p.Effect)
			}
			for _, parameter := range spec.Parameters {
				value := LegendaryPowerRollValue(id, parameter.Key, parameter.Base, roll)
				want := max(int32(1), int32(int64(parameter.Base)*int64(roll)/10000))
				if value != want || value > parameter.Base {
					t.Fatal(id, parameter.Key, value, want)
				}
			}
		}
		// Consumers cannot mutate registry metadata through returned slices.
		spec.Parameters[0].Key = "corrupted"
		for _, again := range LegendaryPowerRollSpecs() {
			if again.ID == id && again.Parameters[0].Key == "corrupted" {
				t.Fatal("mutable parameters")
			}
		}
	}
	if LegendaryPowerRollValid("unknown", 0) {
		t.Fatal("unknown power accepted")
	}
}

func TestLegendaryPowerRollDrawAndJSONPreserveExactValues(t *testing.T) {
	for _, draw := range []int{0, 1234, 2500} {
		v := LegendaryInstance{PowerID: "TY-A02"}
		if !v.RollPower(func(n int) int {
			if n != 2501 {
				t.Fatal(n)
			}
			return draw
		}) || v.PowerRollBP != 7500+int32(draw) {
			t.Fatal(v)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var restored LegendaryInstance
		if json.Unmarshal(raw, &restored) != nil || restored != v {
			t.Fatal("roll was rerolled/lost", string(raw))
		}
	}
	for _, bad := range []int{-1, 2501} {
		v := LegendaryInstance{PowerID: "TY-A02"}
		if v.RollPower(func(int) int { return bad }) || v.PowerRollBP != 0 {
			t.Fatal("bad RNG mutated", v)
		}
	}
	pure := LegendaryInstance{PowerID: "TY-A09"}
	if !pure.RollPower(func(int) int { t.Fatal("pure mechanic consumed RNG"); return 0 }) || pure.PowerRollBP != 0 {
		t.Fatal(pure)
	}
}

func TestLegendaryPowerRollStrongestDuplicateAndWingExtraction(t *testing.T) {
	low, d := wingPowerDonor("TY-D01")
	low.UID = 10
	low.Durability = 10
	low.MaxDurability = 10
	low.Legendary.PowerRollBP = 7500
	high := low
	high.UID = 20
	high.Item++
	high.Legendary.PowerRollBP = 10000
	d2 := d
	d2.ID = high.Item
	copyEquip := *d.Equip
	d2.Equip = &copyEquip
	d2.Equip.Slot = int32(SlotHead)
	worn := NewEquipSet()
	worn.Set(SlotBody, low)
	worn.Set(SlotHead, high)
	defs := func(id ItemID) (ItemDef, bool) {
		if id == d.ID {
			return d, true
		}
		if id == d2.ID {
			return d2, true
		}
		return ItemDef{}, false
	}
	ch := &Character{Level: 100, Race: Warrior}
	active := ActiveLegendaryPowers(worn, defs, ch)
	if len(active) != 1 || active[0].SourceUID != 20 || active[0].PowerRollBP != 10000 {
		t.Fatal("weaker duplicate won", active)
	}
	high.Legendary.PowerRollBP = 0
	worn.Set(SlotHead, high)
	active = ActiveLegendaryPowers(worn, defs, ch)
	if len(active) != 1 || active[0].SourceUID != 20 {
		t.Fatal("legacy lost full strength", active)
	}
	wing := Stack{UID: 90, Item: 20609, Count: 1, InstanceKind: ItemInstanceEquipment}
	got, err := ExtractWingPower(wing, low, d)
	if err != nil || got.WingPowers[1].PowerRollBP != 7500 {
		t.Fatal("wing extraction lost roll", got, err)
	}
	low.Legendary.PowerRollBP = 7499
	if _, err := ExtractWingPower(wing, low, d); err == nil {
		t.Fatal("corrupt roll accepted")
	}
}

func TestLegendaryPowerRollAllParameterOutcomesMonotonic(t *testing.T) {
	for _, spec := range LegendaryPowerRollSpecs() {
		for _, p := range spec.Parameters {
			previous := int32(0)
			for roll := LegendaryPowerRollMinBP; roll <= LegendaryPowerRollMaxBP; roll++ {
				value := LegendaryPowerRollValue(spec.ID, p.Key, p.Base, roll)
				if value < previous || value <= 0 || value > p.Base {
					t.Fatalf("%s %s roll%d: %d after%d", spec.ID, p.Key, roll, value, previous)
				}
				previous = value
			}
			if previous != p.Base {
				t.Fatal("top roll differs baseline", spec.ID, p.Key, previous, p.Base)
			}
		}
	}
	if got := LegendaryPowerRollValue("TY-A02", "benefit", 6001, 7500); got != 6001 {
		t.Fatal("unregistered coefficient silently scaled", got)
	}
	if got := LegendaryPowerRollValue("TY-A02", "trigger", 10, 7500); got != 10 {
		t.Fatal("trigger silently scaled", got)
	}
}

func TestLegendaryPowerRollAllPowersClassified(t *testing.T) {
	specs := LegendaryPowerRollSpecs()
	if len(specs) != len(LegendaryPowers()) {
		t.Fatalf("%d of %d powers classified", len(specs), len(LegendaryPowers()))
	}
	ranged, fixed := 0, 0
	for _, spec := range specs {
		if len(spec.Parameters) == 0 {
			fixed++
			if spec.FixedReason == "" {
				t.Fatal("unexplained fixed power", spec.ID)
			}
			continue
		}
		ranged++
		variable := false
		for _, p := range spec.Parameters {
			variable = variable || LegendaryPowerRollValue(spec.ID, p.Key, p.Base, 7500) < p.Base
		}
		if !variable {
			t.Fatal("cosmetic range without a variable coefficient", spec.ID)
		}
	}
	if ranged != 315 || fixed != 15 {
		t.Fatal("re-review power coverage after classification changes", ranged, fixed)
	}
}

func TestLegendaryEquipmentCurrentValuesAndSeparateCodexRanges(t *testing.T) {
	for _, spec := range LegendaryPowerRollSpecs() {
		ranges := LegendaryPowerRangeText(spec.ID)
		if len(spec.Parameters) == 0 {
			if ranges != "" {
				t.Fatalf("%s fixed effect has a numerical range: %s", spec.ID, ranges)
			}
			continue
		}
		for _, parameter := range spec.Parameters {
			lo := powerRollNumber(LegendaryPowerRollValue(spec.ID, parameter.Key, parameter.Base, LegendaryPowerRollMinBP), parameter.Divisor) + parameter.Unit
			hi := powerRollNumber(parameter.Base, parameter.Divisor) + parameter.Unit
			if !strings.Contains(ranges, lo+"～"+hi) {
				t.Errorf("%s codex missing parameter %s range %s～%s: %s", spec.ID, parameter.Key, lo, hi, ranges)
			}
		}
		for _, roll := range []int32{0, LegendaryPowerRollMinBP, 8123, LegendaryPowerRollMaxBP} {
			p, ok := LegendaryPowerForRoll(spec.ID, roll)
			if !ok || strings.Contains(p.Effect, "～") {
				t.Fatalf("%s invalid equipment description: %s", spec.ID, p.Effect)
			}
			for _, parameter := range spec.Parameters {
				current := powerRollNumber(LegendaryPowerRollValue(spec.ID, parameter.Key, parameter.Base, roll), parameter.Divisor) + parameter.Unit
				if !strings.Contains(p.Effect, current) {
					t.Errorf("%s missing current %s: %s", spec.ID, current, p.Effect)
				}
			}
		}
	}
	if LegendaryPowerRangeText("unknown") != "" {
		t.Fatal("unknown power has ranges")
	}
}

func TestPowerBookRangeTextJSON(t *testing.T) {
	v := EndgamePowerBookView{PowerID: "ZS-A01", RangeText: LegendaryPowerRangeText("ZS-A01")}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EndgamePowerBookView
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RangeText != "连环枪法伤害提高60%～80%" {
		t.Fatalf("codex range contract: %s", raw)
	}
}
