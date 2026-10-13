package session

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// clientMapReadyReceipt is connection-local, server-only state. The native ready
// can arrive while an earlier movement is still queued on the source actor. Keep
// its exact source binding so that actor's immediate ordinary portal handoff can
// inherit it instead of dropping the later ready command with the retired player.
type clientMapReadyReceipt struct {
	serial     uint64
	submitting bool // Reserves the one Post attempt while the ownership lock is released.
	submitted  bool // Set only after the source mailbox accepted that attempt.
	entity     domain.EntityID
	from       domain.SceneID
	epoch      uint64
	mapID      int32
	x, y       int32
}

func (s *Session) registerClientMapReadyLocked(id domain.EntityID, from domain.SceneID, mapID, x, y int32) uint64 {
	old := s.clientMapReady
	if old != nil && old.entity == id && old.from == from && old.epoch == s.mapEpoch &&
		old.mapID == mapID && old.x == x && old.y == y {
		return old.serial
	}
	s.clientMapReadySerial++
	// Zero is reserved for existing internal callers without a network receipt.
	if s.clientMapReadySerial == 0 {
		s.clientMapReadySerial++
	}
	s.clientMapReady = &clientMapReadyReceipt{serial: s.clientMapReadySerial,
		entity: id, from: from, epoch: s.mapEpoch, mapID: mapID, x: x, y: y}
	return s.clientMapReadySerial
}

func (s *Session) matchesClientMapReadyLocked(id domain.EntityID, from, to domain.SceneID, at domain.Pos) bool {
	r := s.clientMapReady
	return r != nil && !s.awaitMapAck && r.entity == id && r.from == from && r.epoch == s.mapEpoch &&
		from.MapID != to.MapID && to.Persistent() && r.mapID == to.MapID && at.MapID == to.MapID &&
		float64(r.x) == at.X && float64(r.y) == at.Y
}

func (s *Session) retireClientMapReady(serial uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clientMapReady != nil && s.clientMapReady.serial == serial {
		s.clientMapReady = nil
		return true
	}
	return false
}

func (s *Session) postClientMapReady(id domain.EntityID, from domain.SceneID, req protocol.Request, receipt uint64) {
	s.mu.Lock()
	r := s.clientMapReady
	current := r != nil && r.serial == receipt && !r.submitting && !r.submitted
	if current {
		r.submitting = true
	}
	s.mu.Unlock()
	// A duplicate shares the reserved/accepted attempt; a source transfer/newer
	// receipt can also retire this command before the network handler resumes.
	if !current {
		return
	}
	accepted := s.deps.Router != nil && s.deps.Router.Post(from, scene.Teleport{
		ID: id, To: domain.SceneID{MapID: req.MapID},
		At:             domain.Pos{MapID: req.MapID, X: float64(req.X), Y: float64(req.Y)},
		ClientMapReady: true, ClientReadyReceipt: receipt,
	})
	s.mu.Lock()
	closeFailed := false
	if current := s.clientMapReady; current != nil && current.serial == receipt {
		if accepted {
			current.submitting, current.submitted = false, true
		} else {
			s.clientMapReady = nil
			closeFailed = true
		}
	}
	s.mu.Unlock()
	// A portal may have consumed this receipt between registration and Post.
	// A failed redundant old-source command must not close its new valid owner.
	// A genuinely failed first attempt always retires and closes, never waits.
	if closeFailed {
		s.sink.Close()
	}
}

func (s *eventSink) ClientMapReadyCurrent(id domain.EntityID, from domain.SceneID, serial uint64) bool {
	if s.sess == nil {
		return false
	}
	owner := s.sess
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.clientMapReadyCurrentLocked(id, from, serial)
}

func (s *Session) clientMapReadyCurrentLocked(id domain.EntityID, from domain.SceneID, serial uint64) bool {
	r := s.clientMapReady
	return s.stage == StageInGame && s.entity == id && s.scene == from &&
		!s.awaitMapAck && r != nil && r.serial == serial && r.entity == id &&
		r.from == from && r.epoch == s.mapEpoch
}

func (s *eventSink) RetireClientMapReady(serial uint64) {
	if s.sess != nil {
		s.sess.retireClientMapReady(serial)
	}
}

func (s *eventSink) TransferSceneWithClientReady(id domain.EntityID, from, to domain.SceneID,
	at domain.Pos, portal bool, receipt uint64, directReady bool, handoff func(bool) bool) bool {
	if s.sess == nil {
		return handoff(false)
	}
	return s.sess.transferSceneWithClientReady(id, from, to, at, portal, receipt, directReady, handoff)
}
