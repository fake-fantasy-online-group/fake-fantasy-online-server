package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func wingPolicyFixture() *RiftWingRewardRules {
	return &RiftWingRewardRules{
		Milestones:  []RiftWingMilestone{{30, 25029, 12, nil}, {50, 25029, 32, nil}, {70, 25029, 2500, nil}},
		RepeatBands: []RiftWingRepeatBand{{1, 25029, 1, nil}, {50, 25029, 2, nil}, {100, 25029, 3, nil}, {150, 25029, 4, nil}},
	}
}

func TestRiftWingPolicyBoundariesAndPersonalMilestones(t *testing.T) {
	r := &RiftRules{Version: 14, MaxTier: 150, WingRewards: wingPolicyFixture()}
	for _, tc := range []struct{ tier, count int32 }{{0, 0}, {1, 1}, {29, 1}, {30, 1}, {49, 1}, {50, 2}, {99, 2}, {100, 3}, {149, 3}, {150, 4}} {
		reward, ok := r.WingRepeatReward(tc.tier)
		if reward.Count != tc.count || ok != (tc.count > 0) {
			t.Fatalf("tier %d: %+v, %v", tc.tier, reward, ok)
		}
	}
	if got := r.MissingWingMilestones(29, nil); len(got) != 0 {
		t.Fatal("premature milestone", got)
	}
	if got := r.MissingWingMilestones(70, []int32{50}); !reflect.DeepEqual(got, []RiftWingMilestone{{30, 25029, 12, nil}, {70, 25029, 2500, nil}}) {
		t.Fatal("skipped tiers not filled exactly", got)
	}
	if got := r.MissingWingMilestones(150, []int32{30, 50, 70}); len(got) != 0 {
		t.Fatal("personal ledger ignored", got)
	}
	r.Version = 15
	if got := r.MissingWingMilestones(150, []int32{30, 50, 70}); len(got) != 0 {
		t.Fatal("rule upgrade reset first clears", got)
	}
	if _, ok := (*RiftRules)(nil).WingRepeatReward(150); ok {
		t.Fatal("missing rule invented reward")
	}
}

func TestRiftWingPolicyValidationAndFrozenCopies(t *testing.T) {
	base := RiftRules{Version: 14, MaxTier: 150, WingRewards: wingPolicyFixture()}
	items := map[ItemID]ItemDef{25029: {ID: 25029, Stackable: true}}
	if err := base.validateWingRewards(items); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RiftRules){
		"historical retrofit":       func(r *RiftRules) { r.Version = 13 },
		"missing policy":            func(r *RiftRules) { r.WingRewards = nil },
		"missing milestones":        func(r *RiftRules) { r.WingRewards.Milestones = nil },
		"missing repeat":            func(r *RiftRules) { r.WingRewards.RepeatBands = nil },
		"duplicate milestone":       func(r *RiftRules) { r.WingRewards.Milestones[1].Tier = 30 },
		"unordered milestones":      func(r *RiftRules) { r.WingRewards.Milestones[1].Tier = 29 },
		"invalid tier":              func(r *RiftRules) { r.WingRewards.Milestones[2].Tier = 151 },
		"zero package":              func(r *RiftRules) { r.WingRewards.Milestones[0].Count = 0 },
		"oversized package":         func(r *RiftRules) { r.WingRewards.Milestones[0].Count = RiftWingMaxRewardCount + 1 },
		"missing material":          func(r *RiftRules) { r.WingRewards.Milestones[0].Item = 999 },
		"missing first repeat band": func(r *RiftRules) { r.WingRewards.RepeatBands[0].MinTier = 2 },
		"duplicate repeat":          func(r *RiftRules) { r.WingRewards.RepeatBands[1].MinTier = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			r := cloneRiftRules(base)
			mutate(&r)
			if err := r.validateWingRewards(items); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
	for _, def := range []ItemDef{{ID: 25029}, {ID: 25029, Stackable: true, HardDungeonOnly: true}, {ID: 25029, Stackable: true, Equip: &EquipDef{}}, {ID: 25029, Stackable: true, InstanceKind: ItemInstanceEquipment}} {
		if err := base.validateWingRewards(map[ItemID]ItemDef{25029: def}); err == nil {
			t.Fatal("invalid item accepted", def)
		}
	}
	copy := cloneRiftRules(base)
	copy.WingRewards.Milestones[0].Count = 99
	copy.WingRewards.RepeatBands[0].Item = 999
	if base.WingRewards.Milestones[0].Count != 12 || base.WingRewards.RepeatBands[0].Item != 25029 {
		t.Fatal("rule aliases configuration")
	}
	old := riftRulesFixture()
	raw, _ := json.Marshal(old)
	if strings.Contains(string(raw), "WingRewards") {
		t.Fatal("historical snapshot gained a field")
	}
	if _, ok := old.WingRepeatReward(150); ok || len(old.MissingWingMilestones(150, nil)) != 0 {
		t.Fatal("historical reward changed")
	}
}

func TestRiftWingConfiguredWhitePackagesAndNestedCopies(t *testing.T) {
	raw, err := os.ReadFile("../../config/rift-wing-rewards-v14.json")
	if err != nil {
		t.Fatal(err)
	}
	var policy RiftWingRewardRules
	if err = json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	r := RiftRules{Version: 14, MaxTier: 150, WingRewards: &policy}
	items := map[ItemID]ItemDef{11682: {ID: 11682, Stackable: true}, 25027: {ID: 25027, Stackable: true}, 25028: {ID: 25028, Stackable: true}, 25029: {ID: 25029, Stackable: true}}
	if err = r.validateWingRewards(items); err != nil {
		t.Fatal(err)
	}
	units := func(ms []CubeMaterialDef) int32 {
		var n int32
		for _, m := range ms {
			u := map[ItemID]int32{25027: 1, 25028: 5, 25029: 25}[m.ItemID]
			if u == 0 {
				t.Fatal("nonwhite reward", m)
			}
			n += m.Count * u
		}
		return n
	}
	for i, want := range []int32{20, 60, 160, 400, 800, 1500, 2500, 4000} {
		if got := units(policy.Milestones[i].Rewards()); got != want {
			t.Fatal(i, got, want)
		}
	}
	for tier := int32(1); tier <= 150; tier++ {
		b, ok := r.WingRepeatReward(tier)
		if !ok || units(b.Rewards()) != 1+tier/10 {
			t.Fatal("repeat", tier, b)
		}
	}
	copy := cloneRiftRules(r)
	copy.WingRewards.Milestones[0].Materials[0].Count = 999
	copy.WingRewards.RepeatBands[0].Materials[0].Count = 999
	if policy.Milestones[0].Materials[0].Count == 999 || policy.RepeatBands[0].Materials[0].Count == 999 {
		t.Fatal("nested reward aliases")
	}
}

func TestRiftWingRejectsRareOrUnapprovedRoutineReward(t *testing.T) {
	for _, id := range []ItemID{11682, 11683, 11684, 4001} {
		r := RiftRules{Version: 14, MaxTier: 150, WingRewards: wingPolicyFixture()}
		r.WingRewards.Milestones[0].Item = id
		items := map[ItemID]ItemDef{25029: {ID: 25029, Stackable: true}, id: {ID: id, Stackable: true}}
		if r.validateWingRewards(items) == nil {
			t.Fatal("unapproved routine first-clear reward", id)
		}
		r.WingRewards = wingPolicyFixture()
		r.WingRewards.RepeatBands[0].Item = id
		if r.validateWingRewards(items) == nil {
			t.Fatal("unapproved routine repeat reward", id)
		}
	}
}

func TestRiftRareFeatherExactApprovedSlotAndRollBoundaries(t *testing.T) {
	r := RiftRareFeatherRule{Item: 11682, Count: 1, ChanceBP: 50}
	for _, n := range []int{0, 49, 50, 9999} {
		calls := 0
		material, ok := r.Roll(func(bound int) int {
			calls++
			if bound != 10000 {
				t.Fatal(bound)
			}
			return n
		})
		if calls != 1 || ok != (n < 50) {
			t.Fatal(n, calls, ok)
		}
		if ok && (material.ItemID != 11682 || material.Count != 1) {
			t.Fatal(material)
		}
	}
	for _, bad := range []RiftRareFeatherRule{{11683, 1, 50}, {11682, 2, 50}, {11682, 1, 51}, {11682, 1, 49}} {
		policy := wingPolicyFixture()
		policy.RareFeather = &bad
		rules := RiftRules{Version: 14, MaxTier: 150, WingRewards: policy}
		if rules.validateWingRewards(nil) == nil {
			t.Fatal("unapproved rare rule", bad)
		}
	}
	policy := wingPolicyFixture()
	policy.RareFeather = &r
	rules := RiftRules{Version: 14, MaxTier: 150, WingRewards: policy}
	copy := cloneRiftRules(rules)
	copy.WingRewards.RareFeather.ChanceBP = 9999
	if r.ChanceBP != 50 {
		t.Fatal("rare rule aliases")
	}
}
