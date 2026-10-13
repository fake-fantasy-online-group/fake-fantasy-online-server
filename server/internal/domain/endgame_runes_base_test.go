package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Plain refers to innate extra properties, never the color derived from holes.
// Base/Bonus are the ov_arm intrinsic white-text stats. Fixed game_equip_extra
// and dropped RolledAffixes are separate properties; set/unique traits are also
// intrinsic even when a particular set bonus is not currently active.
func TestAstralBaseEligibilityUsesInnateProperties(t *testing.T) {
	cases := []struct {
		name string
		edit func(*ItemDef, *Stack)
		want bool
	}{
		{"plain", func(*ItemDef, *Stack) {}, true},
		{"intrinsic_stats", func(d *ItemDef, _ *Stack) {
			d.Equip.Bonus = Stats{MinAtk: 10, MaxAtk: 20, Def: 30, MAtk: 40, MDef: 50, Hit: 60, MaxWeight: 70}
			d.Equip.Base = Base{STR: 1, VIT: 2, INT: 3, SPI: 4, AGI: 5, DEX: 6}
		}, true},
		{"later_upgrades", func(d *ItemDef, st *Stack) {
			d.Equip.RefineEffects = map[int32]RefineEffect{3: {Bonuses: []Affix{{Attr: AttrAtk, Value: 12}}}}
			st.RefineLevel, st.WashQuality, st.WashCount = 3, 4, 1
			st.WashAffixes[0] = Affix{Attr: AttrSTR, Value: 8}
			st.FusedAppearance, st.FusedSoul, st.FusedDragon = 9101, 9102, 9103
			st.Endgame.Revision = 12
		}, true},
		{"empty_set_metadata", func(d *ItemDef, _ *Stack) { d.Equip.Suit = &EquipmentSuit{} }, true},
		{"unknown_source_property", func(d *ItemDef, _ *Stack) { d.Equip.UnknownInnateProperties = true }, false},
		{"fixed_affix", func(d *ItemDef, _ *Stack) { d.Equip.Affixes = []Affix{{Attr: AttrSTR, Value: 1}} }, false},
		{"zero_fixed_affix", func(d *ItemDef, _ *Stack) { d.Equip.Affixes = []Affix{{Attr: AttrSTR}} }, false},
		{"rolled_affix", func(_ *ItemDef, st *Stack) {
			st.RolledAffixCount = 1
			st.RolledAffixes[0] = InstanceAffix{CardID: 3, Affix: Affix{Attr: AttrSTR, Value: 1}}
		}, false},
		{"rolled_count_without_data", func(_ *ItemDef, st *Stack) { st.RolledAffixCount = 1 }, false},
		{"malformed_rolled_count", func(_ *ItemDef, st *Stack) { st.RolledAffixCount = 255 }, false},
		{"hidden_rolled_affix", func(_ *ItemDef, st *Stack) {
			st.RolledAffixes[4] = InstanceAffix{CardID: 3, Affix: Affix{Attr: AttrSTR, Value: 1}}
		}, false},
		{"unique_marker", func(_ *ItemDef, st *Stack) { st.Endgame.UniqueID = int32(st.Item) }, false},
		{"set_membership", func(d *ItemDef, _ *Stack) { d.Equip.Suit = &EquipmentSuit{ID: 5} }, false},
		{"set_groups", func(d *ItemDef, _ *Stack) { d.Equip.Suit = &EquipmentSuit{Groups: [][]ItemID{{d.ID}}} }, false},
		{"set_bonus", func(d *ItemDef, _ *Stack) {
			d.Equip.Suit = &EquipmentSuit{Bonuses: []EquipmentSuitBonus{{Pieces: 2, Affix: Affix{Attr: AttrSTR, Value: 1}}}}
		}, false},
	}
	recipe, _ := AstralRecipeByID(6, testEndgameCatalog())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defs, lookup := astralFixture()
			def := defs[999001]
			st := astralStack(t, recipe, def, lookup)
			tc.edit(&def, &st)
			defs[def.ID] = def
			original := st
			if got := AstralBaseEligible(def, st); got != tc.want {
				t.Fatalf("eligible=%v want %v", got, tc.want)
			}
			if _, got := MatchAstralRecipe(def, st); got != tc.want {
				t.Fatalf("matched=%v want %v", got, tc.want)
			}
			worn := NewEquipSet()
			worn.Set(SlotWeapon, st)
			ch := &Character{Level: 60, Race: Warrior}
			if got := len(ActiveAstralRecipes(worn, lookup, ch)) == 1; got != tc.want {
				t.Fatalf("active=%v want %v", got, tc.want)
			}
			bonuses := AstralBonuses(worn, lookup, ch)
			if tc.want && !reflect.DeepEqual(bonuses, AstralItemBonuses(def, st, recipe)) || !tc.want && len(bonuses) != 0 {
				t.Fatalf("unexpected formation bonuses: %+v", bonuses)
			}
			if st != original || worn.At(SlotWeapon) != original {
				t.Fatal("eligibility evaluation rewrote saved equipment or runes")
			}
			// The rule is derived from existing template/instance fields, so a
			// serialized reload must not restore an old formation activation.
			serialized, err := json.Marshal(st)
			if err != nil {
				t.Fatal(err)
			}
			var restored Stack
			if err := json.Unmarshal(serialized, &restored); err != nil || restored != original {
				t.Fatal("saved equipment did not roundtrip exactly", err)
			}
			worn = NewEquipSet()
			worn.Set(SlotWeapon, restored)
			if got := len(ActiveAstralRecipes(worn, lookup, ch)) == 1; got != tc.want {
				t.Fatalf("serialized reload active=%v want %v", got, tc.want)
			}
			if tc.want && EquipmentAttributeCount(def, st) != len(recipe.Runes) {
				t.Fatal("eligible socketed base should retain its nonzero display-property count")
			}
		})
	}
	if AstralBaseEligible(ItemDef{}, Stack{}) {
		t.Fatal("non-equipment accepted")
	}
}

func TestAstralAffixedGearKeepsStandaloneRunesAndExtraction(t *testing.T) {
	defs, lookup := astralFixture()
	def := defs[999001]
	def.Equip.Affixes = []Affix{{Attr: AttrSTR, Value: 5}}
	defs[def.ID] = def
	recipe, _ := AstralRecipeByID(6, testEndgameCatalog()) // Rune levels 12/14; formation and standalone sockets share this gate.
	st := astralStack(t, recipe, def, lookup)
	st.InstanceKind = ItemInstanceNone // Persisted legacy equipment is not rewritten.
	st.RefineLevel, st.WashCount, st.WashQuality = 3, 1, 2
	st.WashAffixes[0] = Affix{Attr: AttrVIT, Value: 7}
	st.FusedAppearance, st.FusedSoul, st.FusedDragon = 9101, 9102, 9103
	st.SocketCards[0].Effects[0].Value = 17
	st.SocketCards[0].Bound = true
	original := st
	worn := NewEquipSet()
	worn.Set(SlotWeapon, st)
	ch := &Character{Level: 14, Race: Warrior}
	if got := EquipmentRequiredLevel(def, st); got != 14 {
		t.Fatalf("ineligible formation retained a level requirement: %d", got)
	}
	if bonuses := AstralBonuses(worn, lookup, ch); len(bonuses) != 0 {
		t.Fatal("affixed gear gained formation stats", bonuses)
	}
	src := StatSource{Level: 14, Base: Base{STR: 50, VIT: 50}, Worn: worn, Defs: lookup}
	baseWithRunes, withRunes := Compute(src)
	withoutRunes := st
	withoutRunes.Sockets, withoutRunes.SocketUIDs, withoutRunes.SocketCards = [5]ItemID{}, [5]int64{}, [5]CardInstanceState{}
	src.Worn = NewEquipSet()
	src.Worn.Set(SlotWeapon, withoutRunes)
	baseWithout, without := Compute(src)
	if baseWithRunes.STR-baseWithout.STR != 17 || baseWithRunes.VIT-baseWithout.VIT != 3 || withRunes.MaxHP <= without.MaxHP {
		t.Fatal("ineligible formation lost its standalone saved rune effects")
	}
	bag := NewBag(3)
	plan, err := PlanAstralRemove(bag, worn, lookup, st.UID, 0)
	if err != nil {
		t.Fatal("old affixed equipment must still allow extraction", err)
	}
	if plan.Rune.UID != original.SocketUIDs[0] || plan.Rune.Card != original.SocketCards[0] || !plan.Rune.Bound {
		t.Fatal("extraction recreated or changed the saved card")
	}
	expected := original
	expected.InstanceKind = ItemInstanceEquipment
	expected.Sockets[0], expected.SocketUIDs[0], expected.SocketCards[0] = 0, 0, CardInstanceState{}
	if plan.Target != expected || worn.At(SlotWeapon) != original || !bag.At(0).Empty() {
		t.Fatal("extraction changed unrelated gear state or input inventories")
	}
	// Reinsert at the rune level: an ineligible base keeps standalone effects
	// without gaining formation bonuses.
	insert, err := PlanAstralInsert(plan.Bag, plan.Worn, lookup, st.UID, plan.Rune.UID, 0, ch)
	if err != nil {
		t.Fatal("standalone insertion was blocked by the formation-only rule", err)
	}
	expected = original
	expected.InstanceKind, expected.Bound = ItemInstanceEquipment, true
	if insert.Target != expected {
		t.Fatal("standalone insertion changed unrelated gear state")
	}
	if _, ok := MatchAstralRecipe(def, insert.Target); ok {
		t.Fatal("standalone insertion bypassed the base rule")
	}
}

func TestAstralRandomAffixOpportunityIsNotAnInstanceAffix(t *testing.T) {
	defs, lookup := astralFixture()
	def := defs[999001]
	def.Equip.ResourceLevel, def.Equip.Type = 10, 2
	recipe, _ := AstralRecipeByID(6, testEndgameCatalog())
	st := astralStack(t, recipe, def, lookup)
	pool := EquipmentRollTable{
		Counts: map[ColorProfile][]WeightedAffixCount{DefaultColorProfile: {{Count: 0, Weight: 1}, {Count: 1, Weight: 1}}},
		Options: map[EquipmentRollKey][]EquipmentRollOption{
			{Level: 10, Type: 2}: {{CardID: 7, Affix: Affix{Attr: AttrSTR, Value: 5}, Weight: 1}},
		},
	}
	for _, wantCount := range []int{0, 1} {
		rolled := pool.RollWithReservedSockets(def, DefaultColorProfile, st.SocketCount, func(n int64) int64 {
			if wantCount == 0 {
				return 0
			}
			return n - 1
		})
		if len(rolled) != wantCount {
			t.Fatalf("rolled %d, want %d", len(rolled), wantCount)
		}
		instance := st
		instance.RolledAffixCount = uint8(len(rolled))
		copy(instance.RolledAffixes[:], rolled)
		if _, matched := MatchAstralRecipe(def, instance); matched != (wantCount == 0) {
			t.Fatalf("random pool opportunity confused with actual instance affixes: count%d match%v", wantCount, matched)
		}
	}
}

func TestAstralCubeBaseRuleIsAtomicAndExtractionRemainsAvailable(t *testing.T) {
	for _, kind := range []string{"fixed", "rolled", "unique", "set", "hidden_rolled", "unknown"} {
		for _, recipe := range []int32{CubeInscribeArray, CubeReorderRunes, CubeExtractRunes} {
			for _, execute := range []bool{false, true} {
				ctx, req := cubeInputs(recipe)
				st := ctx.Bag.At(200)
				def := ctx.Items[st.Item]
				switch kind {
				case "fixed":
					def.Equip.Affixes = []Affix{{Attr: AttrSTR, Value: 5}}
				case "rolled":
					st.RolledAffixCount = 1
					st.RolledAffixes[0] = InstanceAffix{CardID: 3, Affix: Affix{Attr: AttrSTR, Value: 5}}
				case "unique":
					st.Endgame = EndgameItemState{UniqueID: int32(st.Item), UniqueRollBP: 10000, Revision: 7}
				case "set":
					def.Equip.Suit = &EquipmentSuit{ID: 1}
				case "hidden_rolled":
					st.RolledAffixes[4] = InstanceAffix{CardID: 3}
				case "unknown":
					def.Equip.UnknownInnateProperties = true
				}
				ctx.Items[st.Item] = def
				ctx.Bag.Set(200, st)
				before := ctx.Bag.Clone()
				plan, err := PlanCube(ctx, req, execute)
				if recipe != CubeExtractRunes {
					wantReason := "天生无附加词条"
					if kind == "unknown" {
						wantReason = "天生属性尚未确认"
					}
					if err == nil || CubeErrorCode(err) != "requirements" || !strings.Contains(err.Error(), wantReason) {
						t.Fatalf("%s recipe%d execute%v: %v", kind, recipe, execute, err)
					}
				} else {
					if err != nil {
						t.Fatalf("%s extraction execute%v failed: %v", kind, execute, err)
					}
					if len(plan.Outputs) != int(st.SocketCount) {
						t.Fatal("extraction did not return every saved card")
					}
					for i, card := range plan.Outputs {
						if card.UID != st.SocketUIDs[i] || card.Item != st.Sockets[i] || card.Card != st.SocketCards[i] {
							t.Fatal("cube extraction regenerated a saved rune")
						}
					}
					expected := st
					expected.Sockets, expected.SocketUIDs, expected.SocketCards = [5]ItemID{}, [5]int64{}, [5]CardInstanceState{}
					if execute {
						expected.Endgame.Revision++
					}
					if plan.Target != expected {
						t.Fatal("cube extraction changed unrelated equipment state")
					}
				}
				if !reflect.DeepEqual(before, ctx.Bag) {
					t.Fatal("cube eligibility preview/commit mutated its source inventory")
				}
			}
		}
	}
}
