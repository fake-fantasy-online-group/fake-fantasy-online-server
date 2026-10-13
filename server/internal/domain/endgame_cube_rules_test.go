package domain

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"math"
	"reflect"
	"strings"
	"testing"
)

func cubeRulesFixture(items map[ItemID]ItemDef) *CubeRuleTable {
	var rows []CubeRecipeDef
	if err := testfixture.LoadCubeRules(&rows); err != nil {
		panic(err)
	}
	rules, err := NewCubeRuleTable(rows, items)
	if err != nil {
		panic(err)
	}
	return rules
}
func TestCubeSQLRecipeViewsPreserveAllLegacyValues(t *testing.T) {
	ctx := cubeFixture()
	views := CubeRecipes(ctx.Rules, ctx.Items)
	if len(views) != 66 {
		t.Fatalf("recipe count %d", len(views))
	}
	for _, v := range views {
		old, ok := legacyCubeRecipe(v.ID, Stack{}, ctx.Items)
		if !ok || !reflect.DeepEqual(v, old) {
			t.Fatalf("recipe %d changed: got%+v want%+v", v.ID, v, old)
		}
	}
}
func TestCubeRuleTableIsImmutableAndMissingRulesFailClosed(t *testing.T) {
	ctx, req := cubeInputs(CubeOpenSocket)
	rows := ctx.Rules.Recipes()
	next, err := NewCubeRuleTable(rows, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := next.Recipe(CubeOpenSocket)
	for i := range rows {
		rows[i].Name = "changed"
		for j := range rows[i].Materials {
			rows[i].Materials[j].Count = 1
		}
	}
	got, _ := next.Recipe(CubeOpenSocket)
	if !reflect.DeepEqual(old, got) {
		t.Fatal("constructor retains mutable slices")
	}
	got.Materials[0].Count = 1
	again, _ := next.Recipe(CubeOpenSocket)
	if !reflect.DeepEqual(old, again) {
		t.Fatal("accessor leaks mutable slices")
	}
	ctx.Rules = nil
	if _, err := PlanCube(ctx, req, true); CubeErrorCode(err) != "configuration" {
		t.Fatalf("nil configuration accepted: %v", err)
	}
}
func TestCubeUsesInjectedFeeMaterialsYieldAndQualityStep(t *testing.T) {
	ctx, req := cubeInputs(CubeSalvageNormal)
	rows := ctx.Rules.Recipes()
	for i := range rows {
		if rows[i].ID == req.RecipeID {
			rows[i].Money = 123
			rows[i].Materials = []CubeMaterialDef{{CubePurifiedDust, 2}}
			rows[i].Outputs = []CubeOutputDef{{CubeStarMarrow, 7, 10, 40, 2}}
		}
	}
	var err error
	ctx.Rules, err = NewCubeRuleTable(rows, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Money != ctx.Money-123 || plan.Bag.CountOf(CubePurifiedDust) != 798 || plan.Bag.CountOf(CubeStarMarrow) != 822 {
		t.Fatal("database fee/material/yield ignored")
	}
	ctx, req = cubeInputs(CubeTemperUnique)
	rows = ctx.Rules.Recipes()
	for i := range rows {
		if rows[i].ID == req.RecipeID {
			rows[i].QualityStepBP = 700
		}
	}
	ctx.Rules, err = NewCubeRuleTable(rows, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Endgame.UniqueRollBP < 10700 {
		t.Fatal("configured quality step ignored")
	}
}
func TestCubeRuleValidationRejectsDanglingAndInvalidData(t *testing.T) {
	ctx := cubeFixture()
	base, _ := ctx.Rules.Recipe(CubeOpenSocket)
	for name, change := range map[string]func(*CubeRecipeDef){
		"unknown operation":  func(r *CubeRecipeDef) { r.Operation = "arbitrary.sql" },
		"missing item":       func(r *CubeRecipeDef) { r.Materials[0].ItemID = 1999999999 },
		"negative fee":       func(r *CubeRecipeDef) { r.Money = -1 },
		"bad target":         func(r *CubeRecipeDef) { r.TargetRequired = false },
		"zero cost count":    func(r *CubeRecipeDef) { r.Materials[0].Count = 0 },
		"duplicate material": func(r *CubeRecipeDef) { r.Materials = append(r.Materials, r.Materials[0]) },
		"unexpected output":  func(r *CubeRecipeDef) { r.Outputs = []CubeOutputDef{{CubeStarMarrow, 1, 0, 0, 1}} },
	} {
		t.Run(name, func(t *testing.T) {
			r := cloneCubeRecipe(base)
			change(&r)
			if _, err := NewCubeRuleTable([]CubeRecipeDef{r}, ctx.Items); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
	if _, err := (CubeOutputDef{Count: math.MaxInt32, Multiplier: 2}).Quantity(0); err == nil {
		t.Fatal("overflowing output accepted")
	}
}
func TestCubeSalvageSQLYieldsPreserveLegacyBounds(t *testing.T) {
	ctx := cubeFixture()
	for _, level := range []int32{-1, 0, 1, 4, 5, 50, 60, 100, 200} {
		for _, id := range []int32{CubeSalvageNormal, CubeSalvageUnique} {
			r, _ := ctx.Rules.Recipe(id)
			want := int32(8) + max(int32(0), min(int32(100), level))/5
			if id == CubeSalvageUnique {
				want *= 3
			}
			var actual int32
			for _, o := range r.Outputs {
				q, err := o.Quantity(level)
				if err != nil {
					t.Fatal(err)
				}
				if o.ItemID == CubeStarMarrow {
					actual = q
				}
				if o.ItemID == CubeImmortalEcho && q != 4 {
					t.Fatal("echo yield changed")
				}
			}
			if actual != want {
				t.Fatalf("salvage%d level%d: %d != %d", id, level, actual, want)
			}
		}
	}
}

func TestCubeRuleProtocolBounds(t *testing.T) {
	ctx := cubeFixture()
	rows := ctx.Rules.Recipes()
	r := rows[0]
	r.Name = strings.Repeat("😀", 4097)
	if _, err := NewCubeRuleTable([]CubeRecipeDef{r}, ctx.Items); err == nil {
		t.Fatal("UTF16 name over client limit accepted")
	}
	for i := range rows {
		rows[i].Description = strings.Repeat("长", 8192)
	}
	if _, err := NewCubeRuleTable(rows, ctx.Items); err == nil {
		t.Fatal("oversized catalog JSON page accepted")
	}
}

func TestCubeRecipeNameReservesCompletionReceiptSuffix(t *testing.T) {
	ctx := cubeFixture()
	r, _ := ctx.Rules.Recipe(CubeOpenSocket)
	r.Name = strings.Repeat("x", 8192)
	if _, err := NewCubeRuleTable([]CubeRecipeDef{r}, ctx.Items); err == nil {
		t.Fatal("name accepted but completed receipt would exceed client limit")
	}
}
