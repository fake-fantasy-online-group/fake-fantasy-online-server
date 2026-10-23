# Optional fixed ground effects v1

Capability: `endgame.ground-fx.v1`. The client requests it in the secondary
`capabilities` request together with `endgame.v1` and `endgame.rift-pages.v1`.
The existing eight-entry `hello` is unchanged. A successful correlated secondary
ack must include the ground capability before the client admits these packets.
An old server may acknowledge only rift pages; ground effects remain disabled.
The server sends the extension only to negotiated, map-ready players in the map.
Negotiation survives a map handoff, but not a fresh connection.

Opcode `0x80F1` (unused in both repositories when allocated). Existing entity FX
`0x8045` remains unchanged. Little-endian payload, in exact order:

| Field | Type | Meaning |
| --- | --- | --- |
| owner | i32 | Source entity ID, nonzero, not a follow target |
| mapID | i32 | Positive native map ID |
| x, y | i32 each | Fixed native map-pixel coordinates |
| key | u16 byte length + UTF-8 | Nonempty owner-scoped effect identity, <=128 UTF-8 bytes |
| effect | u16 byte length + UTF-8 | Approved resource name, required except stop |
| durationMS | i32 | Nonnegative; used for sustained effects |
| mode | u8 | 0 once, 1 sustained, 2 stop |

Mode 0 plays the resource once. Mode 1 loops at the fixed position; duration>0
expires it after that interval, duration=0 lasts until stop/replace/map teardown.
Mode 2 stops `owner+key`; an empty effect is legal. Reusing `owner+key` replaces
its previous visual. Every visual, including unfinished one-shots, is removed on
world teardown. The map ID must match the current world. Coordinates convert with
`(x-worldWidth/2)/ppu, (worldHeight/2-y)/ppu`; no actor transform is attached.

The event uses the existing bounded `WorldSkillHitEffect` queue with an explicit
`IsGround` discriminator; no gateway interface is added. This is presentation
only and creates no gameplay timer or damage. Sustained effects are retained in
the owning scene and replayed at the existing full-map entity visibility snapshot
and first capability acknowledgement. Replay preserves fixed coordinates and sends
only the remaining lifetime; one-shots and expired/stopped effects are not replayed.
The existing scene timer wheel removes deadlines without a new polling service.
Owner transfer/logout removes its effects and emits stops. Asset selection/binding remains per-power and
must not reuse obsolete timed-stage artwork for a different gameplay stage.

Validation: `python3 Tests/Endgame/run-ground-effect-contract.py` compiles the
actual engine-free core and actual ground presenter with minimal Unity/renderer
substitutes. It verifies wire rejection, queue metadata, secondary negotiation,
fixed coordinate conversion, replace/stop/cleanup, and integration source guards.
It does not validate Unity rendering or resource availability. No private effect
assets are included.
