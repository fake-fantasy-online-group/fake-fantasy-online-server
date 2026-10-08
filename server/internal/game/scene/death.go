package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// recordPlayerDeathLoss 在死亡瞬间一次性扣除并保存可返还的实际值。场景只
// 记录这次尸体的损失，因此复活技能不会受复活时余额变化影响。
func (s *Scene) recordPlayerDeathLoss(p *entity.Entity) {
	if p == nil || p.Player == nil || p.Player.Char == nil || !p.Player.DeathLoss.Empty() {
		return
	}
	if s.deferTradeMutation(p.ID, func() { s.recordPlayerDeathLoss(p) }) {
		return
	}
	ch := p.Player.Char
	money, err := inventoryMoney(ch.Money)
	if err != nil || money < 0 {
		s.log.Error("死亡损失读取随身金钱失败，只结算经验", "char", p.Name, "err", err)
		money = 0
	}
	loss := s.death.Loss(ch.Level, ch.Exp, money)
	if loss.Empty() {
		s.notifyPlayerDeathLoss(p, loss)
		return
	}
	ch.Exp -= loss.Exp
	if ch.Exp < 0 {
		ch.Exp = 0
	}
	if loss.Money > money {
		loss.Money = money
	}
	if loss.Money > 0 {
		ch.Money = moneyFromInventory(money - loss.Money)
	}
	p.Player.DeathLoss = loss
	p.Player.MarkDirty()
	s.emitTo(p.ID, s.attributeSnapshot(p))
	if loss.Money > 0 {
		if err := s.pushInventory(p); err != nil {
			s.log.Error("死亡扣除金钱后推送背包失败", "char", p.Name, "err", err)
		}
	}
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.notifyPlayerDeathLoss(p, loss)
	s.log.Info("玩家死亡损失", "char", p.Name, "exp", loss.Exp, "money", loss.Money)
}

func (s *Scene) notifyPlayerDeathLoss(p *entity.Entity, loss domain.DeathLoss) {
	money := moneyFromInventory(loss.Money)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: fmt.Sprintf("你被扣除了%d点经验值", loss.Exp)})
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: fmt.Sprintf("你被扣除了%d金币%d银%d铜", money.Gold, money.Silver, money.Copper)})
}

// refundPlayerDeathLoss 消费尸体上的可返还记录。未被本次技能返还的部分永久
// 损失；记录整体清空，防止同一次死亡被多个复活技能重复领取。
func (s *Scene) refundPlayerDeathLoss(p *entity.Entity, pct int32) domain.DeathLoss {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return domain.DeathLoss{}
	}
	loss := p.Player.DeathLoss
	p.Player.DeathLoss = domain.DeathLoss{}
	refund := loss.Refund(pct)
	ch := p.Player.Char
	ch.Exp += refund.Exp
	if refund.Money > 0 {
		money, err := inventoryMoney(ch.Money)
		if err != nil || money < 0 || money > int64(^uint64(0)>>1)-refund.Money {
			s.log.Error("复活返还金钱失败", "char", p.Name, "refund", refund.Money, "err", err)
			refund.Money = 0
		} else {
			ch.Money = moneyFromInventory(money + refund.Money)
		}
	}
	return refund
}

// reviveBySkill 在原地复活死亡玩家，并按技能参数恢复生命、返还本次死亡
// 损失。普通治疗不能把 HP=0 的实体抬活，必须走 0x801a 的正式复活协议。
func (s *Scene) reviveBySkill(src, dst *entity.Entity, def domain.SkillDef, effect domain.SkillEffect) bool {
	if src == nil || dst == nil || dst.Alive() || dst.Kind != domain.KindPlayer || dst.Player == nil {
		return false
	}
	if s.deferTradeMutation(dst.ID, func() { s.reviveBySkill(src, dst, def, effect) }) {
		return true
	}
	hp := int64(dst.MaxHP) * int64(effect.HealPctOfMax) / 100
	if hp < 1 {
		hp = 1
	}
	if hp > int64(dst.MaxHP) {
		hp = int64(dst.MaxHP)
	}
	dst.HP = int32(hp)
	dst.Player.Protect(s.tick + reviveGraceTicks)
	refund := s.refundPlayerDeathLoss(dst, effect.LossRefundPct)
	dst.Player.MarkDirty()

	s.emit(event.PlayerRevived{Who: dst.ID})
	if refund.Exp > 0 {
		s.emitTo(dst.ID, event.ExpGained{Who: dst.ID, Delta: refund.Exp, Total: dst.Player.Char.Exp})
	}
	s.emitTo(dst.ID, s.attributeSnapshot(dst))
	if refund.Money > 0 {
		if err := s.pushInventory(dst); err != nil {
			s.log.Error("复活返还金钱后推送背包失败", "char", dst.Name, "err", err)
		}
	}
	s.emit(s.spawnEvent(dst))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(dst))
	}
	s.log.Info("技能复活玩家", "caster", src.Name, "target", dst.Name, "skill", def.ID,
		"hp", dst.HP, "refundExp", refund.Exp, "refundMoney", refund.Money)
	return true
}
