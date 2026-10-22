package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Native prices are shared with the data-layer fixture checked against init-3.
// These are ordinary NPC copper purchases, not the premium rack shop.
func TestPetSuppliesOpenAndBuy(t *testing.T) {
	raw, err := os.ReadFile("../../data/testdata/pet-shop-items.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []domain.ItemDef
	if err = json.Unmarshal(raw, &defs); err != nil {
		t.Fatal(err)
	}
	for _, shopID := range []int32{1003, 1303, 1403, 1503, 1903, 2102, 2602, 3003, 1101, 1201} {
		t.Run(fmt.Sprint(shopID), func(t *testing.T) {
			s, sink := equipScene(t)
			p := s.players[1]
			p.Player.Bag = domain.NewBag(24)
			p.Player.Char.Money = moneyFromInventory(1000000)
			shop := domain.Shop{ID: shopID}
			for _, def := range defs {
				def.InventoryTab = 1
				def.InventoryTabKnown = true
				def.Stackable = true
				s.items[def.ID] = def
				shop.Items = append(shop.Items, def.ID)
			}
			s.shops = domain.ShopTable{shopID: shop}
			s.entities[300] = &entity.Entity{ID: 300, Kind: domain.KindNPC, NPC: &entity.NPCInfo{Sell: shopID}}
			s.onOpenShop(OpenShop{ID: p.ID, Shop: shopID})
			s.flush()
			view, ok := firstOf[event.ShopItems](sink.take())
			if !ok || len(view.Items) != 12 {
				t.Fatal("pet supply shop failed to open", view)
			}
			remaining := int64(1000000)
			for i, def := range defs {
				if view.Items[i].Item != def.ID || view.Items[i].Price != def.Price {
					t.Fatal("shop changed native price", view.Items[i])
				}
				s.onBuyItem(BuyItem{ID: p.ID, Item: def.ID, Count: 2})
				s.flush()
				if rejected, ok := firstOf[event.Rejected](sink.take()); ok {
					t.Fatal("purchase rejected", rejected)
				}
				remaining -= 2 * def.Price
				money, err := inventoryMoney(p.Player.Char.Money)
				if err != nil || money != remaining || p.Player.Bag.CountOf(def.ID) != 2 {
					t.Fatalf("item%d purchase/money mismatch %d vs %d", def.ID, money, remaining)
				}
			}
			before, _ := inventoryMoney(p.Player.Char.Money)
			s.onBuyItem(BuyItem{ID: p.ID, Item: 3032, Count: 1})
			s.flush()
			if _, ok := firstOf[event.Rejected](sink.take()); !ok {
				t.Fatal("nonstock purchase accepted")
			}
			after, _ := inventoryMoney(p.Player.Char.Money)
			if before != after {
				t.Fatal("rejected purchase charged money")
			}
		})
	}
}
