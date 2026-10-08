package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// onOpenTransport 回应正式客户端 0x1071。列表号必须同时属于当前地图的
// PostgreSQL 配置和当前场景里真实存在的 NPC，不能让客户端枚举全服菜单。
func (s *Scene) onOpenTransport(cmd OpenTransport) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	reject := func(reason event.RejectReason) {
		p.Player.TransportList = 0
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: cmd.CmdName(), Reason: reason})
	}
	if !p.Alive() || cmd.List <= 0 {
		reject(event.RejectInvalid)
		return
	}
	list, ok := s.transports[cmd.List]
	if !ok || list.SourceMapID != s.id.MapID || !s.hasTransportNPC(cmd.List) ||
		len(list.Destinations) == 0 {
		reject(event.RejectNoTarget)
		return
	}
	for _, dest := range list.Destinations {
		if dest.RequireClear && !s.dungeonFloorCleared() {
			reject(event.RejectDungeonNotClear)
			return
		}
	}
	p.Player.TransportList = cmd.List
	s.emitTo(p.ID, event.TransportDestinations{
		Who: p.ID, List: cmd.List, Destinations: list.Destinations,
	})
	s.log.Debug("打开传送列表", "char", p.Name, "list", cmd.List,
		"目的地", len(list.Destinations))
}

// onChooseTransport 处理正式客户端 0x1072。客户端只决定零基索引，地图、坐标
// 和价格全部重新从刚打开过的服务端配置中解析。
func (s *Scene) onChooseTransport(cmd ChooseTransport) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	reject := func(reason event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: cmd.CmdName(), Reason: reason})
	}
	if !p.Alive() || cmd.List <= 0 || cmd.Index < 0 || p.Player.TransportList != cmd.List {
		reject(event.RejectInvalid)
		return
	}
	list, ok := s.transports[cmd.List]
	if !ok || list.SourceMapID != s.id.MapID || !s.hasTransportNPC(cmd.List) {
		p.Player.TransportList = 0
		reject(event.RejectNoTarget)
		return
	}
	var dest *domain.TransportDestination
	for i := range list.Destinations {
		if list.Destinations[i].Index == cmd.Index {
			dest = &list.Destinations[i]
			break
		}
	}
	if dest == nil || dest.Pos.MapID <= 0 || dest.Price != 0 {
		reject(event.RejectInvalid)
		return
	}

	// 选择上下文一次性消费；跨场景也不会把它带到目标 actor。
	p.Player.TransportList = 0
	if dest.Dungeon {
		charName, dungeonName := p.Name, dest.Name
		if dest.RequireClear && !s.dungeonFloorCleared() {
			reject(event.RejectDungeonNotClear)
			return
		}
		if s.dungeon != nil {
			if target, ok := s.dungeons[dest.Pos.MapID]; ok && target.ID == s.dungeon.ID {
				if !s.enterDungeonFloor(p.ID, dest.Pos, 0) {
					return
				}
				s.log.Debug("选择副本楼层", "char", charName, "list", cmd.List,
					"index", cmd.Index, "map", dest.Pos.MapID, "name", dungeonName)
				return
			}
		}
		s.onEnterDungeon(EnterDungeon{ID: p.ID, MapID: dest.Pos.MapID})
		s.log.Debug("选择副本入口", "char", charName, "list", cmd.List,
			"index", cmd.Index, "map", dest.Pos.MapID, "name", dungeonName)
		return
	}
	to := domain.SceneID{MapID: dest.Pos.MapID}
	charName, destName := p.Name, dest.Name
	if !s.onTeleport(Teleport{ID: p.ID, To: to, At: dest.Pos}) {
		reject(event.RejectUnknown)
		return
	}
	s.log.Debug("选择传送目的地", "char", charName, "list", cmd.List,
		"index", cmd.Index, "to", to, "name", destName)
}

func (s *Scene) hasTransportNPC(list int32) bool {
	for _, e := range s.entities {
		if e.Kind == domain.KindNPC && e.NPC != nil && e.NPC.Trans == list {
			return true
		}
	}
	return false
}
