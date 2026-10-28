package domain

import "testing"

func TestSpecialWeaponRegistryEveryOriginalSourceAndBranch(t *testing.T) {
	branches := SpecialWeaponEffectBranches()
	if len(branches) != 56 {
		t.Fatalf("branches=%d", len(branches))
	}
	seen := map[ItemID]bool{}
	for _, branch := range branches {
		seen[branch.BaseID] = true
		effect, ok := RuntimeSpecialWeaponEffect(branch.BaseID, branch.Profession, "skill-definition", false)
		if !ok || !effect.Registered() || effect.Parameters.SpecialWeaponBase != branch.BaseID || effect.Producer {
			t.Fatalf("branch %+v: %+v", branch, effect)
		}
		for _, skill := range branch.Skills {
			if !SpecialWeaponSkillMatches(branch.BaseID, skill, branch.Profession) || SpecialWeaponSkillMatches(branch.BaseID, skill, 99) {
				t.Fatalf("skill gate %+v/%d", branch, skill)
			}
		}
		if SpecialWeaponSkillMatches(branch.BaseID, 999999, branch.Profession) {
			t.Fatal("invented skill")
		}
		branch.Skills[0] = -1
		if SpecialWeaponEffectBranches()[0].Skills[0] == -1 {
			t.Fatal("exported mutable catalog")
		}
	}
	if len(seen) != 48 {
		t.Fatalf("weapons=%d", len(seen))
	}
	enabled, inert := 0, 0
	for _, weapon := range dungeonEquipmentManifest.Weapons {
		for _, id := range []ItemID{weapon.BaseID, HardDungeonItemOffset + weapon.BaseID} {
			def := ItemDef{ID: id, Equip: &EquipDef{Durable: 100}}
			stack := Stack{Item: id, UID: int64(id), Durability: 100, MaxDurability: 100}
			effect, ok := SpecialWeaponSourceCandidate(stack, def)
			want := EnhancedWeaponBaseID(id)
			if ok != (want != 0) {
				t.Fatalf("identity %d got %v want %d", id, ok, want)
			}
			if ok {
				enabled++
				if effect.Item != id || effect.SourceUID != stack.UID || effect.Parameters.SpecialWeaponBase != want {
					t.Fatalf("lost source: %+v", effect)
				}
			} else {
				inert++
			}
			stack.Durability = 0
			if _, ok := SpecialWeaponSourceCandidate(stack, def); ok {
				t.Fatalf("broken %d admitted", id)
			}
		}
	}
	if enabled != 48 || inert != 72 {
		t.Fatalf("enabled=%d inert=%d", enabled, inert)
	}
	if _, ok := SpecialWeaponSourceCandidate(Stack{Item: 1021}, ItemDef{}); ok {
		t.Fatal("non-equipment admitted")
	}
	if _, ok := RuntimeSpecialWeaponEffect(0, 0, "heal", true); ok {
		t.Fatal("unknown base admitted")
	}
	if _, ok := RuntimeSpecialWeaponEffect(1021, 99, "heal", true); ok {
		t.Fatal("unknown profession admitted")
	}
}

func TestSpecialWeaponRegistryMissingDefinitionFailsClosed(t *testing.T) {
	id := specialWeaponDefinitionID(1021, 2)
	equipmentEffectRegistry.Lock()
	saved := equipmentEffectRegistry.defs[id]
	delete(equipmentEffectRegistry.defs, id)
	equipmentEffectRegistry.Unlock()
	defer func() {
		equipmentEffectRegistry.Lock()
		equipmentEffectRegistry.defs[id] = saved
		equipmentEffectRegistry.Unlock()
	}()
	if _, ok := RuntimeSpecialWeaponEffect(1021, 2, "heal", true); ok {
		t.Fatal("unregistered source admitted")
	}
}
