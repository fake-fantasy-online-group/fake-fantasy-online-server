package scene

import (
	"fmt"
	"math"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) rackSnapshot(id domain.EntityID) (event.RackCatalogSnapshot, bool) {
	p := s.players[id]
	if p == nil || p.Player == nil || p.Player.Char == nil || !s.rack.Valid() {
		return event.RackCatalogSnapshot{}, false
	}
	out := event.RackCatalogSnapshot{Who: id, Categories: append([]domain.RackCategory(nil), s.rack.Categories...)}
	for _, good := range s.rack.Goods {
		if good.Pet != 0 {
			def, ok := s.petDefs[good.Pet]
			if !ok || !def.RealPet || def.Capturable() || def.Model <= 0 {
				return event.RackCatalogSnapshot{}, false
			}
			out.Goods = append(out.Goods, event.RackGoodView{Item: good.Item,
				Name: def.Name + "·魂之精", Desc: fmt.Sprintf("%s的魂之精。购买后进入宠物栏，双击孵化并出战。", def.Name),
				Group: good.Category, Price: good.Price, Flags: good.Flags, Remain: good.Remain, Part: good.Part,
				Blocked: good.Remain == 0 || p.Player.Char.Caiyu < good.Price || len(p.Player.Char.Pets) >= domain.MaxPets})
			continue
		}
		def, ok := s.itemDef(good.Item)
		if !ok {
			return event.RackCatalogSnapshot{}, false
		}
		out.Goods = append(out.Goods, event.RackGoodView{Item: good.Item, Name: def.Name,
			Desc: def.Description, Weight: def.Weight, Group: good.Category, Price: good.Price,
			Flags: good.Flags, Remain: good.Remain, Part: good.Part,
			Blocked: good.Remain == 0 || p.Player.Char.Caiyu < good.Price})
	}
	return out, true
}

func (s *Scene) onOpenRack(cmd OpenRack) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	snapshot, ok := s.rackSnapshot(cmd.ID)
	if !ok {
		p.Player.RackOpen = false
		s.emitTo(cmd.ID, event.ServerNotice{Who: cmd.ID, Text: "神奇货架暂不可用"})
		return
	}
	p.Player.RackOpen = true
	s.emitTo(cmd.ID, snapshot)
}

func (s *Scene) rackReply(ch chan<- RackReserveResult, result RackReserveResult) {
	if ch == nil {
		return
	}
	select {
	case ch <- result:
	default:
		s.log.Warn("货架事务保留结果无人接收", "char", result.Reservation.ID)
	}
}

func (s *Scene) onReserveRackPurchase(cmd ReserveRackPurchase) {
	result := RackReserveResult{}
	defer func() { s.rackReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.RackBusy || !p.Alive() || !p.Player.RackOpen || cmd.Count <= 0 {
		result.Reason = "角色或货架状态不可用"
		return
	}
	good, ok := s.rack.Good(cmd.Item)
	def, found := s.itemDef(cmd.Item)
	if !ok || (!found && good.Pet == 0) || good.Price <= 0 || good.Price > math.MaxInt32 || good.Quantity <= 0 || good.Remain == 0 {
		result.Reason = "商城商品当前不可购买"
		return
	}
	totalPrice, priceOK := domain.CheckedMulPositive(int64(cmd.Count), good.Price)
	totalQuantity64, quantityOK := domain.CheckedMulPositive(int64(cmd.Count), int64(good.Quantity))
	if !priceOK || !quantityOK || totalQuantity64 > math.MaxInt32 ||
		(good.Remain > 0 && cmd.Count > good.Remain) {
		result.Reason = "购买数量超过限制"
		return
	}
	if p.Player.Char.Caiyu < totalPrice {
		result.Reason = "彩玉不足"
		return
	}
	if good.Pet != 0 {
		s.reserveRackPetPurchase(p, good, cmd.Count, int32(totalQuantity64), totalPrice, &result)
		return
	}
	bag := p.Player.Bag.Clone()
	if bag == nil || bag.Add(def, int32(totalQuantity64)) != 0 {
		result.Reason = "背包空间不足"
		return
	}
	result.Reservation = RackReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeCaiyu: p.Player.Char.Caiyu}
	p.Player.Bag = bag
	p.Player.Char.Caiyu -= totalPrice
	p.Player.RackBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	result.Item, result.Name = good.Item, def.Name
	result.UnitPrice, result.Bundle, result.Shares = int32(good.Price), good.Quantity, cmd.Count
	result.Total = totalPrice
}

// reserveRackPetPurchase 与普通商品共用彩玉扣款、购买流水和角色快照事务。
// 商品号来自 PostgreSQL 映射，不能根据客户端传入的任意编号凭空发宠。
func (s *Scene) reserveRackPetPurchase(p *entity.Entity, good domain.RackGood, shares, quantity int32,
	total int64, result *RackReserveResult) {
	def, ok := s.petDefs[good.Pet]
	if !ok || !def.RealPet || def.Capturable() || def.Model <= 0 {
		result.Reason = "宠物商品配置无效"
		return
	}
	ch := p.Player.Char
	if !s.petItemRoom(p, int(quantity)) {
		result.Reason = "宠物栏空间不足"
		return
	}
	result.Reservation = RackReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(), BeforeCaiyu: ch.Caiyu}
	for i := int32(0); i < quantity; i++ {
		inst := domain.NewPetInstance(def)
		inst.ID, inst.Slot = s.nextPetInstID(ch), s.nextPetSlot(ch)
		s.appendPet(p, inst)
		result.Reservation.AddedPets = append(result.Reservation.AddedPets, inst.ID)
	}
	ch.Caiyu -= total
	p.Player.RackBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	result.Item, result.Name = good.Item, def.Name+"·魂之精"
	result.UnitPrice, result.Bundle, result.Shares = int32(good.Price), good.Quantity, shares
	result.Total = total
}

func (s *Scene) onReserveRackRefund(cmd ReserveRackRefund) {
	result := RackReserveResult{}
	defer func() { s.rackReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.RackBusy || !p.Alive() || cmd.Shares <= 0 || cmd.Credit <= 0 {
		result.Reason = "角色或退货状态不可用"
		return
	}
	def, ok := s.itemDef(cmd.Offer.Item)
	quantity, quantityOK := domain.CheckedMulPositive(int64(cmd.Offer.Bundle), int64(cmd.Shares))
	if !ok || cmd.Offer.UnitPrice <= 0 || cmd.Offer.Bundle <= 0 ||
		cmd.Offer.RemainingShares < cmd.Shares || !quantityOK || quantity > math.MaxInt32 ||
		p.Player.Char.Caiyu > math.MaxInt64-cmd.Credit {
		result.Reason = "退货参数或购买流水无效"
		return
	}
	bag := p.Player.Bag.Clone()
	if bag == nil || !bag.Remove(cmd.Offer.Item, int32(quantity)) {
		result.Reason = "背包中没有足够的可退商品"
		return
	}
	result.Reservation = RackReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeCaiyu: p.Player.Char.Caiyu}
	p.Player.Bag = bag
	p.Player.Char.Caiyu += cmd.Credit
	p.Player.RackBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	result.Item, result.Name = cmd.Offer.Item, def.Name
	result.UnitPrice, result.Bundle, result.Shares = cmd.Offer.UnitPrice, cmd.Offer.Bundle, cmd.Shares
	result.Total = cmd.Credit
}

func (s *Scene) onFinalizeRackReservation(cmd FinalizeRackReservation) {
	if cmd.Done != nil {
		defer close(cmd.Done)
	}
	p := s.players[cmd.Reservation.ID]
	if p == nil || p.Player == nil {
		return
	}
	if !cmd.Commit {
		p.Player.Bag = cmd.Reservation.BeforeBag
		p.Player.Char.Caiyu = cmd.Reservation.BeforeCaiyu
		for _, id := range cmd.Reservation.AddedPets {
			s.removePet(p, id)
		}
	}
	p.Player.RackBusy = false
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	if len(cmd.Reservation.AddedPets) > 0 {
		s.pushPetSnapshot(p)
	}
	if cmd.RefreshCatalog {
		if snapshot, ok := s.rackSnapshot(p.ID); ok {
			s.emitTo(p.ID, snapshot)
		}
	}
	if cmd.Message != "" {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: cmd.Message})
	}
}

func (s *Scene) onShowRackRefunds(cmd ShowRackRefunds) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil || !cmd.Rule.Valid() {
		return
	}
	out := event.RackRefundSnapshot{Who: p.ID, Percent: cmd.Rule.Percent,
		WindowDays: cmd.Rule.WindowDay, UsedOK: cmd.Rule.UsedOK}
	categoryNames := make([]string, 0, len(s.rack.Categories))
	for _, category := range s.rack.Categories {
		categoryNames = append(categoryNames, category.Name)
	}
	out.Categories = strings.Join(categoryNames, "、")
	for _, offer := range cmd.Offers {
		def, ok := s.itemDef(offer.Item)
		if !ok || offer.Bundle <= 0 || offer.UnitPrice <= 0 || offer.RemainingShares <= 0 {
			continue
		}
		have := p.Player.Bag.UsableCountOf(offer.Item)
		shares := have / offer.Bundle
		if shares > offer.RemainingShares {
			shares = offer.RemainingShares
		}
		gain := int64(offer.UnitPrice) * int64(cmd.Rule.Percent) / 100
		if shares <= 0 || gain <= 0 || gain > math.MaxInt32 {
			continue
		}
		out.Rows = append(out.Rows, event.RackRefundView{Item: offer.Item, Name: def.Name,
			UnitPrice: offer.UnitPrice, Bundle: offer.Bundle, Day: 0, Shares: shares,
			Have: have, Gain: int32(gain), LeftDays: offer.LeftDays})
	}
	s.emitTo(p.ID, out)
}

func (s *Scene) onReserveDeposit(cmd ReserveDeposit) {
	result := FamilyReserveResult{}
	defer func() { s.familyReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.FamilyBusy ||
		!s.depositRule.Valid() || !s.depositRule.AutoCredit || cmd.Amount <= 0 ||
		cmd.Amount > s.depositRule.MaxAmount {
		result.Reason = "充值金额或角色状态无效"
		return
	}
	if int64(cmd.Amount) > math.MaxInt64/s.depositRule.CaiyuPerUnit {
		result.Reason = "充值金额溢出"
		return
	}
	credited := int64(cmd.Amount) * s.depositRule.CaiyuPerUnit
	if p.Player.Char.Caiyu > math.MaxInt64-credited {
		result.Reason = "彩玉余额已达上限"
		return
	}
	result.Reservation = FamilyReservation{ID: p.ID, BeforeMoney: p.Player.Char.Money,
		BeforeCaiyu: p.Player.Char.Caiyu}
	p.Player.Char.Caiyu += credited
	p.Player.FamilyBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	result.Caiyu, result.Credited = p.Player.Char.Caiyu, credited
}
