package scene

import (
	"bytes"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

type memoryPayloadEvent struct{ data []byte }

func (*memoryPayloadEvent) Subject() domain.EntityID { return 0 }

func TestFlushReleasesOutboxReferencesPreservesSinkPayloads(t *testing.T) {
	first, second := &fakeSink{}, &fakeSink{}
	s := &Scene{players: map[domain.EntityID]*entity.Entity{
		1: {ID: 1, Player: &entity.Player{Sink: first}},
		2: {ID: 2, Player: &entity.Player{Sink: second}},
	}}
	payload := &memoryPayloadEvent{data: bytes.Repeat([]byte{0x7a}, 4096)}
	s.push(payload, 0, 0) // both recipients
	s.push(payload, 0, 2) // excluded recipient
	s.emitTo(2, payload)
	s.emitTo(999, payload) // departed recipient must also be released
	backing := s.outbox
	capacity := cap(backing)
	s.flush()
	if len(s.outbox) != 0 || cap(s.outbox) != capacity {
		t.Fatal("outbox buffer was not reused")
	}
	for i, p := range backing {
		if p != (pending{}) {
			t.Fatalf("slot %d still retains an event or routing data", i)
		}
	}
	s.emitTo(1, &memoryPayloadEvent{data: []byte("next frame")})
	s.flush()
	a, b := first.take(), second.take()
	if len(a) != 3 || len(b) != 2 {
		t.Fatalf("delivery counts %d/%d", len(a), len(b))
	}
	for _, ev := range append(a[:2], b...) {
		if ev != payload {
			t.Fatal("sink event identity changed")
		}
	}
	if !bytes.Equal(payload.data, bytes.Repeat([]byte{0x7a}, 4096)) {
		t.Fatal("retained sink payload changed")
	}
}
