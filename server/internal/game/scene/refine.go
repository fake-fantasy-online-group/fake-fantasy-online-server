package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) refineTarget(p *entity.Entity, cmd RefineItem) (domain.Stack, domain.ItemDef, bool) {
	var st domain.Stack
	if cmd.Mode == 0 {
		if p.Player.Bag == nil || cmd.BagSlot < 0 || int(cmd.BagSlot) >= p.Player.Bag.Cap() {
			return st, domain.ItemDef{}, false
		}
		st = p.Player.Bag.At(int(cmd.BagSlot))
	} else if cmd.Mode == 1 {
		if p.Player.Worn == nil {
			return st, domain.ItemDef{}, false
		}
		st = p.Player.Worn.At(domain.EquipSlot(cmd.EquipSlot))
	} else {
		return st, domain.ItemDef{}, false
	}
	def, ok := s.itemDef(st.Item)
	if st.Empty() || st.Locked || !ok || def.Equip == nil || st.RefineLevel >= def.Equip.RefineLimit ||
		(cmd.Mode == 0 && (!def.InventoryTabKnown || def.InventoryTab != cmd.Tab)) {
		return st, def, false
	}
	return st, def, true
}

func (s *Scene) refineInfo(p *entity.Entity, cmd RefineItem) event.RefineInfo {
	info := event.RefineInfo{Who: p.ID, Mode: cmd.Mode, EquipSlot: cmd.EquipSlot,
		Tab: cmd.Tab, BagSlot: cmd.BagSlot}
	st, def, ok := s.refineTarget(p, cmd)
	if !ok {
		return info
	}
	info.Item, info.CurrentLevel = st.Item, st.RefineLevel
	recipe, ok := s.refines.Get(domain.RefineClass(def), def.Equip.Tier, st.RefineLevel)
	if !ok {
		return info
	}
	info.Cost, info.SuccessRate = int32(recipe.Cost), recipe.SuccessRate
	info.SafeLevel = st.RefineLevel
	if recipe.FailureLevel != 255 {
		info.SafeLevel = recipe.FailureLevel
	}
	protectEnabled := cmd.Protect && recipe.ProtectItem != 0
	info.FailDestroys = recipe.FailureDestroys
	value, learned := p.Player.Char.Skills[recipe.RequiredSkill]
	info.CanDo = recipe.RequiredSkill == 0 || (learned && value >= recipe.ProficiencyMin)
	// 缺失/未支持的目标等级不能收费后只增加“+N”标签。
	_, nextKnown := def.Equip.RefineEffectAt(st.RefineLevel + 1)
	info.CanDo = info.CanDo && nextKnown
	if !protectEnabled && !recipe.FailureDestroys && recipe.FailureLevel != 255 {
		_, failureKnown := def.Equip.RefineEffectAt(recipe.FailureLevel)
		info.CanDo = info.CanDo && failureKnown
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	info.CanDo = info.CanDo && err == nil && money >= recipe.Cost
	for _, material := range recipe.Materials {
		mat, _ := s.itemDef(material.Item)
		have := p.Player.Bag.UsableCountOf(material.Item)
		info.Materials = append(info.Materials, event.CraftMaterialView{
			Item: material.Item, Need: material.Qty, Have: have, Name: mat.Name,
		})
		info.CanDo = info.CanDo && have >= material.Qty
	}
	if recipe.ProtectItem != 0 {
		mat, known := s.itemDef(recipe.ProtectItem)
		have := p.Player.Bag.UsableCountOf(recipe.ProtectItem)
		if known {
			info.Materials = append(info.Materials, event.CraftMaterialView{Item: recipe.ProtectItem, Need: 1, Have: have, Name: mat.Name})
		}
		if protectEnabled {
			info.CanDo = info.CanDo && known && have >= 1
		}
	}
	return info
}

func (s *Scene) onRefineItem(cmd RefineItem) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil || s.refines == nil {
		return
	}
	info := s.refineInfo(p, cmd)
	if !cmd.Execute {
		s.emitTo(p.ID, info)
		return
	}
	if !p.Player.RefineReadyAt(s.tick) {
		return
	}
	st, def, ok := s.refineTarget(p, cmd)
	if !ok || !info.CanDo {
		if ok {
			if _, known := def.Equip.RefineEffectAt(st.RefineLevel + 1); !known {
				s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "该等级的精炼属性尚未配置或支持，未扣除材料和费用。"})
			}
		}
		s.emitTo(p.ID, info)
		return
	}
	recipe, ok := s.refines.Get(domain.RefineClass(def), def.Equip.Tier, st.RefineLevel)
	if !ok {
		return
	}
	nextBag := p.Player.Bag.Clone()
	if nextBag == nil {
		return
	}
	for _, material := range recipe.Materials {
		if !nextBag.Remove(material.Item, material.Qty) {
			s.emitTo(p.ID, info)
			return
		}
	}
	protectEnabled := cmd.Protect && recipe.ProtectItem != 0
	if protectEnabled {
		if !nextBag.Remove(recipe.ProtectItem, 1) {
			s.emitTo(p.ID, info)
			return
		}
	}
	money, _ := inventoryMoney(p.Player.Char.Money)
	p.Player.Char.Money = moneyFromInventory(money - recipe.Cost)
	p.Player.StartRefineCooldown(s.tick, domain.Ticks(int(recipe.ConfirmCooldownMS)))
	succeeded := s.rng.Intn(100) < int(recipe.SuccessRate)
	destroyed := !succeeded && !protectEnabled && recipe.FailureDestroys
	if succeeded {
		st.RefineLevel++
	} else if !protectEnabled && recipe.FailureLevel != 255 {
		st.RefineLevel = recipe.FailureLevel
	}
	if destroyed {
		st = domain.Stack{}
	}
	p.Player.Bag = nextBag
	if cmd.Mode == 0 {
		p.Player.Bag.Set(int(cmd.BagSlot), st)
	} else {
		p.Player.Worn.Set(domain.EquipSlot(cmd.EquipSlot), st)
	}
	if recipe.RequiredSkill != 0 && recipe.ProficiencyGain > 0 {
		p.Player.Char.Skills[recipe.RequiredSkill] += recipe.ProficiencyGain
	}
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	if cmd.Mode == 1 {
		s.refreshStats(p)
		if destroyed {
			s.refreshAppearance(p)
		}
	}
	text := "精炼失败。"
	if succeeded {
		text = "精炼成功。"
	} else if destroyed {
		text = "精炼失败，装备损毁。"
	} else if protectEnabled {
		text = "精炼失败，保护道具避免了降级。"
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	s.emitTo(p.ID, s.refineInfo(p, cmd))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}
