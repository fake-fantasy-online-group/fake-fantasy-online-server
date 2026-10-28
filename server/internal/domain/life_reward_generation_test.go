package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLifeRewardVersionPolicy(t *testing.T) {
	legacy := *lootRulesFixture()
	raw, _ := json.Marshal(legacy)
	if strings.Contains(string(raw), "RetiredCubeMaterials") {
		t.Fatal("historical JSON changed")
	}
	for _, tc := range []struct {
		version     int32
		flag, valid bool
	}{{5, false, true}, {5, true, false}, {6, false, false}, {6, true, true}} {
		r := legacy
		r.Version = tc.version
		r.LegendaryPowerPolicy = 1
		r.LegendaryPowerRolls = true
		r.RetiredCubeMaterials = tc.flag
		table, err := NewLootRuleTable(r.Version, []LootRuleSet{legacy, r}, balanceItemsFixture())
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
		if err == nil && table.Active().RetiredCubeMaterials != tc.flag {
			t.Fatal("clone lost policy")
		}
	}
	for _, id := range []ItemID{CubeStarMarrow, CubePurifiedDust, CubeImmortalEcho} {
		if !legacy.AllowsCombatMaterial(id) || (&LootRuleSet{RetiredCubeMaterials: true}).AllowsCombatMaterial(id) {
			t.Fatal("retired material policy", id)
		}
	}
	if !(&LootRuleSet{RetiredCubeMaterials: true}).AllowsCombatMaterial(FirstAstralRune) {
		t.Fatal("star reward lost")
	}
}

func TestLifeRewardPlanOnlyRemovesDust(t *testing.T) {
	old := &RiftRules{MaxTier: 150, DustItem: CubePurifiedDust, DustBase: 10, DustTierDivisor: 5, TimelyDust: 5, Runes: 2, TimelyRunes: 1, NormalBossEquipment: 10, CumulativeLegendaryPerTier: 10, Loot: &LootRuleSet{}}
	fresh := *old
	fresh.Loot = &LootRuleSet{RetiredCubeMaterials: true}
	for tier := int32(1); tier <= 150; tier++ {
		for _, timely := range []bool{false, true} {
			a, b := PlanRiftRewards(tier, timely, old), PlanRiftRewards(tier, timely, &fresh)
			if a.Dust <= 0 || b.Dust != 0 {
				t.Fatal("dust policy", a, b)
			}
			a.Dust = 0
			if a != b {
				t.Fatal("non-dust reward changed", a, b)
			}
		}
	}
}
