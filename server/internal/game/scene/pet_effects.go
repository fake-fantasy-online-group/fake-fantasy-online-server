package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) eachPetEffect(owner *entity.Entity, fn func(*domain.PetInstance, domain.PetSkillEffect, int32)) {
	if owner == nil || owner.Player == nil {
		return
	}
	p := s.entities[owner.Player.Pet]
	if p == nil || p.Pet == nil || p.Pet.Inst == nil || !p.Alive() {
		return
	}
	inst := p.Pet.Inst
	combatLevelBonus := s.petCombatSkillLevelBonus(inst)
	for _, learned := range inst.Skills {
		d, ok := s.petSkills[learned.ID]
		if !ok || d.Effect == nil || learned.Level <= 0 {
			continue
		}
		if d.Effect.RequireSelected && inst.ActiveSkill != learned.ID {
			continue
		}
		level := learned.Level
		if d.Fight {
			level += combatLevelBonus
		}
		if d.MaxLevel > 0 && level > d.MaxLevel {
			level = d.MaxLevel
		}
		fn(inst, *d.Effect, level)
	}
}

// petCombatSkillLevelBonus 实现“愚笨”的战斗技能等级 +1。加成只改变效果
// 结算等级，不改宠物技能的持久化等级，也不能把技能推过自身等级上限。
func (s *Scene) petCombatSkillLevelBonus(inst *domain.PetInstance) int32 {
	if inst == nil {
		return 0
	}
	for _, learned := range inst.Skills {
		d, ok := s.petSkills[learned.ID]
		if ok && d.Effect != nil && d.Effect.Kind == "stupid" {
			return d.Effect.SourceAttr
		}
	}
	return 0
}

func (s *Scene) petEffect(inst *domain.PetInstance, kind string) (domain.PetSkillEffect, int32, bool) {
	if inst == nil {
		return domain.PetSkillEffect{}, 0, false
	}
	combatLevelBonus := s.petCombatSkillLevelBonus(inst)
	for _, learned := range inst.Skills {
		d, ok := s.petSkills[learned.ID]
		if !ok || d.Effect == nil || d.Effect.Kind != kind || learned.Level <= 0 ||
			d.Effect.RequireSelected && inst.ActiveSkill != learned.ID {
			continue
		}
		level := learned.Level
		if d.Fight {
			level += combatLevelBonus
		}
		if d.MaxLevel > 0 && level > d.MaxLevel {
			level = d.MaxLevel
		}
		return *d.Effect, level, true
	}
	return domain.PetSkillEffect{}, 0, false
}

func (s *Scene) petStatAffixes(owner *entity.Entity) []domain.Affix {
	var out []domain.Affix
	s.eachPetEffect(owner, func(inst *domain.PetInstance, e domain.PetSkillEffect, level int32) {
		attr := int32(0)
		switch e.Kind {
		case "physical_attack_pct":
			attr = domain.AttrAtk
		case "magic_attack_pct":
			attr = domain.AttrMAtk
		case "max_weight_from_vit":
			attr = domain.AttrMaxWeight
		}
		if attr != 0 {
			value, mode := e.Amount(level), int32(domain.ModePercent)
			if e.Kind == "max_weight_from_vit" {
				value = (inst.Base.VIT / e.SourceStep) * e.Amount(level)
				mode = domain.ModeAbsolute
			}
			out = append(out, domain.Affix{Attr: attr, Value: value, Mode: mode})
		}
	})
	return out
}

func (s *Scene) petProcAmount(owner *entity.Entity, kind string) int32 {
	var amount int32
	s.eachPetEffect(owner, func(inst *domain.PetInstance, e domain.PetSkillEffect, level int32) {
		if e.Kind != kind || int32(s.rng.Intn(10000)) >= e.Chance(level) {
			return
		}
		stat := int32(0)
		switch e.SourceAttr {
		case domain.AttrSTR:
			stat = inst.Base.STR
		case domain.AttrINT:
			stat = inst.Base.INT
		case domain.AttrAGI:
			stat = inst.Base.AGI
		case domain.AttrVIT:
			stat = inst.Base.VIT
		case domain.AttrSPI:
			stat = inst.Base.SPI
		}
		amount += (stat / e.SourceStep) * e.Amount(level)
	})
	return amount
}

func (s *Scene) petFlatReduction(owner *entity.Entity, magic bool) int32 {
	kind := "physical_reduction_flat"
	if magic {
		kind = "magic_reduction_flat"
	}
	var total int32
	s.eachPetEffect(owner, func(_ *domain.PetInstance, e domain.PetSkillEffect, level int32) {
		if e.Kind == kind {
			total += e.Amount(level)
		}
	})
	return total
}

// petGuardDamage 返回由护主替玩家承担后的玩家伤害。技能先承担来袭伤害的
// 30%，宠物再按 300%-(等级-1)*7% 扣血；宠物最低保留 1 HP。
func (s *Scene) petGuardDamage(owner *entity.Entity, source domain.EntityID, incoming int32) int32 {
	if owner == nil || owner.Player == nil || incoming <= 0 {
		return incoming
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || pet.HP <= 1 {
		return incoming
	}
	effect, level, ok := s.petEffect(pet.Pet.Inst, "guard_damage")
	if !ok {
		return incoming
	}
	redirect := int32(int64(incoming) * int64(effect.Chance(level)) / 10000)
	if redirect <= 0 {
		return incoming
	}
	lossPct := effect.Value - (level-1)*effect.ValuePerLevel
	if lossPct < 1 {
		lossPct = 1
	}
	available := pet.HP - 1
	maxRedirect := int32(int64(available) * 100 / int64(lossPct))
	if redirect > maxRedirect {
		redirect = maxRedirect
	}
	if redirect <= 0 {
		return incoming
	}
	loss := int32((int64(redirect)*int64(lossPct) + 99) / 100)
	if loss > available {
		loss = available
	}
	pet.HP -= loss
	pet.Pet.Inst.HP = pet.HP
	owner.Player.MarkDirty()
	s.emit(event.DamageDealt{Src: source, Dst: pet.ID, Amount: loss, DstHP: pet.HP})
	s.emitTo(owner.ID, s.petSnapshot(owner))
	return incoming - redirect
}

func (s *Scene) petLearningChance(inst *domain.PetInstance, base int32) int32 {
	chance := base
	if effect, level, ok := s.petEffect(inst, "stupid"); ok {
		chance -= effect.Amount(level)
	}
	if effect, level, ok := s.petEffect(inst, "hungry_scholar"); ok {
		chance += effect.Amount(level)
	}
	if chance < 0 {
		return 0
	}
	if chance > 10000 {
		return 10000
	}
	return chance
}

func (s *Scene) petHungerInterval(inst *domain.PetInstance) int32 {
	seconds := s.petRule.HungerIntervalSec
	if effect, _, ok := s.petEffect(inst, "hungry_scholar"); ok && effect.SourceAttr > 1 {
		seconds /= effect.SourceAttr
	}
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

func (s *Scene) petTrustInterval(inst *domain.PetInstance, delta, seconds int32) int32 {
	if seconds <= 0 {
		return seconds
	}
	effect, level, ok := s.petEffect(inst, "loyalty")
	if !ok || effect.Amount(level) <= 1 {
		return seconds
	}
	multiplier := effect.Amount(level)
	if delta > 0 {
		seconds /= multiplier
	} else if delta < 0 {
		seconds *= multiplier
	}
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

func (s *Scene) petWorkEfficiencyPct(owner *entity.Entity, skill domain.SkillID) int32 {
	if owner == nil || owner.Player == nil || skill == 0 {
		return 0
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return 0
	}
	for _, learned := range pet.Pet.Inst.Skills {
		if learned.ID != skill {
			continue
		}
		d, ok := s.petSkills[learned.ID]
		if ok && d.Effect != nil && d.Effect.Kind == "work_efficiency" {
			return d.Effect.Amount(learned.Level)
		}
	}
	return 0
}

// petSkillCleanses supports both legacy single-target and merged cleanse rules.
func petSkillCleanses(e *domain.PetSkillEffect, status domain.StatusID) bool {
	if len(e.CleanseStatusIDs) > 0 {
		for _, id := range e.CleanseStatusIDs {
			if domain.StatusID(id) == status {
				return true
			}
		}
		return false
	}
	return domain.StatusID(e.SourceAttr) == status
}

// tryPetCleanse 每次有对应负面状态真正落到主人身上时只判定一次。客户端
// sp_chg_start 的宠物技能两行分别是 1 级消耗与每级增量，判定无论成功与否
// 都消耗本次施法所需精气。
func (s *Scene) tryPetCleanse(owner *entity.Entity, status domain.StatusID) {
	if owner == nil || owner.Player == nil || owner.Status == nil {
		return
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Alive() || pet.MP <= 0 {
		return
	}
	inst := pet.Pet.Inst
	for _, learned := range inst.Skills {
		d, ok := s.petSkills[learned.ID]
		if !ok || d.Effect == nil || learned.Level <= 0 || d.Effect.Kind != "cleanse_status" ||
			!petSkillCleanses(d.Effect, status) ||
			d.Effect.RequireSelected && inst.ActiveSkill != learned.ID {
			continue
		}
		level := learned.Level + s.petCombatSkillLevelBonus(inst)
		if d.MaxLevel > 0 && level > d.MaxLevel {
			level = d.MaxLevel
		}
		cost := d.Effect.Amount(level)
		if cost <= 0 || pet.MP < cost {
			return
		}
		pet.MP -= cost
		inst.MP = pet.MP
		owner.Player.MarkDirty()
		if int32(s.rng.Intn(10000)) < d.Effect.Chance(level) {
			s.statusesRemoved(owner, owner.Status.RemoveWhere(func(existing domain.StatusDef) bool {
				return existing.ID == status
			}))
		}
		s.emitTo(owner.ID, s.petSnapshot(owner))
		return
	}
}

func (s *Scene) stepPetActiveSkill(pet, owner *entity.Entity) {
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || owner == nil || owner.Player == nil {
		return
	}
	effect, level, ok := s.petEffect(pet.Pet.Inst, "owner_heal")
	if !ok || owner.HP >= owner.MaxHP || pet.MP <= 0 {
		pet.Pet.NextPetSkillAt = 0
		return
	}
	if pet.Pet.NextPetSkillAt != 0 && s.tick < pet.Pet.NextPetSkillAt {
		return
	}
	amount := effect.Amount(level)
	if amount > pet.MP {
		amount = pet.MP
	}
	if missing := owner.MaxHP - owner.HP; amount > missing {
		amount = missing
	}
	if amount <= 0 {
		pet.Pet.NextPetSkillAt = 0
		return
	}
	pet.MP -= amount
	pet.Pet.Inst.MP = pet.MP
	owner.Player.MarkDirty()
	s.emit(combat.Restore(pet, owner, amount, int32(pet.Pet.Inst.ActiveSkill)))
	s.emitAttributesIfPlayer(owner)
	s.emitTo(owner.ID, s.petSnapshot(owner))
	pet.Pet.NextPetSkillAt = s.tick + domain.Ticks(int(effect.SourceStep))
}

func (s *Scene) petExperienceBonus(owner *entity.Entity, experience int64) int64 {
	var bonus int64
	s.eachPetEffect(owner, func(_ *domain.PetInstance, e domain.PetSkillEffect, level int32) {
		if e.Kind == "experience_bonus" && int32(s.rng.Intn(10000)) < e.Chance(level) {
			bonus += experience * int64(e.Amount(level)) / 100
		}
	})
	return bonus
}

// petSkillSpace 按技能的战斗/生活分类独立计算容量，主动/被动不参与分类。
func (s *Scene) petSkillSpace(inst *domain.PetInstance, id domain.SkillID) bool {
	candidate, ok := s.petSkills[id]
	if inst == nil || !ok {
		return false
	}
	limit := s.petRule.MaxLifeSkills
	if candidate.Fight {
		limit = s.petRule.MaxFightSkills
	}
	if limit <= 0 {
		return true // 无数据库的隔离场景可不配置容量；正式启动必须加载有效规则。
	}
	var count int32
	for _, learned := range inst.Skills {
		if known, exists := s.petSkills[learned.ID]; exists && known.Fight == candidate.Fight {
			count++
		}
	}
	return count < limit
}
func (s *Scene) learnPetSkill(inst *domain.PetInstance, id domain.SkillID) bool {
	return s.petSkillSpace(inst, id) && inst.Learn(id)
}
func (s *Scene) canLearnPetSkill(inst *domain.PetInstance, def domain.PetDef, id domain.SkillID) bool {
	return s.petSkillSpace(inst, id) && inst.CanLearn(def, id)
}

// 自然领悟至多获得一个主动战斗技能；药丸学习不受此限制。
// 已学技能即使未启用，也占用自然领悟的主动战斗技能名额。
func (s *Scene) canNaturallyLearnPetSkill(inst *domain.PetInstance, def domain.PetDef, id domain.SkillID) bool {
	if !s.canLearnPetSkill(inst, def, id) {
		return false
	}
	candidate := s.petSkills[id]
	if !candidate.Fight || !candidate.Active {
		return true
	}
	for _, learned := range inst.Skills {
		known := s.petSkills[learned.ID]
		if known.Fight && known.Active {
			return false
		}
	}
	return true
}

func (s *Scene) usePetEgg(p *entity.Entity, slot int, def domain.ItemDef, reject func(event.RejectReason)) {
	pet, ok := s.petDefs[def.GrantPetID]
	if !ok {
		reject(event.RejectInvalid)
		return
	}
	if !s.petItemRoom(p, 1) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "宠物栏已满。"})
		return
	}
	if !p.Player.Bag.RemoveAt(slot, 1) {
		reject(event.RejectNoItem)
		return
	}
	inst := domain.NewPetInstance(pet)
	inst.ID = s.nextPetInstID(p.Player.Char)
	inst.Slot = s.nextPetSlot(p.Player.Char)
	s.appendPet(p, inst)
	p.Player.MarkDirty()
	s.pushInventory(p)
	s.pushPetSnapshot(p)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: fmt.Sprintf("获得%s，请到宠物栏双击孵化。", pet.Name)})
}
