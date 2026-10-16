package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func reserveLifecycleMail(t *testing.T, s *Scene, op string) MailReservation {
	t.Helper()
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailClaim{ID: 1, OperationID: op, Mail: domain.Mail{Money: 10}, Reply: reply})
	r := <-reply
	if r.Reason != "" || r.Reservation.Result == nil || r.Reservation.Done == nil || r.Reservation.Before.Char == nil {
		t.Fatalf("reservation failed: %+v", r)
	}
	return r.Reservation
}

func TestMailCancellationBeforePrepareAndLateReply(t *testing.T) {
	s, _ := equipScene(t)
	cancel := make(chan struct{})
	close(cancel)
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailClaim{ID: 1, OperationID: "cancelled", Cancel: cancel, Mail: domain.Mail{Money: 10}, Reply: reply})
	if r := <-reply; r.Reason == "" || s.players[1].Player.MailBusy || len(s.mailReservations) != 0 {
		t.Fatal("cancelled prepare reserved assets")
	}
	cancel = make(chan struct{})
	s.exec(ReserveMailClaim{ID: 1, OperationID: "late", Cancel: cancel, Mail: domain.Mail{Money: 10}, Reply: reply})
	close(cancel)
	r := <-reply
	// Timeout owns this late response and explicitly supplies known rollback.
	r.Reservation.Result <- false
	s.finishReadyMail(false)
	if s.players[1].Player.MailBusy || s.players[1].Player.Char.Money != r.Reservation.BeforeMoney {
		t.Fatal("late prepare response orphaned assets")
	}
}

func TestMailUnknownOutcomeRetainsFenceAndDefersDisconnect(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			s, _ := equipScene(t)
			saver := newFakeSaver()
			s.saver = saver
			p := s.players[1]
			p.Player.Char.Money = domain.Money{Copper: 100}
			reserved := reserveLifecycleMail(t, s, "mail-one")
			s.saveDirty()
			s.exec(Leave{ID: 1, FinalSaver: saver})
			if !s.deferTradeMutation(1, func() { p.Player.Bag.Add(场景药, 2); p.Player.MarkDirty() }) {
				t.Fatal("reward was not deferred")
			}
			// No receipt means ownership is unknown, never a rollback timeout.
			s.finishReadyMail(false)
			if !p.Player.MailBusy || s.players[1] == nil || saver.count(100) != 0 {
				t.Fatal("unknown mail outcome escaped the actor fence")
			}
			for len(s.mailbox) < cap(s.mailbox) {
				s.mailbox <- MoveTo{ID: 2}
			}
			reserved.Result <- commit
			s.finishReadyMail(false)
			select {
			case <-reserved.Done:
			default:
				t.Fatal("completion lost behind full mailbox")
			}
			want := int64(100)
			if commit {
				want = 110
			}
			money, _ := inventoryMoney(saver.last[100].Money)
			if s.players[1] != nil || money != want || saver.lastBag[100].CountOf(场景药.ID) != 2 {
				t.Fatal("mail completion lost money, reward, or disconnect")
			}
		})
	}
}

func TestMailStaleFinalizeCannotReleaseNewReservation(t *testing.T) {
	s, _ := equipScene(t)
	first := reserveLifecycleMail(t, s, "first")
	first.Result <- false
	s.finishReadyMail(false)
	second := reserveLifecycleMail(t, s, "second")
	s.exec(FinalizeMailReservation{ID: 1, Reservation: first, Commit: true})
	if !s.players[1].Player.MailBusy || len(s.mailReservations) != 1 {
		t.Fatal("stale completion released a new operation")
	}
	second.Result <- true
	s.finishReadyMail(false)
}

func TestMailFencesInventoryWithoutBlockingOtherPlayers(t *testing.T) {
	s, _ := equipScene(t)
	makeEligible(s)
	join(t, s, 2, 200, "other", 150, 150)
	putInBag(s, 0, 场景钢剑)
	r := reserveLifecycleMail(t, s, "fence")
	s.exec(Equip{ID: 1, BagSlot: 0})
	if s.players[1].Player.Worn.At(domain.SlotWeapon).Item != 0 {
		t.Fatal("reserved equipment changed")
	}
	s.exec(MoveTo{ID: 2, To: domain.Pos{MapID: 7, X: 160, Y: 160}})
	if s.players[2].Pos.X != 160 {
		t.Fatal("mail blocked another player")
	}
	r.Result <- false
	s.finishReadyMail(false)
	s.exec(Equip{ID: 1, BagSlot: 0})
	if s.players[1].Player.Worn.At(domain.SlotWeapon).Item != 场景钢剑.ID {
		t.Fatal("mail fence did not release")
	}
}

func TestMailAbandonedReplyCannotBlockSceneOrLeakReservation(t *testing.T) {
	s, _ := equipScene(t)
	before := s.players[1].Player.Char.Money
	s.exec(ReserveMailClaim{ID: 1, OperationID: "abandoned", Mail: domain.Mail{Money: 10}, Reply: make(chan MailReserveResult)})
	if s.players[1].Player.MailBusy || len(s.mailReservations) != 0 || s.players[1].Player.Char.Money != before {
		t.Fatal("undeliverable reservation was not restored")
	}
}

func TestMailCancelAfterPrepareCannotChooseDatabaseOutcome(t *testing.T) {
	s, _ := equipScene(t)
	reply := make(chan MailReserveResult, 1)
	cancel := make(chan struct{})
	s.exec(ReserveMailClaim{ID: 1, OperationID: "submitted", Cancel: cancel, Mail: domain.Mail{Money: 10}, Reply: reply})
	r := <-reply
	close(cancel)
	s.finishReadyMail(false)
	if !s.players[1].Player.MailBusy {
		t.Fatal("prepare cancellation guessed an in-flight database outcome")
	}
	r.Reservation.Result <- true
	s.finishReadyMail(false)
	money, _ := inventoryMoney(s.players[1].Player.Char.Money)
	before, _ := inventoryMoney(r.Reservation.BeforeMoney)
	if money != before+10 {
		t.Fatal("known commit was rolled back")
	}
}

func TestMailShutdownResolvesBeforeFinalSave(t *testing.T) {
	s, _ := equipScene(t)
	saver := newFakeSaver()
	s.saver = saver
	r := reserveLifecycleMail(t, s, "shutdown")
	r.Result <- false
	s.shutdown("test")
	select {
	case <-r.Done:
	default:
		t.Fatal("shutdown did not finalize mail")
	}
	money, _ := inventoryMoney(saver.last[100].Money)
	before, _ := inventoryMoney(r.BeforeMoney)
	if money != before {
		t.Fatal("shutdown persisted uncommitted mail assets")
	}
}

func TestMailPanicShutdownRejectsQueuedPrepare(t *testing.T) {
	s, _ := equipScene(t)
	replies := []chan MailReserveResult{make(chan MailReserveResult, 1), make(chan MailReserveResult, 1)}
	s.pendingMailbox = ReserveMailSend{ID: 1, OperationID: "pending", Reply: replies[0]}
	s.mailbox <- ReserveMailClaim{ID: 1, OperationID: "queued", Reply: replies[1]}
	s.finalizePendingEnters()
	for _, reply := range replies {
		select {
		case result := <-reply:
			if result.Reason == "" || result.Reservation.Done != nil {
				t.Fatal("discarded prepare reported success")
			}
		default:
			t.Fatal("discarded prepare left a timeout waiter behind")
		}
	}
}

func TestMailReservationDefersTrialOpeningSaveAndPreservesReward(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			s, _, _ := riftFixture(t)
			saver := newFakeSaver()
			s.saver = saver
			s.trialLevel = 60
			p := s.players[1]
			r := reserveLifecycleMail(t, s, "trial-mail")
			before := p.Player.Char.Trial
			s.beginEndgameTrial()
			if s.trialRun.Members[100] == nil {
				t.Fatal("reserved player lost opening membership")
			}
			if p.Player.Char.Trial != before || saver.count(100) != 0 {
				t.Fatal("trial opening published reserved assets")
			}
			s.finishEndgameTrialKeys()
			s.stepEndgameTrial()
			if !s.trialRun.Saved {
				t.Fatal("trial completion not durable")
			}
			s.exec(Leave{ID: 1, FinalSaver: saver})
			r.Result <- commit
			s.finishReadyMail(false)
			if s.players[1] != nil {
				t.Fatal("deferred disconnect did not finish")
			}
			saved := saver.last[100]
			if saved.Trial.CompletionID != s.trialRun.Nonce || saved.Trial.KeyReward <= 0 || saved.Quests[s.trial.Quest].State != domain.QuestDeliver {
				t.Fatal("deferred trial entitlement lost on disconnect")
			}
			money, _ := inventoryMoney(saved.Money)
			want, _ := inventoryMoney(r.BeforeMoney)
			if commit {
				want += 10
			}
			if money != want {
				t.Fatal("trial save used an unknown mail outcome")
			}
		})
	}
}
