package session

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func (s *Session) endgameEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stage == StageInGame && s.endgameV1
}
func (s *Session) onEndgame(payload []byte) {
	req, ok := protocol.DecodeEndgame(payload)
	if !ok {
		return
	}
	s.mu.Lock()
	if s.stage != StageInGame || s.awaitMapAck {
		s.mu.Unlock()
		return
	}
	if req.Op == "hello" {
		// This is only the transport opt-in. The scene owns the capability
		// response and must refresh native descriptions before confirming it.
		enabled := false
		for _, v := range req.Capabilities {
			if v == domain.EndgameV1 {
				enabled = true
				s.endgameV1 = true
			}
		}
		id, sc := s.entity, s.scene
		s.mu.Unlock()
		if !enabled {
			s.sink.Send(protocol.EncodeEndgame(domain.EndgameResponse{Kind: "hello", RequestID: req.RequestID, Error: "unsupported_capability"}))
			return
		}
		// A queued hello may wait behind a busy actor before storage even starts.
		// Acknowledge supported transport synchronously, without confirming any
		// capability or scene ownership. Send before Post so the actor cannot
		// publish success followed by a late admission-pending response.
		s.sink.Send(protocol.EncodeEndgame(domain.EndgameResponse{Kind: "hello", RequestID: req.RequestID, Error: "pending", Message: "请求处理中，请稍候"}))
		if s.deps.Router == nil || !s.deps.Router.Post(sc, scene.EndgameRequest{ID: id, Request: req}) {
			s.sink.Send(protocol.EncodeEndgame(domain.EndgameResponse{Kind: "hello", RequestID: req.RequestID, Error: "busy", Message: "演劫系统暂时无法响应，请重试"}))
		}
		return
	}
	enabled, id := s.endgameV1, s.entity
	s.mu.Unlock()
	if enabled && s.deps.Router != nil {
		s.postToScene(scene.EndgameRequest{ID: id, Request: req})
	}
}

func (s *Session) growthReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stage == StageInGame && s.growthV5
}
