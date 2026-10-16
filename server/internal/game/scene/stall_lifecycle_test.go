package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
	"time"
)

func reserveLifecycleStallDeal(t *testing.T, s *Scene, cancel <-chan struct{}) StallReserveResult {
	t.Helper()
	join(t, s, 2, 200, "乙", 150, 150)
	def := 场景药
	def.CanTrade = true
	s.items[def.ID] = def
	s.players[1].Player.Char.Money = domain.Money{Copper: 100}
	owner := s.players[2]
	owner.Player.Stall = domain.NewStall(domain.StallSell, "测试")
	owner.Player.Stall.Add(domain.StallItem{Stack: domain.Stack{Item: def.ID, Count: 2}, UnitPrice: 10})
	reply := make(chan StallReserveResult, 1)
	s.exec(ReserveStallDeal{ID: 1, Owner: 2, Count: 1, ExpectedItem: def.ID, ExpectedPrice: 10, Reply: reply, Cancel: cancel})
	result := <-reply
	if result.Reason != "" || result.Reservation.Done == nil {
		t.Fatalf("prepare: %+v", result)
	}
	return result
}

func TestStallUnknownOutcomeFencesBothPlayersAndDisconnect(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			s, _ := equipScene(t)
			reserved := reserveLifecycleStallDeal(t, s, nil)
			saver := newFakeSaver()
			s.saver = saver
			for _, id := range []domain.EntityID{1, 2} {
				s.players[id].Player.MarkDirty()
				if !s.deferTradeMutation(id, func() {}) {
					t.Fatal("server effect escaped fence")
				}
				s.exec(Leave{ID: id, FinalSaver: saver})
			}
			s.saveDirty()
			s.finishReadyStalls(false)
			if len(s.players) != 2 || saver.count(100) != 0 || saver.count(200) != 0 {
				t.Fatal("unknown outcome escaped ownership")
			}
			for len(s.mailbox) < cap(s.mailbox) {
				s.mailbox <- MoveTo{ID: 99}
			}
			reserved.Reservation.Result <- commit
			s.finishReadyStalls(false)
			select {
			case <-reserved.Reservation.Done:
			default:
				t.Fatal("completion lost behind mailbox")
			}
			if len(s.players) != 0 {
				t.Fatal("disconnect not resumed")
			}
			wantBuyer, wantSeller := int64(100), int64(0)
			wantBuyerItems, wantSellerItems := int32(0), int32(2)
			if commit {
				wantBuyer, wantSeller = 90, 10
				wantBuyerItems, wantSellerItems = 1, 1
			}
			for id, want := range map[int64]int64{100: wantBuyer, 200: wantSeller} {
				money, _ := inventoryMoney(saver.last[id].Money)
				if money != want {
					t.Fatalf("character %d saved %d, want %d", id, money, want)
				}
			}
			if saver.lastBag[100].CountOf(场景药.ID) != wantBuyerItems || saver.lastBag[200].CountOf(场景药.ID) != wantSellerItems {
				t.Fatal("departure lost/duplicated stall goods")
			}
		})
	}
}

func TestStallTimeoutAndStaleCompletion(t *testing.T) {
	s, _ := equipScene(t)
	cancel := make(chan struct{})
	first := reserveLifecycleStallDeal(t, s, cancel)
	close(cancel)
	s.finishReadyStalls(false)
	if s.players[1].Player.StallBusy || s.players[2].Player.StallBusy {
		t.Fatal("late prepare orphaned reservation")
	}
	reply := make(chan StallReserveResult, 1)
	s.exec(ReserveStallEnd{ID: 2, Reply: reply})
	second := <-reply
	s.onFinalizeStallReservation(FinalizeStallReservation{Reservation: first.Reservation, Commit: true})
	if !s.players[2].Player.StallBusy {
		t.Fatal("stale completion released newer operation")
	}
	second.Reservation.Result <- false
	s.finishReadyStalls(false)
	s.exec(ReserveStallEnd{ID: 2, Reply: reply, Cancel: cancel})
	if r := <-reply; r.Reason == "" || s.players[2].Player.StallBusy {
		t.Fatal("cancelled prepare changed inventory")
	}
}

func TestStallShutdownWaitsForOutcome(t *testing.T) {
	s, _ := equipScene(t)
	reserved := reserveLifecycleStallDeal(t, s, nil)
	saver := newFakeSaver()
	s.saver = saver
	finished := make(chan struct{})
	go func() { s.shutdown("test"); close(finished) }()
	select {
	case <-finished:
		t.Fatal("shutdown did not wait for unknown outcome")
	case <-time.After(20 * time.Millisecond):
	}
	reserved.Reservation.Result <- false
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not resume")
	}
	if saver.lastBag[100].CountOf(场景药.ID) != 0 || saver.lastBag[200].CountOf(场景药.ID) != 2 {
		t.Fatal("shutdown saved unresolved candidate")
	}
	select {
	case <-reserved.Reservation.Done:
	default:
		t.Fatal("shutdown missed completion")
	}
}

func TestStallNoopEndRetainsFenceAndAbandonedReplyRollsBack(t *testing.T) {
	s, _ := equipScene(t)
	reply := make(chan StallReserveResult, 1)
	s.exec(ReserveStallEnd{ID: 1, Reply: reply})
	r := <-reply
	if !s.players[1].Player.StallBusy {
		t.Fatal("idempotent end did not retain ownership")
	}
	r.Reservation.Result <- true
	s.finishReadyStalls(false)
	s.exec(ReserveStallEnd{ID: 1})
	if s.players[1].Player.StallBusy || len(s.stallReservations) != 0 {
		t.Fatal("abandoned reply leaked ownership")
	}
}
