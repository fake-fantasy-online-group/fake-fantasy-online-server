package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestEntityMovePacketEncodingAndOwnership(t *testing.T) {
	for _, speed := range []int32{-123, -1, 0, 1, 200, 2147483647} {
		for _, snap := range []bool{false, true} {
			for _, stop := range []bool{false, true} {
				t.Run(fmt.Sprintf("speed=%d/snap=%t/stop=%t", speed, snap, stop), func(t *testing.T) {
					ev := event.EntityMoved{ID: 123, To: domain.Pos{X: 456.75, Y: -789.25}, Speed: speed, Snap: snap, Stop: stop}
					got := EntityMove(ev)
					want := Encode(999, ev)
					if len(want) != 1 || !bytes.Equal(got, want[0]) {
						t.Fatal("single packet differs from generic encoder")
					}
					wireSpeed := speed
					if wireSpeed <= 0 {
						wireSpeed = defaultMoveSpeed
					}
					if snap {
						wireSpeed = -1
					} else if stop {
						wireSpeed = 0
					}
					if len(got) != 19 || binary.LittleEndian.Uint16(got) != SCEntityMove || int32(binary.LittleEndian.Uint32(got[6:])) != wireSpeed || got[10] != 1 || int32(binary.LittleEndian.Uint32(got[11:])) != 456 || int32(binary.LittleEndian.Uint32(got[15:])) != -789 {
						t.Fatalf("incorrect movement wire fields: %x", got)
					}
					retained := append([]byte(nil), got...)
					other := EntityMove(ev)
					other[0] ^= 255
					if !bytes.Equal(got, retained) {
						t.Fatal("packet storage shared across encodes")
					}
				})
			}
		}
	}
}
