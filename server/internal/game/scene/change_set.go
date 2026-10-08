package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) changeSetSnapshot(p *entity.Entity) (event.ChangeSetSnapshot, error) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.ChangeSet == nil {
		return event.ChangeSetSnapshot{}, fmt.Errorf("快速换装状态不可用")
	}
	out := event.ChangeSetSnapshot{Who: p.ID, Items: make([]event.ChangeSetItemView, 0, p.Player.ChangeSet.Count())}
	var buildErr error
	p.Player.ChangeSet.Each(func(cell domain.EquipSlot, st domain.Stack) {
		if buildErr != nil {
			return
		}
		def, ok := s.itemDef(st.Item)
		if !ok || def.Equip == nil || domain.ChangeSetCell(domain.EquipSlot(def.Equip.Slot)) != cell {
			buildErr = fmt.Errorf("部位 %d 的备用装备 %d 定义不匹配", cell, st.Item)
			return
		}
		out.Items = append(out.Items, event.ChangeSetItemView{
			Cell: cell, Item: st.Item, Name: itemDisplayName(def, st), Desc: s.ownedItemTooltip(def, st, p),
			CurDur: st.Durability, MaxDur: domain.MaxDurabilityOf(st, def),
			Refine: st.RefineLevel, Quality: equipmentQuality(def, st),
			Blocked: s.equipmentBlockedFor(p, def),
		})
	})
	if buildErr != nil {
		return event.ChangeSetSnapshot{}, fmt.Errorf("角色 %s 快速换装快照: %w", p.Name, buildErr)
	}
	return out, nil
}

func (s *Scene) pushChangeSet(p *entity.Entity) bool {
	snap, err := s.changeSetSnapshot(p)
	if err != nil {
		s.log.Error("快速换装快照失败", "char", p.Name, "err", err)
		return false
	}
	s.emitTo(p.ID, snap)
	return true
}

// 仅角色条件/穿戴状态变化时，无备用物品就没有待刷新的提示。
// 物品移入/取出仍用 pushChangeSet，无论是否为空都发送完整快照。
func (s *Scene) refreshChangeSetTooltips(p *entity.Entity) {
	if p.Player.ChangeSet != nil && p.Player.ChangeSet.Count() > 0 {
		s.pushChangeSet(p)
	}
}

func (s *Scene) rejectChangeSet(p *entity.Entity, reason event.RejectReason) {
	s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "ChangeSet", Reason: reason})
}

func changeSetPlayerBusy(p *entity.Entity) bool {
	return p.Player.Stall != nil || p.Player.TradeBusy || p.Player.FamilyBusy || p.Player.RackBusy
}

func (s *Scene) onChangeSetItem(cmd ChangeSetItem) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil || p.Player.ChangeSet == nil ||
		!domain.ValidChangeSetCell(cmd.Cell) {
		return
	}
	if changeSetPlayerBusy(p) {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}
	if cmd.Put {
		s.putChangeSetItem(p, cmd)
		return
	}
	s.takeChangeSetItem(p, cmd.Cell)
}

func (s *Scene) putChangeSetItem(p *entity.Entity, cmd ChangeSetItem) {
	if cmd.BagSlot < 0 || cmd.BagSlot >= p.Player.Bag.Cap() {
		s.rejectChangeSet(p, event.RejectNoItem)
		return
	}
	st := p.Player.Bag.At(cmd.BagSlot)
	def, ok := s.itemDef(st.Item)
	if st.Empty() || !ok || def.Equip == nil || !def.InventoryTabKnown ||
		def.InventoryTab != cmd.BagTab || st.Count != 1 {
		s.rejectChangeSet(p, event.RejectNotEquippable)
		return
	}
	if st.Locked || domain.ChangeSetCell(domain.EquipSlot(def.Equip.Slot)) != cmd.Cell {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}
	if def.Equip.Durable > 0 && !def.Equip.NoLimitDurability &&
		st.MaxDurability > 0 && st.Durability <= 0 {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}
	nextBag := p.Player.Bag.Clone()
	nextSet := p.Player.ChangeSet.Clone()
	if nextBag == nil || nextSet == nil || !nextBag.Set(cmd.BagSlot, nextSet.At(cmd.Cell)) ||
		!nextSet.Set(cmd.Cell, st) || !s.changeSetCompatible(nextSet) {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}
	p.Player.Bag, p.Player.ChangeSet = nextBag, nextSet
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	s.pushChangeSet(p)
}

func (s *Scene) takeChangeSetItem(p *entity.Entity, cell domain.EquipSlot) {
	st := p.Player.ChangeSet.At(cell)
	if st.Empty() {
		return
	}
	nextBag := p.Player.Bag.Clone()
	nextSet := p.Player.ChangeSet.Clone()
	def, known := s.itemDef(st.Item)
	if !known {
		s.rejectChangeSet(p, event.RejectNotEquippable)
		return
	}
	free := nextBag.FirstEmptyFor(def)
	if free < 0 {
		s.rejectChangeSet(p, event.RejectBagFull)
		return
	}
	nextBag.Set(free, st)
	nextSet.Set(cell, domain.Stack{})
	p.Player.Bag, p.Player.ChangeSet = nextBag, nextSet
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	s.pushChangeSet(p)
}

func (s *Scene) changeSetCompatible(set *domain.ChangeSet) bool {
	if set == nil {
		return false
	}
	weapon := set.At(domain.SlotWeapon)
	if weapon.Empty() || set.At(domain.SlotShield).Empty() {
		return true
	}
	def, ok := s.itemDef(weapon.Item)
	return ok && def.Equip != nil && domain.EquipSlot(def.Equip.Slot) != domain.SlotTwoHand
}

func (s *Scene) onChangeSetSwap(cmd ChangeSetSwap) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Worn == nil ||
		p.Player.ChangeSet == nil {
		return
	}
	if changeSetPlayerBusy(p) || p.Player.ChangeSet.Count() == 0 || !s.changeSetCompatible(p.Player.ChangeSet) {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}

	// 先完整验证备用套装，再改任何权威状态。客户端只能声明“切换”，不能借此
	// 绕过普通穿戴的等级、六维、性别、职业、锁定和耐久门槛。
	valid := true
	p.Player.ChangeSet.Each(func(cell domain.EquipSlot, st domain.Stack) {
		if !valid {
			return
		}
		def, ok := s.itemDef(st.Item)
		if !ok || def.Equip == nil || st.Locked ||
			domain.ChangeSetCell(domain.EquipSlot(def.Equip.Slot)) != cell ||
			def.Equip.Need.Meet(p.Player.Char.Level, s.equipmentRequirementBaseFromWorn(p, s.functionalWorn(p), domain.EquipSlot(def.Equip.Slot)),
				p.Player.Char.Appear.Gender) != domain.RejectNone ||
			!def.Equip.Need.AllowsCharacter(p.Player.Char) ||
			def.Equip.Durable > 0 && !def.Equip.NoLimitDurability &&
				st.MaxDurability > 0 && st.Durability <= 0 {
			valid = false
		}
	})
	if !valid {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}

	nextWorn := p.Player.Worn.Clone()
	nextSet := domain.NewChangeSet()
	if nextWorn == nil {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}
	currentOK := true
	p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		cell := domain.ChangeSetCell(slot)
		if cell == 0 {
			return
		}
		if !nextSet.At(cell).Empty() || !nextSet.Set(cell, st) {
			currentOK = false
		}
	})
	if !currentOK || !s.changeSetCompatible(nextSet) {
		s.rejectChangeSet(p, event.RejectInvalid)
		return
	}
	for slot := domain.SlotFace; slot <= domain.SlotTreasure; slot++ {
		nextWorn.Set(slot, domain.Stack{})
	}
	nextWorn.Set(domain.SlotTwoHand, domain.Stack{})
	p.Player.ChangeSet.Each(func(_ domain.EquipSlot, st domain.Stack) {
		def, _ := s.itemDef(st.Item)
		nextWorn.Set(domain.EquipSlot(def.Equip.Slot), st)
	})

	oldTreasureSkill := s.equippedTreasureSkill(p)
	p.Player.Worn, p.Player.ChangeSet = nextWorn, nextSet
	s.refreshStats(p)
	s.refreshAppearance(p)
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	s.pushChangeSet(p)
	if newTreasureSkill := s.equippedTreasureSkill(p); equipmentSkillID(oldTreasureSkill) != equipmentSkillID(newTreasureSkill) {
		s.emitTo(p.ID, s.skillSnapshot(p))
	}
}
