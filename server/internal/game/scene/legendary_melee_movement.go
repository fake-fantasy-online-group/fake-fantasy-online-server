package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) legendaryMeleeAdjustStatus(target *entity.Entity, def *domain.StatusDef) {
	// No extra slow-resistance timer in the replacement design.
}
func (s *Scene) legendaryMeleeDisplacementImmune(target *entity.Entity) bool {
	return s.legendarySwordsmanDisplacementImmune(target)
}
