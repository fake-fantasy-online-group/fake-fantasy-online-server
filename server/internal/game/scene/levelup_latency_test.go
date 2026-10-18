package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"reflect"
	"testing"
)

// Uses the same nine-slot passive/requirement graph as the combat regression.
// Fixture construction and admission are outside the measured operation.
func BenchmarkLevelUpDelivery(b *testing.B) {
	for _, bagItems := range []int{0, 54} {
		name := "worn_only"
		if bagItems > 0 {
			name = "equipment_bag"
		}
		b.Run(name, func(b *testing.B) {
			s, p, _, sink := newCombatAttributeBatch(b)
			st := p.Player.Worn.At(domain.SlotHead)
			for i := 0; i < bagItems; i++ {
				slot := p.Player.Bag.FirstEmptyFor(s.items[st.Item])
				if slot < 0 || !p.Player.Bag.Set(slot, st) {
					b.Fatal("bag fixture full")
				}
			}
			if _, err := s.inventorySnapshot(p); err != nil {
				b.Fatal(err)
			}
			original := *p.Player.Char
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				*p.Player.Char = original
				p.Level = original.Level
				s.invalidateCombatAttributes(p)
				s.initializeCombatAttributes(p)
				b.StartTimer()
				s.grantExpExact(p, s.levels.Need(original.Level))
				s.flush()
				b.StopTimer()
				if p.Player.Char.Level != original.Level+1 {
					b.Fatal("level did not advance")
				}
				sink.take()
				b.StartTimer()
			}
		})
	}
}

func TestLevelUpInventorySnapshotResolutionPreservesPackets(t *testing.T) {
	s, p, _, sink := newCombatAttributeBatch(t)
	for i, slot := range []domain.EquipSlot{domain.SlotHead, domain.SlotRing} {
		st := p.Player.Worn.At(slot)
		d := s.items[st.Item]
		equip := *d.Equip
		equip.Need.Base.STR = 40 + int32(i)
		equip.Need.Level = 61
		d.Equip = &equip
		s.items[st.Item] = d
		bagSlot := p.Player.Bag.FirstEmptyFor(d)
		if bagSlot < 0 || !p.Player.Bag.Set(bagSlot, st) {
			t.Fatal("bag setup")
		}
		p.Player.ChangeSet.Set(domain.ChangeSetCell(slot), st)
	}
	// Same item ID, different instance roll must still receive its own tooltip.
	duplicate := p.Player.Worn.At(domain.SlotHead)
	duplicate.UID += 100000
	duplicate.RolledAffixCount = 1
	duplicate.RolledAffixes[0].Affix = domain.Affix{Attr: domain.AttrSTR, Value: 99}
	p.Player.Bag.Set(p.Player.Bag.FirstEmptyFor(s.items[duplicate.Item]), duplicate)
	s.invalidateCombatAttributes(p)
	check := func(t *testing.T) {
		t.Helper()
		panel := s.attributeSnapshot(p)
		got, err := s.inventorySnapshot(p)
		if err != nil {
			t.Fatal(err)
		}
		want := got
		want.Items = append([]event.InventoryItemView(nil), got.Items...)
		want.Equipped = append([]event.EquippedItemView(nil), got.Equipped...)
		for i := range want.Items {
			v := &want.Items[i]
			var st domain.Stack
			p.Player.Bag.Each(func(slot int, candidate domain.Stack) {
				if int32(slot%domain.BagPageSlots) == v.BagIndex && s.items[candidate.Item].InventoryTab == v.Tab {
					st = candidate
				}
			})
			d := s.items[v.Item]
			v.Desc = s.ownedItemTooltip(d, st, p)
			v.Blocked = s.equipmentBlockedFor(p, d, st)
		}
		for i := range want.Equipped {
			v := &want.Equipped[i]
			v.Desc = s.ownedItemTooltip(s.items[v.Item], p.Player.Worn.At(v.Slot), p)
		}
		if !reflect.DeepEqual(protocol.Encode(p.ID, got), protocol.Encode(p.ID, want)) {
			t.Fatal("inventory wire changed from independent evaluations")
		}
		set, err := s.changeSetSnapshot(p)
		if err != nil {
			t.Fatal(err)
		}
		oldSet := set
		oldSet.Items = append([]event.ChangeSetItemView(nil), set.Items...)
		for i := range oldSet.Items {
			v := &oldSet.Items[i]
			st := p.Player.ChangeSet.At(v.Cell)
			d := s.items[st.Item]
			v.Desc = s.ownedItemTooltip(d, st, p)
			v.Blocked = s.equipmentBlockedFor(p, d, st)
		}
		if !reflect.DeepEqual(protocol.Encode(p.ID, set), protocol.Encode(p.ID, oldSet)) {
			t.Fatal("change-set wire changed from independent evaluations")
		}
		if !reflect.DeepEqual(panel, s.attributeSnapshot(p)) {
			t.Fatal("inventory preview changed combat attributes")
		}
	}
	cases := []struct {
		name   string
		mutate func()
	}{
		{"before_level", func() {}},
		{"level_up", func() { s.grantExpExact(p, s.levels.Need(p.Player.Char.Level)); s.flush(); sink.take() }},
		{"status_requirement_bonus", func() {
			p.Status.Apply(domain.StatusDef{ID: 990001, Level: 1, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 40}}}, s.tick, p.ID)
		}},
		{"status_removed", func() { p.Status.Remove(990001) }},
		{"broken_worn", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.Durability = 0
			st.MaxDurability = 100
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"same_slot_replacement", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.UID += 200000
			st.Durability = 100
			st.RolledAffixCount = 1
			st.RolledAffixes[0].Affix = domain.Affix{Attr: domain.AttrSTR, Value: 60}
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"removed_worn", func() { p.Player.Worn.Set(domain.SlotRing, domain.Stack{}) }},
		{"hard_skill_changed", func() { p.Player.Char.Skills[10401] = 10 }},
		{"base_changed", func() { p.Player.Char.Base.STR += 50 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.mutate(); check(t) })
	}
}
