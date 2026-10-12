package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestV6PreservesV5InstancesExceptThreePersonalDefenses(t *testing.T) {
	for _, old := range legacyV5UniqueDefinitions {
		for _, roll := range []int32{8000, 10000, 12000} {
			st := Stack{UID: int64(old.ID), Item: old.BaseID, Count: 1, RefineLevel: 4, WashCount: 3, Endgame: EndgameItemState{UniqueID: old.ID, UniqueRollBP: roll, UniqueContentVersion: 5}}
			before := st
			got, ok := UniqueOf(st)
			if !ok || st != before {
				t.Fatalf("V5 instance changed %d", old.ID)
			}
			expected := cloneUnique(old)
			if old.BaseID == 1107 || old.BaseID == 1111 || old.BaseID == 2603 {
				if len(old.TypedEffects) != 1 || old.TypedEffects[0].Aura == nil {
					t.Fatal("unexpected V5 defense source")
				}
				aura := old.TypedEffects[0].Aura
				if aura.Modifier == "physical_mitigation_bp" {
					if effect := EvaluateUniqueTraits(got.Traits, UniqueContext{}); effect.MitigationBP != aura.Value {
						t.Fatal("personal physical defense changed", effect)
					}
					if effect := EvaluateUniqueTraits(got.Traits, UniqueContext{Magic: true}); effect.MitigationBP != 0 {
						t.Fatal("physical defense became all-damage")
					}
				} else if len(got.TypedEffects) != 1 || got.TypedEffects[0].Kind != "element_resist_bp" || got.TypedEffects[0].Value != aura.Value || got.TypedEffects[0].Element != aura.Element {
					t.Fatal("element defense changed")
				}
				// Only these advertised effects and their explanatory sentence may differ.
				got.TypedEffects = expected.TypedEffects
				got.Traits = expected.Traits
				got.Benefit = expected.Benefit
				got.Identity = expected.Identity
				got.Tradeoff = expected.Tradeoff
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("unapproved V5 reinterpretation %d", old.ID)
			}
		}
	}
}

func TestV6UniqueDiversityAndExclusiveFormationAuras(t *testing.T) {
	counts := map[string]int{}
	tiers := map[string]int{}
	weapons := 0
	distinctWeapons := map[string]bool{}
	mechanicalWeapons := map[string]bool{}
	for _, d := range UniqueCatalog() {
		tiers[d.PowerTier]++
		for _, e := range d.TypedEffects {
			if e.Kind == "aura" {
				t.Fatalf("unique %d retained team aura", d.ID)
			}
			counts[e.Kind]++
		}
		if d.Slot == 4 || d.Slot == 13 {
			weapons++
			distinctWeapons[d.Identity] = true
			mechanicalWeapons[v6MechanicalSignature(d)] = true
		}
	}
	t.Logf("%d weapons, %d mechanical signatures ignoring names and magnitudes", weapons, len(mechanicalWeapons))
	if weapons != 60 || len(mechanicalWeapons) < 50 {
		t.Fatalf("weapon mechanics collapsed: %d weapons, %d signatures", weapons, len(mechanicalWeapons))
	}
	if len(distinctWeapons) != weapons {
		t.Fatal("weapon identities were templated")
	}
	if !reflect.DeepEqual(tiers, map[string]int{"transition": 183, "highvalue": 54, "builddefining": 18, "godlike": 4}) {
		t.Fatal(tiers)
	}
	for kind, min := range map[string]int{"proc": 10, "enemy_element_pierce_bp": 11, "element_damage_bp": 17, "skill_variant": 5, "magic_find_pct": 30} {
		if counts[kind] < min {
			t.Fatalf("lost %s authored diversity: %d", kind, counts[kind])
		}
	}
	auras := map[string]bool{}
	for _, r := range AstralRecipes() {
		for _, e := range r.TypedEffects {
			if e.Aura != nil {
				a := e.Aura
				if auras[a.Family] {
					t.Fatal("duplicate family allocation", a.Family)
				}
				auras[a.Family] = true
			}
		}
	}
	if len(auras) != 7 {
		t.Fatal("missing formation aura discipline", auras)
	}
}

func TestV6FormationReadsFrozenV5WithoutMutation(t *testing.T) {
	defs, lookup := astralFixture()
	for _, old := range legacyV5AstralRecipeCatalog {
		def := defs[999001]
		eq := *def.Equip
		eq.Slot = int32(old.Slots[0])
		def.Equip = &eq
		defs[def.ID] = def
		st := astralStack(t, old, def, lookup)
		st.Endgame.FormationContentVersion = 5
		before := st
		got, ok := MatchAstralRecipe(def, st)
		if !ok || st != before || !reflect.DeepEqual(got, cloneAstralRecipe(old)) {
			t.Fatalf("V5 formation reinterpreted %d", old.ID)
		}
		st.Endgame.FormationContentVersion = UniqueContentVersion
		current, ok := MatchAstralRecipe(def, st)
		if !ok || !reflect.DeepEqual(current, cloneAstralRecipe(astralRecipeCatalog[old.ID-1])) {
			t.Fatalf("V6 formation mismatch %d", old.ID)
		}
	}
}

// Ignore all magnitudes and names: merely nudging numbers or changing prose
// cannot satisfy this catalogue's weapon-mechanic diversity regression.
func v6MechanicalSignature(d UniqueDefinition) string {
	var parts []string
	for _, a := range d.Affixes {
		parts = append(parts, fmt.Sprintf("attr:%d:%d", a.Attr, a.Mode))
	}
	for _, a := range d.Traits {
		parts = append(parts, fmt.Sprintf("trait:%s:%d", a.Kind, a.SkillID))
	}
	for _, a := range d.TypedEffects {
		parts = append(parts, fmt.Sprintf("effect:%s:%s:%s", a.Kind, a.Element, a.VariantID))
	}
	for _, a := range d.SkillBonuses {
		parts = append(parts, fmt.Sprintf("skill:%s:%d:%s:%s:%d", a.Scope, a.Profession, a.Branch, a.School, a.SkillID))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func TestV6CompiledCatalogueMatchesReviewedSource(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/daoist-endgame/catalog/v6/authored_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []UniqueDefinition
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != UniqueCatalogCount {
		t.Fatal("authoring scope drift", len(rows))
	}
	seen := map[ItemID]bool{}
	for _, want := range rows {
		got, ok := UniqueForBase(want.BaseID)
		if !ok || seen[want.BaseID] || !reflect.DeepEqual(got, cloneUnique(want)) {
			t.Fatalf("stale or duplicate compiled V6 item %d", want.BaseID)
		}
		seen[want.BaseID] = true
	}
}
