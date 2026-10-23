package session

import (
	"bytes"
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"testing"
)

func TestNativeAttackRangeConnectionNegotiationAndLifecycle(t *testing.T) {
	ch := &domain.Character{Name: "range", Appear: domain.Appearance{AtkDist: 350}}
	events := []event.Event{event.SelfEntered{ID: 1, Char: ch, NativeAtkDist: 75}, event.AppearanceChanged{Who: 1, Appearance: ch.Appear, NativeAtkDist: 75}, event.EntitySpawned{ID: 2, Kind: domain.KindPlayer, Look: domain.Look{Appearance: ch.Appear}, NativeAtkDist: 75}}
	for _, caps := range []int32{0, 63, 127} {
		for _, ev := range events {
			out := &capSink{}
			s := New(out, Deps{})
			s.stage = StageAuthed
			s.OnPacket(0x1091, protocol.NewW(0).I32(caps).Bytes()[2:])
			sink := &eventSink{out: out, log: s.log, sess: s, observer: 1}
			sink.Emit(ev)
			var legacy []byte
			switch ev.(type) {
			case event.SelfEntered:
				legacy = protocol.EnterWorld(1, ch)
			default:
				legacy = protocol.Encode(1, ev)[0]
			}
			want := legacy
			if caps&64 != 0 {
				want = protocol.WithNativeAttackRange(legacy, ev)
			}
			found := false
			for _, p := range out.pkts {
				if len(p) > 1 && binary.LittleEndian.Uint16(p) == binary.LittleEndian.Uint16(legacy) {
					found = true
					if !bytes.Equal(p, want) {
						t.Fatalf("caps%d %T wrong tail", caps, ev)
					}
				}
			}
			if !found {
				t.Fatalf("missing %T", ev)
			}
			// Session routing/map rebind does not discard the connection capability.
			s.stage = StageInGame
			s.rebind(1, domain.SceneID{MapID: 2})
			s.OnPacket(0x1091, protocol.NewW(0).I32(caps ^ 64).Bytes()[2:])
			out.pkts = nil
			sink.Emit(events[1])
			want = protocol.Encode(1, events[1])[0]
			if caps&64 != 0 {
				want = protocol.WithNativeAttackRange(want, events[1])
			}
			if len(out.pkts) == 0 || !bytes.Equal(out.pkts[0], want) {
				t.Fatal("late capability changed established connection")
			}
		}
	}
	fresh := New(&capSink{}, Deps{})
	if fresh.nativeAttackRangeV1 {
		t.Fatal("reconnect inherited old capability")
	}
}
