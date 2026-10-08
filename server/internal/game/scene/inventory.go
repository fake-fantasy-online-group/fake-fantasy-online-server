package scene

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// inventorySnapshot builds the only client-facing view of a player's bag,
// equipment and currencies. It validates the whole view before returning so a
// caller can never emit a partially trustworthy inventory.
func (s *Scene) inventorySnapshot(p *entity.Entity) (event.InventorySnapshot, error) {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot: entity is not a player")
	}

	ch := p.Player.Char
	money, err := inventoryMoney(ch.Money)
	if err != nil {
		return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: %w", ch.Name, err)
	}
	snap := event.InventorySnapshot{
		Who: p.ID, Money: money, Caiyu: ch.Caiyu, Honor: ch.Honor,
		MaxWeight: p.Stats.MaxWeight,
		Items:     make([]event.InventoryItemView, 0),
		Equipped:  make([]event.EquippedItemView, 0, p.Player.Worn.Count()),
	}

	var weight int64
	var buildErr error
	p.Player.Bag.Each(func(slot int, st domain.Stack) {
		if buildErr != nil {
			return
		}
		def, ok := s.itemDef(st.Item)
		if !ok {
			buildErr = fmt.Errorf("bag slot %d: item definition %d is missing", slot, st.Item)
			return
		}
		if !def.InventoryTabKnown {
			buildErr = fmt.Errorf("bag slot %d: inventory tab for item %d is unknown", slot, st.Item)
			return
		}
		weight, buildErr = addInventoryWeight(weight, def.Weight, st.Count)
		if buildErr != nil {
			buildErr = fmt.Errorf("bag slot %d item %d: %w", slot, st.Item, buildErr)
			return
		}

		clientSlot := slot
		if p.Player.Bag.Cap() >= domain.DefaultBagSlots {
			if !p.Player.Bag.AllowsSlot(def, slot) {
				buildErr = fmt.Errorf("bag slot %d item %d is outside tab %d", slot, st.Item, def.InventoryTab)
				return
			}
			clientSlot %= domain.BagPageSlots
		}
		view := event.InventoryItemView{
			BagIndex: int32(clientSlot), Item: st.Item, Count: st.Count,
			Tab: def.InventoryTab, Name: itemDisplayName(def, st), Desc: s.ownedItemTooltip(def, st, p),
			Sell: def.SellPrice, Slot: -1, Bound: st.Bound, Blocked: s.equipmentBlockedFor(p, def),
			Locked: st.Locked, Quality: equipmentQuality(def, st), Cosmetic: def.AvatarFusion != nil,
			FusedDesc: s.fusedDescription(st),
		}
		if def.Equip != nil {
			view.CurrentDurability = st.Durability
			view.MaxDurability = domain.MaxDurabilityOf(st, def)
			view.Slot = domain.EquipSlot(def.Equip.Slot)
			// 官方抓包与参考服一致：装备/背包装备的 avatar 不承载变装。
			// 变装独立走 0x8038 衣柜状态和 0x800a 外观链。
			view.Avatar = 0
		}
		if def.PetCarrierSpecies > 0 {
			view.Pet = s.petItemInfo(p, st)
			if view.Pet == nil {
				buildErr = fmt.Errorf("pet carrier %d has no pet", st.UID)
				return
			}
			view.Name = s.ownedItemName(p, def, st)
			view.Desc = view.Name
		}
		snap.Items = append(snap.Items, view)
	})
	if buildErr != nil {
		return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: %w", ch.Name, buildErr)
	}

	// 快速换装面板只是另一处随身托管位置，不是免负重仓库。物品详情由
	// 0x8073 下发，但 0x8006 的总负重仍必须包含这些备用装备。
	if p.Player.ChangeSet != nil {
		p.Player.ChangeSet.Each(func(cell domain.EquipSlot, st domain.Stack) {
			if buildErr != nil {
				return
			}
			def, ok := s.itemDef(st.Item)
			if !ok || def.Equip == nil || domain.ChangeSetCell(domain.EquipSlot(def.Equip.Slot)) != cell {
				buildErr = fmt.Errorf("change set cell %d: invalid equipment %d", cell, st.Item)
				return
			}
			weight, buildErr = addInventoryWeight(weight, def.Weight, 1)
		})
		if buildErr != nil {
			return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: %w", ch.Name, buildErr)
		}
	}

	type wornItem struct {
		slot domain.EquipSlot
		st   domain.Stack
	}
	worn := make([]wornItem, 0, p.Player.Worn.Count())
	p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		worn = append(worn, wornItem{slot: slot, st: st})
	})
	sort.Slice(worn, func(i, j int) bool { return worn[i].slot < worn[j].slot })
	for _, item := range worn {
		def, ok := s.itemDef(item.st.Item)
		if !ok {
			return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: worn slot %d item definition %d is missing", ch.Name, item.slot, item.st.Item)
		}
		if !def.InventoryTabKnown {
			return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: inventory tab for worn item %d is unknown", ch.Name, item.st.Item)
		}
		if def.Equip == nil {
			return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: worn slot %d item %d is not equipment", ch.Name, item.slot, item.st.Item)
		}
		weight, err = addInventoryWeight(weight, def.Weight, item.st.Count)
		if err != nil {
			return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: worn slot %d item %d: %w", ch.Name, item.slot, item.st.Item, err)
		}
		snap.Equipped = append(snap.Equipped, event.EquippedItemView{
			Slot: item.slot, Item: item.st.Item, Name: itemDisplayName(def, item.st),
			Desc:              s.ownedItemTooltip(def, item.st, p),
			CurrentDurability: item.st.Durability, MaxDurability: domain.MaxDurabilityOf(item.st, def),
			Avatar: 0, SoulAvatar: int32(item.st.FusedSoul),
			Quality: equipmentQuality(def, item.st),
		})
	}
	if weight < math.MinInt32 || weight > math.MaxInt32 {
		return event.InventorySnapshot{}, fmt.Errorf("inventory snapshot for %s: weight %d does not fit int32", ch.Name, weight)
	}
	snap.Weight = int32(weight)
	return snap, nil
}

// onMoveBagItem 处理正式客户端 0x100d。客户端只提交分页和两个格号；
// 来源物品、可堆叠性及最终快照都由场景内权威背包决定。
func (s *Scene) onMoveBagItem(cmd MoveBagItem) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil {
		return
	}
	bag := p.Player.Bag
	if cmd.From < 0 || cmd.From >= bag.Cap() || cmd.To < 0 || cmd.To >= bag.Cap() || cmd.From == cmd.To {
		_ = s.pushInventory(p)
		return
	}
	src := bag.At(cmd.From)
	def, ok := s.itemDef(src.Item)
	if src.Empty() || !ok || !def.InventoryTabKnown || def.InventoryTab != cmd.Tab {
		_ = s.pushInventory(p)
		return
	}

	planned := bag.Clone()
	if planned == nil {
		return
	}
	if def.Stackable {
		if !planned.Move(cmd.From, cmd.To) {
			_ = s.pushInventory(p)
			return
		}
	} else {
		// Bag.Move 的合并分支无法自行知道物品模板是否可堆叠；不可堆叠物
		// 必须始终交换，不能把两件同模板装备合成 count=2。
		dst := planned.At(cmd.To)
		planned.Set(cmd.From, dst)
		planned.Set(cmd.To, src)
	}

	old, oldQuote := p.Player.Bag, p.Player.RepairQuote
	p.Player.Bag = planned
	p.Player.RepairQuote = nil // 报价引用格号；移动后旧报价必须失效。
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag = old
		p.Player.RepairQuote = oldQuote
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("移动背包物品", "char", p.Name, "tab", cmd.Tab, "from", cmd.From, "to", cmd.To)
}

// onSortBag 处理正式客户端 0x1078。当前背包模型使用一套权威线性格号，
// 因此把所选分页排到前面并按物品号稳定排序，其余分页保持原有相对顺序；
// 这样所选页从第 0 格连续显示，同时不会丢失或伪造任何物品。
func (s *Scene) onSortBag(cmd SortBag) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Bag == nil {
		return
	}
	bag := p.Player.Bag
	selected := make([]domain.Stack, 0)
	rest := make([]domain.Stack, 0)
	pageStart, pageOK := domain.ClientBagSlot(cmd.Tab, 0)
	fourPages := bag.Cap() >= domain.DefaultBagSlots && pageOK
	if fourPages {
		for slot := pageStart; slot < pageStart+domain.BagPageSlots; slot++ {
			if st := bag.At(slot); !st.Empty() {
				selected = append(selected, st)
			}
		}
	} else {
		bag.Each(func(_ int, st domain.Stack) {
			def, ok := s.itemDef(st.Item)
			if ok && def.InventoryTabKnown && def.InventoryTab == cmd.Tab {
				selected = append(selected, st)
				return
			}
			rest = append(rest, st)
		})
	}
	if len(selected) == 0 {
		_ = s.pushInventory(p)
		return
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Item != selected[j].Item {
			return selected[i].Item < selected[j].Item
		}
		if selected[i].Locked != selected[j].Locked {
			return !selected[i].Locked
		}
		if selected[i].Bound != selected[j].Bound {
			return !selected[i].Bound
		}
		return selected[i].UID < selected[j].UID
	})
	selected = s.mergeSortedBagStacks(selected)
	ordered := selected
	if !fourPages {
		ordered = append(ordered, rest...)
	}

	planned := bag.Clone()
	if planned == nil || len(ordered) > planned.Cap() {
		_ = s.pushInventory(p)
		return
	}
	changed := false
	first, last := 0, planned.Cap()
	if fourPages {
		first, last = pageStart, pageStart+domain.BagPageSlots
	}
	for slot := first; slot < last; slot++ {
		var next domain.Stack
		index := slot - first
		if index < len(ordered) {
			next = ordered[index]
		}
		if planned.At(slot) != next {
			changed = true
		}
		planned.Set(slot, next)
	}
	if !changed {
		_ = s.pushInventory(p)
		return
	}

	old, oldQuote := p.Player.Bag, p.Player.RepairQuote
	p.Player.Bag = planned
	p.Player.RepairQuote = nil
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag = old
		p.Player.RepairQuote = oldQuote
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("整理背包", "char", p.Name, "tab", cmd.Tab, "items", len(selected))
}

const dropPlacementRange = 150

// onDropBagItem 处理 0x100f。客户端给出的格号、物品号与落点都只是引用；
// 服务端复核背包实例、分页、距离和 MASK 后，才把同一份实例移到地面。
func (s *Scene) onDropBagItem(cmd DropBagItem) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Bag == nil {
		return
	}
	if p.Player.Stall != nil {
		s.stallNotice(p, "摆摊中不能丢弃物品，请先结束摊位")
		return
	}
	bag := p.Player.Bag
	if s.alloc == nil || cmd.Slot < 0 || cmd.Slot >= bag.Cap() {
		_ = s.pushInventory(p)
		return
	}
	st := bag.At(cmd.Slot)
	def, ok := s.itemDef(st.Item)
	if st.InstanceKind == domain.ItemInstancePet {
		s.petCarrierNotice(p)
		return
	}
	if st.Empty() || !ok || st.Item != cmd.Item || !def.InventoryTabKnown ||
		def.InventoryTab != cmd.Tab || (st.Bound && !domain.IsStartingSword(st.Item)) || st.Locked {
		_ = s.pushInventory(p)
		return
	}
	at := cmd.At
	at.MapID = p.Pos.MapID
	if sqDist(p.Pos, at) > dropPlacementRange*dropPlacementRange ||
		s.walkable != nil && !s.walkable(at) {
		_ = s.pushInventory(p)
		return
	}

	planned := bag.Clone()
	if planned == nil || !planned.Set(cmd.Slot, domain.Stack{}) {
		_ = s.pushInventory(p)
		return
	}
	old, oldQuote := p.Player.Bag, p.Player.RepairQuote
	p.Player.Bag, p.Player.RepairQuote = planned, nil
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag, p.Player.RepairQuote = old, oldQuote
		return
	}
	s.spawnPlayerDrop(def, st, at, p.ID)
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("丢弃背包物品", "char", p.Name, "item", def.Name, "count", st.Count,
		"tab", cmd.Tab, "slot", cmd.Slot, "confirmed", cmd.Confirmed, "x", at.X, "y", at.Y)
}

// onSplitBagItem 处理 0x1016。count 是拆到新格中的数量，不是拆分后的剩余量。
func (s *Scene) onSplitBagItem(cmd SplitBagItem) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Bag == nil {
		return
	}
	bag := p.Player.Bag
	if cmd.Slot < 0 || cmd.Slot >= bag.Cap() || cmd.Count <= 0 {
		_ = s.pushInventory(p)
		return
	}
	st := bag.At(cmd.Slot)
	def, ok := s.itemDef(st.Item)
	if st.Empty() || !ok || !def.Stackable || !def.InventoryTabKnown ||
		def.InventoryTab != cmd.Tab || cmd.Count >= st.Count || st.UID != 0 || st.Locked {
		_ = s.pushInventory(p)
		return
	}
	free := bag.FirstEmptyFor(def)
	if free < 0 {
		_ = s.pushInventory(p)
		return
	}

	planned := bag.Clone()
	if planned == nil {
		return
	}
	remain, split := st, st
	remain.Count -= cmd.Count
	split.Count = cmd.Count
	if !planned.Set(cmd.Slot, remain) || !planned.Set(free, split) {
		_ = s.pushInventory(p)
		return
	}
	old, oldQuote := p.Player.Bag, p.Player.RepairQuote
	p.Player.Bag, p.Player.RepairQuote = planned, nil
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag, p.Player.RepairQuote = old, oldQuote
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("拆分背包物品", "char", p.Name, "item", def.Name, "count", cmd.Count,
		"from", cmd.Slot, "to", free)
}

// mergeSortedBagStacks 合并整理页中可堆叠且实例属性一致的相邻堆。
// 不可堆叠物和绑定状态不同的同名物品仍各占一格。
func (s *Scene) mergeSortedBagStacks(stacks []domain.Stack) []domain.Stack {
	out := make([]domain.Stack, 0, len(stacks))
	for _, st := range stacks {
		def, ok := s.itemDef(st.Item)
		if !ok || !def.Stackable || st.UID != 0 {
			out = append(out, st)
			continue
		}
		for st.Count > 0 {
			if len(out) > 0 {
				last := &out[len(out)-1]
				if last.Item == st.Item && last.UID == 0 && last.Bound == st.Bound &&
					last.Locked == st.Locked && last.Count < domain.MaxStack {
					put := domain.MaxStack - last.Count
					if put > st.Count {
						put = st.Count
					}
					last.Count += put
					st.Count -= put
					continue
				}
			}
			put := st.Count
			if put > domain.MaxStack {
				put = domain.MaxStack
			}
			next := st
			next.Count = put
			out = append(out, next)
			st.Count -= put
		}
	}
	return out
}

// equipmentQuality 是正式客户端用于装备名与属性行着色的品质枚举。
// 用户确认的属性条数颜色不是枚举自然顺序：1深蓝、2淡蓝、3紫、4绿、5黄。
// 客户端枚举为1绿/2深蓝/3紫/4黄。两条属性的淡蓝通过已有0x05文本码
// 表达，协议品质保留0，不向客户端发送不存在的品质枚举。
func equipmentQuality(def domain.ItemDef, stacks ...domain.Stack) uint8 {
	if def.Equip == nil {
		return 0
	}
	var st domain.Stack
	if len(stacks) != 0 {
		st = stacks[0]
	}
	n := domain.EquipmentAttributeCount(def, st)
	if n > 5 {
		n = 5
	}
	return [...]uint8{
		0, // 0条：白
		2, // 1条：深蓝
		0, // 2条：淡蓝暂无已确认枚举
		3, // 3条：紫
		1, // 4条：绿
		4, // 5条及以上：黄
	}[n]
}

// equipmentBlockedFor 为 0x8006 item.blocked 提供角色当下的可穿戴状态。
// 这与 locked（玩家手动锁定实例）、bound（永久绑定）不是同一件事。
// 真正穿戴时 onEquip 会使用同一组规则再做一次权威校验，不信任客户端。
func (s *Scene) equipmentBlockedFor(p *entity.Entity, def domain.ItemDef) bool {
	if def.Equip == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return false
	}
	ch := p.Player.Char
	if def.Equip.Need.Meet(ch.Level, s.equipmentRequirementBaseFromWorn(p, s.functionalWorn(p), domain.EquipSlot(def.Equip.Slot)), ch.Appear.Gender) != domain.RejectNone {
		return true
	}
	return !def.Equip.Need.AllowsCharacter(ch)
}

// itemTooltip 生成 InventoryDlg.Item.desc / EquipDlg.EquippedItem.desc。
//
// 这只是客户端已定义的悬浮提示字符串，不是装备数值的传输通道：
// 槽位/耐久/绑定仍分别写 0x8006 的独立字段，穿戴要求由 onEquip
// 判定，最终六维与战斗值由 0x8007 下发并用于服务端战斗。
func (s *Scene) itemTooltip(def domain.ItemDef, st domain.Stack) string {
	return s.ownedItemTooltip(def, st, nil)
}

// ownedItemTooltip 的人物上下文仅用于需求颜色和套装激活判定。
// 不在服务端描述中添加“已装备/未装备”标签。
func (s *Scene) ownedItemTooltip(def domain.ItemDef, st domain.Stack, owner *entity.Entity) string {
	var ch *domain.Character
	if owner != nil && owner.Player != nil {
		ch = owner.Player.Char
	}
	if def.Equip == nil {
		lines := make([]string, 0, 2)
		if def.Weight != 0 {
			lines = append(lines, fmt.Sprintf("重量 %d", def.Weight))
		}
		text := strings.TrimSpace(def.Description)
		if st.InstanceKind == domain.ItemInstanceSocketCard && len(st.Card.StaticAffixes()) > 0 {
			// 数值只取实例；原始描述仅保留部位限制，避免模板旧数值与实例并列。
			for _, affix := range st.Card.StaticAffixes() {
				lines = append(lines, affixText(affix))
			}
			if at := strings.Index(text, "只可"); at >= 0 {
				lines = append(lines, text[at:])
			}
		} else if text != "" {
			lines = append(lines, text)
		}
		return strings.Join(lines, "\n")
	}

	e := def.Equip
	quality := equipmentQuality(def, st)
	textQuality := quality
	if domain.EquipmentAttributeCount(def, st) == 2 {
		textQuality = tooltipLightBlue
	}
	lines := []string{"\x07(" + equipmentKindName(e) + ")", ""}
	if st.Bound {
		lines = append(lines, "（绑定）（可用天工剪解除绑定）")
	}
	// 用户提供的原版界面截图明确：变装名及其属性整段位于耐久度上方；
	// 正式客户端原生标签使用半角冒号加右侧空格（例如“需要等级: {0}”）；
	// 外观层和部位灵层沿用同一排版，属性行直接跟随，不添加服务端自造前缀。
	if st.FusedAppearance != 0 {
		if fused, ok := s.itemDef(st.FusedAppearance); ok && fused.AvatarFusion != nil {
			lines = append(lines, qualityColoredLine(4, "变装: "+fused.Name))
			for _, affix := range fused.AvatarFusion.Affixes {
				lines = append(lines, qualityColoredLine(4, affixText(affix)))
			}
		}
	}
	if st.FusedSoul != 0 {
		if soul, ok := s.itemDef(st.FusedSoul); ok && soul.EquipmentSoul != nil {
			lines = append(lines, qualityColoredLine(4, "变装: "+soul.Name))
			for _, affix := range soul.EquipmentSoul.Affixes {
				lines = append(lines, qualityColoredLine(4, affixText(affix)))
			}
		}
	}
	if st.FusedDragon != 0 {
		if dragon, ok := s.itemDef(st.FusedDragon); ok && dragon.DragonFusion != nil {
			lines = append(lines, qualityColoredLine(4, "龙系列: "+dragon.Name))
			for _, affix := range dragon.DragonFusion.Affixes {
				lines = append(lines, qualityColoredLine(4, affixText(affix)))
			}
		}
	}
	lines = append(lines, fmt.Sprintf("耐久度: %d/%d", st.Durability, domain.MaxDurabilityOf(st, def)))
	actual := domain.Base{}
	if ch != nil {
		actual = ch.EffectiveBase()
		if owner != nil && owner.Player != nil {
			actual = s.equipmentRequirementBaseFromWorn(owner, s.functionalWorn(owner), domain.EquipSlot(e.Slot))
		}
	}
	appendEquipmentRequirements(&lines, e, ch, actual)
	lines = append(lines, "")
	equipBase, equipStats := e.RefinedIntrinsic(st.RefineLevel)
	appendIntrinsicEquipmentStats(&lines, equipStats, equipBase)
	if def.Weight != 0 {
		lines = append(lines, fmt.Sprintf("重量 %d", def.Weight))
	}
	lines = append(lines, "")
	appendEquipmentAffixes(&lines, e.Affixes, textQuality)
	if st.RefineLevel > 0 {
		if refine, ok := e.RefineEffectAt(st.RefineLevel); !ok {
			text := "精炼属性未配置"
			if refine.DisabledReason != "" {
				text = refine.DisabledReason
			}
			lines = append(lines, "\x02"+text)
		}
	}
	for i := 0; i < int(st.RolledAffixCount) && i < len(st.RolledAffixes); i++ {
		lines = append(lines, qualityColoredLine(textQuality,
			affixText(st.RolledAffixes[i].Affix)))
	}
	for i := 0; i < int(st.WashCount) && i < len(st.WashAffixes); i++ {
		appendEquipmentAffixes(&lines, []domain.Affix{st.WashAffixes[i]}, textQuality)
	}
	// 镶嵌显示与 Compute 读取相同的卡实例属性快照。
	usedSockets := domain.UsedEquipmentSockets(st)
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		if st.Sockets[i] == 0 {
			continue
		}
		appendEquipmentAffixes(&lines, st.SocketCards[i].StaticAffixes(), textQuality)
	}
	// 分母是这件装备实际已打出的孔，不是模板可打孔上限。
	// 无孔装备不显示；空孔计入分母，只有已镶嵌的孔计入分子。
	if st.SocketCount > 0 {
		lines = append(lines, qualityColoredLine(textQuality, fmt.Sprintf("孔数 %d/%d", usedSockets, st.SocketCount)))
	}
	s.appendSuitTooltip(&lines, e.Suit, owner)
	if e.Skill != nil {
		lines = append(lines, "法宝技能: "+e.Skill.Name)
		if text := strings.TrimSpace(e.Skill.Description); text != "" {
			lines = append(lines, "技能效果: "+text)
		}
	}
	if text := strings.TrimSpace(def.Description); text != "" {
		lines = append(lines, text)
	}
	if st.Bound {
		lines = append(lines, "（绑定物品不可交易/摆摊/邮寄, 可卖NPC）")
	}
	return strings.Join(lines, "\n")
}

func (s *Scene) fusedDescription(st domain.Stack) string {
	if st.FusedAppearance == 0 && st.FusedSoul == 0 && st.FusedDragon == 0 {
		return ""
	}
	var lines []string
	if st.FusedAppearance != 0 {
		def, ok := s.itemDef(st.FusedAppearance)
		if ok && def.AvatarFusion != nil {
			lines = append(lines, qualityColoredLine(4, "变装: "+def.Name))
			for _, affix := range def.AvatarFusion.Affixes {
				lines = append(lines, qualityColoredLine(4, affixText(affix)))
			}
			if text := strings.TrimSpace(def.Description); text != "" {
				lines = append(lines, text)
			}
		}
	}
	if st.FusedSoul != 0 {
		def, ok := s.itemDef(st.FusedSoul)
		if ok && def.EquipmentSoul != nil {
			lines = append(lines, qualityColoredLine(4, "变装: "+def.Name))
			for _, affix := range def.EquipmentSoul.Affixes {
				lines = append(lines, qualityColoredLine(4, affixText(affix)))
			}
		}
	}
	if st.FusedDragon != 0 {
		if dragon, ok := s.itemDef(st.FusedDragon); ok && dragon.DragonFusion != nil {
			lines = append(lines, qualityColoredLine(4, "龙系列: "+dragon.Name))
			for _, affix := range dragon.DragonFusion.Affixes {
				lines = append(lines, qualityColoredLine(4, affixText(affix)))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func equipmentKindName(e *domain.EquipDef) string {
	if e.Type != 0 {
		if name := map[int32]string{
			1: "长枪", 2: "长剑", 4: "双手剑", 8: "双刃", 16: "法杖", 32: "暗器",
		}[e.Type]; name != "" {
			return name
		}
	}
	if name := map[int32]string{
		1: "面饰", 2: "帽子", 3: "项链", 5: "盾牌", 6: "手套", 7: "戒指",
		8: "服装", 9: "鞋子", 10: "背包", 11: "法宝", 14: "耳环", 15: "手镯",
		16: "腰带", 17: "兽饰", 18: "兽饰", 19: "兽饰",
	}[e.Category]; name != "" {
		return name
	}
	return "装备"
}

func itemDisplayName(def domain.ItemDef, st domain.Stack) string {
	name := def.Name
	if def.Equip != nil && st.RefineLevel > 0 {
		name = fmt.Sprintf("%s+%d", name, st.RefineLevel)
	}
	if domain.EquipmentAttributeCount(def, st) == 2 {
		name = "\x05" + name
	}
	return name
}

func appendEquipmentRequirements(lines *[]string, e *domain.EquipDef, ch *domain.Character, actual domain.Base) {
	// 1.5.8 UITip.Layout 识别行首0x04为红色。逐条与 onEquip 相同的
	// 有效属性判定比较；不因某一项不满足而把其它已满足项一并染红。
	add := func(text string, unmet bool) {
		if ch != nil && unmet {
			text = "\x04" + text
		}
		*lines = append(*lines, text)
	}
	if e.Need.Level > 0 {
		add(fmt.Sprintf("需要等级: %d", e.Need.Level), ch != nil && ch.Level < e.Need.Level)
	}
	if e.Need.Sex == 1 {
		add("需要性别: 男", ch != nil && ch.Appear.Gender != 1)
	} else if e.Need.Sex == 2 {
		add("需要性别: 女", ch != nil && ch.Appear.Gender != 0)
	}
	b := e.Need.Base
	for _, row := range []struct {
		name          string
		value, actual int32
	}{
		{"力量", b.STR, actual.STR}, {"体质", b.VIT, actual.VIT}, {"敏捷", b.AGI, actual.AGI},
		{"智慧", b.INT, actual.INT}, {"精神", b.SPI, actual.SPI}, {"灵巧", b.DEX, actual.DEX},
	} {
		if row.value != 0 {
			add(fmt.Sprintf("需要%s: %d", row.name, row.value), row.actual < row.value)
		}
	}
	if e.Need.ProfessionRestricted && len(e.ProfessionNames) != 0 &&
		(e.Need.Professions != 0x1f || ch != nil && !e.Need.AllowsCharacter(ch)) {
		add("可装备职业:", ch != nil && !e.Need.AllowsCharacter(ch))
		for _, name := range e.ProfessionNames {
			*lines = append(*lines, "  "+name)
		}
	}
}

// appendIntrinsicEquipmentStats 展示固有属性与加工增量合并后的白字。
//
// 原服的主表属性显示为“命中 6 / 力量 8”，不带附加词条才有的
// 正号或百分号。这一层不得读 game_equip_extra。
func appendIntrinsicEquipmentStats(lines *[]string, stats domain.Stats, base domain.Base) {
	if stats.MinAtk != 0 || stats.MaxAtk != 0 {
		*lines = append(*lines, fmt.Sprintf("攻击 %d~%d", stats.MinAtk, stats.MaxAtk))
	}
	for _, row := range []struct {
		name  string
		value int32
	}{
		{"防御", stats.Def}, {"魔法攻击", stats.MAtk}, {"魔法防御", stats.MDef},
		{"命中", stats.Hit}, {"负重上限", stats.MaxWeight},
		{"力量", base.STR}, {"体质", base.VIT}, {"敏捷", base.AGI},
		{"智慧", base.INT}, {"精神", base.SPI}, {"灵巧", base.DEX},
	} {
		if row.value != 0 {
			*lines = append(*lines, fmt.Sprintf("%s %d", row.name, row.value))
		}
	}
}

// appendEquipmentAffixes 只展示子表 game_equip_extra 中的附加词条。
// 符号和百分比语义属于词条，不会泄漏到主表固有属性。
func appendEquipmentAffixes(lines *[]string, affixes []domain.Affix, quality uint8) {
	for _, affix := range affixes {
		if line, ok := equipmentAffixLine(affix); ok {
			*lines = append(*lines, qualityColoredLine(quality, line))
		}
	}
}

// tooltipLightBlue 仅用于文本，不是协议品质值。
const tooltipLightBlue uint8 = 5

// qualityColoredLine 使用客户端已有的品质前缀或淡蓝文本前缀。
func qualityColoredLine(quality uint8, line string) string {
	if quality == tooltipLightBlue && line != "" {
		return "\x05" + line
	}
	if quality == 0 || quality > 4 || line == "" {
		return line
	}
	return string([]byte{0x03, '0' + quality}) + line
}

func equipmentAffixLine(a domain.Affix) (string, bool) {
	name := map[int32]string{
		domain.AttrSTR: "力量", domain.AttrVIT: "体质", domain.AttrAGI: "敏捷",
		domain.AttrINT: "智慧", domain.AttrSPI: "精神", domain.AttrDEX: "灵巧",
		domain.AttrDef: "防御", domain.AttrMAtk: "魔法攻击", domain.AttrMDef: "魔法防御",
		domain.AttrHit: "命中", domain.AttrCrit: "爆击率", domain.AttrAtkSpeed: "攻击速度",
		domain.AttrMoveSpeed: "移动速度", domain.AttrMinAtk: "最小攻击",
		domain.AttrMaxAtk: "最大攻击", domain.AttrMaxHP: "最大生命",
		domain.AttrMaxMP: "最大法力", domain.AttrHPRegen: "生命恢复",
		domain.AttrMaxWeight: "负重上限", domain.AttrAtk: "攻击",
		domain.AttrMCrit: "魔法爆击率", domain.AttrPhysRes: "伤害抗性",
		domain.AttrMagicRes: "魔法抗性", domain.AttrStatusRes: "不良状态抗性",
	}[a.Attr]
	if name == "" {
		return "", false
	}
	if a.Attr == domain.AttrCrit || a.Attr == domain.AttrMCrit {
		return fmt.Sprintf("%s %s", name, basisPointPercent(a.Value)), true
	}
	suffix := ""
	if a.Mode == domain.ModePercent {
		suffix = "%"
	}
	return fmt.Sprintf("%s %s%d%s", name, signedPrefix(a.Value), a.Value, suffix), true
}

func signedPrefix(v int32) string {
	if v >= 0 {
		return "+"
	}
	return ""
}

// pushInventory emits one complete snapshot after an inventory transaction has
// reached a stable state, then recomputes the task book from the same authoritative
// bag. Item quest progress is derived from current holdings rather than stored
// separately, so every successful inventory mutation must close with both snapshots.
func (s *Scene) pushInventory(p *entity.Entity) error {
	snap, err := s.inventorySnapshot(p)
	if err != nil {
		var id domain.EntityID
		if p != nil {
			id = p.ID
		}
		s.log.Error("构建背包快照失败", "id", id, "err", err)
		return err
	}
	s.emitTo(p.ID, snap)
	s.pushQuestLog(p)
	// 生活技能的学习按钮取决于背包里是否持有对应 book_id。买到、领取、
	// 交易或取出技能书后必须立即刷新 0x8022，否则界面会一直停在不可学习。
	if s.skills != nil {
		s.emitTo(p.ID, s.lifeSkillSnapshot(p))
	}
	return nil
}

func inventoryMoney(m domain.Money) (int64, error) {
	gold, ok := checkedMul64(m.Gold, 1_000_000)
	if !ok {
		return 0, fmt.Errorf("gold conversion overflows int64")
	}
	silver, ok := checkedMul64(m.Silver, 1_000)
	if !ok {
		return 0, fmt.Errorf("silver conversion overflows int64")
	}
	total, ok := checkedAdd64(gold, silver)
	if !ok {
		return 0, fmt.Errorf("gold and silver sum overflows int64")
	}
	total, ok = checkedAdd64(total, m.Copper)
	if !ok {
		return 0, fmt.Errorf("money sum overflows int64")
	}
	return total, nil
}

func addInventoryWeight(total int64, unit, count int32) (int64, error) {
	part := int64(unit) * int64(count)
	next, ok := checkedAdd64(total, part)
	if !ok {
		return 0, fmt.Errorf("weight sum overflows int64")
	}
	return next, nil
}

func checkedMul64(v, factor int64) (int64, bool) {
	if factor == 0 || v == 0 {
		return 0, true
	}
	if v > math.MaxInt64/factor || v < math.MinInt64/factor {
		return 0, false
	}
	return v * factor, true
}

func checkedAdd64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}
