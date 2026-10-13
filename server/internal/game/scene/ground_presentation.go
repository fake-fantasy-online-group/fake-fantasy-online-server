package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sync/atomic"
	"time"
)

// Tokens distinguish new ground lifetimes even when the same inventory UID is dropped again.
// Allocated process-wide, not scene-local, so reconnects and map changes cannot alias IDs.
var groundPresentationToken = time.Now().UnixNano()

func (s *Scene) groundUniqueID(stack domain.Stack) int32 {
	if _, ok := domain.UniqueOf(stack,s.endgameCatalog); ok {
		return stack.Endgame.UniqueID
	}
	return 0
}

func (s *Scene) freshGroundSpawnEvent(e *entity.Entity) event.EntitySpawned {
	fresh := e != nil && e.Drop != nil && e.Drop.PresentationToken == 0
	if fresh {
		e.Drop.PresentationToken = atomic.AddInt64(&groundPresentationToken, 1)
	}
	ev := s.spawnEvent(e)
	if ev.Ground != nil {
		ev.Ground.FreshDrop = fresh
	}
	return ev
}
