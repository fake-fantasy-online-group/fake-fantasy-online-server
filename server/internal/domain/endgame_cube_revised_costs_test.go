package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// Invoked by tests/cube_costs_postgres.mjs using rows produced by actual SQL.
func TestCubeRevisedCostsFromDatabase(t *testing.T) {
	file := os.Getenv("CUBE_REVISED_RULES")
	if file == "" {
		t.Skip("run node tests/cube_costs_postgres.mjs for database-backed migration coverage")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var rows []CubeRecipeDef
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 62 {
		t.Fatalf("active recipes = %d", len(rows))
	}
	for _, recipe := range rows {
		t.Run(strconv.Itoa(int(recipe.ID)), func(t *testing.T) {
			ctx, req := cubeInputs(recipe.ID)
			ctx.Rules, err = NewCubeRuleTable(rows, ctx.Items)
			if err != nil {
				t.Fatal(err)
			}
			// Zero-fee operations must work with no coins. Paid operations consume
			// exactly their entire fee, and one coin less must fail without mutation.
			ctx.Money = recipe.Money
			before := ctx.Bag.Clone()
			input := inputSelection(ctx, req)
			matches, plans, err := MatchCubeInputs(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if recipe.Operation == "rune.inscribe" {
				want = 28
			}
			if recipe.Operation == "rune.convert" {
				want = 6
			}
			if len(matches) != want || len(plans) != want {
				t.Fatalf("pool = %d/%d want %d", len(matches), len(plans), want)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("preview mutated inventory")
			}
			for i := range recipe.Materials {
				wrong := input
				wrong.CubeInputs = append([]CubeInput(nil), input.CubeInputs...)
				wrong.CubeInputs[i].Count++
				if _, _, err := MatchCubeInputs(ctx, wrong); err == nil {
					t.Fatal("incorrect material quantity matched")
				}
			}

			p, err := PlanCube(ctx, req, true)
			if err != nil {
				t.Fatal(err)
			}
			if p.Money != 0 {
				t.Fatalf("remaining money %d", p.Money)
			}
			for _, m := range recipe.Materials {
				if p.Bag.UsableCountOf(m.ItemID) != ctx.Bag.UsableCountOf(m.ItemID)-m.Count {
					t.Fatalf("wrong material deduction %+v", m)
				}
			}
			if recipe.Money > 0 {
				ctx.Money--
				if _, err := PlanCube(ctx, req, true); CubeErrorCode(err) != "funds" {
					t.Fatalf("underfunded error %v", err)
				}
				ctx.Money++
			}
			if recipe.Operation == "rune.convert" {
				if len(p.Materials) != 0 || len(p.ConsumedUIDs) != 3 || len(p.Outputs) != 1 {
					t.Fatalf("conversion count mismatch %+v", p)
				}
				for _, id := range []ItemID{CubeStarMarrow, CubePurifiedDust, CubeImmortalEcho} {
					if p.Bag.UsableCountOf(id) != ctx.Bag.UsableCountOf(id) {
						t.Fatal("conversion consumed extra materials")
					}
				}
				input.CubeInputs = append(input.CubeInputs, CubeInput{ItemID: CubeStarMarrow, Count: 1})
				if _, _, err := MatchCubeInputs(ctx, input); CubeErrorCode(err) != "no_match" {
					t.Fatalf("extra material accepted: %v", err)
				}
			}
			if recipe.Operation == "salvage.normal" || recipe.Operation == "salvage.unique" || recipe.Operation == "salvage.rune" {
				wantMarrow := int32(18)
				if recipe.Operation == "salvage.unique" {
					wantMarrow = 54
					if p.Bag.UsableCountOf(CubeImmortalEcho)-ctx.Bag.UsableCountOf(CubeImmortalEcho) != 1 {
						t.Fatal("unique echo yield must be one")
					}
				}
				if recipe.Operation == "salvage.rune" {
					wantMarrow = 4
				}
				if p.Bag.UsableCountOf(CubeStarMarrow)-ctx.Bag.UsableCountOf(CubeStarMarrow) != wantMarrow {
					t.Fatal("salvage marrow yield changed")
				}
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("execution mutated live inventory")
			}
			for _, id := range []int32{1007, 1008, 1009, 1012} {
				if _, ok := ctx.Rules.Recipe(id); ok {
					t.Fatalf("retired recipe %d restored", id)
				}
			}
		})
	}
}
