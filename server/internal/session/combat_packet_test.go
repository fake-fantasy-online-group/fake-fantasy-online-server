package session

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type combatQueue struct {
	out    chan []byte
	closed bool
}

func (q *combatQueue) Send(p []byte) {
	if q.closed {
		return
	}
	select {
	case q.out <- p:
	default:
		q.closed = true
	}
}
func (q *combatQueue) Close()         { q.closed = true }
func (*combatQueue) SessionID() int64 { return 1 }

func TestCombatPacketRetainedOrdering(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var events []event.Event
	for i := 0; i < 32; i++ {
		events = append(events, event.DamageDealt{Src: domain.EntityID(i + 1), Dst: 1000, Amount: int32(i), DstHP: int32(100 - i), Flag: event.DamageFlag(i), SkillID: int32(i % 2)})
		if i%4 == 0 {
			events = append(events, event.PlayerEmoted{Who: 3, Face: 4})
		}
	}
	observers := make([]eventSink, 3)
	queues := make([]*combatQueue, 3)
	for i := range observers {
		queues[i] = &combatQueue{out: make(chan []byte, len(events))}
		observers[i] = eventSink{out: queues[i], log: log, observer: domain.EntityID(i + 1)}
	}
	for _, ev := range events {
		for i := range observers {
			observers[i].Emit(ev)
		}
	}
	retained := make([][][]byte, 3)
	for i, q := range queues {
		for _, ev := range events {
			for _, want := range protocol.Encode(observers[i].observer, ev) {
				var got []byte
				select {
				case got = <-q.out:
				default:
					t.Fatalf("observer %d missing queued packet", i)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("observer %d packet mismatch", i)
				}
				retained[i] = append(retained[i], got)
			}
		}
		if len(q.out) != 0 || q.closed {
			t.Fatalf("observer %d: unexpected extra packet or queue closure", i)
		}
	}
	original := append([]byte(nil), retained[1][0]...)
	retained[0][0][0] ^= 255
	if !bytes.Equal(original, retained[1][0]) {
		t.Fatal("packet shared between observers")
	}
	for i := range observers {
		observers[i].Emit(event.DamageDealt{Src: 999, Dst: 1000, Amount: 1})
	}
	if !bytes.Equal(original, retained[1][0]) {
		t.Fatal("retained payload overwritten by next event")
	}
}

// Compare both accepted packets and closure against the generic encoder while
// interleaving drains, combat, and unrelated events on a bounded queue.
func TestCombatPacketQueueFullModelParity(t *testing.T) {
	for _, capacity := range []int{0, 1, 3, 8} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			actual := &combatQueue{out: make(chan []byte, capacity)}
			model := &combatQueue{out: make(chan []byte, capacity)}
			s := eventSink{out: actual, log: slog.New(slog.NewTextHandler(io.Discard, nil)), observer: 7}
			for i := 0; i < 40; i++ {
				var ev event.Event = event.DamageDealt{Src: domain.EntityID(i + 1), Dst: 1000, Amount: int32(i), DstHP: int32(100 - i), Flag: event.DamageFlag(i)}
				if i%5 == 0 {
					ev = event.PlayerEmoted{Who: 7, Face: 4}
				}
				s.Emit(ev)
				for _, pkt := range protocol.Encode(s.observer, ev) {
					model.Send(pkt)
				}
				if actual.closed != model.closed || len(actual.out) != len(model.out) {
					t.Fatalf("event %d: queue state differs", i)
				}
				if i%3 == 0 && len(model.out) > 0 {
					if !bytes.Equal(<-actual.out, <-model.out) {
						t.Fatalf("event %d: packet order/content differs", i)
					}
				}
			}
			for len(model.out) > 0 {
				if !bytes.Equal(<-actual.out, <-model.out) {
					t.Fatal("retained packet order/content differs")
				}
			}
		})
	}
}

func BenchmarkCombatPacketQueuedFanout(b *testing.B) {
	for _, n := range []int{1, 100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			sinks := make([]eventSink, n)
			queues := make([]*combatQueue, n)
			for i := range sinks {
				queues[i] = &combatQueue{out: make(chan []byte, 16)}
				sinks[i] = eventSink{out: queues[i], log: log, observer: domain.EntityID(i + 1)}
			}
			var ev event.Event = event.DamageDealt{Src: 999, Dst: 1000, Amount: 123, DstHP: 456}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for k := 0; k < 16; k++ {
					for j := range sinks {
						sinks[j].Emit(ev)
					}
				}
				for _, q := range queues {
					for k := 0; k < 16; k++ {
						<-q.out
					}
				}
			}
		})
	}
}

type combatTipStore struct {
	store.Store
	saved chan int32
}

func (s combatTipStore) MarkTipSeen(_ context.Context, charID int64, tip int32) error {
	if charID != 42 {
		panic("incorrect character for combat tip")
	}
	s.saved <- tip
	return nil
}

func combatQueuedPacket(t *testing.T, q *combatQueue) []byte {
	t.Helper()
	select {
	case p := <-q.out:
		return p
	default:
		t.Fatal("missing queued packet")
		return nil
	}
}

func TestCombatPacketSelfHitTipOrderingAndPersistence(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := &combatQueue{out: make(chan []byte, 8)}
	st := combatTipStore{saved: make(chan int32, 2)}
	sess := &Session{stage: StageInGame, char: &domain.Character{ID: 42}, log: log, deps: Deps{Store: st}}
	s := eventSink{out: q, log: log, observer: 7, sess: sess}
	for i, amount := range []int32{0, 123, 456} {
		ev := event.DamageDealt{Src: 9, Dst: 7, Amount: amount, DstHP: 1000 - amount}
		s.Emit(ev)
		if !bytes.Equal(combatQueuedPacket(t, q), protocol.Encode(7, ev)[0]) {
			t.Fatal("combat packet order/content differs")
		}
		if i == 1 {
			want := protocol.Encode(7, event.NewbieTipRequested{Who: 7, ID: 47})
			if len(want) != 1 || !bytes.Equal(combatQueuedPacket(t, q), want[0]) {
				t.Fatal("missing self-hit tip after combat")
			}
		}
		if len(q.out) != 0 || q.closed {
			t.Fatal("unexpected extra packet or queue closure")
		}
	}
	if _, ok := sess.seenTips[47]; !ok {
		t.Fatal("self-hit tip not claimed")
	}
	select {
	case tip := <-st.saved:
		if tip != 47 {
			t.Fatalf("persisted tip %d", tip)
		}
	case <-time.After(time.Second):
		t.Fatal("self-hit tip not persisted")
	}
	select {
	case <-st.saved:
		t.Fatal("tip persisted twice")
	default:
	}
}
