package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// GMAction 是会话层完成鉴权与参数解析后，允许投进场景的 GM 白名单。
// 场景不接收原始字符串，避免命令解析和权限逻辑渗入世界状态。
type GMAction uint8

const (
	GMAddExp GMAction = iota + 1
	GMAddMoney
	GMAddItem
	GMMaxResources
	GMTeleport
	GMAddGold
	GMAddSilver
	GMAddHonor
	GMTakeMoney
	GMTakeItem
	GMWhere
	GMTakeGold
	GMTakeSilver
	GMTakeHonor
	GMInspect
	GMGod
	GMOneShot
	GMPlayEffect
	GMOwnerSync
)

type GMStateMode uint8

const (
	GMStateQuery GMStateMode = iota
	GMStateEnable
	GMStateDisable
)

type GMCommand struct {
	Skill        domain.SkillID
	EffectTarget uint8
	ID           domain.EntityID
	Action       GMAction
	Amount       int64
	Item         domain.ItemID
	Count        int32
	To           domain.SceneID
	At           domain.Pos
	State        GMStateMode
	Reply        chan<- GMResult
}

func (GMCommand) CmdName() string { return "GMCommand" }

type GMResult struct {
	OK      bool
	Message string
}

func (s *Scene) onGMCommand(cmd GMCommand) {
	reply := func(ok bool, message string) {
		if cmd.Reply == nil {
			return
		}
		select {
		case cmd.Reply <- GMResult{OK: ok, Message: message}:
		default:
			s.log.Warn("GM 命令结果无人接收", "id", cmd.ID, "action", cmd.Action)
		}
	}
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil {
		reply(false, "角色当前不在场景中")
		return
	}

	switch cmd.Action {
	case GMPlayEffect:
		def, ok := s.skills.Get(cmd.Skill, 1)
		if !ok || def.HitEffect == "" {
			reply(false, "该技能没有已核实的实体特效")
			return
		}
		if cmd.EffectTarget > 1 || cmd.EffectTarget == 1 && p.Player.Pet == 0 {
			reply(false, "没有对应的特效目标")
			return
		}
		s.emitTo(p.ID, event.EntityEffectRequested{Who: p.ID, Target: cmd.EffectTarget, Effect: def.HitEffect})
		reply(true, "已播放实体特效："+def.HitEffect)
	case GMOwnerSync:
		pet := s.entities[p.Player.Pet]
		if pet == nil || pet.Pet == nil {
			reply(false, "没有出战宠物")
			return
		}
		s.emit(event.EntityOwnerChanged{Who: pet.ID, Owner: pet.Pet.Owner})
		reply(true, "已同步实体所有者")
	case GMAddExp:
		beforeLevel, beforeExp := p.Player.Char.Level, p.Player.Char.Exp
		s.grantExpExact(p, cmd.Amount)
		reply(true, fmt.Sprintf("经验已增加 %d（Lv%d %d → Lv%d %d）",
			cmd.Amount, beforeLevel, beforeExp, p.Player.Char.Level, p.Player.Char.Exp))
	case GMAddMoney:
		before := p.Player.Char.Money.Copper
		next, ok := checkedAdd64(before, cmd.Amount)
		if !ok || next < 0 {
			reply(false, "金钱数值溢出")
			return
		}
		p.Player.Char.Money.Copper = next
		p.Player.MarkDirty()
		if err := s.pushInventory(p); err != nil {
			p.Player.Char.Money.Copper = before
			reply(false, "背包快照生成失败，金钱未修改")
			return
		}
		reply(true, fmt.Sprintf("铜币已增加 %d，当前 %d", cmd.Amount, next))
	case GMAddGold:
		s.changeGMInventoryValue(p, &p.Player.Char.Money.Gold, cmd.Amount, "金币", "增加", reply)
	case GMAddSilver:
		s.changeGMInventoryValue(p, &p.Player.Char.Money.Silver, cmd.Amount, "银币", "增加", reply)
	case GMAddHonor:
		s.changeGMInventoryValue(p, &p.Player.Char.Honor, cmd.Amount, "名誉", "增加", reply)
	case GMTakeGold:
		s.takeGMInventoryValue(p, &p.Player.Char.Money.Gold, cmd.Amount, "金币", reply)
	case GMTakeSilver:
		s.takeGMInventoryValue(p, &p.Player.Char.Money.Silver, cmd.Amount, "银币", reply)
	case GMTakeHonor:
		s.takeGMInventoryValue(p, &p.Player.Char.Honor, cmd.Amount, "名誉", reply)
	case GMTakeMoney:
		before := p.Player.Char.Money.Copper
		if before < cmd.Amount {
			reply(false, fmt.Sprintf("铜币不足：当前 %d，需要扣除 %d", before, cmd.Amount))
			return
		}
		p.Player.Char.Money.Copper = before - cmd.Amount
		p.Player.MarkDirty()
		if err := s.pushInventory(p); err != nil {
			p.Player.Char.Money.Copper = before
			reply(false, "背包快照生成失败，铜币未扣除")
			return
		}
		reply(true, fmt.Sprintf("铜币已扣除 %d，当前 %d", cmd.Amount, before-cmd.Amount))
	case GMAddItem:
		def, ok := s.itemDef(cmd.Item)
		if !ok {
			reply(false, fmt.Sprintf("物品不存在：%d", cmd.Item))
			return
		}

		if def.PetCarrierSpecies > 0 {
			pet, ok := s.petDefs[def.PetCarrierSpecies]
			if !ok || !pet.RealPet || !s.petItemRoom(p, int(cmd.Count)) {
				reply(false, "宠物或背包空间无效")
				return
			}
			beforeBag, beforePets := p.Player.Bag.Clone(), domain.ClonePets(p.Player.Char.Pets)
			for i := int32(0); i < cmd.Count; i++ {
				inst := domain.NewPetInstance(pet)
				inst.ID, inst.Slot = s.nextPetInstID(p.Player.Char), s.nextPetSlot(p.Player.Char)
				if !s.appendPet(p, inst) {
					p.Player.Bag = beforeBag
					s.assignPetList(p, beforePets)
					reply(false, "宠物物品创建失败")
					return
				}
			}
			p.Player.MarkDirty()
			s.pushPetSnapshot(p)
			reply(true, "宠物已进入背包和宠物栏")
			return
		}
		candidate := p.Player.Bag.Clone()
		if left := candidate.Add(def, cmd.Count); left != 0 {
			reply(false, fmt.Sprintf("背包空间不足，还差 %d 个", left))
			return
		}
		before := p.Player.Bag
		p.Player.Bag = candidate
		p.Player.MarkDirty()
		if err := s.pushInventory(p); err != nil {
			p.Player.Bag = before
			reply(false, "背包快照生成失败，物品未发放")
			return
		}
		reply(true, fmt.Sprintf("已发放 %s（%d）×%d", def.Name, def.ID, cmd.Count))
	case GMTakeItem:
		def, ok := s.itemDef(cmd.Item)
		if !ok {
			reply(false, fmt.Sprintf("物品不存在：%d", cmd.Item))
			return
		}
		if def.PetCarrierSpecies > 0 {
			reply(false, "宠物物品不能按模板删除")
			return
		}
		candidate := p.Player.Bag.Clone()
		if candidate == nil || !candidate.Remove(cmd.Item, cmd.Count) {
			have := int32(0)
			if p.Player.Bag != nil {
				have = p.Player.Bag.CountOf(cmd.Item)
			}
			reply(false, fmt.Sprintf("%s 数量不足：当前 %d，需要扣除 %d", def.Name, have, cmd.Count))
			return
		}
		before := p.Player.Bag
		p.Player.Bag = candidate
		p.Player.MarkDirty()
		if err := s.pushInventory(p); err != nil {
			p.Player.Bag = before
			reply(false, "背包快照生成失败，物品未扣除")
			return
		}
		reply(true, fmt.Sprintf("已扣除 %s（%d）×%d，剩余 %d",
			def.Name, def.ID, cmd.Count, candidate.CountOf(cmd.Item)))
	case GMMaxResources:
		if !p.Alive() {
			s.doRevive(p, false)
		} else {
			p.HP, p.MP = p.MaxHP, p.MaxMP
			s.emitTo(p.ID, s.attributeSnapshot(p))
		}
		reply(true, fmt.Sprintf("生命/法力已补满：%d/%d，%d/%d",
			p.HP, p.MaxHP, p.MP, p.MaxMP))
	case GMTeleport:
		if !s.onTeleport(Teleport{ID: cmd.ID, To: cmd.To, At: cmd.At}) {
			reply(false, "传送未执行")
			return
		}
		reply(true, fmt.Sprintf("已传送到地图 %d（%.0f, %.0f）",
			cmd.To.MapID, cmd.At.X, cmd.At.Y))
	case GMWhere:
		reply(true, fmt.Sprintf("当前位置：地图 %d（%.0f, %.0f），场景实例 %d",
			p.Pos.MapID, p.Pos.X, p.Pos.Y, s.id.Instance))
	case GMInspect:
		bagCap, bagUsed := 0, 0
		if p.Player.Bag != nil {
			bagCap = p.Player.Bag.Cap()
			bagUsed = bagCap - p.Player.Bag.FreeSlots()
		}
		ch := p.Player.Char
		reply(true, fmt.Sprintf(
			"在线角色 %s：Lv%d，经验 %d，HP %d/%d，MP %d/%d，金币 %d，银币 %d，铜币 %d，名誉 %d，背包 %d/%d，地图 %d（%.0f, %.0f）",
			p.Name, ch.Level, ch.Exp, p.HP, p.MaxHP, p.MP, p.MaxMP,
			ch.Money.Gold, ch.Money.Silver, ch.Money.Copper, ch.Honor,
			bagUsed, bagCap, p.Pos.MapID, p.Pos.X, p.Pos.Y))
	case GMGod:
		s.setGMGod(p, cmd.State, reply)
	case GMOneShot:
		s.setGMOneShot(p, cmd.State, reply)
	default:
		reply(false, "未登记的 GM 动作")
	}
}

func (s *Scene) setGMGod(p *entity.Entity, mode GMStateMode, reply func(bool, string)) {
	if mode == GMStateQuery {
		reply(true, "god 当前状态："+gmStateText(p.Player.GMGod))
		return
	}
	enabled := mode == GMStateEnable
	p.Player.GMGod = enabled
	if enabled {
		if !p.Alive() {
			s.doRevive(p, false)
		} else {
			p.HP = p.MaxHP
			s.emitTo(p.ID, s.attributeSnapshot(p))
		}
		if p.Status != nil {
			s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool {
				return d.Harmful()
			}))
		}
	}
	reply(true, "god 已"+gmStateText(enabled)+"（切图保留，退出登录清除）")
}

func (s *Scene) setGMOneShot(p *entity.Entity, mode GMStateMode, reply func(bool, string)) {
	if mode == GMStateQuery {
		reply(true, "oneshot 当前状态："+gmStateText(p.Player.GMOneShot))
		return
	}
	p.Player.GMOneShot = mode == GMStateEnable
	reply(true, "oneshot 已"+gmStateText(p.Player.GMOneShot)+"（切图保留，退出登录清除）")
}

func gmStateText(enabled bool) string {
	if enabled {
		return "开启"
	}
	return "关闭"
}

func (s *Scene) takeGMInventoryValue(p *entity.Entity, value *int64, amount int64,
	label string, reply func(bool, string)) {
	before := *value
	if before < amount {
		reply(false, fmt.Sprintf("%s不足：当前 %d，需要扣除 %d", label, before, amount))
		return
	}
	*value = before - amount
	p.Player.MarkDirty()
	if err := s.pushInventory(p); err != nil {
		*value = before
		reply(false, "背包快照生成失败，"+label+"未扣除")
		return
	}
	reply(true, fmt.Sprintf("%s已扣除 %d，当前 %d", label, amount, before-amount))
}

func (s *Scene) changeGMInventoryValue(p *entity.Entity, value *int64, amount int64,
	label, verb string, reply func(bool, string)) {
	before := *value
	next, ok := checkedAdd64(before, amount)
	if !ok || next < 0 {
		reply(false, label+"数值溢出")
		return
	}
	*value = next
	p.Player.MarkDirty()
	if err := s.pushInventory(p); err != nil {
		*value = before
		reply(false, "背包快照生成失败，"+label+"未修改")
		return
	}
	reply(true, fmt.Sprintf("%s已%s %d，当前 %d", label, verb, amount, next))
}
