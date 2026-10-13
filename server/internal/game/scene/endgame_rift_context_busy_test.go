package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// Characterize the fresh-connection gap without manufacturing a public/private
// flag: admission, instance identity, and SelfEntered all come from the seeded
// paid run and the same Enter path used by existing reconnect tests. A busy
// Session hello cannot repair this gap because it never reaches this actor.
func TestRiftFreshReconnectHasNoContextUntilSuccessfulHello(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	prior := s.players[1]
	snapshot := s.snapshotOf(prior).Clone()
	s.exec(Leave{ID: prior.ID, Reason: "fresh context regression"})
	sink := &fakeSink{}
	p := reconnectRiftPlayer(t, s, snapshot, 900000, 0, nil, sink)
	s.flush()
	before := sink.take()
	if s.rift == nil || s.id.Persistent() || !s.rift.data.Run.Admitted(domain.CharID(p.Player.Char.ID)) || !s.rift.ready[domain.CharID(p.Player.Char.ID)] {
		t.Fatal("reconnect fixture did not enter its admitted private run")
	}
	if p.Player.EndgameRiftPortals {
		t.Fatal("fresh Runtime=nil reconnect unexpectedly inherited the portal capability")
	}
	entered, ok := firstOf[event.SelfEntered](before)
	if !ok || entered.Scene != s.id || entered.Char.Pos.MapID != s.id.MapID {
		t.Fatalf("missing authoritative private SelfEntered: %+v", entered)
	}
	for _, ev := range before {
		if message, ok := ev.(event.EndgameMessage); ok && message.Message.Kind == "rift.context" {
			t.Fatal("fresh unnegotiated reconnect unexpectedly sent a context")
		}
	}
	if dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(entered, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fresh-reconnect-self-entered.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	// The positive control uses the real command and backend completion path,
	// not dispatchEndgame or a manually supplied rift.context response.
	s.exec(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{
		Op: "hello", RequestID: "rift-reconnect-after-busy",
		Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPortalsV1},
	}})
	s.finishEndgame(false)
	s.flush()
	after := sink.take()
	hello, context, state := -1, -1, -1
	for i, ev := range after {
		message, ok := ev.(event.EndgameMessage)
		if !ok {
			continue
		}
		switch message.Message.Kind {
		case "hello":
			if !message.Message.OK {
				t.Fatalf("positive control hello failed: %+v", message.Message)
			}
			hello = i
		case "rift.context":
			if !message.Message.OK || message.Message.Result["inInstance"] != "true" {
				t.Fatalf("actor context did not identify private instance: %+v", message.Message)
			}
			context = i
		case "snapshot":
			if state < 0 {
				state = i
			}
		}
		if dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR"); dir != "" && (message.Message.Kind == "hello" || message.Message.Kind == "rift.context") {
			packet := protocol.EncodeEndgame(message.Message)
			for name, content := range map[string][]byte{message.Message.Kind + "-success.bin": packet, message.Message.Kind + "-success.payload.bin": packet[2:]} {
				if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if !p.Player.EndgameRiftPortals || hello < 0 || context <= hello || state <= context {
		t.Fatalf("successful hello must establish context before snapshot: hello=%d context=%d snapshot=%d", hello, context, state)
	}
	t.Logf("real admitted reconnect scene=%v emitted SelfEntered with no rift.context; successful actor hello later emits hello[%d], private context[%d], snapshot[%d]", entered.Scene, hello, context, state)
}

type heldRiftContextBackend struct {
	*riftFloorTestBackend
	load  chan domain.EndgameLoadResult
	calls int
}

func (b *heldRiftContextBackend) Load(int64) <-chan domain.EndgameLoadResult {
	b.calls++
	return b.load
}

func TestRiftFreshReconnectDelayedLoadRepliesPendingOnRetries(t *testing.T) {
	s, backend, _ := seededRiftFloorFixture(t)
	prior := s.players[1]
	snapshot := s.snapshotOf(prior).Clone()
	s.exec(Leave{ID: prior.ID, Reason: "delayed hello load regression"})
	sink := &fakeSink{}
	p := reconnectRiftPlayer(t, s, snapshot, 900000, 0, nil, sink)
	s.flush()
	sink.take()
	held := &heldRiftContextBackend{riftFloorTestBackend: backend, load: make(chan domain.EndgameLoadResult, 1)}
	s.endgameBackend = held
	for attempt := 1; attempt <= 3; attempt++ {
		req := domain.EndgameRequest{Op: "hello", RequestID: fmt.Sprintf("rift-reconnect-pending-%d", attempt), Capabilities: []string{domain.EndgameV1, domain.EndgameRiftPortalsV1}}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		decoded, ok := protocol.DecodeEndgame(protocol.NewW(0).U16(domain.EndgameSchema).Str(string(data)).Bytes()[2:])
		if !ok {
			t.Fatal("real decoder rejected hello")
		}
		s.exec(EndgameRequest{ID: p.ID, Request: decoded})
		s.finishEndgame(false)
		s.flush()
		events := sink.take()
		if attempt == 1 {
			if len(events) != 0 {
				t.Fatalf("initial accepted hello should wait silently for Load: %+v", events)
			}
			t.Log("hello attempt 1: accepted, zero events while backend Load channel is unresolved")
			continue
		}
		if len(events) != 1 {
			t.Fatalf("hello %d must return one pending response, got %+v", attempt, events)
		}
		message, ok := events[0].(event.EndgameMessage)
		if !ok || message.Message.Kind != "hello" || message.Message.OK || message.Message.RequestID != req.RequestID || message.Message.Error != "pending" || len(message.Message.Capabilities) != 0 || len(message.Message.Result) != 0 {
			t.Fatalf("retry did not disclose active pending negotiation: %+v", events[0])
		}
		if dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR"); dir != "" {
			packet := protocol.EncodeEndgame(message.Message)
			for name, content := range map[string][]byte{
				fmt.Sprintf("hello-pending-%d.bin", attempt):         packet,
				fmt.Sprintf("hello-pending-%d.payload.bin", attempt): packet[2:],
			} {
				if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Logf("hello attempt %d: one hello error=pending, no capabilities or context", attempt)
	}
	if held.calls != 1 || p.Player.EndgameRiftPortals || s.endgamePlayer(p).loaded {
		t.Fatalf("retry restarted Load or inferred ownership: loads=%d", held.calls)
	}
	held.load <- domain.EndgameLoadResult{}
	s.finishEndgame(false)
	s.flush()
	events := sink.take()
	if len(events) < 3 {
		t.Fatalf("released load did not resolve initial hello: %+v", events)
	}
	for i, kind := range []string{"hello", "rift.context", "snapshot"} {
		message, ok := events[i].(event.EndgameMessage)
		if !ok || message.Message.Kind != kind || !message.Message.OK {
			t.Fatalf("released load FIFO[%d] is not successful %s: %+v", i, kind, events[i])
		}
		if i == 0 && message.Message.RequestID != "rift-reconnect-pending-1" {
			t.Fatal("retry replaced the original pending hello")
		}
		if i == 1 && message.Message.Result["inInstance"] != "true" {
			t.Fatal("released load did not establish true private ownership")
		}
		if dir := os.Getenv("RIFT_CONTEXT_BUSY_FIXTURE_DIR"); dir != "" && i < 2 {
			packet := protocol.EncodeEndgame(message.Message)
			if err := os.WriteFile(filepath.Join(dir, "delayed-"+kind+"-success.payload.bin"), packet[2:], 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Log("released Load: successful original hello, private rift.context, then snapshot")
}
