package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Intrinsic high-rank benefits use the same effective-rank policy as character
// aggregation. They stay on their own skill and never invent outgoing bonuses.
func (s *Scene) passiveOverflowEffectLines(def domain.SkillDef) []string {
	var lines []string
	if attr, value := domain.SkillGrowthAttribute(def.ID, def.Level, s.skills.ProgressionCatalog()); value > 0 {
		lines = append(lines, fmt.Sprintf("%s +%d", skillAttrText(attr), value))
	}
	if def.Level > 5 && def.Kind == domain.SkillPassive && def.PassiveCounter != nil {
		lines = append(lines, fmt.Sprintf("反击伤害: %d%%。", def.PassiveCounter.DamagePct))
	}
	return lines
}
