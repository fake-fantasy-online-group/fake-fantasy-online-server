package domain

import "fmt"

// ValidateSkillStatusRuntimeV5 validates the same resolved definitions combat
// consumes, including every bounded rank transition and the soft-rank boundary.
func ValidateSkillStatusRuntimeV5(skills SkillTable, statuses StatusTable, overlay StatusOverlay) error {
	if len(statuses) == 0 || len(overlay) == 0 {
		return fmt.Errorf("V5 status definitions/overlay missing")
	}
	c := skills.ProgressionCatalog()
	if c == nil {
		return fmt.Errorf("V5 skill progression catalog missing")
	}
	for _, id := range V5SkillIDs(c) {
		for rank := int32(1); rank <= 21; rank++ {
			def, ok := skills.Get(id, rank)
			if !ok || def.FormulaVersion != SkillFormulaVersion {
				return fmt.Errorf("V5 skill %d rank%d unavailable", id, rank)
			}
			for _, app := range def.Statuses {
				if _, ok := statuses.ResolveSkillApplication(app); !ok {
					return fmt.Errorf("V5 skill%d rank%d references missing status%d/%d", id, rank, app.ID, app.Level)
				}
			}
			for _, tr := range def.PassiveTriggers {
				if _, ok := statuses.Get(tr.Status, tr.StatusLevel); !ok {
					return fmt.Errorf("V5 passive%d rank%d references missing status%d/%d", id, rank, tr.Status, tr.StatusLevel)
				}
			}
		}
	}
	return nil
}
