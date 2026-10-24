package scene

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func tieredLegendarySceneFixture(t *testing.T, draw int) (*Scene, domain.ItemDef, domain.Stack) {
	t.Helper()
	roller, def := legendaryDropFixture(t)
	def.Name = "青锋剑"
	def.CanTrade = true
	def.InventoryTab = 2
	def.InventoryTabKnown = true
	def.Equip.Base = domain.Base{STR: 2}
	def.Equip.Bonus = domain.Stats{MinAtk: 20, MaxAtk: 30}
	def.Equip.Durable = 100
	st := domain.NewStack(def, 1)
	calls := 0
	if !roller.equipmentRoll.ApplyLegendary(&st, def, func(n int) int {
		calls++
		if calls == 14 {
			return draw
		}
		return 0
	}) {
		t.Fatal("tiered fixture failed")
	}
	s, _ := equipScene(t)
	s.alloc = domain.NewEntityAlloc()
	s.items[def.ID] = def
	p := s.players[1]
	p.Player.Char.Level = 100
	p.Player.Bag = domain.NewBag(400)
	return s, def, st
}

func TestLegendaryTierNativeViewsAndGroundKeepDarkGold(t *testing.T) {
	for _, draw := range []int{0, 1, 11} {
		t.Run(fmt.Sprint(draw), func(t *testing.T) {
			s, def, st := tieredLegendarySceneFixture(t, draw)
			p := s.players[1]
			p.Player.Bag.Set(200, st)
			p.Player.Worn.Set(domain.SlotWeapon, st)
			snap, err := s.inventorySnapshot(p)
			if err != nil || len(snap.Items) != 1 || len(snap.Equipped) != 1 {
				t.Fatal(err, snap)
			}
			item := snap.Items[0]
			before := st
			wantPrefix := ""
			multiplier := 20
			if draw == 0 {
				wantPrefix = "秘*"
				multiplier = 60
			} else if draw == 1 {
				wantPrefix = "真*"
				multiplier = 40
			}
			if !strings.HasPrefix(item.Name, wantPrefix) || strings.Contains(item.Name, "**") || item.Quality != 5 || snap.Equipped[0].Quality != 5 {
				t.Fatal("name/legacy palette", item)
			}
			tip := s.ownedItemTooltip(def, st, p)
			if !strings.Contains(tip, fmt.Sprintf("\x030攻击 %d~%d", multiplier, multiplier*3/2)) || strings.Contains(tip, "\x037") {
				t.Fatal("base value/unchanged palette", tip)
			}
			for _, a := range st.Legendary.Affixes {
				if !strings.Contains(tip, "\x035"+affixText(a.Affix)) {
					t.Fatal("missing six darkgold affixes")
				}
			}
			power, _ := domain.LegendaryPower(st.Legendary.PowerID)
			if strings.Count(tip, "\x06"+power.Name+"："+power.Effect) != 1 {
				t.Fatal("power text changed")
			}
			view := s.endgameItem(st, "bag", 0)
			if view.Name != item.Name || view.Legendary.Tier != st.Legendary.Tier || len(view.Legendary.Affixes) != 6 {
				t.Fatal("sidecar divergence", view)
			}
			wire := protocol.EncodeEndgame(domain.EndgameResponse{Kind: "snapshot", OK: true, Items: []domain.EndgameItemView{view}})
			var decoded domain.EndgameResponse
			if err = json.Unmarshal(wire[6:], &decoded); err != nil || !reflect.DeepEqual(decoded.Items[0], view) {
				t.Fatal("sidecar serialization", err)
			}
			s.spawnGeneratedLoot(def, st, p.Pos, p.ID)
			if len(s.ground) != 1 {
				t.Fatal("expected actual ground entity")
			}
			for _, drop := range s.ground {
				ev := s.spawnEvent(drop)
				requireGroundPresentationWire(t, p.ID, ev, 5, int32(def.ID), false, drop.Drop.PresentationToken)
				if ev.Name != item.Name || drop.Drop.Stack != before {
					t.Fatal("ground/name/state mismatch")
				}
			}
			p.Player.Stall = &domain.Stall{Items: []domain.StallItem{{Stack: st, UnitPrice: 100}}}
			stall := s.stallContents(p, p)
			if len(stall.Rows) != 1 || stall.Rows[0].Name != item.Name || stall.Rows[0].Quality != 5 || stall.Rows[0].Desc != tip {
				t.Fatal("stall mismatch")
			}
			if st != before {
				t.Fatal("read changed instance")
			}
		})
	}
}

func TestLegendaryTierRealTradeEquipAndDropPreserveInstance(t *testing.T) {
	for _, draw := range []int{0, 1, 11} {
		t.Run(fmt.Sprint(draw), func(t *testing.T) {
			s, def, st := tieredLegendarySceneFixture(t, draw)
			join(t, s, 2, 200, "乙", 150, 150)
			s.players[2].Player.Char.Level = 100
			s.players[1].Player.Bag.Set(200, st)
			reply := make(chan TradeSettlementResult, 1)
			s.exec(ReserveTradeSettlement{A: 1, B: 2, AOffer: TradeSettlementOffer{Items: []TradeOfferItem{{Tab: 2, Slot: 200, Stack: st}}}, Reply: reply})
			result := <-reply
			if result.Reason != "" || len(result.Snapshots) != 2 {
				t.Fatal("trade reserve", result.Reason)
			}
			result.Reservation.Result <- true
			s.step()
			p := s.players[2]
			slot := -1
			p.Player.Bag.Each(func(i int, got domain.Stack) {
				if got.UID == st.UID {
					slot = i
					if got != st {
						t.Fatal("trade mutated tier")
					}
				}
			})
			if slot < 0 {
				t.Fatal("trade lost item")
			}

			// Drop and pick up while still unbound, before native equip binds it.
			s.exec(DropBagItem{ID: 2, Slot: slot, Tab: 2, Item: def.ID, At: p.Pos})
			var dropID domain.EntityID
			for id, drop := range s.ground {
				if drop.Drop.Stack.UID == st.UID {
					dropID = id
					if drop.Drop.Stack != st {
						t.Fatal("drop mutated tier")
					}
				}
			}
			if dropID == 0 {
				t.Fatal("drop lost tier")
			}
			s.exec(PickUp{ID: 2, Target: dropID})
			slot = -1
			p.Player.Bag.Each(func(i int, got domain.Stack) {
				if got.UID == st.UID {
					slot = i
					if got != st {
						t.Fatal("pickup mutated tier")
					}
				}
			})
			if slot < 0 {
				t.Fatal("pickup lost tier")
			}
			s.exec(Equip{ID: 2, BagSlot: slot})
			expected := st
			expected.Bound = true // Existing native bind-on-equip rule.
			if p.Player.Worn.At(domain.SlotWeapon) != expected {
				t.Fatal("equip mutated tier")
			}
			s.exec(Unequip{ID: 2, Slot: domain.SlotWeapon})
			found := false
			p.Player.Bag.Each(func(i int, got domain.Stack) {
				if got.UID == st.UID {
					found = true
					if got != expected {
						t.Fatal("unequip mutated tier")
					}
				}
			})
			if !found {
				t.Fatal("unequip lost tier")
			}

		})
	}
}
