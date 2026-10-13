package domain

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func weightedRuneLootFixture() LootRuleSet {
	r := *lootRulesFixture()
	r.Version = 2
	for i := range r.RunePool {
		r.RuneWeights = append(r.RuneWeights, int64(129-4*i))
	}
	return r
}

func TestRuneLootArithmeticWeightsExhaustiveBoundaries(t *testing.T) {
	r := weightedRuneLootFixture()
	table, err := NewLootRuleTable(2, []LootRuleSet{*lootRulesFixture(), r}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	r = *table.Active()
	counts := map[ItemID]int{}
	for roll := 0; roll < 2100; roll++ {
		calls := 0
		id, ok := r.RollRune(func(n int) int {
			calls++
			if n != 2100 {
				t.Fatalf("wrong selection denominator: %d", n)
			}
			return roll
		})
		if !ok || calls != 1 {
			t.Fatalf("roll %d: id=%d ok=%v calls=%d", roll, id, ok, calls)
		}
		counts[id]++
	}
	boundary := 0
	var previousLevel int32
	for i, id := range r.RunePool {
		rune, ok := AstralRune(id, testEndgameCatalog())
		if !ok || rune.MinLevel <= previousLevel {
			t.Fatalf("rune pool is not in strict level order at %d: %+v", i, rune)
		}
		previousLevel = rune.MinLevel
		weight := 129 - 4*i
		if counts[id] != weight {
			t.Fatalf("rune %d: got %d outcomes, want %d", id, counts[id], weight)
		}
		for _, roll := range []int{boundary, boundary + weight - 1} {
			got, ok := r.RollRune(func(int) int { return roll })
			if !ok || got != id {
				t.Fatalf("rune %d boundary %d selected %d", id, roll, got)
			}
		}
		boundary += weight
	}
	if counts[LastAstralRune]*100 != boundary || previousLevel != 60 {
		t.Fatalf("highest level must have exactly 1%%: outcomes=%d/%d level=%d", counts[LastAstralRune], boundary, previousLevel)
	}
	if !reflect.DeepEqual(r.Grades, lootRulesFixture().Grades) {
		t.Fatal("rune occurrence rates or socket weights changed")
	}
}

func TestRuneLootWeightsUseConfigurationAndPreserveLegacyRNG(t *testing.T) {
	legacy := lootRulesFixture()
	actual, expected := rand.New(rand.NewSource(771)), rand.New(rand.NewSource(771))
	for i := 0; i < 100; i++ {
		id, ok := legacy.RollRune(actual.Intn)
		if want := legacy.RunePool[expected.Intn(len(legacy.RunePool))]; !ok || id != want {
			t.Fatalf("legacy selection sequence changed: got %d want %d", id, want)
		}
	}
	r := weightedRuneLootFixture()
	r.RuneWeights[0], r.RuneWeights[27] = 1, 1000000
	table, err := NewLootRuleTable(2, []LootRuleSet{*legacy, r}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	id, ok := table.Active().RollRune(func(int) int { return 1 })
	if !ok || id != r.RunePool[1] {
		t.Fatal("runtime replaced configured weights with a hardcoded curve")
	}
	r.RuneWeights[0] = 999
	copy := table.Active()
	copy.RuneWeights[0] = 777
	if table.Active().RuneWeights[0] != 1 {
		t.Fatal("loot rules leaked mutable weight slice")
	}
}

func TestRuneLootWeightsRejectInvalidRowsAndDraws(t *testing.T) {
	for name, mutate := range map[string]func(*LootRuleSet){
		"short":    func(r *LootRuleSet) { r.RuneWeights = r.RuneWeights[:27] },
		"long":     func(r *LootRuleSet) { r.RuneWeights = append(r.RuneWeights, 1) },
		"zero":     func(r *LootRuleSet) { r.RuneWeights[0] = 0 },
		"negative": func(r *LootRuleSet) { r.RuneWeights[0] = -1 },
		"unsafe":   func(r *LootRuleSet) { r.RuneWeights[0] = 1000001 },
	} {
		t.Run(name, func(t *testing.T) {
			r := weightedRuneLootFixture()
			mutate(&r)
			if _, err := NewLootRuleTable(2, []LootRuleSet{*lootRulesFixture(), r}, balanceItemsFixture()); err == nil {
				t.Fatal("invalid weights accepted")
			}
			if _, ok := r.RollRune(func(int) int { t.Fatal("invalid rules consumed RNG"); return 0 }); ok {
				t.Fatal("invalid rules selected a rune")
			}
		})
	}
	weighted := weightedRuneLootFixture()
	for _, r := range []*LootRuleSet{nil, {}, lootRulesFixture(), &weighted} {
		for _, roll := range []func(int) int{nil, func(int) int { return -1 }, func(n int) int { return n }} {
			if _, ok := r.RollRune(roll); ok {
				t.Fatal("missing rule or invalid random draw selected a rune")
			}
		}
	}
}

func TestWeightedRuneRiftSnapshotsPinOldRunsAndRoundTrip(t *testing.T) {
	legacyLoot, weightedLoot := *lootRulesFixture(), weightedRuneLootFixture()
	loot, err := NewLootRuleTable(2, []LootRuleSet{legacyLoot, weightedLoot}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	v1 := *riftRulesFixture()
	v2 := cloneRiftRules(v1)
	v2.Version, v2.FloorPopulationMultiplier = 2, 2
	v3 := cloneRiftRules(v2)
	v3.Version, v3.LootVersion = 3, 2
	table, err := NewRiftRuleTable(3, []RiftRules{v1, v2, v3}, loot, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int32{1, 2, 3} {
		rules, _ := table.Version(version)
		run, err := NewRiftRun("weighted-snapshot", 10, 1, []TrialKeyMember{{CharID: 1, Level: 60}}, 1000, rules)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "RuneWeights") != (version == 3) {
			t.Fatal("new weight field changed legacy snapshot shape")
		}
		var restored RiftRun
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		if err := restored.BindRules(table); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restored.Rules, run.Rules) {
			t.Fatal("persisted run adopted a different loot distribution")
		}
		if version == 3 {
			if restored.Rules.FloorPopulationMultiplier != 2 || restored.Rules.Loot.RuneWeights[27] != 21 {
				t.Fatal("new admissions lost doubled density or weighted runes")
			}
			restored.Rules.Loot.RuneWeights[0]++
			if err := restored.BindRules(table); err == nil {
				t.Fatal("tampered rune weights accepted")
			}
			if table.Active().Loot.RuneWeights[0] != 129 {
				t.Fatal("snapshot leaked nested weights")
			}
		} else if len(restored.Rules.Loot.RuneWeights) != 0 || restored.Rules.LootVersion != 1 {
			t.Fatal("already-running rift changed rune distribution")
		}
	}
}
