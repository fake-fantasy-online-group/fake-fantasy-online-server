package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// monsterSpeak 把已确认的 RES_SCENE_* 动作接到对应原文。原客户端的
// ResRatio.DialogRatio 没有可用的数据表；当前运行策略是每种动作每次出生
// 至多一句，并按实体号稳定选原文，不把概率猜测伪装成原作规则。
func (s *Scene) monsterSpeak(m *entity.Entity, scene uint8) {
	if m == nil || m.Monster == nil || s.monsterDialogs == nil || scene >= 8 || len(s.players) == 0 {
		return
	}
	bit := uint8(1) << scene
	if m.Monster.SpokenScenes&bit != 0 {
		return
	}
	lines := s.monsterDialogs.SceneLines(m.Monster.TypeID, scene)
	if len(lines) == 0 {
		return
	}
	text := lines[uint64(m.ID)%uint64(len(lines))]
	m.Monster.SpokenScenes |= bit
	s.emit(event.MonsterSpoke{Who: m.ID, Text: text})
}

// sendIdleMonsterGreetingTo 在玩家完成地图加载、已收到怪物出场包之后，
// 让附近最近的一只闲置怪说一句 NORMAL。只给这名玩家一次入图展示，
// 不改变怪物战斗时使用的每次出生标记。
func (s *Scene) sendIdleMonsterGreetingTo(player *entity.Entity) {
	if player == nil || s.monsterDialogs == nil {
		return
	}
	var nearest *entity.Entity
	var nearestDist float64
	for _, m := range s.monsters {
		if m == nil || m.Monster == nil || !m.Alive() || m.Monster.Target != 0 {
			continue
		}
		dist := sqDist(m.Pos, player.Pos)
		viewDist := float64(m.Monster.ViewDist)
		if viewDist <= 0 {
			viewDist = defaultCellSize
		}
		if dist > viewDist*viewDist ||
			len(s.monsterDialogs.SceneLines(m.Monster.TypeID, domain.MonsterSceneNormal)) == 0 {
			continue
		}
		if nearest == nil || dist < nearestDist || (dist == nearestDist && m.ID < nearest.ID) {
			nearest, nearestDist = m, dist
		}
	}
	if nearest == nil {
		return
	}
	lines := s.monsterDialogs.SceneLines(nearest.Monster.TypeID, domain.MonsterSceneNormal)
	text := lines[uint64(nearest.ID)%uint64(len(lines))]
	s.emitTo(player.ID, event.MonsterSpoke{Who: nearest.ID, Text: text})
}
