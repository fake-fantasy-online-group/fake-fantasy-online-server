package domain

import (
	"reflect"
	"testing"
)

func astralFixture() (map[ItemID]ItemDef, func(ItemID) (ItemDef, bool)) {
	defs := map[ItemID]ItemDef{}
	for _, d := range AstralRuneItems(7) {
		defs[d.ID] = d
	}
	defs[999001] = ItemDef{ID: 999001, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{Slot: int32(SlotWeapon), LevelReq: 50, Durable: 10}}
	return defs, func(id ItemID) (ItemDef, bool) { d, ok := defs[id]; return d, ok }
}
func astralStack(t *testing.T, r AstralRecipe, def ItemDef, lookup func(ItemID) (ItemDef, bool)) Stack {
	t.Helper()
	st := NewStack(def, 1)
	st.SocketCount = uint8(len(r.Runes))
	for i, id := range r.Runes {
		rd, ok := lookup(id)
		if !ok {
			t.Fatal(id)
		}
		c := NewStack(rd, 1)
		st.Sockets[i] = id
		st.SocketUIDs[i] = c.UID
		st.SocketCards[i] = c.Card
	}
	return st
}
func TestAstralCatalogCompleteImmutable(t *testing.T) {
	if err := ValidateAstralCatalog(); err != nil {
		t.Fatal(err)
	}
	runes, recipes := AstralRunes(), AstralRecipes()
	if len(runes) != 28 || len(recipes) != 40 {
		t.Fatal("coverage")
	}
	var lengths [6]int
	for _, r := range recipes {
		lengths[len(r.Runes)]++
	}
	for n := 2; n <= 5; n++ {
		if lengths[n] != 10 {
			t.Fatal("length coverage", n)
		}
	}
	for _, d := range AstralRuneItems(7) {
		st := NewStack(d, 1)
		if st.UID <= 0 || st.Count != 1 || !st.Card.Initialized || d.Stackable || len(st.Card.StaticAffixes()) != 1 {
			t.Fatal("invalid card", d.ID)
		}
	}
	runes[0].Affixes[0].Value = 9999
	recipes[0].Runes[0] = 0
	recipes[0].Affixes[0].Value = 9999
	if AstralRunes()[0].Affixes[0].Value == 9999 || AstralRecipes()[0].Runes[0] == 0 || AstralRecipes()[0].Affixes[0].Value == 9999 {
		t.Fatal("mutable catalog escaped")
	}
}
func TestAstralEveryRecipeAndInvalidOrders(t *testing.T) {
	defs, lookup := astralFixture()
	for _, r := range AstralRecipes() {
		t.Run(r.Name, func(t *testing.T) {
			def := defs[999001]
			eq := *def.Equip
			eq.Slot = int32(r.Slots[0])
			def.Equip = &eq
			defs[def.ID] = def
			st := astralStack(t, r, def, lookup)
			if got, ok := MatchAstralRecipe(def, st); !ok || got.ID != r.ID {
				t.Fatal("exact match failed")
			}
			variants := []Stack{st, st, st, st}
			variants[0].Sockets[0], variants[0].Sockets[1] = variants[0].Sockets[1], variants[0].Sockets[0]
			variants[1].Sockets[0] = 0
			variants[1].SocketUIDs[0] = 0
			variants[2].SocketUIDs[1] = variants[2].SocketUIDs[0]
			variants[3].SocketCards[0].Initialized = false
			for i, v := range variants {
				if _, ok := MatchAstralRecipe(def, v); ok {
					t.Fatal("invalid accepted", i)
				}
			}
			if st.SocketCount < 5 {
				v := st
				v.SocketCount++
				if _, ok := MatchAstralRecipe(def, v); ok {
					t.Fatal("prefix accepted")
				}
			}
			worn := NewEquipSet()
			worn.Set(r.Slots[0], st)
			ch := &Character{Level: 60, EmploymentKnown: true, Employed: true}
			for race := Warrior; race <= Warlock; race++ {
				if r.Professions == 0 || r.Professions&(1<<uint8(race)) != 0 {
					ch.Race = race
					break
				}
			}
			if a := ActiveAstralRecipes(worn, lookup, ch); len(a) != 1 || a[0].Recipe.ID != r.ID || !reflect.DeepEqual(AstralBonuses(worn, lookup, ch), r.Affixes) {
				t.Fatal("no executable bonuses")
			}
			ch.Level = r.MinLevel - 1
			if len(ActiveAstralRecipes(worn, lookup, ch)) != 0 {
				t.Fatal("low level active")
			}
			ch.Level = 60
			broken := st
			broken.Durability = 0
			broken.MaxDurability = 10
			worn.Set(r.Slots[0], broken)
			if len(ActiveAstralRecipes(worn, lookup, ch)) != 0 {
				t.Fatal("broken active")
			}
			st.InstanceKind = ItemInstanceNone
			if _, ok := MatchAstralRecipe(def, st); !ok {
				t.Fatal("legacy hydration broke array")
			}
		})
	}
}
func TestAstralChannelAndProfession(t *testing.T) {
	defs, lookup := astralFixture()
	low, _ := AstralRecipeByID(3)
	high, _ := AstralRecipeByID(36)
	def := defs[999001]
	eq := *def.Equip
	eq.Slot = int32(SlotHead)
	def.Equip = &eq
	defs[def.ID] = def
	body := def
	body.ID = 999002
	eq2 := *def.Equip
	eq2.Slot = int32(SlotBody)
	body.Equip = &eq2
	defs[body.ID] = body
	worn := NewEquipSet()
	worn.Set(SlotHead, astralStack(t, low, def, lookup))
	worn.Set(SlotBody, astralStack(t, high, body, lookup))
	ch := &Character{Level: 60, Race: Warrior}
	a := ActiveAstralRecipes(worn, lookup, ch)
	if len(a) != 1 || a[0].Recipe.ID != 36 {
		t.Fatal("wrong winner", a)
	}
	worn.Set(SlotBody, Stack{})
	if a = ActiveAstralRecipes(worn, lookup, ch); len(a) != 1 || a[0].Recipe.ID != 3 {
		t.Fatal("fallback failed")
	}
	r, _ := AstralRecipeByID(6)
	eq3 := *def.Equip
	eq3.Slot = int32(SlotWeapon)
	def.Equip = &eq3
	defs[def.ID] = def
	worn = NewEquipSet()
	worn.Set(SlotWeapon, astralStack(t, r, def, lookup))
	ch.Race = Healer
	if len(ActiveAstralRecipes(worn, lookup, ch)) != 0 {
		t.Fatal("wrong class")
	}
	ch.Race = Warrior
	ch.EmploymentKnown = true
	ch.Employed = false
	if len(ActiveAstralRecipes(worn, lookup, ch)) != 0 {
		t.Fatal("beginner")
	}
}
func TestAstralInsertRemoveIdentityIsolationAndLegacy(t *testing.T) {
	defs, lookup := astralFixture()
	target := NewStack(defs[999001], 1)
	target.SocketCount = 2
	target.InstanceKind = ItemInstanceNone
	rd, _ := lookup(FirstAstralRune)
	rune := NewStack(rd, 1)
	rune.Bound = true
	rune.Card.Effects[0].Value = 17
	bag := NewBag(4)
	bag.Set(0, target)
	bag.Set(1, rune)
	p, err := PlanAstralInsert(bag, NewEquipSet(), lookup, target.UID, rune.UID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if bag.At(1).UID != rune.UID || bag.At(0).Sockets[0] != 0 || bag.At(0).InstanceKind != ItemInstanceNone {
		t.Fatal("original changed")
	}
	if !p.Target.Bound || p.Target.SocketUIDs[0] != rune.UID || p.Target.SocketCards[0].Effects[0].Value != 17 || p.Target.InstanceKind != ItemInstanceEquipment {
		t.Fatal("identity/binding changed")
	}
	out, err := PlanAstralRemove(p.Bag, p.Worn, lookup, target.UID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Rune.UID != rune.UID || out.Rune.Card.Effects[0].Value != 17 || !out.Rune.Bound || out.Target.Sockets[0] != 0 || p.Target.Sockets[0] == 0 {
		t.Fatal("extraction lost saved state")
	}
}
func TestAstralRejectWithoutLoss(t *testing.T) {
	defs, lookup := astralFixture()
	st := NewStack(defs[999001], 1)
	st.SocketCount = 2
	rd, _ := lookup(FirstAstralRune)
	r := NewStack(rd, 1)
	bag := NewBag(3)
	bag.Set(0, st)
	bag.Set(1, r)
	worn := NewEquipSet()
	assert := func(err error, want string) {
		t.Helper()
		if err == nil || err.Error() != want {
			t.Fatalf("got %v want %s", err, want)
		}
	}
	_, err := PlanAstralInsert(bag, worn, lookup, st.UID, r.UID, 2)
	assert(err, "invalid_socket")
	locked := r
	locked.Locked = true
	bag.Set(1, locked)
	_, err = PlanAstralInsert(bag, worn, lookup, st.UID, r.UID, 0)
	assert(err, "locked")
	bag.Set(1, r)
	bag.Set(2, r)
	_, err = PlanAstralInsert(bag, worn, lookup, st.UID, r.UID, 0)
	assert(err, "invalid_rune")
	bag.Set(2, Stack{})
	p, err := PlanAstralInsert(bag, worn, lookup, st.UID, r.UID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < p.Bag.Cap(); i++ {
		p.Bag.Set(i, Stack{Item: 888, Count: 1})
	}
	_, err = PlanAstralRemove(p.Bag, p.Worn, lookup, st.UID, 0)
	assert(err, "bag_full")
	if p.Bag.At(0).SocketUIDs[0] != r.UID {
		t.Fatal("failed extract changed state")
	}
}
func TestAstralSocketDropBudget(t *testing.T) {
	defs, _ := astralFixture()
	d := defs[999001]
	for grade := AstralDropOrdinary; grade <= AstralDropRealm; grade++ {
		for roll := int64(0); roll < 100; roll++ {
			if n := RollDroppedSocketCount(d, grade, func(int64) int64 { return roll }); n > 5 {
				t.Fatal(n)
			}
		}
	}
	if n := RollDroppedSocketCount(d, AstralDropRealm, func(int64) int64 { return 99 }); n != 5 {
		t.Fatal(n)
	}
	d.Equip.Affixes = []Affix{{Attr: AttrSTR}, {Attr: AttrINT}, {Attr: AttrVIT}}
	if n := RollDroppedSocketCount(d, AstralDropRealm, func(int64) int64 { return 99 }); n != 2 {
		t.Fatal(n)
	}
	d.Equip.Affixes = nil
	d.Equip.LevelReq = 10
	if n := RollDroppedSocketCount(d, AstralDropRealm, func(int64) int64 { return 99 }); n != 2 {
		t.Fatal(n)
	}
	table := EquipmentRollTable{Counts: map[ColorProfile][]WeightedAffixCount{1: {{Count: 5, Weight: 1}}}, Options: map[EquipmentRollKey][]EquipmentRollOption{{}: {{Affix: Affix{Attr: AttrSTR, Value: 2}, Weight: 1}, {Affix: Affix{Attr: AttrINT, Value: 2}, Weight: 1}, {Affix: Affix{Attr: AttrVIT, Value: 2}, Weight: 1}, {Affix: Affix{Attr: AttrAGI, Value: 2}, Weight: 1}, {Affix: Affix{Attr: AttrDEX, Value: 2}, Weight: 1}}}}
	for holes := uint8(0); holes <= 5; holes++ {
		if n := len(table.RollWithReservedSockets(d, 1, holes, func(int64) int64 { return 0 })); n != 5-int(holes) {
			t.Fatal(holes, n)
		}
	}
}
