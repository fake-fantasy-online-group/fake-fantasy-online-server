package scene

import (
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onSaveHotbar(cmd SaveHotbar) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	ch := p.Player.Char
	for i := 0; i < domain.HotbarSlotCount; i++ {
		ch.Hotbar[i] = s.sanitizeHotbarSlot(p, cmd.Slots[i])
	}
	ch.HotbarExpanded = cmd.Expanded
	p.Player.MarkDirty()
	// 整份回显让客户端立刻采用服务端派生的 icon，并把这次保存闭成一对请求/快照。
	s.emitTo(p.ID, s.hotbarSnapshot(p))
}

func (s *Scene) hotbarSnapshot(p *entity.Entity) event.HotbarSnapshot {
	ch := p.Player.Char
	out := event.HotbarSnapshot{Who: p.ID, Expanded: ch.HotbarExpanded}
	for i := 0; i < len(out.Slots) && i < len(ch.Hotbar); i++ {
		slot := ch.Hotbar[i]
		clean := s.sanitizeHotbarSlot(p, slot)
		out.Slots[i] = event.HotbarSlotView{ID: clean.ID, Kind: clean.Kind}
		if clean.ID == 0 {
			continue
		}
		switch clean.Kind {
		case domain.HotbarKindSkill:
			lv, _ := s.skillLevelForUse(p, domain.SkillID(clean.ID))
			if d, ok := s.skills.Get(domain.SkillID(clean.ID), lv); ok {
				out.Slots[i].Icon = d.Icon
			}
		case domain.HotbarKindPetSkill:
			out.Slots[i].Icon = s.petSkills[domain.SkillID(clean.ID)].Icon
		case domain.HotbarKindItem:
			if d, ok := s.itemDef(domain.ItemID(clean.ID)); ok {
				out.Slots[i].Icon = d.Icon
			}
		}
	}
	return out
}

// sanitizeHotbarSlot 按客户端已经证明的类别验证一格。宠物技能检查角色拥有的
// 全部已孵化宠物，而不是只检查当前出战宠：收回或换宠后快捷栏仍应保留，点击时
// 再由 UsePetSkill 验证当前出战宠是否真的会该技能。
func (s *Scene) sanitizeHotbarSlot(p *entity.Entity, slot domain.HotbarSlot) domain.HotbarSlot {
	if p == nil || p.Player == nil || p.Player.Char == nil || slot.ID <= 0 {
		return domain.HotbarSlot{}
	}
	switch slot.Kind {
	case domain.HotbarKindSkill:
		if lv, _ := s.skillLevelForUse(p, domain.SkillID(slot.ID)); lv > 0 && s.skills != nil {
			if _, ok := s.skills.Get(domain.SkillID(slot.ID), lv); ok {
				return slot
			}
		}
		// 正式客户端的宠物技能拖拽入口会正确记录 isPet，但落到快捷栏时仍先
		// 以人物技能类型 0 上报。若它不是角色技能、却是该角色宠物确实学会的
		// 数据库宠物技能，在协议边界规范成类型 1；服务端回显后客户端点击该格
		// 就会走 NetClient.PetUseSkill，而不是人物 UseSkill。
		id := domain.SkillID(slot.ID)
		if d, ok := s.petSkills[id]; ok && d.Active && characterPetKnows(p.Player.Char, id) {
			slot.Kind = domain.HotbarKindPetSkill
			return slot
		}
	case domain.HotbarKindPetSkill:
		id := domain.SkillID(slot.ID)
		if d, ok := s.petSkills[id]; ok && d.Active && characterPetKnows(p.Player.Char, id) {
			return slot
		}
	case domain.HotbarKindItem:
		if _, ok := s.itemDef(domain.ItemID(slot.ID)); ok {
			return slot
		}
	}
	return domain.HotbarSlot{}
}

func characterPetKnows(ch *domain.Character, skill domain.SkillID) bool {
	if ch == nil || skill == 0 {
		return false
	}
	for i := range ch.Pets {
		if ch.Pets[i].Hatched && ch.Pets[i].Knows(skill) {
			return true
		}
	}
	return false
}

func (s *Scene) onSetSmartCast(cmd SetSmartCast) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil {
		p.Player.Char.SmartCast = cmd.On
		p.Player.MarkDirty()
	}
}

func (s *Scene) onSetPetView(cmd SetPetView) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil {
		p.Player.Char.PetViewMask = cmd.Mask
		p.Player.MarkDirty()
	}
}

func (s *Scene) onSetGlowMode(cmd SetGlowMode) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil {
		p.Player.Char.Appear.GlowMode = cmd.Mode
		p.Player.Char.Appear.GlowModeKnown = true
		p.Look.Appearance.GlowMode = cmd.Mode
		p.Look.Appearance.GlowModeKnown = true
		p.Player.MarkDirty()
		s.emit(event.AppearanceChanged{Who: p.ID, Appearance: p.Look.Appearance})
	}
}

func (s *Scene) onSetTitle(cmd SetTitle) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	// 防止一条合法 Str 把角色行和广播包无限放大。现有称号资源最长远小于 64 字节。
	custom := p.Player.Char.OwnsTitle("自定义称号")
	if !utf8.ValidString(cmd.Title) || len([]byte(cmd.Title)) > 64 ||
		(cmd.Title != "" && !p.Player.Char.OwnsTitle(cmd.Title) && !custom) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: cmd.CmdName(), Reason: event.RejectInvalid})
		return
	}
	p.Player.Char.Appear.Title = cmd.Title
	p.Look.Appearance.Title = cmd.Title
	p.Player.MarkDirty()
	s.pushTitleSnapshot(p)
	s.emit(event.AppearanceChanged{Who: p.ID, Appearance: p.Look.Appearance})
}

func (s *Scene) pushTitleSnapshot(p *entity.Entity) {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return
	}
	ch := p.Player.Char
	owned := append([]string(nil), ch.OwnedTitles...)
	s.emitTo(p.ID, event.TitleSnapshot{Who: p.ID, Current: ch.Appear.Title, Owned: owned})
}

func (s *Scene) onSetEquipFXMask(cmd SetEquipFXMask) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil {
		p.Player.Char.Appear.EquipFXHideMask = cmd.Mask
		p.Look.Appearance.EquipFXHideMask = cmd.Mask
		p.Player.MarkDirty()
	}
}
