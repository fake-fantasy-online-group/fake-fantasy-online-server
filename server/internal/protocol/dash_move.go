package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"

// SCEntityDash is a same-map skill displacement the client interpolates
// instead of reloading the map. Layout (17 bytes incl. opcode):
// op u16 | id i32 | x i32 | y i32 | dir u8 | durationMS u16.
const SCEntityDash = 0x8078

// ClientCapDashMoveV1 is announced through OpClientCaps (0x1091). Sessions
// without it keep receiving the legacy EntityMove snap (speed=-1).
const ClientCapDashMoveV1 int32 = 1 << 8

// EntityDash encodes e as a dash packet. Callers must only use it for
// sessions that announced ClientCapDashMoveV1.
func EntityDash(e event.EntityMoved) []byte {
	d := e.DurationMS
	if d < 0 {
		d = 0
	}
	if d > 0xFFFF {
		d = 0xFFFF
	}
	return newFixedW(SCEntityDash, 17).
		I32(int32(e.ID)).
		I32(int32(e.To.X)).
		I32(int32(e.To.Y)).
		U8(e.Dir).
		U16(uint16(d)).
		Bytes()
}
