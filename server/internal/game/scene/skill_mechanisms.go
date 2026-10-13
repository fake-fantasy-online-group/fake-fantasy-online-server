package scene

import (
	"fmt"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Keep the full authored prose and add only one compact row for configured
// target-specific results. Old unscoped skills keep their original layout.
func (s *Scene) skillScopedStatusDescription(def domain.SkillDef) string {
	var parts []string
	for _, app := range def.Statuses {
		var target string
		switch app.TargetType {
		case domain.SkillStatusTargetPlayer:
			target = "玩家"
		case domain.SkillStatusTargetNonPlayer:
			target = "非玩家"
		default:
			continue
		}
		status, ok := s.skillDescriptionStatus(app)
		if !ok {
			continue
		}
		chance := ""
		if app.ChanceBP < 10000 {
			chance = equipmentPercent(app.ChanceBP)
		}
		details := []string{fmt.Sprintf("%s：%s%s%d秒", target, chance, status.Name, status.DurationSec)}
		for _, affix := range status.Affixes {
			details = append(details, skillAffixText(affix))
		}
		parts = append(parts, strings.Join(details, "，"))
	}
	return strings.Join(parts, "；")
}
