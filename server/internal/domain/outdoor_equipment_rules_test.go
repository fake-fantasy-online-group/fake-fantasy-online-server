package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func outdoorRuleFixture() *OutdoorEquipmentRule {
	return &OutdoorEquipmentRule{AffixWeights: [6]int64{20, 25, 20, 15, 10, 5}, LegendaryBasePPB: 50_000_000}
}

func TestOutdoorEquipmentRulesValidateCloneAndPreserveSnapshots(t *testing.T) {
	legacy := *lootRulesFixture()
	before, _ := json.Marshal(legacy)
	fresh := legacy
	fresh.Version = 3
	fresh.OutdoorEquipment = outdoorRuleFixture()
	fresh.OutdoorNormalEquipment = &OutdoorEquipmentRule{AffixWeights: [6]int64{46, 20, 15, 10, 5, 2}, LegendaryBasePPB: 20_000_000}
	table, err := NewLootRuleTable(3, []LootRuleSet{legacy, fresh}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	fresh.OutdoorEquipment.AffixWeights[0] = 999
	fresh.OutdoorNormalEquipment.AffixWeights[0] = 999
	active := table.Active()
	if active.OutdoorEquipment.AffixWeights[0] != 20 {
		t.Fatal("constructor mutable alias")
	}
	active.OutdoorNormalEquipment.AffixWeights[0] = 999
	if table.Active().OutdoorNormalEquipment.AffixWeights[0] != 46 {
		t.Fatal("normal configuration clone leaked")
	}
	active.OutdoorEquipment.LegendaryBasePPB = 0
	active.OutdoorEquipment.AffixWeights[0] = 999
	if got := table.Active().OutdoorEquipment; got.LegendaryBasePPB != 50_000_000 || got.AffixWeights[0] != 20 {
		t.Fatal("accessor mutable alias")
	}
	old, _ := table.Version(1)
	after, _ := json.Marshal(old)
	if string(before) != string(after) || old.OutdoorEquipment != nil || old.OutdoorNormalEquipment != nil {
		t.Fatal("legacy snapshot changed")
	}
	if table.Active().UniqueBasePPB != legacy.UniqueBasePPB {
		t.Fatal("shared chance changed")
	}
	fresh.OutdoorNormalEquipment = nil
	for name, mutate := range map[string]func(*OutdoorEquipmentRule){
		"negative weight": func(r *OutdoorEquipmentRule) { r.AffixWeights[0] = -1 },
		"empty":           func(r *OutdoorEquipmentRule) { r.AffixWeights = [6]int64{} },
		"weight overflow": func(r *OutdoorEquipmentRule) { r.AffixWeights[0] = math.MaxInt64 },
		"negative chance": func(r *OutdoorEquipmentRule) { r.LegendaryBasePPB = -1 },
		"chance overflow": func(r *OutdoorEquipmentRule) { r.LegendaryBasePPB = math.MaxInt64 },
		"MF overflow":     func(r *OutdoorEquipmentRule) { r.LegendaryBasePPB = LootProbabilityScaleV5 },
	} {
		t.Run(name, func(t *testing.T) {
			fresh.OutdoorEquipment = outdoorRuleFixture()
			mutate(fresh.OutdoorEquipment)
			if _, err := NewLootRuleTable(3, []LootRuleSet{legacy, fresh}, balanceItemsFixture()); err == nil {
				t.Fatal("invalid accepted")
			}
		})
	}
}

func TestOutdoorEquipmentMagicFindBoundaries(t *testing.T) {
	rule := lootRulesFixture()
	rule.UniqueBasePPB = outdoorRuleFixture().LegendaryBasePPB
	var previous [6]int64
	var previousLegendary int64
	for _, mf := range []int64{math.MinInt64, 0, 1, 100, 200, 400, 800, 1_000_000, math.MaxInt64} {
		p, err := NaturalAffixProbabilitiesV5(outdoorRuleFixture().Counts(), mf)
		if err != nil {
			t.Fatal(err)
		}
		var tail int64
		for k := 5; k >= 0; k-- {
			tail += p[k]
			if p[k] < 0 || tail < previous[k] {
				t.Fatal("MF reduced tail", mf, p)
			}
			previous[k] = tail
		}
		if tail != LootProbabilityScaleV5 {
			t.Fatal("not closed", p)
		}
		l := UniqueProbabilityV5(mf, rule)
		if l < previousLegendary || l < 50_000_000 || l >= 150_000_000 {
			t.Fatal("MF legendary out of bounds", mf, l)
		}
		previousLegendary = l
	}
	for _, tc := range []struct{ mf, want int64 }{{math.MinInt64, 50_000_000}, {0, 50_000_000}, {100, 78_571_428}, {200, 94_444_444}, {math.MaxInt64, 149_999_999}} {
		if got := UniqueProbabilityV5(tc.mf, rule); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	table, def := colorRollTable(), colorRollDef()
	table.Counts[DefaultColorProfile] = outdoorRuleFixture().Counts()
	var histogram [6]int64
	for draw := int64(0); draw < 95; draw++ {
		_, got := table.RollColorFirstEquipment(def, DefaultColorProfile, 0, rule, func(n int64) int64 {
			if n != 95 {
				t.Fatal(n)
			}
			return draw
		}, nil, func(int64) int64 { return 0 })
		histogram[len(got)]++
	}
	if histogram != outdoorRuleFixture().AffixWeights {
		t.Fatal(histogram)
	}
}

func TestOutdoorNormalEquipmentMFAndValidation(t *testing.T) {
	outdoor := &OutdoorEquipmentRule{AffixWeights: [6]int64{46, 20, 15, 10, 5, 2}, LegendaryBasePPB: 20_000_000}
	rule := lootRulesFixture()
	rule.UniqueBasePPB = outdoor.LegendaryBasePPB
	var previous [6]int64
	var previousLegendary int64
	for _, mf := range []int64{math.MinInt64, 0, 1, 100, 200, 400, 800, 1_000_000, math.MaxInt64} {
		p, err := NaturalAffixProbabilitiesV5(outdoor.Counts(), mf)
		if err != nil {
			t.Fatal(err)
		}
		var tail int64
		for k := 5; k >= 0; k-- {
			tail += p[k]
			if p[k] < 0 || tail < previous[k] {
				t.Fatal("MF reduced normal tail", mf, p)
			}
			previous[k] = tail
		}
		if tail != LootProbabilityScaleV5 {
			t.Fatal("not closed", p)
		}
		l := UniqueProbabilityV5(mf, rule)
		if l < previousLegendary || l < 20_000_000 || l >= 60_000_000 {
			t.Fatal("MF normal legendary bounds", mf, l)
		}
		previousLegendary = l
	}
	for _, tc := range []struct{ mf, want int64 }{{0, 20_000_000}, {100, 31_428_571}, {200, 37_777_777}, {math.MaxInt64, 59_999_999}} {
		if got := UniqueProbabilityV5(tc.mf, rule); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	legacy := *lootRulesFixture()
	fresh := legacy
	fresh.Version = 3
	fresh.OutdoorNormalEquipment = outdoor
	for _, bad := range []OutdoorEquipmentRule{{AffixWeights: [6]int64{-1}, LegendaryBasePPB: 20_000_000}, {AffixWeights: [6]int64{}, LegendaryBasePPB: 20_000_000}, {AffixWeights: [6]int64{1}, LegendaryBasePPB: LootProbabilityScaleV5}} {
		fresh.OutdoorNormalEquipment = &bad
		if _, err := NewLootRuleTable(3, []LootRuleSet{legacy, fresh}, balanceItemsFixture()); err == nil {
			t.Fatal("invalid normal override accepted")
		}
	}
}
