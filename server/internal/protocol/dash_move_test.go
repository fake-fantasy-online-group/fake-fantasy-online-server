package protocol

import (
	"encoding/binary"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestEntityDashLayout(t *testing.T) {
	b := EntityDash(event.EntityMoved{ID: 0x01020304, To: domain.Pos{X: 300, Y: -5}, Dir: 6, DurationMS: 250, Dash: true, Snap: true})
	if len(b) != 17 {
		t.Fatalf("len %d want 17", len(b))
	}
	le := binary.LittleEndian
	if le.Uint16(b[0:]) != SCEntityDash || SCEntityDash == SCEntityMove {
		t.Fatalf("opcode %#x", le.Uint16(b[0:]))
	}
	if int32(le.Uint32(b[2:])) != 0x01020304 || int32(le.Uint32(b[6:])) != 300 || int32(le.Uint32(b[10:])) != -5 || b[14] != 6 || le.Uint16(b[15:]) != 250 {
		t.Fatalf("fields % x", b)
	}
	for _, tc := range []struct {
		in   int32
		want uint16
	}{{-1, 0}, {70000, 0xFFFF}, {150, 150}} {
		if got := le.Uint16(EntityDash(event.EntityMoved{DurationMS: tc.in})[15:]); got != tc.want {
			t.Fatalf("duration %d encoded %d want %d", tc.in, got, tc.want)
		}
	}
	if ClientCapDashMoveV1 != 1<<8 {
		t.Fatal("dash capability bit moved")
	}
}
