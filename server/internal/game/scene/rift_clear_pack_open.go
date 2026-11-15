package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// riftClearPackTrueSetRoll lets tests force the extremely low 真* set roll.
var riftClearPackTrueSetRoll func(*Scene) bool

func riftClearPackTrueSetHit(s *Scene) bool {
	if riftClearPackTrueSetRoll != nil {
		return riftClearPackTrueSetRoll(s)
	}
	if s == nil || s.rng == nil {
		return false
	}
	return domain.RiftClearPackTrueSetHit(s.rng.Intn)
}
