package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onApplyAvatar(cmd ApplyAvatar) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.Worn == nil || !p.Alive() {
		return
	}
	reject := func(text string) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	}
	if cmd.Kind > 2 || cmd.TargetKind > 1 || (cmd.Kind != 2 && cmd.TargetKind != 1) || cmd.BagSlot < 0 || cmd.TargetSlot < 0 {
		reject("该外观操作暂不支持")
		return
	}
	toolStack := p.Player.Bag.At(int(cmd.BagSlot))
	toolDef, ok := s.itemDef(toolStack.Item)
	if toolStack.Empty() || toolStack.Locked || !ok ||
		!toolDef.InventoryTabKnown || toolDef.InventoryTab != cmd.BagTab {
		reject("背包中的融合道具无效")
		return
	}
	targetSlot := domain.EquipSlot(cmd.TargetSlot)
	target := p.Player.Worn.At(targetSlot)
	targetBagSlot := -1
	if cmd.TargetKind == 0 {
		var valid bool
		targetBagSlot, valid = domain.ClientBagSlot(cmd.TargetTab, int(cmd.TargetSlot))
		if !valid {
			reject("目标背包格无效")
			return
		}
		target = p.Player.Bag.At(targetBagSlot)
	}
	targetDef, ok := s.itemDef(target.Item)
	if cmd.TargetKind == 0 && ok && targetDef.Equip != nil {
		targetSlot = domain.EquipSlot(targetDef.Equip.Slot)
	}
	if target.Empty() || target.Locked || !ok || targetDef.Equip == nil ||
		domain.EquipSlot(targetDef.Equip.Slot) != targetSlot ||
		(cmd.TargetKind == 0 && (!targetDef.InventoryTabKnown || targetDef.InventoryTab != cmd.TargetTab)) {
		reject("请先穿上可换形的目标装备")
		return
	}
	setTarget := func(st domain.Stack) {
		if cmd.TargetKind == 0 {
			p.Player.Bag.Set(targetBagSlot, st)
		} else {
			p.Player.Worn.Set(targetSlot, st)
		}
	}
	originalTarget := target
	var text string
	var model uint16
	if cmd.Kind == 0 {
		if toolDef.AvatarFusion == nil {
			reject("背包中的换形道具无效")
			return
		}
		appearance := toolDef.AvatarFusion.Appearance
		if !appearance.Known || !domain.AppearancePartAcceptsSlot(appearance.Part, targetSlot) {
			reject("换形道具与目标装备部位不匹配")
			return
		}
		if target.FusedAppearance == toolDef.ID {
			reject("目标装备已经使用该外观")
			return
		}
		target.FusedAppearance = toolDef.ID
		model = appearance.Model
		text = "装备换形成功：" + toolDef.Name
	} else if cmd.Kind == 1 {
		if toolDef.EquipmentSoul == nil || !toolDef.EquipmentSoul.AcceptsSlot(targetSlot) {
			reject("装备灵与目标装备部位不匹配")
			return
		}
		if target.FusedSoul == toolDef.ID {
			reject("目标装备已经镶入该装备灵")
			return
		}
		target.FusedSoul = toolDef.ID
		text = "装备灵镶嵌成功：" + toolDef.Name
	} else {
		if toolDef.DragonFusion == nil || !toolDef.DragonFusion.AcceptsSlot(targetSlot) {
			reject("龙系列道具与目标装备部位不匹配")
			return
		}
		if target.FusedDragon == toolDef.ID {
			reject("目标装备已经融合该龙系列道具")
			return
		}
		target.FusedDragon = toolDef.ID
		text = "龙系列融合成功：" + toolDef.Name
	}
	bag := p.Player.Bag.Clone()
	if bag == nil || !bag.RemoveAt(int(cmd.BagSlot), 1) {
		reject("换形道具扣除失败")
		return
	}
	oldBag := p.Player.Bag
	p.Player.Bag = bag
	setTarget(target)
	var wardrobe event.WardrobeSnapshot
	if cmd.Kind == 0 {
		var err error
		wardrobe, err = s.wardrobeSnapshot(p)
		if err != nil {
			p.Player.Bag = oldBag
			setTarget(originalTarget)
			reject("换形结果同步失败")
			return
		}
	}
	p.Player.MarkDirty()
	s.refreshStats(p)
	s.refreshAppearance(p)
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag = oldBag
		setTarget(originalTarget)
		s.refreshStats(p)
		s.refreshAppearance(p)
		reject("换形结果同步失败")
		return
	}
	if cmd.Kind == 0 {
		s.emitTo(p.ID, wardrobe)
	}
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	reject(text)
	s.log.Info("装备融合", "char", p.Name, "kind", cmd.Kind, "tool", toolDef.ID,
		"target", target.Item, "slot", targetSlot, "model", model)
}
