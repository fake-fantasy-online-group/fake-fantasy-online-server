package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func init() {

	// Healing callbacks below are the actual handler for these powers. CastReleased
	// deliberately has no fabricated heal: only an observed resolved self heal grants it.

}

func (s *Scene) legendaryMeleeHealResolved(src, target *entity.Entity, def domain.SkillDef, actual, overflow int32) {
	// Native dual-sword restoration is now resolved once by the root batch.
}
func (s *Scene) legendaryMeleeDotDamage(target *entity.Entity, amount int32) int32 {
	s.forEachLegendary(target, func(c *legendaryContext) {
		if c.Power.ID == "JK-D02" {
			c.Blow.Origin = domain.DamageDOTOrigin
			amount = legendaryHandlers[c.Power.ID].BeforeIncoming(c, amount)
		}
	})
	return amount
}
