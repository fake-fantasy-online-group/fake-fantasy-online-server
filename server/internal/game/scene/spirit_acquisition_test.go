package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
	"testing"
)

func TestSpiritEssenceRewardFrozenStream(t *testing.T) {
	table, s := wingRiftRulesFixture(t)
	for _, d := range domain.SpiritDefinitions() {
		s.items[d.EssenceID] = domain.ItemDef{ID: d.EssenceID, Stackable: true}
	}
	rules := table.Active()
	rules.Version = 19
	seen := map[domain.ItemID]int{}
	for i := 0; i < 9000; i++ {
		prepareCumulativeRun(s, rules, fmt.Sprintf("spirit-%d", i), 150, false, 60, 60)
		s.rift.data.Run.Phase = domain.RiftCompleted
		a, err := s.riftSpiritEssenceStacks(100)
		if err != nil || len(a) != 1 || a[0].Count < 68 || a[0].Count > 82 {
			t.Fatal(a, err)
		}
		b, _ := s.riftSpiritEssenceStacks(100)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("rerolled")
		}
		seen[a[0].Item]++
	}
	if len(seen) != 9 {
		t.Fatal(seen)
	}
	for _, n := range seen {
		if n < 800 || n > 1200 {
			t.Fatal("biased type distribution", seen)
		}
	}
	s.rift.data.Run.Rules.Version = 18
	if got, err := s.riftSpiritEssenceStacks(100); err != nil || len(got) != 0 {
		t.Fatal("old run changed", got, err)
	}
	s.rift.data.Run.Rules.Version = 19
	s.rift.data.Run.Members[100].Eligible = false
	if got, _ := s.riftSpiritEssenceStacks(100); len(got) != 0 {
		t.Fatal("ineligible rewarded")
	}
	s.rift.data.Run.Members[100].Eligible = true
	delete(s.items, domain.SpiritDefinitions()[0].EssenceID)
	if _, err := s.riftSpiritEssenceStacks(100); err == nil {
		t.Fatal("missing pool accepted")
	}
}
func TestSpiritShopNativePurchase(t *testing.T) {
	for _, d := range domain.SpiritDefinitions() {
		t.Run(d.Name, func(t *testing.T) {
			s, _, _, _ := spiritFixture(t)
			p := s.players[1]
			p.Player.Bag = domain.NewBag(4)
			p.Player.Char.Money = moneyFromInventory(3 * domain.SpiritSealedPrice)
			shop := domain.SpiritShopID(d.Kind)
			s.shops = domain.ShopTable{shop: {ID: shop, Items: []domain.ItemID{d.ItemID}}}
			s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: d.NPCName, Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: d.NPCID, Script: d.NPCScript, Sell: shop}}
			s.items[d.ItemID] = domain.ItemDef{ID: d.ItemID, Price: domain.SpiritSealedPrice, InstanceKind: domain.ItemInstanceSpirit, InventoryTabKnown: true}
			s.onOpenShop(OpenShop{ID: p.ID, Shop: shop})
			if p.Player.ShopID != shop {
				t.Fatal("open")
			}
			s.onBuyItem(BuyItem{ID: p.ID, Item: d.ItemID, Count: 2})
			a, b := p.Player.Bag.At(0), p.Player.Bag.At(1)
			money, _ := inventoryMoney(p.Player.Char.Money)
			if money != domain.SpiritSealedPrice || a.UID == 0 || a.UID == b.UID || a.Spirit.Kind != d.Kind || a.Spirit.AttrCount != 0 || b.Count != 1 {
				t.Fatal("bad purchase", money, a, b)
			}
			s.onBuyItem(BuyItem{ID: p.ID, Item: d.ItemID, Count: 3})
			if p.Player.Bag.CountOf(d.ItemID) != 2 {
				t.Fatal("insufficient money changed inventory")
			}
			p.Pos.X += 10000
			s.onBuyItem(BuyItem{ID: p.ID, Item: d.ItemID, Count: 1})
			if p.Player.Bag.CountOf(d.ItemID) != 2 {
				t.Fatal("remote purchase")
			}
		})
	}
}

func TestSpiritEssenceClaimAtomicRetry(t *testing.T) {
	s, b := completedWingRift(t, 50)
	p := s.players[1]
	for _, d := range domain.SpiritDefinitions() {
		s.items[d.EssenceID] = domain.ItemDef{ID: d.EssenceID, Stackable: true, InventoryTabKnown: true}
	}
	s.rift.data.Run.Rules.Version = 19
	stacks, err := s.riftSpiritEssenceStacks(100)
	if err != nil || len(stacks) != 1 {
		t.Fatal(stacks, err)
	}
	grant := s.rift.data.Grants[100]
	grant.Items = stacks
	s.rift.data.Grants[100] = grant
	d, rev := s.rift.data.clone(), s.rift.revision
	s.rift = nil
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Set(0, domain.Stack{Item: 1900028202, Count: 999})
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "spirit-essence-retry", ExpectedRevision: "0"}
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 0 {
		t.Fatal("full bag consumed reward")
	}
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 1 {
		t.Fatal("missing staged reward")
	}
	b.finish(false)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(stacks[0].Item) != 0 {
		t.Fatal("failed storage published reward")
	}
	s.claimEndgameRift(p, req, d, rev, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Bag.CountOf(stacks[0].Item) != stacks[0].Count {
		t.Fatal("not exact reward")
	}
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 2 || p.Player.Bag.CountOf(stacks[0].Item) != stacks[0].Count {
		t.Fatal("replay duplicated")
	}
}

func TestSpiritShopCapacityCapabilityAndLease(t *testing.T) {
	for _, mode := range []string{"full", "old-client", "busy"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, d := spiritFixture(t)
			p := s.players[1]
			shop := domain.SpiritShopID(d.Kind)
			s.entities[77].NPC.Sell = shop
			s.shops = domain.ShopTable{shop: {ID: shop, Items: []domain.ItemID{d.ItemID}}}
			def := s.items[d.ItemID]
			def.Price = domain.SpiritSealedPrice
			s.items[d.ItemID] = def
			p.Player.Char.Money = moneyFromInventory(2 * domain.SpiritSealedPrice)
			s.onOpenShop(OpenShop{ID: p.ID, Shop: shop})
			switch mode {
			case "full":
				p.Player.Bag = domain.NewBag(1)
				p.Player.Bag.Set(0, domain.NewStack(def, 1))
			case "old-client":
				p.Player.EndgameSpirit = false
			case "busy":
				p.Player.TradeBusy = true
			}
			before := p.Player.Bag.Clone()
			money := p.Player.Char.Money
			s.exec(BuyItem{ID: p.ID, Item: d.ItemID, Count: 1})
			if !reflect.DeepEqual(before, p.Player.Bag) || money != p.Player.Char.Money {
				t.Fatal("rejected shop mutation", mode)
			}
		})
	}
}

// Migration125 sets all nine spirit shops AllowSell=false. Exercise the existing
// authoritative handler rather than adding an NPC-name shortcut to the protocol.
func TestSpiritUIReviewAllShopsBuyOnlyOrdinaryMerchantUnchanged(t *testing.T) {
	for _, d := range domain.SpiritDefinitions() {
		t.Run(d.NPCName, func(t *testing.T) {
			s, _, _, _ := spiritFixture(t)
			p := s.players[1]
			p.Player.Bag = domain.NewBag(4)
			p.Player.Char.Money = moneyFromInventory(2 * domain.SpiritSealedPrice)
			shop := domain.SpiritShopID(d.Kind)
			s.shops = domain.ShopTable{shop: {ID: shop, Items: []domain.ItemID{d.ItemID}, AllowSell: false}}
			s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: d.NPCName, Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: d.NPCID, Script: d.NPCScript, Sell: shop}}
			s.items[d.ItemID] = domain.ItemDef{ID: d.ItemID, Price: domain.SpiritSealedPrice, InstanceKind: domain.ItemInstanceSpirit, InventoryTabKnown: true}
			s.onOpenShop(OpenShop{ID: p.ID, Shop: shop})
			s.onBuyItem(BuyItem{ID: p.ID, Item: d.ItemID, Count: 1})
			money, _ := inventoryMoney(p.Player.Char.Money)
			if p.Player.Bag.CountOf(d.ItemID) != 1 || money != domain.SpiritSealedPrice {
				t.Fatal("10-gold purchase changed", money, p.Player.Bag)
			}
			const junk domain.ItemID = 999001
			s.items[junk] = domain.ItemDef{ID: junk, Stackable: true, SellPrice: 7, InventoryTabKnown: true}
			p.Player.Bag.Set(3, domain.Stack{Item: junk, Count: 2})
			before := p.Player.Bag.Clone()
			s.onSellItem(SellItem{ID: p.ID, Slot: 3, Item: junk, Count: 1, Confirmed: true})
			afterMoney, _ := inventoryMoney(p.Player.Char.Money)
			if !reflect.DeepEqual(before, p.Player.Bag) || afterMoney != money {
				t.Fatal("spirit shop accepted forged sell", afterMoney)
			}
			// Reopening after rejection still permits buying, not a disabled vendor.
			s.onOpenShop(OpenShop{ID: p.ID, Shop: shop})
			s.onBuyItem(BuyItem{ID: p.ID, Item: d.ItemID, Count: 1})
			if p.Player.Bag.CountOf(d.ItemID) != 2 {
				t.Fatal("buy after sell rejection")
			}
			// The same item remains sellable at an ordinary merchant.
			s.shops[700] = domain.Shop{ID: 700, AllowSell: true}
			s.entities[78] = &entity.Entity{ID: 78, Kind: domain.KindNPC, NPC: &entity.NPCInfo{Sell: 700}}
			p.Player.ShopID = 700
			beforeSale, _ := inventoryMoney(p.Player.Char.Money)
			s.onSellItem(SellItem{ID: p.ID, Slot: 3, Item: junk, Count: 1, Confirmed: true})
			afterSale, _ := inventoryMoney(p.Player.Char.Money)
			if p.Player.Bag.CountOf(junk) != 1 || afterSale != beforeSale+7 {
				t.Fatal("ordinary merchant sale changed")
			}
		})
	}
}
