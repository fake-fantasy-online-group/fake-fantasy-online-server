package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLegendaryPowerPolicyRuleValidationAndLegacyJSON(t *testing.T) {
	legacy := *lootRulesFixture()
	encoded, err := json.Marshal(legacy)
	if err != nil || strings.Contains(string(encoded), "LegendaryPowerPolicy") {
		t.Fatalf("legacy policy changed persisted JSON: %s, %v", encoded, err)
	}
	for _, tc := range []struct {
		version, policy int32
		valid           bool
	}{{3, 0, true}, {3, 1, false}, {4, 1, true}, {4, -1, false}, {4, 2, false}} {
		next := legacy
		next.Version, next.LegendaryPowerPolicy = tc.version, tc.policy
		table, err := NewLootRuleTable(tc.version, []LootRuleSet{legacy, next}, balanceItemsFixture())
		if (err == nil) != tc.valid {
			t.Fatalf("v%d policy%d valid=%v: %v", tc.version, tc.policy, tc.valid, err)
		}
		if err == nil && table.Active().LegendaryPowerPolicy != tc.policy {
			t.Fatal("policy lost in clone")
		}
	}
	var historical LootRuleSet
	if err := json.Unmarshal(encoded, &historical); err != nil || historical.LegendaryPowerPolicy != 0 {
		t.Fatal("legacy snapshot did not default to old policy", err)
	}
}

func TestRiftRejectsMismatchedFrozenPowerPolicy(t *testing.T) {
	for _, tc := range []struct{ version, policy int32 }{{1, 1}, {15, 1}, {16, 0}, {17, 0}} {
		r := cloneRiftRules(*riftRulesFixture())
		r.Version = tc.version
		r.Loot.LegendaryPowerPolicy = tc.policy
		if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "legendary power policy") {
			t.Fatalf("rift%d accepted incompatible frozen policy%d: %v", tc.version, tc.policy, err)
		}
	}
}
