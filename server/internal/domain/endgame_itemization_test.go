package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestAuthoredAffixModesActuallyChangeStats(t *testing.T) {
	attrs := []int32{1, 3, 5, 7, 9, 61, 13, 15, 17, 19, 23, 25, 27, 29, 31, 34, 36, 47, 66, 69, 70, 118}
	source := StatSource{Base: Base{STR: 100, INT: 100, VIT: 100, AGI: 100, DEX: 100, SPI: 100}, Innate: Stats{MoveSpeed: 100, AtkSpeedMS: 1000, MaxWeight: 100, PhysResist: 100, MagicResist: 100, StatusResist: 100}}
	base, stats := Compute(source)
	for _, attr := range attrs {
		for _, mode := range []int32{0, 1} {
			a := Affix{Attr: attr, Mode: mode, Value: 20}
			if !SupportedEndgameAffix(a) {
				if !((mode == 1 && (attr == 23 || attr == 66 || attr == 69 || attr == 70 || attr == 118)) || (mode == 0 && attr == 25)) {
					t.Fatal("unexpected unsupported", a)
				}
				continue
			}
			candidate := source
			candidate.Passive = []Affix{a}
			gotBase, gotStats := Compute(candidate)
			if reflect.DeepEqual(base, gotBase) && reflect.DeepEqual(stats, gotStats) {
				t.Fatal("inert authored effect", a)
			}
		}
	}
	for _, a := range []Affix{{Attr: 40, Value: 100}, {Attr: 40, Value: 10, Mode: 1}, {Attr: 25, Value: -100, Mode: 0}, {Attr: 38, Value: 1}, {Attr: 23, Value: 5, Mode: 1}, {Attr: 66, Value: 5, Mode: 1}, {Attr: 999, Value: 1}, {Attr: 1, Value: 1, Mode: 2}, {Attr: 1}} {
		if SupportedEndgameAffix(a) {
			t.Fatal("unsupported mode advertised", a)
		}
	}
}
func TestReservedSkillLevelsStayDefinitionOnly(t *testing.T) {
	if ReservedSkillHardPointCap != 20 {
		t.Fatal("hard point design drift")
	}
	for _, b := range []ReservedSkillAttribute{{Scope: "all", Levels: 1}, {Scope: "class", Target: "warlock", Levels: 2}, {Scope: "tree", Target: "ice", Levels: 3}, {Scope: "skill", Target: "10409", Levels: 1}} {
		if !b.Valid() {
			t.Fatal(b)
		}
	}
	for _, b := range []ReservedSkillAttribute{{Scope: "all", Target: "x", Levels: 1}, {Scope: "class", Levels: 1}, {Scope: "all", Levels: 20}, {Scope: "unknown", Levels: 1}} {
		if b.Valid() {
			t.Fatal(b)
		}
	}
	d := UniqueDefinition{Lore: "旧剑", Identity: "审计身份", Benefit: "设计用途", Tradeoff: "设计取舍", ReservedSkills: []ReservedSkillAttribute{{Scope: "all", Levels: 3}}}
	if d.Description() != "旧剑" {
		t.Fatal("inactive skill levels leaked into active tooltip", d.Description())
	}
	r := AstralRecipe{Lore: "星图", Identity: "审计身份", Benefit: "设计用途", Tradeoff: "设计取舍", Traits: []UniqueTrait{{Kind: "basic_focus", PowerBP: 1000}}, ReservedSkills: d.ReservedSkills}
	if r.Description() != "星图" {
		t.Fatal("inactive skill levels leaked into array tooltip")
	}
}
func TestSkillScopedTraitsFilterBeforeStrongestKind(t *testing.T) {
	a := UniqueTrait{Kind: "swift_cast", PowerBP: 2000, SkillID: 10401}
	b := UniqueTrait{Kind: "swift_cast", PowerBP: 4500, SkillID: 10402}
	if !ValidUniqueTrait(a) || !strings.Contains(a.Describe(10000), "火弹术") {
		t.Fatal("canonical skill scope missing")
	}
	if got := EvaluateUniqueTraits([]UniqueTrait{a, b}, UniqueContext{Skill: true, SkillID: 10401}).CastReductionBP; got != 2000 {
		t.Fatal("other skill suppressed matching weaker trait", got)
	}
	if got := EvaluateUniqueTraits([]UniqueTrait{a, b}, UniqueContext{Skill: true, SkillID: 10402}).CastReductionBP; got != 4500 {
		t.Fatal(got)
	}
	if got := EvaluateUniqueTraits([]UniqueTrait{a, b}, UniqueContext{}); got != (UniqueEffects{}) {
		t.Fatal("basic attack activated skill trait", got)
	}
	for _, bad := range []UniqueTrait{{Kind: "swift_cast", PowerBP: 2000, SkillID: 99999}, {Kind: "swift_cast", PowerBP: 2000, SkillID: 10401, SkillName: "假名"}, {Kind: "swift_cast", PowerBP: 2000, SkillID: 10301}, {Kind: "swift_cast", PowerBP: 2000, SkillID: 10412}, {Kind: "area_expansion", PowerBP: 2000, SkillID: 10401}, {Kind: "spell_economy", PowerBP: 2000, SkillID: 10304}, {Kind: "skill_focus", PowerBP: 2000, SkillID: 10301}, {Kind: "kill_mend", PowerBP: 2000, SkillID: 10401}} {
		if ValidUniqueTrait(bad) {
			t.Fatal("bad skill target accepted", bad)
		}
	}
	if TraitMatchesProfessions(a, 1) || !TraitMatchesProfessions(a, 16) {
		t.Fatal("class affinity ignored")
	}
	if got := EvaluateUniqueTraits([]UniqueTrait{{Kind: "skill_economy", PowerBP: 2500, SkillID: 10021}}, UniqueContext{Skill: true, SkillID: 10021, Magic: false}).ManaCostReductionBP; got != 2500 {
		t.Fatal("physical skill economy inert", got)
	}
}
func TestUniquePotencyImprovesBenefitsAndDrawbacks(t *testing.T) {
	for _, a := range []Affix{{Attr: 47, Value: 100}, {Attr: 17, Value: -100}, {Attr: 25, Value: -100}, {Attr: 25, Value: 100}} {
		low, high := scaleUniqueAffix(a, 8000), scaleUniqueAffix(a, 12000)
		if a.Attr == 25 {
			if high >= low {
				t.Fatal("attack interval worsened", low, high)
			}
		} else if high <= low {
			t.Fatal("better potency worsened affix", low, high)
		}
	}
}
func TestAstralTraitsUseSameWinnerAndPreserveSnapshots(t *testing.T) {
	defs, lookup := astralFixture()
	r, _ := AstralRecipeByID(21, testEndgameCatalog())
	def := defs[999001]
	w := NewEquipSet()
	st := astralStack(t, r, def, lookup)
	before := st
	w.Set(SlotWeapon, st)
	ch := &Character{Level: 100, Race: Warrior, EmploymentKnown: true, Employed: true}
	if got := AstralTraits(w, lookup, ch); !reflect.DeepEqual(got, r.Traits) {
		t.Fatal("formation traits not active", got)
	}
	if w.At(SlotWeapon) != before {
		t.Fatal("derived array mutated socket snapshots")
	}
	got := AstralRecipes(testEndgameCatalog())
	got[20].Traits[0].PowerBP = 9999
	if again, _ := AstralRecipeByID(21, testEndgameCatalog()); again.Traits[0].PowerBP == 9999 {
		t.Fatal("mutable trait escaped")
	}
	ch.Race = Warlock
	if len(AstralTraits(w, lookup, ch)) != 0 {
		t.Fatal("wrong class active")
	}
	ch.Race = Warrior
	st.Durability = 0
	w.Set(SlotWeapon, st)
	if len(AstralTraits(w, lookup, ch)) != 0 {
		t.Fatal("broken array active")
	}
	w.Set(SlotWeapon, Stack{})
	if len(AstralTraits(w, lookup, ch)) != 0 {
		t.Fatal("unequipped array remained")
	}
}

func TestClassLockedTraitsHaveNativeConsumer(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		mask  uint8
		valid bool
	}{
		{"fire_mastery", 1, false}, {"ice_mastery", 2, false}, {"holy_mastery", 4, false}, {"swift_cast", 4, false},
		{"spell_economy", 4, false}, {"healing_cost", 16, false}, {"trap_expansion", 8, false}, {"invisible_focus", 1, false},
		{"fire_mastery", 16, true}, {"healing_grace", 8, true}, {"swift_cast", 24, true}, {"holy_mastery", 31, true}, {"trap_expansion", 4, true},
	} {
		if got := TraitMatchesProfessions(UniqueTrait{Kind: tc.kind}, tc.mask); got != tc.valid {
			t.Fatal(tc, got)
		}
	}
}

func TestSkillScopeEvidenceMatchesRuntimeSemanticAllowlist(t *testing.T) {
	var rows []struct {
		SkillID                    SkillID
		Name                       string
		Profession                 uint8
		AllowedTraits              []string
		MinPrepareMS, MaxPrepareMS int32
		Area, Trap, Healing        bool
	}
	raw, e := os.ReadFile("../../../docs/daoist-endgame/catalog/skill_scopes.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != len(endgameSkillScopes) {
		t.Fatal("skill scope evidence coverage drift")
	}
	for _, r := range rows {
		d, ok := EndgameSkillTarget(r.SkillID)
		if !ok || d.Name != r.Name || d.Profession != r.Profession {
			t.Fatal("skill scope identity drift", r.SkillID)
		}
		allowed := map[string]bool{}
		for _, kind := range r.AllowedTraits {
			allowed[kind] = true
		}
		for kind := range uniqueTraitNames {
			if d.Supports(kind) != allowed[kind] {
				t.Fatal("semantic allowlist drift", r.SkillID, kind)
			}
		}
		if r.MaxPrepareMS == 0 && d.Supports("swift_cast") || !r.Area && d.Supports("area_expansion") || !r.Trap && d.Supports("trap_expansion") || r.Healing && d.Supports("skill_focus") {
			t.Fatal("scope advertises inert channel", r.SkillID)
		}
	}
}

func TestRuneCatalogueRecoveryDescriptionUsesActualCadence(t *testing.T) {
	r, ok := AstralRune(LastAstralRune, testEndgameCatalog())
	if !ok || !strings.Contains(r.Description(), "每2秒") || !strings.Contains(r.Description(), "生命恢复数值") {
		t.Fatal("rune recovery cadence absent")
	}
	if r.Affixes[0] != (Affix{Attr: AttrHPRegen, Value: 1, Mode: ModeAbsolute}) {
		t.Fatal("historical rune snapshot definition changed")
	}
}

func TestAstralRuneAffixValidationRejectsInactiveModes(t *testing.T) {
	cases := []struct {
		index int
		affix Affix
	}{
		{0, Affix{Attr: AttrHPRegen, Value: 10}},
		{0, Affix{Attr: AttrMaxWeight, Value: 100}},
		{0, Affix{Attr: AttrAtkSpeed, Value: -100}},
		{27, Affix{Attr: AttrHPRegen, Value: 10, Mode: ModePercent}},
		{27, Affix{Attr: AttrHPRegen, Value: -1}},
	}
	for _, tc := range cases {
		original := astralRuneCatalog[tc.index]
		astralRuneCatalog[tc.index].Affixes = []Affix{tc.affix}
		err := ValidateAstralCatalog(testEndgameCatalog())
		astralRuneCatalog[tc.index] = original
		if err == nil {
			t.Fatalf("inactive rune effect accepted: %d %+v", tc.index, tc.affix)
		}
	}
	if !SupportedAstralRuneDefinitionAffix(AstralRuneDef{RegenerationIntervalMS: 2000}, Affix{Attr: AttrHPRegen, Value: 5}) {
		t.Fatal("snapshot-backed rune regeneration rejected")
	}
	if err := ValidateAstralCatalog(testEndgameCatalog()); err != nil {
		t.Fatal(err)
	}
}
