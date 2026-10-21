package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) challengeMonsterScene() bool {
	return s.rift != nil || s.trial != nil && s.trial.IsMap(s.id.MapID)
}

// Keep a valid nearby target for at most three seconds after path failure.
// The existing local A* budget and retry cache remain authoritative; waiting
// cannot permit attacks through a wall or unbounded whole-map path searches.
func (s *Scene) waitChallengeMonsterPath(m *entity.Entity, target domain.Pos, trace float64) bool {
	if !s.challengeMonsterScene() || sqDist(m.Pos, target) > trace*trace {
		return false
	}
	if m.Monster.ChallengePathWaitUntil == 0 {
		m.Monster.ChallengePathWaitUntil = s.tick + domain.Ticks(3000)
		if m.Monster.CombatPathActive {
			s.emit(event.EntityMoved{ID: m.ID, To: m.Pos, Snap: true})
			m.Monster.CombatPathActive = false
		}
	}
	return s.tick < m.Monster.ChallengePathWaitUntil
}
