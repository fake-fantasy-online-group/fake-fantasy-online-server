package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// onOpenShop 回应正式客户端 0x1019。ShopID 必须同时存在于 PostgreSQL 配置和
// 当前场景的真实 NPC 上；空库存也必须回完整 0x801c，客户端才能结束等待状态。
func (s *Scene) onOpenShop(cmd OpenShop) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	p.Player.RepairQuote = nil
	reject := func(reason event.RejectReason) {
		p.Player.ShopID = 0
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: cmd.CmdName(), Reason: reason})
	}
	if !p.Alive() || cmd.Shop <= 0 {
		reject(event.RejectInvalid)
		return
	}
	shop, ok := s.shops[cmd.Shop]
	if !ok || !s.hasShopNPC(cmd.Shop) {
		reject(event.RejectNoTarget)
		return
	}
	views := make([]event.ShopItemView, 0, len(shop.Items))
	for _, id := range shop.Items {
		def, ok := s.itemDef(id)
		if !ok || def.Price <= 0 || def.Price > math.MaxInt32 {
			reject(event.RejectNoTarget)
			return
		}
		st := domain.Stack{Item: id, Count: 1}
		if def.Equip != nil {
			st.Durability = def.Equip.Durable
			st.MaxDurability = def.Equip.Durable
		}
		views = append(views, event.ShopItemView{
			Item: id, Price: def.Price, Name: def.Name, Desc: s.itemTooltip(def, st),
			Blocked: s.equipmentBlockedFor(p, def),
		})
	}
	p.Player.ShopID = cmd.Shop
	s.emitTo(p.ID, event.ShopItems{Who: p.ID, Shop: cmd.Shop, Items: views})
	s.log.Debug("打开 NPC 商店", "char", p.Name, "shop", cmd.Shop,
		"库存", len(views), "来源", shop.Source)
}

func (s *Scene) onBuyItem(cmd BuyItem) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	reject := func(reason event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: cmd.CmdName(), Reason: reason})
	}
	shop, ok := s.shops[p.Player.ShopID]
	if !p.Alive() || !ok || !s.hasShopNPC(p.Player.ShopID) {
		p.Player.ShopID = 0
		reject(event.RejectNoTarget)
		return
	}
	if cmd.Count <= 0 || !shop.Contains(cmd.Item) {
		reject(event.RejectInvalid)
		return
	}
	def, ok := s.itemDef(cmd.Item)
	if !ok || def.Price <= 0 {
		reject(event.RejectNoTarget)
		return
	}
	cost, ok := checkedMul64(def.Price, int64(cmd.Count))
	if !ok || cost <= 0 {
		reject(event.RejectInvalid)
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < cost {
		reject(event.RejectNotEnough)
		return
	}
	planned := p.Player.Bag.Clone()
	if planned == nil || planned.Add(def, cmd.Count) != 0 {
		reject(event.RejectBagFull)
		return
	}
	oldBag, oldMoney := p.Player.Bag, p.Player.Char.Money
	p.Player.Bag = planned
	p.Player.Char.Money = moneyFromInventory(money - cost)
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag, p.Player.Char.Money = oldBag, oldMoney
		reject(event.RejectUnknown)
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("购买 NPC 商品", "char", p.Name, "shop", shop.ID,
		"item", cmd.Item, "count", cmd.Count, "cost", cost)
}

func (s *Scene) onSellItem(cmd SellItem) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	reject := func(reason event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: cmd.CmdName(), Reason: reason})
	}
	shop, ok := s.shops[p.Player.ShopID]
	if !p.Alive() || !ok || !shop.AllowSell || !s.hasShopNPC(p.Player.ShopID) {
		p.Player.ShopID = 0
		reject(event.RejectNoTarget)
		return
	}
	if !cmd.Confirmed || cmd.Count <= 0 || cmd.Slot < 0 {
		reject(event.RejectInvalid)
		return
	}
	stack := p.Player.Bag.At(cmd.Slot)
	if stack.Empty() || stack.Item != cmd.Item || stack.Count < cmd.Count {
		reject(event.RejectNoItem)
		return
	}
	def, ok := s.itemDef(stack.Item)
	if !ok || !def.InventoryTabKnown || def.InventoryTab != cmd.Tab || def.SellPrice <= 0 {
		reject(event.RejectInvalid)
		return
	}
	proceeds, ok := checkedMul64(def.SellPrice, int64(cmd.Count))
	if !ok || proceeds <= 0 {
		reject(event.RejectInvalid)
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money > math.MaxInt64-proceeds {
		reject(event.RejectUnknown)
		return
	}
	planned := p.Player.Bag.Clone()
	if planned == nil || !planned.RemoveAt(cmd.Slot, cmd.Count) {
		reject(event.RejectNoItem)
		return
	}
	oldBag, oldMoney := p.Player.Bag, p.Player.Char.Money
	p.Player.Bag = planned
	p.Player.Char.Money = moneyFromInventory(money + proceeds)
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag, p.Player.Char.Money = oldBag, oldMoney
		reject(event.RejectUnknown)
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("出售物品给 NPC", "char", p.Name, "shop", shop.ID,
		"item", cmd.Item, "count", cmd.Count, "proceeds", proceeds)
}

func (s *Scene) hasShopNPC(shop int32) bool {
	if shop <= 0 {
		return false
	}
	for _, e := range s.entities {
		if e.Kind == domain.KindNPC && e.NPC != nil && e.NPC.Sell == shop {
			return true
		}
	}
	return false
}

func moneyFromInventory(total int64) domain.Money {
	if total <= 0 {
		return domain.Money{}
	}
	return domain.Money{
		Gold:   total / 1_000_000,
		Silver: (total % 1_000_000) / 1_000,
		Copper: total % 1_000,
	}
}
