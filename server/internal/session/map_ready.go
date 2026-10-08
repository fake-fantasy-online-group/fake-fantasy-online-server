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
