package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) usePlayerExperienceItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	ch := p.Player.Char
	if ch.Level >= domain.LevelCap {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "已达等级上限，经验道具未消耗"})
		return
	}
	experience := item.PlayerExperience[ch.Level]
	if experience <= 0 {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "该道具尚无当前等级的经验配置，物品未消耗"})
		return
	}
	if s.levels.Need(ch.Level) <= 0 || ch.Exp < 0 || ch.Exp > math.MaxInt64-experience {
		reject(event.RejectInvalid)
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	beforeLevel := ch.Level
	// 固定经验不叠加经验卡、状态或宠物倍率，也不向宠物分享。
	// grantExpExact 复用连升、成长、客户端属性快照与角色存档链路。
	s.grantExpExact(p, experience)
	startCooldown()
	s.pushInventory(p)
	s.log.Info("使用角色经验道具", "char", ch.Name, "item", item.ID,
		"level", beforeLevel, "experience", experience, "toLevel", ch.Level)
}
