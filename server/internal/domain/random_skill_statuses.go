package domain

import "fmt"

// ValidateRandomStatusGroups checks the generic select-one configuration. A
// group has one target scope and one base probability; weights only decide
// which candidate reaches the existing single effect-hit/resistance contest.
// A one-candidate group is valid after an administrator deliberately trims a
// pool. Duplicate identities with overlapping scopes are rejected so saved
// effects can reconcile their payload by status ID without rerolling a pool.
func ValidateRandomStatusGroups(apps []StatusApplication) error {
	groups := map[int32]StatusApplication{}
	for i, app := range apps {
		if app.RandomGroup < 0 || app.RandomWeight < 0 || (app.RandomGroup == 0) != (app.RandomWeight == 0) {
			return fmt.Errorf("status %d/%d has invalid random group/weight %d/%d", app.ID, app.Level, app.RandomGroup, app.RandomWeight)
		}
		if app.RandomGroup == 0 {
			continue
		}
		if !app.ChanceExplicit || app.ChanceBP < 0 || app.ChanceBP > 10000 {
			return fmt.Errorf("random group %d requires an explicit valid base chance", app.RandomGroup)
		}
		if first, ok := groups[app.RandomGroup]; ok {
			if first.TargetType != app.TargetType || first.ChanceBP != app.ChanceBP || first.ChanceExplicit != app.ChanceExplicit {
				return fmt.Errorf("random group %d mixes target scopes or base chances", app.RandomGroup)
			}
		} else {
			groups[app.RandomGroup] = app
		}
		for j, other := range apps {
			if i == j || app.ID != other.ID {
				continue
			}
			if app.TargetType == SkillStatusTargetAny || other.TargetType == SkillStatusTargetAny || app.TargetType == other.TargetType {
				return fmt.Errorf("random status %d has ambiguous overlapping applications", app.ID)
			}
		}
	}
	return nil
}
