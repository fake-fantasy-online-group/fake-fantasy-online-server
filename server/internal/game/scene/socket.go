package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	itemSeparationScroll domain.ItemID = 12581
	itemBindingShears    domain.ItemID = 25361
)

type socketTargetRef struct {
	kind      uint8
	equipSlot uint8
	tab       uint8
	bagSlot   int32
}

func (s *Scene) socketTarget(p *entity.Entity, ref socketTargetRef) (domain.Stack, domain.ItemDef, bool) {
	var st domain.Stack
	if ref.kind == 0 {
		if p.Player.Bag == nil || ref.bagSlot < 0 || int(ref.bagSlot) >= p.Player.Bag.Cap() {
			return st, domain.ItemDef{}, false
		}
		st = p.Player.Bag.At(int(ref.bagSlot))
	} else if ref.kind == 1 {
		if p.Player.Worn == nil {
			return st, domain.ItemDef{}, false
		}
		st = p.Player.Worn.At(domain.EquipSlot(ref.equipSlot))
	} else {
		return st, domain.ItemDef{}, false
	}
	def, ok := s.itemDef(st.Item)
	if st.Empty() || st.Locked || !ok || def.Equip == nil ||
		(ref.kind == 0 && (!def.InventoryTabKnown || def.InventoryTab != ref.tab)) {
		return st, def, false
	}
	return st, def, true
}

func setSocketTarget(p *entity.Entity, ref socketTargetRef, st domain.Stack) {
	if ref.kind == 0 {
		p.Player.Bag.Set(int(ref.bagSlot), st)
	} else {
		p.Player.Worn.Set(domain.EquipSlot(ref.equipSlot), st)
	}
}

func (s *Scene) socketInfo(p *entity.Entity, ref socketTargetRef) event.SocketInfo {
	info := event.SocketInfo{Who: p.ID}
	st, def, ok := s.socketTarget(p, ref)
	if !ok {
		return info
	}
	info.Item, info.SocketCount = st.Item, st.SocketCount
	info.MaxHoles = uint8(domain.EquipmentSocketCapacity(def, st))
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		info.Sockets = append(info.Sockets, st.Sockets[i])
		if st.Sockets[i] != 0 {
			if socketDef, ok := s.itemDef(st.Sockets[i]); ok {
				info.SocketNames = append(info.SocketNames, event.SocketNameView{Item: st.Sockets[i], Name: socketDef.Name})
			}
		}
	}
	if st.SocketCount >= info.MaxHoles {
		info.Reason = "已达到最大孔数"
		return info
	}
	recipe, ok := s.sockets.Get(st.SocketCount, domain.RefineClass(def), def.Equip.Tier)
	if !ok {
		info.Reason = "没有适用的打孔配方"
		return info
	}
	info.NextCost, info.NextSuccess = int32(recipe.DrillCost), recipe.SuccessRate
	info.FailDestroys = recipe.DestroyRate > 0
	money, err := inventoryMoney(p.Player.Char.Money)
	info.CanDrill = err == nil && money >= recipe.DrillCost
	if recipe.RequiredSkill != 0 {
		_, learned := p.Player.Char.Skills[recipe.RequiredSkill]
		info.CanDrill = info.CanDrill && learned
	}
	for _, material := range recipe.Materials {
		mat, _ := s.itemDef(material.Item)
		have := p.Player.Bag.UsableCountOf(material.Item)
		info.Materials = append(info.Materials, event.CraftMaterialView{Item: material.Item,
			Need: material.Qty, Have: have, Name: mat.Name})
		info.CanDrill = info.CanDrill && have >= material.Qty
	}
	return info
}

func (s *Scene) onDrillItem(cmd DrillItem) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil || s.sockets == nil {
		return
	}
	ref := socketTargetRef{kind: cmd.TargetKind, equipSlot: cmd.EquipSlot, tab: cmd.Tab, bagSlot: cmd.BagSlot}
	info := s.socketInfo(p, ref)
	if !cmd.Execute {
		s.emitTo(p.ID, info)
		return
	}
	st, def, ok := s.socketTarget(p, ref)
	if !ok || !info.CanDrill || int(st.SocketCount) >= len(st.Sockets) {
		s.emitTo(p.ID, info)
		return
	}
	recipe, ok := s.sockets.Get(st.SocketCount, domain.RefineClass(def), def.Equip.Tier)
	if !ok {
		return
	}
	nextBag := p.Player.Bag.Clone()
	for _, material := range recipe.Materials {
		if nextBag == nil || !nextBag.Remove(material.Item, material.Qty) {
			return
		}
	}
	if cmd.Protect && (recipe.ProtectItem == 0 || !nextBag.Remove(recipe.ProtectItem, 1)) {
		return
	}
	money, _ := inventoryMoney(p.Player.Char.Money)
	p.Player.Char.Money = moneyFromInventory(money - recipe.DrillCost)
	roll := int32(s.rng.Intn(100))
	succeeded := roll < recipe.SuccessRate
	destroyed := !succeeded && !cmd.Protect && int32(s.rng.Intn(100)) < recipe.DestroyRate
	if succeeded {
		st.SocketCount++
	} else if destroyed {
		st = domain.Stack{}
	}
	p.Player.Bag = nextBag
	setSocketTarget(p, ref, st)
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	if ref.kind == 1 {
		s.refreshStats(p)
	}
	text := "打孔失败。"
	if succeeded {
		text = "打孔成功。"
	} else if destroyed {
		text = "打孔失败，装备损毁。"
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	s.emitTo(p.ID, s.socketInfo(p, ref))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}

func (s *Scene) onInlayItem(cmd InlayItem) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || s.sockets == nil {
		return
	}
	ref := socketTargetRef{kind: cmd.TargetKind, equipSlot: cmd.EquipSlot, tab: cmd.Tab, bagSlot: cmd.BagSlot}
	st, def, ok := s.socketTarget(p, ref)
	if !ok || int(cmd.Hole) >= int(st.SocketCount) || int(cmd.Hole) >= domain.EquipmentSocketCapacity(def, st) ||
		int(cmd.Hole) >= len(st.Sockets) || st.Sockets[cmd.Hole] != 0 ||
		cmd.RuneBagSlot < 0 || int(cmd.RuneBagSlot) >= p.Player.Bag.Cap() {
		return
	}
	rune := p.Player.Bag.At(int(cmd.RuneBagSlot))
	runeDef, ok := s.itemDef(rune.Item)
	if !ok || rune.Empty() || rune.Locked || !rune.Card.Initialized || rune.Count != 1 || rune.UID <= 0 ||
		rune.InstanceKind != domain.ItemInstanceSocketCard ||
		runeDef.InstanceKind != domain.ItemInstanceSocketCard ||
		!runeDef.InventoryTabKnown || runeDef.InventoryTab != cmd.RuneTab {
		return
	}
	recipe, ok := s.sockets.Get(cmd.Hole, domain.RefineClass(def), def.Equip.Tier)
	if !ok {
		return
	}
	if recipe.RequiredSkill != 0 {
		if _, learned := p.Player.Char.Skills[recipe.RequiredSkill]; !learned {
			return
		}
	}
	nextBag := p.Player.Bag.Clone()
	if nextBag == nil || !nextBag.RemoveAt(int(cmd.RuneBagSlot), 1) {
		return
	}
	for _, material := range recipe.Materials {
		if !nextBag.Remove(material.Item, material.Qty) {
			return
		}
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < recipe.InlayCost {
		return
	}
	p.Player.Char.Money = moneyFromInventory(money - recipe.InlayCost)
	roll := int32(s.rng.Intn(100))
	succeeded := roll < recipe.SuccessRate
	destroyed := !succeeded && int32(s.rng.Intn(100)) < recipe.DestroyRate
	if succeeded {
		st.Sockets[cmd.Hole] = rune.Item
		st.SocketUIDs[cmd.Hole] = rune.UID
		st.SocketCards[cmd.Hole] = rune.Card
		st.SocketCards[cmd.Hole].Bound = rune.Bound
		st.SocketCards[cmd.Hole].Locked = rune.Locked
	} else if destroyed {
		st = domain.Stack{}
	}
	p.Player.Bag = nextBag
	setSocketTarget(p, ref, st)
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	if ref.kind == 1 {
		s.refreshStats(p)
	}
	text := "镶嵌失败。"
	if succeeded {
		text = "镶嵌成功。"
	} else if destroyed {
		text = "镶嵌失败，装备损毁。"
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	s.emitTo(p.ID, s.socketInfo(p, ref))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}

func (s *Scene) onUseItemOn(cmd UseItemOn) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	ref := socketTargetRef{kind: cmd.TargetKind, tab: cmd.Tab, bagSlot: cmd.Slot}
	if cmd.TargetKind == 1 {
		ref.equipSlot = uint8(cmd.Slot)
	}
	st, _, ok := s.socketTarget(p, ref)
	if !ok || p.Player.Bag.UsableCountOf(cmd.Tool) <= 0 {
		return
	}
	next := p.Player.Bag.Clone()
	if next == nil || !next.Remove(cmd.Tool, 1) {
		return
	}
	text := ""
	switch cmd.Tool {
	case itemBindingShears:
		if !st.Bound {
			return
		}
		st.Bound = false
		text = "装备已解除绑定。"
	case itemSeparationScroll:
		separated := false
		for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
			if st.Sockets[i] == 0 {
				continue
			}
			def, ok := s.itemDef(st.Sockets[i])
			if !ok || def.InstanceKind != domain.ItemInstanceSocketCard || st.SocketUIDs[i] <= 0 || !st.SocketCards[i].Initialized {
				return
			}
			card := domain.Stack{UID: st.SocketUIDs[i], Item: st.Sockets[i], Count: 1,
				InstanceKind: domain.ItemInstanceSocketCard, Card: st.SocketCards[i],
				Bound: st.SocketCards[i].Bound, Locked: st.SocketCards[i].Locked}
			if next.AddStack(def, card) != 0 {
				return
			}
			st.Sockets[i] = 0
			st.SocketUIDs[i] = 0
			st.SocketCards[i] = domain.CardInstanceState{}
			separated = true
		}
		if !separated {
			return
		}
		text = "镶嵌物已分离。"
	default:
		return
	}
	p.Player.Bag = next
	setSocketTarget(p, ref, st)
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	if ref.kind == 1 {
		s.refreshStats(p)
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	s.emitTo(p.ID, s.socketInfo(p, ref))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}
