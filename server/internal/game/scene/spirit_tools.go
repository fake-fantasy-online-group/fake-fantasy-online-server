package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
)

func (s *Scene) applySpiritTool(p *entity.Entity, req domain.EndgameRequest) {
	reject := func(code, text string) { s.endgameError(p, req, code, text) }
	uid, err := strconv.ParseInt(req.TargetUID, 10, 64)
	if err != nil || uid <= 0 || strconv.FormatInt(uid, 10) != req.TargetUID {
		reject("target", "请选择有效的目标装备")
		return
	}
	candidate := s.snapshotOf(p).Clone()
	toolSlot, tool, err := spiritSelection(candidate.Bag, req.SpiritItem, s.itemDef)
	if err != nil {
		reject("selection", err.Error())
		return
	}
	var target domain.Stack
	bagSlot := -1
	var wornSlot domain.EquipSlot
	count := 0
	candidate.Bag.Each(func(slot int, st domain.Stack) {
		if !st.Empty() && st.UID == uid {
			target = st
			bagSlot = slot
			count++
		}
	})
	candidate.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		if !st.Empty() && st.UID == uid {
			target = st
			wornSlot = slot
			count++
		}
	})
	td, ok := s.itemDef(target.Item)
	if count != 1 || target.Locked || !ok || td.Equip == nil || target.Count != 1 || (target.InstanceKind != domain.ItemInstanceNone && target.InstanceKind != domain.ItemInstanceEquipment) || tool.UID == uid {
		reject("target", "目标装备已变化或被锁定")
		return
	}
	slot := domain.EquipSlot(td.Equip.Slot)
	if err := domain.ValidateSpiritStack(target); err != nil {
		reject("state", "装备灵记录不完整")
		return
	}
	target.InstanceKind = domain.ItemInstanceEquipment
	var returned domain.Stack
	text := ""
	switch req.Op {
	case "spirit.fuse":
		def, ok := domain.SpiritDefinitionForItem(tool.Item)
		// Pre-feature base souls were ordinary equipment instances. Their
		// historical fusion consumed that item and kept fixed template stats.
		// Preserve that meaning explicitly, rather than inventing rolled rows
		// or attempting to change the immutable kind of its old UID.
		if ok && tool.Spirit == (domain.SpiritState{}) && (tool.InstanceKind == domain.ItemInstanceEquipment || tool.InstanceKind == domain.ItemInstanceNone) {
			if !plainLegacySpirit(tool) {
				reject("legacy_soul_state", "此旧灵仍有独立养成属性，不能消耗，以免遗失")
				return
			}
			legacy := domain.NewStack(s.items[tool.Item], 1)
			legacy.Spirit = domain.SpiritState{Kind: def.Kind, LegacyFixed: true}
			legacy.Bound, legacy.Locked = tool.Bound, tool.Locked
			tool = legacy
		}

		if !ok || !def.AcceptsSlot(slot) || domain.ValidateSpiritStack(tool) != nil || (tool.Spirit.AttrCount == 0 && !tool.Spirit.LegacyFixed) {
			reject("requirements", "请选用已解封且部位相合的灵")
			return
		}
		if !target.FusedSpirit.Empty() {
			returned = target.FusedSpirit.Stack()
		} else if target.FusedSoul != 0 {
			old, exists := s.itemDef(target.FusedSoul)
			if !exists {
				reject("state", "原有装备灵记录不完整")
				return
			}
			// Old fixed souls retain their template; never manufacture dynamic rolls.
			returned = domain.NewStack(old, 1)
			returned.Bound = target.Bound
			if _, dynamic := domain.SpiritDefinitionForItem(returned.Item); dynamic {
				returned.Spirit.LegacyFixed = true
			}
		}
		target.FusedSpirit = domain.SpiritAttachmentOf(tool)
		target.FusedSoul = tool.Item
		if tool.Bound {
			target.Bound = true
		}
		text = "灵已融合，原有装备属性保留"
	case "spirit.separate":
		var def domain.SpiritDefinition
		found := false
		for _, d := range domain.SpiritDefinitions() {
			if d.SeparatorID == tool.Item {
				def = d
				found = true
				break
			}
		}
		if !found || !def.AcceptsSlot(slot) || target.FusedSoul == 0 {
			reject("requirements", "请使用对应部位的分离符")
			return
		}
		if !target.FusedSpirit.Empty() {
			if target.FusedSpirit.State.Kind != def.Kind {
				reject("requirements", "分离符与装备灵不符")
				return
			}
			returned = target.FusedSpirit.Stack()
		} else {
			old, exists := s.itemDef(target.FusedSoul)
			if !exists || old.EquipmentSoul == nil || !old.EquipmentSoul.AcceptsSlot(slot) {
				reject("requirements", "没有可分离的装备灵")
				return
			}
			returned = domain.NewStack(old, 1)
			returned.Bound = target.Bound
			if _, dynamic := domain.SpiritDefinitionForItem(returned.Item); dynamic {
				returned.Spirit.LegacyFixed = true
			}
			// Keep fixed-template stats without inventing dynamic processing history.
		}
		target.FusedSoul = 0
		target.FusedSpirit = domain.SpiritAttachment{}
		text = "灵与装备已分离，各自属性保留"
	case "spirit.appearance":
		appearance, ok := domain.SpiritAppearanceForItem(tool.Item)
		def, exists := domain.SpiritDefinitionForKind(appearance.Kind)
		if !ok || !exists || !def.AcceptsSlot(slot) || target.FusedSpirit.Empty() || !appearance.Meets(target.FusedSpirit.State) {
			reject("requirements", "装备上的灵尚未达到此外形所需属性")
			return
		}
		target.SpiritAppearance = tool.Item
		text = "灵外形已融合，附加属性已生效"
	default:
		reject("operation", "不支持的灵操作")
		return
	}
	if !candidate.Bag.RemoveAt(toolSlot, 1) {
		reject("selection", "材料已变化")
		return
	}
	if !returned.Empty() {
		d, exists := s.itemDef(returned.Item)
		if !exists || candidate.Bag.AddStack(d, returned) != 0 {
			reject("bag_full", "背包空间不足，尚未扣除任何物品")
			return
		}
	}
	if bagSlot >= 0 {
		candidate.Bag.Set(bagSlot, target)
	} else {
		candidate.Worn.Set(wornSlot, target)
	}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "spirit.result", Message: text}, nil, 0, nil)
}

// Historical fusion kept only template soul properties. Do not silently discard
// player-authored state when accepting a pre-feature equipment-kind base soul.
func plainLegacySpirit(st domain.Stack) bool {
	if st.DurabilityWearRaw != 0 || (st.MaxDurability > 0 && st.Durability != st.MaxDurability) {
		return false
	}
	st.UID = 0
	st.Item = 0
	st.Count = 0
	st.InstanceKind = 0
	st.Bound = false
	st.Locked = false
	st.Durability = 0
	st.MaxDurability = 0
	st.Endgame.SourceLevel = 0
	st.Endgame.FormationRollBP = 0
	st.Endgame.FormationContentVersion = 0
	return st == (domain.Stack{})
}
