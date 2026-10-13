package domain

import (
	"reflect"
	"testing"
)

func TestAstralEveryRuneRequiredLevelAndAtomicInsert(t *testing.T) {
	defs, lookup := astralFixture()
	runes := AstralRunes(testEndgameCatalog())
	if runes[0].MinLevel != 12 || runes[27].MinLevel != 60 {
		t.Fatal("progression endpoints")
	}
	for _, rune := range runes {
		t.Run(rune.Name, func(t *testing.T) {
			rd := defs[rune.Item]
			if rd.Level != rune.MinLevel || rd.UseLevel != rune.MinLevel {
				t.Fatal("item requirement drift")
			}
			target := NewStack(defs[999001], 1)
			target.SocketCount = 2
			card := NewStack(rd, 1)
			bag := NewBag(3)
			bag.Set(0, target)
			bag.Set(1, card)
			before := bag.Clone()
			if _, err := PlanAstralInsert(bag, NewEquipSet(), lookup, target.UID, card.UID, 0, nil); err == nil {
				t.Fatal("missing character accepted")
			}
			for _, level := range []int32{11, 12, 59, 60, rune.MinLevel - 1, rune.MinLevel} {
				plan, err := PlanAstralInsert(bag, NewEquipSet(), lookup, target.UID, card.UID, 0, &Character{Level: level})
				if (err == nil) != (level >= rune.MinLevel) {
					t.Fatalf("level %d requirement %d: %v", level, rune.MinLevel, err)
				}
				if !reflect.DeepEqual(bag, before) {
					t.Fatal("candidate mutated input")
				}
				if err == nil && (EquipmentRequiredLevel(defs[999001], plan.Target) != rune.MinLevel || plan.Target.SocketUIDs[0] != card.UID) {
					t.Fatal("effective requirement or identity")
				}
			}
		})
	}
}

func TestAstralFormationMaxRuneInsertGateAndExtractionRecovery(t *testing.T) {
	defs, lookup := astralFixture()
	recipe, _ := AstralRecipeByID(6, testEndgameCatalog()) // Ingredient levels 12/14; there is no independent formation tier.
	target := NewStack(defs[999001], 1)
	target.SocketCount = 2
	bag := NewBag(4)
	bag.Set(0, target)
	a := NewStack(defs[recipe.Runes[0]], 1)
	b := NewStack(defs[recipe.Runes[1]], 1)
	bag.Set(1, a)
	bag.Set(2, b)
	p, err := PlanAstralInsert(bag, NewEquipSet(), lookup, target.UID, a.UID, 0, &Character{Level: 14})
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range []int32{12, 13, 14, 19, 20, 60} {
		plan, err := PlanAstralInsert(p.Bag, p.Worn, lookup, target.UID, b.UID, 1, &Character{Level: level})
		if (err == nil) != (level >= recipe.MinLevel) {
			t.Fatalf("formation level %d: %v", level, err)
		}
		if err == nil {
			if EquipmentRequiredLevel(defs[999001], plan.Target) != recipe.MinLevel {
				t.Fatal("formation requirement missing")
			}
			out, err := PlanAstralRemove(plan.Bag, plan.Worn, lookup, target.UID, 1)
			if err != nil || out.Rune.UID != b.UID || EquipmentRequiredLevel(defs[999001], out.Target) != 12 {
				t.Fatal("extraction recovery", err)
			}
		}
	}
}

func TestAstralCubeCannotBypassLevelOnPreviewOrExecute(t *testing.T) {
	for _, op := range []int32{CubeInscribeArray, CubeReorderRunes} {
		for _, execute := range []bool{false, true} {
			ctx, req := cubeInputs(op)
			before := ctx.Bag.Clone()
			ctx.Level = 11
			if _, err := PlanCube(ctx, req, execute); err == nil {
				t.Fatalf("cube %d accepted level11", op)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("failed cube consumed inputs")
			}
			ctx.Level = 60
			if _, err := PlanCube(ctx, req, execute); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, req := cubeInputs(CubeExtractRunes)
	ctx.Level = 11
	if _, err := PlanCube(ctx, req, true); err != nil {
		t.Fatal("underlevel extraction must remain available", err)
	}
}

func TestAstralEveryFormationIngredientAndLevelBoundaries(t *testing.T) {
	defs, lookup := astralFixture()
	for _, r := range AstralRecipes(testEndgameCatalog()) {
		def := defs[999001]
		eq := *def.Equip
		eq.Slot = int32(r.Slots[0])
		def.Equip = &eq
		defs[def.ID] = def
		maxRune := int32(0)
		for _, id := range r.Runes {
			rune, _ := AstralRune(id, testEndgameCatalog())
			if rune.MinLevel > maxRune {
				maxRune = rune.MinLevel
			}
		}
		if r.MinLevel != maxRune || r.MinLevel < 12 || r.MinLevel > 60 {
			t.Fatal("invalid formation level", r.ID)
		}
		st := astralStack(t, r, def, lookup)
		worn := NewEquipSet()
		worn.Set(r.Slots[0], st)
		ch := &Character{EmploymentKnown: true, Employed: true}
		for race := Warrior; race <= Warlock; race++ {
			if r.Professions == 0 || r.Professions&(1<<uint8(race)) != 0 {
				ch.Race = race
				break
			}
		}
		for _, level := range []int32{11, 12, 59, 60, r.MinLevel - 1, r.MinLevel} {
			ch.Level = level
			if got := len(ActiveAstralRecipes(worn, lookup, ch)) == 1; got != (level >= r.MinLevel) {
				t.Fatalf("formation%d level%d active=%v", r.ID, level, got)
			}
		}
	}
}
