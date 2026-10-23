package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"

func (s *Scene) legendaryCasterModifyHeal(target *entity.Entity, amount int32) int32 {
	return s.legendaryIncomingHeal(target, amount)
}
