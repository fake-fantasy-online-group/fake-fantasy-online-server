package scene

import (
	"fmt"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// selectSkillStatuses runs only after this target's damage/landing/death gate.
// One draw selects a candidate, not whether it succeeds. The selected status
// then uses applyStatusDef's single probability/resistance contest, immunity
// and overlay rules. A failed candidate never causes a replacement draw.
func (s *Scene) selectSkillStatuses(apps []domain.StatusApplication, kind domain.EntityKind) []domain.StatusApplication {
	out := make([]domain.StatusApplication, 0, len(apps))
	seen := map[int32]bool{}
	for _, app := range apps {
		if !app.TargetType.Matches(kind) {
			continue
		}
		if app.RandomGroup == 0 {
			out = append(out, app)
			continue
		}
		if seen[app.RandomGroup] {
			continue
		}
		seen[app.RandomGroup] = true
		var total int64
		for _, candidate := range apps {
			if candidate.RandomGroup == app.RandomGroup && candidate.RandomWeight > 0 && candidate.TargetType.Matches(kind) {
				total += int64(candidate.RandomWeight)
			}
		}
		if total <= 0 { // Runtime-created malformed pools fail closed.
			continue
		}
		draw := s.rng.Int63n(total)
		for _, candidate := range apps {
			if candidate.RandomGroup != app.RandomGroup || candidate.RandomWeight <= 0 || !candidate.TargetType.Matches(kind) {
				continue
			}
			draw -= int64(candidate.RandomWeight)
			if draw < 0 {
				out = append(out, candidate)
				break
			}
		}
	}
	return out
}

// Present the actual resolved pool without rewriting the historic skill prose.
func (s *Scene) skillRandomStatusDescription(def domain.SkillDef) string {
	seen := map[int32]bool{}
	var parts []string
	for _, app := range def.Statuses {
		if app.RandomGroup <= 0 || seen[app.RandomGroup] {
			continue
		}
		seen[app.RandomGroup] = true
		var options []string
		for _, candidate := range def.Statuses {
			if candidate.RandomGroup != app.RandomGroup {
				continue
			}
			status, ok := s.skillDescriptionStatus(candidate)
			if !ok {
				continue
			}
			options = append(options, status.Name)
		}
		parts = append(parts, fmt.Sprintf("有%s几率随机施加%s之一。", equipmentPercent(app.ChanceBP), strings.Join(options, "、")))
	}
	return strings.Join(parts, "。")
}
