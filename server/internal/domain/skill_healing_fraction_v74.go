package domain

import "math/big"

// ApplySkillHealingBP scales both healing channels without overflowing an
// intermediate numerator. Authored rank fractions stay exact after reduction;
// only the existing integer/rational payload boundaries can narrow the result.
func ApplySkillHealingBP(e *SkillEffect, bp int32) {
	if e == nil || bp == 0 {
		return
	}
	e.HealFlat, e.HealFlatNumerator, e.HealFlatDenominator = scaleHealingFractionBP(e.HealFlat, e.HealFlatNumerator, e.HealFlatDenominator, bp)
	e.HealPctOfMax, e.HealPctNumerator, e.HealPctDenominator = scaleHealingFractionBP(e.HealPctOfMax, e.HealPctNumerator, e.HealPctDenominator, bp)
}

func scaleHealingFractionBP(value int32, numerator, denominator int64, bp int32) (int32, int64, int64) {
	if denominator <= 0 {
		numerator, denominator = int64(value), 1
	}
	r := new(big.Rat).SetFrac64(numerator, denominator)
	r.Mul(r, big.NewRat(10000+int64(bp), 10000))
	// Keep the historical integer mirror. Healing consumers use the exact
	// fraction below and round only after flat and recipient-HP channels combine.
	value = ApplyUniqueBP(value, bp)
	if r.Num().IsInt64() && r.Denom().IsInt64() {
		return value, r.Num().Int64(), r.Denom().Int64()
	}
	// Authored healing curves have denominators 1, 4 or 50, so every result
	// within the resource representation fits after reduction. For an oversized
	// payload, saturate at the existing int32 boundary rather than wrapping.
	q := new(big.Int).Quo(r.Num(), r.Denom())
	if !q.IsInt64() {
		if q.Sign() < 0 {
			return -2147483648, -2147483648, 1
		}
		return 2147483647, 2147483647, 1
	}
	value = SaturateInt32(q.Int64())
	return value, int64(value), 1
}
