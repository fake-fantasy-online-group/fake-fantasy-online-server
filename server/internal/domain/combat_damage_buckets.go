package domain

// CombatDamageBuckets is the resolved numerical input to a direct hit's
// modifier stage. Source adapters (equipment, conditional powers, status,
// auras and pets) add values here; formulas never inspect their identities.
// It does not recompute Stats or the native attack/defense/critical formula.
// Immunity, reflection, consumable shields and HP commitment remain lifecycle
// operations after/beside these arithmetic stages.
type CombatDamageBuckets struct {
	DamageIncreaseBP          int64
	PVPScaledDamageIncreaseBP int64
	// Scaled contributions retain fractional BP until the common sum is rounded.
	ScaledDamageIncreaseNumerator              int64
	ResistanceBP, ResistancePierceBP           int32
	MitigationBP                               int64
	DamageTakenIncreasePct, DamageReductionPct int32
	FlatReduction                              int64
	HalfDamageLayers                           int
	PayloadScaleBP                             int32
	PayloadScaleKnown                          bool
	PVP                                        bool
}

// Additive bonuses share the existing +100% ceiling. This ceiling is retained
// for compatibility, not asserted as a new balance requirement. Elemental
// bonuses currently have half PvP effectiveness, applied once to their bucket.
func (b CombatDamageBuckets) OutgoingDamage(amount int32) int32 {
	scale := int64(10000)
	if b.PVP {
		scale = 5000
	}
	scaled := (b.PVPScaledDamageIncreaseBP*scale + b.ScaledDamageIncreaseNumerator) / 10000
	bonus := min(int64(10000), max(int64(0), b.DamageIncreaseBP+scaled))
	amount = max(int32(1), ApplyUniqueBP(amount, int32(bonus)))
	if b.PayloadScaleKnown || b.PayloadScaleBP > 0 {
		amount = max(int32(1), SaturateInt32(int64(amount)*int64(b.PayloadScaleBP)/10000))
	}
	return amount
}

// IncomingDamage retains the existing ordered multiplicative layers and the
// post-native-defense 75% combined reduction cap. True shield pools and
// immunity can still prevent all damage and are deliberately not this bucket.
func (b CombatDamageBuckets) IncomingDamage(amount int32) int32 {
	taken := max(int32(0), b.DamageTakenIncreasePct)
	wide := int64(amount) + int64(amount)*int64(taken)/100
	wide = MitigatePercentDamageV7(wide, b.ResistanceBP, b.ResistancePierceBP,
		SaturateInt32(b.MitigationBP), b.DamageReductionPct, b.HalfDamageLayers, b.PVP, b.FlatReduction)
	return max(int32(0), SaturateInt32(wide))
}
