package scene

import (
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func reserveLetter(t *testing.T, s *Scene, operation string) MailReserveResult {
	t.Helper()
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailSend{ID: 1, OperationID: operation, Reply: reply})
	return <-reply
}

func TestLetterPaperReservationCommitRollbackAndRepeatedSend(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			s, _ := equipScene(t)
			p := s.players[1]
			p.Player.Bag.Set(0, domain.Stack{Item: domain.MailLetterPaperID, Count: 2})
			p.Player.Bag.Set(1, domain.Stack{Item: 场景药.ID, Count: 7})
			beforeMoney := p.Player.Char.Money
			result := reserveLetter(t, s, "letter")
			if result.Reason != "" || !p.Player.MailBusy || result.Reservation.Done == nil {
				t.Fatalf("letter reservation failed: %+v", result)
			}
			if p.Player.Bag.CountOf(domain.MailLetterPaperID) != 1 ||
				result.Reservation.BeforeBag.CountOf(domain.MailLetterPaperID) != 2 {
				t.Fatal("paper was not reserved exactly once with a rollback snapshot")
			}
			if repeated := reserveLetter(t, s, "duplicate"); repeated.Reason == "" ||
				p.Player.Bag.CountOf(domain.MailLetterPaperID) != 1 {
				t.Fatal("repeated send bypassed MailBusy or consumed another paper")
			}
			// An unresolved result remains reserved; it must not silently refund.
			s.finishReadyMail(false)
			if !p.Player.MailBusy || p.Player.Bag.CountOf(domain.MailLetterPaperID) != 1 {
				t.Fatal("uncertain outcome lost its reservation fence")
			}
			result.Reservation.Result <- commit
			s.finishReadyMail(false)
			want := int32(2)
			if commit {
				want = 1
			}
			if p.Player.MailBusy || p.Player.Bag.CountOf(domain.MailLetterPaperID) != want ||
				p.Player.Bag.CountOf(场景药.ID) != 7 || p.Player.Char.Money != beforeMoney {
				t.Fatal("mail completion changed the wrong item/money or lost its paper outcome")
			}
			s.exec(FinalizeMailReservation{ID: 1, Reservation: result.Reservation, Commit: !commit})
			if p.Player.Bag.CountOf(domain.MailLetterPaperID) != want {
				t.Fatal("stale finalization changed a completed letter")
			}
		})
	}
}

func TestLetterPaperMissingLockedAndCanceled(t *testing.T) {
	for _, scenario := range []string{"missing", "locked", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			s, _ := equipScene(t)
			p := s.players[1]
			if scenario != "missing" {
				p.Player.Bag.Set(0, domain.Stack{Item: domain.MailLetterPaperID, Count: 3, Locked: scenario == "locked"})
			}
			before := p.Player.Bag.CountOf(domain.MailLetterPaperID)
			reply := make(chan MailReserveResult, 1)
			var cancel chan struct{}
			if scenario == "cancelled" {
				cancel = make(chan struct{})
				close(cancel)
			}
			s.exec(ReserveMailSend{ID: 1, Cancel: cancel, Reply: reply})
			result := <-reply
			if result.Reason == "" || p.Player.MailBusy || len(s.mailReservations) != 0 ||
				p.Player.Bag.CountOf(domain.MailLetterPaperID) != before {
				t.Fatal("rejected/cancelled editor flow consumed paper or reserved mail")
			}
			if scenario != "cancelled" && !strings.Contains(result.Reason, "信纸") {
				t.Fatal("missing paper should explain the original merchant item")
			}
		})
	}
}

func TestLetterPaperUsesUnlockedStackAndParcelBehaviorIsUnchanged(t *testing.T) {
	s, _ := equipScene(t)
	p := s.players[1]
	p.Player.Bag.Set(0, domain.Stack{Item: domain.MailLetterPaperID, Count: 2, Locked: true})
	p.Player.Bag.Set(1, domain.Stack{Item: domain.MailLetterPaperID, Count: 2})
	result := reserveLetter(t, s, "unlocked")
	if result.Reason != "" || p.Player.Bag.At(0).Count != 2 || p.Player.Bag.At(1).Count != 1 {
		t.Fatal("letter consumed a locked paper or failed to find the available stack")
	}
	result.Reservation.Result <- true
	s.finishReadyMail(false)
	p.Player.Char.Money = domain.Money{Copper: 10}
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailSend{ID: 1, Money: 1, OperationID: "parcel", Reply: reply})
	result = <-reply
	if result.Reason != "" || p.Player.Bag.CountOf(domain.MailLetterPaperID) != 3 {
		t.Fatal("letter restoration must not invent a parcel paper cost")
	}
	result.Reservation.Result <- false
	s.finishReadyMail(false)
}
