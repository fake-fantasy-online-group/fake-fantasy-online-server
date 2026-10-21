package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// classSetPieces resolves the same functional equipment as the normal stat
// pipeline. Never call it while that pipeline is being prepared; callers there
// must pass their already-resolved worn set to classSetPiecesInWorn.
func (s *Scene) classSetPieces(p *entity.Entity, suitID int32) int32 {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return 0
	}
	return s.classSetPiecesInWorn(p, s.functionalWorn(p), suitID)
}

func (s *Scene) classSetPiecesInWorn(p *entity.Entity, worn *domain.EquipSet, suitID int32) int32 {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil || worn == nil {
		return 0
	}
	var race domain.Race
	switch suitID {
	case 1:
		race = domain.Assassin
	case 2:
		race = domain.Healer
	case 3:
		race = domain.Warlock
	case 4:
		race = domain.Swordsman
	case 5:
		race = domain.Warrior
	default:
		return 0
	}
	if p.Player.Char.Race != race {
		return 0
	}
	var suit *domain.EquipmentSuit
	worn.Each(func(_ domain.EquipSlot, stack domain.Stack) {
		if suit != nil {
			return
		}
		def, ok := s.itemDef(stack.Item)
		if ok && def.Equip != nil && def.Equip.Suit != nil && def.Equip.Suit.ID == suitID && domain.EquipmentFunctional(stack, def) {
			suit = def.Equip.Suit
		}
	})
	if suit == nil {
		return 0
	}
	pieces, _ := suit.WornMembers(worn, s.itemDef)
	return pieces
}

// classSetSkillText describes the server-owned rules alongside the native
// database bonuses. Values are not inferred from unrelated imported card IDs.
func classSetSkillText(suitID, pieces int32) string {
	switch suitID {
	case 1:
		if pieces == 4 {
			return "淬毒术总伤害额外增加物理攻击均值的1000%"
		}
		if pieces == 6 {
			return "再次施加淬毒时，额外造成原淬毒剩余伤害的20%；光环：自身及300范围内队友物理/法术暴击率+5个百分点，同类不叠加"
		}
	case 2:
		if pieces == 4 {
			return "治疗友军时自身获得等额治疗；每3秒恢复3%最大法力"
		}
		if pieces == 6 {
			return "伤害技能附带治愈术，治疗技能附带光弹术；单体技能作用于目标周围200范围，群体技能使用原范围；光环：自身及300范围内队友受到魔法伤害减少8%，同类不叠加"
		}
	case 3:
		if pieces == 4 {
			return "冰系技能使火系伤害增加50%，火系技能使冰系伤害增加50%，持续3秒"
		}
		if pieces == 6 {
			return "火系技能命中目标时追加火弹术；冰系技能命中目标时追加冰箭术；光环：自身及300范围内队友物理/法术暴击伤害+20个百分点，同类不叠加"
		}
	case 4:
		if pieces == 4 {
			return "挑衅获得一层不羁：每层减伤5%，持续30秒，最多10层"
		}
		if pieces == 6 {
			return "旋风剑消耗一层不羁，使本次伤害提高100%；光环：自身及300范围内队友受到物理伤害减少8%，同类不叠加"
		}
	case 5:
		if pieces == 4 {
			return "按已损失生命比例提高伤害"
		}
		if pieces == 6 {
			return "常驻龙人：生命翻倍、攻击+60%、受到伤害+50%、技能消耗10%当前生命代替法力、吸血20%、移动速度+20%；光环：自身及300范围内队友最大生命+15%，同类不叠加"
		}
	}
	return ""
}
