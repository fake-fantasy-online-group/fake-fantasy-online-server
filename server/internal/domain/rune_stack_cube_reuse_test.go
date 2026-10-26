package domain

import (
	"reflect"
	"testing"
)

// The matcher expands an explicitly selected source stack into per-unit internal
// sources. It never needs to select another UID or invent new public wire fields.
func TestCubeRuneConvertExpandedSingleSelectedStack(t *testing.T) {
	for _, count := range []int32{4, 5, 7} {
		ctx, req := cubeInputs(CubeConvertRuneBase + 1)
		recipe, _ := ctx.Rules.Recipe(req.RecipeID)
		recipe.SourceCount = 5
		var err error
		ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
		if err != nil {
			t.Fatal(err)
		}
		source := ctx.Bag.At(110)
		source.Count = count
		ctx.Bag.Set(110, source)
		// Deliberately leave other eligible source UIDs in the bag; none may be used.
		unselectedA, unselectedB := ctx.Bag.At(111), ctx.Bag.At(112)
		req.ItemUIDs = []string{"501", "501", "501", "501", "501"}
		before := ctx.Bag.Clone()
		p, err := PlanCube(ctx, req, true)
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("planner mutated input")
		}
		if count < 5 {
			if err == nil {
				t.Fatal("insufficient selected source accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(count, err)
		}
		if p.Bag.At(111) != unselectedA || p.Bag.At(112) != unselectedB {
			t.Fatal("consumed unselected source")
		}
		if count == 5 {
			if !p.Bag.At(110).Empty() || len(p.ConsumedUIDs) != 1 || p.ConsumedUIDs[0] != 501 {
				t.Fatal("depleted stack identity", p.ConsumedUIDs)
			}
		} else if p.Bag.At(110).Count != count-5 || p.Bag.At(110).UID != 501 || len(p.ConsumedUIDs) != 0 {
			t.Fatal("partial source consumption")
		}
	}
}

func repeatedRuneArrayContext(t *testing.T, count int32) (CubeContext, EndgameRequest) {
	t.Helper()
	ctx, req := cubeInputs(CubeInscribeArray)
	// Synthetic test-only formation exercises repeated positions, without changing
	// any shipped formation, order, effects, equipment requirement, or probability.
	catalog := *ctx.Catalog
	catalog.formations = map[int32][]AstralRecipe{}
	recipe := AstralRecipes(ctx.Catalog)[0]
	recipe.Runes = []ItemID{FirstAstralRune, FirstAstralRune}
	recipe.Slots = []EquipSlot{SlotWeapon}
	for version := range ctx.Catalog.formations {
		catalog.formations[version] = []AstralRecipe{recipe}
	}
	ctx.Catalog = &catalog
	d := ctx.Items[1001]
	d.EndgameCatalog = &catalog
	ctx.Items[1001] = d
	source := ctx.Bag.At(110)
	source.Count = count
	ctx.Bag.Set(110, source)
	req.ItemUIDs = []string{"501", "501"}
	ctx.NewUID = NewItemInstanceID
	return ctx, req
}
func TestCubeRuneArrayExpandedSingleSelectedStack(t *testing.T) {
	for _, count := range []int32{1, 2, 3} {
		ctx, req := repeatedRuneArrayContext(t, count)
		before := ctx.Bag.Clone()
		p, err := PlanCube(ctx, req, true)
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("planner mutated input")
		}
		if count < 2 {
			if err == nil {
				t.Fatal("insufficient stack accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(count, err)
		}
		if p.Target.SocketUIDs[0] <= 0 || p.Target.SocketUIDs[0] == p.Target.SocketUIDs[1] {
			t.Fatal("duplicate socket identity")
		}
		if p.Target.SocketCards[0] != before.At(110).Card || p.Target.SocketCards[1] != before.At(110).Card {
			t.Fatal("card snapshot changed")
		}
		if p.Bag.At(111) != before.At(111) {
			t.Fatal("consumed unselected UID")
		}
		if count == 2 {
			if !p.Bag.At(110).Empty() || p.Target.SocketUIDs[1] != 501 {
				t.Fatal("last unit did not transfer source UID")
			}
		} else if p.Bag.At(110).Count != 1 || p.Bag.At(110).UID != 501 || p.Target.SocketUIDs[1] == 501 {
			t.Fatal("partial array UID duplication")
		}
	}
}
