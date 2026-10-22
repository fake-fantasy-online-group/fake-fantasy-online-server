package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Keep catalog IDs and saved rule snapshots intact. This encounter-local policy
// also covers native guardians, summons and clones, not just elite affixes.
func (s *Scene) riftMonster(e *entity.Entity) bool {
	return s.rift != nil && e != nil && e.Kind == domain.KindMonster
}

func disabledRiftMonsterStatus(d domain.StatusDef) bool {
	return d.Invulnerable ||
		d.PhysicalDamageReductionPct >= 100 || d.MagicDamageReductionPct >= 100 ||
		d.PhysicalReflectPct > 0 || d.MagicReflectPct > 0 ||
		d.PhysicalReflectFlat > 0 || d.MagicReflectFlat > 0 || d.MagicFullReflectChancePct > 0
}

func (s *Scene) clearDisabledRiftMonsterStatuses(e *entity.Entity) {
	if s.riftMonster(e) && e.Status != nil {
		s.statusesRemoved(e, e.Status.RemoveWhere(disabledRiftMonsterStatus))
	}
}

// Do not spend an AI action on a native self-buff whose only effect is disabled.
// Mixed skills retain their other effects; applyStatusDef rejects the defense.
func (s *Scene) disabledRiftMonsterSkill(m *entity.Entity, rule domain.MonsterSkillRule) bool {
	if !s.riftMonster(m) || !rule.Self || len(rule.Skill.DirectEffects()) != 0 || len(rule.Extra) != 0 || len(rule.Skill.Statuses) == 0 {
		return false
	}
	for _, app := range rule.Skill.Statuses {
		d, ok := s.resolveSkillStatusDef(rule.Skill, app, 0)
		if !ok || !disabledRiftMonsterStatus(d) {
			return false
		}
	}
	return true
}
