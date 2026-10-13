package domain

// FinalizeSkillManaCost applies scoped high-rank reduction after equipment and
// training. The resulting integer is displayed, checked and paid identically.
// A positive cost bottoms out at one. Preserve the exact rate so the accepted
// cast can carry already-paid rounding credit; this function is still pure.
func FinalizeSkillManaCost(def *SkillDef) {
	if def == nil || def.ManaCostReductionRanks <= 0 || def.MPCost <= 0 || def.ManaCostReductionApplied {
		return
	}
	numerator := int64(def.MPCost) * 50
	denominator := 50 + int64(def.ManaCostReductionRanks)
	def.ManaCostFractionNumerator, def.ManaCostFractionDenominator = numerator, denominator
	def.MPCost = int32((numerator + denominator - 1) / denominator)
	def.ManaCostReductionApplied = true
}
