package domain

import "testing"

func TestFixedItemBindingOnlyChangesDefinedBinding(t *testing.T) {
	defs, _ := astralFixture()
	d := defs[FirstAstralRune]
	d.FixedBinding = ItemBindingFixedUnbound
	old := NewStack(d, 7)
	old.Bound = true
	old.Card.Bound = true
	old.Locked = true
	old.Card.Locked = true
	old.Card.Effects[0].Value = 123
	got := NormalizeItemBinding(d, old)
	want := old
	want.Bound = false
	want.Card.Bound = false
	if got != want {
		t.Fatal("normalization lost non-binding instance metadata")
	}
	if NormalizeItemBinding(d, got) != got {
		t.Fatal("not idempotent")
	}
	gear := ItemDef{ID: 999, Equip: &EquipDef{}, FixedBinding: ItemBindingFixedUnbound}
	st := Stack{Item: 999, Count: 1, UID: 500, Bound: true}
	if NormalizeItemBinding(gear, st) != st {
		t.Fatal("equipment bind-on-equip changed")
	}
	d.FixedBinding = ItemBindingUnspecified
	if NormalizeItemBinding(d, old) != old {
		t.Fatal("unknown rule was guessed")
	}
}

func TestFixedItemBindingAllAcquisitionIngressAndNestedCards(t *testing.T) {
	defs, _ := astralFixture()
	d := defs[FirstAstralRune]
	d.FixedBinding = ItemBindingFixedUnbound
	a := NewStack(d, 2)
	b := NewStack(d, 3)
	b.Bound = true
	b.Card.Bound = true
	bag := NewBag(4)
	if bag.AddStack(d, a) != 0 || bag.AddStack(d, b) != 0 || bag.At(0).Count != 5 || bag.At(0).Bound || bag.At(0).Card.Bound || !bag.At(1).Empty() {
		t.Fatal("channel override survived AddStack")
	}
	old := bag.At(0)
	old.Bound = true
	old.Card.Bound = true
	bag.Set(0, old)
	if bag.Add(d, 4) != 0 || bag.At(0).Count != 9 || bag.At(0).Bound || bag.At(0).Card.Bound {
		t.Fatal("Add did not share canonical binding")
	}
	gear := Stack{Item: 999, UID: 777, Count: 1, Bound: true}
	gear.Sockets[0] = d.ID
	gear.SocketUIDs[0] = b.UID
	gear.SocketCards[0] = b.Card
	lookup := func(id ItemID) (ItemDef, bool) {
		if id == d.ID {
			return d, true
		}
		return ItemDef{}, false
	}
	got := NormalizeStackBindings(gear, lookup)
	want := gear
	want.SocketCards[0].Bound = false
	if got != want {
		t.Fatal("nested card repair changed equipment or effects")
	}
	tool := ItemDef{ID: PowerScrollItemID, InstanceKind: ItemInstanceEquipment, FixedBinding: ItemBindingFixedBound}
	scroll := NewStack(tool, 1)
	if !scroll.Bound || scroll.UID == 0 {
		t.Fatal("non-gear storage kind1 treated as wearable equipment")
	}
}

func TestFixedItemBindingCubeOutputsMatchCanonicalBag(t *testing.T) {
	ctx, req := cubeInputs(CubeInscribeRuneBase + 1)
	for id, d := range ctx.Items {
		if id >= FirstAstralRune && id <= LastAstralRune {
			d.FixedBinding = ItemBindingFixedUnbound
			ctx.Items[id] = d
		}
	}
	plan, err := PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, st := range plan.Outputs {
		if st.Item >= FirstAstralRune && st.Item <= LastAstralRune {
			count++
			if st.Bound || st.Card.Bound {
				t.Fatal("cube response retained wrong binding despite canonical bag")
			}
		}
	}
	if count == 0 {
		t.Fatal("no rune output exercised")
	}
}

func TestFixedItemBindingSocketDoesNotInheritOrPolluteEquipment(t *testing.T) {
	for _, gearBound := range []bool{false, true} {
		defs, lookup := astralFixture()
		d := defs[FirstAstralRune]
		d.FixedBinding = ItemBindingFixedUnbound
		defs[d.ID] = d
		gear := NewStack(defs[999001], 1)
		gear.SocketCount = 2
		gear.Bound = gearBound
		rune := NewStack(d, 3)
		rune.Bound = true
		rune.Card.Bound = true
		bag := NewBag(4)
		bag.Set(0, gear)
		bag.Set(1, rune)
		plan, err := PlanAstralInsert(bag, NewEquipSet(), lookup, gear.UID, rune.UID, 0, &Character{Level: 100})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Target.Bound != gearBound || plan.Target.SocketCards[0].Bound || plan.Rune.Bound || plan.Bag.At(1).Bound || bag.At(1) != rune {
			t.Fatal("socket changed equipment binding or retained source binding")
		}
		out, err := PlanAstralRemove(plan.Bag, plan.Worn, lookup, gear.UID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if out.Bag.At(1).Count != 3 || out.Bag.At(1).Bound || out.Bag.At(1).Card.Bound || out.Target.Bound != gearBound || out.Bag.At(1).UID != rune.UID {
			t.Fatal("socket roundtrip polluted binding/quantity/UID")
		}
	}
}
