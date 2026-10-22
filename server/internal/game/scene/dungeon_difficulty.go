package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// nativeDifficultyDungeon covers the seven shipped difficulty menus plus six
// audited native transport destinations and their floors. Trials, rifts, public
// maps, wedding and unrelated event instances remain excluded.
func (s *Scene) nativeDifficultyDungeon() bool {
	if s == nil || s.dungeon == nil || s.id.Persistent() || s.rift != nil || (s.trial != nil && s.trial.IsMap(s.id.MapID)) {
		return false
	}
	for _, entry := range []int32{20100, 20300, 20502, 20600, 20701, 20801, 20901, 23800, 24400, 24500, 26800, 26900, 30600} {
		if d, ok := s.dungeons[entry]; ok && d.ID == s.dungeon.ID {
			return true
		}
	}
	return false
}

// Always derive changed stats from immutable definitions, never an already
// scaled entity. Router also calls this before publication once run metadata
// is attached; no actor or player can observe that intermediate state.
func (s *Scene) scaleDungeonMonster(m *entity.Entity) {
	if !s.nativeDifficultyDungeon() || m == nil || m.Monster == nil || !m.Monster.Kind.Hostile() || s.defs == nil {
		return
	}
	d, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return
	}
	hp := int64(d.HP)
	hit := int64(d.Stats.Hit)
	if s.dungeonDifficulty == domain.DungeonHard {
		if m.Monster.Kind == domain.MonsterBoss {
			hit *= 2
		} else {
			hp = hp * 3 / 2
		}
	} else {
		hp /= 2
	}
	m.MaxHP = max(int32(1), domain.SaturateInt32(hp))
	m.HP = m.MaxHP
	m.Stats.MaxHP = m.MaxHP
	m.Stats.Hit = domain.SaturateInt32(hit)
}
