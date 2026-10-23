package domain

import "testing"

func TestLegendaryRegistryPreservesApprovedCountAndIsolation(t *testing.T) {
	all := LegendaryPowers()
	if len(all) != 330 {
		t.Fatal(len(all))
	}
	seen := map[string]bool{}
	for _, p := range all {
		if seen[p.ID] || p.Name == "" || p.Effect == "" {
			t.Fatal(p.ID)
		}
		seen[p.ID] = true
	}
	first := all[0]
	first.Slots[0] = "modified"
	again, _ := LegendaryPower(first.ID)
	if again.Slots[0] == "modified" {
		t.Fatal("mutable catalog alias")
	}
}
func TestLegendaryTypeScopeAndOriginalLevelRules(t *testing.T) {
	p, _ := LegendaryPower("ZS-A01")
	base := ItemDef{Equip: &EquipDef{Slot: int32(SlotWeapon), LevelReq: p.MinLevel, Need: Requirement{Professions: 1}}}
	if !LegendaryPowerEligible(p, base) {
		t.Fatal("valid class attack weapon")
	}
	base.Equip.LevelReq--
	if LegendaryPowerEligible(p, base) {
		t.Fatal("low original base grew to power threshold")
	}
	base.Equip.LevelReq++
	base.Equip.Slot = int32(SlotRing)
	if LegendaryPowerEligible(p, base) {
		t.Fatal("class power leaked to universal-only slot")
	}
	shared, _ := LegendaryPower("TY-A01")
	if !LegendaryPowerEligible(shared, base) {
		t.Fatal("shared attack ring")
	}
	base.Equip.Slot = int32(SlotShoe)
	if LegendaryPowerEligible(shared, base) {
		t.Fatal("shoe not an approved legendary slot")
	}
	base.Equip.Slot = int32(SlotTwoHand)
	base.Equip.Need.Professions = 2
	if LegendaryPowerEligible(p, base) {
		t.Fatal("wrong class base")
	}
}
