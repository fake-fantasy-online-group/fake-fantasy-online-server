package domain

import (
	"reflect"
	"strconv"
	"testing"
)

func cubeFixture() CubeContext {
	items := map[ItemID]ItemDef{}
	for _, d := range CubeMaterialDefinitions() {
		items[d.ID] = d
	}
	for _, d := range AstralRuneItems(7) {
		items[d.ID] = d
	}
	items[1001] = ItemDef{ID: 1001, Name: "铁剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{Slot: 4, LevelReq: 50, Durable: 100}}
	b := NewBag(DefaultBagSlots)
	for i, id := range []ItemID{CubeStarMarrow, CubePurifiedDust, CubeImmortalEcho} {
		b.Set(100+i, Stack{Item: id, Count: 800, Bound: true})
	}
	b.Set(200, Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 77, MaxDurability: 95, DurabilityWearRaw: 12, RefineLevel: 3, FusedAppearance: 999, FusedSoul: 111, FusedDragon: 222})
	return CubeContext{Bag: b, Money: 10000000, Items: items, Intn: func(n int) int { return n / 2 }, NewUID: func() int64 { return 999999 }}
}
func cubeRune(ctx CubeContext, id ItemID, uid int64) Stack {
	return Stack{UID: uid, Item: id, Count: 1, InstanceKind: ItemInstanceSocketCard, Card: ctx.Items[id].CardDefaults}
}
func cubeInstall(ctx *CubeContext, ids ...ItemID) {
	st := ctx.Bag.At(200)
	st.SocketCount = uint8(len(ids))
	for i, id := range ids {
		st.Sockets[i] = id
		st.SocketUIDs[i] = int64(501 + i)
		st.SocketCards[i] = ctx.Items[id].CardDefaults
		st.SocketCards[i].Effects[0].Value += int32(i)
		st.SocketCards[i].Bound = i%2 == 0
	}
	ctx.Bag.Set(200, st)
}
func cubeMakeUnique(ctx *CubeContext) {
	st := ctx.Bag.At(200)
	st.Endgame = EndgameItemState{UniqueID: 1001, UniqueRollBP: 10000, Revision: 7}
	ctx.Bag.Set(200, st)
}
func cubeInputs(id int32) (CubeContext, EndgameRequest) {
	ctx := cubeFixture()
	req := EndgameRequest{Op: "cube.quote", RecipeID: id, TargetUID: "42"}
	switch id {
	case CubeSalvageNormal:
		cubeInstall(&ctx, FirstAstralRune)
	case CubeSalvageUnique:
		cubeMakeUnique(&ctx)
		cubeInstall(&ctx, FirstAstralRune)
	case CubeSalvageRune:
		req.TargetUID = ""
		req.RuneUID = "501"
		ctx.Bag.Set(110, cubeRune(ctx, FirstAstralRune, 501))
	case CubeExtractRunes:
		cubeInstall(&ctx, FirstAstralRune, FirstAstralRune+5)
	case CubeReorderRunes:
		cubeInstall(&ctx, FirstAstralRune, FirstAstralRune+5)
		req.ItemUIDs = []string{"502", "501"}
	case CubeInscribeArray:
		st := ctx.Bag.At(200)
		st.SocketCount = 2
		ctx.Bag.Set(200, st)
		ctx.Bag.Set(110, cubeRune(ctx, FirstAstralRune, 501))
		ctx.Bag.Set(111, cubeRune(ctx, FirstAstralRune+5, 502))
		req.ItemUIDs = []string{"501", "502"}
	case CubeReforgeUnique, CubeTemperUnique:
		cubeMakeUnique(&ctx)
	default:
		if _, ok := cubeRecipeRune(id, CubeInscribeRuneBase); ok {
			req.TargetUID = ""
		}
		if out, ok := cubeRecipeRune(id, CubeConvertRuneBase); ok {
			req.TargetUID = ""
			source := FirstAstralRune + ((out-FirstAstralRune)/7)*7
			if source == out {
				source++
			}
			for i := 0; i < 3; i++ {
				ctx.Bag.Set(110+i, cubeRune(ctx, source, int64(501+i)))
				req.ItemUIDs = append(req.ItemUIDs, strconv.Itoa(501+i))
			}
		}
	}
	return ctx, req
}
func TestCubeEveryRecipePreviewAndExecution(t *testing.T) {
	all := CubeRecipes(cubeFixture().Items)
	if len(all) != 66 {
		t.Fatalf("recipes=%d", len(all))
	}
	ops := map[string]bool{}
	for _, recipe := range all {
		ops[recipe.Operation] = true
		t.Run(strconv.Itoa(int(recipe.ID)), func(t *testing.T) {
			ctx, req := cubeInputs(recipe.ID)
			before := ctx.Bag.Clone()
			rng, alloc := 0, 0
			ctx.Intn = func(n int) int { rng++; return n / 2 }
			ctx.NewUID = func() int64 { alloc++; return 999999 }
			preview, err := PlanCube(ctx, req, false)
			if err != nil {
				t.Fatal(err)
			}
			if rng != 0 || alloc != 0 || !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("preview mutated or generated RNG/UID")
			}
			if preview.Recipe.ID != recipe.ID || preview.Fingerprint != CubeFingerprint(ctx.Bag, ctx.Money, req) {
				t.Fatal("quote identity invalid")
			}
			plan, err := PlanCube(ctx, req, true)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("execute mutated input")
			}
			if plan.Bag == ctx.Bag || plan.Money > ctx.Money {
				t.Fatal("invalid candidate")
			}
			for _, m := range plan.Materials {
				if plan.Bag.UsableCountOf(ItemID(m.ItemID)) != ctx.Bag.UsableCountOf(ItemID(m.ItemID))-m.Count {
					t.Fatalf("wrong consumption %+v", m)
				}
			}
			wantRNG := 0
			if recipe.ID == CubeAwakenUnique || recipe.ID == CubeReforgeUnique {
				wantRNG = 1
			}
			if rng != wantRNG {
				t.Fatalf("RNG=%d want%d", rng, wantRNG)
			}
			wantAlloc := 0
			if recipe.ID > 11000 {
				wantAlloc = 1
			}
			if alloc != wantAlloc {
				t.Fatalf("alloc=%d", alloc)
			}
		})
	}
	if len(ops) != 12 {
		t.Fatalf("operations=%d", len(ops))
	}
}
func TestCubeUniqueMutationPreservesInstance(t *testing.T) {
	for _, id := range []int32{CubeAwakenUnique, CubeReforgeUnique, CubeTemperUnique} {
		ctx, req := cubeInputs(id)
		cubeInstall(&ctx, FirstAstralRune, FirstAstralRune+5)
		before := ctx.Bag.At(200)
		plan, err := PlanCube(ctx, req, true)
		if err != nil {
			t.Fatal(err)
		}
		after := plan.Target
		metadata := after.Endgame
		after.Endgame = before.Endgame
		if after != before {
			t.Fatalf("%d changed unrelated instance fields", id)
		}
		if metadata.UniqueID != 1001 || metadata.Revision != before.Endgame.Revision+1 {
			t.Fatal("unique identity/revision not retained")
		}
	}
}
func TestCubeExtractionAndReorderPreserveExactCards(t *testing.T) {
	ctx, req := cubeInputs(CubeExtractRunes)
	before := ctx.Bag.At(200)
	p, err := PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Target.SocketCount != 2 || UsedEquipmentSockets(p.Target) != 0 {
		t.Fatal("empty holes not retained")
	}
	for i, st := range p.Outputs {
		if st.UID != before.SocketUIDs[i] || st.Card != before.SocketCards[i] || st.Bound != before.SocketCards[i].Bound {
			t.Fatal("card identity/effects/binding changed")
		}
	}
	ctx, req = cubeInputs(CubeReorderRunes)
	before = ctx.Bag.At(200)
	p, err = PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if p.Target.SocketUIDs[i] != before.SocketUIDs[1-i] || p.Target.SocketCards[i] != before.SocketCards[1-i] {
			t.Fatal("reorder changed card state")
		}
	}
}
func TestCubePreflightFailureConsumesNothing(t *testing.T) {
	tests := []struct {
		name   string
		recipe int32
		change func(*CubeContext, *EndgameRequest)
		code   string
	}{
		{"locked target", 1010, func(c *CubeContext, r *EndgameRequest) { st := c.Bag.At(200); st.Locked = true; c.Bag.Set(200, st) }, "locked"},
		{"locked material", 1010, func(c *CubeContext, r *EndgameRequest) { st := c.Bag.At(100); st.Locked = true; c.Bag.Set(100, st) }, "materials"},
		{"no funds", 1010, func(c *CubeContext, r *EndgameRequest) { c.Money = 0 }, "funds"},
		{"not owner", 1010, func(c *CubeContext, r *EndgameRequest) { r.TargetUID = "998" }, "not_owner"},
		{"duplicate uid", 1010, func(c *CubeContext, r *EndgameRequest) { c.Bag.Set(201, c.Bag.At(200)) }, "invalid_request"},
		{"over capacity", 1006, func(c *CubeContext, r *EndgameRequest) { st := c.Bag.At(200); st.SocketCount = 5; c.Bag.Set(200, st) }, "requirements"},
		{"locked socket", 1007, func(c *CubeContext, r *EndgameRequest) {
			st := c.Bag.At(200)
			st.SocketCards[0].Locked = true
			c.Bag.Set(200, st)
		}, "locked"},
		{"repeated array rune", 1009, func(c *CubeContext, r *EndgameRequest) { r.ItemUIDs = []string{"501", "501"} }, "not_owner"},
		{"invalid array order", 1009, func(c *CubeContext, r *EndgameRequest) { r.ItemUIDs = []string{"502", "501"} }, "requirements"},
		{"no-op reorder", 1008, func(c *CubeContext, r *EndgameRequest) { r.ItemUIDs = []string{"501", "502"} }, "requirements"},
		{"temper capped", 1012, func(c *CubeContext, r *EndgameRequest) {
			st := c.Bag.At(200)
			st.Endgame.UniqueRollBP = 12000
			c.Bag.Set(200, st)
		}, "requirements"},
		{"missing material definition", 1010, func(c *CubeContext, r *EndgameRequest) { delete(c.Items, CubePurifiedDust) }, "requirements"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, req := cubeInputs(tt.recipe)
			tt.change(&ctx, &req)
			before := ctx.Bag.Clone()
			ctx.Intn = func(int) int { t.Fatal("RNG before successful preflight"); return 0 }
			ctx.NewUID = func() int64 { t.Fatal("UID before successful preflight"); return 0 }
			_, err := PlanCube(ctx, req, true)
			if err == nil || CubeErrorCode(err) != tt.code {
				t.Fatalf("error=%v code=%s want%s", err, CubeErrorCode(err), tt.code)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("rejection changed inventory")
			}
		})
	}
}
func TestCubeFullBagPreflightDoesNotAllocate(t *testing.T) {
	ctx, req := cubeInputs(11001)
	for i := 103; i < 200; i++ {
		ctx.Bag.Set(i, cubeRune(ctx, FirstAstralRune, int64(2000+i)))
	}
	ctx.NewUID = func() int64 { t.Fatal("allocated before bag check"); return 0 }
	_, err := PlanCube(ctx, req, true)
	if err == nil || CubeErrorCode(err) != "bag_full" {
		t.Fatalf("full bag error %v", err)
	}
	// Exactly consumed material slots are available to the output.
	ctx.Bag.Set(100, Stack{Item: CubeStarMarrow, Count: 30, Bound: true})
	ctx.Bag.Set(101, Stack{Item: CubePurifiedDust, Count: 10, Bound: true})
	ctx.NewUID = func() int64 { return 999999 }
	p, err := PlanCube(ctx, req, true)
	if err != nil || p.Bag.At(100).UID != 999999 {
		t.Fatalf("freed material slot unavailable: %v", err)
	}
}
func TestCubeFingerprintCoversNativeMutations(t *testing.T) {
	ctx, req := cubeInputs(1010)
	old := CubeFingerprint(ctx.Bag, ctx.Money, req)
	st := ctx.Bag.At(200)
	st.RefineLevel++
	ctx.Bag.Set(200, st)
	if old == CubeFingerprint(ctx.Bag, ctx.Money, req) {
		t.Fatal("native refine absent")
	}
	if CubeFingerprint(ctx.Bag, ctx.Money, req) == CubeFingerprint(ctx.Bag, ctx.Money+1, req) {
		t.Fatal("money absent")
	}
	r := req
	r.RecipeID++
	if CubeFingerprint(ctx.Bag, ctx.Money, req) == CubeFingerprint(ctx.Bag, ctx.Money, r) {
		t.Fatal("recipe absent")
	}
}
func TestCubeReforgeCannotReturnCurrentPotency(t *testing.T) {
	for _, old := range []int32{8000, 10000, 12000} {
		for _, roll := range []int{0, 1999, 2000, 3999} {
			ctx, req := cubeInputs(1011)
			st := ctx.Bag.At(200)
			st.Endgame.UniqueRollBP = old
			ctx.Bag.Set(200, st)
			ctx.Intn = func(int) int { return roll }
			p, err := PlanCube(ctx, req, true)
			if err != nil || p.Target.Endgame.UniqueRollBP == old || p.Target.Endgame.UniqueRollBP < 8000 || p.Target.Endgame.UniqueRollBP > 12000 {
				t.Fatalf("bad reforge old%d roll%d: %+v %v", old, roll, p.Target.Endgame, err)
			}
		}
	}
}
