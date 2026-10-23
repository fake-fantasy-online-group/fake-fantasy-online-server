package protocol

import (
	"bytes"
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestNativeAttackRangeFinalTailPreservesCompleteLegacyPacket(t *testing.T) {
	for _, distance := range []uint16{0, 75, 65535} {
		ev := event.AppearanceChanged{Who: 1, Appearance: domain.Appearance{AtkDist: 350}, NativeAtkDist: distance}
		old := Encode(1, ev)[0]
		saved := append([]byte(nil), old...)
		got := WithNativeAttackRange(old, ev)
		if len(got) != len(old)+2 || !bytes.Equal(got[:len(old)], old) || !bytes.Equal(saved, old) || binary.LittleEndian.Uint16(got[len(old):]) != distance {
			t.Fatal("tail mutated legacy or truncated value")
		}
		unrelated := NewW(SCPartySnapshot).U8(0).Bytes()
		if !bytes.Equal(WithNativeAttackRange(unrelated, ev), unrelated) {
			t.Fatal("party packet extended")
		}
	}
}
