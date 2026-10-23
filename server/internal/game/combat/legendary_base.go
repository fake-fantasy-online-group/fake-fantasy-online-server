package combat

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"math/big"
)

func rollStrikeWithAddedBase(a, d domain.Stats, b Blow, rng Rand) int32 {
	base := int64(a.MAtk)
	mdef, resist := d.MDef, d.MagicResist
	if !b.Magic {
		base = int64(min(a.MinAtk, a.MaxAtk))
		span := int64(max(a.MinAtk, a.MaxAtk)) - base
		if span > 0 && rng != nil {
			base += int64(rng.Intn(int(span) + 1))
		}
		mdef = 0
		resist = d.PhysResist
	}
	n, den := b.SkillPctNumerator, b.SkillPctDenominator
	if den <= 0 {
		n = int64(b.SkillPct)
		if n <= 0 {
			n = 100
		}
		den = 1
	}
	// (attack*coefficient + added base) is one base, defended exactly once.
	var value, extra, div big.Int
	value.Mul(big.NewInt(base), big.NewInt(n))
	div.Mul(big.NewInt(100), big.NewInt(den))
	extra.Mul(big.NewInt(int64(b.BaseDamageFlat)), &div)
	value.Add(&value, &extra)
	extra.Mul(big.NewInt(int64(mdef)*63), big.NewInt(den))
	value.Sub(&value, &extra)
	value.Quo(&value, &div)
	value.Sub(&value, big.NewInt(int64(resist)))
	if !value.IsInt64() {
		if value.Sign() > 0 {
			return 2147483647
		}
		return 1
	}
	return boundedDamage(value.Int64())
}
