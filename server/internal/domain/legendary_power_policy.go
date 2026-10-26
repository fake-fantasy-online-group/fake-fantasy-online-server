package domain

import "strings"

// LegendaryPowerPolicySplit selects profession/universal with equal probability
// before selecting a power. Zero is the immutable historical mixed-pool policy.
const LegendaryPowerPolicySplit int32 = 1

// The profession pool may use every actual equipment slot. Universal powers
// retain their original A/D/U slot restrictions. Native wear restrictions and
// monster source-level requirements are not relaxed.
func legendaryPowerEligibleAllClassSlots(p LegendaryPowerDefinition, base ItemDef, sourceLevel int32) bool {
	if p.Class == "全职业" {
		return LegendaryPowerEligibleAtLevel(p, base, sourceLevel)
	}
	e := base.Equip
	if e == nil || !LegendaryPowerSourceLevelEligible(p, sourceLevel) || e.Slot < int32(SlotFace) || e.Slot > int32(maxEquipSlot) {
		return false
	}
	// Validate the same catalog power categories without their historical slots.
	parts := strings.Split(p.ID, "-")
	if len(parts) != 2 || len(parts[1]) == 0 {
		return false
	}
	category := parts[1][0]
	validCategory := category == 'A' || category == 'D' || category == 'U'
	mask := legendaryClassMask(p.Class)
	restricted := e.Need.ProfessionRestricted || e.Need.Professions != 0
	return validCategory && mask != 0 && (!restricted || e.Need.Professions&mask != 0)
}

func LegendaryCandidatesForRecipientWithPolicyAtLevel(def ItemDef, sourceLevel int32, recipient LegendaryRecipientClass, policy int32) []LegendaryPowerDefinition {
	if policy == 0 {
		return LegendaryCandidatesForRecipientAtLevel(def, sourceLevel, recipient)
	}
	if policy != LegendaryPowerPolicySplit || !def.LegendaryEnabled {
		return nil
	}
	var powers []LegendaryPowerDefinition
	for _, p := range LegendaryPowers() {
		if recipient.allows(p) && LegendaryPowerExecutable(p.ID) && legendaryPowerEligibleAllClassSlots(p, def, sourceLevel) {
			powers = append(powers, p)
		}
	}
	return powers
}

func chooseLegendaryPowerPool(powers []LegendaryPowerDefinition, rng func(int) int) ([]LegendaryPowerDefinition, bool) {
	var profession, universal []LegendaryPowerDefinition
	for _, p := range powers {
		if p.Class == "全职业" {
			universal = append(universal, p)
		} else {
			profession = append(profession, p)
		}
	}
	// A missing eligible class pool (including an unknown/unemployed recipient)
	// falls back to universal. Never silently discard an awarded Legendary.
	if len(profession) == 0 {
		return universal, len(universal) > 0
	}
	if len(universal) == 0 {
		return profession, true
	}
	n := rng(2)
	if n == 0 {
		return profession, true
	}
	return universal, n == 1
}

func (t EquipmentRollTable) ApplyLegendaryForRecipientWithPolicyAtLevel(st *Stack, def ItemDef, sourceLevel int32, recipient LegendaryRecipientClass, policy int32, variants bool, rng func(int) int) bool {
	if policy != 0 && policy != LegendaryPowerPolicySplit {
		return false
	}
	return t.applyLegendary(st, def, sourceLevel, rng, variants, true, &recipient, policy)
}

func (t EquipmentRollTable) ApplyLegendaryForRecipientWithPolicy(st *Stack, def ItemDef, recipient LegendaryRecipientClass, policy int32, rng func(int) int) bool {
	if st == nil || policy != 0 && policy != LegendaryPowerPolicySplit {
		return false
	}
	return t.applyLegendary(st, def, LegendaryGenerationLevel(def, *st), rng, true, false, &recipient, policy)
}
