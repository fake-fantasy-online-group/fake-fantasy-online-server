package domain

import (
	"reflect"
	"strconv"
	"testing"
)

func batchSalvageFixture(t *testing.T, n int) (CubeContext, EndgameRequest) {
	t.Helper()
	ctx, req := lifeCubeFixture(t, "life.salvage.unique", nil)
	var err error
	ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{
		{ID: 13001, Name: "分解", Description: "原矿", Operation: "life.salvage.normal", TargetRequired: true},
		{ID: 13002, Name: "分解", Description: "原矿", Operation: "life.salvage.unique", TargetRequired: true},
	}, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	original := ctx.Bag.At(200)
	req.CubeInputs = nil
	for i := 0; i < n; i++ {
		st := original
		st.UID = int64(42 + i)
		if i%2 != 0 {
			st.Legendary = LegendaryInstance{}
			st.Endgame = EndgameItemState{}
		}
		ctx.Bag.Set(200+i, st)
		req.CubeInputs = append(req.CubeInputs, CubeInput{UID: strconv.FormatInt(st.UID, 10), Count: int32(i - 1)})
	}
	return ctx, req
}

func TestBatchSalvageSelectedEquipmentCountsAndAggregation(t *testing.T) {
	for _, n := range []int{1, 2, 35} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			ctx, req := batchSalvageFixture(t, n)
			before := ctx.Bag.Clone()
			calls := 0
			ctx.Intn = func(int) int { calls++; return 0 }
			choices, plans, err := MatchCubeInputs(ctx, req)
			if err != nil || len(choices) != 1 || len(plans) != 1 {
				t.Fatalf("match: %v", err)
			}
			if calls != 0 || !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("preview mutated or rolled")
			}
			plan, err := PlanCube(ctx, choices[0], true)
			if err != nil {
				t.Fatal(err)
			}
			var total int32
			for i := 0; i < n; i++ {
				st := before.At(200 + i)
				d := ctx.Items[st.Item]
				total += EquipmentSalvageOreCount(d.Equip.Slot, EquipmentSalvageQuality(st, d))
				if !plan.Bag.At(200 + i).Empty() {
					t.Fatal("not all selected consumed")
				}
			}
			if len(plan.ConsumedUIDs) != n || plan.Bag.CountOf(4001) != total || int32(calls) != total {
				t.Fatalf("wrong consumption/output: %d %d %d want %d", len(plan.ConsumedUIDs), plan.Bag.CountOf(4001), calls, total)
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("candidate mutated live bag")
			}
			counts := map[ItemID]int{}
			for _, o := range plan.Outputs {
				if o.UID == 0 {
					counts[o.Item]++
				}
			}
			if counts[4001] != 1 {
				t.Fatal("output ore was not aggregated")
			}
		})
	}
}

func TestBatchSalvageInvalidInputsAtomicNoRNG(t *testing.T) {
	for _, name := range []string{"duplicate", "noncanonical", "unknown", "locked", "corrupt_count", "duplicate_bag", "material", "source_arg", "36"} {
		t.Run(name, func(t *testing.T) {
			ctx, req := batchSalvageFixture(t, 2)
			switch name {
			case "duplicate":
				req.CubeInputs[1] = req.CubeInputs[0]
			case "noncanonical":
				req.CubeInputs[1].UID = "043"
			case "unknown":
				req.CubeInputs[1].UID = "99999"
			case "locked":
				st := ctx.Bag.At(201)
				st.Locked = true
				ctx.Bag.Set(201, st)
			case "corrupt_count":
				st := ctx.Bag.At(201)
				st.Count = 2
				ctx.Bag.Set(201, st)
			case "duplicate_bag":
				ctx.Bag.Set(202, ctx.Bag.At(201))
			case "material":
				req.CubeInputs = append(req.CubeInputs, CubeInput{ItemID: CubeStarMarrow})
			case "source_arg":
				req.ItemUIDs = []string{"42"}
			case "36":
				ctx, req = batchSalvageFixture(t, 36)
			}
			before := ctx.Bag.Clone()
			ctx.Intn = func(int) int { t.Fatal("invalid inputs used RNG"); return 0 }
			if _, _, err := MatchCubeInputs(ctx, req); err == nil {
				t.Fatal("accepted invalid input")
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("failed operation mutated bag")
			}
		})
	}
}

func TestBatchSalvageReturnsCardsAndChecksFullMaterialBagBeforeRNG(t *testing.T) {
	ctx, req := batchSalvageFixture(t, 2)
	st := ctx.Bag.At(200)
	st.Legendary = LegendaryInstance{} // Six innate darkgold affixes leave no socket capacity.
	st.Endgame = EndgameItemState{}
	ctx.Bag.Set(200, st)
	cubeInstall(&ctx, FirstAstralRune)
	choices, _, err := MatchCubeInputs(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanCube(ctx, choices[0], true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Bag.CountOf(FirstAstralRune) != 1 {
		t.Fatal("embedded card not returned")
	}
	for i := 100; i < 200; i++ {
		ctx.Bag.Set(i, Stack{Item: CubeStarMarrow, Count: 999, Bound: true})
	}
	before := ctx.Bag.Clone()
	ctx.Intn = func(int) int { t.Fatal("full material bag rolled"); return 0 }
	if _, _, err = MatchCubeInputs(ctx, req); err == nil || CubeErrorCode(err) != "bag_full" {
		t.Fatal("accepted full material bag", err)
	}
	if !reflect.DeepEqual(before, ctx.Bag) {
		t.Fatal("capacity failure consumed targets")
	}
}

func TestBatchSalvageMixedLevelsSlotsAndSelectedOnly(t *testing.T) {
	ctx, req := batchSalvageFixture(t, 2)
	st := ctx.Bag.At(201)
	def := ctx.Items[st.Item]
	equip := *def.Equip
	def.ID = 199999
	def.Equip = &equip
	def.Equip.LevelReq = 151
	def.Equip.Slot = int32(SlotHead)
	ctx.Items[def.ID] = def
	st.Item = def.ID
	ctx.Bag.Set(201, st)
	untouched := st
	untouched.UID = 999
	ctx.Bag.Set(202, untouched)
	first := ctx.Bag.At(200)
	d := ctx.Items[first.Item]
	low := *d.Equip
	d.Equip = &low
	d.Equip.LevelReq = 1
	ctx.Items[d.ID] = d
	ctx.Intn = func(n int) int { return n - 1 }
	choices, _, err := MatchCubeInputs(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanCube(ctx, choices[0], true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Bag.CountOf(4001) != EquipmentSalvageOreCount(d.Equip.Slot, EquipmentSalvageQuality(first, d)) || plan.Bag.CountOf(4028) != EquipmentSalvageOreCount(def.Equip.Slot, EquipmentSalvageQuality(st, def)) {
		t.Fatal("levels/quality/slot yields not evaluated independently", plan.Outputs)
	}
	if !reflect.DeepEqual(plan.Bag.At(202), untouched) {
		t.Fatal("salvaged unselected equipment")
	}
}

func TestBatchSalvageRejectsRemovedWingExtraction(t *testing.T) {
	wingCtx := wingCubeFixture(t)
	ctx, req := batchSalvageFixture(t, 2)
	ctx.WingRules = wingCtx.WingRules
	if _, _, err := MatchCubeInputs(ctx, req); err != nil {
		t.Fatalf("two non-wings incorrectly entered wing extraction: %v", err)
	}
	donor, def := wingPowerDonor("TY-A01")
	def.ID = 900999
	def.InventoryTab, def.InventoryTabKnown, def.InstanceKind = 2, true, ItemInstanceEquipment
	donor.Item, donor.UID = def.ID, 43
	wingCtx.Items[def.ID] = def
	wingCtx.Bag.Set(201, donor)
	wingReq := EndgameRequest{CubeInputs: []CubeInput{{UID: "42"}, {UID: "43"}}}
	choices, plans, err := MatchCubeInputs(wingCtx, wingReq)
	if err == nil || len(plans) != 0 || len(choices) != 0 {
		t.Fatalf("wing recipe was reinterpreted as salvage: %v %+v", err, choices)
	}
	// Even an invalid donor must fail, never destroy the wing.
	donor.Legendary = LegendaryInstance{}
	wingCtx.Bag.Set(201, donor)
	if _, _, err = MatchCubeInputs(wingCtx, wingReq); err == nil {
		t.Fatal("invalid wing recipe silently salvaged")
	}
}

func TestBatchSalvageStacksIntoFullMaterialTab(t *testing.T) {
	ctx, req := batchSalvageFixture(t, 2)
	var total int32
	for i := 200; i < 202; i++ {
		st := ctx.Bag.At(i)
		d := ctx.Items[st.Item]
		eq := *d.Equip
		d.Equip = &eq
		d.Equip.LevelReq = 1
		ctx.Items[d.ID] = d
		total += EquipmentSalvageOreCount(d.Equip.Slot, EquipmentSalvageQuality(st, d))
	}
	for i := 100; i < 200; i++ {
		ctx.Bag.Set(i, Stack{Item: CubeStarMarrow, Count: MaxStack, Bound: true})
	}
	ctx.Bag.Set(100, Stack{Item: 4001, Count: MaxStack - total, Bound: true})
	choices, _, err := MatchCubeInputs(ctx, req)
	if err != nil {
		t.Fatal("existing ore stack capacity should be usable", err)
	}
	plan, err := PlanCube(ctx, choices[0], true)
	if err != nil || plan.Bag.At(100).Count != MaxStack {
		t.Fatal("wrong aggregate stack merge", err)
	}
}
