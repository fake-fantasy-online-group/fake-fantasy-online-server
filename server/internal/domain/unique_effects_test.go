package domain

import "testing"

func TestEveryUniqueTraitHasExecutableResult(t *testing.T) {
	if len(uniqueTraitNames) != 28 {
		t.Fatal(len(uniqueTraitNames))
	}
	for kind := range uniqueTraitNames {
		t.Run(kind, func(t *testing.T) {
			c := UniqueContext{HPPercent: 50, TargetHPPercent: 50, Magic: true, Critical: true, TargetElite: true, Invisible: true, Trap: true, Area: true, Skill: true, Healing: true, Ally: true, HurtType: HurtFire}
			switch kind {
			case "physical_guard":
				c.Magic = false
			case "basic_focus":
				c.Skill = false
			case "single_focus":
				c.Area = false
				c.Trap = false
			case "ice_mastery":
				c.HurtType = HurtIce
			case "holy_mastery":
				c.HurtType = HurtHoly
			}
			power := UniqueTrait{Kind: kind, PowerBP: 1000, ThresholdPct: 50}
			got := EvaluateUniqueTraits([]UniqueTrait{power}, c)
			if got == (UniqueEffects{}) {
				t.Fatal("tooltip-only trait", kind)
			}
			if doubled := EvaluateUniqueTraits([]UniqueTrait{power, power}, c); doubled != got {
				t.Fatal("same power stacked")
			}
		})
	}
}
func TestUniqueBoundariesCapsAndFunctionalClassRules(t *testing.T) {
	tr := UniqueTrait{Kind: "desperate_guard", PowerBP: 10000, ThresholdPct: 30}
	if x := EvaluateUniqueTraits([]UniqueTrait{tr}, UniqueContext{HPPercent: 30}); x.MitigationBP != 4500 {
		t.Fatal(x)
	}
	if x := EvaluateUniqueTraits([]UniqueTrait{tr}, UniqueContext{HPPercent: 31}); x.MitigationBP != 0 {
		t.Fatal(x)
	}
	if ApplyUniqueBP(2147483647, 10000) != 2147483647 || ApplyUniqueBP(100, -4500) != 55 {
		t.Fatal("unsafe scaling")
	}
	d := UniqueCatalog()[0]
	def := uniqueFixture(d)
	def.Equip.Need = Requirement{Professions: 1, ProfessionRestricted: true}
	w := NewEquipSet()
	st := Stack{UID: 77, Item: d.BaseID, Count: 1, MaxDurability: 100, Durability: 100}
	ApplyUnique(&st, def, func(int) int { return 2000 })
	w.Set(EquipSlot(def.Equip.Slot), st)
	ch := &Character{Race: Race(0), Level: 100}
	defs := func(ItemID) (ItemDef, bool) { return def, true }
	if len(ActiveUniqueTraits(w, defs, ch)) != 2 {
		t.Fatal("eligible traits absent")
	}
	ch.Race = Race(1)
	if len(ActiveUniqueTraits(w, defs, ch)) != 0 {
		t.Fatal("wrong class active")
	}
	ch.Race = Race(0)
	st.Durability = 0
	w.Set(EquipSlot(def.Equip.Slot), st)
	if len(ActiveUniqueTraits(w, defs, ch)) != 0 {
		t.Fatal("broken gear active")
	}
}
