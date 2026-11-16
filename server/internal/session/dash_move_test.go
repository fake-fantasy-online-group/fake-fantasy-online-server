package session

import (
	"bytes"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// Old clients (no dash capability) keep the legacy snap move, so a same-map
// dash never shows a black screen on either client generation.
func TestDashMovePerConnectionCapability(t *testing.T) {
	dash := event.EntityMoved{ID: 77, To: domain.Pos{X: 300, Y: 120}, Dir: 3, Stop: true, Snap: true, Dash: true, DurationMS: 200}
	plain := dash
	plain.Dash = false
	for _, caps := range []int32{0, 63, 63 | protocol.ClientCapDashMoveV1} {
		for _, ev := range []event.EntityMoved{dash, plain} {
			out := &capSink{}
			s := New(out, Deps{})
			s.stage = StageAuthed
			if caps != 0 {
				s.OnPacket(0x1091, protocol.NewW(0).I32(caps).Bytes()[2:])
			}
			sink := &eventSink{out: out, log: s.log, sess: s, observer: 1}
			sink.Emit(ev)
			want := protocol.EntityMove(ev)
			if ev.Dash && caps&protocol.ClientCapDashMoveV1 != 0 {
				want = protocol.EntityDash(ev)
			}
			if len(out.pkts) != 1 || !bytes.Equal(out.pkts[0], want) {
				t.Fatalf("caps %d dash %v wrong wire % x", caps, ev.Dash, out.pkts)
			}
		}
	}
}
