package scene

import (
	"math"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestMailSendUsesTotalMoneyAndRollbackRestoresDenominations(t *testing.T) {
	s, _ := equipScene(t)
	p := s.players[1]
	before := domain.Money{Gold: 1, Silver: 2, Copper: 3}
	p.Player.Char.Money = before
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailSend{ID: 1, Money: 1004, Reply: reply})
	result := <-reply
	if result.Reason != "" || result.Snapshot.Char == nil || !p.Player.MailBusy {
		t.Fatalf("send should spend gold/silver balance: %+v", result)
	}
	if got, want := p.Player.Char.Money, (domain.Money{Gold: 1, Copper: 999}); got != want {
		t.Fatalf("balance = %+v, want %+v", got, want)
	}
	s.exec(FinalizeMailReservation{ID: 1, Reservation: result.Reservation, Done: make(chan struct{})})
	if p.Player.Char.Money != before || p.Player.MailBusy {
		t.Fatal("failed mail did not restore original money")
	}
}

func TestMailClaimNormalizesMoneyAndRejectsOverflow(t *testing.T) {
	s, _ := equipScene(t)
	p := s.players[1]
	p.Player.Char.Money = domain.Money{Copper: 999}
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailClaim{ID: 1, Mail: domain.Mail{Money: 1}, Reply: reply})
	result := <-reply
	if result.Reason != "" || p.Player.Char.Money != (domain.Money{Silver: 1}) {
		t.Fatalf("mail money was not normalized: %+v", result)
	}
	s.exec(FinalizeMailReservation{ID: 1, Reservation: result.Reservation, Commit: true, Done: make(chan struct{})})
	p.Player.Char.Money = domain.Money{Copper: math.MaxInt64}
	s.exec(ReserveMailClaim{ID: 1, Mail: domain.Mail{Money: 1}, Reply: reply})
	if result := <-reply; result.Reason == "" || p.Player.Char.Money.Copper != math.MaxInt64 {
		t.Fatal("overflowed mail balance was accepted")
	}
	s.exec(ReserveMailClaim{ID: 1, Mail: domain.Mail{Money: -1}, Reply: reply})
	if result := <-reply; result.Reason == "" {
		t.Fatal("negative mail money was accepted")
	}
}
