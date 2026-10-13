package protocol

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const ClientCapGroundPresentationV1 int32 = 1 << 5

// GroundSpawnPresentation preserves the exact legacy prefix. Only opted-in sessions receive
// v1:u8 | flags:u8 (bit0=fresh) | uniqueID:i32 | dropToken:i64 after quality.
func GroundSpawnPresentation(observer domain.EntityID, e event.EntitySpawned) []byte {
	if e.Ground == nil {
		return nil
	}
	legacy := encodeGroundSpawn(observer, e)
	tail := NewW(0).U8(1).U8(b2u8(e.Ground.FreshDrop)).I32(e.Ground.UniqueID).I64(e.Ground.DropToken).Bytes()[2:]
	return append(legacy, tail...)
}
