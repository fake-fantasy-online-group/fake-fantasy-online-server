package scene

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Session-level tests exercise wire decoding and page-local coordinates; these
// actor tests receive the already-translated authoritative inventory slots.
func nativeAstralCommand(t *testing.T, hole byte, runeSlot int32) InlayItem {
	t.Helper()
	return InlayItem{ID: 1, TargetKind: 0, Tab: 2, BagSlot: 200, Hole: hole, RuneTab: 1, RuneBagSlot: runeSlot}
}
func nativeAstralPreview(t *testing.T, s *Scene) {
	t.Helper()
	s.exec(DrillItem{ID: 1, TargetKind: 0, Tab: 2, BagSlot: 200})
}
func nativeAstralSuccess(s *Scene, b *astralTestBackend) {
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
}

func TestAstralNativeWireAtomicSuccessFailureAndRetry(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "storage_retry"}[fail], func(t *testing.T) {
			s, sink, b := astralSceneFixture(t)
			p := s.players[1]
			// No ordinary gem recipe, no funds/materials: insertion consumes only the chosen rune.
			s.sockets = nil
			p.Player.Char.Money = moneyFromInventory(0)
			before := s.snapshotOf(p).Clone()
			nativeAstralPreview(t, s)
			cmd := nativeAstralCommand(t, 0, 100)
			s.exec(cmd)
			if b.commits != 0 || !reflect.DeepEqual(before.Bag, p.Player.Bag) {
				t.Fatal("receipt gate bypassed")
			}
			s.finishEndgame(false)
			if b.commits != 1 || !p.Player.TradeBusy || !reflect.DeepEqual(before.Bag, p.Player.Bag) {
				t.Fatal("not an atomic candidate")
			}
			if b.plan.Snapshot.Bag.At(200).SocketUIDs[0] != 51 || !b.plan.Snapshot.Bag.At(100).Empty() || b.plan.Snapshot.Char.Money != before.Char.Money || b.plan.Snapshot.Bag.At(102) != before.Bag.At(102) {
				t.Fatal("wrong rune identity/cost")
			}
			requestID, digest := b.plan.Receipt.RequestID, b.plan.Receipt.Digest
			s.exec(cmd) // actor rejects repeated packets while commit owns inventory
			if b.commits != 1 {
				t.Fatal("concurrent packet committed twice")
			}
			if fail {
				b.result <- domain.EndgameCommitResult{Err: errors.New("disk unavailable")}
				s.finishEndgame(false)
				if p.Player.TradeBusy || !reflect.DeepEqual(before.Bag, p.Player.Bag) {
					t.Fatal("storage failure consumed rune")
				}
				s.exec(cmd)
				s.finishEndgame(false)
				if b.commits != 2 || b.plan.Receipt.RequestID != requestID || b.plan.Receipt.Digest != digest {
					t.Fatal("retry changed durable identity")
				}
			}
			nativeAstralSuccess(s, b)
			if p.Player.Bag.At(200).SocketUIDs[0] != 51 || p.Player.TradeBusy {
				t.Fatal("successful commit did not publish")
			}
			foundNotice, foundInfo := false, false
			for _, ev := range astralTake(s, sink) {
				switch v := ev.(type) {
				case event.ServerNotice:
					foundNotice = foundNotice || strings.Contains(v.Text, "星篆已镶入")
				case event.SocketInfo:
					foundInfo = foundInfo || len(v.Sockets) == 2 && v.Sockets[0] == domain.FirstAstralRune
				}
			}
			if !foundNotice || !foundInfo {
				t.Fatal("native furnace did not receive success/socket refresh")
			}
			commits := b.commits
			s.exec(cmd)
			s.finishEndgame(false)
			if b.commits != commits {
				t.Fatal("replayed native packet consumed twice")
			}
		})
	}
}

func TestAstralNativeWireRejectsInvalidSelections(t *testing.T) {
	for _, mode := range []string{"no_preview", "replacement_gear", "replacement_rune", "moved_gear", "locked_gear", "locked_rune", "locked_card", "rune_level", "occupied", "capacity", "wrong_tab", "wrong_rune_tab", "stale_revision", "mail_busy", "stall_busy", "trade_busy"} {
		t.Run(mode, func(t *testing.T) {
			s, _, b := astralSceneFixture(t)
			p := s.players[1]
			nativeAstralPreview(t, s)
			cmd := nativeAstralCommand(t, 0, 100)
			switch mode {
			case "no_preview":
				s.endgamePlayer(p).nativeSocket = nil
			case "replacement_gear":
				st := p.Player.Bag.At(200)
				st.UID++
				p.Player.Bag.Set(200, st)
			case "replacement_rune":
				st := p.Player.Bag.At(100)
				st.UID++
				p.Player.Bag.Set(100, st)
			case "moved_gear":
				st := p.Player.Bag.At(200)
				p.Player.Bag.Set(200, domain.Stack{})
				p.Player.Bag.Set(201, st)
			case "locked_gear":
				st := p.Player.Bag.At(200)
				st.Locked = true
				p.Player.Bag.Set(200, st)
				nativeAstralPreview(t, s)
			case "locked_rune":
				st := p.Player.Bag.At(100)
				st.Locked = true
				p.Player.Bag.Set(100, st)
				nativeAstralPreview(t, s)
			case "locked_card":
				st := p.Player.Bag.At(100)
				st.Card.Locked = true
				p.Player.Bag.Set(100, st)
				nativeAstralPreview(t, s)
			case "rune_level":
				p.Player.Char.Level = 11
			case "occupied":
				st := p.Player.Bag.At(200)
				st.Sockets[0] = domain.FirstAstralRune
				p.Player.Bag.Set(200, st)
				nativeAstralPreview(t, s)
			case "capacity":
				st := p.Player.Bag.At(200)
				st.SocketCount = 6
				p.Player.Bag.Set(200, st)
				nativeAstralPreview(t, s)
			case "wrong_tab":
				cmd.Tab = 1
			case "wrong_rune_tab":
				cmd.RuneTab = 2
			case "stale_revision":
				s.endgamePlayer(p).state.Revision++
			case "mail_busy":
				p.Player.MailBusy = true
			case "stall_busy":
				p.Player.StallBusy = true
			case "trade_busy":
				p.Player.TradeBusy = true
			}
			before := s.snapshotOf(p).Clone()
			s.exec(cmd)
			s.finishEndgame(false)
			if b.commits != 0 || !reflect.DeepEqual(before.Bag, p.Player.Bag) || before.Char.Money != p.Player.Char.Money {
				t.Fatal("invalid native packet mutated state")
			}
		})
	}
}

func TestAstralNativeWireRechecksAfterReceiptLookup(t *testing.T) {
	for _, mode := range []string{"move", "replace_rune", "forget_preview", "unrelated_revision", "economic_busy"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := astralSceneFixture(t)
			p := s.players[1]
			b := newDispatchBackend()
			s.endgameBackend = b
			nativeAstralPreview(t, s)
			s.exec(nativeAstralCommand(t, 0, 100))
			if b.receipts != 1 || b.commits != 0 {
				t.Fatal("missing async receipt lookup")
			}
			switch mode {
			case "move":
				st := p.Player.Bag.At(200)
				p.Player.Bag.Set(200, domain.Stack{})
				p.Player.Bag.Set(201, st)
			case "replace_rune":
				st := p.Player.Bag.At(100)
				st.UID++
				p.Player.Bag.Set(100, st)
			case "forget_preview":
				s.endgamePlayer(p).nativeSocket = nil
			case "unrelated_revision":
				s.endgamePlayer(p).state.Revision++
			case "economic_busy":
				p.Player.RackBusy = true
			}
			before := s.snapshotOf(p).Clone()
			b.receipt <- domain.EndgameReceiptLoadResult{}
			s.finishEndgame(false)
			if b.commits != 0 || !reflect.DeepEqual(before.Bag, p.Player.Bag) {
				t.Fatal("async stale slot passed")
			}
		})
	}
}

func TestAstralNativeWireLoadsDurableStateWithoutHandshake(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	b := newDispatchBackend()
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = false
	nativeAstralPreview(t, s)
	s.exec(nativeAstralCommand(t, 0, 100))
	if b.loads != 1 || b.receipts != 0 || b.commits != 0 {
		t.Fatal("native load not gated")
	}
	b.load <- domain.EndgameLoadResult{State: domain.EndgameState{Revision: 7, Payload: "{}"}}
	s.finishEndgame(false)
	s.exec(nativeAstralCommand(t, 0, 100))
	b.receipt <- domain.EndgameReceiptLoadResult{}
	s.finishEndgame(false)
	if b.commits != 1 || b.plan.ExpectedRevision != 7 {
		t.Fatal("native-only client did not use durable revision")
	}
}

func TestAstralNativeWireFullBagAndOrderedFormation(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordered", true: "reversed"}[reverse], func(t *testing.T) {
			s, sink, b := astralSceneFixture(t)
			p := s.players[1]
			card := domain.NewStack(s.items[domain.FirstAstralRune+5], 1)
			card.UID = 52
			p.Player.Bag.Set(101, card)
			for i := 100; i < 200; i++ {
				if p.Player.Bag.At(i).Empty() {
					p.Player.Bag.Set(i, domain.Stack{Item: domain.CubeStarMarrow, Count: 99})
				}
			}
			nativeAstralPreview(t, s)
			first, second := int32(100), int32(101)
			if reverse {
				first, second = second, first
			}
			s.exec(nativeAstralCommand(t, 0, first))
			s.finishEndgame(false)
			if b.commits != 1 {
				t.Fatal("full bag blocked insertion")
			}
			nativeAstralSuccess(s, b)
			// Socket refresh must rebind the updated target for the next drag.
			s.exec(nativeAstralCommand(t, 1, second))
			s.finishEndgame(false)
			if b.commits != 2 {
				t.Fatal("second native drag failed")
			}
			nativeAstralSuccess(s, b)
			st := p.Player.Bag.At(200)
			_, matched := domain.MatchAstralRecipe(s.items[st.Item], st)
			if matched == reverse || st.SocketUIDs[0] == st.SocketUIDs[1] {
				t.Fatal("hole order/formation identity wrong")
			}
			if info, ok := lastNativeSocketInfo(astralTake(s, sink)); !ok || len(info.Sockets) != 2 || info.Sockets[1] != st.Sockets[1] {
				t.Fatal("native holes stale after formation")
			}
		})
	}
}
func lastNativeSocketInfo(events []event.Event) (event.SocketInfo, bool) {
	var out event.SocketInfo
	ok := false
	for _, ev := range events {
		if v, yes := ev.(event.SocketInfo); yes {
			out = v
			ok = true
		}
	}
	return out, ok
}

func TestAstralNativeWireEveryRuneLevelBoundary(t *testing.T) {
	sample, _, _ := astralSceneFixture(t)
	for _, rune := range domain.AstralRunes(sample.endgameCatalog) {
		for _, allowed := range []bool{false, true} {
			t.Run(rune.Name+map[bool]string{false: "/below", true: "/at"}[allowed], func(t *testing.T) {
				s, _, b := astralSceneFixture(t)
				p := s.players[1]
				st := domain.NewStack(s.items[rune.Item], 1)
				st.UID = 51
				st.Bound = true
				p.Player.Bag.Set(100, st)
				p.Player.Char.Level = rune.MinLevel
				if !allowed {
					p.Player.Char.Level--
				}
				nativeAstralPreview(t, s)
				before := p.Player.Bag.Clone()
				s.exec(nativeAstralCommand(t, 0, 100))
				s.finishEndgame(false)
				if (b.commits == 1) != allowed || !reflect.DeepEqual(before, p.Player.Bag) {
					t.Fatal("level gate or candidate isolation")
				}
				if allowed {
					nativeAstralSuccess(s, b)
					gear := p.Player.Bag.At(200)
					if !gear.Bound || !gear.SocketCards[0].Bound || gear.SocketUIDs[0] != st.UID || gear.SocketCards[0].Effects != st.Card.Effects {
						t.Fatal("binding/rolled card identity lost")
					}
				}
			})
		}
	}
}

func TestAstralNativeWireWornFormationRefreshesStats(t *testing.T) {
	s, _, b := astralSceneFixture(t)
	p := s.players[1]
	st := p.Player.Bag.At(200)
	p.Player.Bag.Set(200, domain.Stack{})
	p.Player.Worn.Set(domain.SlotWeapon, st)
	rune := domain.NewStack(s.items[domain.FirstAstralRune+5], 1)
	rune.UID = 52
	p.Player.Bag.Set(101, rune)
	s.exec(DrillItem{ID: p.ID, TargetKind: 1, EquipSlot: uint8(domain.SlotWeapon)})
	cmd := nativeAstralCommand(t, 0, 100)
	cmd.TargetKind = 1
	cmd.EquipSlot = uint8(domain.SlotWeapon)
	cmd.BagSlot = 0
	cmd.Tab = 0
	s.exec(cmd)
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("worn first rune rejected")
	}
	nativeAstralSuccess(s, b)
	before := p.Stats
	cmd.Hole = 1
	cmd.RuneBagSlot = 101
	s.exec(cmd)
	s.finishEndgame(false)
	if b.commits != 2 {
		t.Fatal("worn second rune rejected")
	}
	nativeAstralSuccess(s, b)
	gear := p.Player.Worn.At(domain.SlotWeapon)
	if _, ok := domain.MatchAstralRecipe(s.items[gear.Item], gear); !ok || before == p.Stats || len(domain.ActiveAstralRecipes(p.Player.Worn, s.itemDef, p.Player.Char)) != 1 {
		t.Fatal("worn formation/stats did not refresh")
	}
}

func TestNativeLegacyCardStillUsesRecipeChanceCostsAndDestruction(t *testing.T) {
	for _, chance := range []int32{0, 100} {
		t.Run(map[int32]string{0: "destroy", 100: "success"}[chance], func(t *testing.T) {
			s, _, b := astralSceneFixture(t)
			p := s.players[1]
			def := domain.ItemDef{ID: 888001, Name: "旧符文", InventoryTab: 1, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceSocketCard, CardDefaults: domain.CardInstanceState{Initialized: true}}
			s.items[def.ID] = def
			card := domain.NewStack(def, 1)
			card.UID = 53
			p.Player.Bag.Set(100, card)
			gear := p.Player.Bag.At(200)
			equipment := s.items[gear.Item]
			s.sockets = domain.SocketTable{[3]int32{0, domain.RefineClass(equipment), equipment.Equip.Tier}: {InlayCost: 1000, SuccessRate: chance, DestroyRate: 100, Materials: []domain.CraftMaterial{{Item: domain.CubeStarMarrow, Qty: 1}}}}
			s.rng = rand.New(rand.NewSource(5))
			nativeAstralPreview(t, s)
			s.exec(nativeAstralCommand(t, 0, 100))
			if b.commits != 0 || !p.Player.Bag.At(100).Empty() || p.Player.Char.Money != moneyFromInventory(99000) || p.Player.Bag.CountOf(domain.CubeStarMarrow) != 0 {
				t.Fatal("legacy recipe cost changed")
			}
			if chance == 0 && !p.Player.Bag.At(200).Empty() {
				t.Fatal("legacy destruction bypassed")
			}
			if chance == 100 && (p.Player.Bag.At(200).SocketUIDs[0] != 53 || p.Player.Bag.At(200).SocketCards[0] != card.Card) {
				t.Fatal("legacy successful card identity changed")
			}
		})
	}
}

func TestAstralStaleNativeDragCannotConsumeReplacementLegacyCard(t *testing.T) {
	s, _, b := astralSceneFixture(t)
	p := s.players[1]
	nativeAstralPreview(t, s)
	def := domain.ItemDef{ID: 888001, Name: "旧符文", InventoryTab: 1, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceSocketCard, CardDefaults: domain.CardInstanceState{Initialized: true}}
	s.items[def.ID] = def
	replacement := domain.NewStack(def, 1)
	replacement.UID = 53
	p.Player.Bag.Set(100, replacement)
	gear := p.Player.Bag.At(200)
	equipment := s.items[gear.Item]
	s.sockets = domain.SocketTable{[3]int32{0, domain.RefineClass(equipment), equipment.Equip.Tier}: {SuccessRate: 100}}
	before := p.Player.Bag.Clone()
	s.exec(nativeAstralCommand(t, 0, 100))
	s.finishEndgame(false)
	if b.commits != 0 || !reflect.DeepEqual(before, p.Player.Bag) {
		t.Fatal("stale astral packet entered legacy random path")
	}
}

func TestAstralNativeSelectionRefreshesAfterOriginalDrill(t *testing.T) {
	s, _, b := astralSceneFixture(t)
	p := s.players[1]
	st := p.Player.Bag.At(200)
	st.SocketCount = 0
	p.Player.Bag.Set(200, st)
	def := s.items[st.Item]
	s.sockets = domain.SocketTable{[3]int32{0, domain.RefineClass(def), def.Equip.Tier}: {SuccessRate: 100}}
	nativeAstralPreview(t, s)
	s.exec(DrillItem{ID: p.ID, Tab: 2, BagSlot: 200, Execute: true})
	if p.Player.Bag.At(200).SocketCount != 1 {
		t.Fatal("original drill failed")
	}
	s.exec(nativeAstralCommand(t, 0, 100))
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("drilling required unnecessary equipment reselection")
	}
	nativeAstralSuccess(s, b)
}
