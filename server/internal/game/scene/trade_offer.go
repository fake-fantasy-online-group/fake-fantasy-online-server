package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
)

// onValidateTradeOffer turns untrusted client slot references into immutable item
// facts while the scene actor owns the bag. It does not reserve or remove items;
// final settlement must revalidate and mutate atomically in the same actor.
func (s *Scene) onValidateTradeOffer(cmd ValidateTradeOffer) {
	result := TradeOfferResult{Money: cmd.Money}
	reply := func() {
		if cmd.Reply == nil {
			return
		}
		select {
		case cmd.Reply <- result:
		default:
			s.log.Warn("交易报价结果无人接收", "id", cmd.ID)
		}
	}
	defer reply()

	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil || p.Player.Char == nil ||
		p.Player.TradeBusy || p.Player.Stall != nil || p.Player.StallBusy || cmd.Money < 0 {
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || cmd.Money > money || len(cmd.Items) > 255 {
		return
	}

	seen := make(map[int32]struct{}, len(cmd.Items))
	items := make([]TradeOfferItem, 0, len(cmd.Items))
	for _, ref := range cmd.Items {
		if ref.Slot < 0 || ref.Count <= 0 {
			return
		}
		if _, duplicate := seen[ref.Slot]; duplicate {
			return
		}
		seen[ref.Slot] = struct{}{}
		stack := p.Player.Bag.At(int(ref.Slot))
		if stack.Empty() || stack.Bound || stack.Locked || ref.Count > stack.Count {
			return
		}
		def, ok := s.itemDef(stack.Item)
		if !ok || !def.CanTrade || !def.InventoryTabKnown || def.InventoryTab != ref.Tab || !s.petTradeAllowed(p, stack) {
			return
		}
		offered := stack
		offered.Count = ref.Count
		items = append(items, TradeOfferItem{
			Tab: ref.Tab, Slot: ref.Slot, Stack: offered, Pet: s.petItemInfo(p, offered), Name: s.ownedItemName(p, def, offered),
			Info: s.itemTooltip(def, offered), Quality: equipmentQuality(def, offered),
		})
	}
	result.OK = true
	result.Items = items
}

func (s *Scene) onReserveTradeSettlement(cmd ReserveTradeSettlement) {
	result := TradeSettlementResult{}
	defer func() {
		if cmd.Reply == nil {
			return
		}
		select {
		case cmd.Reply <- result:
		default:
			// No caller owns this reservation, so no transaction can be committed.
			if result.Reservation.Result != nil {
				s.onFinalizeTradeSettlement(FinalizeTradeSettlement{Reservation: result.Reservation})
			}
			s.log.Warn("交易结算保留结果无人接收", "a", cmd.A, "b", cmd.B)
		}
	}()

	if cmd.Reply == nil {
		return
	}
	select {
	case <-cmd.Cancel:
		result.Reason = "交易确认已超时"
		return
	default:
	}
	a, b := s.players[cmd.A], s.players[cmd.B]
	if a == nil || b == nil || a == b || a.Player == nil || b.Player == nil ||
		a.Player.Char == nil || b.Player.Char == nil || a.Player.Bag == nil || b.Player.Bag == nil ||
		a.Player.TradeBusy || b.Player.TradeBusy || a.Player.Stall != nil || b.Player.Stall != nil ||
		a.Player.StallBusy || b.Player.StallBusy || a.Player.FamilyBusy || b.Player.FamilyBusy ||
		a.Player.RackBusy || b.Player.RackBusy || a.Player.MailBusy || b.Player.MailBusy {
		result.Reason = "交易双方状态已经变化"
		return
	}
	aItems, reason := s.validateTradeSettlementOffer(a, cmd.AOffer)
	if reason != "" {
		result.Reason = "报价已变化：" + reason
		return
	}
	bItems, reason := s.validateTradeSettlementOffer(b, cmd.BOffer)
	if reason != "" {
		result.Reason = "报价已变化：" + reason
		return
	}

	aBag, bBag := a.Player.Bag.Clone(), b.Player.Bag.Clone()
	if aBag == nil || bBag == nil {
		result.Reason = "背包状态无效"
		return
	}
	for _, item := range cmd.AOffer.Items {
		if !aBag.RemoveAt(int(item.Slot), item.Stack.Count) {
			result.Reason = "报价已变化：物品数量不足"
			return
		}
	}
	for _, item := range cmd.BOffer.Items {
		if !bBag.RemoveAt(int(item.Slot), item.Stack.Count) {
			result.Reason = "报价已变化：物品数量不足"
			return
		}
	}
	for _, stack := range aItems {
		def, _ := s.itemDef(stack.Item)
		if bBag.AddStack(def, stack) != 0 {
			result.Reason = "有一方背包空间不足"
			return
		}
	}
	for _, stack := range bItems {
		def, _ := s.itemDef(stack.Item)
		if aBag.AddStack(def, stack) != 0 {
			result.Reason = "有一方背包空间不足"
			return
		}
	}

	aMoney, aMoneyErr := inventoryMoney(a.Player.Char.Money)
	bMoney, bMoneyErr := inventoryMoney(b.Player.Char.Money)
	aAfter, aAddOK := checkedAdd64(aMoney-cmd.AOffer.Money, cmd.BOffer.Money)
	bAfter, bAddOK := checkedAdd64(bMoney-cmd.BOffer.Money, cmd.AOffer.Money)
	if aMoneyErr != nil || bMoneyErr != nil || aMoney < cmd.AOffer.Money || bMoney < cmd.BOffer.Money ||
		!aAddOK || !bAddOK {
		result.Reason = "交易金钱不足或已达上限"
		return
	}

	aPets, bPets, petErr := s.transferPetLists(a, b, aItems, bItems)
	if petErr != "" {
		result.Reason = petErr
		return
	}
	result.Reservation = TradeSettlementReservation{Players: []TradeSettlementPlayer{
		{ID: a.ID, BeforeBag: a.Player.Bag.Clone(), BeforeMoney: a.Player.Char.Money, BeforePets: domain.ClonePets(a.Player.Char.Pets)},
		{ID: b.ID, BeforeBag: b.Player.Bag.Clone(), BeforeMoney: b.Player.Char.Money, BeforePets: domain.ClonePets(b.Player.Char.Pets)},
	}}
	a.Player.Bag, a.Player.Char.Money, a.Player.TradeBusy = aBag, moneyFromInventory(aAfter), true
	b.Player.Bag, b.Player.Char.Money, b.Player.TradeBusy = bBag, moneyFromInventory(bAfter), true
	s.assignPetList(a, aPets)
	s.assignPetList(b, bPets)
	s.trackTradeSettlement(&result.Reservation, cmd.Cancel)
	result.Snapshots = []domain.Snapshot{s.snapshotOf(a), s.snapshotOf(b)}
}

func (s *Scene) validateTradeSettlementOffer(p *entity.Entity, offer TradeSettlementOffer) ([]domain.Stack, string) {
	if p == nil || p.Player == nil || p.Player.Bag == nil || offer.Money < 0 || len(offer.Items) > 255 {
		return nil, "报价无效"
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < offer.Money {
		return nil, "随身金钱不足"
	}
	seen := make(map[int32]struct{}, len(offer.Items))
	stacks := make([]domain.Stack, 0, len(offer.Items))
	for _, item := range offer.Items {
		if item.Slot < 0 || item.Stack.Empty() || item.Stack.Count <= 0 {
			return nil, "物品引用无效"
		}
		if _, duplicate := seen[item.Slot]; duplicate {
			return nil, "同一背包格重复报价"
		}
		seen[item.Slot] = struct{}{}
		actual := p.Player.Bag.At(int(item.Slot))
		def, ok := s.itemDef(actual.Item)
		if !ok || actual.Empty() || actual.Count < item.Stack.Count || actual.Bound || actual.Locked ||
			!def.CanTrade || !def.InventoryTabKnown || def.InventoryTab != item.Tab ||
			!sameTradeInstance(actual, item.Stack) || !s.petTradeAllowed(p, actual) || !reflect.DeepEqual(item.Pet, s.petItemInfo(p, actual)) {
			return nil, "物品状态已经变化"
		}
		transfer := actual
		transfer.Count = item.Stack.Count
		stacks = append(stacks, transfer)
	}
	return stacks, ""
}

func sameTradeInstance(actual, expected domain.Stack) bool {
	actual.Count = expected.Count
	return actual == expected
}

func (s *Scene) onFinalizeTradeSettlement(cmd FinalizeTradeSettlement) {
	if cmd.Done != nil {
		defer close(cmd.Done)
	}
	done := s.removePendingTrade(cmd.Reservation)
	if done == nil {
		return
	}
	defer close(done)
	for _, saved := range cmd.Reservation.Players {
		p := s.players[saved.ID]
		if p == nil || p.Player == nil || p.Player.Char == nil {
			continue
		}
		if !cmd.Commit {
			p.Player.Bag = saved.BeforeBag
			p.Player.Char.Money = saved.BeforeMoney
			s.assignPetList(p, saved.BeforePets)
		}
		p.Player.TradeBusy = false
		p.Player.MarkDirty()
		_ = s.pushInventory(p)
		s.pushPetSnapshot(p)
		if !cmd.Commit && s.saver != nil {
			s.saver.Save(s.snapshotOf(p))
		}
	}
	for _, saved := range cmd.Reservation.Players {
		s.runDeferredTradeMutations(saved.ID)
	}
	for _, saved := range cmd.Reservation.Players {
		s.finishTradeLifecycle(saved.ID)
	}
}
