package domain

import "testing"

func TestEquipmentSkillVariantClosedDefinitions(t *testing.T) {
	seen := map[SkillID]bool{}
	for _, id := range []string{"sword_gale_fan", "warrior_piercing_lance", "assassin_bomb_fuse", "healer_guarded_mend", "warlock_flash_fire"} {
		v, ok := testEndgameCatalog().Variant(id)
		if !ok || v.SkillID == 0 || v.MaxTargets < 0 || v.MaxTargets > 6 || seen[v.SkillID] {
			t.Fatal(v)
		}
		seen[v.SkillID] = true
		if err := ValidateEquipmentEffects([]EquipmentEffect{{Kind: "skill_variant", VariantID: id}}, testEndgameCatalog()); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []EquipmentEffect{{Kind: "skill_variant"}, {Kind: "skill_variant", VariantID: "infinite_recursion"}, {Kind: "skill_variant", VariantID: "warlock_flash_fire", Value: 1}, {Kind: "magic_find_pct", Value: 1, VariantID: "warlock_flash_fire"}} {
		if ValidateEquipmentEffects([]EquipmentEffect{e}, testEndgameCatalog()) == nil {
			t.Fatal("accepted malformed variant", e)
		}
	}
}

func TestEquipmentVariantAreaHeadcountsRetired(t *testing.T) {
	for _, id := range []string{"sword_gale_fan", "warrior_piercing_lance"} {
		v, ok := testEndgameCatalog().Variant(id)
		if !ok || v.MaxTargets != 0 {
			t.Fatalf("%s retains target cap: %+v", id, v)
		}
	}
	trap, ok := testEndgameCatalog().Variant("assassin_bomb_fuse")
	if !ok || trap.Params.TrapMaxTargets != 0 || trap.MaxTargets != 1 {
		t.Fatal("trap victims unlimited; one selected trigger remains", trap)
	}
}
