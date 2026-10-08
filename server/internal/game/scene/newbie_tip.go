package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// requestNewbieTip 只表达“一个权威玩法结果已经满足提示条件”。是否已展示、
// 文案如何编码和何时持久化都属于会话/协议边界，不进入场景状态。
func (s *Scene) requestNewbieTip(who domain.EntityID, tipID int32) {
	s.emitTo(who, event.NewbieTipRequested{Who: who, ID: tipID})
}

func (s *Scene) onRequestNewbieTip(cmd RequestNewbieTip) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil {
		s.requestNewbieTip(cmd.ID, cmd.Tip)
	}
}

// requestFirstKillTip 将宠物补刀归到主人；只有真实玩家击杀怪物才满足“首次击杀”。
// 怪物击杀宠物/NPC 等其它死亡结果不能借用这条提示。
func (s *Scene) requestFirstKillTip(dead *entity.Entity, killer domain.EntityID) {
	if dead == nil || dead.Monster == nil || killer == 0 {
		return
	}
	if pet, ok := s.pets[killer]; ok && pet.Pet != nil {
		killer = pet.Pet.Owner
	}
	if p, ok := s.players[killer]; ok && p.Player != nil {
		s.requestNewbieTip(p.ID, 1)
	}
}
