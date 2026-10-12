package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"
)

type v5BudgetItem struct {
	BaseID     ItemID  `json:"base_id"`
	UniqueName string  `json:"unique_name"`
	Tier       string  `json:"tier"`
	Lane       string  `json:"intended_lane"`
	Classes    []Race  `json:"intended_class_ids"`
	Floor      float64 `json:"min_fresh_passive_lane_ratio"`
	Total      float64 `json:"total_budget_u"`
	Core       float64 `json:"budget_core_u"`
	Local      float64 `json:"budget_local_u"`
	Advanced   float64 `json:"budget_advanced_u"`
	Trait      float64 `json:"budget_retained_trait_u"`
}

func v5BudgetItems(t *testing.T) []v5BudgetItem {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/daoist-endgame/catalog/v5/assignment.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Items []v5BudgetItem `json:"items"`
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.Items
}

// Checks actual Compute with all MF, skills, procs and auras absent from the
// static score. The yellow is an enumerated compatible same-base max-roll lane
// reference, never a falsely labelled whole-item P90. All sockets/refines = 0.
func TestV5UniqueSameBaseStaticBudgetRuntime(t *testing.T) {
	bases, _, pools := cardBudgetLoad(t)
	items := v5BudgetItems(t)
	if len(items) != 259 {
		t.Fatal("V5 manifest count", len(items))
	}
	counts := map[string]int{}
	minimum := math.Inf(1)
	scenarios := 0
	type result struct {
		BaseID                          ItemID
		Class                           Race
		Roll                            int32
		Lane                            string
		Yellow, Fresh, Converted, Ratio float64
	}
	var report []result
	for _, item := range items {
		r := bases[item.BaseID]
		d, ok := UniqueForBase(item.BaseID)
		if !ok || d.Name != item.UniqueName || d.PowerTier != item.Tier || !r.Native || r.Level > 60 {
			t.Fatalf("V5 identity/scope mismatch %d", item.BaseID)
		}
		if math.Abs(item.Total-item.Core-item.Local-item.Advanced-item.Trait) > 1e-8 {
			t.Fatalf("budget ledger %d", item.BaseID)
		}
		counts[d.PowerTier]++
		for _, class := range item.Classes {
			base := cardBudgetCharacter(t, r, class)
			gender := uint8(0)
			if r.Sex == 1 {
				gender = 1
			}
			if cardBudgetDefinition(r).Equip.Need.Meet(max(1, r.Level), base, gender) != RejectNone {
				t.Fatalf("illegal gender/requirement fixture %d", r.BaseID)
			}
			useful := map[int32]bool{}
			for _, a := range ResolveVariableAffixes(d.Affixes, d.VariableAffixes, UniqueRollMinBP) {
				if cardBudgetUsefulLine(a, class, item.Lane) {
					useful[a.Attr] = true
				}
			}
			if len(useful) < 5 {
				t.Errorf("%d class%d only %d meaningful static fields", item.BaseID, class, len(useful))
			}
			yellow, score := cardBudgetBestYellow(t, r, base, item.Lane, pools[[2]int32{r.Tier, r.Type}])
			last := 0.0
			for _, roll := range []int32{8000, 10000, 12000} {
				fresh := cardBudgetMetric(t, cardBudgetComputeVersion(r, base, nil, roll, UniqueContentVersion), item.Lane, max(1, r.Level))
				converted := cardBudgetMetric(t, cardBudgetComputeVersion(r, base, yellow, roll, UniqueContentVersion), item.Lane, max(1, r.Level))
				ratio := fresh / score
				if ratio+1e-9 < item.Floor {
					t.Errorf("V5_BUDGET_LOW base=%d class=%d roll=%d lane=%s fresh=%g yellow=%g ratio=%.6f need=%.6f", item.BaseID, class, roll, item.Lane, fresh, score, ratio, item.Floor)
				}
				if fresh < last || converted < fresh || converted <= score {
					t.Errorf("nonmonotonic or destructive conversion %d", item.BaseID)
				}
				last = fresh
				minimum = math.Min(minimum, ratio)
				scenarios++
				report = append(report, result{item.BaseID, class, roll, item.Lane, score, fresh, converted, ratio})
			}
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"transition": 183, "highvalue": 54, "builddefining": 18, "godlike": 4}) {
		t.Fatal(counts)
	}
	if path := os.Getenv("V5_BUDGET_REPORT"); path != "" {
		raw, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("V5 actual Compute: %d same-base legal class/roll scenarios, min static ratio %.6f", scenarios, minimum)
}

func TestV5CatalogVersionPreservesAllOldInstances(t *testing.T) {
	if len(legacyUniqueDefinitions) != 1619 {
		t.Fatal(len(legacyUniqueDefinitions))
	}
	excluded := 0
	for _, old := range legacyUniqueDefinitions {
		st := Stack{UID: int64(old.BaseID), Item: old.BaseID, Count: 1, Endgame: EndgameItemState{UniqueID: old.ID, UniqueRollBP: 10000}}
		before := st
		got, ok := UniqueOf(st)
		if !ok || !reflect.DeepEqual(got, cloneUnique(old)) || st != before {
			t.Fatalf("legacy instance altered %d", old.BaseID)
		}
		if _, allowed := UniqueForBase(old.BaseID); !allowed {
			excluded++
			plain := st
			plain.Endgame = EndgameItemState{}
			def := uniqueFixture(old)
			if ApplyUnique(&plain, def, func(int) int { return 0 }) || RollUniqueDrop(&plain, def, func(int) int { return 0 }) {
				t.Fatalf("outside scope generated %d", old.BaseID)
			}
			st.Endgame.UniqueContentVersion = UniqueContentVersion
			if _, ok := UniqueOf(st); ok {
				t.Fatal("V5 accepted excluded base", old.BaseID)
			}
		}
	}
	if excluded != 1360 {
		t.Fatal(excluded)
	}
	for _, d := range UniqueCatalog() {
		st := Stack{UID: int64(d.BaseID), Item: d.BaseID, Count: 1}
		if !ApplyUnique(&st, uniqueFixture(d), func(int) int { return 0 }) || st.Endgame.UniqueContentVersion != 5 {
			t.Fatal(fmt.Sprintf("new V5 instance %d", d.BaseID))
		}
	}
}

// Authoring-only opt-in solver. Its output is reviewed and frozen as explicit
// stat rows; production never tunes stats or rotates attributes by item ID.
func TestV5AuthorStaticValuesUsingActualCompute(t *testing.T) {
	path := os.Getenv("V5_AUTHOR_STATS")
	if path == "" {
		t.Skip("explicit authoring output path required")
	}
	bases, _, pools := cardBudgetLoad(t)
	items := v5BudgetItems(t)
	type authored struct {
		BaseID            ItemID
		Affixes           []Affix
		VariableAffixes   []VariableAffix
		LocalEnhancements []ItemLocalEnhancement
		TypedEffects      []EquipmentEffect
		SkillBonuses      []SkillRankBonus
	}
	var out []authored
	for _, item := range items {
		r := bases[item.BaseID]
		original := uniqueByBase[item.BaseID]
		d := cloneUnique(original)
		target := map[string]float64{"transition": 1.10, "highvalue": 1.17, "builddefining": 1.13, "godlike": 1.38}[item.Tier]
		// Pure numeric godlike has no hidden damage source; conditional/skill routes
		// reserve a measured passive margin for their separately budgeted mechanic.
		if item.Tier == "godlike" {
			if len(d.SkillBonuses) == 0 && len(d.Traits) == 0 {
				target = 1.48
			} else if len(d.Traits) > 0 {
				target = 1.28
			} else {
				target = 1.35
			}
		}
		var attr int32
		switch item.Lane {
		case "physical":
			attr = AttrAtk
		case "magic":
			attr = AttrMAtk
		case "vitality", "physical_guard", "magic_guard":
			attr = AttrMaxHP
		case "sustain", "healing_sustain":
			attr = AttrMaxMP
		default:
			t.Fatal(item.Lane)
		}
		idx := -1
		for i, a := range d.Affixes {
			if a.Attr == attr && a.Value > 0 {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("%d lacks primary lane field %d", item.BaseID, attr)
		}
		type reference struct {
			base  Base
			score float64
		}
		var refs []reference
		for _, class := range item.Classes {
			base := cardBudgetCharacter(t, r, class)
			_, score := cardBudgetBestYellow(t, r, base, item.Lane, pools[[2]int32{r.Tier, r.Type}])
			refs = append(refs, reference{base, score})
		}
		set := func(value int32) {
			d.Affixes[idx].Value = value
			for j, v := range d.VariableAffixes {
				if v.Attr == attr {
					d.VariableAffixes[j].Min = value
					d.VariableAffixes[j].Max = value + max(1, value/10)
				}
			}
			uniqueByBase[item.BaseID] = d
		}
		ratio := func(value int32) float64 {
			set(value)
			lowest := math.Inf(1)
			for _, ref := range refs {
				score := cardBudgetMetric(t, cardBudgetComputeVersion(r, ref.base, nil, 8000, 5), item.Lane, max(1, r.Level))
				lowest = math.Min(lowest, score/ref.score)
			}
			return lowest
		}
		minimumValue := int32(2)
		// A tiny positive number is not an acceptable fifth-line placeholder.
		// Keep at least one quarter of this source pool's strongest comparable
		// yellow line (flat dual attack compares the two native end points).
		var minAttack, maxAttack int32
		for _, a := range pools[[2]int32{r.Tier, r.Type}] {
			if a.Attr == attr && a.Mode == d.Affixes[idx].Mode {
				minimumValue = max(minimumValue, (a.Value+3)/4)
			}
			if a.Mode == 0 && a.Attr == AttrMinAtk {
				minAttack = max(minAttack, a.Value)
			}
			if a.Mode == 0 && a.Attr == AttrMaxAtk {
				maxAttack = max(maxAttack, a.Value)
			}
		}
		if attr == AttrAtk && d.Affixes[idx].Mode == 0 {
			minimumValue = max(minimumValue, (minAttack+maxAttack+7)/8)
		}
		lo, hi := minimumValue, max(minimumValue, d.Affixes[idx].Value)
		for ratio(hi) < target {
			hi *= 2
			if hi > 100000 {
				t.Fatalf("no bounded value %d", item.BaseID)
			}
		}
		for lo < hi {
			mid := lo + (hi-lo)/2
			if ratio(mid) >= target {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		set(lo)
		out = append(out, authored{d.BaseID, d.Affixes, d.VariableAffixes, d.LocalEnhancements, d.TypedEffects, d.SkillBonuses})
		uniqueByBase[item.BaseID] = original
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestV5FormationVersionAndAdvancedReplacement(t *testing.T) {
	defs, lookup := astralFixture()
	changed := 0
	for _, r := range AstralRecipes() {
		def := defs[999001]
		copyEquip := *def.Equip
		copyEquip.Slot = int32(r.Slots[0])
		def.Equip = &copyEquip
		defs[def.ID] = def
		st := astralStack(t, r, def, lookup)
		current, ok := MatchAstralRecipe(def, st)
		if !ok || !reflect.DeepEqual(current, cloneAstralRecipe(r)) {
			t.Fatalf("new formation %d does not resolve V5", r.ID)
		}
		st.Endgame.FormationContentVersion = 0
		before := st
		legacy, ok := MatchAstralRecipe(def, st)
		if !ok || !reflect.DeepEqual(legacy, cloneAstralRecipe(legacyAstralRecipeCatalog[r.ID-1])) || st != before {
			t.Fatalf("legacy formation changed %d", r.ID)
		}
		if len(r.TypedEffects)+len(r.SkillBonuses) > 0 {
			changed++
			if len(r.Traits) >= len(legacy.Traits) {
				t.Fatalf("formation %d added a free unbudgeted mechanic", r.ID)
			}
			if !reflect.DeepEqual(r.Affixes, legacy.Affixes) || !reflect.DeepEqual(r.Runes, legacy.Runes) || r.MinLevel != legacy.MinLevel {
				t.Fatalf("formation %d altered frozen rune or static identity", r.ID)
			}
		}
		st.Endgame.FormationContentVersion = 6
		if _, ok := MatchAstralRecipe(def, st); ok {
			t.Fatal("unknown formation content version accepted")
		}
	}
	if changed != 10 {
		t.Fatal("formation upgrades", changed)
	}
}

func TestV5AdvancedCatalogExactCountsAndDetachedCopies(t *testing.T) {
	counts := map[string]int{}
	skills, locals := 0, 0
	vars := map[int]int{}
	for _, d := range UniqueCatalog() {
		if len(d.SkillBonuses) > 0 {
			skills++
		}
		if len(d.LocalEnhancements) > 0 {
			locals++
		}
		n := len(d.VariableAffixes)
		for _, local := range d.LocalEnhancements {
			if local.MinPercent != 0 {
				n++
			}
		}
		vars[n]++
		for _, effect := range d.TypedEffects {
			counts[effect.Kind]++
		}
	}
	expected := map[string]int{"magic_find_pct": 22, "element_resist_bp": 21, "element_absorb_bp": 3, "element_damage_bp": 7, "enemy_element_pierce_bp": 1, "element_mitigation_bp": 1, "proc": 3, "aura": 3}
	if !reflect.DeepEqual(counts, expected) || skills != 28 || locals != 51 || !reflect.DeepEqual(vars, map[int]int{0: 124, 1: 120, 2: 15}) {
		t.Fatal(counts, skills, locals, vars)
	}
	d, _ := UniqueForBase(1107)
	d.TypedEffects[0].Aura.Value = 9999
	again, _ := UniqueForBase(1107)
	if again.TypedEffects[0].Aura.Value != 400 {
		t.Fatal("aura pointer escaped catalog")
	}
	d, _ = UniqueForBase(1306)
	d.TypedEffects[0].Proc.FixedRank = 9999
	again, _ = UniqueForBase(1306)
	if again.TypedEffects[0].Proc.FixedRank != 10 {
		t.Fatal("proc pointer escaped catalog")
	}
	d, _ = UniqueForBase(2409)
	d.SkillBonuses[0].Levels = 9999
	again, _ = UniqueForBase(2409)
	if again.SkillBonuses[0].Levels != 1 {
		t.Fatal("skill slice escaped catalog")
	}
}

func TestV5NarrowingDoesNotRemoveOrdinaryOrLegacySalvage(t *testing.T) {
	var old UniqueDefinition
	for _, candidate := range legacyUniqueDefinitions {
		if _, ok := UniqueForBase(candidate.BaseID); !ok {
			old = candidate
			break
		}
	}
	for _, unique := range []bool{false, true} {
		ctx := cubeFixture()
		def := ctx.Items[1001]
		def.ID = old.BaseID
		ctx.Items[old.BaseID] = def
		st := ctx.Bag.At(200)
		st.Item = old.BaseID
		recipe := CubeSalvageNormal
		if unique {
			st.Endgame = EndgameItemState{UniqueID: old.ID, UniqueRollBP: 10000}
			recipe = CubeSalvageUnique
		}
		ctx.Bag.Set(200, st)
		before := ctx.Bag.Clone()
		if _, err := PlanCube(ctx, EndgameRequest{RecipeID: recipe, TargetUID: "42"}, false); err != nil {
			t.Fatal("old salvage scope removed", old.BaseID, unique, err)
		}
		if !reflect.DeepEqual(ctx.Bag, before) {
			t.Fatal("quote mutated owned item")
		}
		st.Endgame = EndgameItemState{}
		ctx.Bag.Set(200, st)
		if _, err := PlanCube(ctx, EndgameRequest{RecipeID: CubeAwakenUnique, TargetUID: "42"}, false); err == nil {
			t.Fatal("outside scope awakening quoted")
		}
	}
}

func TestV5CompletionUpgradesOldBlankAndPartialWithoutReroll(t *testing.T) {
	defs, lookup := astralFixture()
	r, _ := AstralRecipeByID(11)
	def := defs[999001]
	eq := *def.Equip
	eq.Slot = int32(r.Slots[0])
	def.Equip = &eq
	defs[def.ID] = def
	for _, roll := range []int32{0, 10123} {
		for _, initial := range []int{0, len(r.Runes) - 1} {
			full := astralStack(t, r, def, lookup)
			full.Endgame = EndgameItemState{FormationRollBP: roll}
			bag := NewBag(10)
			worn := NewEquipSet()
			st := full
			for i := initial; i < len(r.Runes); i++ {
				bag.Set(i+1, Stack{UID: full.SocketUIDs[i], Item: full.Sockets[i], Count: 1, InstanceKind: ItemInstanceSocketCard, Card: full.SocketCards[i]})
				st.Sockets[i] = 0
				st.SocketUIDs[i] = 0
				st.SocketCards[i] = CardInstanceState{}
			}
			bag.Set(0, st)
			original := st
			for i := initial; i < len(r.Runes); i++ {
				plan, err := PlanAstralInsert(bag, worn, lookup, st.UID, full.SocketUIDs[i], uint8(i), &Character{Level: 100})
				if err != nil {
					t.Fatal(err)
				}
				if bag.At(0) != st {
					t.Fatal("plan mutated source")
				}
				bag, worn, st = plan.Bag, plan.Worn, plan.Target
				expectedVersion := int32(0)
				if i == len(r.Runes)-1 {
					expectedVersion = 5
				}
				if st.Endgame.FormationContentVersion != expectedVersion || st.Endgame.FormationRollBP != roll {
					t.Fatal("completion version or roll", st.Endgame)
				}
			}
			got, ok := MatchAstralRecipe(def, st)
			if !ok || len(got.SkillBonuses) != 1 || got.SkillBonuses[0].SkillID != 10002 {
				t.Fatal("new V5 payload absent")
			}
			if original.Endgame.FormationContentVersion != 0 {
				t.Fatal("original instance upgraded in place")
			}
		}
	}
	// A complete old formation keeps the old scoped-damage trait on read; after
	// explicit extraction and reinsertion it is deliberately a new V5 formation.
	old := astralStack(t, r, def, lookup)
	old.Endgame = EndgameItemState{FormationRollBP: 10123}
	bag := NewBag(10)
	bag.Set(0, old)
	worn := NewEquipSet()
	got, _ := MatchAstralRecipe(def, old)
	if len(got.SkillBonuses) != 0 || len(got.Traits) == 0 {
		t.Fatal("legacy read upgraded")
	}
	clone := old
	if adoptCurrentFormationOnCompletion(def, old, &clone) || clone != old {
		t.Fatal("complete formation silently upgraded")
	}
	removed, err := PlanAstralRemove(bag, worn, lookup, old.UID, 0)
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := PlanAstralInsert(removed.Bag, removed.Worn, lookup, old.UID, removed.Rune.UID, 0, &Character{Level: 100})
	if err != nil {
		t.Fatal(err)
	}
	if inserted.Target.Endgame.FormationContentVersion != 5 || inserted.Target.Endgame.FormationRollBP != 10123 {
		t.Fatal("explicit reform version/roll")
	}
}

func TestV5CubeCompletionUpgradesOldBlankWithoutReroll(t *testing.T) {
	ctx, req := cubeInputs(CubeInscribeArray)
	before := ctx.Bag.At(200)
	before.Endgame = EndgameItemState{FormationRollBP: 10123}
	ctx.Bag.Set(200, before)
	preview, err := PlanCube(ctx, req, false)
	if err != nil {
		t.Fatal(err)
	}
	executed, err := PlanCube(ctx, req, true)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Bag.At(200) != before || preview.Target.Endgame.FormationContentVersion != 5 || executed.Target.Endgame.FormationContentVersion != 5 || executed.Target.Endgame.FormationRollBP != 10123 {
		t.Fatal("cube completion changed input or roll", executed.Target.Endgame)
	}
}

func TestV5OrdinaryTypeDropPoolRetainsOriginalScope(t *testing.T) {
	if len(nativeEquipmentDropBases) != 885 {
		t.Fatal("ordinary native scope drift", len(nativeEquipmentDropBases))
	}
	outside := 0
	for _, old := range legacyUniqueDefinitions {
		def := uniqueFixture(old)
		if NativeEquipmentDropEligible(def) != old.NativeDropEligible {
			t.Fatal("ordinary source classification changed", old.BaseID)
		}
		if old.NativeDropEligible {
			if _, ok := UniqueForBase(old.BaseID); !ok {
				outside++
			}
		}
	}
	if outside != 626 {
		t.Fatal("native bases outside unique scope", outside)
	}
}
