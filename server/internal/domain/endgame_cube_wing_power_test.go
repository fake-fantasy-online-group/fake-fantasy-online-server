package domain

import (
	"reflect"
	"testing"
)

func TestWingScrollSelectionAndCubeInheritance(t *testing.T) {
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
		wing := ctx.Bag.At(200)
		wing.Legendary = LegendaryInstance{}
		got, err := (LegendaryPowerBook{id: 8123}).ImprintWingPower(wing, ctx.Items[wing.Item], ctx.Character, id, ctx.WingRules)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("imprint mutated original inventory")
		}
		if ctx.Bag.At(201) != source || ctx.Bag.At(202) != extra {
			t.Fatal("scroll imprint consumed unrelated donor")
		}
		ctx.Bag.Set(200, got)
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
