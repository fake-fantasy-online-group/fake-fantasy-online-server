package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"

// EntityMove encodes one independently owned movement packet. Callers that only
// need this packet can avoid the outer packet-list allocation in Encode.
func EntityMove(e event.EntityMoved) []byte { return encodeMove(e) }
