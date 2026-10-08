package scene

import (
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func reserveInventoryTestTrade(t *testing.T, s *Scene, cancel <-chan struct{}) TradeSettlementResult {
	t.Helper()
	if s.players[2] == nil {
		join(t, s, 2, 200, "乙", 150, 150)
	}
	p := s.players[1]
	p.Player.Char.Money = domain.Money{Gold: 1}
	s.players[2].Player.Char.Money = domain.Money{}
	reply := make(chan TradeSettlementResult, 1)
	s.exec(ReserveTradeSettlement{A: 1, B: 2,
		AOffer: TradeSettlementOffer{Money: 100}, Reply: reply, Cancel: cancel})
	result := <-reply
	if result.Reason != "" || len(result.Snapshots) != 2 {
		t.Fatalf("reserve failed: %+v", result)
	}
	return result
}

func TestTradeReservationProtectsEquipmentAndBagUntilRollback(t *testing.T) {
	s, _ := equipScene(t)
	makeEligible(s)
	p := s.players[1]
	putInBag(s, 0, 场景钢剑)
	p.Player.Worn.Set(domain.SlotShield, domain.NewStack(场景盾, 1))
	p.Player.Bag.Set(1, domain.Stack{Item: 场景药.ID, Count: 3})
	beforeBag, beforeWorn := p.Player.Bag.Clone(), p.Player.Worn.Clone()
	reserved := reserveInventoryTestTrade(t, s, nil)
	for _, command := range []Command{
		Equip{ID: 1, BagSlot: 0}, Unequip{ID: 1, Slot: domain.SlotShield},
		MoveBagItem{ID: 1, Tab: 2, From: 0, To: 3},
		SplitBagItem{ID: 1, Tab: 1, Slot: 1, Count: 1},
		DropBagItem{ID: 1, Slot: 0, Tab: 2, Item: 场景钢剑.ID, At: p.Pos},
		SetItemLock{ID: 1}, UseItem{ID: 1},
	} {
		s.exec(command)
		if !reflect.DeepEqual(p.Player.Bag, beforeBag) || !reflect.DeepEqual(p.Player.Worn, beforeWorn) {
			t.Fatalf("%s changed reserved inventory", command.CmdName())
		}
	}
	// A regular position update still works while the pair is in the database.
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 110, Y: 110}})
	if p.Pos.X != 110 || p.Pos.Y != 110 {
		t.Fatalf("trade blocked ordinary movement: %+v", p.Pos)
	}
	reserved.Reservation.Result <- false
	s.step()
	if !reflect.DeepEqual(p.Player.Bag, beforeBag) || !reflect.DeepEqual(p.Player.Worn, beforeWorn) ||
		p.Player.Char.Money != (domain.Money{Gold: 1}) || p.Player.TradeBusy {
		t.Fatal("rollback changed an equipment instance or failed to release reservation")
	}
	s.exec(Equip{ID: 1, BagSlot: 0})
	if p.Player.Worn.At(domain.SlotWeapon).Item != 场景钢剑.ID {
		t.Fatal("equipment stayed blocked after rollback")
	}
}

func TestTradeReservationRepliesToOtherInventoryTransactions(t *testing.T) {
	s, _ := equipScene(t)
	reserved := reserveInventoryTestTrade(t, s, nil)
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailSend{ID: 1, Money: 1, Reply: reply})
	select {
	case result := <-reply:
		if result.Reason == "" || result.Snapshot.Char != nil {
			t.Fatal("mail started during trade")
		}
	default:
		t.Fatal("rejected mail did not receive its reply")
	}
	reserved.Reservation.Result <- false
	s.step()
}

func TestTradeCompletionSurvivesFullMailboxAndDefersFinalSave(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			s, _ := equipScene(t)
			saver := newFakeSaver()
			s.saver = saver
			reserved := reserveInventoryTestTrade(t, s, nil)
			p := s.players[1]
			p.Player.MarkDirty()
			s.saveDirty()
			if saver.count(100) != 0 {
				t.Fatal("periodic saver received uncommitted trade inventory")
			}
			s.exec(Leave{ID: 1, FinalSaver: saver})
			// Even an effect queued after disconnect must be applied before its
			// final save, rather than disappearing when Leave removes the player.
			s.deferTradeMutation(1, func() {
				p.Player.Bag.Add(场景药, 2)
				p.Player.MarkDirty()
			})
			if s.players[1] == nil || saver.count(100) != 0 {
				t.Fatal("disconnect saved or removed reserved inventory too early")
			}
			for len(s.mailbox) < cap(s.mailbox) {
				s.mailbox <- MoveTo{ID: 2}
			}
			reserved.Reservation.Result <- commit
			s.step() // result has its own channel; mailbox fullness is irrelevant
			select {
			case <-reserved.Reservation.Done:
			default:
				t.Fatal("settlement completion was lost")
			}
			if s.players[1] != nil || saver.lastBag[100].CountOf(场景药.ID) != 2 {
				t.Fatal("deferred leave lost the server-owned reward")
			}
			money, _ := inventoryMoney(saver.last[100].Money)
			want := int64(1_000_000)
			if commit {
				want -= 100
			}
			if money != want {
				t.Fatalf("final saved money = %d, want %d", money, want)
			}
		})
	}
}

func TestTradeCompletionDuringShutdownAndReservationTimeout(t *testing.T) {
	for _, cancelIt := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "timeout"}[cancelIt], func(t *testing.T) {
			s, _ := equipScene(t)
			saver := newFakeSaver()
			s.saver = saver
			cancel := make(chan struct{})
			reserved := reserveInventoryTestTrade(t, s, cancel)
			s.Stop()
			if cancelIt {
				close(cancel)
			} else {
				reserved.Reservation.Result <- true
			}
			s.shutdown("test")
			money, _ := inventoryMoney(saver.last[100].Money)
			want := int64(999_900)
			if cancelIt {
				want = 1_000_000
			}
			if money != want || len(s.tradeSettlements) != 0 {
				t.Fatalf("shutdown saved unresolved money %d, want %d", money, want)
			}
		})
	}
}

func TestTradeCannotStartDuringMailReservation(t *testing.T) {
	s, _ := equipScene(t)
	join(t, s, 2, 200, "乙", 150, 150)
	s.players[1].Player.MailBusy = true
	reply := make(chan TradeSettlementResult, 1)
	s.exec(ReserveTradeSettlement{A: 1, B: 2, Reply: reply})
	if result := <-reply; result.Reason == "" || len(result.Snapshots) != 0 {
		t.Fatal("trade overlapped a pending mail mutation")
	}
}
