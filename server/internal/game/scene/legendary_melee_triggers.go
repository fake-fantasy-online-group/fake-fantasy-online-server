package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) legendaryMeleeEnemyCast(caster *entity.Entity) {
	// Superseded by event-local control builds.
}

func (s *Scene) legendaryMeleeMoved(mover *entity.Entity, from, to domain.Pos, forced bool) {
	// Superseded movement-history powers are deliberately inactive.
}
