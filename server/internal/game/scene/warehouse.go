package scene

import (
	"fmt"
	"math"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) warehouseSnapshot(p *entity.Entity) (event.WarehouseSnapshot, bool) {
	if p == nil || p.Player == nil || p.Player.Warehouse == nil {
		return event.WarehouseSnapshot{}, false
	}
	w := p.Player.Warehouse
	if w.PageCount() <= 0 || w.PageCount() > 255 || w.Money() < 0 {
		return event.WarehouseSnapshot{}, false
	}
	out := event.WarehouseSnapshot{Who: p.ID, PageCount: uint8(w.PageCount()), Money: w.Money()}
	ok := true
	w.Each(func(page, slot int, st domain.Stack) {
		if !ok {
			return
		}
		def, found := s.itemDef(st.Item)
		if !found || page < 0 || page > 254 || slot < 0 {
			ok = false
			return
		}
		view := event.WarehouseItemView{Tab: uint8(page), Slot: int32(slot), Item: st.Item,
			Count: st.Count, Name: itemDisplayName(def, st), Desc: s.ownedItemTooltip(def, st, p),
			Blocked: s.equipmentBlockedFor(p, def), Quality: equipmentQuality(def, st)}
		if def.Equip != nil {
			view.CurrentDurability = st.Durability
			view.MaxDurability = domain.MaxDurabilityOf(st, def)
		}
		out.Items = append(out.Items, view)
	})
	return out, ok
}

// An open warehouse keeps its equipment availability shading in sync with the
// same character requirements used by the inventory and change set.
func (s *Scene) refreshWarehouseBlocked(p *entity.Entity) {
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen {
		return
	}
	if snap, ok := s.warehouseSnapshot(p); ok {
		s.emitTo(p.ID, snap)
	}
}

func (s *Scene) onOpenWarehouse(cmd OpenWarehouse) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Warehouse == nil {
		return
	}
	if snap, ok := s.warehouseSnapshot(p); ok {
		p.Player.WarehouseOpen = true
		s.emitTo(p.ID, snap)
	}
}

func removeStackCount(st domain.Stack, count int32) (domain.Stack, domain.Stack, bool) {
	if st.Empty() || count <= 0 || count > st.Count {
		return st, domain.Stack{}, false
	}
	moved := st
	moved.Count = count
	st.Count -= count
	if st.Count == 0 {
		st = domain.Stack{}
	}
	return st, moved, true
}

func putWarehouse(w *domain.Warehouse, def domain.ItemDef, page, slot int, moved domain.Stack) bool {
	if w == nil || moved.Empty() || page < 0 || page >= w.PageCount() {
		return false
	}
	if !def.Stackable || moved.UID != 0 {
		if moved.Count != 1 {
			return false
		}
		if slot < 0 {
			slot = w.FirstEmpty(page)
		}
		return slot >= 0 && w.At(page, slot).Empty() && w.Set(page, slot, moved)
	}
	if moved.Count > w.MaxStack() {
		return false
	}
	if slot < 0 {
		for i := 0; i < w.SlotsPerPage(); i++ {
			dst := w.At(page, i)
			if domain.SameStackInstance(dst, moved) && w.MaxStack()-dst.Count >= moved.Count {
				dst.Count += moved.Count
				return w.Set(page, i, dst)
			}
		}
		slot = w.FirstEmpty(page)
	}
	if slot < 0 || slot >= w.SlotsPerPage() {
		return false
	}
	dst := w.At(page, slot)
	if dst.Empty() {
		return w.Set(page, slot, moved)
	}
	if !domain.SameStackInstance(dst, moved) || w.MaxStack()-dst.Count < moved.Count {
		return false
	}
	dst.Count += moved.Count
	return w.Set(page, slot, dst)
}

func putBag(bag *domain.Bag, def domain.ItemDef, slot int, moved domain.Stack) bool {
	if bag == nil || moved.Empty() {
		return false
	}
	if slot < 0 {
		return bag.AddStack(def, moved) == 0
	}
	if !bag.AllowsSlot(def, slot) {
		return false
	}
	dst := bag.At(slot)
	if !def.Stackable || moved.UID != 0 {
		return moved.Count == 1 && dst.Empty() && bag.Set(slot, moved)
	}
	if moved.Count > domain.MaxStack {
		return false
	}
	if dst.Empty() {
		return bag.Set(slot, moved)
	}
	if !domain.SameStackInstance(dst, moved) || domain.MaxStack-dst.Count < moved.Count {
		return false
	}
	dst.Count += moved.Count
	return bag.Set(slot, dst)
}

func (s *Scene) commitWarehouse(p *entity.Entity, oldBag *domain.Bag, oldWarehouse *domain.Warehouse,
	oldMoney domain.Money, bagChanged bool) bool {
	warehouse, ok := s.warehouseSnapshot(p)
	if !ok {
		p.Player.Bag, p.Player.Warehouse, p.Player.Char.Money = oldBag, oldWarehouse, oldMoney
		return false
	}
	var inventory event.InventorySnapshot
	if bagChanged {
		var err error
		inventory, err = s.inventorySnapshot(p)
		if err != nil {
			p.Player.Bag, p.Player.Warehouse, p.Player.Char.Money = oldBag, oldWarehouse, oldMoney
			return false
		}
	}
	if bagChanged {
		s.emitTo(p.ID, inventory)
		s.pushQuestLog(p)
	}
	s.emitTo(p.ID, warehouse)
	p.Player.RepairQuote = nil
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	return true
}

func (s *Scene) onWarehouseMove(cmd WarehouseMove) {
	p := s.players[cmd.ID]
	s.log.Info("处理个人仓库物品请求", "entity", cmd.ID, "dir", cmd.Dir,
		"depositDir", s.warehouseRule.MoveDepositDir, "bagTab", cmd.BagTab,
		"fromSlot", cmd.FromSlot, "warehouseTab", cmd.WarehouseTab,
		"count", cmd.Count, "toSlot", cmd.ToSlot,
		"playerFound", p != nil)
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen || p.Player.Bag == nil ||
		p.Player.Warehouse == nil || !s.warehouseRule.Valid() || cmd.Count <= 0 {
		return
	}
	deposit := cmd.Dir == s.warehouseRule.MoveDepositDir
	oldBag, oldWarehouse, oldMoney := p.Player.Bag, p.Player.Warehouse, p.Player.Char.Money
	bag, warehouse := oldBag.Clone(), oldWarehouse.Clone()
	if bag == nil || warehouse == nil {
		return
	}
	if deposit {
		if cmd.FromSlot < 0 || int(cmd.FromSlot) >= bag.Cap() {
			return
		}
		src := bag.At(int(cmd.FromSlot))
		if src.InstanceKind == domain.ItemInstancePet {
			s.petCarrierNotice(p)
			return
		}
		def, ok := s.itemDef(src.Item)
		count := cmd.Count
		if count == math.MaxInt32 {
			count = src.Count
		}
		if src.Empty() || !ok || count <= 0 || (!def.Stackable && count != 1) {
			return
		}
		remain, moved, ok := removeStackCount(src, count)
		if !ok || !putWarehouse(warehouse, def, int(cmd.WarehouseTab), int(cmd.ToSlot), moved) ||
			!bag.Set(int(cmd.FromSlot), remain) {
			return
		}
	} else {
		if int(cmd.WarehouseTab) >= warehouse.PageCount() || cmd.FromSlot < 0 ||
			int(cmd.FromSlot) >= warehouse.SlotsPerPage() {
			return
		}
		src := warehouse.At(int(cmd.WarehouseTab), int(cmd.FromSlot))
		def, ok := s.itemDef(src.Item)
		count := cmd.Count
		if count == math.MaxInt32 {
			count = src.Count
		}
		if src.Empty() || !ok || count <= 0 || (!def.Stackable && count != 1) {
			return
		}
		remain, moved, ok := removeStackCount(src, count)
		toSlot := int(cmd.ToSlot)
		if toSlot >= 0 && bag.Cap() >= domain.DefaultBagSlots {
			var valid bool
			toSlot, valid = domain.ClientBagSlot(def.InventoryTab, toSlot)
			if !valid {
				return
			}
		}
		if !ok || !putBag(bag, def, toSlot, moved) ||
			!warehouse.Set(int(cmd.WarehouseTab), int(cmd.FromSlot), remain) {
			return
		}
	}
	p.Player.Bag, p.Player.Warehouse = bag, warehouse
	if s.commitWarehouse(p, oldBag, oldWarehouse, oldMoney, true) {
		s.log.Debug("个人仓库存取物品", "char", p.Name, "deposit", deposit,
			"page", cmd.WarehouseTab, "count", cmd.Count)
	}
}

func (s *Scene) onWarehouseMoney(cmd WarehouseMoney) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen || p.Player.Warehouse == nil ||
		!s.warehouseRule.Valid() || cmd.Amount <= 0 {
		return
	}
	oldBag, oldWarehouse, oldMoney := p.Player.Bag, p.Player.Warehouse, p.Player.Char.Money
	warehouse := oldWarehouse.Clone()
	carried, err := inventoryMoney(oldMoney)
	if warehouse == nil || err != nil {
		return
	}
	deposit := cmd.Mode == s.warehouseRule.MoneyDepositMode
	if deposit {
		if carried < cmd.Amount || warehouse.Money() > math.MaxInt64-cmd.Amount {
			return
		}
		carried -= cmd.Amount
		warehouse.SetMoney(warehouse.Money() + cmd.Amount)
	} else {
		if warehouse.Money() < cmd.Amount || carried > math.MaxInt64-cmd.Amount {
			return
		}
		warehouse.SetMoney(warehouse.Money() - cmd.Amount)
		carried += cmd.Amount
	}
	p.Player.Warehouse = warehouse
	p.Player.Char.Money = moneyFromInventory(carried)
	if s.commitWarehouse(p, oldBag, oldWarehouse, oldMoney, true) {
		s.log.Debug("个人仓库存取金钱", "char", p.Name, "deposit", deposit, "amount", cmd.Amount)
	}
}

func (s *Scene) onExpandWarehouse(cmd ExpandWarehouse) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen || p.Player.Warehouse == nil ||
		!s.warehouseRule.Valid() {
		return
	}
	oldBag, oldWarehouse, oldMoney := p.Player.Bag, p.Player.Warehouse, p.Player.Char.Money
	cost, ok := s.warehouseRule.ExpansionCost(oldWarehouse.PageCount())
	carried, err := inventoryMoney(oldMoney)
	if !ok {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "个人仓库已经扩展到上限"})
		return
	}
	if err != nil || carried < cost {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "扩展个人仓库所需金钱不足"})
		return
	}
	warehouse := oldWarehouse.Clone()
	if warehouse == nil || !warehouse.Expand(s.warehouseRule.MaxPages) {
		return
	}
	p.Player.Warehouse = warehouse
	p.Player.Char.Money = moneyFromInventory(carried - cost)
	if s.commitWarehouse(p, oldBag, oldWarehouse, oldMoney, true) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
			Text: fmt.Sprintf("个人仓库已扩展到%d页，花费%d铜币", warehouse.PageCount(), cost)})
	}
}

func (s *Scene) onWarehouseSplit(cmd WarehouseSplit) {
	p := s.players[cmd.ID]
	s.log.Info("处理个人仓库拆分请求", "entity", cmd.ID, "tab", cmd.Tab,
		"slot", cmd.Slot, "count", cmd.Count, "playerFound", p != nil)
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen || p.Player.Warehouse == nil ||
		cmd.Count <= 0 {
		return
	}
	old := p.Player.Warehouse
	w := old.Clone()
	page, slot := int(cmd.Tab), int(cmd.Slot)
	if w == nil || page >= w.PageCount() || slot < 0 || slot >= w.SlotsPerPage() {
		return
	}
	src := w.At(page, slot)
	def, ok := s.itemDef(src.Item)
	if src.Empty() || !ok || !def.Stackable || src.UID != 0 || cmd.Count >= src.Count {
		return
	}
	to := w.FirstEmpty(page)
	if to < 0 {
		return
	}
	remain, moved, ok := removeStackCount(src, cmd.Count)
	if !ok || !w.Set(page, slot, remain) || !w.Set(page, to, moved) {
		return
	}
	p.Player.Warehouse = w
	if s.commitWarehouse(p, p.Player.Bag, old, p.Player.Char.Money, false) {
		s.log.Debug("拆分个人仓库物品", "char", p.Name, "tab", page,
			"from", slot, "to", to, "count", cmd.Count)
	}
}

func (s *Scene) onWarehouseRearrange(cmd WarehouseRearrange) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen || p.Player.Warehouse == nil ||
		cmd.FromTab != cmd.ToTab {
		return
	}
	old := p.Player.Warehouse
	w := old.Clone()
	page, from, to := int(cmd.FromTab), int(cmd.FromSlot), int(cmd.ToSlot)
	if w == nil || page >= w.PageCount() || from < 0 || to < 0 || from == to ||
		from >= w.SlotsPerPage() || to >= w.SlotsPerPage() {
		return
	}
	src, dst := w.At(page, from), w.At(page, to)
	if src.Empty() {
		return
	}
	def, ok := s.itemDef(src.Item)
	if !ok {
		return
	}
	if def.Stackable && src.UID == 0 && domain.SameStackInstance(src, dst) && dst.Count < w.MaxStack() {
		put := w.MaxStack() - dst.Count
		if put > src.Count {
			put = src.Count
		}
		dst.Count += put
		src.Count -= put
		if src.Count == 0 {
			src = domain.Stack{}
		}
		w.Set(page, from, src)
		w.Set(page, to, dst)
	} else {
		w.Set(page, from, dst)
		w.Set(page, to, src)
	}
	p.Player.Warehouse = w
	s.commitWarehouse(p, p.Player.Bag, old, p.Player.Char.Money, false)
}

func (s *Scene) onSortWarehouse(cmd SortWarehouse) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || !p.Player.WarehouseOpen || p.Player.Warehouse == nil {
		return
	}
	old := p.Player.Warehouse
	w := old.Clone()
	page := int(cmd.Tab)
	if w == nil || page >= w.PageCount() {
		return
	}
	var items []domain.Stack
	for slot := 0; slot < w.SlotsPerPage(); slot++ {
		if st := w.At(page, slot); !st.Empty() {
			items = append(items, st)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Item != items[j].Item {
			return items[i].Item < items[j].Item
		}
		if items[i].Locked != items[j].Locked {
			return !items[i].Locked
		}
		if items[i].Bound != items[j].Bound {
			return !items[i].Bound
		}
		return items[i].UID < items[j].UID
	})
	merged := make([]domain.Stack, 0, len(items))
	for _, st := range items {
		def, ok := s.itemDef(st.Item)
		if !ok {
			return
		}
		if def.Stackable && st.UID == 0 && len(merged) > 0 {
			last := &merged[len(merged)-1]
			if domain.SameStackInstance(*last, st) && last.Count < w.MaxStack() {
				put := w.MaxStack() - last.Count
				if put > st.Count {
					put = st.Count
				}
				last.Count += put
				st.Count -= put
			}
		}
		if st.Count > 0 {
			merged = append(merged, st)
		}
	}
	for slot := 0; slot < w.SlotsPerPage(); slot++ {
		var st domain.Stack
		if slot < len(merged) {
			st = merged[slot]
		}
		w.Set(page, slot, st)
	}
	p.Player.Warehouse = w
	s.commitWarehouse(p, p.Player.Bag, old, p.Player.Char.Money, false)
}
