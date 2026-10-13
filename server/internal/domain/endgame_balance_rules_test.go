package domain

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestPublishedRiftVersionPinsLegacyAndNewRuns(t *testing.T) {
	base := *riftRulesFixture()
	v2 := cloneRiftRules(base)
	v2.Version = 2
	v2.ExperienceDivisor = 20
	v2.NormalProgress = 99
	v2.TimedMS = 800000
	v2.LootVersion = 2
	loot1 := *lootRulesFixture()
	loot2 := cloneLootRuleSet(loot1)
	loot2.Version = 2
	loot2.Grades[3].SocketWeights = [6]int64{100, 0, 0, 0, 0, 0}
	lootTable, err := NewLootRuleTable(2, []LootRuleSet{loot1, loot2}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	table, err := NewRiftRuleTable(2, []RiftRules{base, v2}, lootTable, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	old, err := NewRiftRun("old", 10, 1, []TrialKeyMember{{CharID: 1, Level: 60}}, 1000, &base)
	if err != nil {
		t.Fatal(err)
	}
	old.RulesVersion = 0
	old.Rules = nil
	if err = old.BindRules(table); err != nil {
		t.Fatal(err)
	}
	if old.RulesVersion != 1 || old.Rules.NormalProgress != 40 || old.Rules.TimedMS != 900000 {
		t.Fatal("legacy run followed new active version")
	}
	fresh, err := NewRiftRun("new", 10, 1, []TrialKeyMember{{CharID: 1, Level: 60}}, 1000, table.Active())
	if err != nil {
		t.Fatal(err)
	}
	if old.Rules.Loot.Version != 1 || old.Rules.Loot.Grades[3].SocketWeights[0] != 15 || fresh.Rules.Loot.Version != 2 || fresh.Rules.Loot.Grades[3].SocketWeights[0] != 100 {
		t.Fatal("active loot version leaked into old run or was ignored by new run")
	}
	if fresh.RulesVersion != 2 || fresh.Rules.NormalProgress != 99 {
		t.Fatal("new run ignored active version")
	}
	copy := table.Active()
	copy.Classes[0].HPBP = 1
	copy.Abilities[0].DamagePct = 1
	copy.Loot.RunePool[0] = 999
	copy.Loot.Grades[0].SocketWeights[0] = 999
	unchanged := table.Active()
	if unchanged.Classes[0].HPBP != 10000 || unchanged.Abilities[0].DamagePct != 140 || unchanged.Loot.RunePool[0] == 999 || unchanged.Loot.Grades[0].SocketWeights[0] == 999 {
		t.Fatal("rift rule accessor leaked mutable children")
	}
	raw, _ := json.Marshal(fresh)
	var restored RiftRun
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err = restored.BindRules(table); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Rules, fresh.Rules) {
		t.Fatal("rule snapshot changed on serialization")
	}
	restored.Rules.ExperienceDivisor++
	if err = restored.BindRules(table); err == nil {
		t.Fatal("tampered published snapshot accepted")
	}
	if got := RiftBossExperience(1000, 10, fresh.Rules); got != 500 {
		t.Fatal("configured experience divisor ignored", got)
	}
	if got := RiftBossExperience(math.MaxInt64, 150, fresh.Rules); got != math.MaxInt64 {
		t.Fatal("experience overflow not saturated")
	}
}
func TestLootConfigurationIsConsumedAndImmutable(t *testing.T) {
	base := *lootRulesFixture()
	v2 := cloneLootRuleSet(base)
	v2.Version = 2
	v2.MinRuneLevel = 30
	v2.Grades[0].RuneRateBP = 10000
	v2.Grades[0].SocketWeights = [6]int64{0, 0, 100, 0, 0, 0}
	v2.UniqueBasePPB = 20000000
	table, err := NewLootRuleTable(2, []LootRuleSet{base, v2}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	rule := table.Active()
	if rule.MinRuneLevel != 30 || rule.RuneDrop(AstralDropOrdinary).RatePct != 100 {
		t.Fatal("changed drop row ignored")
	}
	def := ItemDef{Equip: &EquipDef{Slot: 4, LevelReq: 50}}
	if got := RollDroppedSocketCount(def, AstralDropOrdinary, func(int64) int64 { return 0 }, rule); got != 2 {
		t.Fatal("configured socket weights ignored", got)
	}
	if got := UniqueProbabilityV5(0, rule); got != 20000000 {
		t.Fatal("live unique chance ignores database", got)
	}
	rule.RunePool[0] = 999
	rule.Grades[0].SocketWeights[0] = 100
	if table.Active().RunePool[0] == 999 || table.Active().Grades[0].SocketWeights[0] != 0 {
		t.Fatal("mutable rule snapshot leaked")
	}
	old, _ := table.Version(1)
	if old.Grades[0].RuneRateBP != 60 {
		t.Fatal("active changed old version")
	}
}
func TestBalanceConfigurationRejectsMissingOrCorruptVersions(t *testing.T) {
	items := balanceItemsFixture()
	loot := *lootRulesFixture()
	rift := *riftRulesFixture()
	bad := cloneLootRuleSet(loot)
	bad.Grades[0].SocketWeights[0]++
	if _, err := NewLootRuleTable(1, []LootRuleSet{bad}, items); err == nil {
		t.Fatal("bad probability accepted")
	}
	if _, err := NewLootRuleTable(2, []LootRuleSet{loot}, items); err == nil {
		t.Fatal("missing active accepted")
	}
	badR := cloneRiftRules(rift)
	badR.LootVersion = 2
	if _, err := NewRiftRuleTable(1, []RiftRules{badR}, lootRulesTableFixture(), items); err == nil {
		t.Fatal("missing pinned loot accepted")
	}
	badR = cloneRiftRules(rift)
	badR.ExperienceDivisor = 0
	if _, err := NewRiftRuleTable(1, []RiftRules{badR}, lootRulesTableFixture(), items); err == nil {
		t.Fatal("zero divisor accepted")
	}
	if _, err := NewRiftRun("missing", 1, 1, []TrialKeyMember{{CharID: 1, Level: 60}}, 1000, nil); err == nil {
		t.Fatal("missing rules silently defaulted")
	}
}

func TestRiftRulesRejectUnsafeMathAndWireValues(t *testing.T) {
	cases := map[string]func(*RiftRules){
		"chance overflow":               func(r *RiftRules) { r.UniqueBaseChance = math.MaxInt32 },
		"timer overflow":                func(r *RiftRules) { r.AbilityStartMS = math.MaxInt64 },
		"timer multiplication overflow": func(r *RiftRules) { r.AbilityClassReductionMS = math.MaxInt64 },
		"zero line length":              func(r *RiftRules) { r.Abilities[2].Length = 0 },
		"ability name":                  func(r *RiftRules) { r.Abilities[0].Name = strings.Repeat("界", 129) },
		"affix UTF16":                   func(r *RiftRules) { r.Abilities[0].Affix = strings.Repeat("😀", 33) },
		"large snapshot": func(r *RiftRules) {
			r.Name = strings.Repeat("界", 8192)
			r.BossName = strings.Repeat("界", 8192)
			r.BlueOrbs = 32
			r.GoldOrbs = 32
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := riftRulesFixture()
			mutate(r)
			if err := r.Validate(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestBalanceV1PreservesIndependentLegacyAlgorithms(t *testing.T) {
	r, l := riftRulesFixture(), lootRulesFixture()
	if r.ProgressMax != 10000 || r.NormalProgress != 40 || r.GlobeProgress != 200 || r.BlueOrbs != 1 || r.GoldOrbs != 4 || r.TimedMS != 900000 || r.HardMS != 1200000 || r.ReconnectMS != 120000 || r.DeathPenaltyMS != 5000 || r.PresenceBP != 5000 || r.TierJumpSeconds != 180 || r.GuaranteedUniqueTier != 10 || l.MinRuneLevel != 20 {
		t.Fatal("legacy v1 progression changed")
	}
	for i, rate := range []int32{60, 800, 2000, 0} {
		if l.Grades[i].RuneRateBP != rate {
			t.Fatal("legacy rune drop rate changed")
		}
	}
	for tier := int32(1); tier <= 150; tier++ {
		for _, timely := range []bool{false, true} {
			if PlanRiftRewards(tier, timely, r) != legacyBalancePlanRiftRewards(tier, timely) {
				t.Fatal("legacy reward changed", tier, timely)
			}
		}
		for _, base := range []int32{1, 100, 1000000} {
			for party := 1; party <= 5; party++ {
				for class := int32(0); class <= 3; class++ {
					for _, hp := range []bool{false, true} {
						if RiftScaledStat(base, tier, party, class, hp, r) != legacyBalanceRiftScaledStat(base, tier, party, class, hp) {
							t.Fatal("legacy scaled stat changed", base, tier, party, class, hp)
						}
					}
				}
			}
		}
	}
	for level := int32(0); level <= 70; level++ {
		for grade := AstralDropOrdinary; grade <= AstralDropRealm; grade++ {
			for roll := int64(0); roll < 100; roll++ {
				def := ItemDef{Equip: &EquipDef{Slot: 4, LevelReq: level}}
				rng := func(int64) int64 { return roll }
				if RollDroppedSocketCount(def, grade, rng, l) != legacyBalanceRollDroppedSocketCount(def, grade, rng) {
					t.Fatal("legacy socket changed", level, grade, roll)
				}
			}
		}
	}
	for _, mf := range []int64{-1, 0, 1, 100, 250, 10000, math.MaxInt64} {
		if UniqueProbabilityV5(mf, l) != legacyBalanceUniqueProbabilityV5(mf) {
			t.Fatal("legacy MF chance changed", mf)
		}
	}
}
