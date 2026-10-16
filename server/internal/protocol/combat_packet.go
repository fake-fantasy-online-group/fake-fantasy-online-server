package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"

// CombatDamage encodes one independently owned combat packet without the outer
// packet-list allocation needed by the generic Encode API.
func CombatDamage(e event.DamageDealt) []byte { return encodeDamage(e) }
