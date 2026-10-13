package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type balanceReference struct {
	Arm  []map[string]json.RawMessage
	Desc []map[string]json.RawMessage
}

func fixtureInt(row map[string]json.RawMessage, key string) int32 {
	var n int32
	_ = json.Unmarshal(row[key], &n)
	return n
}
func authoredBalanceSource(t *testing.T) (map[ItemID]UniqueDefinition, map[ItemID]map[string]json.RawMessage, map[[4]int32]int32) {
	t.Helper()
	root := "../../../docs/daoist-endgame"
	var input balanceReference
	raw, e := os.ReadFile(root + "/balance/baseline_fixture_inputs.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &input); e != nil {
		t.Fatal(e)
	}
	arms := map[ItemID]map[string]json.RawMessage{}
	types := map[ItemID]int32{}
	for _, row := range input.Desc {
		types[ItemID(fixtureInt(row, "index"))] = fixtureInt(row, "type")
	}
	for _, row := range input.Arm {
		id := ItemID(fixtureInt(row, "index"))
		b, _ := json.Marshal(types[id])
		row["type"] = b
		arms[id] = row
	}
	var pool []struct{ Tier, Type, Attr, Mode, P90 int32 }
	raw, e = os.ReadFile(root + "/balance/yellow_affix_benchmarks.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &pool); e != nil {
		t.Fatal(e)
	}
	p90 := map[[4]int32]int32{}
	for _, p := range pool {
		p90[[4]int32{p.Tier, p.Type, p.Attr, p.Mode}] = p.P90
	}
	authored := map[ItemID]UniqueDefinition{}
	files, _ := filepath.Glob(root + "/catalog/authored/*.json")
	for _, file := range files {
		raw, e = os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var rows []UniqueDefinition
		if e = json.Unmarshal(raw, &rows); e != nil {
			t.Fatal(e)
		}
		for _, d := range rows {
			authored[d.BaseID] = d
		}
	}
	return authored, arms, p90
}
func fixtureBuild(t *testing.T, race Race, level int32, a map[string]json.RawMessage) (Base, Stats) {
	t.Helper()
	base := StartingBase(race)
	for n := int32(2); n <= level; n++ {
		base = base.Add(GrowthAt(race, n))
	}
	points := 2 * (level - 1)
	for _, v := range []struct {
		ptr *int32
		key string
	}{{&base.STR, "str_need"}, {&base.VIT, "vit_need"}, {&base.INT, "int_need"}, {&base.SPI, "spi_need"}, {&base.AGI, "agi_need"}, {&base.DEX, "dex_need"}} {
		delta := max(int32(0), fixtureInt(a, v.key)-*v.ptr)
		*v.ptr += delta
		points -= delta
	}
	if points < 0 {
		t.Fatal("invalid gear requirement fixture")
	}
	if race < Healer {
		base.STR += points
	} else {
		base.INT += points
	}
	base = base.Add(Base{STR: fixtureInt(a, "str"), VIT: fixtureInt(a, "vit"), INT: fixtureInt(a, "int_"), SPI: fixtureInt(a, "spi"), AGI: fixtureInt(a, "agi"), DEX: fixtureInt(a, "dex")})
	innate := Stats{MinAtk: fixtureInt(a, "min_atk"), MaxAtk: fixtureInt(a, "max_atk"), MAtk: fixtureInt(a, "matk"), Def: fixtureInt(a, "def"), MDef: fixtureInt(a, "mdef"), Hit: fixtureInt(a, "hit_rate")}
	return base, innate
}
func fixtureYellow(t *testing.T, race Race, a map[string]json.RawMessage, p map[[4]int32]int32) []Affix {
	t.Helper()
	keys := [][2]int32{{47, 1}, {29, 0}, {31, 0}, {1, 0}, {19, 1}}
	if race >= Healer {
		keys = [][2]int32{{15, 1}, {3, 0}, {61, 0}, {34, 0}, {17, 0}}
	}
	var out []Affix
	for _, k := range keys {
		v, ok := p[[4]int32{fixtureInt(a, "level"), fixtureInt(a, "type"), k[0], k[1]}]
		if !ok {
			t.Fatal("not a real yellow source pool", a, k)
		}
		out = append(out, Affix{Attr: k[0], Value: v, Mode: k[1]})
	}
	return out
}
func fixtureOutput(base Base, innate Stats, affixes []Affix, traits []UniqueTrait, ctx UniqueContext, magic bool) float64 {
	_, stats := Compute(StatSource{Base: base, Innate: innate, Passive: affixes})
	panel := float64(stats.MinAtk+stats.MaxAtk) / 2
	if magic {
		panel = float64(stats.MAtk)
	}
	return panel * (1 + float64(EvaluateUniqueTraits(traits, ctx).DamageBP)/10000)
}

// These are designated offensive specialists, not a blanket requirement for
// niche/utility uniques. The strong yellow uses five compatible per-line P90
// values, never a claim to represent a median drop or a whole-item percentile.
// Comparisons execute Compute and EvaluateUniqueTraits with original SQL base
// stats and actual class growth; no label/string uniqueness can satisfy them.
func TestAuthoredOffensiveSpecialistsEarnFreshDropPurpose(t *testing.T) {
	authored, arms, p90 := authoredBalanceSource(t)
	cases := []struct {
		id   ItemID
		race Race
		ctx  UniqueContext
	}{
		{1610, Warrior, UniqueContext{HPPercent: 25, TargetHPPercent: 100}},
		{1010, Swordsman, UniqueContext{HPPercent: 100, TargetHPPercent: 100}},
		{1806, Assassin, UniqueContext{HPPercent: 25, TargetHPPercent: 100}},
		{1810, Assassin, UniqueContext{HPPercent: 100, TargetHPPercent: 100, TargetElite: true}},
		{1310, Warlock, UniqueContext{HPPercent: 100, TargetHPPercent: 100, Skill: true, SkillID: 10401, Magic: true, HurtType: HurtFire}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			d, ok := authored[tc.id]
			if !ok {
				t.Fatal("designated specialist missing authored source")
			}
			a := arms[tc.id]
			base, innate := fixtureBuild(t, tc.race, fixtureInt(a, "level_need"), a)
			yellow := fixtureYellow(t, tc.race, a, p90)
			y := fixtureOutput(base, innate, yellow, nil, tc.ctx, tc.race >= Healer)
			reference := append(ResolveVariableAffixes(d.Affixes, d.VariableAffixes, 10000), ResolveItemLocalEnhancements(&EquipDef{Bonus: innate}, d.LocalEnhancements, 10000)...)
			fresh := fixtureOutput(base, innate, reference, d.Traits, tc.ctx, tc.race >= Healer)
			converted := fixtureOutput(base, innate, append(append([]Affix{}, yellow...), reference...), d.Traits, tc.ctx, tc.race >= Healer)
			t.Logf("%s fresh=%.2f strong-yellow=%.2f converted=%.2f", d.Name, fresh, y, converted)
			for _, roll := range []int32{8000, 10000, 12000} {
				scaled := append(ResolveVariableAffixes(d.Affixes, d.VariableAffixes, roll), ResolveItemLocalEnhancements(&EquipDef{Bonus: innate}, d.LocalEnhancements, roll)...)
				traits := append([]UniqueTrait{}, d.Traits...)
				off := tc.ctx
				off.HPPercent = 100
				off.TargetHPPercent = 50
				off.TargetElite = false
				off.Invisible = false
				for _, condition := range []struct {
					name string
					ctx  UniqueContext
				}{{"intended", tc.ctx}, {"off_condition", off}} {
					freshOutput := fixtureOutput(base, innate, scaled, traits, condition.ctx, tc.race >= Healer)
					convertedAffixes := append(append([]Affix{}, yellow...), scaled...)
					convertedOutput := fixtureOutput(base, innate, convertedAffixes, traits, condition.ctx, tc.race >= Healer)
					_, freshStats := Compute(StatSource{Base: base, Innate: innate, Passive: scaled})
					_, convertedStats := Compute(StatSource{Base: base, Innate: innate, Passive: convertedAffixes})
					power := EvaluateUniqueTraits(traits, condition.ctx)
					t.Logf("BALANCE\t%d\t%s\t%d\t%s\t%.2f\t%.2f\t%.2f\t%.3f\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d", tc.id, d.Name, roll, condition.name, y, freshOutput, convertedOutput, convertedOutput/y, condition.ctx.HPPercent, condition.ctx.TargetHPPercent, power.DamageBP, freshStats.MaxHP, convertedStats.MaxHP, freshStats.MDef, convertedStats.MDef, freshStats.PhysResist+freshStats.MagicResist)
				}
			}
			if fresh < y {
				t.Errorf("%s %s fails its intended fresh-drop offense condition: %.2f < %.2f; redistribute actual useful power", d.Name, d.PowerTier, fresh, y)
			}
			if converted < y || converted < fresh {
				t.Fatal("converted overlay lost expected yellow benefits")
			}
		})
	}
}
func TestFiveRunePhysicalSpecialistCompetesWithStrongYellow(t *testing.T) {
	_, arms, p90 := authoredBalanceSource(t)
	a := arms[1610]
	base, innate := fixtureBuild(t, Warrior, 54, a)
	yellow := fixtureYellow(t, Warrior, a, p90)
	r, _ := AstralRecipeByID(31, testEndgameCatalog())
	affixes := append([]Affix{}, r.Affixes...)
	for _, id := range r.Runes {
		rune, _ := AstralRune(id, testEndgameCatalog())
		affixes = append(affixes, rune.Affixes...)
	}
	ctx := UniqueContext{HPPercent: 100, TargetHPPercent: 100}
	y := fixtureOutput(base, innate, yellow, nil, ctx, false)
	formed := fixtureOutput(base, innate, affixes, r.Traits, ctx, false)
	t.Logf("five-rune %s intended basic-hit output=%.2f strong-yellow=%.2f", r.Name, formed, y)
	if formed < y {
		t.Fatalf("five-rune offensive investment failed useful yellow baseline: %.2f < %.2f", formed, y)
	}
}
