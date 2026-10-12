package domain

// FinalizeSkillManaEfficiency runs after equipment and training modifiers.
// MPCost remains the exact admission debit. Its prepaid fractional remainder
// is returned only on accepted casts; rank growth never rounds away savings.
func FinalizeSkillManaEfficiency(def *SkillDef) {
	if def == nil || def.ManaEfficiencyRanks <= 0 || def.MPCost <= 0 || def.ManaCostDenominator > 0 {
		return
	}
	def.ManaCostNumerator = int64(def.MPCost) * 50
	def.ManaCostDenominator = 50 + int64(def.ManaEfficiencyRanks)
	def.MPCost = int32((def.ManaCostNumerator + def.ManaCostDenominator - 1) / def.ManaCostDenominator)
}
