package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

const lifecycleHorn domain.ItemID = 777001

func prepareTestHorn(t *testing.T, s *Scene, cancel <-chan struct{}) MailReserveResult {
	t.Helper()
	s.items[lifecycleHorn] = domain.ItemDef{ID: lifecycleHorn, Name: "horn", HornTier: 1, HornSkin: 2, Stackable: true}
	s.players[1].Player.Bag.Set(0, domain.Stack{Item: lifecycleHorn, Count: 2})
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveHorn{ID: 1, Item: lifecycleHorn, Skin: 2, OperationID: "horn-operation", Cancel: cancel, Reply: reply})
	return <-reply
}
func TestHornKnownFailureRestoresItemAndUnknownRetainsFence(t *testing.T) {
	s, _ := equipScene(t)
	saver := newFakeSaver()
	s.saver = saver
	r := prepareTestHorn(t, s, nil)
	p := s.players[1]
	if r.Reason != "" || r.Reservation.Result == nil || r.Reservation.Before.Bag.CountOf(lifecycleHorn) != 2 || p.Player.Bag.CountOf(lifecycleHorn) != 1 || !p.Player.MailBusy {
		t.Fatal("horn reservation is untracked")
	}
	s.saveDirty()
	s.exec(Leave{ID: 1, FinalSaver: saver})
	s.finishReadyMail(false)
	if saver.count(100) != 0 || s.players[1] == nil || !p.Player.MailBusy {
		t.Fatal("unknown horn outcome escaped fence")
	}
	r.Reservation.Result <- false
	s.finishReadyMail(false)
	if saver.lastBag[100].CountOf(lifecycleHorn) != 2 || s.players[1] != nil {
		t.Fatal("failed horn was lost")
	}
}
func TestHornPrepareCancellationAndLateResult(t *testing.T) {
	s, _ := equipScene(t)
	cancel := make(chan struct{})
	close(cancel)
	r := prepareTestHorn(t, s, cancel)
	if r.Reason == "" || s.players[1].Player.MailBusy || s.players[1].Player.Bag.CountOf(lifecycleHorn) != 2 {
		t.Fatal("cancelled horn consumed")
	}
	cancel = make(chan struct{})
	r = prepareTestHorn(t, s, cancel)
	close(cancel)
	s.finishReadyMail(false)
	if !s.players[1].Player.MailBusy {
		t.Fatal("cancel guessed database outcome")
	}
	r.Reservation.Result <- false
	s.finishReadyMail(false)
	if s.players[1].Player.Bag.CountOf(lifecycleHorn) != 2 {
		t.Fatal("late horn reply could not restore")
	}
}
func TestHornUsesSharedInventoryFence(t *testing.T) {
	for _, busy := range []string{"mail", "trade", "stall"} {
		t.Run(busy, func(t *testing.T) {
			s, _ := equipScene(t)
			p := s.players[1].Player
			switch busy {
			case "mail":
				p.MailBusy = true
			case "trade":
				p.TradeBusy = true
			case "stall":
				p.StallBusy = true
			}
			r := prepareTestHorn(t, s, nil)
			if r.Reason == "" || p.Bag.CountOf(lifecycleHorn) != 2 {
				t.Fatal("horn bypassed inventory fence")
			}
		})
	}
}
func TestHornPanicShutdownRejectsPrepare(t *testing.T) {
	s, _ := equipScene(t)
	reply := make(chan MailReserveResult, 1)
	s.mailbox <- ReserveHorn{ID: 1, Reply: reply}
	s.finalizePendingEnters()
	select {
	case r := <-reply:
		if r.Reason == "" {
			t.Fatal("discarded horn reported success")
		}
	default:
		t.Fatal("horn waiter orphaned")
	}
}
