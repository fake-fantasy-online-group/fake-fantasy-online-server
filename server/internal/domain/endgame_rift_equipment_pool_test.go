package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRiftEquipmentPoolNearestLegalTier(t *testing.T) {
	r := &RiftRules{EquipmentPools: []RiftEquipmentPool{
		{MonsterLevel: 30, SourceMonsters: []MonsterID{1}, Items: []ItemID{11, 12}},
		{MonsterLevel: 40, SourceMonsters: []MonsterID{2}, Items: []ItemID{21, 22}},
		{MonsterLevel: 60, SourceMonsters: []MonsterID{3}, Items: []ItemID{31}},
	}}
	for _, tt := range []struct {
		level   int32
		blocked ItemID
		want    []ItemID
	}{
		{35, 0, []ItemID{11, 12}}, {39, 21, []ItemID{22}}, {100, 0, []ItemID{31}}, {1, 0, []ItemID{11, 12}},
	} {
		got := r.EquipmentPoolForLevel(tt.level, func(id ItemID) bool { return id != tt.blocked })
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("level %d got %v want %v", tt.level, got, tt.want)
		}
	}
	if got := r.EquipmentPoolForLevel(40, func(ItemID) bool { return false }); len(got) != 0 {
		t.Fatal("invented fallback", got)
	}
}
func TestRiftEquipmentPoolsPinnedValidatedAndCloned(t *testing.T) {
	old := riftRulesFixture()
	revised := cloneRiftRules(*old)
	revised.Version = 7
	revised.EquipmentPools = []RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []MonsterID{123}, Items: []ItemID{1234}}}
	items := balanceItemsFixture()
	items[1234] = ItemDef{ID: 1234, Equip: &EquipDef{NativeDropEligible: true, LevelReq: 40}}
	table, err := NewRiftRuleTable(7, []RiftRules{*old, revised}, lootRulesTableFixture(), items)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(old)
	var persisted RiftRules
	_ = json.Unmarshal(raw, &persisted)
	if got, err := table.Resolve(1, &persisted); err != nil || !reflect.DeepEqual(got, old) {
		t.Fatal("historical snapshot changed", err)
	}
	revised.EquipmentPools[0].Items[0] = 9876
	a := table.Active()
	a.EquipmentPools[0].SourceMonsters[0] = 999
	a.EquipmentPools[0].Items[0] = 999
	if got := table.Active().EquipmentPools[0]; got.Items[0] != 1234 || got.SourceMonsters[0] != 123 {
		t.Fatal("aliased published pools", got)
	}
	bad := table.Active()
	items[1234].Equip.NativeDropEligible = false
	items[1234].Equip.NoTypeDrop = true
	if err := bad.validateEquipmentPools(items); err != nil {
		t.Fatal("explicit native named equipment rejected", err)
	}
	items[1234].Equip.AscensionOnly = true
	if err := bad.validateEquipmentPools(items); err == nil {
		t.Fatal("ascension-only item accepted")
	}
	items[1234].Equip.AscensionOnly = false
	items[1234].Equip.LevelReq = 61
	if err := bad.validateEquipmentPools(items); err == nil {
		t.Fatal("over60 item accepted")
	}
	bad.EquipmentPools[0].Items = []ItemID{1234, 1234}
	if err := bad.validateEquipmentPools(nil); err == nil {
		t.Fatal("duplicate item accepted")
	}
}

func TestRiftEquipmentPoolVersionFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		r       RiftRules
		wantErr bool
	}{
		{RiftRules{Version: 6}, false}, {RiftRules{Version: 7}, true},
		{RiftRules{Version: 7, EquipmentPools: []RiftEquipmentPool{}}, true},
		{RiftRules{Version: 7, EquipmentPools: []RiftEquipmentPool{{MonsterLevel: 63, SourceMonsters: []MonsterID{1}, Items: []ItemID{1}}}}, false},
	} {
		if got := tt.r.validateEquipmentPools(nil); (got != nil) != tt.wantErr {
			t.Fatalf("version %d validation %v", tt.r.Version, got)
		}
	}
}
