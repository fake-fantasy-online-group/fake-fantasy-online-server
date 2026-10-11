package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestItemLocalEnhancementUsesOnlyUnrefinedItemBase(t *testing.T) {
	e := &EquipDef{Bonus: Stats{MinAtk: 11, MaxAtk: 23, MAtk: 17, Def: 7, MDef: 9}, RefineEffects: map[int32]RefineEffect{3: {Bonuses: []Affix{{Attr: AttrMinAtk, Value: 100}, {Attr: AttrMaxAtk, Value: 100}}}}}
	local := []ItemLocalEnhancement{{Attr: AttrAtk, Percent: 150}, {Attr: AttrMDef, Percent: 125}}
	want := []Affix{{Attr: AttrMinAtk, Value: 16}, {Attr: AttrMaxAtk, Value: 34}, {Attr: AttrMDef, Value: 11}}
	if got := ResolveItemLocalEnhancements(e, local, 10000); !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	_, stats := EnhancedIntrinsic(e, 3, local, 10000)
	if stats.MinAtk != 127 || stats.MaxAtk != 157 || stats.MDef != 20 || e.Bonus.MinAtk != 11 {
		t.Fatal("refinement compounded, base mutated, or local rounding wrong", stats)
	}
	if got := ResolveItemLocalEnhancements(&EquipDef{}, local, 10000); len(got) != 0 {
		t.Fatal("zero-base enhancement created stats", got)
	}
}
func TestExplicitVariablesLeaveAllOtherAffixesAndTraitsFixed(t *testing.T) {
	d := UniqueCatalog()[0]
	old := uniqueByBase[d.BaseID]
	t.Cleanup(func() { uniqueByBase[d.BaseID] = old })
	d.Affixes = validFiveExtraAffixes()
	d.VariableAffixes = []VariableAffix{{Attr: AttrSTR, Min: 10, Max: 30}}
	d.LocalEnhancements = []ItemLocalEnhancement{{Attr: AttrAtk, Percent: 150, MinPercent: 100, MaxPercent: 200}}
	d.Traits = []UniqueTrait{{Kind: "basic_focus", PowerBP: 1200}}
	uniqueByBase[d.BaseID] = d
	def := uniqueFixture(d)
	def.Equip.Bonus = Stats{MinAtk: 10, MaxAtk: 20}
	st := Stack{UID: 55, Item: d.BaseID, Count: 1, Durability: 100, MaxDurability: 100, Endgame: EndgameItemState{UniqueID: d.ID, UniqueRollBP: 8000}}
	lo := UniqueItemBonuses(def, st)
	before := st
	st.Endgame.UniqueRollBP = 12000
	hi := UniqueItemBonuses(def, st)
	if lo[0].Value != 10 || hi[0].Value != 30 {
		t.Fatal(lo, hi)
	}
	for i := 1; i < len(d.Affixes); i++ {
		if lo[i] != hi[i] || lo[i] != d.Affixes[i] {
			t.Fatal("fixed line scaled", i, lo, hi)
		}
	}
	if lo[len(lo)-2].Value != 10 || hi[len(hi)-2].Value != 20 {
		t.Fatal("local roll did not use item white", lo, hi)
	}
	worn := NewEquipSet()
	worn.Set(EquipSlot(d.Slot), st)
	ch := &Character{Level: 1000, EmploymentKnown: true, Employed: true}
	for race := Warrior; race <= Warlock; race++ {
		if d.Professions == 0 || d.Professions&(1<<uint8(race)) != 0 {
			ch.Race = race
			break
		}
	}
	got := ActiveUniqueTraits(worn, func(ItemID) (ItemDef, bool) { return def, true }, ch)
	if len(got) != 1 || got[0].PowerBP != 1200 {
		t.Fatal("fixed trait scaled", got)
	}
	st.Endgame.UniqueRollBP = before.Endgame.UniqueRollBP
	if st != before {
		t.Fatal("read rewrote saved item")
	}
}
func TestSharedQualityHasReachablePerfectPlateau(t *testing.T) {
	counts := map[int32]int{}
	for roll := UniqueRollMinBP; roll <= UniqueRollMaxBP; roll++ {
		counts[endgameRollBand(roll)]++
	}
	if len(counts) != 11 || counts[10] < 360 || counts[10] > 365 {
		t.Fatal("perfect probability is not approximately 1/11", counts)
	}
	v := VariableAffix{Attr: AttrAtkSpeed, Min: -30, Max: -10}
	if variableValue(v, 8000) != -10 || variableValue(v, 12000) != -30 || endgameRollBand(0) != 5 {
		t.Fatal("benefit direction or legacy midpoint wrong")
	}
}
func TestFormationRollCreatedOnceAndLegacyZeroPreserved(t *testing.T) {
	def := ItemDef{ID: 99001, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{Slot: int32(SlotWeapon)}}
	for i := 0; i < 100; i++ {
		st := NewStack(def, 1)
		if st.Endgame.FormationRollBP < 8000 || st.Endgame.FormationRollBP > 12000 {
			t.Fatal(st.Endgame)
		}
	}
	legacy := Stack{UID: 99, Item: def.ID, Count: 1}
	raw, _ := json.Marshal(legacy)
	var loaded Stack
	if err := json.Unmarshal(raw, &loaded); err != nil || loaded.Endgame.FormationRollBP != 0 {
		t.Fatal(loaded, err)
	}
	before := loaded
	_ = ResolveVariableAffixes([]Affix{{Attr: AttrSTR, Value: 20}}, []VariableAffix{{Attr: AttrSTR, Min: 10, Max: 30}}, loaded.Endgame.FormationRollBP)
	if loaded != before {
		t.Fatal("legacy read assigned a roll")
	}
	defs, lookup := astralFixture()
	r, _ := AstralRecipeByID(1)
	for _, roll := range []int32{0, 8000, 10417, 12000} {
		target := astralStack(t, r, defs[999001], lookup)
		target.Endgame.FormationRollBP = roll
		bag := NewBag(6)
		bag.Set(0, target)
		removed, err := PlanAstralRemove(bag, NewEquipSet(), lookup, target.UID, 0)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := PlanAstralInsert(removed.Bag, removed.Worn, lookup, target.UID, removed.Rune.UID, 0, &Character{Level: 100})
		if err != nil || restored.Target.Endgame.FormationRollBP != roll {
			t.Fatal("extraction/reinsertion rerolled", roll, restored.Target.Endgame, err)
		}
	}
}

func TestFormationLocalEnhancementUsesWinningItemOnly(t *testing.T) {
	original0, original1 := cloneAstralRecipe(astralRecipeCatalog[0]), cloneAstralRecipe(astralRecipeCatalog[1])
	t.Cleanup(func() { astralRecipeCatalog[0] = original0; astralRecipeCatalog[1] = original1 })
	for i, slot := range []EquipSlot{SlotWeapon, SlotTwoHand} {
		r := &astralRecipeCatalog[i]
		r.Slots = []EquipSlot{slot}
		r.Channel = AstralBattle
		r.Professions = 0
		r.VariableAffixes = nil
		r.LocalEnhancements = []ItemLocalEnhancement{{Attr: AttrAtk, Percent: 200}}
	}
	defs, lookup := astralFixture()
	d1 := defs[999001]
	e1 := *d1.Equip
	e1.Slot = int32(SlotWeapon)
	e1.Bonus = Stats{MinAtk: 10, MaxAtk: 20}
	d1.Equip = &e1
	defs[d1.ID] = d1
	d2 := d1
	d2.ID = 999002
	e2 := e1
	e2.Slot = int32(SlotTwoHand)
	e2.Bonus = Stats{MinAtk: 100, MaxAtk: 200}
	d2.Equip = &e2
	defs[d2.ID] = d2
	w := NewEquipSet()
	w.Set(SlotWeapon, astralStack(t, astralRecipeCatalog[0], d1, lookup))
	w.Set(SlotTwoHand, astralStack(t, astralRecipeCatalog[1], d2, lookup))
	ch := &Character{Level: 100, EmploymentKnown: true, Employed: true}
	active := ActiveAstralRecipes(w, lookup, ch)
	if len(active) != 1 || active[0].Recipe.ID != 1 {
		t.Fatal("unexpected channel winner", active)
	}
	bonuses := AstralBonuses(w, lookup, ch)
	var minAttack, maxAttack int32
	for _, a := range bonuses {
		if a.Mode == ModeAbsolute {
			if a.Attr == AttrMinAtk {
				minAttack += a.Value
			}
			if a.Attr == AttrMaxAtk {
				maxAttack += a.Value
			}
		}
	}
	// Recipes retain unrelated authored bonuses; compare the full exact package.
	want := AstralItemBonuses(d1, w.At(SlotWeapon), astralRecipeCatalog[0])
	if !reflect.DeepEqual(bonuses, want) {
		t.Fatal("suppressed item enhanced the winner", bonuses, want)
	}
	_ = minAttack
	_ = maxAttack
	broken := w.At(SlotWeapon)
	broken.Durability = 0
	w.Set(SlotWeapon, broken)
	active = ActiveAstralRecipes(w, lookup, ch)
	if len(active) != 1 || active[0].Recipe.ID != 2 || !reflect.DeepEqual(AstralBonuses(w, lookup, ch), AstralItemBonuses(d2, w.At(SlotTwoHand), astralRecipeCatalog[1])) {
		t.Fatal("broken winner did not yield exact other item local base", active)
	}
}
func TestRollModelRejectsInvalidRangesAndUnsupportedLocalScope(t *testing.T) {
	good := validFiveExtraAffixes()
	for _, local := range [][]ItemLocalEnhancement{{{Attr: AttrMaxHP, Percent: 100}}, {{Attr: AttrAtk, Percent: 100, MinPercent: 50, MaxPercent: 40}}, {{Attr: AttrAtk, Percent: 100}, {Attr: AttrAtk, Percent: 200}}} {
		if validateEndgameRollModel(good, nil, local) == nil {
			t.Fatal("invalid local model accepted", local)
		}
	}
	if validateEndgameRollModel(good, []VariableAffix{{Attr: AttrSTR, Min: 1, Max: 30}, {Attr: AttrVIT, Min: 1, Max: 30}}, []ItemLocalEnhancement{{Attr: AttrAtk, Percent: 100, MinPercent: 90, MaxPercent: 110}}) == nil {
		t.Fatal("third variable line accepted")
	}
}

func TestComputeLocalEnhancementCannotScaleCharacterOtherGearOrSnapshots(t *testing.T) {
	d := UniqueCatalog()[0]
	old := uniqueByBase[d.BaseID]
	t.Cleanup(func() { uniqueByBase[d.BaseID] = old })
	d.Affixes = validFiveExtraAffixes()
	d.VariableAffixes = nil
	d.LocalEnhancements = []ItemLocalEnhancement{{Attr: AttrAtk, Percent: 200}}
	def := uniqueFixture(d)
	def.Equip.Bonus = Stats{MinAtk: 10, MaxAtk: 20}
	def.Equip.RefineEffects = map[int32]RefineEffect{3: {Bonuses: []Affix{{Attr: AttrMinAtk, Value: 100}, {Attr: AttrMaxAtk, Value: 100}}}}
	st := Stack{UID: 55, Item: d.BaseID, Count: 1, Durability: 100, MaxDurability: 100, RefineLevel: 3, RolledAffixCount: 1, RolledAffixes: [5]InstanceAffix{{Affix: Affix{Attr: AttrAtk, Value: 500}}}, SocketCount: 1, Sockets: [5]ItemID{77}, SocketUIDs: [5]int64{78}, SocketCards: [5]CardInstanceState{{Initialized: true, Count: 1, Effects: [8]CardEffect{{Op: 1, Attr: AttrAtk, Value: 200, Probability: 100}}}}, Endgame: EndgameItemState{UniqueID: d.ID, UniqueRollBP: 10000}}
	other := ItemDef{ID: 999999, Equip: &EquipDef{Slot: int32(SlotHead), Bonus: Stats{MinAtk: 1000, MaxAtk: 2000}}}
	worn := NewEquipSet()
	worn.Set(EquipSlot(d.Slot), st)
	worn.Set(SlotHead, Stack{UID: 90, Item: other.ID, Count: 1})
	lookup := func(id ItemID) (ItemDef, bool) {
		if id == def.ID {
			return def, true
		}
		return other, id == other.ID
	}
	for _, strength := range []int32{1, 1000} {
		uniqueByBase[d.BaseID] = d
		_, with := Compute(StatSource{Base: Base{STR: strength}, Level: 1000, Worn: worn, Defs: lookup})
		noLocal := d
		noLocal.LocalEnhancements = nil
		uniqueByBase[d.BaseID] = noLocal
		_, without := Compute(StatSource{Base: Base{STR: strength}, Level: 1000, Worn: worn, Defs: lookup})
		if with.MinAtk-without.MinAtk != 20 || with.MaxAtk-without.MaxAtk != 40 {
			t.Fatal("local bonus leaked into unrelated source", strength, with, without)
		}
		if worn.At(EquipSlot(d.Slot)) != st {
			t.Fatal("Compute changed instance")
		}
	}
}
