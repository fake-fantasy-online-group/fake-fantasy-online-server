package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"os"
	"testing"
)

func TestRuneStackNativeSocketAndSplit(t *testing.T) {
	s, sink, b := astralSceneFixture(t)
	p := s.players[1]
	rune := p.Player.Bag.At(100)
	rune.Count = 3
	p.Player.Bag.Set(100, rune)
	inv, err := s.inventorySnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	s.sendEndgameSnapshot(p)
	messages := endgameMessages(s, sink)
	if path := os.Getenv("RUNE_STACK_FIXTURE"); path != "" {
		packets := protocol.Encode(1, inv)
		for i, pkt := range packets {
			if i == 0 {
				if err := os.WriteFile(path+".inventory.bin", pkt, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		raw, _ := json.MarshalIndent(struct {
			Inventory any
			Endgame   any
		}{inv, messages}, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	nativeAstralPreview(t, s)
	s.exec(nativeAstralCommand(t, 0, 100))
	s.finishEndgame(false)
	if b.commits != 1 {
		t.Fatal("no candidate", b.commits)
	}
	candidate := b.plan.Snapshot.Bag
	if candidate.At(100).Count != 2 || candidate.At(100).UID != rune.UID || candidate.At(200).SocketUIDs[0] == rune.UID {
		t.Fatal("stack socket identity", candidate.At(100), candidate.At(200))
	}
	nativeAstralSuccess(s, b)
	// A second hole takes one more, keeping the bag stack and both socket IDs distinct.
	nativeAstralPreview(t, s)
	s.exec(nativeAstralCommand(t, 1, 100))
	s.finishEndgame(false)
	if b.commits != 2 {
		t.Fatal("second candidate", b.commits)
	}
	nativeAstralSuccess(s, b)
	gear := p.Player.Bag.At(200)
	if p.Player.Bag.At(100).Count != 1 || gear.SocketUIDs[0] == gear.SocketUIDs[1] || gear.SocketUIDs[1] == rune.UID {
		t.Fatal("repeated insert duplicated UID")
	}
}
func TestRuneStackBagSplitAndWarehouseTransfer(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	r := p.Player.Bag.At(100)
	r.Count = 5
	p.Player.Bag.Set(100, r)
	s.exec(SplitBagItem{ID: 1, Tab: 1, Slot: 100, Count: 2})
	a, b := p.Player.Bag.At(100), p.Player.Bag.At(101)
	if a.Count != 3 || b.Count != 2 || a.UID != r.UID || b.UID == r.UID || a.Card != b.Card {
		t.Fatal("bag split", a, b)
	}
	rem, moved, ok := removeStackCount(a, 1)
	if !ok || rem.Count != 2 || moved.Count != 1 || rem.UID == moved.UID {
		t.Fatal("warehouse partial", rem, moved)
	}
	d := s.items[r.Item]
	if !putBag(p.Player.Bag, d, 101, moved) || p.Player.Bag.At(101).Count != 3 {
		t.Fatal("warehouse merge")
	}
}
func TestRuneStackTradeAndMailPartialIdentity(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	r := p.Player.Bag.At(100)
	r.Count = 5
	p.Player.Bag.Set(100, r)
	d := s.items[r.Item]
	d.CanTrade = true
	d.CanMail = true
	s.items[r.Item] = d
	quote := r
	quote.Count = 2
	stacks, reason := s.validateTradeSettlementOffer(p, TradeSettlementOffer{Items: []TradeOfferItem{{Tab: 1, Slot: 100, Stack: quote}}})
	if reason != "" || len(stacks) != 1 || stacks[0].Count != 2 || stacks[0].UID == r.UID || stacks[0].Card != r.Card || p.Player.Bag.At(100) != r {
		t.Fatal("trade identity", stacks, reason)
	}
	reply := make(chan MailReserveResult, 1)
	s.exec(ReserveMailSend{ID: 1, OperationID: "rune-mail", Items: []MailItemRef{{Tab: 1, Slot: 100, Count: 2}}, Reply: reply})
	result := <-reply
	if result.Reason != "" || len(result.Attachments) != 1 {
		t.Fatal("mail reject", result.Reason)
	}
	attach := result.Attachments[0].Stack
	if attach.Count != 2 || attach.UID == r.UID || attach.Card != r.Card || p.Player.Bag.At(100).Count != 3 || p.Player.Bag.At(100).UID != r.UID {
		t.Fatal("mail identity", attach)
	}
	result.Reservation.Result <- false
	s.finishReadyMail(false)
	if p.Player.Bag.At(100) != r {
		t.Fatal("mail rollback")
	}
}
func TestRuneStackLockSynchronizesSnapshot(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	r := p.Player.Bag.At(100)
	r.Count = 3
	r.Card.Locked = true
	p.Player.Bag.Set(100, r)
	s.exec(SetItemLock{ID: 1, Tab: 1, Slot: 100, Item: r.Item, On: false})
	if p.Player.Bag.At(100).Locked || p.Player.Bag.At(100).Card.Locked {
		t.Fatal("legacy snapshot lock not cleared")
	}
	s.exec(SetItemLock{ID: 1, Tab: 1, Slot: 100, Item: r.Item, On: true})
	if !p.Player.Bag.At(100).Locked || !p.Player.Bag.At(100).Card.Locked {
		t.Fatal("stack lock not synchronized")
	}
}
func TestRuneStackSnapshotProtectionBlocksTransfers(t *testing.T) {
	for _, bound := range []bool{false, true} {
		s, _, _ := astralSceneFixture(t)
		p := s.players[1]
		r := p.Player.Bag.At(100)
		r.Count = 3
		r.Card.Bound = bound
		r.Card.Locked = !bound
		p.Player.Bag.Set(100, r)
		d := s.items[r.Item]
		d.CanTrade = true
		d.CanMail = true
		s.items[r.Item] = d
		quote := r
		quote.Count = 1
		if _, reason := s.validateTradeSettlementOffer(p, TradeSettlementOffer{Items: []TradeOfferItem{{Tab: 1, Slot: 100, Stack: quote}}}); reason == "" {
			t.Fatal("protected snapshot traded")
		}
		reply := make(chan MailReserveResult, 1)
		s.exec(ReserveMailSend{ID: 1, OperationID: "protected-rune", Items: []MailItemRef{{Tab: 1, Slot: 100, Count: 1}}, Reply: reply})
		if result := <-reply; result.Reason == "" || p.Player.Bag.At(100) != r {
			t.Fatal("protected snapshot mailed")
		}
	}
}
