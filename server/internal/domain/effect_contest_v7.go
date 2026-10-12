package domain

// EffectiveStatusChanceBP keeps old in-memory/saved definitions compatible while
// allowing new configuration to explicitly disable an effect with zero chance.
func EffectiveStatusChanceBP(chance int32, explicit bool) int32 {
	if !explicit && chance <= 0 {
		return 10000
	}
	return chance
}

// EffectSuccessProbability evaluates the entire harmful effect once. Accuracy
// and resistance may exceed 100%; only the final probability is clamped. Negative
// ratings do not grant resistance penetration or create a non-positive denominator.
// Precision is retained to the final random draw, with no minimum success floor.
func EffectSuccessProbability(baseChanceBP int32, effectHitBP, effectResistBP int64) float64 {
	if baseChanceBP <= 0 {
		return 0
	}
	hit := max(int64(0), effectHitBP)
	resist := max(int64(0), effectResistBP)
	return min(1, float64(baseChanceBP)/10000*(1+float64(hit)/10000)/(1+float64(resist)/10000))
}
