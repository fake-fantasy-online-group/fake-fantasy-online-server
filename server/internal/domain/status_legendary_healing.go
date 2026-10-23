package domain

// HealingReceivedAmount applies the strongest active non-stacking reduction.
// It changes health restoration only, never maximum HP or mana restoration.
func (s *StatusSet) HealingReceivedAmount(amount int32) int32 {
	if amount <= 0 {
		return 0
	}
	pct := int32(0)
	if s != nil {
		s.EachEffective(func(a Active) { pct = max(pct, a.Def.HealingReceivedReductionPct) })
	}
	return SaturateInt32(int64(amount) * int64(100-min(100, max(0, pct))) / 100)
}
