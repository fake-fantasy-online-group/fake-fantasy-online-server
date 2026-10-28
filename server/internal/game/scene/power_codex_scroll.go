package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
)

func (s *Scene) hasPowerCodex(p *entity.Entity) bool {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || !p.Alive() || !p.Player.Char.Quests.Done(domain.PowerCodexQuestID) {
		return false
	}
	found := false
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == domain.PowerCodexItemID && st.Count == 1 && st.Bound && !st.Locked {
			found = true
		}
	})
	return found
}
func (s *Scene) powerCodexConfiguration() map[string]string {
	d, ok := s.itemDef(domain.PowerCodexItemID)
	if !ok {
		return nil
	}
	return map[string]string{"powerBookItem": "1", "powerCodexItemId": strconv.Itoa(int(domain.PowerCodexItemID)), "powerScrollItemId": strconv.Itoa(int(domain.PowerScrollItemID)), "powerBookName": d.Name, "powerBookNPC": "巧匠左慈"}
}
func powerCodexSlots(power domain.LegendaryPowerDefinition) []string {
	names := map[int32]string{1: "面饰", 2: "头盔", 3: "项链", 4: "单手武器", 5: "盾", 6: "手套", 7: "戒指", 8: "衣服", 9: "鞋", 10: "背包", 11: "法宝", 13: "双手武器", 14: "耳饰", 15: "手镯", 16: "腰带"}
	out := []string{}
	for slot := int32(1); slot <= 25; slot++ {
		if names[slot] == "" {
			continue
		}
		d := domain.ItemDef{Equip: &domain.EquipDef{Slot: slot, LevelReq: 100}}
		if domain.LegendaryPowerEligibleForItem(power, d, domain.Stack{Endgame: domain.EndgameItemState{SourceLevel: 100}}) {
			name := names[slot]
			if name == "" {
				name = "部位" + strconv.Itoa(int(slot))
			}
			out = append(out, name)
		}
	}
	return out
}
func powerUID(value string) (int64, bool) {
	n, e := strconv.ParseInt(value, 10, 64)
	return n, e == nil && n > 0 && strconv.FormatInt(n, 10) == value
}
func (s *Scene) extractPowerScroll(p *entity.Entity, req domain.EndgameRequest, book domain.LegendaryPowerBook) {
	if !s.hasPowerCodex(p) {
		s.endgameError(p, req, "tool_required", "请将秘典带在背包中再提取")
		return
	}
	if req.PowerID == "" || req.TargetUID != "" || req.RuneUID != "" {
		s.endgameError(p, req, "invalid_request", "请选择已收录的威能")
		return
	}
	roll, ok := book[req.PowerID]
	if !ok || !domain.LegendaryPowerExecutable(req.PowerID) {
		s.endgameError(p, req, "power_locked", "此威能尚未收录")
		return
	}
	d, ok := s.itemDef(domain.PowerScrollItemID)
	if !ok || d.Stackable || d.CanTrade || d.CanMail || d.ConsumeOnUse || d.Equip != nil {
		s.endgameError(p, req, "storage", "卷轴尚未备妥")
		return
	}
	candidate := s.snapshotOf(p).Clone()
	st := domain.NewStack(d, 1)
	if st.UID == 0 {
		st.UID = domain.NewItemInstanceID()
	}
	st.Bound = true
	st.Endgame.PowerScrollID = req.PowerID
	if domain.LegendaryPowerHasRoll(req.PowerID) {
		st.Endgame.PowerScrollRollBP = domain.EffectiveLegendaryPowerRollBP(req.PowerID, roll)
	}
	if candidate.Bag.AddStack(d, st) != 0 {
		s.endgameError(p, req, "bag_full", "请先腾出一个背包空位")
		return
	}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "powerbook.result", Message: "威能已凝成卷轴。"}, nil, 0, nil)
}
func (s *Scene) applyPowerScroll(p *entity.Entity, req domain.EndgameRequest) {
	if !p.Alive() {
		s.endgameError(p, req, "requirements", "角色当前不能附着威能")
		return
	}
	target, ok := powerUID(req.TargetUID)
	scrollUID, ok2 := powerUID(req.RuneUID)
	if !ok || !ok2 || target == scrollUID || req.PowerID != "" {
		s.endgameError(p, req, "invalid_request", "请选择卷轴与暗金装备或羽翼")
		return
	}
	scrollSlot, targetSlot, wornSlot := -1, -1, domain.EquipSlot(0)
	var scroll, gear domain.Stack
	count := 0
	p.Player.Bag.Each(func(i int, st domain.Stack) {
		if st.UID == scrollUID {
			scrollSlot = i
			scroll = st
			count++
		}
		if st.UID == target {
			targetSlot = i
			gear = st
			count++
		}
	})
	p.Player.Worn.Each(func(i domain.EquipSlot, st domain.Stack) {
		if st.UID == target {
			wornSlot = i
			gear = st
			count++
		}
	})
	if count != 2 || scrollSlot < 0 || targetSlot < 0 && wornSlot == 0 {
		s.endgameError(p, req, "not_owner", "卷轴或装备已移动，请重新选择")
		return
	}
	if scroll.Item != domain.PowerScrollItemID || scroll.Count != 1 || !scroll.Bound || scroll.Locked || scroll.Endgame.PowerScrollID == "" || !domain.LegendaryPowerRollValid(scroll.Endgame.PowerScrollID, scroll.Endgame.PowerScrollRollBP) {
		s.endgameError(p, req, "invalid_scroll", "卷轴状态不符，尚未消耗")
		return
	}
	d, ok := s.itemDef(gear.Item)
	if !ok {
		s.endgameError(p, req, "requirements", "装备模板无效")
		return
	}
	if targetSlot >= 0 && (!d.InventoryTabKnown || !p.Player.Bag.AllowsSlot(d, targetSlot)) {
		s.endgameError(p, req, "requirements", "装备位置无效")
		return
	}
	snapshotBook := domain.LegendaryPowerBook{scroll.Endgame.PowerScrollID: scroll.Endgame.PowerScrollRollBP}
	var changed domain.Stack
	var err error
	if s.wingRules != nil {
		if _, wing := s.wingRules.Item(gear.Item); wing {
			changed, err = snapshotBook.ImprintWingPower(gear, d, p.Player.Char, scroll.Endgame.PowerScrollID, s.wingRules)
		} else {
			changed, err = snapshotBook.ImprintPower(gear, d, p.Player.Char, scroll.Endgame.PowerScrollID)
		}
	} else {
		changed, err = snapshotBook.ImprintPower(gear, d, p.Player.Char, scroll.Endgame.PowerScrollID)
	}
	if err != nil {
		s.endgameError(p, req, domain.CubeErrorCode(err), err.Error())
		return
	}
	changed.Endgame.Revision++
	candidate := s.snapshotOf(p).Clone()
	candidate.Bag.Set(scrollSlot, domain.Stack{})
	if targetSlot >= 0 {
		candidate.Bag.Set(targetSlot, changed)
	} else {
		candidate.Worn.Set(wornSlot, changed)
	}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "powerbook.result", Message: "威能已附着于器物。", Result: map[string]string{"itemUid": req.TargetUID, "powerId": scroll.Endgame.PowerScrollID}}, nil, 0, func() { s.refreshStats(p) })
}
