package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRiftSlotFallbackMissingOnlyHighestWearableTier(t *testing.T) {
	items := map[ItemID]ItemDef{}
	add := func(id ItemID, slot EquipSlot, wear, resource int32) {
		items[id] = ItemDef{ID: id, Equip: &EquipDef{Slot: int32(slot), LevelReq: wear, ResourceLevel: resource, NativeDropEligible: true, Need: Requirement{Level: wear}}}
	}
	add(1, SlotWeapon, 1, 1)
	add(11, SlotFace, 6, 1)
	add(12, SlotFace, 18, 3)
	add(13, SlotFace, 18, 3)
	add(14, SlotFace, 40, 2) // Higher wear level is not higher resource tier.
	add(21, SlotBag, 12, 2)
	add(22, SlotBag, 36, 6)
	add(23, SlotBag, 36, 6)
	add(999, SlotFace, 1, 99) // Not in the frozen list, never eligible for fallback.
	r := &RiftRules{Version: 11, EquipmentSlotFallbacks: []ItemID{11, 12, 13, 14, 21, 22, 23}}
	for _, tc := range []struct {
		name    string
		level   int32
		pool    []ItemID
		blocked ItemID
		want    []ItemID
	}{
		{"below first wear", 5, []ItemID{1}, 0, []ItemID{1}},
		{"first face", 6, []ItemID{1}, 0, []ItemID{1, 11}},
		{"lower wearable", 17, []ItemID{1}, 0, []ItemID{1, 11, 21}},
		{"all same-tier variants", 36, []ItemID{1}, 0, []ItemID{1, 12, 13, 22, 23}},
		{"resource not wear", 43, []ItemID{1}, 0, []ItemID{1, 12, 13, 22, 23}},
		{"existing lower face preserved", 43, []ItemID{1, 11}, 0, []ItemID{1, 11, 22, 23}},
		{"existing slots unchanged", 43, []ItemID{23, 1, 11}, 0, []ItemID{23, 1, 11}},
		{"filter before tier choice", 36, []ItemID{1, 12}, 22, []ItemID{1, 12, 23}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backing := make([]ItemID, len(tc.pool), 32)
			copy(backing, tc.pool)
			got := r.FillMissingEquipmentSlots(backing, tc.level, items, func(id ItemID) bool { return id != tc.blocked })
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if !reflect.DeepEqual(backing, tc.pool) {
				t.Fatal("caller pool mutated")
			}
			if len(got) > len(backing) && backing[:cap(backing)][len(backing)] != 0 {
				t.Fatal("caller backing array mutated")
			}
		})
	}
	// Recipient class/gender restrictions may eliminate every top-tier variant.
	got := r.FillMissingEquipmentSlots([]ItemID{1}, 43, items, func(id ItemID) bool { return id != 12 && id != 13 && id != 14 && id != 22 && id != 23 })
	if !reflect.DeepEqual(got, []ItemID{1, 11, 21}) {
		t.Fatal("lower legal tier not selected", got)
	}
	// Source and runtime both enforce both original wearer level fields.
	items[12].Equip.Need.Level = 60
	items[13].Equip.Need.Level = 60
	got = r.FillMissingEquipmentSlots([]ItemID{1}, 43, items, func(ItemID) bool { return true })
	if !reflect.DeepEqual(got, []ItemID{1, 14, 22, 23}) {
		t.Fatal("native wear requirement ignored", got)
	}
	r.Version = 10
	if got = r.FillMissingEquipmentSlots([]ItemID{1}, 43, items, func(ItemID) bool { return true }); !reflect.DeepEqual(got, []ItemID{1}) {
		t.Fatal("old run expanded", got)
	}
}

func TestRiftSlotFallbackValidationFreezeAndClone(t *testing.T) {
	old := riftRulesFixture()
	fresh := cloneRiftRules(*old)
	fresh.Version = 11
	fresh.EquipmentPools = []RiftEquipmentPool{{MonsterLevel: 43, SourceMonsters: []MonsterID{1}, Items: []ItemID{1}}}
	fresh.EquipmentSlotFallbacks = []ItemID{1, 2}
	items := balanceItemsFixture()
	items[1] = ItemDef{ID: 1, Equip: &EquipDef{Slot: 1, LevelReq: 18, ResourceLevel: 3, NativeDropEligible: true}}
	items[2] = ItemDef{ID: 2, Equip: &EquipDef{Slot: 10, LevelReq: 36, ResourceLevel: 6, NativeDropEligible: true}}
	table, err := NewRiftRuleTable(11, []RiftRules{*old, fresh}, lootRulesTableFixture(), items)
	if err != nil {
		t.Fatal(err)
	}
	fresh.EquipmentSlotFallbacks[0] = 999
	active := table.Active()
	active.EquipmentSlotFallbacks[0] = 888
	if !reflect.DeepEqual(table.Active().EquipmentSlotFallbacks, []ItemID{1, 2}) {
		t.Fatal("fallback aliased")
	}
	for _, r := range []*RiftRules{old, table.Active()} {
		raw, _ := json.Marshal(r)
		var saved RiftRules
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		resolved, e := table.Resolve(r.Version, &saved)
		if e != nil || !reflect.DeepEqual(resolved, r) {
			t.Fatal("frozen fallback did not round trip", e)
		}
	}
	for _, change := range []func(*RiftRules){
		func(r *RiftRules) { r.EquipmentSlotFallbacks = nil },
		func(r *RiftRules) { r.EquipmentSlotFallbacks = []ItemID{} },
		func(r *RiftRules) { r.EquipmentSlotFallbacks = []ItemID{1} },
		func(r *RiftRules) { r.EquipmentSlotFallbacks = []ItemID{1, 1, 2} },
		func(r *RiftRules) { r.EquipmentSlotFallbacks = []ItemID{1, 2, 999} },
		func(r *RiftRules) { r.Version = 10 },
	} {
		r := table.Active()
		change(r)
		if r.validateEquipmentSlotFallbacks(items) == nil {
			t.Fatal("invalid fallback accepted", r.EquipmentSlotFallbacks)
		}
	}
	for name, change := range map[string]func(*ItemDef){
		"fixed":                 func(d *ItemDef) { d.Equip.Affixes = []Affix{{Attr: 1, Value: 1}} },
		"unknown fixed":         func(d *ItemDef) { d.Equip.UnknownInnateProperties = true },
		"ascension":             func(d *ItemDef) { d.Equip.AscensionOnly = true },
		"dungeon":               func(d *ItemDef) { d.HardDungeonOnly = true },
		"costume":               func(d *ItemDef) { d.Equip.NoTypeDrop = true },
		"not random":            func(d *ItemDef) { d.Equip.NativeDropEligible = false },
		"other slot":            func(d *ItemDef) { d.Equip.Slot = 2 },
		"wear above cap":        func(d *ItemDef) { d.Equip.LevelReq = 61 },
		"native wear above cap": func(d *ItemDef) { d.Equip.Need.Level = 61 },
		"negative resource":     func(d *ItemDef) { d.Equip.ResourceLevel = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			d := items[1]
			e := *d.Equip
			d.Equip = &e
			change(&d)
			catalog := map[ItemID]ItemDef{1: d, 2: items[2]}
			r := table.Active()
			if r.validateEquipmentSlotFallbacks(catalog) == nil {
				t.Fatal("invalid source accepted")
			}
			if got := r.FillMissingEquipmentSlots(nil, 65, catalog, func(ItemID) bool { return true }); !reflect.DeepEqual(got, []ItemID{2}) {
				t.Fatal("runtime accepted invalid fallback", got)
			}
		})
	}
}
