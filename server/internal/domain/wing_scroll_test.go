package domain

import (
	"reflect"
	"testing"
)

func TestWingScrollSlotsOverwriteRollsAndPreserveUpgrade(t *testing.T) {
	ctx := wingCubeFixture(t)
	wing := ctx.Bag.At(200)
	wing.Legendary = LegendaryInstance{}
	wing.InstanceKind = ItemInstanceEquipment
	original := wing
	book := LegendaryPowerBook{"TY-A01": 8123, "TY-D01": 8234, "TY-U01": 8345, "TY-A02": 8456, "ZS-A01": 8567}
	for _, id := range []string{"TY-A01", "TY-D01", "TY-U01", "TY-A02", "ZS-A01"} {
		RegisterLegendaryPowerExecutable(id)
		before := wing
		got, err := book.ImprintWingPower(wing, ctx.Items[wing.Item], ctx.Character, id, ctx.WingRules)
		if err != nil {
			t.Fatal(id, err)
		}
		power, _ := LegendaryPower(id)
		idx, _ := WingPowerIndex(power)
		if got.WingPowers[idx].PowerID != id || got.WingPowers.Validate() != nil {
			t.Fatal("invalid imprint", got)
		}
		if LegendaryPowerHasRoll(id) && got.WingPowers[idx].PowerRollBP != book[id] {
			t.Fatal("lost roll")
		}
		for i := range got.WingPowers {
			if i != idx && got.WingPowers[i] != before.WingPowers[i] {
				t.Fatal("changed other category")
			}
		}
		copy := got
		copy.WingPowers = WingPowers{}
		if copy != original {
			t.Fatal("changed non-power state")
		}
		wing = got
	}
	ctx.Bag.Set(200, wing)
	for _, operation := range []string{"wing.upgrade", "wing.change"} {
		var recipe WingCubeRecipeDef
		for _, r := range ctx.WingRules.Recipes() {
			if r.Source == wing.Item && r.Operation == operation {
				recipe = r
				break
			}
		}
		if recipe.ID == 0 {
			t.Fatal("missing test edge", operation)
		}
		reqs, _, err := MatchCubeInputs(ctx, wingInputs(wing.UID, recipe.Materials...))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := PlanCube(ctx, reqs[0], true)
		if err != nil || plan.Target.WingPowers != wing.WingPowers {
			t.Fatal("upgrade/change lost powers", err)
		}
	}
	for _, mutate := range []func(*Stack){func(s *Stack) { s.Locked = true }, func(s *Stack) { s.Count = 2 }, func(s *Stack) { s.Item = 1001 }, func(s *Stack) { s.Legendary.PowerID = "TY-A01" }, func(s *Stack) { s.WingPowers[1].PowerRollBP = -1 }} {
		bad := wing
		mutate(&bad)
		got, err := book.ImprintWingPower(bad, ctx.Items[bad.Item], ctx.Character, "TY-A02", ctx.WingRules)
		if err == nil || got != bad {
			t.Fatal("invalid target changed")
		}
	}
	wrongClass := &Character{Race: Healer, EmploymentKnown: true, Employed: true}
	if _, err := book.ImprintWingPower(wing, ctx.Items[wing.Item], wrongClass, "ZS-A01", ctx.WingRules); CubeErrorCode(err) != "power_class" {
		t.Fatal("class gate lost", err)
	}
}

func TestRemovedWingRecipeDirectAndGridNeverConsumes(t *testing.T) {
	ctx := wingCubeFixture(t)
	donor, def := wingPowerDonor("TY-A01")
	donor.UID = 43
	def.InventoryTab, def.InventoryTabKnown, def.InstanceKind = 2, true, ItemInstanceEquipment
	ctx.Items[def.ID] = def
	ctx.Bag.Set(201, donor)
	before := ctx.Bag.Clone()
	req := EndgameRequest{RecipeID: WingPowerRecipeID, TargetUID: "42", ItemUIDs: []string{"43"}}
	for _, execute := range []bool{false, true} {
		if _, err := PlanCube(ctx, req, execute); CubeErrorCode(err) != "recipe_removed" {
			t.Fatal("direct legacy route open", err)
		}
	}
	for _, inputs := range [][]CubeInput{{{UID: "42"}, {UID: "43"}}, {{UID: "42"}}} {
		if _, _, err := MatchCubeInputs(ctx, EndgameRequest{CubeInputs: inputs}); err == nil {
			t.Fatal("wing salvaged")
		}
	}
	if !reflect.DeepEqual(before, ctx.Bag) {
		t.Fatal("failed request mutated bag")
	}
}
