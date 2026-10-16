package domain

import "testing"

func TestMergeSnapshotsPreservesDirtyAggregate(t *testing.T) {
	full := Snapshot{Char: &Character{ID: 1, Level: 20}, Bag: NewBag(3)}
	full.Bag.Set(0, Stack{Item: 1, Count: 2})
	pos := Snapshot{Parts: SnapshotPosition, Char: &Character{ID: 1, Pos: Pos{MapID: 2, X: 3, Y: 4}, SceneInstance: 5}}
	merged := MergeSnapshots(full, pos)
	if merged.Parts != 0 || merged.Char.Level != 20 || merged.Char.Pos != pos.Char.Pos || merged.Char.SceneInstance != 5 || merged.Bag.At(0).Count != 2 {
		t.Fatalf("lost older full mutation: %+v", merged)
	}
	if full.Char.Pos == pos.Char.Pos {
		t.Fatal("merge mutated older in-flight snapshot")
	}
	merged.Bag.Set(0, Stack{})
	if full.Bag.At(0).Count != 2 {
		t.Fatal("merge aliases inventory")
	}
	if got := MergeSnapshots(pos, full); got.Char.Level != 20 || got.Parts != 0 {
		t.Fatal("full update must supersede position")
	}
	if got := pos.Clone(); got.Parts != SnapshotPosition {
		t.Fatal("clone lost selective metadata")
	}
}
