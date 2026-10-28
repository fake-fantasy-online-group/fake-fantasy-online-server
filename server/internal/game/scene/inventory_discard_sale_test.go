package scene

import (
	"math"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type discardCase struct {
	name  string
	def   domain.ItemDef
	stack domain.Stack
}

func discardCases() []discardCase {
	material := domain.ItemDef{ID: 4001, Name: "铁矿石", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
	rune := astralRuneItemsFixture(0)[0]
	spirit := domain.ItemDef{ID: domain.SpiritDefinitions()[0].ItemID, InstanceKind: domain.ItemInstanceSpirit, InventoryTabKnown: true}
	power := domain.ItemDef{ID: domain.PowerScrollItemID, InventoryTab: 3, InventoryTabKnown: true}
	card := domain.ItemDef{ID: 888001, InstanceKind: domain.ItemInstanceSocketCard, InventoryTab: 1, InventoryTabKnown: true, CardDefaults: domain.CardInstanceState{Initialized: true}}
	gear := 场景钢剑
	gear.InstanceKind = domain.ItemInstanceEquipment
	starting := gear
	starting.ID = 1011
	names := []string{"material", "rune", "spirit", "power-scroll", "socket-card", "equipment", "starting-sword"}
	cases := []discardCase{}
	for i, d := range []domain.ItemDef{material, rune, spirit, power, card, gear, starting} {
		count := int32(1)
		if d.Stackable {
			count = 7
		}
		st := domain.NewStack(d, count)
		st.Bound = true
		cases = append(cases, discardCase{names[i], d, st})
	}
	cases[1].stack.Card.Bound = true
	cases[3].stack.UID = 9001
	cases[3].stack.Endgame.PowerScrollID = "TY-A01"
	cases[3].stack.Endgame.PowerScrollRollBP = 10000
	legacy := cases[1]
	legacy.name = "legacy-card-bound-only"
	legacy.stack.Bound = false
	return append(cases, legacy)
}

func discardFixture(t *testing.T, tc discardCase) (*Scene, *entity.Entity, *fakeSink, *fakeSaver, int) {
	t.Helper()
	saver := newFakeSaver()
	s := newTestScene(t, saver)
	s.items = map[domain.ItemID]domain.ItemDef{tc.def.ID: tc.def}
	sink := join(t, s, 1, 100, "discard", 100, 100)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	p.Player.Char.Money = moneyFromInventory(1234567)
	slot := int(tc.def.InventoryTab) * domain.BagPageSlots
	p.Player.Bag.Set(slot, tc.stack)
	s.flush()
	sink.take()
	return s, p, sink, saver, slot
}

func TestBoundDropDestroysOnlySelectedStackAndPersists(t *testing.T) {
	for _, tc := range discardCases() {
		t.Run(tc.name, func(t *testing.T) {
			s, p, sink, saver, slot := discardFixture(t, tc)
			neighbor := tc.stack
			if neighbor.UID != 0 {
				neighbor.UID++
			}
			p.Player.Bag.Set(slot+1, neighbor)
			money := p.Player.Char.Money
			p.Player.RepairQuote = &entity.RepairQuote{}
			// Destruction has no ground entity, allocator or placement dependency.
			s.alloc = nil
			s.walkable = func(domain.Pos) bool { t.Fatal("destroy queried ground mask"); return false }
			cmd := DropBagItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, Confirmed: true, At: domain.Pos{X: 100000}}
			s.exec(cmd)
			if !p.Player.Bag.At(slot).Empty() || p.Player.Bag.At(slot+1) != neighbor || p.Player.Char.Money != money || p.Player.RepairQuote != nil || len(s.ground) != 0 {
				t.Fatal("destroy changed neighbor/money, kept quote, or spawned loot")
			}
			if saver.count(100) != 1 || !saver.lastBag[100].At(slot).Empty() || saver.lastBag[100].At(slot+1) != neighbor || saver.last[100].Money != money {
				t.Fatal("saved snapshot lost destruction or surviving UID metadata")
			}
			s.flush()
			evs := sink.take()
			if _, ok := firstOf[event.InventorySnapshot](evs); !ok || countOf[event.EntitySpawned](evs) != 0 {
				t.Fatal("missing inventory receipt or emitted ground spawn")
			}
			s.exec(cmd)
			if saver.count(100) != 1 || p.Player.Bag.At(slot+1) != neighbor {
				t.Fatal("replayed discard consumed neighbor")
			}
		})
	}
}

func TestBoundDropPartialViaExistingSplitPreservesRemainingIdentity(t *testing.T) {
	for _, tc := range discardCases()[:2] {
		t.Run(tc.name, func(t *testing.T) {
			s, p, _, saver, slot := discardFixture(t, tc)
			s.exec(SplitBagItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Count: 2})
			remain, taken := p.Player.Bag.At(slot), p.Player.Bag.At(slot+1)
			want := tc.stack
			want.Count -= 2
			if remain != want || taken.Count != 2 || taken.Bound != tc.stack.Bound || taken.Card != tc.stack.Card || tc.stack.UID != 0 && (taken.UID == 0 || taken.UID == remain.UID) {
				t.Fatal("split changed ownership metadata")
			}
			s.exec(DropBagItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot + 1, Item: tc.def.ID, Confirmed: true})
			if p.Player.Bag.At(slot) != want || !p.Player.Bag.At(slot+1).Empty() || len(s.ground) != 0 || saver.lastBag[100].At(slot) != want {
				t.Fatal("partial discard removed wrong quantity/UID")
			}
		})
	}
}

func TestBoundDropRejectsUnconfirmedLockedInvalidAndBusy(t *testing.T) {
	for _, mode := range []string{"unconfirmed", "root-lock", "card-lock", "wrong-item", "wrong-tab", "negative-slot", "large-slot", "empty-count", "missing-definition", "unknown-tab", "stall", "pet", "trade", "mail", "worn-only"} {
		t.Run(mode, func(t *testing.T) {
			tc := discardCases()[1]
			s, p, _, saver, slot := discardFixture(t, tc)
			cmd := DropBagItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, Confirmed: true}
			st := tc.stack
			switch mode {
			case "unconfirmed":
				cmd.Confirmed = false
			case "root-lock":
				st.Locked = true
			case "card-lock":
				st.Card.Locked = true
			case "wrong-item":
				cmd.Item++
			case "wrong-tab":
				cmd.Tab++
			case "negative-slot":
				cmd.Slot = -1
			case "large-slot":
				cmd.Slot = p.Player.Bag.Cap()
			case "empty-count":
				st.Count = 0
			case "missing-definition":
				delete(s.items, st.Item)
			case "unknown-tab":
				d := tc.def
				d.InventoryTabKnown = false
				s.items[d.ID] = d
			case "stall":
				p.Player.Stall = &domain.Stall{}
			case "pet":
				st.InstanceKind = domain.ItemInstancePet
			case "trade":
				p.Player.TradeBusy = true
			case "mail":
				p.Player.MailBusy = true
			case "worn-only":
				p.Player.Worn.Set(domain.SlotWeapon, st)
				st = domain.Stack{}
			}
			p.Player.Bag.Set(slot, st)
			before, worn, money := p.Player.Bag.Clone(), p.Player.Worn.Clone(), p.Player.Char.Money
			s.exec(cmd)
			if !reflect.DeepEqual(before, p.Player.Bag) || !reflect.DeepEqual(worn, p.Player.Worn) || money != p.Player.Char.Money || saver.count(100) != 0 || len(s.ground) != 0 {
				t.Fatal("rejected discard mutated state")
			}
		})
	}
}

func TestLegacyRuneProtectionProjectedForNativeConfirmation(t *testing.T) {
	tc := discardCases()[7]
	s, p, _, _, _ := discardFixture(t, tc)
	snap, err := s.inventorySnapshot(p)
	if err != nil || len(snap.Items) != 1 || !snap.Items[0].Bound {
		t.Fatal("legacy card-bound rune skips native confirmation", err)
	}
	st := tc.stack
	st.Card.Locked = true
	p.Player.Bag.Set(100, st)
	snap, err = s.inventorySnapshot(p)
	if err != nil || !snap.Items[0].Locked {
		t.Fatal("legacy lock invisible", err)
	}
}

func TestUnboundDropStillValidatesPlacementAndPreservesInstance(t *testing.T) {
	for _, mode := range []string{"ordinary", "far", "blocked", "no-allocator"} {
		t.Run(mode, func(t *testing.T) {
			tc := discardCases()[5]
			tc.stack.Bound = false
			s, p, _, saver, slot := discardFixture(t, tc)
			s.alloc = domain.NewEntityAlloc()
			cmd := DropBagItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, At: p.Pos}
			switch mode {
			case "far":
				cmd.At.X += 100000
			case "blocked":
				s.walkable = func(domain.Pos) bool { return false }
			case "no-allocator":
				s.alloc = nil
			}
			s.exec(cmd)
			if mode != "ordinary" {
				if p.Player.Bag.At(slot) != tc.stack || len(s.ground) != 0 || saver.count(100) != 0 {
					t.Fatal("invalid placement changed inventory")
				}
				return
			}
			if !p.Player.Bag.At(slot).Empty() || len(s.ground) != 1 {
				t.Fatal("unbound item not dropped")
			}
			for id, ground := range s.ground {
				if ground.Drop.Stack != tc.stack {
					t.Fatal("ground lost UID/metadata")
				}
				s.exec(PickUp{ID: 1, Target: id})
			}
			if p.Player.Bag.At(slot) != tc.stack || len(s.ground) != 0 {
				t.Fatal("pickup changed identity")
			}
		})
	}
}

func allowDiscardFixtureShop(s *Scene, p *entity.Entity) {
	s.shops = domain.ShopTable{700: {ID: 700, AllowSell: true}}
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, NPC: &entity.NPCInfo{Sell: 700}}
	p.Player.ShopID = 700
}

func TestNPCZeroPriceSaleConsumesExactCountWithoutMoney(t *testing.T) {
	for _, tc := range discardCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, full := range []bool{false, true} {
				s, p, _, saver, slot := discardFixture(t, tc)
				allowDiscardFixtureShop(s, p)
				count := int32(1)
				if full {
					count = tc.stack.Count
				}
				money := p.Player.Char.Money
				s.exec(SellItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, Count: count, Confirmed: true})
				want := tc.stack
				want.Count -= count
				if want.Count == 0 {
					want = domain.Stack{}
				}
				if p.Player.Bag.At(slot) != want || p.Player.Char.Money != money || len(s.ground) != 0 || saver.count(100) != 1 || saver.lastBag[100].At(slot) != want || saver.last[100].Money != money {
					t.Fatal("zero sale changed currency, count or remaining UID metadata")
				}
			}
		})
	}
}

func TestNPCZeroPriceSaleKeepsValidationGuards(t *testing.T) {
	for _, mode := range []string{"negative-price", "missing-definition", "unknown-tab", "wrong-tab", "wrong-item", "zero-count", "negative-count", "excess-count", "unconfirmed", "root-lock", "card-lock", "buy-only", "missing-npc", "negative-slot", "trade", "mail", "multiply-overflow", "money-overflow", "worn-only"} {
		t.Run(mode, func(t *testing.T) {
			tc := discardCases()[1]
			s, p, _, saver, slot := discardFixture(t, tc)
			allowDiscardFixtureShop(s, p)
			cmd := SellItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, Count: 1, Confirmed: true}
			st, d := tc.stack, tc.def
			switch mode {
			case "negative-price":
				d.SellPrice = -1
			case "unknown-tab":
				d.InventoryTabKnown = false
			case "wrong-tab":
				cmd.Tab++
			case "wrong-item":
				cmd.Item++
			case "zero-count":
				cmd.Count = 0
			case "negative-count":
				cmd.Count = -1
			case "excess-count":
				cmd.Count = st.Count + 1
			case "unconfirmed":
				cmd.Confirmed = false
			case "root-lock":
				st.Locked = true
			case "card-lock":
				st.Card.Locked = true
			case "buy-only":
				s.shops[700] = domain.Shop{ID: 700, AllowSell: false}
			case "missing-npc":
				delete(s.entities, 77)
			case "negative-slot":
				cmd.Slot = -1
			case "trade":
				p.Player.TradeBusy = true
			case "mail":
				p.Player.MailBusy = true
			case "multiply-overflow":
				d.SellPrice = math.MaxInt64
				cmd.Count = 2
			case "money-overflow":
				d.SellPrice = 1
				p.Player.Char.Money = moneyFromInventory(math.MaxInt64)
			case "worn-only":
				p.Player.Worn.Set(domain.SlotWeapon, st)
				st = domain.Stack{}
			}
			s.items[d.ID] = d
			if mode == "missing-definition" {
				delete(s.items, d.ID)
			}
			p.Player.Bag.Set(slot, st)
			before, worn, money := p.Player.Bag.Clone(), p.Player.Worn.Clone(), p.Player.Char.Money
			s.exec(cmd)
			if !reflect.DeepEqual(before, p.Player.Bag) || !reflect.DeepEqual(worn, p.Player.Worn) || money != p.Player.Char.Money || saver.count(100) != 0 {
				t.Fatal("rejected sale mutated inventory or money")
			}
		})
	}
}

func TestDiscardAndSaleSnapshotFailureRollsBackAllState(t *testing.T) {
	for _, mode := range []string{"bound-drop", "unbound-drop", "zero-sale", "paid-sale"} {
		t.Run(mode, func(t *testing.T) {
			tc := discardCases()[0]
			if mode == "unbound-drop" {
				tc.stack.Bound = false
			}
			if mode == "paid-sale" {
				tc.def.SellPrice = 25
			}
			s, p, sink, saver, slot := discardFixture(t, tc)
			s.alloc = domain.NewEntityAlloc()
			allowDiscardFixtureShop(s, p)
			bad := domain.ItemDef{ID: 777001, InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Weight: math.MaxInt32}
			s.items[bad.ID] = bad
			p.Player.Bag.Set(slot+1, domain.NewStack(bad, 2))
			quote := &entity.RepairQuote{}
			p.Player.RepairQuote = quote
			before, money := p.Player.Bag, p.Player.Char.Money
			if mode == "bound-drop" || mode == "unbound-drop" {
				s.exec(DropBagItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, Confirmed: true, At: p.Pos})
			} else {
				s.exec(SellItem{ID: 1, Tab: tc.def.InventoryTab, Slot: slot, Item: tc.def.ID, Count: 2, Confirmed: true})
			}
			if p.Player.Bag != before || p.Player.Bag.At(slot) != tc.stack || p.Player.Char.Money != money || p.Player.RepairQuote != quote || len(s.ground) != 0 || saver.count(100) != 0 {
				t.Fatal("invalid snapshot committed partial inventory/currency or ground spawn")
			}
			s.flush()
			if countOf[event.InventorySnapshot](sink.take()) != 0 {
				t.Fatal("invalid inventory published")
			}
		})
	}
}
