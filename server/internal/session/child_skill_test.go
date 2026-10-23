package session

import (
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"testing"
)

func TestChildSkillReleaseConnectionCapability(t *testing.T) {
	for _, caps := range []int32{0, 127, 255} {
		out := &capSink{}
		s := New(out, Deps{})
		s.stage = StageAuthed
		s.OnPacket(0x1091, protocol.NewW(0).I32(caps).Bytes()[2:])
		sink := &eventSink{out: out, log: s.log, sess: s, observer: 1}
		ev := event.ChildSkillReleased{Caster: 1, Skill: 10301, Target: 2, Origin: domain.Pos{MapID: 3, X: 100, Y: 200}, Center: domain.Pos{MapID: 3, X: 300, Y: 400}}
		s.stage = StageInGame
		s.OnPacket(0x1091, protocol.NewW(0).I32(caps ^ 128).Bytes()[2:])
		sink.Emit(ev)
		want := 0
		if caps&128 != 0 {
			want = 1
		}
		if len(out.pkts) != want {
			t.Fatalf("caps%d packets%d", caps, len(out.pkts))
		}
		if want == 1 {
			p := out.pkts[0]
			if len(p) != 35 || binary.LittleEndian.Uint16(p) != 0x80F2 {
				t.Fatalf("bad packet %x", p)
			}
			if p[34] != 0 {
				t.Fatal("release mode")
			}
			for i, n := range []int32{1, 10301, 2, 3, 100, 200, 300, 400} {
				if int32(binary.LittleEndian.Uint32(p[2+i*4:])) != n {
					t.Fatalf("field%d", i)
				}
			}
		}
		out.pkts = nil
		ev.Mode = 1
		sink.Emit(ev)
		if want == 1 && (len(out.pkts) != 1 || out.pkts[0][34] != 1) {
			t.Fatal("impact mode missing")
		}
		out.pkts = nil
		ev.Mode = 2
		sink.Emit(ev)
		if len(out.pkts) != 0 {
			t.Fatal("invalid mode encoded")
		}
	}
	if New(&capSink{}, Deps{}).childSkillVisualV1 {
		t.Fatal("fresh connection inherited capability")
	}
}
