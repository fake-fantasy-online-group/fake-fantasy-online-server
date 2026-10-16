package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) positionSnapshotOf(e *entity.Entity) domain.Snapshot {
	pos, instance := s.persistedPosition(e)
	return domain.Snapshot{Parts: domain.SnapshotPosition, Char: &domain.Character{
		ID: e.Player.Char.ID, Name: e.Player.Char.Name, Pos: pos, SceneInstance: instance,
	}}
}
