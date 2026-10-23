package protocol

import (
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const ClientCapNativeAttackRangeV1 int32 = 1 << 6

// WithNativeAttackRange appends one final U16 only to the corresponding packet.
// The session calls this only after pre-world-entry connection opt-in.
func WithNativeAttackRange(packet []byte, ev event.Event) []byte {
	if len(packet) < 2 {
		return packet
	}
	var want uint16
	var distance uint16
	switch x := ev.(type) {
	case event.SelfEntered:
		want = SCSelfData
		distance = x.NativeAtkDist
	case event.AppearanceChanged:
		want = SCAppearance
		distance = x.NativeAtkDist
	case event.EntitySpawned:
		if x.Kind != domain.KindPlayer {
			return packet
		}
		want = SCRemotePlayer
		distance = x.NativeAtkDist
	default:
		return packet
	}
	if binary.LittleEndian.Uint16(packet) != want {
		return packet
	}
	out := make([]byte, len(packet)+2)
	copy(out, packet)
	binary.LittleEndian.PutUint16(out[len(packet):], distance)
	return out
}
