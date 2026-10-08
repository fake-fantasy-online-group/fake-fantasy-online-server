package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func washRef(cmd WashAffix) socketTargetRef {
	ref := socketTargetRef{kind: cmd.Kind, tab: cmd.Tab, bagSlot: cmd.Slot}
	if cmd.Kind == 1 {
		ref.equipSlot = uint8(cmd.Slot)
	}
	return ref
}

func affixText(a domain.Affix) string {
	name := map[int32]string{
		domain.AttrSTR: "力量", domain.AttrINT: "智慧", domain.AttrVIT: "体质",
		domain.AttrAGI: "敏捷", domain.AttrDEX: "灵巧", domain.AttrSPI: "精神",
		domain.AttrDef: "防御", domain.AttrMAtk: "魔法攻击", domain.AttrMDef: "魔法防御",
		domain.AttrHit: "命中", domain.AttrCrit: "暴击率", domain.AttrMCrit: "魔法暴击率",
		domain.AttrAtkSpeed: "攻击速度", domain.AttrMoveSpeed: "移动速度",
		domain.AttrMinAtk: "最小攻击", domain.AttrMaxAtk: "最大攻击",
		domain.AttrMaxHP: "最大生命", domain.AttrMaxMP: "最大法力", domain.AttrHPRegen: "生命恢复",
		domain.AttrMaxWeight: "最大负重", domain.AttrAtk: "攻击",
		domain.AttrPhysRes: "伤害抗性", domain.AttrMagicRes: "魔法抗性",
		domain.AttrStatusRes: "不良状态抗性",
	}[a.Attr]
	if name == "" {
		name = fmt.Sprintf("属性%d", a.Attr)
	}
	if a.Attr == domain.AttrCrit || a.Attr == domain.AttrMCrit {
		return fmt.Sprintf("%s %s", name, basisPointPercent(a.Value))
	}
	suffix := ""
	if a.Mode == domain.ModePercent {
		suffix = "%"
	}
	return fmt.Sprintf("%s %+d%s", name, a.Value, suffix)
}

// basisPointPercent 把装备表万分比转换成人类百分数；1000 -> +10%，50 -> +0.5%。
func basisPointPercent(value int32) string {
	sign := "+"
	if value < 0 {
		sign = "-"
		value = -value
	}
	whole, fraction := value/100, value%100
	if fraction == 0 {
		return fmt.Sprintf("%s%d%%", sign, whole)
	}
	if fraction%10 == 0 {
		return fmt.Sprintf("%s%d.%d%%", sign, whole, fraction/10)
	}
	return fmt.Sprintf("%s%d.%02d%%", sign, whole, fraction)
}

func washInfoOf(p *entity.Entity, ref socketTargetRef, st domain.Stack, pending *entity.WashPending) event.AttrWashInfo {
	info := event.AttrWashInfo{Who: p.ID, Kind: ref.kind, Tab: ref.tab, Slot: domain.ClientBagPosition(ref.bagSlot),
		Item: st.Item, Quality: st.WashQuality}
	if ref.kind == 1 {
		info.Slot = int32(ref.equipSlot)
	}
	affixes, count := st.WashAffixes, st.WashCount
	if pending != nil {
		affixes, count, info.Quality = pending.Affixes, pending.Count, pending.Quality
	}
	for i := 0; i < int(count) && i < len(affixes); i++ {
		info.Affixes = append(info.Affixes, affixText(affixes[i]))
	}
	return info
}

func pendingMatches(p *entity.Entity, ref socketTargetRef, st domain.Stack) *entity.WashPending {
	pending := p.Player.WashPending
	if pending == nil || pending.TargetKind != ref.kind || pending.EquipSlot != ref.equipSlot ||
		pending.Tab != ref.tab || pending.BagSlot != ref.bagSlot || pending.Item != st.Item || pending.UID != st.UID {
		return nil
	}
	return pending
}

func (s *Scene) onWashAffix(cmd WashAffix) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || s.washes == nil {
		return
	}
	ref := washRef(cmd)
	st, def, ok := s.socketTarget(p, ref)
	if !ok {
		return
	}
	pending := pendingMatches(p, ref, st)
	switch {
	case cmd.Index == -1:
		s.emitTo(p.ID, s.washInfo(p, ref, st, nil))
		return
	case cmd.Index == -2:
		p.Player.WashPending = nil
		s.emitTo(p.ID, s.washInfo(p, ref, st, nil))
		return
	case cmd.Index == -3:
		if pending == nil {
			s.emitTo(p.ID, s.washInfo(p, ref, st, nil))
			return
		}
		st.WashQuality, st.WashCount, st.WashAffixes = pending.Quality, pending.Count, pending.Affixes
		setSocketTarget(p, ref, st)
		p.Player.WashPending = nil
		p.Player.MarkDirty()
		_ = s.pushInventory(p)
		if ref.kind == 1 {
			s.refreshStats(p)
		}
		s.emitTo(p.ID, s.washInfo(p, ref, st, nil))
		if s.saver != nil {
			s.saver.Save(s.snapshotOf(p))
		}
		return
	case cmd.Index < 0 || cmd.Index > 3:
		return
	}
	quality := uint8(cmd.Index + 1)
	recipe, ok := s.washes.Get(domain.RefineClass(def), def.Equip.Tier, quality)
	if !ok {
		return
	}
	next := p.Player.Bag.Clone()
	if next == nil || !next.Remove(recipe.Material, recipe.MaterialQty) {
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < recipe.Cost {
		return
	}
	choices := append([]domain.WashChoice(nil), recipe.Choices...)
	var affixes [4]domain.Affix
	count := int(quality)
	if count > len(choices) {
		count = len(choices)
	}
	for i := 0; i < count; i++ {
		total := int32(0)
		for _, c := range choices {
			total += c.Weight
		}
		if total <= 0 {
			break
		}
		pick := int32(s.rng.Intn(int(total)))
		chosen := 0
		for j, c := range choices {
			if pick < c.Weight {
				chosen = j
				break
			}
			pick -= c.Weight
		}
		affixes[i] = choices[chosen].Affix
		choices = append(choices[:chosen], choices[chosen+1:]...)
	}
	p.Player.Bag = next
	p.Player.Char.Money = moneyFromInventory(money - recipe.Cost)
	p.Player.WashPending = &entity.WashPending{TargetKind: ref.kind, EquipSlot: ref.equipSlot,
		Tab: ref.tab, BagSlot: ref.bagSlot, UID: st.UID, Item: st.Item, Quality: quality, Count: uint8(count), Affixes: affixes}
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	s.emitTo(p.ID, s.washInfo(p, ref, st, p.Player.WashPending))
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "洗练完成，请确认是否替换原属性。"})
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}

// washInfo uses the same PostgreSQL recipe as the actual affix roll.
func (s *Scene) washInfo(p *entity.Entity, ref socketTargetRef, st domain.Stack, pending *entity.WashPending) event.AttrWashInfo {
	info := washInfoOf(p, ref, st, pending)
	affixes := st.WashAffixes
	if pending != nil {
		affixes = pending.Affixes
	}
	def, exists := s.itemDef(st.Item)
	var recipe domain.WashRecipe
	ok := false
	if exists && def.Equip != nil {
		recipe, ok = s.washes.Get(domain.RefineClass(def), def.Equip.Tier, info.Quality)
	}
	for i := range info.Affixes {
		atMax, found := true, false
		if ok {
			for _, c := range recipe.Choices {
				if c.Affix.Attr == affixes[i].Attr && c.Affix.Mode == affixes[i].Mode {
					found = true
					if c.Affix.Value > affixes[i].Value {
						atMax = false
					}
				}
			}
		}
		info.AtMax = append(info.AtMax, found && atMax)
	}
	return info
}
