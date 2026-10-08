package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

var familyMailStationery = [...]domain.ItemID{3313, 3314, 3315}

func (s *Scene) familyReply(ch chan<- FamilyReserveResult, result FamilyReserveResult) {
	if ch == nil {
		return
	}
	select {
	case ch <- result:
	default:
		s.log.Warn("家族事务保留结果无人接收", "char", result.Reservation.ID)
	}
}

func (s *Scene) onReserveFamilyCreate(cmd ReserveFamilyCreate) {
	result := FamilyReserveResult{}
	defer func() { s.familyReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.FamilyBusy {
		result.Reason = "角色状态暂不可用"
		return
	}
	ch := p.Player.Char
	if ch.Level < domain.FamilyCreateMinLevel {
		result.Reason = "创建家族需要达到40级"
		return
	}
	if ch.Honor < domain.FamilyCreateHonor {
		result.Reason = "创建家族需要300点名誉"
		return
	}
	money, err := inventoryMoney(ch.Money)
	if err != nil || money < domain.FamilyCreateCost {
		result.Reason = "创建家族需要10金币"
		return
	}
	result.Reservation = FamilyReservation{ID: p.ID, BeforeMoney: ch.Money, BeforeCaiyu: ch.Caiyu}
	ch.Money = moneyFromInventory(money - domain.FamilyCreateCost)
	p.Player.FamilyBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
}

func (s *Scene) onReserveFamilyMail(cmd ReserveFamilyMail) {
	result := FamilyReserveResult{}
	defer func() { s.familyReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || p.Player.FamilyBusy {
		result.Reason = "角色背包状态暂不可用"
		return
	}
	bag := p.Player.Bag.Clone()
	var consumed domain.ItemID
	for _, item := range familyMailStationery {
		if bag.UsableCountOf(item) > 0 && bag.Remove(item, 1) {
			consumed = item
			break
		}
	}
	if consumed == 0 {
		result.Reason = "发送家族邮件需要家族羽笺、家族丝笺或家族玉笺"
		return
	}
	result.Reservation = FamilyReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(), BeforeMoney: p.Player.Char.Money, BeforeCaiyu: p.Player.Char.Caiyu}
	p.Player.Bag = bag
	p.Player.FamilyBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	if def, ok := s.itemDef(consumed); ok {
		result.Consumed = def.Name
	}
}

func (s *Scene) onReserveFamilyStashDeposit(cmd ReserveFamilyStashDeposit) {
	result := FamilyReserveResult{}
	defer func() { s.familyReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.FamilyBusy || cmd.BagSlot < 0 || int(cmd.BagSlot) >= p.Player.Bag.Cap() {
		result.Reason = "角色背包或家族状态不可用"
		return
	}
	stack := p.Player.Bag.At(int(cmd.BagSlot))
	if stack.InstanceKind == domain.ItemInstancePet {
		result.Reason = "宠物不能存入家族仓库"
		return
	}
	def, ok := s.itemDef(stack.Item)
	if stack.Empty() || !ok || stack.Locked || stack.Bound || def.InventoryTab != cmd.BagTab {
		result.Reason = "该物品不能存入家族仓库"
		return
	}
	bag := p.Player.Bag.Clone()
	if bag == nil || !bag.Set(int(cmd.BagSlot), domain.Stack{}) {
		result.Reason = "背包状态不可用"
		return
	}
	result.Reservation = FamilyReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeMoney: p.Player.Char.Money, BeforeCaiyu: p.Player.Char.Caiyu}
	p.Player.Bag = bag
	p.Player.FamilyBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	result.Stack = stack
}

func (s *Scene) onReserveFamilyStashWithdraw(cmd ReserveFamilyStashWithdraw) {
	result := FamilyReserveResult{}
	defer func() { s.familyReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.FamilyBusy || cmd.Stack.Empty() {
		result.Reason = "角色背包或家族状态不可用"
		return
	}
	def, ok := s.itemDef(cmd.Stack.Item)
	bag := p.Player.Bag.Clone()
	if !ok || bag == nil || !putBag(bag, def, -1, cmd.Stack) {
		result.Reason = "背包空间不足"
		return
	}
	result.Reservation = FamilyReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeMoney: p.Player.Char.Money, BeforeCaiyu: p.Player.Char.Caiyu}
	p.Player.Bag = bag
	p.Player.FamilyBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	result.Stack = cmd.Stack
}

func (s *Scene) onShowFamilyStash(cmd ShowFamilyStash) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || cmd.Stash.Capacity <= 0 {
		return
	}
	out := event.FamilyStashSnapshot{Who: p.ID, Capacity: cmd.Stash.Capacity,
		TakePos: uint8(cmd.Stash.TakePos), CanTake: cmd.Stash.CanTake, IsLeader: cmd.Stash.IsLeader}
	for _, entry := range cmd.Stash.Entries {
		def, ok := s.itemDef(entry.Stack.Item)
		if !ok || entry.Stack.Empty() {
			continue
		}
		view := event.FamilyStashView{Item: entry.Stack.Item, Count: entry.Stack.Count,
			Name: itemDisplayName(def, entry.Stack), Desc: s.itemTooltip(def, entry.Stack),
			Quality: int32(equipmentQuality(def, entry.Stack)), DepositedBy: entry.By}
		if def.Equip != nil {
			view.CurDur = entry.Stack.Durability
			view.MaxDur = domain.MaxDurabilityOf(entry.Stack, def)
		}
		out.Entries = append(out.Entries, view)
	}
	s.emitTo(p.ID, out)
}

func (s *Scene) onFinalizeFamilyReservation(cmd FinalizeFamilyReservation) {
	if cmd.Done != nil {
		defer close(cmd.Done)
	}
	p := s.players[cmd.Reservation.ID]
	if p == nil || p.Player == nil {
		return
	}
	if !cmd.Commit {
		p.Player.Char.Money = cmd.Reservation.BeforeMoney
		p.Player.Char.Caiyu = cmd.Reservation.BeforeCaiyu
		if cmd.Reservation.BeforeBag != nil {
			p.Player.Bag = cmd.Reservation.BeforeBag
		}
	}
	p.Player.FamilyBusy = false
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
}
