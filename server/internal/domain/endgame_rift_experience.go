package domain

import "math"

// RiftBossExperience scales the existing per-character trial base, not a trial
// round multiplier or monster XP. Keep the fraction until the final division.
func RiftBossExperience(base int64, tier int32, rule *RiftRules) int64 {
	if rule == nil {
		return 0
	}
	if base <= 0 || tier < 1 || tier > rule.MaxTier {
		return 0
	}
	// Quotient/remainder is exactly base*tier/10 without an overflowing product.
	q, rem := base/int64(rule.ExperienceDivisor), base%int64(rule.ExperienceDivisor)
	extra := rem * int64(tier) / int64(rule.ExperienceDivisor)
	if q > (math.MaxInt64-extra)/int64(tier) {
		return math.MaxInt64
	}
	return q*int64(tier) + extra
}
