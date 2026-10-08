package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onChangeHair(cmd ChangeHair) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	notice := func(text string) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	}
	if !p.Alive() || p.Player.Work != nil || p.Player.Riding || p.Player.Stall != nil {
		notice("改变发型失败：死亡、打工、骑乘或摆摊状态下不能操作")
		return
	}
	option, ok := s.hairRules.Get(cmd.Mode, cmd.Target)
	if !ok {
		notice("改变发型失败：选项不存在")
		return
	}
	ch := p.Player.Char
	current := ch.Appear.Hair
	label := "发色"
	if cmd.Mode == domain.HairStyle {
		current = ch.Appear.Head
		label = "发型"
	}
	if current == cmd.Target {
		s.emit(event.HairChanged{Who: p.ID, Appearance: ch.Appear})
		notice(label + "已经是当前选择")
		return
	}
	money, err := inventoryMoney(ch.Money)
	if err != nil || money < option.Money {
		notice(fmt.Sprintf("改变%s失败：需要%d铜币", label, option.Money))
		return
	}
	bag := p.Player.Bag.Clone()
	if bag == nil {
		notice("改变" + label + "失败：背包状态无效")
		return
	}
	if option.DyeItem != 0 {
		def, exists := s.itemDef(option.DyeItem)
		if !exists || bag.UsableCountOf(option.DyeItem) < option.DyeCount ||
			!bag.Remove(option.DyeItem, option.DyeCount) {
			name := fmt.Sprintf("染发剂%d", option.DyeItem)
			if exists {
				name = def.Name
			}
			notice(fmt.Sprintf("改变%s失败：需要%s×%d", label, name, option.DyeCount))
			return
		}
	}
	beforeBag, beforeMoney, beforeAppearance := p.Player.Bag, ch.Money, ch.Appear
	p.Player.Bag = bag
	ch.Money = moneyFromInventory(money - option.Money)
	if cmd.Mode == domain.HairStyle {
		ch.Appear.Head = cmd.Target
	} else {
		ch.Appear.Hair = cmd.Target
	}
	p.Look.Appearance = ch.Appear
	inventory, err := s.inventorySnapshot(p)
	if err != nil {
		p.Player.Bag, ch.Money, ch.Appear = beforeBag, beforeMoney, beforeAppearance
		p.Look.Appearance = beforeAppearance
		notice("改变" + label + "失败：服务器无法生成完整状态")
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.emitTo(p.ID, inventory)
	s.pushQuestLog(p)
	s.emit(event.HairChanged{Who: p.ID, Appearance: ch.Appear})
	notice(fmt.Sprintf("%s已更换为%d号", label, cmd.Target))
}
