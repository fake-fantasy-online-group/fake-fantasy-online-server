package domain

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

func TestAstralEveryFormationCraftingUsesMaxRuneLevel(t *testing.T) {
	for _, recipe := range AstralRecipes(testEndgameCatalog()) {
		t.Run(fmt.Sprintf("recipe%d", recipe.ID), func(t *testing.T) {
			maxRune := int32(0)
			for _, id := range recipe.Runes {
				rune, _ := AstralRune(id, testEndgameCatalog())
				maxRune = max(maxRune, rune.MinLevel)
			}
			for _, op := range []int32{CubeInscribeArray, CubeReorderRunes} {
				for _, baseLevel := range []int32{0, maxRune + 3} {
					required := max(baseLevel, maxRune)
					for _, execute := range []bool{false, true} {
						for _, level := range []int32{required - 1, required} {
							ctx := cubeFixture()
							ctx.Level = level
							def := ctx.Items[1001]
							def.Equip.Slot, def.Equip.Need.Level = int32(recipe.Slots[0]), baseLevel
							ctx.Items[def.ID] = def
							st := ctx.Bag.At(200)
							st.SocketCount = uint8(len(recipe.Runes))
							req := EndgameRequest{Op: "cube.quote", RecipeID: op, TargetUID: "42"}
							for i, id := range recipe.Runes {
								rune := cubeRune(ctx, id, int64(501+i))
								req.ItemUIDs = append(req.ItemUIDs, strconv.FormatInt(rune.UID, 10))
								if op == CubeInscribeArray {
									ctx.Bag.Set(110+i, rune)
								} else {
									j := len(recipe.Runes) - i - 1
									st.Sockets[j], st.SocketUIDs[j], st.SocketCards[j] = id, rune.UID, rune.Card
								}
							}
							ctx.Bag.Set(200, st)
							before := ctx.Bag.Clone()
							plan, err := PlanCube(ctx, req, execute)
							if (err == nil) != (level >= required) {
								t.Fatalf("op%d execute%v base%d level%d want%d: %v", op, execute, baseLevel, level, required, err)
							}
							if !reflect.DeepEqual(ctx.Bag, before) {
								t.Fatal("cube candidate mutated original input")
							}
							if err == nil {
								if got := EquipmentRequiredLevel(def, plan.Target); got != required {
									t.Fatalf("result requirement %d want %d", got, required)
								}
								matched, ok := MatchAstralRecipe(def, plan.Target)
								if !ok || matched.ID != recipe.ID {
									t.Fatal("crafted order did not match expected recipe")
								}
							}
						}
					}
				}
			}
		})
	}
}
