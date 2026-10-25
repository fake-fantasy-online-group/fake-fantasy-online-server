package domain

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestRiftSlotFirstEnumeratesPartsThenTemplates(t *testing.T) {
	items := map[ItemID]ItemDef{}
	add := func(id ItemID, slot EquipSlot) { items[id] = ItemDef{ID: id, Equip: &EquipDef{Slot: int32(slot)}} }
	for _, id := range []ItemID{15, 11, 14, 12, 13} {
		add(id, SlotFace)
	}
	add(21, SlotHead)
	add(41, SlotWeapon)
	add(42, SlotTwoHand)
	add(51, SlotShield)
	pool := []ItemID{42, 15, 21, 11, 51, 14, 41, 12, 13, 11, 999}
	before := append([]ItemID(nil), pool...)
	expected := [][]ItemID{{11, 12, 13, 14, 15}, {21}, {41, 42}, {51}}
	rule := &RiftRules{Version: 15}
	for part, templates := range expected {
		for template, want := range templates {
			calls := 0
			got, ok := rule.RollEquipmentBase(pool, items, func(n int) int {
				calls++
				if calls == 1 {
					if n != len(expected) {
						t.Fatalf("parts=%d", n)
					}
					return part
				}
				if calls != 2 || n != len(templates) {
					t.Fatalf("template draw %d/%d", calls, n)
				}
				return template
			})
			if !ok || got != want || calls != 2 {
				t.Fatalf("part %d template %d: %d/%v/%d", part, template, got, ok, calls)
			}
		}
	}
	if !reflect.DeepEqual(pool, before) {
		t.Fatal("modified frozen pool")
	}
	for _, p := range [][]ItemID{nil, {999}} {
		if _, ok := rule.RollEquipmentBase(p, items, func(int) int { t.Fatal("empty group consumed RNG"); return 0 }); ok {
			t.Fatal("empty pool yielded base")
		}
	}
}

func TestRiftSlotFirstHistoricalDrawsAndOrdering(t *testing.T) {
	pool := []ItemID{99, 5, 88, 3}
	for version := int32(1); version <= 14; version++ {
		rule := &RiftRules{Version: version}
		actual, expected := rand.New(rand.NewSource(127)), rand.New(rand.NewSource(127))
		for i := 0; i < 100; i++ {
			got, ok := rule.RollEquipmentBase(pool, nil, actual.Intn)
			want := pool[expected.Intn(len(pool))]
			if !ok || got != want {
				t.Fatalf("v%d draw %d got %d want %d", version, i, got, want)
			}
		}
		if actual.Int63() != expected.Int63() {
			t.Fatal("historical stream shifted")
		}
	}
}

func TestRiftSlotFirstUniformGroupsAndTemplates(t *testing.T) {
	items := map[ItemID]ItemDef{}
	pool := []ItemID{}
	for i := 1; i <= 9; i++ {
		slot := SlotFace
		if i == 6 {
			slot = SlotHead
		}
		if i == 7 {
			slot = SlotWeapon
		}
		if i == 8 {
			slot = SlotTwoHand
		}
		if i == 9 {
			slot = SlotShield
		}
		id := ItemID(i)
		pool = append(pool, id)
		items[id] = ItemDef{ID: id, Equip: &EquipDef{Slot: int32(slot)}}
	}
	rule := &RiftRules{Version: 15}
	rng := rand.New(rand.NewSource(815))
	counts := map[ItemID]int{}
	for i := 0; i < 100000; i++ {
		id, _ := rule.RollEquipmentBase(pool, items, rng.Intn)
		counts[id]++
	}
	for id, count := range counts {
		want := 25000
		if id <= 5 {
			want = 5000
		}
		if id == 7 || id == 8 {
			want = 12500
		}
		if count < want*95/100 || count > want*105/100 {
			t.Fatalf("template %d count %d expected near %d", id, count, want)
		}
	}
	// Reordering configuration must not alter replay for version 15.
	reversed := append([]ItemID(nil), pool...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	a, b := rand.New(rand.NewSource(991)), rand.New(rand.NewSource(991))
	for i := 0; i < 100; i++ {
		x, _ := rule.RollEquipmentBase(pool, items, a.Intn)
		y, _ := rule.RollEquipmentBase(reversed, items, b.Intn)
		if x != y {
			t.Fatal("order-dependent replay")
		}
	}
}
