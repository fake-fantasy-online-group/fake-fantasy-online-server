package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// onSetItemLock 在场景 actor 内切换实例保护。账号安全码只属于连接身份，已在
// session 层校验；这里负责物品身份、分页、格号、持久状态和客户端快照。
func (s *Scene) onSetItemLock(cmd SetItemLock) {
	p := s.players[cmd.ID]
	result := event.ItemLockChanged{Who: cmd.ID, On: cmd.On, Tab: cmd.Tab,
		Slot: cmd.Slot, Item: cmd.Item, Message: "物品不存在或已发生变化"}
	if p == nil || p.Player == nil || p.Player.Bag == nil || cmd.Slot < 0 || int(cmd.Slot) >= p.Player.Bag.Cap() {
		if p != nil {
			s.emitTo(p.ID, result)
		}
		return
	}

	st := p.Player.Bag.At(int(cmd.Slot))
	def, ok := s.itemDef(st.Item)
	if st.Empty() || !ok || st.Item != cmd.Item || !def.InventoryTabKnown || def.InventoryTab != cmd.Tab {
		s.emitTo(p.ID, result)
		_ = s.pushInventory(p)
		return
	}
	if st.Locked == cmd.On {
		result.OK = true
		if cmd.On {
			result.Message = "物品已经锁定"
		} else {
			result.Message = "物品已经解锁"
		}
		s.emitTo(p.ID, result)
		_ = s.pushInventory(p)
		return
	}

	planned := p.Player.Bag.Clone()
	st.Locked = cmd.On
	if planned == nil || !planned.Set(int(cmd.Slot), st) {
		s.emitTo(p.ID, result)
		return
	}
	old := p.Player.Bag
	p.Player.Bag = planned
	snap, err := s.inventorySnapshot(p)
	if err != nil {
		p.Player.Bag = old
		result.Message = "背包状态更新失败"
		s.emitTo(p.ID, result)
		return
	}

	result.OK = true
	if cmd.On {
		result.Message = "物品锁定成功"
	} else {
		result.Message = "物品解锁成功"
	}
	s.emitTo(p.ID, result)
	s.emitTo(p.ID, snap)
	if p.Player.Char != nil && p.Player.Char.PetByItem(st.UID) != nil {
		s.emitTo(p.ID, s.petSnapshot(p))
	}
	s.pushQuestLog(p)
	p.Player.RepairQuote = nil
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("切换物品锁定", "char", p.Name, "item", domain.ItemID(cmd.Item),
		"tab", cmd.Tab, "slot", cmd.Slot, "locked", cmd.On)
}
