package domain

// MitigatePercentDamageV7 starts after the ordinary attack-vs-defense formula.
// It preserves multiplicative percentage layers but limits their combined
// reduction to 75%. Flat reductions share the same final budget; shield pools, reflection,
// and immunity are deliberately separate. Wide intermediates avoid amplification overflow.
func MitigatePercentDamageV7(amount int64, rawResist, pierce, equipmentBP, statusPct int32, halfCount int, pvp bool, flatReduction ...int64) int64 {
	pierceCap := int32(3000)
	if pvp {
		pierce /= 2
		pierceCap = 1500
	}
	pierce = min(pierceCap, max(int32(0), pierce))
	resist := max(int64(-5000), min(int64(7500), int64(rawResist)-int64(pierce)))
	baseline := amount
	// Negative elemental resistance is amplification, not mitigation.
	if resist < 0 {
		amount = scaleDamageWide(amount, 10000-resist, 10000)
		baseline = amount
	} else {
		amount = scaleDamageWide(amount, 10000-resist, 10000)
	}
	mitigation := max(int64(0), min(int64(4500), int64(equipmentBP)))
	amount = scaleDamageWide(amount, 10000-mitigation, 10000)
	for i := 0; i < halfCount; i++ {
		amount = (amount + 1) / 2
	}
	reduction := max(int64(0), min(int64(100), int64(statusPct)))
	// Preserve native percentage subtraction rounding for status reductions.
	amount -= scaleDamageWide(amount, reduction, 100)
	for _, flat := range flatReduction {
		amount -= max(int64(0), flat)
	}
	return max(amount, (baseline+3)/4)
}

// Splitting the quotient before multiplying keeps even large amplified damage
// safe without prematurely saturating it to the final HP wire representation.
func scaleDamageWide(amount, numerator, denominator int64) int64 {
	return (amount/denominator)*numerator + (amount%denominator)*numerator/denominator
}
