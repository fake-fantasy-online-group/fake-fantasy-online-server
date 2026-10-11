package session

import (
 "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
 "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
 "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)
func (s *Session) endgameEnabled() bool { s.mu.Lock();defer s.mu.Unlock();return s.stage==StageInGame&&s.endgameV1 }
func (s *Session) onEndgame(payload []byte) {
 req,ok:=protocol.DecodeEndgame(payload);if !ok{return}
 s.mu.Lock()
 if s.stage!=StageInGame||s.awaitMapAck{s.mu.Unlock();return}
 if req.Op=="hello" {
  for _,v:=range req.Capabilities{if v==domain.EndgameV1{s.endgameV1=true}}
  enabled:=s.endgameV1;s.mu.Unlock()
  reply:=domain.EndgameResponse{Kind:"hello",RequestID:req.RequestID,OK:enabled}
  if enabled{reply.Capabilities=[]string{domain.EndgameV1}}else{reply.Error="unsupported_capability"}
  s.sink.Send(protocol.EncodeEndgame(reply));return
 }
 enabled,id:=s.endgameV1,s.entity;s.mu.Unlock()
 if enabled&&s.deps.Router!=nil{s.postToScene(scene.EndgameRequest{ID:id,Request:req})}
}
