package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"

// EmotePicker.Count=96 是现有客户端静态字段；发送编号超出这个范围只会让
// 客户端查不到内置资源，因此服务端在广播前做同一边界校验。
const clientEmoteCount int32 = 96

func (s *Scene) onShowEmote(cmd ShowEmote) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || cmd.Face < 0 || cmd.Face >= clientEmoteCount {
		return
	}
	s.emit(event.PlayerEmoted{Who: p.ID, Face: cmd.Face})
}
