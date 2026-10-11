package domain

import (
	"reflect"
	"strings"
	"testing"
)

func validFiveExtraAffixes() []Affix {
	return []Affix{{Attr: AttrSTR, Value: 20}, {Attr: AttrVIT, Value: 20},
		{Attr: AttrHit, Value: 20}, {Attr: AttrAtkSpeed, Value: -10, Mode: ModePercent},
		{Attr: AttrMaxHP, Value: 100}}
}

func TestExtraAffixContractRejectsPaddingAndInactiveValues(t *testing.T) {
	for _, unique := range []bool{false, true} {
		good := validFiveExtraAffixes()
		if err := validateEndgameExtraAffixes(good, unique); err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name string
			rows []Affix
			want string
		}{
			{"four", good[:4], "at least five beneficial"},
			{"curse_is_not_fifth", append(append([]Affix{}, good[:4]...), Affix{Attr: AttrMaxHP, Value: -100}), "at least five beneficial"},
			{"same_attr", append(append([]Affix{}, good...), good[0]), "duplicate extra Attr"},
			{"same_attr_other_mode", append(append([]Affix{}, good...), Affix{Attr: AttrSTR, Value: 20, Mode: ModePercent}), "duplicate extra Attr"},
			{"attack_percent_alias", append(append([]Affix{}, good...), Affix{Attr: AttrMinAtk, Value: 20, Mode: ModePercent}, Affix{Attr: AttrMaxAtk, Value: 20, Mode: ModePercent}), "duplicate percentage attack"},
		}
		for _, tc := range cases {
			t.Run(tc.name+map[bool]string{false: "/formation", true: "/unique"}[unique], func(t *testing.T) {
				if err := validateEndgameExtraAffixes(tc.rows, unique); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted %q, got %v", tc.want, err)
				}
			})
		}
		for _, bad := range []Affix{{Attr: AttrHPRegen, Value: 20}, {Attr: AttrMaxWeight, Value: 20},
			{Attr: AttrCrit, Value: 20, Mode: ModePercent}, {Attr: AttrMCrit, Value: 20, Mode: ModePercent},
			{Attr: AttrAtkSpeed, Value: -20}, {Attr: AttrSTR}, {Attr: AttrSTR, Value: 20, Mode: 2},
			{Attr: 999, Value: 20}, {Attr: AttrPhysRes, Value: 20, Mode: ModePercent},
			{Attr: AttrMagicRes, Value: 20, Mode: ModePercent}, {Attr: AttrStatusRes, Value: 20, Mode: ModePercent}} {
			rows := validFiveExtraAffixes()
			rows[0] = bad
			if err := validateEndgameExtraAffixes(rows, unique); err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("inactive affix accepted or wrong failure: %+v: %v", bad, err)
			}
		}
	}
}

func TestUniqueExtraAffixesStayNonzeroAndInt32SafeAtEveryPotency(t *testing.T) {
	for _, value := range []int32{1, -1, 2147483647, -2147483648} {
		rows := validFiveExtraAffixes()
		rows[0].Value = value
		if err := validateEndgameExtraAffixes(rows, true); err == nil {
			t.Fatal("invalid scaled unique value accepted", value)
		}
	}
	for _, d := range UniqueCatalog() {
		for _, roll := range []int32{UniqueRollMinBP, 10000, UniqueRollMaxBP} {
			st := Stack{UID: 9007199254740993, Item: d.BaseID, Count: 1,
				Endgame: EndgameItemState{UniqueID: d.ID, UniqueRollBP: roll}}
			before := st
			rows := UniqueAffixes(st)
			benefits := 0
			for _, a := range rows {
				if a.Value == 0 {
					t.Fatalf("unique %d has vanishing line at %d: %+v", d.ID, roll, a)
				}
				if BeneficialEndgameAffix(a) {
					benefits++
				}
			}
			if benefits < MinBeneficialEndgameExtraAffixes || len(rows) != len(d.Affixes) || st != before {
				t.Fatalf("unique %d lost extras or rewrote saved state at %d", d.ID, roll)
			}
		}
	}
}

func TestUniqueLoaderCannotCountTraitsReservedLevelsOrNativeStats(t *testing.T) {
	d := UniqueCatalog()[0]
	d.PowerTier = "transition"
	d.Affixes = validFiveExtraAffixes()[:4]
	d.Traits = []UniqueTrait{{Kind: "basic_focus", PowerBP: 2000}}
	d.ReservedSkills = []ReservedSkillAttribute{{Scope: "all", Levels: 3}}
	if err := validateUniqueDefinition(d); err == nil || !strings.Contains(err.Error(), "at least five beneficial") {
		t.Fatal("other effect sources padded missing unique extra", err)
	}
	for _, tier := range []string{"niche", "specialist", "chase", ""} {
		d.PowerTier = tier
		d.Affixes = validFiveExtraAffixes()
		if validateUniqueDefinition(d) == nil {
			t.Fatal("legacy tier accepted", tier)
		}
	}
}

func TestAstralRuntimeGateCannotCountRunesTraitsOrReservedLevels(t *testing.T) {
	original := cloneAstralRecipe(astralRecipeCatalog[0])
	t.Cleanup(func() { astralRecipeCatalog[0] = original })
	for _, mutate := range []func(*AstralRecipe){
		func(r *AstralRecipe) {
			r.Affixes = validFiveExtraAffixes()[:4]
			r.Traits = []UniqueTrait{{Kind: "basic_focus", PowerBP: 2000}}
			r.ReservedSkills = []ReservedSkillAttribute{{Scope: "all", Levels: 3}}
		},
		func(r *AstralRecipe) {
			r.Affixes = append(validFiveExtraAffixes(), Affix{Attr: AttrSTR, Value: 20, Mode: ModePercent})
		},
		func(r *AstralRecipe) { r.Affixes[0] = Affix{Attr: AttrHPRegen, Value: 20} },
		func(r *AstralRecipe) { r.PowerTier = "niche" },
	} {
		astralRecipeCatalog[0] = cloneAstralRecipe(original)
		mutate(&astralRecipeCatalog[0])
		if err := ValidateAstralCatalog(); err == nil {
			t.Fatal("invalid formation extras accepted")
		}
	}
	astralRecipeCatalog[0] = original
	if err := ValidateAstralCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestExtendedExtraAffixSlicesDoNotTruncateOrRewriteInstances(t *testing.T) {
	d := UniqueCatalog()[0]
	original := uniqueByBase[d.BaseID]
	t.Cleanup(func() { uniqueByBase[d.BaseID] = original })
	d.VariableAffixes = nil
	d.LocalEnhancements = nil
	d.Affixes = append(validFiveExtraAffixes(), Affix{Attr: AttrAGI, Value: 10},
		Affix{Attr: AttrDEX, Value: 12}, Affix{Attr: AttrAtk, Value: 30, Mode: ModePercent},
		Affix{Attr: AttrMDef, Value: -8}, Affix{Attr: AttrMaxMP, Value: -10})
	d.PowerTier = "godlike"
	if err := validateUniqueDefinition(d); err != nil {
		t.Fatal(err)
	}
	uniqueByBase[d.BaseID] = d
	st := Stack{UID: 9007199254740993, Item: d.BaseID, Count: 1, RolledAffixCount: 1,
		Durability: 100, MaxDurability: 100,
		RolledAffixes: [5]InstanceAffix{{CardID: 17, Affix: Affix{Attr: AttrSTR, Value: 7}}},
		SocketCount:   1, Sockets: [5]ItemID{FirstAstralRune}, SocketUIDs: [5]int64{9007199254740995},
		Endgame: EndgameItemState{UniqueID: d.ID, UniqueRollBP: 10123, Revision: 3}}
	before := st
	got := UniqueAffixes(st)
	want := append([]Affix{}, d.Affixes...)
	want = ResolveVariableAffixes(want, d.VariableAffixes, st.Endgame.UniqueRollBP)
	if len(got) != 10 || !reflect.DeepEqual(got, want) || st != before {
		t.Fatal("extended definition was truncated or persisted instance changed")
	}
	def := uniqueFixture(d)
	worn := NewEquipSet()
	worn.Set(EquipSlot(d.Slot), st)
	base := Base{STR: 100, INT: 100, VIT: 100, AGI: 100, DEX: 100, SPI: 100}
	actualBase, actualStats := Compute(StatSource{Base: base, Level: 1000, Worn: worn,
		Defs: func(id ItemID) (ItemDef, bool) { return def, id == def.ID }})
	expectedBase, expectedStats := Compute(StatSource{Base: base, Level: 1000,
		Passive: append(append([]Affix{}, want...), st.RolledAffixes[0].Affix)})
	if actualBase != expectedBase || actualStats != expectedStats || worn.At(EquipSlot(d.Slot)) != before {
		t.Fatal("Compute lost extended extra lines, failed retained-roll stacking or changed saved state")
	}
}

func TestAstralRuneSnapshotCapacityStillBoundsOnlyRunes(t *testing.T) {
	original := astralRuneCatalog[0]
	t.Cleanup(func() { astralRuneCatalog[0] = original })
	astralRuneCatalog[0].Affixes = make([]Affix, len((CardInstanceState{}).Effects)+1)
	for i := range astralRuneCatalog[0].Affixes {
		astralRuneCatalog[0].Affixes[i] = Affix{Attr: AttrSTR, Value: 20}
	}
	if err := ValidateAstralCatalog(); err == nil {
		t.Fatal("oversized persisted rune snapshot accepted")
	}
}
