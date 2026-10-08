package scene

import (
	"fmt"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) stallReply(ch chan<- StallReserveResult, result StallReserveResult) {
	if ch == nil {
		return
	}
	select {
	case ch <- result:
	default:
		s.log.Warn("摆摊保留结果无人接收", "owner", result.Reservation.Owner)
	}
}

func (s *Scene) stallNotice(p *entity.Entity, text string) {
	if p != nil && text != "" {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	}
}

func (s *Scene) onCreateStall(cmd CreateStall) {
	result := StallReserveResult{}
	defer func() { s.stallReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		result.Reason = "角色背包状态无效"
		return
	}
	name := strings.TrimSpace(cmd.Name)
	if !cmd.Type.Valid() || name == "" {
		result.Reason = "摊位类型或名称无效"
		return
	}
	if p.Player.Stall != nil || p.Player.StallBusy {
		result.Reason = "当前已经在摆摊"
		return
	}
	if !p.Alive() || p.Player.Work != nil || p.Player.Riding {
		result.Reason = "死亡、打工或骑乘状态不能摆摊"
		return
	}
	coin, ok := s.stallOpenItems.ItemFor(cmd.Type)
	if !ok {
		result.Reason = "开店古币配置不可用"
		return
	}
	coinDef, ok := s.itemDef(coin)
	if !ok || p.Player.Bag.UsableCountOf(coin) < 1 {
		result.Reason = fmt.Sprintf("需要%s×1", coinDef.Name)
		if !ok || coinDef.Name == "" {
			result.Reason = fmt.Sprintf("需要开店古币%d×1", coin)
		}
		return
	}
	before := StallPlayerReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeMoney: p.Player.Char.Money, BeforeStall: nil}
	bag := p.Player.Bag.Clone()
	if bag == nil || !bag.Remove(coin, 1) {
		result.Reason = "开店古币状态已经变化"
		return
	}
	p.Player.StopResting()
	p.Player.StallMoveWarned = false
	p.Player.Bag, p.Player.Stall, p.Player.StallBusy = bag, domain.NewStall(cmd.Type, name), true
	result.Reservation = StallReservation{
		Players: []StallPlayerReservation{before}, Owner: p.ID, Create: true,
	}
	result.Snapshots = []domain.Snapshot{s.snapshotOf(p)}
	result.ActorMessage = fmt.Sprintf("已消耗%s×1，摊位已开启", coinDef.Name)
}

func (s *Scene) onReserveStallAdd(cmd ReserveStallAdd) {
	result := StallReserveResult{}
	defer func() { s.stallReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.Stall == nil || p.Player.StallBusy {
		result.Reason = "摊位当前不可修改"
		return
	}
	stack := p.Player.Bag.At(int(cmd.Slot))
	if stack.InstanceKind == domain.ItemInstancePet {
		if p.Player.Stall.Type != domain.StallSell || !s.petTradeAllowed(p, stack) {
			result.Reason = "该宠物当前不能出售"
			return
		}
		// Without a listing-version field in the native request, do not reuse a
		// pet listing while another client's older item/price quote may exist.
		for viewer := range s.stallBrowsers[p.ID] {
			if viewer != p.ID {
				result.Reason = "已有买家浏览，请收摊重新开店后再上架宠物"
				return
			}
		}
	}
	def, ok := s.itemDef(stack.Item)
	if !ok || stack.Empty() || stack.Item != cmd.Item || cmd.Count <= 0 ||
		(p.Player.Stall.Type == domain.StallSell && cmd.Count > stack.Count) ||
		cmd.Price <= 0 || stack.Bound || stack.Locked || !def.CanTrade ||
		!def.InventoryTabKnown || def.InventoryTab != cmd.Tab {
		result.Reason = "该物品不能上架"
		return
	}
	if _, ok := domain.CheckedMulPositive(int64(cmd.Count), cmd.Price); !ok {
		result.Reason = "价格或数量过大"
		return
	}
	before := StallPlayerReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeMoney: p.Player.Char.Money, BeforeStall: p.Player.Stall.Clone()}
	bag, stall := p.Player.Bag.Clone(), p.Player.Stall.Clone()
	listed := stack
	listed.Count = cmd.Count
	if stall.Type == domain.StallSell {
		if !bag.RemoveAt(int(cmd.Slot), cmd.Count) {
			result.Reason = "物品状态已经变化"
			return
		}
	}
	if !stall.Add(domain.StallItem{Stack: listed, UnitPrice: cmd.Price}) {
		p.Player.Char.Money = before.BeforeMoney
		result.Reason = "同一种物品不能重复上架"
		return
	}
	p.Player.Bag, p.Player.Stall, p.Player.StallBusy = bag, stall, true
	result.Reservation = StallReservation{Players: []StallPlayerReservation{before}, Owner: p.ID}
	result.Snapshots = []domain.Snapshot{s.snapshotOf(p)}
	result.ActorMessage = "物品已上架"
}

func (s *Scene) onReserveStallDel(cmd ReserveStallDel) {
	result := StallReserveResult{}
	defer func() { s.stallReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Stall == nil || p.Player.StallBusy {
		result.Reason = "摊位当前不可修改"
		return
	}
	before := StallPlayerReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeMoney: p.Player.Char.Money, BeforeStall: p.Player.Stall.Clone()}
	bag, stall := p.Player.Bag.Clone(), p.Player.Stall.Clone()
	item, ok := stall.Remove(int(cmd.Index))
	if !ok {
		result.Reason = "上架索引无效"
		return
	}
	if stall.Type == domain.StallSell {
		def, exists := s.itemDef(item.Stack.Item)
		if !exists || bag.AddStack(def, item.Stack) != 0 {
			result.Reason = "背包空间不足，不能下架"
			return
		}
	}
	p.Player.Bag, p.Player.Stall, p.Player.StallBusy = bag, stall, true
	result.Reservation = StallReservation{Players: []StallPlayerReservation{before}, Owner: p.ID}
	result.Snapshots = []domain.Snapshot{s.snapshotOf(p)}
	result.ActorMessage = "物品已下架"
}

func (s *Scene) onReserveStallEnd(cmd ReserveStallEnd) {
	result := StallReserveResult{}
	defer func() { s.stallReply(cmd.Reply, result) }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || p.Player.StallBusy {
		result.Reason = "当前没有可结束的摊位"
		return
	}
	before := StallPlayerReservation{ID: p.ID, BeforeBag: p.Player.Bag.Clone(),
		BeforeMoney: p.Player.Char.Money, BeforeStall: p.Player.Stall.Clone()}
	// 收摊必须是幂等操作。游戏服重启、崩溃恢复或旧回包丢失后，客户端可能
	// 仍保留本地摆摊状态，而服务端权威状态已经没有摊位。此时仍提交一份
	// stall=nil 的角色快照并发送 0x803e，客户端才能解除移动/操作锁定。
	if p.Player.Stall == nil {
		result.Reservation = StallReservation{
			Players: []StallPlayerReservation{before}, Owner: p.ID, Close: true,
		}
		result.Snapshots = []domain.Snapshot{s.snapshotOf(p)}
		result.ActorMessage = "摆摊状态已结束"
		return
	}
	bag := p.Player.Bag.Clone()
	if reason := s.returnStallAssets(p.Player.Stall, bag); reason != "" {
		result.Reason = reason
		return
	}
	p.Player.Bag, p.Player.Stall, p.Player.StallBusy = bag, nil, true
	result.Reservation = StallReservation{Players: []StallPlayerReservation{before}, Owner: p.ID, Close: true}
	result.Snapshots = []domain.Snapshot{s.snapshotOf(p)}
	result.ActorMessage = "摊位已结束"
}

func (s *Scene) onBrowseStall(cmd BrowseStall) {
	viewer, owner := s.players[cmd.ID], s.players[cmd.Owner]
	if viewer == nil || owner == nil || owner.Player == nil || owner.Player.Stall == nil ||
		owner.Player.StallBusy {
		if viewer != nil && viewer.ID != cmd.Owner {
			s.stallNotice(viewer, "浏览摊位失败：摊位不存在或正在更新")
		}
		return
	}
	// 0x803c 对摊主本人会路由到 GroceryStatusDlg/StallSalesBox；禁止
	// owner==viewer 会导致窗口一旦关闭就永远无法重新打开。
	if viewer.ID != owner.ID {
		if s.stallBrowsers[owner.ID] == nil {
			s.stallBrowsers[owner.ID] = map[domain.EntityID]struct{}{}
		}
		s.stallBrowsers[owner.ID][viewer.ID] = struct{}{}
	}
	s.emitTo(viewer.ID, s.stallContents(owner, viewer))
}

func (s *Scene) onReserveStallDeal(cmd ReserveStallDeal) {
	result := StallReserveResult{}
	defer func() { s.stallReply(cmd.Reply, result) }()
	actor, owner := s.players[cmd.ID], s.players[cmd.Owner]
	if actor == nil || owner == nil || actor.ID == owner.ID || actor.Player == nil ||
		owner.Player == nil || actor.Player.Bag == nil || owner.Player.Bag == nil ||
		actor.Player.Stall != nil || actor.Player.StallBusy || owner.Player.Stall == nil ||
		owner.Player.StallBusy || cmd.Count <= 0 {
		result.Reason = "摊位成交条件无效"
		return
	}
	listing, ok := owner.Player.Stall.At(int(cmd.Index))
	if !ok || cmd.Count > listing.Stack.Count {
		result.Reason = "商品数量不足"
		return
	}
	if cmd.ExpectedItem != listing.Stack.Item || cmd.ExpectedPrice != listing.UnitPrice {
		result.Reason = "摊位商品或价格已经变化"
		return
	}
	def, ok := s.itemDef(listing.Stack.Item)
	if !ok || !def.CanTrade || !def.InventoryTabKnown {
		result.Reason = "商品当前不可交易"
		return
	}
	if listing.Stack.InstanceKind == domain.ItemInstancePet && (owner.Player.Stall.Type != domain.StallSell || !s.petTradeAllowed(owner, listing.Stack)) {
		result.Reason = "宠物状态已经变化"
		return
	}
	total, ok := domain.CheckedMulPositive(int64(cmd.Count), listing.UnitPrice)
	if !ok {
		result.Reason = "成交金额过大"
		return
	}
	actorBefore := StallPlayerReservation{ID: actor.ID, BeforeBag: actor.Player.Bag.Clone(),
		BeforeMoney: actor.Player.Char.Money, BeforeStall: actor.Player.Stall.Clone()}
	ownerBefore := StallPlayerReservation{ID: owner.ID, BeforeBag: owner.Player.Bag.Clone(),
		BeforeMoney: owner.Player.Char.Money, BeforeStall: owner.Player.Stall.Clone()}
	actorBag, ownerBag, ownerStall := actor.Player.Bag.Clone(), owner.Player.Bag.Clone(), owner.Player.Stall.Clone()
	actorMoney, ownerMoney := actor.Player.Char.Money, owner.Player.Char.Money
	if ownerStall.Type == domain.StallSell {
		money, err := inventoryMoney(actorMoney)
		if err != nil || money < total {
			result.Reason = "随身金钱不足"
			return
		}
		transfer := listing.Stack
		transfer.Count = cmd.Count
		if actorBag.AddStack(def, transfer) != 0 {
			result.Reason = "背包空间不足"
			return
		}
		ownerTotal, err := inventoryMoney(ownerMoney)
		nextOwner, addOK := checkedAdd64(ownerTotal, total)
		if err != nil || !addOK {
			result.Reason = "摊主金钱已达上限"
			return
		}
		actorMoney, ownerMoney = moneyFromInventory(money-total), moneyFromInventory(nextOwner)
	} else {
		ownerTotal, ownerMoneyErr := inventoryMoney(ownerMoney)
		if ownerMoneyErr != nil || ownerTotal < total {
			result.Reason = "摊主余额不足，暂时无法收购"
			return
		}
		if cmd.Slot < 0 {
			result.Reason = "出售物品位置无效"
			return
		}
		source := actorBag.At(int(cmd.Slot))
		if source.Empty() || source.Item != listing.Stack.Item || source.Count < cmd.Count ||
			source.Bound || source.Locked || cmd.Tab != def.InventoryTab {
			result.Reason = "出售物品状态无效"
			return
		}
		transfer := source
		transfer.Count = cmd.Count
		if ownerBag.AddStack(def, transfer) != 0 || !actorBag.RemoveAt(int(cmd.Slot), cmd.Count) {
			result.Reason = "摊主背包空间不足"
			return
		}
		money, err := inventoryMoney(actorMoney)
		next, addOK := checkedAdd64(money, total)
		if err != nil || !addOK {
			result.Reason = "随身金钱已达上限"
			return
		}
		actorMoney = moneyFromInventory(next)
		ownerMoney = moneyFromInventory(ownerTotal - total)
	}
	remaining := listing.Stack.Count - cmd.Count
	if !ownerStall.SetCount(int(cmd.Index), remaining) {
		result.Reason = "摊位商品状态已经变化"
		return
	}
	ownerStall.AddSale(domain.StallSale{Time: time.Now(), Item: def.Name, Count: cmd.Count, Money: total})
	if listing.Stack.InstanceKind == domain.ItemInstancePet {
		aPets, oPets, reason := s.transferPetLists(actor, owner, nil, []domain.Stack{listing.Stack})
		if reason != "" {
			result.Reason = reason
			return
		}
		actorBefore.PetsKnown, ownerBefore.PetsKnown = true, true
		actorBefore.IncomingPetUID = listing.Stack.UID
		actorBefore.BeforePets = domain.ClonePets(actor.Player.Char.Pets)
		ownerBefore.BeforePets = domain.ClonePets(owner.Player.Char.Pets)
		s.assignPetList(actor, aPets)
		s.assignPetList(owner, oPets)
	}
	actor.Player.Bag, actor.Player.Char.Money, actor.Player.StallBusy = actorBag, actorMoney, true
	owner.Player.Bag, owner.Player.Char.Money = ownerBag, ownerMoney
	owner.Player.Stall, owner.Player.StallBusy = ownerStall, true
	result.Reservation = StallReservation{Players: []StallPlayerReservation{actorBefore, ownerBefore}, Owner: owner.ID}
	result.Snapshots = []domain.Snapshot{s.snapshotOf(actor), s.snapshotOf(owner)}
	if ownerStall.Type == domain.StallSell {
		result.ActorMessage = fmt.Sprintf("购买成功：%s×%d，花费 %d 铜币", def.Name, cmd.Count, total)
		result.OwnerMessage = fmt.Sprintf("售出 %s×%d，获得 %d 铜币", def.Name, cmd.Count, total)
	} else {
		result.ActorMessage = fmt.Sprintf("出售成功：%s×%d，获得 %d 铜币", def.Name, cmd.Count, total)
		result.OwnerMessage = fmt.Sprintf("收购 %s×%d，支付 %d 铜币", def.Name, cmd.Count, total)
	}
}

func (s *Scene) onFinalizeStallReservation(cmd FinalizeStallReservation) {
	if cmd.Done != nil {
		defer close(cmd.Done)
	}
	for _, saved := range cmd.Reservation.Players {
		p := s.players[saved.ID]
		if p == nil || p.Player == nil {
			continue
		}
		if !cmd.Commit {
			p.Player.Bag = saved.BeforeBag
			p.Player.Char.Money = saved.BeforeMoney
			p.Player.Stall = saved.BeforeStall
			if saved.PetsKnown {
				s.restorePetMembership(p, saved.BeforePets, saved.IncomingPetUID)
			}
		}
		p.Player.StallBusy = false
		if p.Player.Stall == nil {
			p.Player.StallMoveWarned = false
		}
		p.Player.MarkDirty()
		_ = s.pushInventory(p)
		if saved.PetsKnown {
			s.pushPetSnapshot(p)
		}
	}
	owner := s.players[cmd.Reservation.Owner]
	if cmd.Commit {
		if len(cmd.Reservation.Players) > 0 && cmd.ActorMessage != "" {
			s.stallNotice(s.players[cmd.Reservation.Players[0].ID], cmd.ActorMessage)
		}
		if cmd.OwnerMessage != "" && owner != nil &&
			(len(cmd.Reservation.Players) == 0 || owner.ID != cmd.Reservation.Players[0].ID) {
			s.stallNotice(owner, cmd.OwnerMessage)
		}
	}
	if owner != nil && owner.Player.Stall != nil {
		s.emit(event.StallOwnerChanged{Owner: owner.ID, Type: owner.Player.Stall.Type,
			Name: owner.Player.Stall.Name})
	}
	if cmd.Reservation.Close && cmd.Commit {
		s.closeStallView(cmd.Reservation.Owner)
	} else if owner != nil && owner.Player.Stall != nil {
		s.pushStallContents(owner)
	}
	if !cmd.Commit && s.saver != nil {
		for _, saved := range cmd.Reservation.Players {
			if p := s.players[saved.ID]; p != nil {
				s.saver.Save(s.snapshotOf(p))
			}
		}
	}
}

func (s *Scene) stallContents(owner, viewer *entity.Entity) event.StallContents {
	stall := owner.Player.Stall
	out := event.StallContents{Owner: owner.ID, OwnerName: owner.Name, Type: stall.Type, Name: stall.Name}
	for _, item := range stall.Items {
		def, ok := s.itemDef(item.Stack.Item)
		if !ok {
			continue
		}
		blocked := false
		if viewer != nil && viewer.ID != owner.ID {
			blocked = s.stallRowBlocked(owner, viewer, item, def)
		}
		out.Rows = append(out.Rows, event.StallRowView{Item: item.Stack.Item,
			Count: item.Stack.Count, Price: item.UnitPrice, Name: s.ownedItemName(owner, def, item.Stack),
			Desc: s.itemTooltip(def, item.Stack), Blocked: blocked, Quality: equipmentQuality(def, item.Stack), Pet: s.petItemInfo(owner, item.Stack)})
	}
	for _, sale := range stall.Sales {
		out.Sales = append(out.Sales, event.StallSaleView{Time: sale.Time.Unix(), Item: sale.Item,
			Count: sale.Count, Money: sale.Money})
	}
	return out
}

func (s *Scene) stallRowBlocked(owner, viewer *entity.Entity, item domain.StallItem, def domain.ItemDef) bool {
	if viewer == nil || viewer.Player == nil || viewer.Player.Bag == nil || viewer.Player.Stall != nil {
		return true
	}
	if owner.Player.Stall.Type == domain.StallSell {
		total, ok := domain.CheckedMulPositive(1, item.UnitPrice)
		money, err := inventoryMoney(viewer.Player.Char.Money)
		trial := viewer.Player.Bag.Clone()
		one := item.Stack
		one.Count = 1
		return !ok || err != nil || money < total || trial == nil || trial.AddStack(def, one) != 0 || (one.InstanceKind == domain.ItemInstancePet && (len(viewer.Player.Char.Pets) >= domain.MaxPets || !s.petTradeAllowed(owner, one)))
	}
	var available int32
	viewer.Player.Bag.Each(func(_ int, stack domain.Stack) {
		if stack.Item == item.Stack.Item && !stack.Bound && !stack.Locked {
			available += stack.Count
		}
	})
	return available <= 0
}

func (s *Scene) pushStallContents(owner *entity.Entity) {
	if owner == nil || owner.Player == nil || owner.Player.Stall == nil || owner.Player.StallBusy {
		return
	}
	s.emitTo(owner.ID, s.stallContents(owner, owner))
	for viewerID := range s.stallBrowsers[owner.ID] {
		viewer := s.players[viewerID]
		if viewer == nil {
			delete(s.stallBrowsers[owner.ID], viewerID)
			continue
		}
		s.emitTo(viewer.ID, s.stallContents(owner, viewer))
	}
}

func (s *Scene) closeStallView(owner domain.EntityID) {
	delete(s.stallBrowsers, owner)
	s.emit(event.StallClosed{Owner: owner})
}

func (s *Scene) returnStallAssets(stall *domain.Stall, bag *domain.Bag) string {
	if stall == nil || bag == nil {
		return "摊位托管状态无效"
	}
	if stall.Type == domain.StallSell {
		for _, item := range stall.Items {
			def, ok := s.itemDef(item.Stack.Item)
			if !ok || bag.AddStack(def, item.Stack) != 0 {
				return "背包空间不足，不能结束摊位"
			}
		}
		return ""
	}
	return ""
}

// closeStallForExit 尽最大努力归还托管物权。归还失败时保留 Stall 进入最终快照，
// 下次登录会在角色进入场景前继续恢复，不能为了“成功收摊”吞掉货物。
func (s *Scene) closeStallForExit(p *entity.Entity) {
	if p == nil || p.Player == nil || p.Player.Stall == nil {
		return
	}
	bag := p.Player.Bag.Clone()
	if reason := s.returnStallAssets(p.Player.Stall, bag); reason != "" {
		s.log.Error("离场归还摊位托管失败，保留恢复记录", "char", p.Name, "reason", reason)
		return
	}
	p.Player.Bag, p.Player.Stall = bag, nil
	p.Player.StallBusy = false
	p.Player.MarkDirty()
	s.closeStallView(p.ID)
}

func (s *Scene) sendStallSigns(to domain.EntityID) {
	for _, owner := range s.players {
		if owner.Player != nil && owner.Player.Stall != nil && !owner.Player.StallBusy {
			s.emitTo(to, event.StallOwnerChanged{Owner: owner.ID, Type: owner.Player.Stall.Type,
				Name: owner.Player.Stall.Name})
		}
	}
}
