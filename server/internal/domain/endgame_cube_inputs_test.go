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
		"actual shortage": {func(c *CubeContext, r *EndgameRequest) { st := c.Bag.At(133); st.Count = 1; c.Bag.Set(133, st) }, "insufficient_count"},
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
		"extra material": func(c *CubeContext, r *EndgameRequest) {
			r.CubeInputs = append(r.CubeInputs, CubeInput{ItemID: CubeImmortalEcho, Count: 1})
		},
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
	matches, plans, err := MatchCubeInputs(ctx, r)
	if err != nil || len(matches) != 1 || len(plans) != 1 || plans[0].Materials[0].Count != 9 {
		t.Fatalf("server cost must override old declared count: plans=%+v err=%v", plans, err)
	}
	st := ctx.Bag.At(100)
	st.Count = 8
	ctx.Bag.Set(100, st)
	r.CubeInputs[0].Count = 9
	if _, _, err = MatchCubeInputs(ctx, r); CubeErrorCode(err) != "insufficient_count" {
		t.Fatalf("actual shortage accepted: %v", err)
	}
	st.Count = 800
	ctx.Bag.Set(100, st)
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
			matches, plans, err := MatchCubeInputs(ctx, inputs)
			if id == CubeTemperUnique {
				// Retired 1012 used 60/40/6 and 200000 money. Active 1011
				// uses the SAME selected types with 40/25/4 and 100000.
				// Counts are hints now: the sole active recipe must remain usable.
				current, _ := ctx.Rules.Recipe(CubeReforgeUnique)
				wantCosts := []CubeMaterialDef{{CubeStarMarrow, 40}, {CubePurifiedDust, 25}, {CubeImmortalEcho, 4}}
				if current.Money != 100000 || !reflect.DeepEqual(current.Materials, wantCosts) {
					t.Fatalf("unexpected active fixture costs: %+v", current)
				}
				if err != nil || len(matches) != 1 || len(plans) != 1 || matches[0].RecipeID != CubeReforgeUnique || plans[0].Recipe.Operation != "unique.reforge" {
					t.Fatalf("same types must select only active reforge: %+v %v", matches, err)
				}
				ctx.Intn = func(int) int { return 0 }
				executed, executeErr := PlanCube(ctx, matches[0], true)
				if executeErr != nil {
					t.Fatal(executeErr)
				}
				if executed.Money != ctx.Money-current.Money {
					t.Fatal("charged retired recipe fee")
				}
				for _, m := range current.Materials {
					if executed.Bag.UsableCountOf(m.ItemID) != ctx.Bag.UsableCountOf(m.ItemID)-m.Count {
						t.Fatalf("wrong active recipe consumption: %+v", m)
					}
				}
				oldTarget, target := ctx.Bag.At(200), executed.Bag.At(200)
				if target.UID != oldTarget.UID || target.Endgame.UniqueID != oldTarget.Endgame.UniqueID || target.RefineLevel != oldTarget.RefineLevel || target.Endgame.UniqueRollBP == oldTarget.Endgame.UniqueRollBP {
					t.Fatalf("active reforge failed to reroll while preserving identity: before=%+v after=%+v", oldTarget, target)
				}
			} else if err == nil || len(matches) != 0 {
				t.Fatalf("retired-only inputs matched: %+v %v", matches, err)
			}
			for _, match := range matches {
				if retired[match.RecipeID] {
					t.Fatalf("retired recipe leaked into generic matching: %d", match.RecipeID)
				}
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

// Count is a legacy hint for a uniquely determined recipe, including on UID
// selections. Actual inventory is the only quantity authority.
func TestCubeInputsIgnoreDeclaredCountsForUniqueRecipe(t *testing.T) {
	for _, count := range []int32{math.MinInt32, -1, 0, 1, 6, math.MaxInt32} {
		t.Run(strconv.Itoa(int(count)), func(t *testing.T) {
			ctx, req := cubeCapacityFixture(t, CubeInputCapacity)
			for i := range req.CubeInputs {
				req.CubeInputs[i].Count = count
			}
			before := ctx.Bag.Clone()
			matches, plans, err := MatchCubeInputs(ctx, req)
			if err != nil || len(matches) != 1 || len(plans) != 1 {
				t.Fatalf("hint %d rejected: %v", count, err)
			}
			plan, err := PlanCube(ctx, matches[0], true)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range plan.Materials {
				if got := plan.Bag.UsableCountOf(ItemID(m.ItemID)); got != 3 {
					t.Fatalf("surplus not preserved: material %d remaining %d", m.ItemID, got)
				}
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("planning mutated live bag")
			}
		})
	}
}

func TestCubeInputsDoNotSpendUnselectedOrLockedMaterials(t *testing.T) {
	for _, mode := range []string{"unselected", "locked surplus"} {
		t.Run(mode, func(t *testing.T) {
			ctx, req := cubeCapacityFixture(t, 2)
			if mode == "unselected" {
				req.CubeInputs = req.CubeInputs[1:]
			} else {
				st := ctx.Bag.At(100)
				st.Count = 1
				ctx.Bag.Set(100, st)
				st.Count, st.Locked = 100, true
				ctx.Bag.Set(101, st)
			}
			for i := range req.CubeInputs {
				req.CubeInputs[i].Count = math.MaxInt32
			}
			before := ctx.Bag.Clone()
			matches, plans, err := MatchCubeInputs(ctx, req)
			if err == nil || len(matches) != 0 || len(plans) != 0 {
				t.Fatalf("unauthorized materials accepted: %v", err)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("rejection mutated bag")
			}
		})
	}
}

func TestCubeInputsRandomPoolRequiresEqualMaterialCosts(t *testing.T) {
	for _, available := range []int32{3, 800} {
		for _, hint := range []int32{-1, 0, 1, 2, 4, math.MaxInt32} {
			t.Run(strconv.Itoa(int(available))+"/"+strconv.Itoa(int(hint)), func(t *testing.T) {
				ctx, legacy := cubeInputs(CubeInscribeRuneBase + 1)
				first, _ := ctx.Rules.Recipe(legacy.RecipeID)
				first.Materials = []CubeMaterialDef{{CubeStarMarrow, 2}}
				second := cloneCubeRecipe(first)
				second.ID++
				second.Outputs[0].ItemID++
				second.Materials[0].Count = 4
				var err error
				ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{first, second}, ctx.Items)
				if err != nil {
					t.Fatal(err)
				}
				st := ctx.Bag.At(100)
				st.Count = available
				ctx.Bag.Set(100, st)
				req := inputSelection(ctx, legacy)
				req.CubeInputs[0].Count = hint
				before := ctx.Bag.Clone()
				ctx.Intn = func(int) int { t.Fatal("matching rolled pool RNG"); return 0 }
				matches, _, err := MatchCubeInputs(ctx, req)
				switch {
				case hint == 2:
					if err != nil || len(matches) != 1 || matches[0].RecipeID != first.ID {
						t.Fatalf("exact cheap intent lost: %+v %v", matches, err)
					}
				case hint == 4 && available >= 4:
					if err != nil || len(matches) != 1 || matches[0].RecipeID != second.ID {
						t.Fatalf("exact expensive intent lost: %+v %v", matches, err)
					}
				case hint == 4:
					if CubeErrorCode(err) != "insufficient_count" {
						t.Fatalf("intent must not fall back to affordable row: %v", err)
					}
				default:
					if CubeErrorCode(err) != "configuration" || len(matches) != 0 {
						t.Fatalf("different-cost pool chose by wealth/hint: %+v %v", matches, err)
					}
				}
				if !reflect.DeepEqual(before, ctx.Bag) {
					t.Fatal("pool matching mutated bag")
				}
			})
		}
	}
}

func TestCubeInputsRuneQuantityUsesOnlySelectedUIDInventory(t *testing.T) {
	for _, available := range []int32{1, 3, 5} {
		for _, locked := range []bool{false, true} {
			t.Run(strconv.Itoa(int(available))+"/locked="+strconv.FormatBool(locked), func(t *testing.T) {
				ctx, legacy := cubeInputs(CubeConvertRuneBase + 1)
				recipe, _ := ctx.Rules.Recipe(legacy.RecipeID)
				var err error
				ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{recipe}, ctx.Items)
				if err != nil {
					t.Fatal(err)
				}
				st := ctx.Bag.At(110)
				st.Count, st.Card.Locked = available, locked
				ctx.Bag.Set(110, st)
				// Unselected stacks are deliberately plentiful and compatible.
				spare := ctx.Bag.At(111)
				spare.Count = 100
				ctx.Bag.Set(111, spare)
				req := inputSelection(ctx, legacy)
				req.CubeInputs = req.CubeInputs[:len(recipe.Materials)+1]
				for i := range req.CubeInputs {
					req.CubeInputs[i].Count = 0
				}
				before := ctx.Bag.Clone()
				matches, _, err := MatchCubeInputs(ctx, req)
				if locked || available < recipe.SourceCount {
					if err == nil || len(matches) != 0 {
						t.Fatalf("spent locked or unselected UID: %+v %v", matches, err)
					}
				} else {
					if err != nil || len(matches) != 1 {
						t.Fatalf("selected stack quantity ignored: %v", err)
					}
					plan, err := PlanCube(ctx, matches[0], true)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(plan.Bag.At(111), spare) || !reflect.DeepEqual(plan.Bag.At(112), ctx.Bag.At(112)) {
						t.Fatal("consumed unselected rune UID")
					}
					remaining := int32(0)
					plan.Bag.Each(func(_ int, s Stack) {
						if s.UID == 501 {
							remaining += s.Count
						}
					})
					if remaining != available-recipe.SourceCount {
						t.Fatalf("selected UID remainder %d want %d", remaining, available-recipe.SourceCount)
					}
				}
				if !reflect.DeepEqual(before, ctx.Bag) {
					t.Fatal("planning mutated live inventory")
				}
			})
		}
	}
}

func TestCubeInputsRandomPoolSourceCostRequiresExactIntent(t *testing.T) {
	ctx, legacy := cubeInputs(CubeConvertRuneBase + 1)
	first, _ := ctx.Rules.Recipe(legacy.RecipeID)
	first.SourceCount = 2
	second := cloneCubeRecipe(first)
	second.ID++
	second.SourceCount = 3
	var err error
	ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{first, second}, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{2, 3} {
		req := inputSelection(ctx, legacy)
		req.CubeInputs = req.CubeInputs[:len(first.Materials)+count]
		before := ctx.Bag.Clone()
		matches, _, err := MatchCubeInputs(ctx, req)
		want := first.ID
		if count == 3 {
			want = second.ID
		}
		if err != nil || len(matches) != 1 || matches[0].RecipeID != want {
			t.Fatalf("source cost %d intent lost or randomized: %+v %v", count, matches, err)
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("ambiguous matching mutated inventory")
		}
	}
}
