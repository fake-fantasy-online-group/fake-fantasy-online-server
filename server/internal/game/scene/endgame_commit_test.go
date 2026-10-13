package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestEndgameRecoveryWinsAndPreservesCharacterPointer(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "unflagged_authoritative", true: "stale_revision"}[stale], func(t *testing.T) {
			s := newTestScene(t, nil)
			sink := join(t, s, 1, 100, "甲", 100, 100)
			p := s.players[1]
			shared := p.Player.Char
			x := s.endgamePlayer(p)
			x.loaded = true
			x.candidate = s.snapshotOf(p)
			x.candidate.Char.Caiyu = 999
			authoritative := s.snapshotOf(p)
			authoritative.Char.Caiyu = 77
			after, failed := 0, 0
			x.after = func() { after++ }
			x.onFailure = func() { failed++ }
			p.Player.TradeBusy = true
			r := domain.EndgameCommitResult{State: domain.EndgameState{Revision: 3, Payload: "{}"}, Authoritative: &authoritative}
			if stale {
				r.Err = domain.ErrEndgameRevision
				r.RequiresReload = true
			}
			s.finishEndgameCommit(p, x, r)
			s.flush()
			_ = sink.take()
			if p.Player.Char != shared || shared.Caiyu != 77 || x.state.Revision != 3 || after != 0 || failed != 1 || p.Player.TradeBusy {
				t.Fatal("recovery replaced pointer, published candidate, or lost state")
			}
		})
	}
}
func TestEndgameRecoveredReplayReturnsReceipt(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.flush()
	_ = sink.take()
	p := s.players[1]
	x := s.endgamePlayer(p)
	x.loaded = true
	x.pending = domain.EndgameRequest{Op: "cube.execute", RequestID: "request-000000001"}
	a := s.snapshotOf(p)
	p.Player.TradeBusy = true
	s.finishEndgameCommit(p, x, domain.EndgameCommitResult{State: domain.EndgameState{Revision: 5, Payload: "{}"}, Replayed: true, RequiresReload: true, Authoritative: &a, Receipt: domain.EndgameReceipt{Response: `{"kind":"cube.result","requestId":"request-000000001","ok":true,"revision":"2"}`}})
	s.flush()
	found := false
	for _, ev := range sink.take() {
		if msg, ok := ev.(event.EndgameMessage); ok && msg.Message.RequestID == x.pending.RequestID {
			if !msg.Message.OK || msg.Message.Revision != "2" {
				t.Fatalf("replay changed receipt: %+v", msg.Message)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing replay receipt")
	}
}
func TestEndgameCommitRechecksAsyncRevision(t *testing.T) {
	s := newTestScene(t, nil)
	_ = join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	s.endgamePlayer(p).state.Revision = 2
	called := false
	if s.endgameCommitPayload(p, domain.EndgameRequest{Op: "rift.claim", ExpectedRevision: "1"}, s.snapshotOf(p), domain.EndgameResponse{}, nil, 0, func() { called = true }, "{}") {
		t.Fatal("accepted stale asynchronous plan")
	}
	if p.Player.TradeBusy || called {
		t.Fatal("stale plan acquired lease")
	}
}
