package domain

import "math"

// RiftBossExperience scales the existing per-character trial base, not a trial
// round multiplier or monster XP. Keep the fraction until the final division.
func RiftBossExperience(base int64, tier int32, rule *RiftRules) int64 {
	if rule == nil || rule.ExperienceDivisor < 1 {
		return 0
	}
	if base <= 0 || tier < 1 || tier > rule.MaxTier {
		return 0
	}
	// Add one full trial base: base * (1 + tier/10) with published rules.
	// Quotient/remainder preserves the fraction without an overflowing product.
	multiplier := int64(rule.ExperienceDivisor) + int64(tier)
	q, rem := base/int64(rule.ExperienceDivisor), base%int64(rule.ExperienceDivisor)
	extra := rem * multiplier / int64(rule.ExperienceDivisor)
	if q > (math.MaxInt64-extra)/multiplier {
		return math.MaxInt64
	}
	return q*multiplier + extra
}
