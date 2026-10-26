package domain

import "testing"

func TestAstralRuneStackIdentityAndSnapshots(t *testing.T) {
	defs, _ := astralFixture()
	d := defs[FirstAstralRune]
	a, b := NewStack(d, 3), NewStack(d, 2)
	if !IsAstralRuneStack(a) || !SameStackInstance(a, b) {
		t.Fatal("equivalent runes not stackable", a, b)
	}
	bag := NewBag(8)
	if bag.AddStack(d, a) != 0 || bag.AddStack(d, b) != 0 || bag.At(0).Count != 5 || bag.At(0).UID != a.UID {
		t.Fatal("merge changed identity or count", bag.At(0))
	}
	remain, taken, ok := SplitOwnedStack(bag.At(0), 2)
	if !ok || remain.Count != 3 || taken.Count != 2 || remain.UID != a.UID || taken.UID == a.UID || taken.Card != a.Card {
		t.Fatal("partial split", remain, taken)
	}
	_, whole, ok := SplitOwnedStack(taken, 2)
	if !ok || whole != taken {
		t.Fatal("whole move")
	}
	for _, change := range []func(*Stack){func(s *Stack) { s.Bound = true }, func(s *Stack) { s.Locked = true }, func(s *Stack) { s.Card.Bound = true }, func(s *Stack) { s.Card.Locked = true }, func(s *Stack) { s.Card.Effects[0].Value++ }, func(s *Stack) { s.Item++ }, func(s *Stack) { s.Durability = 1 }} {
		c := b
		change(&c)
		if SameStackInstance(a, c) {
			t.Fatal("merged unequal metadata", c)
		}
	}
	bag = NewBag(2)
	a.Count = MaxStack - 1
	b.Count = 3
	if bag.AddStack(d, a) != 0 || bag.AddStack(d, b) != 0 || bag.At(0).Count != MaxStack || bag.At(1).Count != 2 || bag.At(1).UID != b.UID {
		t.Fatal("capacity split")
	}
}
func TestAstralRuneStackSocketConsumesOneAndReturns(t *testing.T) {
	defs, lookup := astralFixture()
	gear := NewStack(defs[999001], 1)
	gear.SocketCount = 2
	rune := NewStack(defs[FirstAstralRune], 3)
	bag := NewBag(4)
	bag.Set(0, gear)
	bag.Set(1, rune)
	p, err := PlanAstralInsert(bag, NewEquipSet(), lookup, gear.UID, rune.UID, 0, &Character{Level: 100})
	if err != nil {
		t.Fatal(err)
	}
	if bag.At(1) != rune || p.Bag.At(1).Count != 2 || p.Bag.At(1).UID != rune.UID || p.Target.SocketUIDs[0] == rune.UID || p.Rune.Count != 1 || p.Target.SocketCards[0] != rune.Card {
		t.Fatal("socket consumed stack or copied identity", p)
	}
	out, err := PlanAstralRemove(p.Bag, p.Worn, lookup, gear.UID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bag.At(1).Count != 3 || out.Bag.At(1).UID != rune.UID || out.Target.SocketUIDs[0] != 0 {
		t.Fatal("return failed to merge", out)
	}
}
func TestAstralRuneStackCubeSingleUnit(t *testing.T) {
	for _, id := range []int32{CubeSalvageRune, CubeInscribeArray, CubeConvertRuneBase + 1} {
		ctx, req := cubeInputs(id)
		for i := 110; i < 113; i++ {
			st := ctx.Bag.At(i)
			if !st.Empty() {
				st.Count = 3
				ctx.Bag.Set(i, st)
			}
		}
		ctx.NewUID = NewItemInstanceID
		plan, err := PlanCube(ctx, req, true)
		if err != nil {
			t.Fatal(id, err)
		}
		if plan.Bag.At(110).Count != 2 || plan.Bag.At(110).UID != 501 || ctx.Bag.At(110).Count != 3 {
			t.Fatal("cube consumed whole stack", id, plan.Bag.At(110))
		}
		if len(plan.ConsumedUIDs) != 0 {
			t.Fatal("live stack tombstoned", id)
		}
		if id == CubeInscribeArray && (plan.Target.SocketUIDs[0] == 501 || plan.Target.SocketUIDs[1] == 502) {
			t.Fatal("array duplicated UID")
		}
	}
}
func TestAstralRuneStackRewardsMoveAndDuplicateUID(t *testing.T) {
	defs, _ := astralFixture()
	d := defs[FirstAstralRune]
	bag := NewBag(3)
	if bag.Add(d, MaxStack+4) != 0 || bag.At(0).Count != MaxStack || bag.At(1).Count != 4 || bag.At(0).UID == bag.At(1).UID {
		t.Fatal("reward stack capacity")
	}
	if left := bag.AddStack(d, bag.At(1)); left != 4 || !bag.At(2).Empty() {
		t.Fatal("duplicate owned UID accepted")
	}
	a, b := NewStack(d, 2), NewStack(d, 3)
	bag = NewBag(2)
	bag.Set(0, a)
	bag.Set(1, b)
	if !bag.Move(0, 1) || !bag.At(0).Empty() || bag.At(1).Count != 5 || bag.At(1).UID != b.UID {
		t.Fatal("drag merge")
	}
	bag = NewBag(1)
	a.Count = MaxStack - 1
	bag.Set(0, a)
	b.Count = 4
	if left := bag.AddStack(d, b); left != 3 || bag.At(0).Count != MaxStack || bag.At(0).UID != a.UID {
		t.Fatal("partial pickup")
	}
}
func TestAstralRuneStackCraftPreviewMergesWithoutAllocating(t *testing.T) {
	ctx, req := cubeInputs(CubeInscribeRuneBase + 1)
	r := cubeRune(ctx, FirstAstralRune, 501)
	r.Bound = true
	r.Card.Bound = true
	ctx.Bag.Set(110, r)
	for i := 100; i < 200; i++ {
		if ctx.Bag.At(i).Empty() {
			ctx.Bag.Set(i, Stack{Item: CubePurifiedDust, Count: MaxStack})
		}
	}
	allocated := 0
	ctx.NewUID = func() int64 { allocated++; return 999999 }
	preview, err := PlanCube(ctx, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if allocated != 0 || preview.Bag.At(110).Count != 2 || ctx.Bag.At(110).Count != 1 {
		t.Fatal("preview must merge without allocating or mutating", allocated)
	}
	result, err := PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if allocated != 1 || result.Bag.At(110).Count != 2 || result.Bag.At(110).UID != 501 {
		t.Fatal("craft didn't merge", allocated)
	}
}
