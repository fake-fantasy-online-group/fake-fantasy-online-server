package session

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

type movementQueue struct {
	out    chan []byte
	closed bool
}

func (q *movementQueue) Send(p []byte) {
	if q.closed {
		return
	}
	select {
	case q.out <- p:
	default:
		q.closed = true
	}
}
func (q *movementQueue) Close()         { q.closed = true }
func (*movementQueue) SessionID() int64 { return 1 }

func TestMovementPacketRetainedOrdering(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var events []event.Event
	for i := 0; i < 32; i++ {
		events = append(events, event.EntityMoved{ID: domain.EntityID(i + 1), To: domain.Pos{X: float64(i * 100), Y: float64(-i)}, Speed: int32(i - 4), Snap: i%3 == 0, Stop: i%5 == 0})
		if i%4 == 0 {
			events = append(events, event.PlayerEmoted{Who: 3, Face: 4})
		}
	}
	observers := make([]eventSink, 3)
	queues := make([]*movementQueue, 3)
	for i := range observers {
		queues[i] = &movementQueue{out: make(chan []byte, len(events))}
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
		observers[i].Emit(event.EntityMoved{ID: 999})
	}
	if !bytes.Equal(original, retained[1][0]) {
		t.Fatal("retained payload overwritten by next event")
	}
}

// Compare both accepted packets and closure against the generic encoder while
// interleaving drains, movement, and unrelated events on a bounded queue.
func TestMovementPacketQueueFullModelParity(t *testing.T) {
	for _, capacity := range []int{0, 1, 3, 8} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			actual := &movementQueue{out: make(chan []byte, capacity)}
			model := &movementQueue{out: make(chan []byte, capacity)}
			s := eventSink{out: actual, log: slog.New(slog.NewTextHandler(io.Discard, nil)), observer: 7}
			for i := 0; i < 40; i++ {
				var ev event.Event = event.EntityMoved{ID: domain.EntityID(i + 1), To: domain.Pos{X: float64(i), Y: -1}, Speed: int32(i), Snap: i%3 == 0, Stop: i%4 == 0}
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

func BenchmarkMovementPacketQueuedFanout(b *testing.B) {
	for _, n := range []int{1, 100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			sinks := make([]eventSink, n)
			queues := make([]*movementQueue, n)
			for i := range sinks {
				queues[i] = &movementQueue{out: make(chan []byte, 16)}
				sinks[i] = eventSink{out: queues[i], log: log, observer: domain.EntityID(i + 1)}
			}
			var ev event.Event = event.EntityMoved{ID: 1000, To: domain.Pos{X: 123, Y: 456}, Speed: 200}
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
