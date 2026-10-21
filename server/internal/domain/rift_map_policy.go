package domain

// RiftMapExcluded removes confirmed unwanted scenery from new rift selections.
// It is deliberately separate from RiftFloorPlan.Valid: old durable records
// must remain readable for completed rewards and interruption compensation.
// Ordinary world access and native portal definitions are not changed.
func RiftMapExcluded(mapID int32) bool {
	switch mapID {
	case 48: // Current 1.0.1 gate to 49 lies entirely in blocked native MASK cells.
		return true
	case 60, 21000: // Existing special island/protocol alias exclusions.
		return true
	case 23800, 23900, 24000, 24100, 24200, 24300: // All pw23800 Huang Lei Island aliases.
		return true
	case 20100, 28502, 28503, 28504, 28505: // Mixed native/script-only Trap gates.
		return true
	case 24600, 24700, 24800, 36700: // No ordinary gates or missing native records: unverified, not proven broken.
		return true
	case 933, 934, 935, 936, 937: // Every native map with family stone monster 30069.
		return true
	}
	return false
}
