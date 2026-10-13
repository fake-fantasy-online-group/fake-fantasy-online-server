package domain

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestRiftRebalanceExactAnchorAndMonotoneAllTiers(t *testing.T) {
	old := riftRulesFixture()
	revised := cloneRiftRules(*old)
	revised.Version, revised.StatAnchorTier, revised.NormalBossEquipment = 4, 30, 10
	for _, base := range []int32{1, 100, 4404, 8814, 19845, 29748, 3000000, math.MaxInt32} {
		for party := 1; party <= 5; party++ {
			for class := int32(0); class <= 3; class++ {
				for _, hp := range []bool{false, true} {
					if got, want := RiftScaledStat(base, 30, party, class, hp, &revised), RiftScaledStat(base, 1, party, class, hp, old); got != want {
						t.Fatalf("base%d party%d class%d hp%v: anchor %d != %d", base, party, class, hp, got, want)
					}
					prior := int32(0)
					for tier := int32(1); tier <= 150; tier++ {
						got := RiftScaledStat(base, tier, party, class, hp, &revised)
						if got < prior || got <= 0 {
							t.Fatalf("nonmonotone tier%d: %d -> %d", tier, prior, got)
						}
						prior = got
					}
				}
			}
		}
	}
	if got := RiftScaledStat(1000000, 1, 1, 0, true, &revised); got != 107328 {
		t.Fatalf("new tier1 HP anchor %d", got)
	}
	if got := RiftScaledStat(1000000, 1, 1, 0, false, &revised); got != 320652 {
		t.Fatalf("new tier1 damage anchor %d", got)
	}
}

func TestRiftRebalancePinsOldSnapshotAndKeepsSpecialRewards(t *testing.T) {
	old := riftRulesFixture()
	revised := cloneRiftRules(*old)
	revised.Version, revised.StatAnchorTier, revised.NormalBossEquipment = 4, 30, 10
	table, err := NewRiftRuleTable(4, []RiftRules{*old, revised}, lootRulesTableFixture(), balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(old)
	var persisted RiftRules
	if err = json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	pinned, err := table.Resolve(1, &persisted)
	if err != nil || !reflect.DeepEqual(pinned, old) {
		t.Fatal("legacy snapshot changed", err)
	}
	for tier := int32(1); tier <= 150; tier++ {
		for _, timely := range []bool{false, true} {
			before := PlanRiftRewards(tier, timely, old)
			after := PlanRiftRewards(tier, timely, &revised)
			if after.NormalBossEquipment != 10 {
				t.Fatal("normal count depends on tier/timing")
			}
			after.NormalBossEquipment = 0
			if after != before {
				t.Fatal("special gear/rune/dust rules changed", tier, timely)
			}
		}
	}
	for _, value := range []int32{-1, 151} {
		bad := cloneRiftRules(revised)
		bad.StatAnchorTier = value
		if bad.Validate() == nil {
			t.Fatal("invalid anchor accepted")
		}
	}
	for _, value := range []int32{-1, 21} {
		bad := cloneRiftRules(revised)
		bad.NormalBossEquipment = value
		if bad.Validate() == nil {
			t.Fatal("invalid normal slots accepted")
		}
	}
}
