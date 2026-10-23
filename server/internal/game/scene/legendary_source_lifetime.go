package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Foreign permanent equipment shields require source and recipient to remain
// in the same scene. Self-owned state is deliberately left for lossless transfer.
func (s *Scene) revokeLegendaryForeignShields(leaving *entity.Entity) {
	if leaving == nil {
		return
	}
	for _, target := range s.entities {
		if target.Status == nil {
			continue
		}
		sources := map[domain.EntityID]bool{}
		target.Status.Each(func(a domain.Active) {
			d := a.Def
			if a.Source != target.ID && (target.ID == leaving.ID || a.Source == leaving.ID) && d.LegendaryPowerID != "" && d.LegendarySourceUID != 0 && d.DamageShield > 0 && (d.Permanent || d.DurationSec <= 0) {
				sources[a.Source] = true
			}
		})
		for source := range sources {
			removed := target.Status.RemoveSourceWhere(source, func(d domain.StatusDef) bool {
				return d.LegendaryPowerID != "" && d.LegendarySourceUID != 0 && d.DamageShield > 0 && (d.Permanent || d.DurationSec <= 0)
			})
			s.statusesRemoved(target, removed)
		}
		if len(sources) > 0 {
			s.refreshEntityStats(target)
			s.pushStatusSnapshots(target)
		}
	}
}
