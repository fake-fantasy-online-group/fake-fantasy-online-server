package domain

import "strings"

// LegendaryDisplayName composes the independent power and base identity.
// It never renames the catalog base or changes its appearance/drop eligibility.
func LegendaryDisplayName(base string, power LegendaryPowerDefinition) string {
	base = strings.TrimSpace(base)
	if power.Name == "" {
		return base
	}
	if base == "" {
		return power.Name
	}
	return power.Name + "之" + base
}
