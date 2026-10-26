package domain

import (
	"reflect"
	"testing"
)

func TestCubeMismatchClassesAndReservationSafety(t *testing.T) {
	for _, tc := range []struct {
		name, code, message string
		alter               func(*CubeContext, *EndgameRequest)
	}{
		{"short", "insufficient_count", "材料虽合，数目尚缺。", func(c *CubeContext, r *EndgameRequest) { cubeShortenSelectedMaterial(c, r, 0) }},
		{"missing type", "partial_match", "部分材料相合，尚需调配。", func(c *CubeContext, r *EndgameRequest) { r.CubeInputs = r.CubeInputs[1:] }},
		{"extra type", "partial_match", "部分材料相合，尚需调配。", func(c *CubeContext, r *EndgameRequest) {
			r.CubeInputs = append(r.CubeInputs, CubeInput{ItemID: CubeImmortalEcho, Count: 1})
		}},
		{"short plus surplus", "insufficient_count", "材料虽合，数目尚缺。", func(c *CubeContext, r *EndgameRequest) {
			cubeShortenSelectedMaterial(c, r, 0)
			st := c.Bag.At(101)
			st.Count += 20
			c.Bag.Set(101, st)
		}},
		{"wrong", "no_match", "鼎中诸物，未合炼方。", func(c *CubeContext, r *EndgameRequest) {
			r.CubeInputs = []CubeInput{{ItemID: CubeImmortalEcho, Count: 1}}
		}},
		{"wrong equipment plus short", "partial_match", "部分材料相合，尚需调配。", func(c *CubeContext, r *EndgameRequest) { cubeShortenSelectedMaterial(c, r, 0); cubeMakeUnique(c) }},
		{"missing equipment", "partial_match", "部分材料相合，尚需调配。", func(c *CubeContext, r *EndgameRequest) { r.CubeInputs = r.CubeInputs[:2] }},
		{"invalid mixed uid", "invalid_request", "", func(c *CubeContext, r *EndgameRequest) {
			cubeShortenSelectedMaterial(c, r, 0)
			r.CubeInputs[2].UID = "042"
		}},
		{"locked mixed uid", "locked", "", func(c *CubeContext, r *EndgameRequest) {
			cubeShortenSelectedMaterial(c, r, 0)
			st := c.Bag.At(200)
			st.Locked = true
			c.Bag.Set(200, st)
		}},
		{"missing ownership", "not_owner", "", func(c *CubeContext, r *EndgameRequest) { cubeShortenSelectedMaterial(c, r, 0); c.Bag.Set(200, Stack{}) }},
		{"unavailable quantity", "materials", "", func(c *CubeContext, r *EndgameRequest) { c.Bag.Set(100, Stack{}) }},
		{"invalid parameters", "invalid_request", "", func(c *CubeContext, r *EndgameRequest) { cubeShortenSelectedMaterial(c, r, 0); r.Tier = 1 }},
		{"funds", "funds", "", func(c *CubeContext, r *EndgameRequest) { c.Money = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, legacy := cubeInputs(CubeAwakenUnique)
			recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
			var err error
			ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
			if err != nil {
				t.Fatal(err)
			}
			req := inputSelection(ctx, legacy)
			tc.alter(&ctx, &req)
			before, money := ctx.Bag.Clone(), ctx.Money
			ctx.Intn = func(int) int { t.Fatal("diagnosis used RNG"); return 0 }
			ctx.NewUID = func() int64 { t.Fatal("diagnosis allocated UID"); return 0 }
			requests, plans, err := MatchCubeInputs(ctx, req)
			if err == nil || CubeErrorCode(err) != tc.code || len(requests) != 0 || len(plans) != 0 {
				t.Fatalf("got %v (%s), want %s", err, CubeErrorCode(err), tc.code)
			}
			if tc.message != "" && err.Error() != tc.message {
				t.Fatalf("unexpected disclosure/message: %v", err)
			}
			if !reflect.DeepEqual(before, ctx.Bag) || ctx.Money != money {
				t.Fatal("failure consumed inputs")
			}
		})
	}
}

func TestCubeMismatchOrderIndependentPrecedence(t *testing.T) {
	ctx, legacy := cubeInputs(CubeAwakenUnique)
	req := inputSelection(ctx, legacy)
	cubeShortenSelectedMaterial(&ctx, &req, 0)
	recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
	other := cloneCubeRecipe(recipe)
	other.ID += 10000
	other.Materials = other.Materials[:1]
	for _, rows := range [][]CubeRecipeDef{{recipe, other}, {other, recipe}} {
		var err error
		ctx.Rules, err = NewCubeRuleTable(rows, ctx.Items)
		if err != nil {
			t.Fatal(err)
		}
		// Also reverse internal iteration to prove ranking is not an ID-order side effect.
		for i := 0; i < 2; i++ {
			_, _, err = MatchCubeInputs(ctx, req)
			if CubeErrorCode(err) != "insufficient_count" {
				t.Fatalf("candidate precedence: %v", err)
			}
			ctx.Rules.ids[0], ctx.Rules.ids[1] = ctx.Rules.ids[1], ctx.Rules.ids[0]
		}
	}
}

func TestCubeConversionShortageRespectsRuneTypes(t *testing.T) {
	ctx, legacy := cubeInputs(CubeConvertRuneBase + 1)
	recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
	var err error
	ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	req := inputSelection(ctx, legacy)
	req.CubeInputs = req.CubeInputs[:len(req.CubeInputs)-1]
	_, _, err = MatchCubeInputs(ctx, req)
	if CubeErrorCode(err) != "insufficient_count" {
		t.Fatalf("short identical rune set: %v", err)
	}
	ctx.Bag.Set(111, cubeRune(ctx, FirstAstralRune+7, 502))
	_, _, err = MatchCubeInputs(ctx, req)
	if CubeErrorCode(err) == "insufficient_count" {
		t.Fatal("mixed rune types reported as count shortage")
	}
}

func TestCubeEveryRecipeMaterialShortage(t *testing.T) {
	for _, recipe := range cubeFixture().Rules.Recipes() {
		if len(recipe.Materials) == 0 || recipe.Materials[0].Count < 2 {
			continue
		}
		ctx, legacy := cubeInputs(recipe.ID)
		var err error
		ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
		if err != nil {
			t.Fatal(err)
		}
		req := inputSelection(ctx, legacy)
		cubeShortenSelectedMaterial(&ctx, &req, 0)
		before := ctx.Bag.Clone()
		_, _, err = MatchCubeInputs(ctx, req)
		if CubeErrorCode(err) != "insufficient_count" {
			t.Errorf("recipe %d: got %v (%s)", recipe.ID, err, CubeErrorCode(err))
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatalf("recipe %d mutated inventory", recipe.ID)
		}
	}
}

func TestCubePartialRuneOverlapDoesNotDependOnOrder(t *testing.T) {
	ctx, legacy := cubeInputs(CubeConvertRuneBase + 1)
	recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
	recipe.Materials = nil
	var err error
	ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Bag.Set(111, cubeRune(ctx, FirstAstralRune+7, 502))
	for _, runes := range [][]string{{"501", "502"}, {"502", "501"}} {
		err := cubeInputMismatch(ctx, EndgameRequest{}, map[ItemID]int32{}, "", runes)
		if CubeErrorCode(err) != "partial_match" {
			t.Fatalf("mixed sources: %v", err)
		}
	}
}

// A count shortage is a shortage of owned inventory, not a UI count hint.
func cubeShortenSelectedMaterial(c *CubeContext, r *EndgameRequest, index int) {
	selected := r.CubeInputs[index]
	required := selected.Count
	c.Bag.Each(func(slot int, st Stack) {
		if st.Item == selected.ItemID {
			st.Count = required - 1
			c.Bag.Set(slot, st)
		}
	})
}
