package domain

import (
	"math"
	"reflect"
	"strconv"
	"testing"
)

// Use synthetic test-only material requirements so the boundary exercises a
// successful match without expanding or changing any shipped recipe.
func cubeCapacityFixture(t *testing.T, count int) (CubeContext, EndgameRequest) {
	t.Helper()
	ctx, legacy := cubeInputs(CubeOpenSocket)
	recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
	recipe.Materials = nil
	for i := 0; i < count-1; i++ {
		id := ItemID(1999990000 + i)
		ctx.Items[id] = ItemDef{ID: id, Name: "容量测试材料", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
		ctx.Bag.Set(100+i, Stack{Item: id, Count: 5})
		recipe.Materials = append(recipe.Materials, CubeMaterialDef{ItemID: id, Count: 2})
	}
	var err error
	ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, inputSelection(ctx, legacy)
}

func TestCubeInputCapacityPreservesMatchingAndAtomicity(t *testing.T) {
	if CubeInputCapacity != 35 {
		t.Fatalf("native 5-by-7 grid capacity = %d, want 35", CubeInputCapacity)
	}
	for _, count := range []int{1, 12, 13, 35, 36} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			ctx, req := cubeCapacityFixture(t, count)
			before, money := ctx.Bag.Clone(), ctx.Money
			ctx.Intn = func(int) int { t.Fatal("preview rolled RNG"); return 0 }
			ctx.NewUID = func() int64 { t.Fatal("preview allocated UID"); return 0 }
			matches, plans, err := MatchCubeInputs(ctx, req)
			if count > CubeInputCapacity {
				if err == nil || CubeErrorCode(err) != "invalid_request" || len(matches) != 0 || len(plans) != 0 {
					t.Fatalf("oversize selection did not fail closed: %v", err)
				}
			} else {
				if err != nil || len(matches) != 1 || len(plans) != 1 || matches[0].RecipeID != CubeOpenSocket || matches[0].TargetUID != "42" || len(plans[0].Materials) != count-1 {
					t.Fatalf("valid %d-slot match failed: requests=%+v err=%v", count, matches, err)
				}
				for i, j := 0, len(req.CubeInputs)-1; i < j; i, j = i+1, j-1 {
					req.CubeInputs[i], req.CubeInputs[j] = req.CubeInputs[j], req.CubeInputs[i]
				}
				reordered, _, err := MatchCubeInputs(ctx, req)
				if err != nil || !reflect.DeepEqual(matches, reordered) {
					t.Fatalf("grid order changed recipe match: %v", err)
				}
			}
			if !reflect.DeepEqual(before, ctx.Bag) || ctx.Money != money {
				t.Fatal("matching mutated live inventory or money")
			}
		})
	}
}

func TestCubeFullGridRejectsInvalidSelectionsWithoutConsumption(t *testing.T) {
	cases := map[string]struct {
		alter func(*CubeContext, *EndgameRequest)
		code  string
	}{
		"duplicate material": {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33] = r.CubeInputs[0] }, "invalid_request"},
		"duplicate instance": {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33] = r.CubeInputs[34] }, "invalid_request"},
		"moved out of bag":   {func(c *CubeContext, r *EndgameRequest) { c.Bag.Set(200, Stack{}) }, "not_owner"},
		"locked instance": {func(c *CubeContext, r *EndgameRequest) {
			st := c.Bag.At(200)
			st.Locked = true
			c.Bag.Set(200, st)
		}, "locked"},
		"locked material": {func(c *CubeContext, r *EndgameRequest) {
			st := c.Bag.At(133)
			st.Locked = true
			c.Bag.Set(133, st)
		}, "materials"},
		"unavailable quantity": {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33].Count = 6 }, "materials"},
		"maximum quantity":     {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33].Count = math.MaxInt32 }, "materials"},
		"wrong quantity":       {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33].Count = 1 }, "no_match"},
		"zero quantity":        {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33].Count = 0 }, "invalid_request"},
		"negative quantity":    {func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[33].Count = -1 }, "invalid_request"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, req := cubeCapacityFixture(t, CubeInputCapacity)
			tc.alter(&ctx, &req)
			before, money := ctx.Bag.Clone(), ctx.Money
			matches, plans, err := MatchCubeInputs(ctx, req)
			if err == nil || CubeErrorCode(err) != tc.code || len(matches) != 0 || len(plans) != 0 {
				t.Fatalf("invalid selection: error=%v, want %s", err, tc.code)
			}
			if !reflect.DeepEqual(before, ctx.Bag) || ctx.Money != money {
				t.Fatal("invalid full grid consumed inventory or money")
			}
		})
	}
}

func inputSelection(ctx CubeContext, legacy EndgameRequest) EndgameRequest {
	r := EndgameRequest{Op: "cube.quote", ExpectedRevision: "0"}
	recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
	for _, m := range recipe.Materials {
		r.CubeInputs = append(r.CubeInputs, CubeInput{ItemID: m.ItemID, Count: m.Count})
	}
	if legacy.TargetUID != "" {
		r.CubeInputs = append(r.CubeInputs, CubeInput{UID: legacy.TargetUID, Count: 1})
	}
	if legacy.RuneUID != "" {
		r.CubeInputs = append(r.CubeInputs, CubeInput{UID: legacy.RuneUID, Count: 1})
	}
	if recipe.Operation == "socket.reorder" {
		r.ItemUIDs = append([]string(nil), legacy.ItemUIDs...)
	} else {
		for _, uid := range legacy.ItemUIDs {
			r.CubeInputs = append(r.CubeInputs, CubeInput{UID: uid, Count: 1})
		}
	}
	return r
}
func TestCubeInputsAllDatabaseRecipesMatchWithoutDisclosureOrMutation(t *testing.T) {
	for _, r := range cubeFixture().Rules.Recipes() {
		t.Run(strconv.Itoa(int(r.ID)), func(t *testing.T) {
			ctx, legacy := cubeInputs(r.ID)
			selection := inputSelection(ctx, legacy)
			before := ctx.Bag.Clone()
			money := ctx.Money
			ctx.Intn = func(int) int { t.Fatal("preview rolled RNG"); return 0 }
			ctx.NewUID = func() int64 { t.Fatal("preview allocated UID"); return 0 }
			matches, plans, err := MatchCubeInputs(ctx, selection)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for i, req := range matches {
				if req.RecipeID == r.ID {
					found = true
				}
				if plans[i].Recipe.Operation != r.Operation {
					t.Fatal("cross-operation match")
				}
			}
			if !found {
				t.Fatalf("lost existing recipe %d", r.ID)
			}
			want := 1
			if r.Operation == "rune.inscribe" {
				want = 28
			}
			if r.Operation == "rune.convert" {
				want = 6
			}
			if len(matches) != want {
				t.Fatalf("got %d candidates want %d", len(matches), want)
			}
			if !reflect.DeepEqual(ctx.Bag, before) || ctx.Money != money {
				t.Fatal("selection moved or consumed items")
			}
		})
	}
}
func TestCubeInputsRejectInvalidSelectionsWithoutConsumption(t *testing.T) {
	cases := map[string]func(*CubeContext, *EndgameRequest){
		"empty": func(c *CubeContext, r *EndgameRequest) { r.CubeInputs = nil },
		"duplicate instance": func(c *CubeContext, r *EndgameRequest) {
			r.CubeInputs = append(r.CubeInputs, CubeInput{UID: "42", Count: 1})
		},
		"duplicate material": func(c *CubeContext, r *EndgameRequest) { r.CubeInputs = append(r.CubeInputs, r.CubeInputs[0]) },
		"wrong quantity":     func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[0].Count++ },
		"extra material": func(c *CubeContext, r *EndgameRequest) {
			r.CubeInputs = append(r.CubeInputs, CubeInput{ItemID: CubeImmortalEcho, Count: 1})
		},
		"negative":             func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[0].Count = -1 },
		"not owned":            func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[2].UID = "43" },
		"quantity unavailable": func(c *CubeContext, r *EndgameRequest) { c.Bag.Set(100, Stack{}) },
		"locked":               func(c *CubeContext, r *EndgameRequest) { st := c.Bag.At(200); st.Locked = true; c.Bag.Set(200, st) },
		"mixed identity":       func(c *CubeContext, r *EndgameRequest) { r.CubeInputs[2].ItemID = 1001 },
		"recipe override":      func(c *CubeContext, r *EndgameRequest) { r.RecipeID = CubeAwakenUnique },
		"target override":      func(c *CubeContext, r *EndgameRequest) { r.TargetUID = "42" },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, legacy := cubeInputs(CubeAwakenUnique)
			r := inputSelection(ctx, legacy)
			alter(&ctx, &r)
			before := ctx.Bag.Clone()
			if _, _, err := MatchCubeInputs(ctx, r); err == nil {
				t.Fatal("accepted invalid input")
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("invalid input consumed bag")
			}
		})
	}
}
func TestCubeInputsHonorDatabaseCostsAndRejectCrossOperationAmbiguity(t *testing.T) {
	ctx, legacy := cubeInputs(CubeOpenSocket)
	r := inputSelection(ctx, legacy)
	defs := ctx.Rules.Recipes()
	for i := range defs {
		if defs[i].ID == CubeOpenSocket {
			defs[i].Materials[0].Count = 9
		}
	}
	rules, err := NewCubeRuleTable(defs, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Rules = rules
	if _, _, err = MatchCubeInputs(ctx, r); err == nil {
		t.Fatal("hardcoded old count accepted")
	}
	r.CubeInputs[0].Count = 9
	if _, _, err = MatchCubeInputs(ctx, r); err != nil {
		t.Fatal(err)
	}
	for i := range defs {
		if defs[i].ID == CubeAwakenUnique {
			defs[i].Materials = []CubeMaterialDef{{CubeStarMarrow, 9}}
		}
	}
	rules, err = NewCubeRuleTable(defs, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Rules = rules
	if _, _, err = MatchCubeInputs(ctx, r); CubeErrorCode(err) != "configuration" {
		t.Fatalf("ambiguous operations did not fail closed: %v", err)
	}
}
func TestCubeInputsBagFullAndDuplicateBagIdentityRemainAtomic(t *testing.T) {
	ctx, legacy := cubeInputs(CubeInscribeRuneBase + 1)
	r := inputSelection(ctx, legacy)
	for slot := 100; slot < 200; slot++ {
		if ctx.Bag.At(slot).Empty() {
			ctx.Bag.Set(slot, cubeRune(ctx, FirstAstralRune, int64(1000+slot)))
		}
	}
	before := ctx.Bag.Clone()
	if _, _, err := MatchCubeInputs(ctx, r); CubeErrorCode(err) != "bag_full" {
		t.Fatalf("full bag accepted: %v", err)
	}
	if !reflect.DeepEqual(before, ctx.Bag) {
		t.Fatal("full bag changed")
	}
	ctx, legacy = cubeInputs(CubeOpenSocket)
	r = inputSelection(ctx, legacy)
	ctx.Bag.Set(201, ctx.Bag.At(200))
	if _, _, err := MatchCubeInputs(ctx, r); err == nil {
		t.Fatal("duplicate bag UID accepted")
	}
}

func TestCubeSnapshotMaterialBoundsFailAtStartup(t *testing.T) {
	ctx := cubeFixture()
	// Cube-only fixtures do not have the native rift key definition.
	ctx.Items[RiftKeyItemID] = ItemDef{ID: RiftKeyItemID, Name: "符钥"}
	if err := ValidateCubeSnapshotMaterials(ctx.Items, ctx.Catalog, ctx.Rules); err != nil {
		t.Fatal(err)
	}
	defs := ctx.Rules.Recipes()
	for i := 0; i < 33; i++ {
		id := ItemID(1999990000 + i)
		ctx.Items[id] = ItemDef{ID: id, Name: "额外材料", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
		defs[0].Materials = append(defs[0].Materials, CubeMaterialDef{ItemID: id, Count: 1})
	}
	rules, err := NewCubeRuleTable(defs, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	if len(CubeSnapshotMaterialIDs(rules, ctx.Catalog)) != 65 {
		t.Fatal("fixture did not exceed wire bound")
	}
	if err := ValidateCubeSnapshotMaterials(ctx.Items, ctx.Catalog, rules); err == nil {
		t.Fatal("65 material entries reached runtime instead of failing startup")
	}
}

// Historical mechanics remain testable, but a disabled DB identity is absent
// from the runtime snapshot shared by both native inputs and agent requests.
func TestCubeRetiredRecipesRejectInputsAndLegacyIDs(t *testing.T) {
	retired := map[int32]bool{1007: true, 1008: true, 1009: true, 1012: true}
	for id := range retired {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			ctx, legacy := cubeInputs(id)
			inputs := inputSelection(ctx, legacy)
			var active []CubeRecipeDef
			for _, r := range ctx.Rules.Recipes() {
				if !retired[r.ID] {
					active = append(active, r)
				}
			}
			var err error
			ctx.Rules, err = NewCubeRuleTable(active, ctx.Items)
			if err != nil {
				t.Fatal(err)
			}
			before := ctx.Bag.Clone()
			ctx.Intn = func(int) int { t.Fatal("disabled recipe rolled RNG"); return 0 }
			ctx.NewUID = func() int64 { t.Fatal("disabled recipe allocated UID"); return 0 }
			for _, execute := range []bool{false, true} {
				if _, err = PlanCube(ctx, legacy, execute); CubeErrorCode(err) != "not_found" {
					t.Fatalf("legacy accepted: %v", err)
				}
			}
			matches, _, err := MatchCubeInputs(ctx, inputs)
			if err == nil || len(matches) != 0 {
				t.Fatalf("retired inputs matched: %+v %v", matches, err)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("retired request mutated saved equipment")
			}
		})
	}
}

func TestCubeRemaining62RecipesStillMatchAndExecute(t *testing.T) {
	retired := map[int32]bool{1007: true, 1008: true, 1009: true, 1012: true}
	count := 0
	for _, recipe := range cubeFixture().Rules.Recipes() {
		if retired[recipe.ID] {
			continue
		}
		count++
		t.Run(strconv.Itoa(int(recipe.ID)), func(t *testing.T) {
			ctx, legacy := cubeInputs(recipe.ID)
			var active []CubeRecipeDef
			for _, r := range ctx.Rules.Recipes() {
				if !retired[r.ID] {
					active = append(active, r)
				}
			}
			var err error
			ctx.Rules, err = NewCubeRuleTable(active, ctx.Items)
			if err != nil {
				t.Fatal(err)
			}
			matches, _, err := MatchCubeInputs(ctx, inputSelection(ctx, legacy))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, m := range matches {
				if m.RecipeID == recipe.ID {
					found = true
				}
			}
			if !found {
				t.Fatal("remaining recipe not matched")
			}
			if _, err := PlanCube(ctx, legacy, true); err != nil {
				t.Fatal(err)
			}
		})
	}
	if count != 62 {
		t.Fatalf("remaining count=%d", count)
	}
}
