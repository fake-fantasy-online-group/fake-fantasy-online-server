package domain

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestLegendaryPowerRollFrozenGeneratorKeepsEveryDraw(t *testing.T) {
	def, table := sourceLevelFixture(t)
	for _, policy := range []int32{0, LegendaryPowerPolicySplit} {
		for _, variants := range []bool{false, true} {
			for seed := int64(0); seed < 100; seed++ {
				a := NewStack(def, 1)
				b := a
				ra, rb := rand.New(rand.NewSource(seed)), rand.New(rand.NewSource(seed))
				var drawsA, drawsB []int
				fa := func(n int) int { drawsA = append(drawsA, n); return ra.Intn(n) }
				fb := func(n int) int { drawsB = append(drawsB, n); return rb.Intn(n) }
				old := table.ApplyLegendaryForRecipientWithPolicyAtLevel(&a, def, 43, 16, policy, variants, fa)
				now := table.ApplyLegendaryForRecipientWithGenerationAtLevel(&b, def, 43, 16, policy, variants, false, fb)
				if old != now || a != b || b.Legendary.PowerRollBP != 0 || !reflect.DeepEqual(drawsA, drawsB) || ra.Int63() != rb.Int63() {
					t.Fatal("frozen stream drift", policy, variants, seed)
				}
			}
		}
	}
}

func TestLegendaryPowerRollNewGeneratorsEndpointsAndInvalidDrawAtomicity(t *testing.T) {
	def, table := legendaryRollFixture(t)
	def.InstanceKind = ItemInstanceEquipment
	old := legendaryExecutable
	legendaryExecutable = map[string]bool{"TY-A02": true}
	t.Cleanup(func() { legendaryExecutable = old })
	generators := []struct {
		name string
		run  func(*Stack, func(int) int) bool
	}{
		{"new frozen drop", func(st *Stack, rng func(int) int) bool {
			return table.ApplyLegendaryForRecipientWithGenerationAtLevel(st, def, 100, 1, LegendaryPowerPolicySplit, true, true, rng)
		}},
		{"cube policy awakening", func(st *Stack, rng func(int) int) bool {
			return table.ApplyLegendaryForRecipientWithPolicy(st, def, 1, LegendaryPowerPolicySplit, rng)
		}},
		{"awakening", func(st *Stack, rng func(int) int) bool { return table.ApplyLegendary(st, def, rng) }},
		{"recipient awakening", func(st *Stack, rng func(int) int) bool { return table.ApplyLegendaryForRecipient(st, def, 1, rng) }},
		{"explicit new source", func(st *Stack, rng func(int) int) bool { return table.ApplyLegendaryAtLevel(st, def, 100, rng) }},
	}
	for _, g := range generators {
		t.Run(g.name, func(t *testing.T) {
			for _, high := range []bool{false, true} {
				st := NewStack(def, 1)
				if !g.run(&st, func(n int) int {
					if high {
						return n - 1
					}
					return 0
				}) {
					t.Fatal("generation failed")
				}
				want := LegendaryPowerRollMinBP
				if high {
					want = LegendaryPowerRollMaxBP
				}
				if st.Legendary.PowerRollBP != want || st.Legendary.Validate() != nil {
					t.Fatal("wrong endpoint", st.Legendary)
				}
			}
			st := NewStack(def, 1)
			before := st
			if g.run(&st, func(n int) int {
				if n == 2501 {
					return n
				}
				return 0
			}) || st != before {
				t.Fatal("bad power RNG committed partial item")
			}
		})
	}
	st := NewStack(def, 1)
	before := st
	if table.ApplyLegendaryForRecipientWithGenerationAtLevel(&st, def, 100, 1, 1, false, true, func(int) int { t.Fatal("invalid generation policy drew RNG"); return 0 }) || st != before {
		t.Fatal("invalid flags accepted")
	}
}

func TestLegendaryPowerRollRuleVersionAndHistoricalJSON(t *testing.T) {
	legacy := *lootRulesFixture()
	raw, err := json.Marshal(legacy)
	if err != nil || strings.Contains(string(raw), "LegendaryPowerRolls") {
		t.Fatal("legacy snapshot shape changed", string(raw), err)
	}
	var restored LootRuleSet
	if json.Unmarshal(raw, &restored) != nil || restored.LegendaryPowerRolls {
		t.Fatal("legacy flag default changed")
	}
	for _, tc := range []struct {
		version      int32
		rolls, valid bool
	}{{4, false, true}, {4, true, false}, {5, false, false}, {5, true, true}, {6, true, true}} {
		r := legacy
		r.Version = tc.version
		r.RetiredCubeMaterials = tc.version >= 6
		r.LegendaryPowerPolicy = 1
		r.LegendaryPowerRolls = tc.rolls
		table, err := NewLootRuleTable(tc.version, []LootRuleSet{legacy, r}, balanceItemsFixture())
		if (err == nil) != tc.valid {
			t.Fatalf("version%d flag%v validity%v: %v", tc.version, tc.rolls, tc.valid, err)
		}
		if err == nil && table.Active().LegendaryPowerRolls != tc.rolls {
			t.Fatal("clone lost flag")
		}
	}
}
