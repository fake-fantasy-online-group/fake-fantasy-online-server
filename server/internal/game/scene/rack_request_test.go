package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func rackRequestScene(t *testing.T) *Scene {
	s, _ := equipScene(t)
	s.rack = domain.NewRackCatalog([]domain.RackCategory{{ID: 0, Name: "test"}}, []domain.RackGood{{Item: 场景药.ID, Price: 10, Quantity: 1, Remain: -1}})
	s.players[1].Player.Char.Caiyu = 100
	s.onOpenRack(OpenRack{ID: 1})
	return s
}
func TestRackCancelledBeforePrepareDoesNotCharge(t *testing.T) {
	s := rackRequestScene(t)
	r := &RackRequest{}
	r.Cancel()
	ch := make(chan RackReserveResult, 1)
	s.onReserveRackPurchase(ReserveRackPurchase{ID: 1, Item: 场景药.ID, Count: 1, Reply: ch, Request: r})
	p := s.players[1].Player
	if p.Char.Caiyu != 100 || p.Bag.CountOf(场景药.ID) != 0 || p.RackBusy {
		t.Fatal("cancelled prepare changed ownership")
	}
	select {
	case <-ch:
		t.Fatal("cancelled owner received late result")
	default:
	}
}
func TestRackCancellationAtPublicationRollsBack(t *testing.T) {
	for _, refund := range []bool{false, true} {
		t.Run(map[bool]string{false: "purchase", true: "refund"}[refund], func(t *testing.T) {
			s := rackRequestScene(t)
			p := s.players[1].Player
			ch := make(chan RackReserveResult, 1)
			want := int32(0)
			if refund {
				p.Bag.Add(场景药, 2)
				want = 2
				s.onReserveRackRefund(ReserveRackRefund{ID: 1, Offer: domain.RackRefundOffer{Item: 场景药.ID, UnitPrice: 10, Bundle: 1, RemainingShares: 2}, Shares: 1, Credit: 8, Reply: ch})
			} else {
				s.onReserveRackPurchase(ReserveRackPurchase{ID: 1, Item: 场景药.ID, Count: 1, Reply: ch})
			}
			prepared := <-ch
			if prepared.Reservation.ID == 0 || !p.RackBusy {
				t.Fatal("not prepared")
			}
			r := &RackRequest{}
			r.Cancel()
			s.rackReply(ch, prepared, r)
			if p.Char.Caiyu != 100 || p.Bag.CountOf(场景药.ID) != want || p.RackBusy {
				t.Fatal("late result left charge/items/busy state")
			}
			s.onReserveRackPurchase(ReserveRackPurchase{ID: 1, Item: 场景药.ID, Count: 1, Reply: ch, Request: &RackRequest{}})
			retry := <-ch
			if retry.Reservation.ID == 0 || retry.Reason != "" {
				t.Fatalf("retry blocked: %+v", retry)
			}
			s.onFinalizeRackReservation(FinalizeRackReservation{Reservation: retry.Reservation})
		})
	}
}
func TestRackPublicationWinsTimeoutAndMissingConsumerRollsBack(t *testing.T) {
	s := rackRequestScene(t)
	r := &RackRequest{}
	ch := make(chan RackReserveResult, 1)
	s.onReserveRackPurchase(ReserveRackPurchase{ID: 1, Item: 场景药.ID, Count: 1, Reply: ch, Request: r})
	if r.Cancel() {
		t.Fatal("timeout stole published ownership")
	}
	v := <-ch
	if !s.players[1].Player.RackBusy || v.Snapshot.Char == nil {
		t.Fatal("lost candidate")
	}
	s.onFinalizeRackReservation(FinalizeRackReservation{Reservation: v.Reservation, Commit: true})
	s.onReserveRackPurchase(ReserveRackPurchase{ID: 1, Item: 场景药.ID, Count: 1})
	p := s.players[1].Player
	if p.Char.Caiyu != 90 || p.RackBusy || p.Bag.CountOf(场景药.ID) != 1 {
		t.Fatal("missing consumer orphaned candidate")
	}
}
func TestRackHandoffHasExactlyOneOwner(t *testing.T) {
	for i := 0; i < 1000; i++ {
		r := &RackRequest{}
		ch := make(chan bool, 1)
		go func() { ch <- r.claim() }()
		cancelled := r.Cancel()
		published := <-ch
		if cancelled == published {
			t.Fatal("handoff must have exactly one owner")
		}
	}
}

func TestRackPetCancellationRestoresCurrencyAndPets(t *testing.T) {
	s := rackRequestScene(t)
	def := domain.PetDef{ID: 999, Name: "rack pet", Model: 100, RealPet: true, InitHP: 100, MaxLevel: 100}
	s.petDefs = domain.PetTable{def.ID: def}
	s.items[90000999] = domain.ItemDef{ID: 90000999, Name: "carrier", InventoryTab: 3, InventoryTabKnown: true, InstanceKind: domain.ItemInstancePet, PetCarrierSpecies: def.ID}
	s.rack = domain.NewRackCatalog([]domain.RackCategory{{ID: 0, Name: "pets"}}, []domain.RackGood{{Item: 90000999, Pet: def.ID, Price: 10, Quantity: 1, Remain: -1}})
	reply := make(chan RackReserveResult, 1)
	s.onReserveRackPurchase(ReserveRackPurchase{ID: 1, Item: 90000999, Count: 1, Reply: reply})
	prepared := <-reply
	p := s.players[1].Player
	if prepared.Reservation.ID == 0 || len(p.Char.Pets) != 1 || len(prepared.Reservation.AddedPets) != 1 {
		t.Fatalf("pet not prepared: %+v", prepared)
	}
	request := &RackRequest{}
	request.Cancel()
	s.rackReply(reply, prepared, request)
	if p.Char.Caiyu != 100 || len(p.Char.Pets) != 0 || p.Bag.CountOf(90000999) != 0 || p.RackBusy {
		t.Fatal("pet cancellation did not restore ownership")
	}
}
