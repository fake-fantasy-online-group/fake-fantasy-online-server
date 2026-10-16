package entity

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// MarkPositionDirty never narrows an already dirty full aggregate.
func (p *Player) MarkPositionDirty() {
	if !p.dirty {
		p.dirtyParts = domain.SnapshotPosition
	}
	p.dirty = true
}

// TakeDirtyParts must only be called after Dirty and reservation checks, when
// the caller is about to hand ownership of a snapshot to the retrying writer.
func (p *Player) TakeDirtyParts() domain.SnapshotParts {
	parts := p.dirtyParts
	p.TakeDirty()
	return parts
}
