package domain

import (
	"math"
	"reflect"
	"testing"
)

func TestMagicFindV5GoldenProbabilities(t *testing.T) {
	for _, tc := range []struct {
		mf       int64
		ordinary [6]int64
		elite    [6]int64
		unique   int64
	}{
		{0, [6]int64{700000000, 200000000, 100000000}, [6]int64{606060607, 202020202, 101010101, 50505050, 30303030, 10101010}, 10000000},
		{100, [6]int64{636363637, 234604105, 129032258}, [6]int64{535714286, 223779385, 122859271, 64490912, 39733328, 13422818}, 15714285},
		{200, [6]int64{608695653, 248447205, 142857142}, [6]int64{506329114, 230998075, 132238029, 71028842, 44330564, 15075376}, 18888888},
		{400, [6]int64{583333334, 260416666, 156250000}, [6]int64{480000000, 236417911, 140724947, 77283372, 48851362, 16722408}, 22307692},
		{800, [6]int64{564516130, 268817204, 166666666}, [6]int64{460829494, 239695901, 146932233, 82092275, 52414025, 18036072}, 25238095},
	} {
		for profile, want := range map[ColorProfile][6]int64{1: tc.elite, 2: tc.ordinary} {
			got, err := NaturalAffixProbabilitiesV5(colorRollTable().CountsFor(profile), tc.mf)
			if err != nil || got != want {
				t.Fatalf("MF%d profile%d: got %v err %v want %v", tc.mf, profile, got, err, want)
			}
		}
		if got := UniqueProbabilityV5(tc.mf, lootRulesFixture()); got != tc.unique {
			t.Fatalf("MF%d unique=%d want=%d", tc.mf, got, tc.unique)
		}
	}
}

func TestMagicFindV5MonotoneClosedAndOverflowSafe(t *testing.T) {
	for _, profile := range []ColorProfile{1, 2} {
		var previous [6]int64
		for _, mf := range []int64{0, 1, 100, 200, 400, 800, 1_000_000, math.MaxInt64} {
			p, err := NaturalAffixProbabilitiesV5(colorRollTable().CountsFor(profile), mf)
			if err != nil {
				t.Fatal(err)
			}
			var tail int64
			for k := 5; k >= 0; k-- {
				if p[k] < 0 {
					t.Fatal("negative probability", p)
				}
				tail += p[k]
				if tail < previous[k] {
					t.Fatal("MF reduced a tail", mf, k)
				}
				previous[k] = tail
			}
			if tail != LootProbabilityScaleV5 || profile == 2 && p[3]+p[4]+p[5] != 0 {
				t.Fatal("probability support or closure changed", p)
			}
		}
	}
	if got := UniqueProbabilityV5(math.MaxInt64, lootRulesFixture()); got != 29_999_999 {
		t.Fatal("unique asymptote overflow", got)
	}
	if UniqueProbabilityV5(math.MinInt64, lootRulesFixture()) != UniqueProbabilityV5(0, lootRulesFixture()) {
		t.Fatal("negative MF not clamped")
	}
	for _, counts := range [][]WeightedAffixCount{
		{}, {{0, -1}}, {{6, 1}}, {{0, 1}, {0, 2}}, {{0, math.MaxInt64}, {1, 1}},
	} {
		if _, err := NaturalAffixProbabilitiesV5(counts, 100); err == nil {
			t.Fatal("accepted invalid distribution", counts)
		}
	}
	if _, err := NaturalAffixProbabilitiesV5([]WeightedAffixCount{{0, math.MaxInt64 - 1}, {5, 1}}, math.MaxInt64); err != nil {
		t.Fatal("safe large inputs rejected", err)
	}
}

func TestMagicFindV5ZeroPreservesExactElite99Weights(t *testing.T) {
	table, def := colorRollTable(), colorRollDef()
	var histogram [6]int
	for draw := int64(0); draw < 99; draw++ {
		got := table.RollWithMagicFindV5(def, 1, 0, 0, func(n int64) int64 {
			if n != 99 {
				t.Fatalf("baseline uses %d-part weights instead of99", n)
			}
			return draw
		}, func(int64) int64 { return 0 })
		histogram[len(got)]++
	}
	if histogram != [6]int{60, 20, 10, 5, 3, 1} {
		t.Fatal(histogram)
	}
}

func TestMagicFindV5QuotaDistinctOptionsAndUnchangedValues(t *testing.T) {
	table, def := colorRollTable(), colorRollDef()
	last := func(n int64) int64 { return n - 1 }
	first := func(int64) int64 { return 0 }
	for fixed := 0; fixed <= 5; fixed++ {
		def.Equip.Affixes = make([]Affix, fixed)
		for sockets := 0; sockets <= 5-fixed; sockets++ {
			got := table.RollWithMagicFindV5(def, 1, uint8(sockets), math.MaxInt64, last, first)
			if len(got) != 5-fixed-sockets {
				t.Fatal("fixed/socket quota broken", fixed, sockets, got)
			}
		}
	}
	def.Equip.Affixes = nil
	base := table.RollWithMagicFindV5(def, 1, 0, 0, last, seqIntn())
	boosted := table.RollWithMagicFindV5(def, 1, 0, 800, last, seqIntn())
	if !reflect.DeepEqual(base, boosted) {
		t.Fatal("MF changed affix identities or values for equal counts", base, boosted)
	}
	key := EquipmentRollKey{Level: 1, Type: 101}
	table.Options[key] = table.Options[key][:1]
	if n := len(table.RollWithMagicFindV5(def, 1, 0, 800, last, first)); n != 1 {
		t.Fatal("fabricated unavailable attributes", n)
	}
	if n := len(colorRollTable().RollWithMagicFindV5(def, 2, 0, math.MaxInt64, last, first)); n != 2 {
		t.Fatal("ordinary support changed", n)
	}
}

func TestMagicFindV5UniqueEligibilityThresholdAndVariableIsolation(t *testing.T) {
	var unique UniqueDefinition
	for _, u := range UniqueCatalog(testEndgameCatalog()) {
		if u.NativeDropEligible {
			unique = u
			break
		}
	}
	if unique.BaseID == 0 {
		t.Fatal("no natural unique fixture")
	}
	def := ItemDef{EndgameCatalog: testEndgameCatalog(), ID: unique.BaseID, InstanceKind: ItemInstanceEquipment, Equip: &EquipDef{}}
	newStack := func() Stack {
		return Stack{UID: 17, Item: def.ID, Count: 1, SocketCount: 2, RolledAffixCount: 1, RolledAffixes: [5]InstanceAffix{{Affix: Affix{Attr: AttrSTR, Value: 7}}}}
	}
	for _, mf := range []int64{0, 100, 800, math.MaxInt64} {
		threshold := UniqueProbabilityV5(mf, lootRulesFixture())
		for _, draw := range []int64{threshold - 1, threshold, LootProbabilityScaleV5 - 1} {
			st := newStack()
			variableCalls := 0
			ok := RollNaturalUniqueWithMagicFindV5(&st, def, mf, func(int64) int64 { return draw }, func(int) int { variableCalls++; return 1234 }, lootRulesFixture())
			if ok != (draw < threshold) || variableCalls != boolInt(ok) {
				t.Fatal("chance threshold/variable calls", mf, draw, ok, variableCalls)
			}
			if ok && st.Endgame.UniqueRollBP != UniqueRollMinBP+1234 || st.SocketCount != 2 || st.RolledAffixCount != 1 || st.RolledAffixes[0] != newStack().RolledAffixes[0] {
				t.Fatal("unique mutated its existing natural layer", st)
			}
		}
	}
	for _, ineligible := range []ItemDef{{ID: 999999999, Equip: &EquipDef{}}, {ID: def.ID, Equip: &EquipDef{NoTypeDrop: true}}, {ID: def.ID}} {
		st := Stack{UID: 17, Item: ineligible.ID, Count: 1}
		called := false
		if RollNaturalUniqueWithMagicFindV5(&st, ineligible, 800, func(int64) int64 { called = true; return 0 }, func(int) int { return 0 }, lootRulesFixture()) || called {
			t.Fatal("ineligible source reached RNG", ineligible)
		}
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
