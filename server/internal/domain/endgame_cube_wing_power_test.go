package domain

import (
	"reflect"
	"strconv"
	"testing"
)

func TestWingCubePowerSelectionAndInheritance(t *testing.T) {
	ctx := wingCubeFixture(t)
	extra, def := wingPowerDonor("TY-A02")
	extra.UID = 999
	ctx.Bag.Set(202, extra)
	for n, id := range []string{"TY-A01", "TY-D01", "TY-U01", "TY-A02"} {
		source, d := wingPowerDonor(id)
		source.UID = int64(100 + n)
		if n == 0 {
			source.InstanceKind = ItemInstanceNone
		} // legacy native location rows
		def = d
		def.InventoryTab = 2
		def.InventoryTabKnown = true
		def.InstanceKind = ItemInstanceEquipment
		ctx.Items[def.ID] = def
		ctx.Bag.Set(201, source)
		before := ctx.Bag.Clone()
		prior := ctx.Bag.At(200).WingPowers
		req := wingInputs(42)
		req.CubeInputs = append(req.CubeInputs, CubeInput{UID: strconv.FormatInt(source.UID, 10), Count: 1})
		requests, plans, err := MatchCubeInputs(ctx, req)
		if err != nil || len(requests) != 1 || len(plans) != 1 {
			t.Fatalf("two gear match %s: %v", id, err)
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("quote mutated original inventory")
		}
		plan, err := PlanCube(ctx, requests[0], true)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Bag.At(201).Empty() || plan.Bag.At(202) != extra || len(plan.ConsumedUIDs) != 1 || plan.ConsumedUIDs[0] != source.UID || plan.Target.UID != 42 {
			t.Fatal("selected UID consumption violated", plan)
		}
		p, _ := LegendaryPower(id)
		index, _ := WingPowerIndex(p)
		for i, v := range plan.Target.WingPowers {
			if i == index {
				if v.PowerID != id {
					t.Fatal("wrong power extracted")
				}
			} else if v != prior[i] {
				t.Fatal("other category overwritten")
			}
		}
		ctx.Bag = plan.Bag
	}
	powers := ctx.Bag.At(200).WingPowers
	for _, materials := range [][]CubeMaterialDef{{{25029, 2}, {25028, 2}}, {{25029, 8}, {11682, 1}}} {
		reqs, _, err := MatchCubeInputs(ctx, wingInputs(42, materials...))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := PlanCube(ctx, reqs[0], true)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Target.WingPowers != powers || plan.Bag.At(202) != extra {
			t.Fatal("upgrade/change lost powers or consumed unrelated donor")
		}
		ctx.Bag = plan.Bag
	}
	if got := ctx.Bag.At(200).Item; got != 22864 {
		t.Fatal("did not upgrade and change series", got)
	}
	for _, mutate := range []func(*Stack, *ItemDef){func(s *Stack, d *ItemDef) { s.Locked = true }, func(s *Stack, d *ItemDef) { s.Legendary = LegendaryInstance{} }, func(s *Stack, d *ItemDef) { d.LegendaryEnabled = false }, func(s *Stack, d *ItemDef) { d.Equip.Slot = int32(SlotShield) }} {
		source, d := wingPowerDonor("TY-A01")
		source.UID = 700
		d.InventoryTab = 2
		d.InventoryTabKnown = true
		d.InstanceKind = ItemInstanceEquipment
		mutate(&source, &d)
		ctx.Items[d.ID] = d
		ctx.Bag.Set(201, source)
		req := wingInputs(42)
		req.CubeInputs = append(req.CubeInputs, CubeInput{UID: "700", Count: 1})
		before := ctx.Bag.Clone()
		if _, _, err := MatchCubeInputs(ctx, req); err == nil || !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("invalid donor accepted or consumed")
		}
	}
}
