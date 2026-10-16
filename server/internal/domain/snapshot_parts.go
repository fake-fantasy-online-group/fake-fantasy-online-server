package domain

// SnapshotParts is deliberately conservative: all unclassified mutations and
// legacy callers save the full aggregate. Only proven movement is selective.
type SnapshotParts uint8

const SnapshotPosition SnapshotParts = 1

// MergeSnapshots coalesces chronological snapshots without dropping an older
// inventory mutation when a newer snapshot contains only position. Neither
// input is mutated; snapshots may already be owned by an in-flight writer.
func MergeSnapshots(older, newer Snapshot) Snapshot {
	if newer.Parts != SnapshotPosition || older.Char == nil || newer.Char == nil || older.Char.ID != newer.Char.ID {
		return newer
	}
	if older.Parts == SnapshotPosition {
		return newer
	}
	merged := older.Clone()
	merged.Char.Pos, merged.Char.SceneInstance = newer.Char.Pos, newer.Char.SceneInstance
	return merged
}
