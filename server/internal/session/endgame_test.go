package session

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"testing"
)

func endgameHello() []byte {
	return protocol.NewW(0).U16(1).Str(`{"op":"hello","capabilities":["endgame.v1"]}`).Bytes()[2:]
}
func TestEndgameNegotiationRequiresWorld(t *testing.T) {
	out := &capSink{}
	s := New(out, Deps{})
	s.OnPacket(protocol.OpEndgame, endgameHello())
	if s.endgameEnabled() || len(out.pkts) != 0 {
		t.Fatal("preauth negotiation")
	}
	s.stage = StageInGame
	s.OnPacket(0x1091, protocol.NewW(0).I32(31).Bytes()[2:])
	if s.endgameEnabled() {
		t.Fatal("native capability changed")
	}
	s.OnPacket(protocol.OpEndgame, endgameHello())
	if !s.endgameEnabled() || len(out.pkts) != 1 {
		t.Fatal("hello not negotiated")
	}
	s.leaveWorld("test", StageAuthed)
	if s.endgameEnabled() || s.endgameV1 {
		t.Fatal("capability survived leave")
	}
}
func TestEndgameMapBarrierAndEventGate(t *testing.T) {
	out := &capSink{}
	s := New(out, Deps{})
	s.stage = StageInGame
	s.awaitMapAck = true
	s.OnPacket(protocol.OpEndgame, endgameHello())
	if s.endgameV1 {
		t.Fatal("mapload bypass")
	}
	sink := &eventSink{out: out, log: s.log, sess: s, observer: 1}
	sink.Emit(event.EndgameMessage{Who: 1, Message: domain.EndgameResponse{Kind: "snapshot", OK: true}})
	if len(out.pkts) != 0 {
		t.Fatal("unnegotiated event")
	}
	s.awaitMapAck = false
	s.OnPacket(protocol.OpEndgame, endgameHello())
	sink.Emit(event.EndgameMessage{Who: 1, Message: domain.EndgameResponse{Kind: "snapshot", OK: true}})
	if len(out.pkts) != 2 {
		t.Fatal("negotiated event missing")
	}
}
