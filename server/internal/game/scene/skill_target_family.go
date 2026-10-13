package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// The same predicate guards admission and settlement, including direct calls
// from traps/monster effects. Family rules never widen normal target admission.
// Summoned combat monsters keep their template family; this does not change
// the existing no-reward, expiry or despawn rules for summons.
func (s *Scene) matchesSkillTargetFamilies(target *entity.Entity, def domain.SkillDef) bool {
	if len(def.TargetFamilies) == 0 {
		return true
	}
	if target == nil || target.Kind != domain.KindMonster || target.Monster == nil ||
		!target.Monster.Kind.Hostile() || s.defs == nil {
		return false
	}
	template, ok := s.defs.Def(target.Monster.TypeID)
	return ok && template.Kind.Hostile() && def.MatchesMonsterFamily(template.Family)
}
