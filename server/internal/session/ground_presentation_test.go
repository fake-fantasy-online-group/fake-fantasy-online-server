package session

import (
	"bytes"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"testing"
)

func TestGroundPresentationPerConnectionCapability(t *testing.T) {
	ev := event.EntitySpawned{ID: 77, Kind: domain.KindDrop, Ground: &event.GroundItemView{Item: 1601, Count: 1, Quality: 4, UniqueID: 1601, FreshDrop: true, DropToken: 999}}
	for _, caps := range []int32{0, 31, 63} {
		out := &capSink{}
		s := New(out, Deps{})
		s.stage = StageAuthed
		if caps != 0 {
			s.OnPacket(0x1091, protocol.NewW(0).I32(caps).Bytes()[2:])
		}
		sink := &eventSink{out: out, log: s.log, sess: s, observer: 1}
		sink.Emit(ev)
		want := protocol.Encode(1, ev)[0]
		if caps&32 != 0 {
			want = protocol.GroundSpawnPresentation(1, ev)
		}
		if len(out.pkts) != 1 || !bytes.Equal(out.pkts[0], want) {
			t.Fatalf("caps %d wrong wire", caps)
		}
	}
}
