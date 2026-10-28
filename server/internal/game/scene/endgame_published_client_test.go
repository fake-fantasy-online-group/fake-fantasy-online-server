package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"os"
	"testing"
)

func TestPublishedClientHelloNegotiatesSpiritAndCodex(t *testing.T) {
	b, err := os.ReadFile("../../protocol/testdata/client-1.0.6-hello.payload.bin")
	if err != nil {
		t.Fatal(err)
	}
	r, ok := protocol.DecodeEndgame(b)
	if !ok {
		t.Fatal("published client hello rejected before scene")
	}
	s, sink, _ := powerBookSceneFixture(t)
	p := s.players[1]
	p.Player.EndgamePowerCodex = false
	p.Player.EndgameSpirit = false
	s.exec(EndgameRequest{ID: p.ID, Request: r})
	s.finishEndgame(false)
	s.flush()
	if !p.Player.EndgamePowerCodex || !p.Player.EndgameSpirit {
		t.Fatal("actual hello failed to negotiate codex/spirit")
	}
	for _, ev := range sink.take() {
		m, ok := ev.(event.EndgameMessage)
		if !ok || m.Message.Kind != "hello" {
			continue
		}
		if !m.Message.OK || m.Message.Result["powerBook"] != "1" || len(m.Message.Spirits) != 9 {
			t.Fatalf("incomplete hello: %+v", m.Message)
		}
		caps := map[string]bool{}
		for _, c := range m.Message.Capabilities {
			caps[c] = true
		}
		if !caps[domain.EndgamePowerCodexV1] || !caps[domain.EndgameSpiritV1] {
			t.Fatal("missing negotiated capability echo")
		}
		return
	}
	t.Fatal("hello reply absent")
}
