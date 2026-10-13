package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
	"time"
)

type dispatchEndgameBackend struct {
	load                     chan domain.EndgameLoadResult
	receipt                  chan domain.EndgameReceiptLoadResult
	commit                   chan domain.EndgameCommitResult
	plan                     domain.EndgameCommitPlan
	loads, receipts, commits int
}

func (b *dispatchEndgameBackend) Load(int64) <-chan domain.EndgameLoadResult {
	b.loads++
	return b.load
}
func (b *dispatchEndgameBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	b.receipts++
	return b.receipt
}
func (b *dispatchEndgameBackend) LoadRun(string) <-chan domain.EndgameRunLoadResult { return nil }
func (b *dispatchEndgameBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	b.commits++
	b.plan = p
	return b.commit
}
func newDispatchBackend() *dispatchEndgameBackend {
	return &dispatchEndgameBackend{load: make(chan domain.EndgameLoadResult, 1), receipt: make(chan domain.EndgameReceiptLoadResult, 1), commit: make(chan domain.EndgameCommitResult, 1)}
}
func dispatchMessages(s *Scene, sink *fakeSink) []domain.EndgameResponse {
	s.flush()
	var out []domain.EndgameResponse
	for _, e := range sink.take() {
		if e, ok := e.(event.EndgameMessage); ok {
			out = append(out, e.Message)
		}
	}
	return out
}
func TestEndgameReconnectReplayPrecedesOwnershipAndRevision(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	b := newDispatchBackend()
	s.endgameBackend = b
	dispatchMessages(s, sink)
	req := domain.EndgameRequest{Op: "socket.insert", RequestID: "reconnect-0000001", ExpectedRevision: "0", TargetUID: "99999", RuneUID: "88888"}
	s.onEndgame(EndgameRequest{ID: 1, Request: req})
	if b.loads != 1 || b.receipts != 0 {
		t.Fatal("not lazy")
	}
	b.load <- domain.EndgameLoadResult{State: domain.EndgameState{Revision: 7, Payload: "{}"}}
	s.finishEndgame(false)
	if b.receipts != 1 {
		t.Fatal("revision checked before receipt")
	}
	s.players[1].Player.MailBusy = true // Other current ownership/affordability is irrelevant to an old receipt.
	b.receipt <- domain.EndgameReceiptLoadResult{Receipt: &domain.EndgameReceipt{CharID: 100, RequestID: req.RequestID, Operation: req.Op, Digest: endgameDigest(req), Revision: 1, Response: `{"kind":"socket.insert","requestId":"reconnect-0000001","ok":true,"revision":"1"}`}}
	s.finishEndgame(false)
	msgs := dispatchMessages(s, sink)
	if len(msgs) != 1 || !msgs[0].OK || msgs[0].Revision != "1" || b.commits != 0 {
		t.Fatalf("replay revalidated/changed result: %+v", msgs)
	}
}
func TestEndgameReceiptConflictAndLookupFailureDoNotExecute(t *testing.T) {
	for _, mode := range []string{"digest", "owner", "storage"} {
		t.Run(mode, func(t *testing.T) {
			s := newTestScene(t, nil)
			sink := join(t, s, 1, 100, "甲", 100, 100)
			b := newDispatchBackend()
			s.endgameBackend = b
			s.endgamePlayer(s.players[1]).loaded = true
			dispatchMessages(s, sink)
			req := domain.EndgameRequest{Op: "socket.insert", RequestID: "reconnect-0000001", ExpectedRevision: "0"}
			s.onEndgame(EndgameRequest{ID: 1, Request: req})
			r := domain.EndgameReceiptLoadResult{Receipt: &domain.EndgameReceipt{CharID: 100, RequestID: req.RequestID, Operation: req.Op, Digest: endgameDigest(req), Response: `{"ok":true}`}}
			switch mode {
			case "digest":
				r.Receipt.Digest = "different"
			case "owner":
				r.Receipt.CharID = 101
			case "storage":
				r.Err = errors.New("offline")
			}
			b.receipt <- r
			s.finishEndgame(false)
			msgs := dispatchMessages(s, sink)
			if len(msgs) != 1 || msgs[0].OK || b.commits != 0 || s.players[1].Player.TradeBusy {
				t.Fatalf("receipt error executed: %+v", msgs)
			}
		})
	}
}
func TestEndgameDisconnectDiscardsReadOnlyPendingState(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		t.Run(map[bool]string{false: "lazy_load", true: "receipt_lookup"}[loaded], func(t *testing.T) {
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "甲", 100, 100)
			b := newDispatchBackend()
			s.endgameBackend = b
			s.endgamePlayer(s.players[1]).loaded = loaded
			s.onEndgame(EndgameRequest{ID: 1, Request: domain.EndgameRequest{Op: "socket.remove", RequestID: "disconnect-000001", ExpectedRevision: "0"}})
			s.exec(Leave{ID: 1})
			s.finishEndgame(false)
			if s.endgamePlayers[1] != nil || b.commits != 0 {
				t.Fatal("read-only state leaked after disconnect")
			}
		})
	}
}
func TestEndgameDisconnectWaitsForDurableResultAndDeferredReward(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[success], func(t *testing.T) {
			saver := newFakeSaver()
			s := newTestScene(t, saver)
			join(t, s, 1, 100, "甲", 100, 100)
			p := s.players[1]
			b := newDispatchBackend()
			s.endgameBackend = b
			s.endgamePlayer(p).loaded = true
			p.Player.Char.Caiyu = 100
			candidate := s.snapshotOf(p)
			candidate.Char.Caiyu = 90
			req := domain.EndgameRequest{Op: "cube.execute", RequestID: "disconnect-000001", ExpectedRevision: "0"}
			if !s.endgameCommit(p, req, candidate, domain.EndgameResponse{}, nil, 0, nil) {
				t.Fatal("no commit")
			}
			p.Player.MarkDirty()
			s.saveDirty()
			s.exec(Leave{ID: 1, FinalSaver: saver})
			s.deferTradeMutation(1, func() { p.Player.Bag.Set(0, domain.Stack{Item: 42, Count: 2}); p.Player.MarkDirty() })
			if s.players[1] == nil || saver.count(100) != 0 {
				t.Fatal("left or autosaved before outcome")
			}
			if success {
				b.commit <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			} else {
				b.commit <- domain.EndgameCommitResult{Err: errors.New("rolled back")}
			}
			s.finishEndgame(false)
			want := int64(100)
			if success {
				want = 90
			}
			if s.players[1] != nil || saver.last[100].Caiyu != want || saver.lastBag[100].At(0).Count != 2 {
				t.Fatal("final save lost result/deferred reward")
			}
		})
	}
}
func TestEndgameShutdownWaitsForCommit(t *testing.T) {
	saver := newFakeSaver()
	s := newTestScene(t, saver)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	b := newDispatchBackend()
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	candidate := s.snapshotOf(p)
	candidate.Char.Caiyu = 75
	s.endgameCommit(p, domain.EndgameRequest{Op: "cube.execute", RequestID: "shutdown-00000001", ExpectedRevision: "0"}, candidate, domain.EndgameResponse{}, nil, 0, nil)
	state, receipt := b.plan.State, b.plan.Receipt
	go func() {
		time.Sleep(20 * time.Millisecond)
		b.commit <- domain.EndgameCommitResult{State: state, Receipt: receipt}
	}()
	start := time.Now()
	s.shutdown("test")
	if time.Since(start) < 15*time.Millisecond || saver.count(100) == 0 || saver.last[100].Caiyu != 75 {
		t.Fatal("shutdown did not drain commit")
	}
}
