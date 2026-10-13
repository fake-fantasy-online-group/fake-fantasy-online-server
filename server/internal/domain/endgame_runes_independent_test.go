package domain

import "testing"

func TestAstralIndividualInsertNeverRequiresFormationEligibility(t *testing.T) {
	defs, lookup := astralFixture()
	recipe, _ := AstralRecipeByID(6, testEndgameCatalog())
	// Save the higher-level ingredient first, then insert the lower-level rune
	// into bag gear at its own level. The assembled item's wear gate remains the
	// highest rune requirement, but forming an inactive array cannot reject it.
	low, high := 0, 1
	a, _ := AstralRune(recipe.Runes[0], testEndgameCatalog())
	b, _ := AstralRune(recipe.Runes[1], testEndgameCatalog())
	if a.MinLevel > b.MinLevel {
		low, high = 1, 0
		a, b = b, a
	}
	if a.MinLevel >= b.MinLevel {
		t.Fatal("fixture needs two different rune levels")
	}
	gear := NewStack(defs[999001], 1)
	gear.SocketCount = 2
	bag := NewBag(3)
	bag.Set(0, gear)
	for i, id := range recipe.Runes {
		bag.Set(1+i, NewStack(defs[id], 1))
	}
	first, err := PlanAstralInsert(bag, NewEquipSet(), lookup, gear.UID, bag.At(1+high).UID, uint8(high), &Character{Level: b.MinLevel})
	if err != nil {
		t.Fatal(err)
	}
	last, err := PlanAstralInsert(first.Bag, first.Worn, lookup, gear.UID, first.Bag.At(1+low).UID, uint8(low), &Character{Level: a.MinLevel})
	if err != nil {
		t.Fatal("formation eligibility blocked a valid individual rune", err)
	}
	if got := EquipmentRequiredLevel(defs[gear.Item], last.Target); got != b.MinLevel {
		t.Fatal("lost max installed rune wear requirement", got)
	}
	worn := NewEquipSet()
	worn.Set(SlotWeapon, last.Target)
	if active := ActiveAstralRecipes(worn, lookup, &Character{Level: a.MinLevel, Race: Warrior}); len(active) != 0 {
		t.Fatal("inactive formation bypassed wear requirement")
	}
}
