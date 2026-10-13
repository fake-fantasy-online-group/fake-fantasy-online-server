package domain

// Independent integration checks use the compiled catalogue, original-source
// equipment fixtures, live UniqueAffixes, and the actual worn-equipment Compute
// path. They do not trust the budget generator's recorded outcome ratios.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"testing"
)

type cardBudgetSource struct {
	BaseID       ItemID           `json:"base_id"`
	Name         string           `json:"base_name"`
	Level        int32            `json:"wearer_level"`
	Tier         int32            `json:"resource_tier"`
	Type         int32            `json:"type"`
	Slot         int32            `json:"slot"`
	Professions  []Race           `json:"professions"`
	Sex          uint8            `json:"sex"`
	Native       bool             `json:"native_yellow_pool"`
	Beginner     bool             `json:"beginner_allowed"`
	Restricted   bool             `json:"profession_restricted"`
	Unknown      bool             `json:"unknown_innate_properties"`
	SuitID       int32            `json:"suit_id"`
	Fixed        []Affix          `json:"fixed_affixes"`
	MaxRandom    int              `json:"max_random_lines"`
	Intrinsic    map[string]int32 `json:"intrinsic"`
	Requirements map[string]int32 `json:"requirements"`
}

type cardBudgetTarget struct {
	BaseID, ReferenceBaseID   ItemID
	Lane, Evidence, PowerTier string
	IntendedClasses           []Race
	RequiredLowRollRatio      float64
}

type cardBudgetPoolRow struct {
	Tier, Type, Attr, Mode, Max int32
	Sign                        int32 `json:"value_sign"`
}

func cardBudgetRead(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile("../../../docs/daoist-endgame/balance/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func cardBudgetLoad(t *testing.T) (map[ItemID]cardBudgetSource, []cardBudgetTarget, map[[2]int32][]Affix) {
	t.Helper()
	var source struct{ Bases []cardBudgetSource }
	var targets []cardBudgetTarget
	var pool []cardBudgetPoolRow
	cardBudgetRead(t, "native_equipment_budget_references.json", &source)
	cardBudgetRead(t, "unique_power_targets.json", &targets)
	cardBudgetRead(t, "yellow_affix_benchmarks.json", &pool)
	bases := make(map[ItemID]cardBudgetSource)
	native := 0
	for _, row := range source.Bases {
		if _, duplicate := bases[row.BaseID]; duplicate {
			t.Fatalf("duplicate original-source base %d", row.BaseID)
		}
		bases[row.BaseID] = row
		if row.Native {
			native++
		}
		if row.MaxRandom != max(0, MaxEquipmentAttributes-len(row.Fixed)) {
			t.Fatalf("base %d ignores fixed-natural-line capacity", row.BaseID)
		}
	}
	if len(bases) != 1619 || native != 298 || len(targets) != len(bases) {
		t.Fatalf("source/target coverage differs: bases=%d native=%d targets=%d", len(bases), native, len(targets))
	}
	pools := make(map[[2]int32][]Affix)
	for _, r := range pool {
		if r.Sign != 1 && r.Sign != -1 {
			t.Fatalf("invalid source affix sign %+v", r)
		}
		key := [2]int32{r.Tier, r.Type}
		pools[key] = append(pools[key], Affix{Attr: r.Attr, Mode: r.Mode, Value: r.Max * r.Sign})
	}
	if len(pools) != 135 {
		t.Fatalf("invented or missing native pool keys: %d", len(pools))
	}
	return bases, targets, pools
}

func cardBudgetDefinition(r cardBudgetSource) ItemDef {
	i, req := r.Intrinsic, r.Requirements
	var mask uint8
	for _, class := range r.Professions {
		mask |= 1 << class
	}
	def := ItemDef{EndgameCatalog: testEndgameCatalog(), ID: r.BaseID, Name: r.Name, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{
		Slot: r.Slot, LevelReq: r.Level, ResourceLevel: r.Tier, Type: r.Type,
		Durable: 100, Affixes: append([]Affix(nil), r.Fixed...), UnknownInnateProperties: r.Unknown,
		Base:  Base{STR: i["str"], VIT: i["vit"], INT: i["int_"], SPI: i["spi"], AGI: i["agi"], DEX: i["dex"]},
		Bonus: Stats{MinAtk: i["min_atk"], MaxAtk: i["max_atk"], MAtk: i["matk"], Def: i["def"], MDef: i["mdef"], Hit: i["hit_rate"]},
		Need: Requirement{Level: r.Level, Sex: r.Sex, Professions: mask, BeginnerAllowed: r.Beginner, ProfessionRestricted: r.Restricted,
			Base: Base{STR: req["str_need"], VIT: req["vit_need"], INT: req["int_need"], SPI: req["spi_need"], AGI: req["agi_need"], DEX: req["dex_need"]}},
	}}
	if r.SuitID != 0 {
		// Membership alone excludes an equipment base from formation recipes.
		def.Equip.Suit = &EquipmentSuit{ID: r.SuitID}
	}
	return def
}

func cardBudgetCharacter(t *testing.T, r cardBudgetSource, race Race) Base {
	t.Helper()
	if !race.Valid() {
		t.Fatalf("invalid intended class %d", race)
	}
	def := cardBudgetDefinition(r)
	if !def.Equip.Need.AllowsProfession(race) && !(r.Beginner && len(r.Professions) == 0) {
		t.Fatalf("base %d claims an illegal intended class %d", r.BaseID, race)
	}
	level := max(int32(1), r.Level)
	base := StartingBase(race)
	for n := int32(2); n <= level; n++ {
		base = base.Add(GrowthAt(race, n))
	}
	points := FreePointsPerLevel * (level - 1)
	for _, field := range []struct {
		actual *int32
		need   int32
	}{
		{&base.STR, def.Equip.Need.Base.STR}, {&base.VIT, def.Equip.Need.Base.VIT},
		{&base.INT, def.Equip.Need.Base.INT}, {&base.SPI, def.Equip.Need.Base.SPI},
		{&base.AGI, def.Equip.Need.Base.AGI}, {&base.DEX, def.Equip.Need.Base.DEX},
	} {
		delta := max(int32(0), field.need-*field.actual)
		*field.actual += delta
		points -= delta
	}
	if points < 0 {
		t.Fatalf("base %d class %d cannot meet source requirements with the stated naked-growth fixture", r.BaseID, race)
	}
	if race < Healer {
		base.STR += points
	} else {
		base.INT += points
	}
	return base
}

func cardBudgetCompute(r cardBudgetSource, base Base, natural []Affix, potency int32) Stats {
	return cardBudgetComputeVersion(r, base, natural, potency, 0)
}

func cardBudgetComputeVersion(r cardBudgetSource, base Base, natural []Affix, potency, version int32) Stats {
	def := cardBudgetDefinition(r)
	st := Stack{UID: int64(r.BaseID), Item: r.BaseID, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 100, MaxDurability: 100}
	for i, affix := range natural {
		st.RolledAffixes[i] = InstanceAffix{Affix: affix}
	}
	st.RolledAffixCount = uint8(len(natural))
	if potency != 0 {
		st.Endgame.UniqueID = int32(r.BaseID)
		st.Endgame.UniqueRollBP = potency
		st.Endgame.UniqueContentVersion = version
	}
	worn := NewEquipSet()
	worn.Set(EquipSlot(r.Slot), st)
	_, stats := Compute(StatSource{Base: base, Level: max(int32(1), r.Level), Innate: Stats{MoveSpeed: 100}, Worn: worn,
		Defs: func(id ItemID) (ItemDef, bool) { return def, id == def.ID }})
	return stats
}

func cardBudgetMetric(t *testing.T, s Stats, lane string, level int32) float64 {
	t.Helper()
	switch lane {
	case "physical":
		return float64(s.MinAtk+s.MaxAtk) / 2
	case "magic":
		return float64(s.MAtk)
	case "vitality":
		return float64(s.MaxHP)
	case "sustain", "healing_sustain":
		// This is an explicit MP/sustain score, not a throughput or movement claim.
		return float64(s.MaxMP)
	case "physical_guard":
		hit := float64(20 + 3*level)
		return float64(s.MaxHP) / math.Max(1, float64(100+12*level-s.PhysResist)) * (hit + float64(max(0, s.Def))) / hit
	case "magic_guard":
		return float64(s.MaxHP) / math.Max(1, float64(100+12*level)-.63*float64(s.MDef)-float64(s.MagicResist))
	default:
		t.Fatalf("unknown outcome lane %q", lane)
		return 0
	}
}

func cardBudgetRelevant(lane string, attr int32) bool {
	var allowed []int32
	switch lane {
	case "physical":
		allowed = []int32{1, 47, 29, 31}
	case "magic":
		allowed = []int32{3, 15}
	case "vitality":
		allowed = []int32{5, 34}
	case "sustain", "healing_sustain":
		allowed = []int32{61, 36}
	case "physical_guard":
		allowed = []int32{5, 34, 7, 13, 69}
	case "magic_guard":
		allowed = []int32{5, 34, 61, 17, 70}
	}
	for _, k := range allowed {
		if k == attr {
			return true
		}
	}
	return false
}

func cardBudgetBestYellow(t *testing.T, r cardBudgetSource, base Base, lane string, pool []Affix) ([]Affix, float64) {
	t.Helper()
	byAttr := map[int32][]Affix{}
	for _, a := range pool {
		if cardBudgetRelevant(lane, a.Attr) {
			byAttr[a.Attr] = append(byAttr[a.Attr], a)
		}
	}
	var ids []int
	for attr := range byAttr {
		ids = append(ids, int(attr))
	}
	sort.Ints(ids)
	best, score := []Affix(nil), -1.0
	var visit func(int, []Affix)
	visit = func(index int, selected []Affix) {
		if index == len(ids) {
			value := cardBudgetMetric(t, cardBudgetCompute(r, base, selected, 0), lane, max(int32(1), r.Level))
			if value > score {
				score, best = value, append([]Affix(nil), selected...)
			}
			return
		}
		visit(index+1, selected)
		if len(selected) == r.MaxRandom {
			return
		}
		for _, a := range byAttr[int32(ids[index])] {
			visit(index+1, append(selected, a))
		}
	}
	visit(0, nil)
	return best, score
}

func TestUniqueAllSourceBudgetLanesThroughWornRuntime(t *testing.T) {
	bases, targets, pools := cardBudgetLoad(t)
	seen := map[ItemID]bool{}
	counts := map[string]int{}
	worst := math.Inf(1)
	maxConverted := 0.0
	maxConvertedID := ItemID(0)
	for _, target := range targets {
		t.Run(fmt.Sprint(target.BaseID), func(t *testing.T) {
			r, ok := bases[target.BaseID]
			if !ok || seen[r.BaseID] {
				t.Fatal("missing or duplicate source target")
			}
			seen[r.BaseID] = true
			ref, ok := bases[target.ReferenceBaseID]
			if !ok || !ref.Native {
				t.Fatal("reference does not have an actual native pool")
			}
			pool := pools[[2]int32{ref.Tier, ref.Type}]
			if len(pool) == 0 {
				t.Fatal("reference native pool missing")
			}
			if r.Native {
				if target.Evidence != "native_pool" || ref.BaseID != r.BaseID {
					t.Fatal("native item sidestepped its own yellow pool")
				}
			} else if target.Evidence != "provisional_anchor" {
				t.Fatal("missing native pool falsely represented as native evidence")
			}
			d, ok := legacyUniqueByBase[r.BaseID]
			if !ok || d.PowerTier != target.PowerTier || d.LegacyAffixPoolReady != r.Native {
				t.Fatal("compiled unique and source target metadata differ")
			}
			if target.RequiredLowRollRatio <= 1 || len(target.IntendedClasses) == 0 {
				t.Fatal("missing strict-upgrade outcome target")
			}
			minimumMargin, knownTier := map[string]float64{"transition": 1.08, "highvalue": 1.16, "builddefining": 1.23, "godlike": 1.38}[target.PowerTier]
			if !knownTier || target.RequiredLowRollRatio+1e-9 < minimumMargin {
				t.Fatal("target weakens the independently specified tier margin")
			}
			for _, class := range target.IntendedClasses {
				base := cardBudgetCharacter(t, r, class)
				usefulLines := 0
				for _, a := range ResolveVariableAffixes(d.Affixes, d.VariableAffixes, 8000) {
					if cardBudgetUsefulLine(a, class, target.Lane) {
						usefulLines++
					}
				}
				if usefulLines < 5 {
					t.Errorf("class=%d lane=%s has only %d beneficial class-relevant executable lines at minimum potency", class, target.Lane, usefulLines)
				}
				yellow, yellowScore := cardBudgetBestYellow(t, r, base, target.Lane, pool)
				if yellowScore <= 0 {
					t.Fatal("zero or invalid yellow outcome")
				}
				lastScore := 0.0
				for _, potency := range []int32{8000, 10000, 12000} {
					fresh := cardBudgetCompute(r, base, nil, potency)
					converted := cardBudgetCompute(r, base, yellow, potency)
					freshScore := cardBudgetMetric(t, fresh, target.Lane, max(int32(1), r.Level))
					convertedScore := cardBudgetMetric(t, converted, target.Lane, max(int32(1), r.Level))
					ratio := freshScore / yellowScore
					if ratio+1e-9 < target.RequiredLowRollRatio {
						t.Errorf("class=%d potency=%d %s %s fresh=%g yellow=%g ratio=%.6f required=%.6f affixes=%+v", class, potency, target.Evidence, target.Lane, freshScore, yellowScore, ratio, target.RequiredLowRollRatio, yellow)
					}
					if freshScore+1e-9 < lastScore {
						t.Error("greater potency worsened the intended outcome")
					}
					if convertedScore+1e-9 < freshScore || convertedScore <= yellowScore {
						t.Error("additive conversion lost fresh or yellow value")
					}
					lastScore = freshScore
					worst = math.Min(worst, ratio)
					if convertedScore/yellowScore > maxConverted {
						maxConverted, maxConvertedID = convertedScore/yellowScore, r.BaseID
					}
					counts[target.Evidence]++
					switch r.BaseID {
					case 1001, 1003, 1006, 1010, 1301, 1303, 1306, 1310, 1606, 1610, 1806, 1810, 2002, 2106, 2560, 9901, 15043:
						t.Logf("SOURCE_BUDGET\t%d\t%d\t%d\t%s\t%s\t%.6f\t%.6f\t%.6f\t%.6f\t%.6f", r.BaseID, class, potency, target.Lane, target.Evidence, yellowScore, freshScore, convertedScore, ratio, convertedScore/yellowScore)
					}
				}
			}
		})
	}
	if len(seen) != 1619 {
		t.Fatalf("incomplete tested catalogue: %d", len(seen))
	}
	t.Logf("actual worn runtime scenarios=%v lowest fresh/yellow=%.6f highest converted/yellow=%.6f base=%d; provisional anchors are not observed same-level native gear", counts, worst, maxConverted, maxConvertedID)
}

// Count executable beneficial properties, not padding from the other offense
// route. The high reference baseline avoids treating integer rounding on a
// particular low-level character as proof that a real primary stat is inert.
// The actual per-item outcome floor is tested separately above.
func cardBudgetUsefulLine(a Affix, class Race, lane string) bool {
	if a.Value == 0 || a.Attr != AttrAtkSpeed && a.Value < 0 || a.Attr == AttrAtkSpeed && a.Value > 0 {
		return false
	}
	magicRoute := class >= Healer
	if lane == "physical" {
		magicRoute = false
	}
	if lane == "magic" {
		magicRoute = true
	}
	if magicRoute {
		switch a.Attr {
		case AttrSTR, AttrDEX, AttrHit, AttrCrit, AttrAtk, AttrMinAtk, AttrMaxAtk, AttrAtkSpeed:
			return false
		}
	} else {
		switch a.Attr {
		case AttrINT, AttrMAtk, AttrMCrit:
			return false
		}
	}
	if a.Attr == AttrHPRegen || a.Attr == AttrMaxWeight || a.Attr == AttrAtkSpeed && a.Mode != ModePercent || (a.Attr == AttrCrit || a.Attr == AttrMCrit) && a.Mode != ModeAbsolute {
		return false
	}
	in := StatSource{Base: Base{STR: 1000, VIT: 1000, INT: 1000, SPI: 1000, AGI: 1000, DEX: 1000}, Innate: Stats{MoveSpeed: 100}}
	b0, s0 := Compute(in)
	in.Passive = []Affix{a}
	b1, s1 := Compute(in)
	return b1 != b0 || s1 != s0
}

type formationBudgetTarget struct {
	RecipeID    int32
	PowerTier   string
	Comparisons []struct {
		Class           Race
		BaseID          ItemID
		Lane, Reference string
		FixtureLevel    int32
		RequiredRatio   float64
	}
}

func TestFormationAllSourceBudgetLanesThroughActivation(t *testing.T) {
	bases, _, pools := cardBudgetLoad(t)
	var targets []formationBudgetTarget
	cardBudgetRead(t, "formation_power_targets.json", &targets)
	if len(targets) != 40 {
		t.Fatalf("formation manifest coverage=%d", len(targets))
	}
	runeDefs := map[ItemID]ItemDef{}
	for _, d := range AstralRuneItems(1) {
		runeDefs[d.ID] = d
	}
	seen := map[int32]bool{}
	scenarios := 0
	minimum := math.Inf(1)
	for _, target := range targets {
		t.Run(fmt.Sprint(target.RecipeID), func(t *testing.T) {
			recipe := legacyAstralRecipeCatalog[target.RecipeID-1]
			ok := recipe.ID != 0
			if !ok || seen[recipe.ID] || recipe.PowerTier != target.PowerTier || len(target.Comparisons) == 0 {
				t.Fatal("invalid or stale formation target")
			}
			seen[recipe.ID] = true
			for _, comparison := range target.Comparisons {
				r, ok := bases[comparison.BaseID]
				if !ok || !r.Native || comparison.Reference != "native_plain_pool_at_effective_item_level" {
					t.Fatal("formation reference is not an actual plain-native pool")
				}
				if r.Level > comparison.FixtureLevel {
					t.Fatal("reference exceeds the actual finished-equipment wearer-level boundary")
				}
				def := cardBudgetDefinition(r)
				st := Stack{UID: int64(r.BaseID), Item: r.BaseID, Count: 1, InstanceKind: ItemInstanceEquipment, Durability: 100, MaxDurability: 100, SocketCount: uint8(len(recipe.Runes))}
				if !AstralBaseEligible(def, st) {
					t.Fatalf("source base %d is not genuinely plain: %s", r.BaseID, AstralBaseIneligibility(def, st))
				}
				level := max(recipe.MinLevel, r.Level)
				for i, id := range recipe.Runes {
					rune, valid := AstralRune(id, testEndgameCatalog())
					if !valid {
						t.Fatal("recipe ingredient absent")
					}
					level = max(level, rune.MinLevel)
					st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = id, int64(900000+i), runeDefs[id].CardDefaults
				}
				if got := EquipmentRequiredLevel(def, st); got != level {
					t.Fatalf("formation/base requirement=%d want %d", got, level)
				}
				if comparison.FixtureLevel != level {
					t.Fatalf("fixture level=%d want actual runtime requirement=%d", comparison.FixtureLevel, level)
				}
				buildSource := r
				buildSource.Level = level
				base := cardBudgetCharacter(t, buildSource, comparison.Class)
				gender := uint8(0)
				if r.Sex == 1 {
					gender = 1
				}
				if def.Equip.Need.Meet(level, base, gender) != RejectNone {
					t.Fatal("formation source equipment cannot be worn by fixture")
				}
				ch := &Character{Race: comparison.Class, Level: level, Base: base, EmploymentKnown: true, Employed: true}
				worn := NewEquipSet()
				worn.Set(EquipSlot(r.Slot), st)
				lookup := func(id ItemID) (ItemDef, bool) {
					if id == def.ID {
						return def, true
					}
					d, exists := runeDefs[id]
					return d, exists
				}
				active := ActiveAstralRecipes(worn, lookup, ch)
				if len(active) != 1 || active[0].Recipe.ID != recipe.ID {
					t.Fatalf("formation did not actually activate on base %d class %d at level %d", r.BaseID, comparison.Class, level)
				}
				bonuses := AstralBonuses(worn, lookup, ch)
				if !reflect.DeepEqual(bonuses, AstralItemBonuses(def, st, recipe)) {
					t.Fatal("actual activation does not expose all authored extras")
				}
				useful := 0
				// The five-line minimum cannot be padded with resolved local deltas.
				for _, a := range ResolveVariableAffixes(recipe.Affixes, recipe.VariableAffixes, st.Endgame.FormationRollBP) {
					if cardBudgetUsefulLine(a, comparison.Class, comparison.Lane) {
						useful++
					}
				}
				if useful < 5 {
					t.Errorf("class=%d has %d useful authored lines; ingredient cards and traits cannot supply the missing five", comparison.Class, useful)
				}
				_, formed := Compute(StatSource{Base: base, Level: level, Innate: Stats{MoveSpeed: 100}, Worn: worn, Defs: lookup, Passive: bonuses})
				_, ingredientsOnly := Compute(StatSource{Base: base, Level: level, Innate: Stats{MoveSpeed: 100}, Worn: worn, Defs: lookup})
				pool := pools[[2]int32{r.Tier, r.Type}]
				_, yellowScore := cardBudgetBestYellow(t, buildSource, base, comparison.Lane, pool)
				formedScore := cardBudgetMetric(t, formed, comparison.Lane, level)
				ingredientScore := cardBudgetMetric(t, ingredientsOnly, comparison.Lane, level)
				ratio := formedScore / yellowScore
				// Exercise the actual winner at both persisted roll boundaries,
				// preserving the same ingredient snapshots and native base.
				for _, roll := range []int32{UniqueRollMinBP, UniqueRollMaxBP} {
					rolled := st
					rolled.Endgame.FormationRollBP = roll
					worn.Set(EquipSlot(r.Slot), rolled)
					_, rolledStats := Compute(StatSource{Base: base, Level: level, Innate: Stats{MoveSpeed: 100}, Worn: worn, Defs: lookup, Passive: AstralBonuses(worn, lookup, ch)})
					score := cardBudgetMetric(t, rolledStats, comparison.Lane, level)
					if score/yellowScore+1e-9 < comparison.RequiredRatio {
						t.Errorf("formation %d roll%d falls below unchanged yellow margin: %g", recipe.ID, roll, score/yellowScore)
					}
					if roll == UniqueRollMinBP && score > formedScore+1e-9 || roll == UniqueRollMaxBP && score+1e-9 < formedScore {
						t.Error("formation quality is not monotonic")
					}
				}
				worn.Set(EquipSlot(r.Slot), st)
				minRatio := map[string]float64{"transition": 1.12, "highvalue": 1.20, "builddefining": 1.30, "godlike": 1.38}[recipe.PowerTier]
				if minRatio <= 1 || comparison.RequiredRatio+1e-9 < minRatio {
					t.Fatal("formation target weakens independently specified tier floor")
				}
				if yellowScore <= 0 || ratio+1e-9 < comparison.RequiredRatio {
					t.Errorf("class=%d base=%d level=%d lane=%s formed=%g yellow=%g ratio=%.6f want>=%.6f", comparison.Class, r.BaseID, level, comparison.Lane, formedScore, yellowScore, ratio, comparison.RequiredRatio)
				}
				if formedScore <= ingredientScore {
					t.Error("authored formation bonuses add no intended outcome beyond ingredient snapshots")
				}
				low := *ch
				low.Level = level - 1
				if len(ActiveAstralRecipes(worn, lookup, &low)) != 0 {
					t.Error("below ingredient/recipe level incorrectly activates")
				}
				for race := Warrior; race <= Warlock; race++ {
					if recipe.Professions != 0 && recipe.Professions&(1<<uint8(race)) == 0 {
						wrong := *ch
						wrong.Race = race
						if len(ActiveAstralRecipes(worn, lookup, &wrong)) != 0 {
							t.Errorf("excluded recipe class %d activates", race)
						}
					}
				}
				scenarios++
				minimum = math.Min(minimum, ratio)
				t.Logf("FORMATION_BUDGET\t%d\t%d\t%d\t%d\t%s\t%.6f\t%.6f\t%.6f\t%.6f", recipe.ID, comparison.Class, r.BaseID, level, comparison.Lane, yellowScore, ingredientScore, formedScore, ratio)
			}
		})
	}
	if len(seen) != 40 {
		t.Fatal("formation activation coverage incomplete")
	}
	t.Logf("actual formation activation scenarios=%d minimum formed/yellow=%.6f; real saved rune snapshots included separately from authored extra-line count", scenarios, minimum)
}
