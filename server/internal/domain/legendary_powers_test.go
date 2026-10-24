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
	if !LegendaryPowerEligible(shared, base) {
		t.Fatal("shared attack shoe")
	}
	base.Equip.Slot = int32(SlotTwoHand)
	base.Equip.Need.Professions = 2
	if LegendaryPowerEligible(p, base) {
		t.Fatal("wrong class base")
	}
}

// Preserve every existing category/scope intersection while admitting shoes to
// all three universal categories, never to profession-specific powers.
func TestLegendarySlotCategoryAndProfessionMatrix(t *testing.T) {
	categories := map[EquipSlot]map[string]bool{
		SlotFace:    {"防御": true, "通用": true},
		SlotHead:    {"攻击": true, "防御": true},
		SlotNeck:    {"攻击": true, "通用": true},
		SlotWeapon:  {"攻击": true, "通用": true},
		SlotShield:  {"防御": true, "通用": true},
		SlotGlove:   {"攻击": true, "防御": true},
		SlotRing:    {"攻击": true, "通用": true},
		SlotBody:    {"攻击": true, "防御": true},
		SlotShoe:    {"攻击": true, "防御": true, "通用": true},
		SlotBag:     {"防御": true, "通用": true},
		SlotTwoHand: {"攻击": true, "通用": true},
	}
	for slot := EquipSlot(0); slot <= maxEquipSlot+1; slot++ {
		for _, p := range LegendaryPowers() {
			base := ItemDef{Equip: &EquipDef{Slot: int32(slot), LevelReq: p.MinLevel, Need: Requirement{Professions: 31}}}
			classSlot := slot == SlotHead || slot == SlotBody || slot == SlotWeapon || slot == SlotTwoHand
			want := categories[slot][p.Category] && (p.Class == "全职业" || classSlot)
			if got := LegendaryPowerEligible(p, base); got != want {
				t.Fatalf("slot %d power %s category %s class %s: eligible=%v want %v", slot, p.ID, p.Category, p.Class, got, want)
			}
			base.Equip.LevelReq--
			if got := LegendaryPowerEligible(p, base); got != (want && p.Class == "全职业") {
				t.Fatalf("slot %d power %s has wrong universal/profession level policy", slot, p.ID)
			}
			base.Equip.LevelReq++
			base.Equip.Need.Professions = 0
			if got := LegendaryPowerEligible(p, base); got != (want && p.Class == "全职业") {
				t.Fatalf("slot %d power %s ignored profession restriction", slot, p.ID)
			}
		}
	}
}
