package domain

import "math/big"

func skillHealAmount(e SkillEffect, hp int32) int32 {
	pct := big.NewRat(int64(e.HealPctOfMax), 1)
	flat := big.NewRat(int64(e.HealFlat), 1)
	if e.HealPctDenominator > 0 {
		pct.SetFrac64(e.HealPctNumerator, e.HealPctDenominator)
	}
	if e.HealFlatDenominator > 0 {
		flat.SetFrac64(e.HealFlatNumerator, e.HealFlatDenominator)
	}
	pct.Mul(pct, big.NewRat(int64(hp), 100))
	pct.Add(pct, flat)
	n := new(big.Int).Quo(pct.Num(), pct.Denom())
	if !n.IsInt64() || n.Int64() > 2147483647 {
		return 2147483647
	}
	return SaturateInt32(n.Int64())
}

// BoostSkillDamage preserves exact authored rank fractions through additive buckets.
func BoostSkillDamage(e *SkillEffect, bonusPct int32) {
	if e == nil || bonusPct == 0 {
		return
	}
	e.BaseDamageFlat = SaturateInt32(int64(e.BaseDamageFlat) * (100 + int64(bonusPct)) / 100)
	n, d := e.DamagePctNumerator, e.DamagePctDenominator
	if d <= 0 {
		n, d = int64(e.DamagePct), 1
		if n <= 0 {
			n = 100
		}
	}
	r := new(big.Rat).SetFrac64(n, d)
	r.Mul(r, big.NewRat(100+int64(bonusPct), 100))
	if r.Num().IsInt64() && r.Denom().IsInt64() {
		e.DamagePctNumerator = r.Num().Int64()
		e.DamagePctDenominator = r.Denom().Int64()
	}
	q := new(big.Int).Quo(r.Num(), r.Denom())
	if q.IsInt64() {
		e.DamagePct = SaturateInt32(q.Int64())
	}
}

func ScaleSkillDamageFraction(e *SkillEffect, numerator, denominator int64) {
	if e != nil && denominator > 0 {
		e.BaseDamageFlat = SaturateInt32(int64(e.BaseDamageFlat) * numerator / denominator)
	}
	if e == nil || denominator <= 0 {
		return
	}
	n, d := e.DamagePctNumerator, e.DamagePctDenominator
	if d <= 0 {
		n, d = int64(e.DamagePct), 1
		if n <= 0 {
			n = 100
		}
	}
	r := new(big.Rat).SetFrac64(n, d)
	r.Mul(r, big.NewRat(numerator, denominator))
	if r.Num().IsInt64() && r.Denom().IsInt64() {
		e.DamagePctNumerator = r.Num().Int64()
		e.DamagePctDenominator = r.Denom().Int64()
	}
	q := new(big.Int).Quo(r.Num(), r.Denom())
	if q.IsInt64() {
		e.DamagePct = SaturateInt32(q.Int64())
	}
}
