package session

import (
	"reflect"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

type inventorySlot struct {
	tab  uint8
	slot int32
}

func (s *eventSink) inventoryPackets(next event.InventorySnapshot) [][]byte {
	if s.inventory == nil {
		pkts := protocol.Encode(s.observer, next)
		if len(pkts) != 0 {
			cloned := cloneInventory(next)
			s.inventory = &cloned
		}
		return pkts
	}
	items, equipped, changed, ok := inventoryDiff(*s.inventory, next)
	if !ok {
		return s.replaceInventorySnapshot(next)
	}
	if !changed {
		cloned := cloneInventory(next)
		s.inventory = &cloned
		return nil
	}
	// 8069没有PP尾；宠物资料变化必须用含完整PP的8006快照。
	for _, item := range items {
		if item.Pet != nil {
			return s.replaceInventorySnapshot(next)
		}
	}
	pkt := protocol.InventoryDelta(next, items, equipped)
	if pkt == nil {
		return s.replaceInventorySnapshot(next)
	}
	cloned := cloneInventory(next)
	s.inventory = &cloned
	return [][]byte{pkt}
}

func (s *eventSink) replaceInventorySnapshot(next event.InventorySnapshot) [][]byte {
	pkts := protocol.Encode(s.observer, next)
	if len(pkts) != 0 {
		cloned := cloneInventory(next)
		s.inventory = &cloned
	}
	return pkts
}

func inventoryDiff(old, next event.InventorySnapshot) ([]event.InventoryItemView,
	[]event.EquippedItemView, bool, bool) {
	oldItems, ok := indexInventory(old.Items)
	if !ok {
		return nil, nil, false, false
	}
	nextItems, ok := indexInventory(next.Items)
	if !ok {
		return nil, nil, false, false
	}
	var items []event.InventoryItemView
	for _, item := range next.Items {
		key := inventorySlot{tab: item.Tab, slot: item.BagIndex}
		if previous, exists := oldItems[key]; !exists || !reflect.DeepEqual(previous, item) {
			items = append(items, item)
		}
	}
	for _, item := range old.Items {
		key := inventorySlot{tab: item.Tab, slot: item.BagIndex}
		if _, exists := nextItems[key]; !exists {
			items = append(items, event.InventoryItemView{
				Tab: item.Tab, BagIndex: item.BagIndex, Slot: domain.EquipSlot(-1),
			})
		}
	}

	oldEquipped, ok := indexEquipped(old.Equipped)
	if !ok {
		return nil, nil, false, false
	}
	nextEquipped, ok := indexEquipped(next.Equipped)
	if !ok {
		return nil, nil, false, false
	}
	var equipped []event.EquippedItemView
	for _, item := range next.Equipped {
		if previous, exists := oldEquipped[item.Slot]; !exists || !reflect.DeepEqual(previous, item) {
			equipped = append(equipped, item)
		}
	}
	for _, item := range old.Equipped {
		if _, exists := nextEquipped[item.Slot]; !exists {
			equipped = append(equipped, event.EquippedItemView{Slot: item.Slot})
		}
	}
	headerChanged := old.Money != next.Money || old.Caiyu != next.Caiyu || old.Honor != next.Honor ||
		old.Weight != next.Weight || old.MaxWeight != next.MaxWeight
	return items, equipped, headerChanged || len(items) != 0 || len(equipped) != 0, true
}

func indexInventory(items []event.InventoryItemView) (map[inventorySlot]event.InventoryItemView, bool) {
	out := make(map[inventorySlot]event.InventoryItemView, len(items))
	for _, item := range items {
		key := inventorySlot{tab: item.Tab, slot: item.BagIndex}
		if _, exists := out[key]; exists {
			return nil, false
		}
		out[key] = item
	}
	return out, true
}

func indexEquipped(items []event.EquippedItemView) (map[domain.EquipSlot]event.EquippedItemView, bool) {
	out := make(map[domain.EquipSlot]event.EquippedItemView, len(items))
	for _, item := range items {
		if _, exists := out[item.Slot]; exists {
			return nil, false
		}
		out[item.Slot] = item
	}
	return out, true
}

func cloneInventory(in event.InventorySnapshot) event.InventorySnapshot {
	in.Items = append([]event.InventoryItemView(nil), in.Items...)
	in.Equipped = append([]event.EquippedItemView(nil), in.Equipped...)
	return in
}

func (s *eventSink) warehousePackets(next event.WarehouseSnapshot) [][]byte {
	if s.warehouse == nil {
		return s.replaceWarehouseSnapshot(next)
	}
	items, changed, ok := warehouseDiff(*s.warehouse, next)
	if !ok {
		return s.replaceWarehouseSnapshot(next)
	}
	if !changed {
		cloned := cloneWarehouse(next)
		s.warehouse = &cloned
		return nil
	}
	pkt := protocol.WarehouseDelta(next, items)
	if pkt == nil {
		return s.replaceWarehouseSnapshot(next)
	}
	cloned := cloneWarehouse(next)
	s.warehouse = &cloned
	return [][]byte{pkt}
}

func (s *eventSink) replaceWarehouseSnapshot(next event.WarehouseSnapshot) [][]byte {
	pkts := protocol.Encode(s.observer, next)
	if len(pkts) != 0 {
		cloned := cloneWarehouse(next)
		s.warehouse = &cloned
	}
	return pkts
}

func warehouseDiff(old, next event.WarehouseSnapshot) ([]event.WarehouseItemView, bool, bool) {
	oldItems, ok := indexWarehouse(old.Items)
	if !ok {
		return nil, false, false
	}
	nextItems, ok := indexWarehouse(next.Items)
	if !ok {
		return nil, false, false
	}
	var items []event.WarehouseItemView
	for _, item := range next.Items {
		key := inventorySlot{tab: item.Tab, slot: item.Slot}
		if previous, exists := oldItems[key]; !exists || !reflect.DeepEqual(previous, item) {
			items = append(items, item)
		}
	}
	for _, item := range old.Items {
		key := inventorySlot{tab: item.Tab, slot: item.Slot}
		if _, exists := nextItems[key]; !exists {
			items = append(items, event.WarehouseItemView{Tab: item.Tab, Slot: item.Slot})
		}
	}
	headerChanged := old.PageCount != next.PageCount || old.Money != next.Money
	return items, headerChanged || len(items) != 0, true
}

func indexWarehouse(items []event.WarehouseItemView) (map[inventorySlot]event.WarehouseItemView, bool) {
	out := make(map[inventorySlot]event.WarehouseItemView, len(items))
	for _, item := range items {
		key := inventorySlot{tab: item.Tab, slot: item.Slot}
		if _, exists := out[key]; exists {
			return nil, false
		}
		out[key] = item
	}
	return out, true
}

func cloneWarehouse(in event.WarehouseSnapshot) event.WarehouseSnapshot {
	in.Items = append([]event.WarehouseItemView(nil), in.Items...)
	return in
}

func (s *eventSink) petPackets(next event.PetSnapshot) [][]byte {
	if s.pet == nil || !samePetStructure(*s.pet, next) {
		return s.replacePetSnapshot(next)
	}
	if samePetVitals(s.pet.Active, next.Active) {
		cloned := clonePet(next)
		s.pet = &cloned
		return nil
	}
	if next.ActiveSlot == event.NoActivePet {
		return s.replacePetSnapshot(next)
	}
	pkt := protocol.PetVitals(next.Active)
	cloned := clonePet(next)
	s.pet = &cloned
	return [][]byte{pkt}
}

func (s *eventSink) replacePetSnapshot(next event.PetSnapshot) [][]byte {
	pkts := protocol.Encode(s.observer, next)
	if len(pkts) != 0 {
		cloned := clonePet(next)
		s.pet = &cloned
	}
	return pkts
}

func samePetStructure(a, b event.PetSnapshot) bool {
	if a.Who != b.Who || a.ActiveSlot != b.ActiveSlot || a.ActiveBound != b.ActiveBound || a.Show != b.Show || a.Ridable != b.Ridable ||
		a.SharedMountModel != b.SharedMountModel ||
		!reflect.DeepEqual(a.Pets, b.Pets) {
		return false
	}
	zeroPetVitals(&a.Active)
	zeroPetVitals(&b.Active)
	return reflect.DeepEqual(a.Active, b.Active)
}

func samePetVitals(a, b event.PetView) bool {
	return a.Level == b.Level && a.Exp == b.Exp && a.ExpToNext == b.ExpToNext &&
		a.HP == b.HP && a.MaxHP == b.MaxHP && a.MP == b.MP && a.MaxMP == b.MaxMP &&
		a.Starve == b.Starve && a.Trust == b.Trust && a.FreePoints == b.FreePoints
}

func zeroPetVitals(p *event.PetView) {
	p.Level, p.Exp, p.ExpToNext = 0, 0, 0
	p.HP, p.MaxHP, p.MP, p.MaxMP = 0, 0, 0, 0
	p.Starve, p.Trust, p.FreePoints = 0, 0, 0
}

func clonePet(in event.PetSnapshot) event.PetSnapshot {
	in.Active.Skills = append([]event.PetSkillView(nil), in.Active.Skills...)
	in.Pets = append([]event.PetSlotView(nil), in.Pets...)
	for i := range in.Pets {
		in.Pets[i].Skills = append([]event.PetSkillView(nil), in.Pets[i].Skills...)
	}
	return in
}
