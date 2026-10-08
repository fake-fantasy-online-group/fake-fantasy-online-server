package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 穿戴与脱下。
//
// **属性只在这里重算, 不在别处。** 换一件装备要影响的东西太多了
// (血上限、攻击区间、命中、暴击率、攻速、各种抗性), 散落在各处的话
// 迟早有一处漏掉, 而那种 bug 表现为"某个属性偶尔不对", 极难查。

// onEquip 穿上背包里第 slot 格的装备。
func (s *Scene) onEquip(cmd Equip) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	if p.Player.Stall != nil {
		s.stallNotice(p, "摆摊中不能更换装备，请先结束摊位")
		return
	}
	oldTreasureSkill := s.equippedTreasureSkill(p)
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "Equip", Reason: r})
	}
	st := p.Player.Bag.At(cmd.BagSlot)
	if st.Empty() {
		reject(event.RejectNoTarget)
		return
	}
	if st.Locked {
		reject(event.RejectInvalid)
		return
	}
	def, ok := s.itemDef(st.Item)
	if !ok || def.Equip == nil {
		reject(event.RejectNotEquippable)
		return
	}
	if def.Equip.Durable > 0 && !def.Equip.NoLimitDurability &&
		st.MaxDurability > 0 && st.Durability <= 0 {
		// 耐久归零但不消失的装备会留在背包；必须先修好才能重新穿戴。
		reject(event.RejectInvalid)
		return
	}
	ch := p.Player.Char
	slot := domain.EquipSlot(def.Equip.Slot)

	// 门槛按当前有效六维判定，但先移除目标槽位和冲突槽位的旧装备。
	// 这样换装时旧装备不会错误地支撑新装备，身上的其它装备加成则可以参与判定。
	if why := def.Equip.Need.Meet(ch.Level, s.equipmentRequirementBaseFromWorn(p, s.functionalWorn(p), slot), ch.Appear.Gender); why != domain.RejectNone {
		reject(equipReject(why))
		return
	}
	if !def.Equip.Need.AllowsCharacter(ch) {
		reject(event.RejectWrongProf)
		return
	}

	// 先把冲突槽位和目标槽位上的旧装备退回背包。
	// 退不下就整件事作废 —— 半途而废会让装备卡在"既不在身上也不在包里"。
	off := append(domain.ConflictSlots(slot), slot)
	if !s.canUnequipAll(p, off, cmd.BagSlot) {
		reject(event.RejectBagFull)
		return
	}
	if !p.Player.Bag.RemoveAt(cmd.BagSlot, 1) {
		reject(event.RejectNoTarget)
		return
	}
	for _, sl := range off {
		s.unequipTo(p, sl)
	}
	// 客户端在发 0x1007 前已对未绑定普通装备显示“装备后将绑定”。
	// 原服成功快照的装备描述也随即出现“（绑定）”；绑定必须改在
	// 	这个装备实例上，才能在脱下、存档和重登后继续保留。
	// NetClient.IsMiningTool 确认的两种采矿工具不走绑定确认，保留原值。
	if !domain.IsMiningTool(st.Item) {
		st.Bound = true
	}
	p.Player.Worn.Set(slot, st)

	s.refreshStats(p)
	s.refreshAppearance(p)
	p.Player.MarkDirty()
	s.pushInventory(p)
	s.refreshChangeSetTooltips(p)
	if newTreasureSkill := s.equippedTreasureSkill(p); equipmentSkillID(oldTreasureSkill) != equipmentSkillID(newTreasureSkill) {
		s.emitTo(p.ID, s.skillSnapshot(p))
	}
	s.requestNewbieTip(p.ID, 4)
	s.log.Debug("穿戴", "char", ch.Name, "装备", def.Name, "槽位", slot)
}

// equipmentRequirementBase 返回穿上目标槽位装备前可用于门槛判定的有效六维。
// 目标槽位及其冲突槽位先清空，避免被替换的装备给自己提供门槛属性。
func (s *Scene) equipmentRequirementBase(p *entity.Entity, slot domain.EquipSlot) domain.Base {
	if p == nil || p.Player == nil {
		return domain.Base{}
	}
	return s.equipmentRequirementBaseFromWorn(p, p.Player.Worn, slot)
}

func (s *Scene) equipmentRequirementBaseFromWorn(p *entity.Entity, worn *domain.EquipSet, slot domain.EquipSlot) domain.Base {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return domain.Base{}
	}
	worn = worn.Clone()
	for _, conflict := range append(domain.ConflictSlots(slot), slot) {
		worn.Set(conflict, domain.Stack{})
	}
	// 被动技能的装备条件也必须使用试算后的穿戴，不能借用旧武器激活的被动。
	preview := *p
	player := *p.Player
	player.Worn = worn
	preview.Player = &player
	ch := p.Player.Char
	base, _ := domain.Compute(domain.StatSource{
		Base: ch.EffectiveBase(), Level: ch.Level, Innate: characterInnateStats(ch),
		Worn: worn, Defs: s.itemDef, Passive: s.passiveStatAffixes(&preview), Status: p.Status,
	})
	return base
}

// functionalWorn resolves equipment requirements as an acyclic dependency graph.
// Only equipment that is independently usable from the character's own attributes
// is activated first; later layers may depend on already activated equipment. A
// mutual dependency therefore activates neither item instead of making both appear
// valid merely because they happen to be persisted in the worn slots.
func (s *Scene) functionalWorn(p *entity.Entity) *domain.EquipSet {
	if p == nil || p.Player == nil || p.Player.Worn == nil {
		return domain.NewEquipSet()
	}
	source := p.Player.Worn
	active := domain.NewEquipSet()
	resolved := make(map[domain.EquipSlot]bool)
	for pass := 0; pass <= source.Count(); pass++ {
		changed := false
		source.Each(func(slot domain.EquipSlot, st domain.Stack) {
			if resolved[slot] {
				return
			}
			def, ok := s.itemDef(st.Item)
			if !ok || def.Equip == nil || !domain.EquipmentFunctional(st, def) {
				return
			}
			ch := p.Player.Char
			if def.Equip.Need.Meet(ch.Level, s.equipmentRequirementBaseFromWorn(p, active, slot), ch.Appear.Gender) != domain.RejectNone ||
				!def.Equip.Need.AllowsCharacter(ch) {
				return
			}
			for _, conflict := range domain.ConflictSlots(slot) {
				active.Set(conflict, domain.Stack{})
			}
			active.Set(slot, st)
			resolved[slot] = true
			changed = true
		})
		if !changed {
			break
		}
	}
	return active
}

// onUnequip 脱下某个槽位的装备, 放回背包。
func (s *Scene) onUnequip(cmd Unequip) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	if p.Player.Stall != nil {
		s.stallNotice(p, "摆摊中不能更换装备，请先结束摊位")
		return
	}
	if p.Player.Worn.At(cmd.Slot).Empty() {
		return // 那个槽位本来就是空的, 静默即可
	}
	oldTreasureSkill := s.equippedTreasureSkill(p)
	if !s.canUnequipAll(p, []domain.EquipSlot{cmd.Slot}, -1) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "Unequip", Reason: event.RejectBagFull})
		return
	}
	s.unequipTo(p, cmd.Slot)
	s.refreshStats(p)
	s.refreshAppearance(p)
	p.Player.MarkDirty()
	s.pushInventory(p)
	s.refreshChangeSetTooltips(p)
	if newTreasureSkill := s.equippedTreasureSkill(p); equipmentSkillID(oldTreasureSkill) != equipmentSkillID(newTreasureSkill) {
		s.emitTo(p.ID, s.skillSnapshot(p))
	}
}

func equipmentSkillID(skill *domain.EquipmentSkill) domain.SkillID {
	if skill == nil {
		return 0
	}
	return skill.ID
}

// canUnequipAll 判断这些槽位上的东西能不能都退回背包。
//
// freed 是这次穿戴会腾出来的那一格(装备从背包里拿走了), 要算进可用空间。
func (s *Scene) canUnequipAll(p *entity.Entity, slots []domain.EquipSlot, freed int) bool {
	need := 0
	for _, sl := range slots {
		if !p.Player.Worn.At(sl).Empty() {
			need++
		}
	}
	if need == 0 {
		return true
	}
	first, last := 0, p.Player.Bag.Cap()
	if p.Player.Bag.Cap() >= domain.DefaultBagSlots {
		first, _ = domain.ClientBagSlot(2, 0)
		last = first + domain.BagPageSlots
	}
	free := 0
	for i := first; i < last; i++ {
		if p.Player.Bag.At(i).Empty() {
			free++
		}
	}
	if freed >= first && freed < last && !p.Player.Bag.At(freed).Empty() {
		free++ // 那一格马上就空出来了
	}
	return free >= need
}

// unequipTo 把某个槽位上的东西放回背包。调用方保证放得下。
func (s *Scene) unequipTo(p *entity.Entity, slot domain.EquipSlot) {
	st := p.Player.Worn.At(slot)
	if st.Empty() {
		return
	}
	def, ok := s.itemDef(st.Item)
	if !ok || def.Equip == nil {
		panic("scene: 脱下的装备没有有效模板")
	}
	p.Player.Worn.Set(slot, domain.Stack{})
	// 装备不可堆叠, 一件占一格; 耐久要跟着回去, 不能重置成满的
	if p.Player.Bag.AddStack(def, st) == 0 {
		return
	}
	// 走到这说明调用方没先问 canUnequipAll。宁可崩在测试里也不能默默吞掉一件装备。
	panic("scene: 脱装备时背包没位置 —— 调用方必须先调 canUnequipAll")
}

// refreshStats 重算属性并把变化发给本人。
//
// 血量按**上限的变化量**跟着走: 换上加血的装备当场补上, 换下来就扣掉。
// 不这么做的话, 反复穿脱一件加血装备等于无限回血。
func (s *Scene) refreshStats(p *entity.Entity) {
	ch := p.Player.Char
	wasAlive := p.Alive()
	oldMaxHP, oldMaxMP := p.MaxHP, p.MaxMP

	worn := s.functionalWorn(p)
	preview := *p
	player := *p.Player
	player.Worn = worn
	preview.Player = &player
	_, stats := domain.Compute(domain.StatSource{
		Base: ch.EffectiveBase(), Level: ch.Level, Innate: characterInnateStats(ch),
		Worn: worn, Defs: s.itemDef, Passive: s.passiveStatAffixes(&preview), Status: p.Status,
	})
	p.Stats = stats
	p.MaxHP = stats.MaxHP
	p.MaxMP = stats.MaxMP

	if d := p.MaxHP - oldMaxHP; d != 0 {
		p.HP += d
	}
	if d := p.MaxMP - oldMaxMP; d != 0 {
		p.MP += d
	}
	if p.HP > p.MaxHP {
		p.HP = p.MaxHP
	}
	if p.MP > p.MaxMP {
		p.MP = p.MaxMP
	}
	if !wasAlive {
		// 状态清理也走这条统一属性重算管线。玩家已经死亡时必须保持 0 HP，
		// 不能套用“脱装备不得致死”的存活保护，否则死亡清 Buff 会把人抬活。
		p.HP = 0
	} else if p.HP < 1 && p.MaxHP > 0 {
		// 脱掉加血装备不该把人脱死 —— 那会变成一种自杀手段, 也会绕过死亡惩罚
		p.HP = 1
	}
	if p.MP < 0 {
		p.MP = 0
	}
	s.emitTo(p.ID, s.attributeSnapshot(p))
	s.refreshWarehouseBlocked(p)
}

func (s *Scene) passiveStatAffixes(p *entity.Entity) []domain.Affix {
	if s == nil || p == nil || p.Player == nil ||
		p.Player.Char == nil || p.Player.Worn == nil {
		return nil
	}
	ch := p.Player.Char
	out := s.petStatAffixes(p)
	for skill, level := range ch.Skills {
		def, ok := s.skills.Get(skill, level)
		if !ok || def.Kind != domain.SkillPassive || !ch.AllowsSkill(def.ID, def.Prof) {
			continue
		}
		for _, modifier := range def.PassiveStats {
			if modifier.RequiredEquipType != 0 && !s.hasFunctionalEquipType(p, modifier.RequiredEquipType) {
				continue
			}
			out = append(out, domain.Affix{
				Attr: modifier.Attr, Value: modifier.Value, Mode: modifier.Mode,
			})
		}
	}
	return out
}

func (s *Scene) hasFunctionalEquipType(p *entity.Entity, equipType int32) bool {
	if p == nil || p.Player == nil || p.Player.Worn == nil || equipType <= 0 {
		return false
	}
	found := false
	p.Player.Worn.Each(func(_ domain.EquipSlot, stack domain.Stack) {
		if found || stack.Empty() {
			return
		}
		def, ok := s.itemDef(stack.Item)
		found = ok && def.Equip != nil && def.Equip.Type == equipType &&
			domain.EquipmentFunctional(stack, def)
	})
	return found
}

// refreshAppearance 从权威穿戴重算外观并广播完整快照。
// 调用顺序必须紧跟 refreshStats、早于 pushInventory：原服两次穿戴抓包均为
// 0x8007 → 0x800a → 0x8006，客户端会在背包快照前刷新角色模型。
func (s *Scene) refreshAppearance(p *entity.Entity) {
	ch := p.Player.Char
	next := domain.AppearanceFromEquipmentAndWardrobe(ch.Appear, s.functionalWorn(p), p.Player.Wardrobe,
		s.itemDef, s.wardrobes, ch.Appear.Gender)
	ch.Appear = next
	p.Look.Appearance = next
	s.emit(event.AppearanceChanged{Who: p.ID, Appearance: next})
	_ = s.pushWardrobe(p)
}

// itemDef 查物品模板。没配物品表时返回 false。
func (s *Scene) itemDef(id domain.ItemID) (domain.ItemDef, bool) {
	if s.items == nil {
		return domain.ItemDef{}, false
	}
	d, ok := s.items[id]
	return d, ok
}

// equipReject 把 domain 的拒绝原因翻成对外事件的原因。
// domain 不认识 event 包, 这一层翻译是分层的必然代价, 也是它的价值。
func equipReject(r domain.RejectReason) event.RejectReason {
	switch r {
	case domain.RejectLevelTooLow:
		return event.RejectLevelTooLow
	case domain.RejectStatTooLow:
		return event.RejectStatTooLow
	case domain.RejectWrongSex:
		return event.RejectWrongSex
	case domain.RejectWrongSlot:
		return event.RejectNotEquippable
	}
	return event.RejectUnknown
}
