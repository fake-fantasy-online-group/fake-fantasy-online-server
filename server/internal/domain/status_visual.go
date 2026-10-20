package domain

// StatusVisualKey matches the original status identity and rank-specific name.
// A rule must not apply to another status that happens to reuse either one.
type StatusVisualKey struct {
	ID           StatusID
	ExpectedName string
}

// StatusVisualPositionInherit preserves the status's existing display position.
const StatusVisualPositionInherit int16 = -1

// StatusVisualRule contains presentation metadata only. It never changes a
// StatusDef, combat behavior, saved status identity, or animation metadata.
type StatusVisualRule struct {
	EffectName string
	// False preserves the existing self-HUD name while world effects use EffectName.
	UseEffectNameForSelf bool
	// -1 inherits the existing position; 0..3 select a client display position.
	PositionMode int16
}

// StatusVisualTable is optional database configuration, loaded once at startup.
// An empty EffectName is the explicit sentinel for no configured mapping.
type StatusVisualTable map[StatusVisualKey]StatusVisualRule

// Get returns only an active rule for the exact original identity and name.
func (t StatusVisualTable) Get(id StatusID, expectedName string) (StatusVisualRule, bool) {
	rule, ok := t[StatusVisualKey{ID: id, ExpectedName: expectedName}]
	return rule, ok && rule.EffectName != ""
}
