package session

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (s *eventSink) BeginMapLoad(id domain.EntityID, sc domain.SceneID) (uint64, bool) {
	if s.sess == nil {
		return 0, true
	}
	owner := s.sess
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.stage != StageInGame || owner.entity != id || owner.scene != sc {
		return 0, false
	}
	if !owner.awaitMapAck {
		owner.clientMapReady = nil
		owner.mapEpoch++
		owner.awaitMapAck = true
	}
	return owner.mapEpoch, true
}

func (s *eventSink) CompleteMapLoad(id domain.EntityID, sc domain.SceneID, epoch uint64) bool {
	if s.sess == nil {
		return false
	}
	owner := s.sess
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.stage != StageInGame || owner.entity != id || owner.scene != sc ||
		!owner.awaitMapAck || owner.mapEpoch != epoch {
		return false
	}
	owner.awaitMapAck = false
	return true
}

// CurrentSceneReady rejects a queued rebuild acknowledgment once another load,
// transfer or session teardown has changed the binding captured by onMove.
func (s *eventSink) CurrentSceneReady(id domain.EntityID, sc domain.SceneID, epoch uint64) bool {
	if s.sess == nil {
		return false
	}
	owner := s.sess
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.stage == StageInGame && owner.entity == id && owner.scene == sc &&
		!owner.awaitMapAck && owner.mapEpoch == epoch
}

// ConfirmClientGateReady commits a same-map native gate only while the exact
// settled receipt binding still owns it. This is the already-built equivalent
// of BeginMapLoad + CompleteMapLoad, atomically fenced against teardown/rebind.
func (s *eventSink) ConfirmClientGateReady(id domain.EntityID, sc domain.SceneID, epoch uint64) bool {
	if s.sess == nil {
		return false
	}
	owner := s.sess
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.stage != StageInGame || owner.entity != id || owner.scene != sc || owner.awaitMapAck || owner.mapEpoch != epoch {
		return false
	}
	owner.clientMapReady = nil
	owner.mapEpoch++
	// The old-world flush may have captured a refused reconciliation Remove.
	// Keep that scene queue head, but re-encode it against this new binding on
	// retry. Do not retire the transport epoch: accepted ordered removals must
	// still precede the rebuilt snapshot.
	s.CancelPendingEmit()
	return true
}
