package domain

import "math"

// SaturateInt32 narrows a wide combat/stat intermediate only after clamping.
// Signed values are retained for debuffs and periodic resource changes.
func SaturateInt32(value int64) int32 {
	return int32(max(int64(math.MinInt32), min(value, int64(math.MaxInt32))))
}
