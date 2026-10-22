package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Scale one delivered hit/tick, not the shared template, skill payload or DOT
// definition. Reflection and player-origin procs never pass this source gate.
func (s *Scene) dungeonMonsterDamage(src, dst *entity.Entity, amount int32) int32 {
	if amount <= 0 || !s.nativeDifficultyDungeon() || src == nil || src.Monster == nil || !src.Monster.Kind.Hostile() || dst == nil || (dst.Player == nil && dst.Pet == nil) {
		return amount
	}
	if s.dungeonDifficulty == domain.DungeonHard {
		return max(int32(1), clampI32(int64(amount)*3/2))
	}
	if src.Monster.Kind == domain.MonsterBoss {
		return max(int32(1), amount/2)
	}
	return amount
}
