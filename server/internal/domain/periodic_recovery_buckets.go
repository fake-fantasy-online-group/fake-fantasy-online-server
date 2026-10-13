package domain

// PeriodicRecoveryBuckets adds flat recovery within one cadence. Different
// cadences are distinct dimensions, never summed into a fictitious common rate.
// The bucket carries no item, status or effect identity. Source eligibility and
// clocks belong to the scene; restoration uses the ordinary bounded HP API.
type PeriodicRecoveryBuckets map[int32]int64

func ValidRecoveryIntervalMS(ms int32) bool {
	return ms >= int32(TickMS) && ms <= 60000 && ms%int32(TickMS) == 0
}
func (b PeriodicRecoveryBuckets) AddFlatHP(intervalMS int32, amount int64) {
	if b == nil || !ValidRecoveryIntervalMS(intervalMS) || amount <= 0 {
		return
	}
	b[intervalMS] += amount
}
