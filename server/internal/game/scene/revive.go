package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	ReviveTypeRespawn = 0 // 回城复活
	ReviveTypeInPlace = 1 // 原地复活(需要复活卷轴)
)

// onRevive 处理玩家主动复活请求。
//
// 死亡会一直保持，直到正式客户端在复活框中做出选择并发来
// 0x1018：回城(有回魂娃娃时消耗并免除损失)或原地(消耗通用替身娃娃)。
func (s *Scene) onRevive(cmd Revive) {
	p, ok := s.players[cmd.ID]
	if !ok {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "Revive", Reason: r})
	}

	// 只有死亡状态才能复活
	if p.Alive() {
		reject(event.RejectAlreadyAlive)
		return
	}

	switch cmd.ReviveType {
	case ReviveTypeRespawn:
		// 回魂娃娃不是普通双击物品。玩家在死亡面板选择回城时，若背包有
		// 可用娃娃则自动消耗并返还本次死亡损失；没有时仍可普通回城。
		if slot, item, found := s.cityReviveItem(p); found {
			if !p.Player.Bag.RemoveAt(slot, 1) {
				reject(event.RejectNoItem)
				return
			}
			refund := s.refundPlayerDeathLoss(p, 100)
			if refund.Exp > 0 {
				s.emitTo(p.ID, event.ExpGained{Who: p.ID, Delta: refund.Exp, Total: p.Player.Char.Exp})
			}
			if err := s.pushInventory(p); err != nil {
				s.log.Error("回魂娃娃生效后推送背包失败", "char", p.Name, "err", err)
			}
			s.log.Info("使用回魂娃娃回城复活", "char", p.Name, "item", item.Name,
				"refundExp", refund.Exp, "refundMoney", refund.Money)
		}
		s.doRevive(p, true)
	case ReviveTypeInPlace:
		slot, item, found := s.reviveItem(p)
		if !found {
			reject(event.RejectNotEnough)
			return
		}
		if !p.Player.Bag.RemoveAt(slot, 1) {
			reject(event.RejectNoItem)
			return
		}
		refund := s.refundPlayerDeathLoss(p, 100)
		s.doRevive(p, false)
		if refund.Exp > 0 {
			s.emitTo(p.ID, event.ExpGained{Who: p.ID, Delta: refund.Exp, Total: p.Player.Char.Exp})
		}
		if err := s.pushInventory(p); err != nil {
			s.log.Error("原地复活后推送背包失败", "char", p.Name, "err", err)
		}
		s.log.Info("使用物品原地复活", "char", p.Name, "item", item.Name,
			"refundExp", refund.Exp, "refundMoney", refund.Money)
	default:
		reject(event.RejectInvalid)
	}
}

func (s *Scene) cityReviveItem(p *entity.Entity) (int, domain.ItemDef, bool) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return -1, domain.ItemDef{}, false
	}
	slot, found := -1, false
	var selected domain.ItemDef
	p.Player.Bag.Each(func(index int, stack domain.Stack) {
		if found || stack.Empty() || stack.Locked {
			return
		}
		def, ok := s.itemDef(stack.Item)
		if !ok || !def.UseCityRevive || p.Player.Char.Level < def.UseLevel {
			return
		}
		slot, selected, found = index, def, true
	})
	return slot, selected, found
}

func (s *Scene) reviveItem(p *entity.Entity) (int, domain.ItemDef, bool) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return -1, domain.ItemDef{}, false
	}
	slot, found := -1, false
	var selected domain.ItemDef
	p.Player.Bag.Each(func(index int, stack domain.Stack) {
		if found || stack.Empty() || stack.Locked {
			return
		}
		def, ok := s.itemDef(stack.Item)
		if !ok || !def.UseRevive || p.Player.Char.Level < def.UseLevel {
			return
		}
		slot, selected, found = index, def, true
	})
	return slot, selected, found
}

// doRevive 执行复活逻辑(满血 + 保护 + 回城/原地)。
//
// respawn=true 表示回城, false 表示原地复活。
// 由 onRevive 和明确的 GM 复活能力调用；死亡流程自身不调用。
func (s *Scene) doRevive(e *entity.Entity, respawn bool) {
	id := e.ID
	e.HP = e.MaxHP
	e.MP = e.MaxMP
	// 主动回城/原地复活不会返还死亡损失；尸体的可返还记录至此失效。
	e.Player.DeathLoss = domain.DeathLoss{}
	e.Player.Protect(s.tick + reviveGraceTicks)
	e.Player.MarkDirty()

	s.emit(event.PlayerRevived{Who: id})
	s.emitTo(id, s.attributeSnapshot(e))

	if respawn && s.revive.Enabled() {
		// 回城。传送里会重新广播出场, 这里不用再发。
		name, to := e.Name, s.revive.Scene
		if s.onTeleport(Teleport{ID: id, To: to, At: s.revive.Pos}) {
			s.log.Info("玩家复活回城", "char", name, "id", id, "回", to)
			return
		}
		// 回城失败: 降级原地复活
		s.log.Error("玩家复活回城未执行, 降级原地复活", "char", name, "id", id, "回", to)
	}

	// 原地复活或回城失败
	s.emit(s.spawnEvent(e))
	s.log.Info("玩家原地复活", "char", e.Name, "id", id)
}
