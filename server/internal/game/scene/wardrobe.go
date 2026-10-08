package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const wardrobeExtractVoucher domain.ItemID = 0x350c

func (s *Scene) wardrobeSnapshot(p *entity.Entity) (event.WardrobeSnapshot, error) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Wardrobe == nil {
		return event.WardrobeSnapshot{}, fmt.Errorf("wardrobe snapshot: entity has no wardrobe")
	}
	w := p.Player.Wardrobe
	snap := event.WardrobeSnapshot{Who: p.ID, Capacity: w.Capacity(),
		Items: make([]event.WardrobeItemView, 0, w.Count()+p.Player.Worn.Count())}
	seen := make(map[domain.ItemID]struct{}, w.Count()+p.Player.Worn.Count())
	var buildErr error
	w.Each(func(_ int, entry domain.WardrobeEntry) {
		if buildErr != nil {
			return
		}
		def, ok := s.wardrobes[entry.Item]
		if !ok {
			buildErr = fmt.Errorf("item definition %d is missing", entry.Item)
			return
		}
		if def.Category != entry.Category {
			buildErr = fmt.Errorf("item %d category mismatch persisted=%d config=%d",
				entry.Item, entry.Category, def.Category)
			return
		}
		snap.Items = append(snap.Items, event.WardrobeItemView{
			Item: entry.Item, Category: entry.Category, Worn: entry.Worn,
			Blocked: domain.WardrobeBlocked(def, p.Player.Char.Appear.Gender),
			Name:    def.Name, Desc: def.Description,
		})
		seen[entry.Item] = struct{}{}
	})
	if buildErr != nil {
		return event.WardrobeSnapshot{}, fmt.Errorf("wardrobe snapshot for %s: %w", p.Name, buildErr)
	}
	// 0x1036 kind=0 的绑定变装在参考服中以独立 0x8038 worn 条目下发，
	// 0x8006 equipped.avatar 仍为 0。当前服务端继续以装备实例 UID 保存绑定
	// 归属，这里把绑定事实投影成客户端需要的衣柜穿着视图。
	p.Player.Worn.Each(func(_ domain.EquipSlot, st domain.Stack) {
		if buildErr != nil || st.FusedAppearance == 0 {
			return
		}
		if _, exists := seen[st.FusedAppearance]; exists {
			for index := range snap.Items {
				if snap.Items[index].Item == st.FusedAppearance {
					snap.Items[index].Worn = true
				}
			}
			return
		}
		def, ok := s.wardrobes[st.FusedAppearance]
		if !ok {
			buildErr = fmt.Errorf("bound avatar %d has no wardrobe definition", st.FusedAppearance)
			return
		}
		snap.Items = append(snap.Items, event.WardrobeItemView{
			Item: st.FusedAppearance, Category: def.Category, Worn: true,
			Blocked: domain.WardrobeBlocked(def, p.Player.Char.Appear.Gender),
			Name:    def.Name, Desc: def.Description,
		})
		seen[st.FusedAppearance] = struct{}{}
	})
	if buildErr != nil {
		return event.WardrobeSnapshot{}, fmt.Errorf("wardrobe fusion projection for %s: %w", p.Name, buildErr)
	}
	// 参考服按每类 100 格下发。绑定变装即使魔法衣橱尚未手工开启，也必须
	// 让客户端接收这条独立穿着记录，不能用 capacity=0 把它裁掉。
	if len(snap.Items) > 0 && snap.Capacity < 100 {
		snap.Capacity = 100
	}
	return snap, nil
}

func (s *Scene) pushWardrobe(p *entity.Entity) error {
	if p == nil || p.Player == nil || p.Player.Wardrobe == nil {
		return nil
	}
	snap, err := s.wardrobeSnapshot(p)
	if err != nil {
		s.log.Error("构建衣柜快照失败", "id", p.ID, "err", err)
		return err
	}
	s.emitTo(p.ID, snap)
	return nil
}

func (s *Scene) wardrobeNotice(p *entity.Entity, text string) {
	if p == nil || text == "" {
		return
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
}

// useWardrobeKey 同时服务于衣橱按钮可能使用的 WardrobeStore 路径和普通
// UseItem 路径。容量 0 时一次消耗 10 把开启 10 格；之后每次解锁下一格，
// 费用来自 ov_magicwardrobe(11..200)。
func (s *Scene) useWardrobeKey(p *entity.Entity, tab uint8, slot int32) {
	if p == nil || p.Player == nil || p.Player.Bag == nil || p.Player.Wardrobe == nil ||
		!s.wardrobeRule.Valid() || slot < 0 || slot >= int32(p.Player.Bag.Cap()) {
		return
	}
	stack := p.Player.Bag.At(int(slot))
	def, ok := s.itemDef(stack.Item)
	if !ok || stack.Empty() || stack.Item != s.wardrobeRule.KeyItem || stack.Locked ||
		!def.InventoryTabKnown || def.InventoryTab != tab {
		s.wardrobeNotice(p, "魔法衣橱开启失败：钥匙位置无效")
		return
	}
	next, cost, ok := s.wardrobeRule.NextUnlock(p.Player.Wardrobe.Capacity())
	if !ok {
		if p.Player.Wardrobe.Capacity() >= s.wardrobeRule.MaxCapacity {
			s.wardrobeNotice(p, "魔法衣橱已经达到200格上限")
		} else {
			s.wardrobeNotice(p, "魔法衣橱容量状态无效")
		}
		return
	}
	if p.Player.Bag.UsableCountOf(s.wardrobeRule.KeyItem) < cost {
		s.wardrobeNotice(p, fmt.Sprintf("魔法衣橱需要%d把钥匙，当前数量不足", cost))
		return
	}
	bag, wardrobe := p.Player.Bag.Clone(), p.Player.Wardrobe.Clone()
	if bag == nil || wardrobe == nil || !bag.Remove(s.wardrobeRule.KeyItem, cost) ||
		!wardrobe.SetCapacity(next) {
		s.wardrobeNotice(p, "魔法衣橱开启失败：钥匙或容量状态已变化")
		return
	}
	oldBag, oldWardrobe := p.Player.Bag, p.Player.Wardrobe
	p.Player.Bag, p.Player.Wardrobe = bag, wardrobe
	inventory, invErr := s.inventorySnapshot(p)
	wardrobeSnap, wardrobeErr := s.wardrobeSnapshot(p)
	if invErr != nil || wardrobeErr != nil {
		p.Player.Bag, p.Player.Wardrobe = oldBag, oldWardrobe
		s.wardrobeNotice(p, "魔法衣橱开启失败：服务器无法生成完整状态")
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.emitTo(p.ID, inventory)
	s.pushQuestLog(p)
	s.emitTo(p.ID, wardrobeSnap)
	if oldWardrobe.Capacity() == 0 {
		s.wardrobeNotice(p, "魔法衣橱已开启，当前可用10格")
	} else {
		s.wardrobeNotice(p, fmt.Sprintf("魔法衣橱已解锁第%d格", next))
	}
}

func (s *Scene) onWardrobeStore(cmd WardrobeStore) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil || p.Player.Wardrobe == nil {
		return
	}
	if cmd.Slot < 0 || cmd.Slot >= int32(p.Player.Bag.Cap()) {
		s.wardrobeNotice(p, "存入衣柜失败：背包位置无效")
		return
	}
	stack := p.Player.Bag.At(int(cmd.Slot))
	itemDef, itemOK := s.itemDef(stack.Item)
	wardrobeDef, wardrobeOK := s.wardrobes[stack.Item]
	if stack.Empty() || !itemOK || !itemDef.InventoryTabKnown || itemDef.InventoryTab != cmd.Tab {
		s.wardrobeNotice(p, "存入衣柜失败：该位置没有可用物品")
		return
	}
	if stack.Item == s.wardrobeRule.KeyItem {
		s.useWardrobeKey(p, cmd.Tab, cmd.Slot)
		return
	}
	if !wardrobeOK {
		s.wardrobeNotice(p, "存入魔法衣橱失败：该物品不是已支持的魔法斗篷")
		return
	}
	if p.Player.Wardrobe.Capacity() == 0 {
		s.wardrobeNotice(p, fmt.Sprintf("存入魔法衣橱失败：请先使用%d把魔法衣橱钥匙开启",
			s.wardrobeRule.OpenKeyCost))
		return
	}
	if stack.Locked {
		s.wardrobeNotice(p, "存入衣柜失败：锁定物品不能消耗")
		return
	}
	if p.Player.Wardrobe.Contains(stack.Item) {
		s.wardrobeNotice(p, "存入衣柜失败：该外观已经收藏")
		return
	}
	if p.Player.Wardrobe.Count() >= int(p.Player.Wardrobe.Capacity()) {
		s.wardrobeNotice(p, "存入衣柜失败：衣柜容量已满")
		return
	}

	bag := p.Player.Bag.Clone()
	wardrobe := p.Player.Wardrobe.Clone()
	if bag == nil || wardrobe == nil || !bag.RemoveAt(int(cmd.Slot), 1) ||
		!wardrobe.Add(stack.Item, wardrobeDef.Category) {
		s.wardrobeNotice(p, "存入衣柜失败：物品状态已变化")
		return
	}
	oldBag, oldWardrobe := p.Player.Bag, p.Player.Wardrobe
	p.Player.Bag, p.Player.Wardrobe = bag, wardrobe
	inventory, invErr := s.inventorySnapshot(p)
	wardrobeSnap, wardrobeErr := s.wardrobeSnapshot(p)
	if invErr != nil || wardrobeErr != nil {
		p.Player.Bag, p.Player.Wardrobe = oldBag, oldWardrobe
		s.log.Error("存入衣柜后快照构建失败，已回滚", "char", p.Name,
			"inventoryErr", invErr, "wardrobeErr", wardrobeErr)
		s.wardrobeNotice(p, "存入衣柜失败：服务器无法生成完整状态")
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.emitTo(p.ID, inventory)
	s.pushQuestLog(p)
	s.emitTo(p.ID, wardrobeSnap)
	s.wardrobeNotice(p, "外观已存入衣柜")
}

func (s *Scene) onWardrobeWear(cmd WardrobeWear) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Wardrobe == nil || p.Player.Char == nil {
		return
	}
	def, ok := s.wardrobes[cmd.Item]
	if !ok || !p.Player.Wardrobe.Contains(cmd.Item) {
		s.wardrobeNotice(p, "穿戴外观失败：衣柜中没有该外观")
		return
	}
	if !p.Player.Wardrobe.IsWorn(cmd.Item) &&
		domain.WardrobeBlocked(def, p.Player.Char.Appear.Gender) {
		s.wardrobeNotice(p, "激活魔法斗篷失败：该外观不适用于当前角色")
		return
	}
	if !p.Player.Wardrobe.ToggleWear(cmd.Item) {
		s.wardrobeNotice(p, "切换外观失败：衣柜状态已变化")
		return
	}
	p.Player.MarkDirty()
	s.refreshAppearance(p)
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}

func (s *Scene) onWardrobeRemove(cmd WardrobeRemove) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil ||
		p.Player.Wardrobe == nil || p.Player.Char == nil {
		return
	}
	if !p.Player.Wardrobe.Contains(cmd.Item) {
		s.wardrobeNotice(p, "取出外观失败：衣橱中没有该外观")
		return
	}
	if p.Player.Bag.UsableCountOf(wardrobeExtractVoucher) < 1 {
		s.wardrobeNotice(p, "没有【魔法衣橱提取凭证】，无法取出")
		return
	}
	itemDef, itemOK := s.itemDef(cmd.Item)
	if !itemOK || !itemDef.InventoryTabKnown {
		s.wardrobeNotice(p, "取出外观失败：物品配置不可用")
		return
	}

	wasWorn := p.Player.Wardrobe.IsWorn(cmd.Item)
	bag, wardrobe := p.Player.Bag.Clone(), p.Player.Wardrobe.Clone()
	if bag == nil || wardrobe == nil ||
		!bag.Remove(wardrobeExtractVoucher, 1) {
		s.wardrobeNotice(p, "取出外观失败：提取凭证状态已变化")
		return
	}
	if _, ok := wardrobe.Extract(cmd.Item); !ok || bag.Add(itemDef, 1) != 0 {
		s.wardrobeNotice(p, "取出外观失败：背包空间不足或衣橱状态已变化")
		return
	}

	oldBag, oldWardrobe := p.Player.Bag, p.Player.Wardrobe
	p.Player.Bag, p.Player.Wardrobe = bag, wardrobe
	inventory, invErr := s.inventorySnapshot(p)
	wardrobeSnap, wardrobeErr := s.wardrobeSnapshot(p)
	if invErr != nil || wardrobeErr != nil {
		p.Player.Bag, p.Player.Wardrobe = oldBag, oldWardrobe
		s.log.Error("从魔法衣橱取出后快照构建失败，已回滚", "char", p.Name,
			"inventoryErr", invErr, "wardrobeErr", wardrobeErr)
		s.wardrobeNotice(p, "取出外观失败：服务器无法生成完整状态")
		return
	}

	if wasWorn {
		next := domain.AppearanceFromEquipmentAndWardrobe(
			p.Player.Char.Appear, p.Player.Worn, p.Player.Wardrobe,
			s.itemDef, s.wardrobes, p.Player.Char.Appear.Gender)
		p.Player.Char.Appear = next
		p.Look.Appearance = next
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.emitTo(p.ID, inventory)
	s.pushQuestLog(p)
	if wasWorn {
		s.emit(event.AppearanceChanged{
			Who: p.ID, Appearance: p.Player.Char.Appear,
		})
	}
	s.emitTo(p.ID, wardrobeSnap)
	s.wardrobeNotice(p, "外观已从魔法衣橱取回背包")
}

func (s *Scene) onWardrobeMove(cmd WardrobeMove) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Wardrobe == nil {
		return
	}
	if cmd.To < 0 || cmd.To >= int32(p.Player.Wardrobe.CategoryCount(cmd.Item)) {
		s.wardrobeNotice(p, "调整衣柜顺序失败：目标位置无效")
		return
	}
	if !p.Player.Wardrobe.Move(cmd.Item, int(cmd.To)) {
		// 拖回原位是合法的幂等操作，不需要报错；仍补一份权威快照。
		if p.Player.Wardrobe.Contains(cmd.Item) {
			_ = s.pushWardrobe(p)
			return
		}
		s.wardrobeNotice(p, "调整衣柜顺序失败：衣柜中没有该外观")
		return
	}
	p.Player.MarkDirty()
	_ = s.pushWardrobe(p)
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}
