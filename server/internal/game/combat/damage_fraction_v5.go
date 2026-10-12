package combat

import "math/big"

// fractionDamage retains the curve's exact percentage through the native fixed
// defenses. big.Int is a safe fallback for extreme authored soft ranks.
func fractionDamage(base int64, n, d int64, mdef, resist int32) int32 {
	if d <= 0 {
		n, d = 100, 1
	}
	if n <= 0 {
		return 0
	}
	// Common legal ranks stay allocation-free; reject overflow before multiplying.
	const limit int64 = 1<<63 - 1
	if n <= limit/max(int64(1), base) && d <= limit/max(int64(100), int64(mdef)*63) {
		attack := base * n
		defense := int64(mdef) * 63 * d
		if defense >= 0 && attack >= 0 {
			return boundedDamage((attack-defense)/(100*d) - int64(resist))
		}
	}
	var a, b, den big.Int
	a.Mul(big.NewInt(base), big.NewInt(n))
	b.Mul(big.NewInt(int64(mdef)*63), big.NewInt(d))
	a.Sub(&a, &b)
	den.Mul(big.NewInt(100), big.NewInt(d))
	a.Quo(&a, &den)
	a.Sub(&a, big.NewInt(int64(resist)))
	if !a.IsInt64() {
		if a.Sign() > 0 {
			return 2147483647
		}
		return 1
	}
	return boundedDamage(a.Int64())
}
func RollPhysicalSkillFraction(minAtk, maxAtk, resist int32, n, d int64, rng Rand) int32 {
	if maxAtk < minAtk {
		minAtk, maxAtk = maxAtk, minAtk
	}
	base := int64(minAtk)
	if span := int64(maxAtk) - base; span > 0 && rng != nil {
		base += int64(rng.Intn(int(span) + 1))
	}
	return fractionDamage(base, n, d, 0, resist)
}
func RollMagicFixedDefenseFraction(matk int32, n, d int64, mdef, mresist int32) int32 {
	return fractionDamage(int64(matk), n, d, mdef, mresist)
}
