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

	// One immutable actor evaluation serves every tooltip and blocked flag.
	// Discard it before returning; never cache across inventory mutations.
	resolution := newEquipmentRequirementResolution()
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
			Tab: def.InventoryTab, Name: itemDisplayName(def, st), Desc: s.itemTooltipForViewerResolved(def, st, p, p, resolution),
			Sell: def.SellPrice, Slot: -1, Bound: st.Bound, Blocked: s.equipmentBlockedForResolved(p, def, st, resolution),
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
			Desc:              s.itemTooltipForViewerResolved(def, item.st, p, p, resolution),
			CurrentDurability: item.st.Durability, MaxDurability: domain.MaxDurabilityOf(item.st, def),
			Avatar: 0, SoulAvatar: domain.EquipmentSoulAvatar(item.st),
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
		def.InventoryTab != cmd.Tab || (st.Bound && !domain.IsStartingSword(st.Item)) || st.Locked || domain.AstralRuneTransferProtected(st) {
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
		def.InventoryTab != cmd.Tab || cmd.Count >= st.Count || (st.UID != 0 && !domain.IsAstralRuneStack(st)) || st.Locked {
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
	remain, split, valid := domain.SplitOwnedStack(st, cmd.Count)
	if !valid {
		return
	}
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
		if !ok || !def.Stackable || (st.UID != 0 && !domain.IsAstralRuneStack(st)) {
			out = append(out, st)
			continue
		}
		for st.Count > 0 {
			if len(out) > 0 {
				last := &out[len(out)-1]
				if domain.SameStackInstance(*last, st) && last.Count < domain.MaxStack {
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
// 客户端枚举为1绿/2深蓝/3紫/4黄/6淡蓝；5保留给暗金。
// 两条属性的名称统一发送品质6，详情继续使用已有0x05淡蓝文本码。
func equipmentQuality(def domain.ItemDef, stacks ...domain.Stack) uint8 {
	if def.Equip == nil {
		return 0
	}
	var st domain.Stack
	if len(stacks) != 0 {
		st = stacks[0]
	}
	if _, ok := domain.LegendaryPowerForRoll(st.Legendary.PowerID, st.Legendary.PowerRollBP); ok {
		return 5
	}
	if _, ok := domain.UniqueOf(st, def.EndgameCatalog); ok {
		return 4
	}
	n := domain.EquipmentAttributeCount(def, st)
	if n > 5 {
		n = 5
	}
	return [...]uint8{
		0, // 0条：白
		2, // 1条：深蓝
		6, // 2条：淡蓝
		3, // 3条：紫
		1, // 4条：绿
		4, // 5条及以上：黄
	}[n]
}

// equipmentBlockedFor 为 0x8006 item.blocked 提供角色当下的可穿戴状态。
// 这与 locked（玩家手动锁定实例）、bound（永久绑定）不是同一件事。
// 真正穿戴时 onEquip 会使用同一组规则再做一次权威校验，不信任客户端。
func (s *Scene) equipmentBlockedFor(p *entity.Entity, def domain.ItemDef, st domain.Stack) bool {
	return s.equipmentBlockedForResolved(p, def, st, newEquipmentRequirementResolution())
}

func (s *Scene) equipmentBlockedForResolved(p *entity.Entity, def domain.ItemDef, st domain.Stack, resolution *equipmentRequirementResolution) bool {
	if def.Equip == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return false
	}
	ch := p.Player.Char
	if ch.Level < domain.EquipmentRequiredLevel(def, st) || def.Equip.Need.Meet(ch.Level, s.equipmentRequirementBaseResolved(p, s.functionalWorn(p), domain.EquipSlot(def.Equip.Slot), resolution), ch.Appear.Gender) != domain.RejectNone {
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
	return s.itemTooltipForViewer(def, st, owner, owner)
}

// Viewer controls renderer negotiation; owner controls requirements/activation.
func (s *Scene) itemTooltipForViewer(def domain.ItemDef, st domain.Stack, owner, viewer *entity.Entity) string {
	return s.itemTooltipForViewerResolved(def, st, owner, viewer, newEquipmentRequirementResolution())
}

func (s *Scene) itemTooltipForViewerResolved(def domain.ItemDef, st domain.Stack, owner, viewer *entity.Entity, resolution *equipmentRequirementResolution) string {
	var ch *domain.Character
	if owner != nil && owner.Player != nil {
		ch = owner.Player.Char
	}
	if def.Equip == nil {
		lines := make([]string, 0, 2)
		if st.Spirit.Kind != 0 && st.Spirit.Validate() == nil {
			lines = append(lines, s.spiritInstanceTooltipLines(st.Item, st.Spirit)...)
			if st.Bound {
				lines = append(lines, "（绑定物品不可交易/摆摊/邮寄, 可卖NPC）")
			}
			return strings.Join(lines, "\n")
		}
		if rune, ok := domain.AstralRune(st.Item, s.endgameCatalog); ok {
			level := fmt.Sprintf("需要等级: %d", rune.MinLevel)
			if ch != nil && ch.Level < rune.MinLevel {
				level = "\x02" + level
			}
			lines = append(lines, level)
		}
		if def.Weight != 0 {
			lines = append(lines, fmt.Sprintf("重量 %d", def.Weight))
		}
		text := strings.TrimSpace(def.Description)
		if st.InstanceKind == domain.ItemInstanceSocketCard && len(st.Card.StaticAffixes()) > 0 {
			// 数值只取实例；原始描述仅保留部位限制，避免模板旧数值与实例并列。
			for _, affix := range st.Card.StaticAffixes() {
				lines = append(lines, s.astralRuneAffixText(st.Item, affix))
			}
			if at := strings.Index(text, "只可"); at >= 0 {
				lines = append(lines, text[at:])
			}
		} else if text != "" {
			lines = append(lines, text)
		}
		if st.Item == domain.PowerScrollItemID {
			if power, ok := domain.LegendaryPowerForRoll(st.Endgame.PowerScrollID, st.Endgame.PowerScrollRollBP); ok {
				lines = append(lines, "\x06"+power.Name+"："+power.Effect)
			}
		}
		return strings.Join(lines, "\n")
	}

	e := def.Equip
	quality := equipmentQuality(def, st)
	textQuality := quality
	_, unique := domain.UniqueOf(st, s.endgameCatalog)
	_, legendary := domain.LegendaryPowerForRoll(st.Legendary.PowerID, st.Legendary.PowerRollBP)
	_, formed := domain.MatchAstralRecipe(def, st)
	nativeStats := s.nativeEndgameStats(viewer) && (unique || formed)
	if nativeStats || legendary {
		textQuality = tooltipDarkGold
	} else if !unique && domain.EquipmentAttributeCount(def, st) == 2 {
		textQuality = tooltipLightBlue
	}
	var specialEffects []string
	lines := []string{"\x07(" + equipmentKindName(e) + ")", ""}
	if d, ok := domain.UniqueOf(st, s.endgameCatalog); ok {
		lines = append(lines, qualityColoredLine(textQuality, "仙遗·"+d.Name), "基底："+def.Name)
		if !nativeStats {
			for _, a := range domain.UniqueAffixes(st, s.endgameCatalog) {
				lines = append(lines, qualityColoredLine(4, affixText(a)))
			}
			for _, line := range localEnhancementLines(def, d.LocalEnhancements, st.Endgame.UniqueRollBP) {
				lines = append(lines, qualityColoredLine(4, line))
			}
		}
		for _, tr := range d.Traits {
			specialEffects = append(specialEffects, tr.Describe(10000))
		}
		lines = append(lines, d.Description(), "")
	}
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
	lines = append(lines, s.spiritAppearanceTooltipLines(st)...)
	if !st.FusedSpirit.Empty() {
		if spirit, ok := domain.SpiritDefinitionForKind(st.FusedSpirit.State.Kind); ok {
			lines = append(lines, qualityColoredLine(4, "变装: "+spirit.Name))
			lines = append(lines, s.spiritInstanceTooltipLines(st.FusedSpirit.Item, st.FusedSpirit.State)...)
		}
	} else if st.FusedSoul != 0 {
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
			actual = s.equipmentRequirementBaseResolved(owner, s.functionalWorn(owner), domain.EquipSlot(e.Slot), resolution)
		}
	}
	requirements := *e
	requirements.Need.Level = domain.EquipmentRequiredLevel(def, st)
	appendEquipmentRequirements(&lines, &requirements, ch, actual)
	lines = append(lines, "")
	var local []domain.ItemLocalEnhancement
	roll := st.Endgame.UniqueRollBP
	if d, ok := domain.UniqueOf(st, s.endgameCatalog); ok {
		local = d.LocalEnhancements
	}
	formation, includeFormation := s.tooltipFormation(def, st, owner)
	if nativeStats && includeFormation {
		local = append(local, formation.LocalEnhancements...)
		roll = st.Endgame.FormationRollBP
	}
	equipBase, equipStats := domain.EnhancedIntrinsic(e, st.RefineLevel, local, roll)
	_, refinedStats := e.RefinedIntrinsic(st.RefineLevel)
	if legendary {
		equipBase, equipStats = domain.LegendaryIntrinsic(def, st)
		// The tier multiplier is intrinsic white text, not a random affix.
		refinedStats = equipStats
	}
	appendIntrinsicEquipmentStats(&lines, equipStats, equipBase, refinedStats, nativeStats || legendary)
	if def.Weight != 0 {
		lines = append(lines, fmt.Sprintf("重量 %d", def.Weight))
	}
	lines = append(lines, "")
	if st.Legendary.PowerID == "" {
		appendEquipmentAffixes(&lines, e.Affixes, textQuality)
	}
	if power, ok := domain.LegendaryPowerForRoll(st.Legendary.PowerID, st.Legendary.PowerRollBP); ok {
		for i := range st.Legendary.Affixes {
			a, _ := st.Legendary.EffectiveAffix(i)
			lines = append(lines, qualityColoredLine(tooltipDarkGold, affixText(a.Affix)))
		}
		specialEffects = append(specialEffects, power.Name+"："+power.Effect)
	}
	if e.Slot == int32(domain.SlotBag) && st.WingPowers.Validate() == nil {
		for _, v := range st.WingPowers {
			if power, ok := domain.LegendaryPowerForRoll(v.PowerID, v.PowerRollBP); ok {
				specialEffects = append(specialEffects, power.Name+"："+power.Effect)
			}
		}
	}
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
	// The native-stat renderer owns one authoritative extra-attribute block.
	// Rune snapshots and active formation extras merge only on identical scope.
	usedSockets := domain.UsedEquipmentSockets(st)
	if nativeStats {
		var socketAffixes []domain.Affix
		periodic := make(domain.PeriodicRecoveryBuckets)
		for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
			if st.Sockets[i] != 0 {
				rune, known := s.endgameCatalog.Rune(st.Sockets[i])
				for _, a := range st.SocketCards[i].StaticAffixes() {
					if known && periodicRecoveryAffix(rune, a) {
						periodic.AddFlatHP(rune.RegenerationIntervalMS, int64(a.Value))
					} else {
						socketAffixes = append(socketAffixes, a)
					}
				}
			}
		}
		var formationAffixes []domain.Affix
		if includeFormation {
			formationAffixes = domain.ResolveVariableAffixes(formation.Affixes, formation.VariableAffixes, st.Endgame.FormationRollBP)
		}
		for _, a := range mergedEndgameAffixes(domain.UniqueAffixes(st, s.endgameCatalog), socketAffixes, formationAffixes) {
			lines = append(lines, qualityColoredLine(tooltipDarkGold, affixText(a)))
		}
		// Different execution scopes never merge just because they share attr38.
		intervals := make([]int, 0, len(periodic))
		for interval := range periodic {
			intervals = append(intervals, int(interval))
		}
		sort.Ints(intervals)
		for _, interval := range intervals {
			lines = append(lines, qualityColoredLine(tooltipDarkGold, periodicRecoveryText(int32(interval), periodic[int32(interval)])))
		}
		appendV5 := func(effects []domain.EquipmentEffect, bonuses []domain.SkillRankBonus) {
			attributes, special := s.equipmentV5Lines(effects, bonuses)
			for _, text := range attributes {
				lines = append(lines, qualityColoredLine(tooltipDarkGold, text))
			}
			specialEffects = append(specialEffects, special...)
		}
		if u, ok := domain.UniqueOf(st, s.endgameCatalog); ok {
			appendV5(u.TypedEffects, u.SkillBonuses)
		}
		if includeFormation {
			appendV5(formation.TypedEffects, formation.SkillBonuses)
			for _, tr := range formation.Traits {
				specialEffects = append(specialEffects, tr.Describe(10000))
			}
		}
		for _, line := range localEnhancementLines(def, local, roll) {
			lines = append(lines, qualityColoredLine(tooltipDarkGold, line))
		}
	} else {
		for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
			if st.Sockets[i] != 0 {
				for _, a := range st.SocketCards[i].StaticAffixes() {
					lines = append(lines, qualityColoredLine(textQuality, s.astralRuneAffixText(st.Sockets[i], a)))
				}
			}
		}
	}
	// Ordinary named weapons do not use the client's legendary overlay. Emit
	// their actual skill effects at the same visible attribute-block boundary,
	// before sockets, sets and lore, using its existing orange marker.
	for _, text := range domain.DungeonWeaponEffectLines(def.ID) {
		lines = append(lines, "\x06"+text)
	}
	// 分母是这件装备实际已打出的孔，不是模板可打孔上限。
	// 无孔装备不显示；空孔计入分母，只有已镶嵌的孔计入分子。
	if st.SocketCount > 0 {
		socketQuality := textQuality
		if nativeStats {
			socketQuality = tooltipDarkGold
		}
		lines = append(lines, qualityColoredLine(socketQuality, fmt.Sprintf("孔数 %d/%d", usedSockets, st.SocketCount)))
	}
	s.appendSuitTooltip(&lines, e.Suit, owner, textQuality)
	if e.Skill != nil {
		lines = append(lines, "法宝技能: "+e.Skill.Name)
		if text := strings.TrimSpace(e.Skill.Description); text != "" {
			lines = append(lines, "技能效果: "+text)
		}
	}
	// Unique and formed astral equipment use their authored lore. Omit only
	// the base template prose here, before it becomes an unstructured tooltip;
	// requirements, set/skill effects and binding notices remain untouched.
	// Recipe identity, not current activation, also covers bag and broken gear.
	if _, formed := domain.MatchAstralRecipe(def, st); !unique && !formed {
		if text := strings.TrimSpace(def.Description); text != "" {
			lines = append(lines, text)
		}
	}
	if st.Bound {
		lines = append(lines, "（绑定物品不可交易/摆摊/邮寄, 可卖NPC）")
	}
	// Special effects are the final block, after native metadata and lore.
	// 0x06 is the existing orange text marker, independent of item quality.
	for _, text := range specialEffects {
		lines = append(lines, "\x06"+text)
	}
	return strings.Join(lines, "\n")
}

func (s *Scene) fusedDescription(st domain.Stack) string {
	if st.FusedAppearance == 0 && st.FusedSoul == 0 && st.FusedDragon == 0 && st.FusedSpirit.Empty() && st.SpiritAppearance == 0 {
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
	lines = append(lines, s.spiritAppearanceTooltipLines(st)...)
	if !st.FusedSpirit.Empty() {
		if spirit, ok := domain.SpiritDefinitionForKind(st.FusedSpirit.State.Kind); ok {
			lines = append(lines, qualityColoredLine(4, "变装: "+spirit.Name))
			lines = append(lines, s.spiritInstanceTooltipLines(st.FusedSpirit.Item, st.FusedSpirit.State)...)
		}
	} else if st.FusedSoul != 0 {
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
	if power, ok := domain.LegendaryPowerForRoll(st.Legendary.PowerID, st.Legendary.PowerRollBP); ok {
		name = domain.LegendaryInstanceDisplayName(def.Name, power, st.Legendary)
	}
	uniqueName, unique := domain.UniqueName(st, def.EndgameCatalog)
	if unique {
		name = uniqueName
	}
	if def.Equip != nil && st.RefineLevel > 0 {
		name = fmt.Sprintf("%s+%d", name, st.RefineLevel)
	}
	if !unique && domain.EquipmentAttributeCount(def, st) == 2 {
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

// appendIntrinsicEquipmentStats 展示固有属性与加工增量合并后的基础数值。
//
// 原服的主表属性显示为“命中 6 / 力量 8”，不带附加词条才有的
// 正号或百分号。这一层不得读 game_equip_extra。只将相对同级精炼数值
// 确有本件强化增量的对应行标为暗金；零基数、零百分比及取整为零仍白色。
func appendIntrinsicEquipmentStats(lines *[]string, stats domain.Stats, base domain.Base, refined domain.Stats, explicitWhite bool) {
	add := func(line string, enhanced bool) {
		if enhanced {
			line = qualityColoredLine(tooltipDarkGold, line)
		} else if explicitWhite {
			line = "\x030" + line
		}
		*lines = append(*lines, line)
	}
	if stats.MinAtk != 0 || stats.MaxAtk != 0 {
		add(fmt.Sprintf("攻击 %d~%d", stats.MinAtk, stats.MaxAtk), stats.MinAtk != refined.MinAtk || stats.MaxAtk != refined.MaxAtk)
	}
	for _, row := range []struct {
		name     string
		value    int32
		enhanced bool
	}{
		{"防御", stats.Def, stats.Def != refined.Def}, {"魔法攻击", stats.MAtk, stats.MAtk != refined.MAtk}, {"魔法防御", stats.MDef, stats.MDef != refined.MDef},
		{"命中", stats.Hit, false}, {"负重上限", stats.MaxWeight, false},
		{"力量", base.STR, false}, {"体质", base.VIT, false}, {"敏捷", base.AGI, false},
		{"智慧", base.INT, false}, {"精神", base.SPI, false}, {"灵巧", base.DEX, false},
	} {
		if row.value != 0 {
			add(fmt.Sprintf("%s %d", row.name, row.value), row.enhanced)
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

// Text-only colors are not equipment-quality protocol values. In particular,
// retain the existing light-blue value; dark gold emits quality marker 5.
// Its private sentinel must not collide with the light-blue protocol quality 6.
const (
	tooltipLightBlue uint8 = 5
	tooltipDarkGold  uint8 = 7
)

// qualityColoredLine 使用客户端已有的品质前缀或淡蓝文本前缀。
func qualityColoredLine(quality uint8, line string) string {
	if quality == tooltipDarkGold && line != "" {
		return "\x035" + line
	}
	if quality == tooltipLightBlue && line != "" {
		return "\x05" + line
	}
	if quality == 0 || quality > 6 || line == "" {
		return line
	}
	return string([]byte{0x03, '0' + quality}) + line
}

func equipmentAffixLine(a domain.Affix) (string, bool) {
	if a.Attr == domain.AttrAtkSpeed {
		return attackSpeedAffixText(a), true
	}
	name := map[int32]string{
		domain.AttrSTR: "力量", domain.AttrVIT: "体质", domain.AttrAGI: "敏捷",
		domain.AttrINT: "智慧", domain.AttrSPI: "精神", domain.AttrDEX: "灵巧",
		domain.AttrDef: "防御", domain.AttrMAtk: "魔法攻击", domain.AttrMDef: "魔法防御",
		domain.AttrHit: "命中", domain.AttrCrit: "爆击率",
		domain.AttrMoveSpeed: "移动速度", domain.AttrMinAtk: "最小攻击",
		domain.AttrMaxAtk: "最大攻击", domain.AttrMaxHP: "最大生命",
		domain.AttrMaxMP: "最大法力", domain.AttrHPRegen: "生命恢复",
		domain.AttrMaxWeight: "负重上限", domain.AttrAtk: "攻击",
		domain.AttrMCrit: "魔法爆击率", domain.AttrPhysRes: "伤害抗性",
		domain.AttrMagicRes: "魔法抗性", domain.AttrStatusRes: "不良状态抗性",
		domain.AttrEffectHitBP: "效果命中",
	}[a.Attr]
	if name == "" {
		return "", false
	}
	if a.Attr == domain.AttrCrit || a.Attr == domain.AttrMCrit || a.Attr == domain.AttrEffectHitBP {
		return fmt.Sprintf("%s %s", name, basisPointPercent(a.Value)), true
	}
	suffix := ""
	if a.Mode == domain.ModePercent {
		suffix = "%"
	}
	value := int64(a.Value)
	prefix := ""
	if value >= 0 {
		prefix = "+"
	}
	return fmt.Sprintf("%s %s%d%s", name, prefix, value, suffix), true
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
	s.sendEndgameSnapshot(p)
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

func spiritStateTooltipLines(state domain.SpiritState) []string {
	if state.Validate() != nil {
		return nil
	}
	if state.AttrCount == 0 {
		return []string{"尚未解封"}
	}
	lines := []string{fmt.Sprintf("加工等级: %d", state.Level())}
	levels, perfect := state.Levels(), state.PerfectLevels()
	for i, a := range state.Affixes() {
		lines = append(lines, qualityColoredLine(4, affixText(a))+
			fmt.Sprintf("（加工 %d，完美 %d）", levels[i], perfect[i]))
	}
	return lines
}

func (s *Scene) spiritAppearanceTooltipLines(st domain.Stack) []string {
	appearance, ok := domain.SpiritAppearanceForItem(st.SpiritAppearance)
	if !ok {
		return nil
	}
	name := ""
	if def, ok := s.itemDef(appearance.BonusItemID); ok {
		name = def.Name
	}
	if name == "" {
		if def, ok := s.itemDef(appearance.ItemID); ok {
			name = def.Name
		}
	}
	lines := []string{qualityColoredLine(4, "变装: "+name)}
	for _, a := range appearance.Extra {
		lines = append(lines, qualityColoredLine(4, affixText(a)))
	}
	return lines
}

func (s *Scene) spiritInstanceTooltipLines(item domain.ItemID, state domain.SpiritState) []string {
	if !state.LegacyFixed {
		return spiritStateTooltipLines(state)
	}
	if state.Validate() != nil {
		return nil
	}
	var lines []string
	if def, ok := s.itemDef(item); ok && def.EquipmentSoul != nil {
		for _, a := range def.EquipmentSoul.Affixes {
			lines = append(lines, qualityColoredLine(4, affixText(a)))
		}
	}
	return lines
}
