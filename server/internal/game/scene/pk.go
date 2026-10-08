package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const clientPKProtectLevel int32 = 20

// pkStateSnapshot 明确表达当前 PVP 边界：保留玩家的模式偏好，但所有地图
// 都按安全区处理，罪恶值和可互伤状态均为 0。
func pkStateSnapshot(p *entity.Entity) event.PKStateSnapshot {
	return event.PKStateSnapshot{
		Who: p.ID, Mode: p.Player.Char.PKMode,
		Karma: 0, State: 0, MapSafe: true, ProtectLevel: clientPKProtectLevel,
	}
}

func (s *Scene) onSetPKMode(cmd SetPKMode) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || cmd.Mode > 4 {
		return
	}
	p.Player.Char.PKMode = cmd.Mode
	p.Player.MarkDirty()
	s.emitTo(p.ID, pkStateSnapshot(p))
	// 0x8053 表达的是当前可互伤状态，不是模式偏好。PVP 未开放时无论玩家
	// 选择哪种模式，旁观者都只收到中性状态 0。
	s.emitExcept(event.PlayerPKStateChanged{Who: p.ID, State: 0}, p.ID)
	if s.saver != nil && !p.Player.TradeBusy {
		s.saver.Save(s.snapshotOf(p))
	}
}
