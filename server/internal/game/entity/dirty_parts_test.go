package entity

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestDirtyPartsNeverNarrowFullMutation(t *testing.T) {
	p := &Player{}
	p.MarkPositionDirty()
	if !p.Dirty() || p.TakeDirtyParts() != domain.SnapshotPosition || p.Dirty() {
		t.Fatal("position dirty lifecycle")
	}
	p.MarkPositionDirty()
	p.MarkDirty()
	p.MarkPositionDirty()
	if p.TakeDirtyParts() != 0 {
		t.Fatal("position update narrowed full snapshot")
	}
	p.MarkDirty()
	p.MarkPositionDirty()
	if p.TakeDirtyParts() != 0 {
		t.Fatal("full snapshot was lost")
	}
	p.MarkPositionDirty()
	p.TakeDirty()
	p.MarkDirty()
	if p.TakeDirtyParts() != 0 {
		t.Fatal("legacy TakeDirty leaked parts")
	}
}
