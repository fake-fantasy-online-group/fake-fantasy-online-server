package domain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCubeRandomSocketUniformBoundsAndPreview(t *testing.T) {
	for capacity := 1; capacity <= 5; capacity++ {
		for roll := 0; roll < capacity; roll++ {
			ctx, req := cubeInputs(CubeOpenSocket)
			def := ctx.Items[1001]
			def.Equip.Affixes = make([]Affix, 5-capacity)
			ctx.Items[1001] = def
			calls := 0
			ctx.Intn = func(n int) int {
				calls++
				if n != capacity {
					t.Fatalf("range %d want %d", n, capacity)
				}
				return roll
			}
			before := ctx.Bag.Clone()
			preview, err := PlanCube(ctx, req, false)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 || preview.Target.SocketCount != 0 || !strings.Contains(preview.Description, fmt.Sprintf("每种孔数概率1/%d", capacity)) {
				t.Fatalf("preview leaked RNG or lacks range: %+v", preview)
			}
			plan, err := PlanCube(ctx, req, true)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || int(plan.Target.SocketCount) != roll+1 {
				t.Fatalf("invalid outcome %d / calls %d", plan.Target.SocketCount, calls)
			}
			if plan.Money != ctx.Money-5000 || plan.Bag.CountOf(CubeStarMarrow) != ctx.Bag.CountOf(CubeStarMarrow)-4 || plan.Bag.CountOf(CubePurifiedDust) != ctx.Bag.CountOf(CubePurifiedDust) {
				t.Fatal("random socket costs changed")
			}
			after := plan.Target
			after.SocketCount = 0
			after.Endgame = before.At(200).Endgame
			if after != before.At(200) || !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("opening changed existing identity or input state")
			}
			ctx.Bag = plan.Bag
			ctx.Money = plan.Money
			if _, err = PlanCube(ctx, req, true); CubeErrorCode(err) != "requirements" || calls != 1 {
				t.Fatal("existing holes could reroll")
			}
		}
	}
}

func TestCubeRandomSocketRejectsInvalidRNGAndKeepsExistingCards(t *testing.T) {
	for _, roll := range []int{-1, 5, 999} {
		ctx, req := cubeInputs(CubeOpenSocket)
		before := ctx.Bag.Clone()
		ctx.Intn = func(int) int { return roll }
		if _, err := PlanCube(ctx, req, true); CubeErrorCode(err) != "requirements" || !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("invalid RNG accepted or consumed input")
		}
	}
	ctx, req := cubeInputs(CubeOpenSocket)
	cubeInstall(&ctx, FirstAstralRune)
	before := ctx.Bag.Clone()
	ctx.Intn = func(int) int { t.Fatal("existing socket consumed RNG"); return 0 }
	if _, err := PlanCube(ctx, req, true); CubeErrorCode(err) != "requirements" || !reflect.DeepEqual(before, ctx.Bag) {
		t.Fatal("existing card replaced")
	}
}

func TestCubeRandomSocketWorksInFullBagWithoutDust(t *testing.T) {
	ctx, req := cubeInputs(CubeOpenSocket)
	ctx.Bag.Set(101, Stack{})
	// A full equipment page is safe: the existing target is changed in place.
	for i := 201; i < 300; i++ {
		ctx.Bag.Set(i, Stack{UID: int64(1000 + i), Item: 1001, Count: 1, InstanceKind: ItemInstanceEquipment})
	}
	if _, err := PlanCube(ctx, req, true); err != nil {
		t.Fatal(err)
	}
}
