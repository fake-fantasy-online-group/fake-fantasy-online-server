package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func legendaryTriggerControl(d domain.StatusDef) bool {
	return d.Control == domain.ControlStun || d.Control == domain.ControlSilence || d.BlockMove
}

// Snapshot receipts before expiration; after removal the instance's shield
// balance, source identity and dynamic control definition would be lost.
func (s *Scene) legendaryExpiringStatuses(e *entity.Entity) []domain.Active {
	if e == nil || e.Status == nil {
		return nil
	}
	var out []domain.Active
	e.Status.Each(func(a domain.Active) {
		if a.ExpireAt != 0 && a.ExpireAt <= s.tick && (a.ShieldLeft > 0 || legendaryTriggerControl(a.Def)) {
			out = append(out, a)
		}
	})
	return out
}
func (s *Scene) legendaryNaturalStatusesExpired(e *entity.Entity, expired []domain.Active) {
	ended := false
	for _, a := range expired {
		if a.ShieldLeft > 0 {
			s.universalShieldExpired(e, a.ShieldLeft, a.Def.LegendaryPowerID)
		}
		if legendaryTriggerControl(a.Def) && !e.Status.Has(a.Def.ID) {
			ended = true
		}
	}
	if ended {
		s.universalControlEnded(e)
	}
}

// Explicit removals must not be mistaken for natural shield expiration.
func (s *Scene) legendaryRemovedControlStatuses(e *entity.Entity, ids []domain.StatusID) {
	if e == nil || e.Status == nil {
		return
	}
	for _, id := range ids {
		if e.Status.Has(id) {
			continue
		}
		if d, ok := s.statusDefByID(id); ok && legendaryTriggerControl(d) {
			s.universalControlEnded(e)
			return
		}
		switch id {
		case domain.StatusStun, domain.StatusSilence, domain.StatusFreeze, domain.StatusPetrify, 1034:
			s.universalControlEnded(e)
			return
		}
	}
}
