package domain

import (
	"encoding/json"
	"os"
	"testing"
)

func eliteRulesFixture(t *testing.T) *RiftRules {
	t.Helper()
	r := riftRulesFixture()
	r.Version = 6
	b, err := os.ReadFile("../../docs/endgame/elite-skills-v6.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestRiftEliteRulesAndHistoricalBinding(t *testing.T) {
	r := eliteRulesFixture(t)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	table, err := NewRiftRuleTable(1, []RiftRules{*riftRulesFixture(), *r}, lootRulesTableFixture(), balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	if table.Active().Version != 1 {
		t.Fatal("staged version was activated")
	}
	old := riftRulesFixture()
	if _, err = table.Resolve(1, old); err != nil {
		t.Fatal("historical snapshot changed", err)
	}
	snap, _ := table.Version(6)
	snap.Elite.Guardians[0].Abilities[0] = 7
	fresh, _ := table.Version(6)
	if fresh.Elite.Guardians[0].Abilities[0] != 0 {
		t.Fatal("nested rules alias")
	}
}
func TestRiftEliteInvalidRules(t *testing.T) {
	for _, mutate := range []func(*RiftRules){
		func(r *RiftRules) { r.Abilities[0].DamageReductionPct = 75 },
		func(r *RiftRules) { r.Abilities[2].Trigger = "active" },
		func(r *RiftRules) { r.Abilities[3].PulseMS = 0 },
		func(r *RiftRules) { r.Elite.Guardians[0].MonsterID = 1 },
		func(r *RiftRules) { r.Elite.Guardians[0].Abilities = []int32{0, 0, 2} },
	} {
		r := eliteRulesFixture(t)
		mutate(r)
		if r.Validate() == nil {
			t.Fatal("accepted invalid new rules")
		}
	}
}
