package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type durabilityWearKind uint8

const (
	durabilityWearAttack durabilityWearKind = iota
	durabilityWearHit
	durabilityWearDeath
)

// consumeAttackDurability 在一次已经结算的玩家攻击/伤害技能之后磨损当前武器。
// miss 也算完成一次攻击动作；本次伤害仍使用出手前属性，武器若在本次归零，
// 从下一次行动起才失效。
func (s *Scene) consumeAttackDurability(p *entity.Entity) {
	s.consumeWornDurability(p, durabilityWearAttack)
}

// consumeHitDurability 只处理实际承受的攻击伤害。miss、治疗和 0 伤害都不磨损。
func (s *Scene) consumeHitDurability(p *entity.Entity, dealt event.DamageDealt) {
	if p == nil || p.Kind != domain.KindPlayer || dealt.Amount <= 0 || dealt.Flag.Has(event.DamageMiss) {
		return
	}
	s.consumeWornDurability(p, durabilityWearHit)
}

func (s *Scene) consumeDeathDurability(p *entity.Entity) {
	s.consumeWornDurability(p, durabilityWearDeath)
}

// consumeSkillWeaponDurability 扣除像裂空之枪这样的额外施法耐久。它不与
// ov_arm.attack_consume 的每次攻击磨损混用，但武器因本次代价损坏时
// 仍必须立即刷新属性和外观。
func (s *Scene) consumeSkillWeaponDurability(p *entity.Entity, cost skillWeaponDurabilityCost, skill domain.SkillID) {
	if p == nil || p.Player == nil || p.Player.Worn == nil || cost.amount <= 0 {
		return
	}
	st := p.Player.Worn.At(cost.slot)
	def, ok := s.itemDef(st.Item)
	if !ok || st.Empty() || def.Equip == nil {
		return
	}
	st.Durability -= cost.amount
	broken := st.Durability <= 0
	if broken {
		st.Durability, st.DurabilityWearRaw = 0, 0
	}
	if broken && def.Equip.DisappearIfZero {
		p.Player.Worn.Set(cost.slot, domain.Stack{})
	} else {
		p.Player.Worn.Set(cost.slot, st)
	}
	p.Player.MarkDirty()
	if broken {
		s.refreshStats(p)
		s.refreshAppearance(p)
	}
	if err := s.pushInventory(p); err != nil {
		s.log.Error("技能扣除武器耐久后推送背包失败", "char", p.Name,
			"skill", skill, "err", err)
	}
}

func (s *Scene) consumeWornDurability(p *entity.Entity, kind durabilityWearKind) {
	if p == nil || p.Kind != domain.KindPlayer || p.Player == nil || p.Player.Worn == nil {
		return
	}
	oldTreasureSkill := s.equippedTreasureSkill(p)
	changedAny, displayChanged, equipmentBroke := false, false, false
	p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		def, ok := s.itemDef(st.Item)
		if !ok || def.Equip == nil {
			return
		}
		var raw int32
		switch kind {
		case durabilityWearAttack:
			if slot != domain.SlotWeapon && slot != domain.SlotTwoHand {
				return
			}
			raw = def.Equip.AttackDurabilityCostRaw
			if reduction := s.attackDurabilityReductionPct(p, def.Equip.Type); reduction > 0 {
				raw = int32((int64(raw)*int64(100-reduction) + 99) / 100)
			}
		case durabilityWearHit:
			raw = def.Equip.BeHitDurabilityCostRaw
		case durabilityWearDeath:
			raw = def.Equip.DeathDurabilityCostRaw
		}
		changed, visible, broken := domain.ConsumeDurabilityRaw(&st, def, raw)
		if !changed {
			return
		}
		changedAny = true
		displayChanged = displayChanged || visible
		equipmentBroke = equipmentBroke || broken
		if broken && def.Equip.DisappearIfZero {
			p.Player.Worn.Set(slot, domain.Stack{})
		} else {
			p.Player.Worn.Set(slot, st)
		}
	})
	if !changedAny {
		return
	}
	p.Player.MarkDirty()
	if equipmentBroke {
		s.refreshStats(p)
		s.refreshAppearance(p)
		if newSkill := s.equippedTreasureSkill(p); equipmentSkillID(oldTreasureSkill) != equipmentSkillID(newSkill) {
			s.emitTo(p.ID, s.skillSnapshot(p))
		}
	}
	if displayChanged {
		if err := s.pushInventory(p); err != nil {
			s.log.Error("耐久变化后推送背包失败", "char", p.Name, "kind", kind, "err", err)
		}
	}
}

// attackDurabilityReductionPct 返回当前武器子类型的普通攻击耐久减免。
// 被动的 RequiredWeaponType 直接来自 ov_skilldesc.arm_type，因而暗器回收
// 不会误伤双刃、剑或法杖。
func (s *Scene) attackDurabilityReductionPct(p *entity.Entity, equipType int32) int32 {
	if s == nil || s.skills == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return 0
	}
	ch := p.Player.Char
	var reduction int32
	for skill, level := range ch.Skills {
		passive, ok := s.skills.Get(skill, level)
		if !ok || passive.Kind != domain.SkillPassive || !ch.AllowsSkill(passive.ID, passive.Prof) ||
			passive.RequiredWeaponType != 0 && passive.RequiredWeaponType != equipType {
			continue
		}
		for _, modifier := range passive.PassiveModifiers {
			if modifier.Kind == domain.PassiveAttackDurabilityReductionPct {
				reduction += modifier.Value
			}
		}
	}
	if reduction > 100 {
		return 100
	}
	return reduction
}
