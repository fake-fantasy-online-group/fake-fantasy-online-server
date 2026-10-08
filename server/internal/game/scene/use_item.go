package scene

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// onUseItem 执行普通自用物品的确定性状态效果。
//
// 客户端 0x1015 同时带 itemId、tab、slot；三项都与服务端背包核对，不能只凭
// itemId 从任意格扣一件。当前只接 ov_item_entry 已证明的 mode=4 状态操作，
// 其他效果脚本仍拒绝，避免“扣了东西却没效果”。
func (s *Scene) onUseItem(cmd UseItem) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil || !p.Alive() {
		return
	}
	if p.Player.Stall != nil {
		// 古币本身只负责在客户端打开开店界面；店铺已经运行时再次使用，
		// 直接补发自己的 0x803c，让关闭过的营业窗口可以重新出现。
		if _, isOpenCoin := s.stallOpenItems.TypeFor(cmd.Item); isOpenCoin {
			s.pushStallContents(p)
			return
		}
		s.stallNotice(p, "摆摊中不能使用物品，请先结束摊位")
		return
	}
	reject := func(reason event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "UseItem", Reason: reason})
	}
	bagSlot := cmd.BagSlot
	shortcut := cmd.Tab == 0xff && cmd.BagSlot == -1
	if shortcut {
		// GameHud.UseHotSlot 只有物品号，没有背包格上下文，因此客户端按协议
		// 发送 tab=0xff、slot=-1。按背包顺序取第一格同类物品，结果稳定，且
		// 仍以服务端背包为准，不能凭客户端物品号凭空使用。
		bagSlot = -1
		p.Player.Bag.Each(func(slot int, stack domain.Stack) {
			if bagSlot == -1 && stack.Item == cmd.Item {
				bagSlot = slot
			}
		})
	}
	stack := p.Player.Bag.At(bagSlot)
	if stack.Empty() || stack.Item != cmd.Item {
		reject(event.RejectNoItem)
		return
	}
	if stack.Locked {
		reject(event.RejectInvalid)
		return
	}
	def, ok := s.itemDef(stack.Item)
	if !ok || def.Equip != nil || !def.InventoryTabKnown ||
		(!shortcut && def.InventoryTab != cmd.Tab) {
		reject(event.RejectInvalid)
		return
	}
	if def.PetCarrierSpecies > 0 {
		s.usePetCarrier(p, stack)
		return
	}
	if p.Player.RideAnchor != 0 && !def.UseRestore.Valid() {
		s.rejectRidePassenger(p.ID)
		return
	}
	if def.HornTier > 0 {
		s.emitTo(p.ID, event.HornDialogOpened{Who: p.ID, Item: def.ID, Tier: def.HornTier, Skin: def.HornSkin})
		return
	}
	if stack.Item == s.wardrobeRule.KeyItem {
		tab := cmd.Tab
		if shortcut {
			tab = def.InventoryTab
		}
		s.useWardrobeKey(p, tab, int32(bagSlot))
		return
	}
	if _, isOpenCoin := s.stallOpenItems.TypeFor(stack.Item); isOpenCoin {
		// 0x1015 是“打开配置窗口”，不是开店成功。古币必须等后续
		// 0x1060 通过全部状态校验后，和摊位快照在同一事务中消耗。
		return
	}
	if p.Player.Char.Level < def.UseLevel {
		reject(event.RejectLevelTooLow)
		return
	}
	if def.Saddle != nil {
		s.useSaddle(p, def, reject)
		return
	}
	if def.CooldownSec > 0 && !p.Player.ItemReadyAt(def.CooldownGroup, s.tick) {
		reject(event.RejectOnCooldown)
		return
	}
	startCooldown := func() {
		p.Player.StartItemCooldown(def.CooldownGroup, s.tick,
			domain.Ticks(int(def.CooldownSec)*1000))
	}
	if def.StatReset != domain.StatResetNone {
		s.useCharacterStatResetItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UseCaiyu > 0 {
		s.useCaiyuItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UseCopper > 0 {
		s.useMoneyItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if len(def.UseRewards) > 0 {
		s.useFixedRewardItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.GachaBox != nil {
		s.useGachaBoxItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UseTitle != "" {
		s.useTitleUnlockItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.ExperienceBoost.Valid() {
		s.useExperienceBoostItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if len(def.PlayerExperience) > 0 {
		s.usePlayerExperienceItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if learning, ok := s.petLearnItems[stack.Item]; ok {
		s.usePetLearningItem(p, bagSlot, learning, reject)
		return
	}
	if food, ok := s.petFoods[stack.Item]; ok {
		s.usePetFood(p, bagSlot, food, reject)
		return
	}
	if def.UsePetRestore.Valid() {
		s.usePetResourceRestoreItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if len(def.PetRewards) > 0 {
		s.usePetRewardItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.GrantPetID != 0 {
		s.usePetEgg(p, bagSlot, def, reject)
		return
	}
	if def.PetExperience != nil {
		s.usePetExperienceItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.PetAffectionPP != nil {
		s.usePetPPItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UsePetReset != domain.PetResetNone {
		s.usePetResetItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.PetTransmog != nil {
		s.usePetTransmogItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UseSkillBook != 0 {
		s.useSkillBook(p, bagSlot, def, reject)
		return
	}
	if def.UseReturn {
		s.useReturnItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UseRestore.Valid() {
		s.useResourceRestoreItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if def.UseClearStatuses || len(def.UseRemoveStatuses) > 0 {
		s.useStatusRemovalItem(p, bagSlot, def, startCooldown, reject)
		return
	}
	if len(def.UseStatuses) == 0 || s.statuses == nil {
		if def.UseDisabledReason != "" {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: def.UseDisabledReason})
			s.log.Debug("拒绝未开放物品效果", "char", p.Name, "item", def.ID,
				"reason", def.UseDisabledReason)
			return
		}
		reject(event.RejectInvalid)
		return
	}

	// 先把所有状态定义取齐，再产生任何领域变化；静态数据若缺一行，不留下
	// “前一个状态已加、物品还没扣”的半截结果。
	statusDefs := make([]domain.StatusDef, 0, len(def.UseStatuses))
	for _, effect := range def.UseStatuses {
		statusDef, ok := s.statuses.Get(effect.ID, effect.Level)
		if !ok {
			reject(event.RejectInvalid)
			return
		}
		if effect.DurationSec > 0 {
			statusDef.DurationSec = effect.DurationSec
		}
		applyStatusSource(&statusDef, def.Icon, 0)
		statusDefs = append(statusDefs, statusDef)
	}

	applied := false
	for _, statusDef := range statusDefs {
		if s.applyStatusDef(p, statusDef, p.ID) {
			applied = true
		}
	}
	if !applied {
		reject(event.RejectInvalid)
		return
	}
	// Bag.At 已在本场景 actor 内验证过，期间没有并发修改；这里不会半扣。
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		s.log.Error("物品状态已施加但背包扣除失败", "char", p.Player.Char.Name,
			"item", cmd.Item, "slot", bagSlot)
		return
	}
	p.Player.MarkDirty()
	startCooldown()
	s.pushInventory(p)
	s.requestNewbieTip(p.ID, 5)
	s.log.Debug("使用状态物品", "char", p.Player.Char.Name, "item", def.Name,
		"statusCount", len(statusDefs))
}

func (s *Scene) useMoneyItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	total, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || item.UseCopper <= 0 || total > math.MaxInt64-item.UseCopper {
		reject(event.RejectInvalid)
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	p.Player.Char.Money = moneyFromInventory(total + item.UseCopper)
	p.Player.MarkDirty()
	startCooldown()
	s.pushInventory(p)
	gain := moneyFromInventory(item.UseCopper)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: fmt.Sprintf("获得%d金%d银%d铜", gain.Gold, gain.Silver, gain.Copper)})
	s.log.Info("使用金钱兑换物品", "char", p.Name, "item", item.Name, "copper", item.UseCopper)
}

func (s *Scene) useCaiyuItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if item.UseCaiyu <= 0 || p.Player.Char.Caiyu > math.MaxInt64-item.UseCaiyu {
		reject(event.RejectInvalid)
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	p.Player.Char.Caiyu += item.UseCaiyu
	p.Player.MarkDirty()
	startCooldown()
	s.pushInventory(p)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: fmt.Sprintf("获得%d彩玉", item.UseCaiyu)})
	s.log.Info("使用彩玉兑换物品", "char", p.Name, "item", item.Name,
		"caiyu", item.UseCaiyu, "balance", p.Player.Char.Caiyu)
}

func (s *Scene) usePetTransmogItem(owner *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if owner.Player.Pet == 0 {
		reject(event.RejectNoPet)
		return
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Alive() {
		reject(event.RejectNoPet)
		return
	}
	def := item.PetTransmog
	if def == nil || !def.Valid() {
		reject(event.RejectInvalid)
		return
	}
	if def.RequireUnmounted && owner.Player.Riding {
		s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: "骑乘状态不能使用金甲或银甲幻化书"})
		return
	}
	if !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	inst := pet.Pet.Inst
	inst.TransmogModel = def.Model
	inst.TransmogUntil = s.now().Add(time.Duration(def.DurationSec) * time.Second)
	pet.Look.ModelID = def.Model
	owner.Player.MarkDirty()
	startCooldown()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	if owner.Player.Riding {
		owner.Player.MountModel = s.ridingModel(owner, inst, pet.Pet.Def)
		s.emit(s.ridingEvent(owner))
	} else {
		s.emit(event.EntityDespawned{ID: pet.ID, Kind: pet.Kind, Reason: event.DespawnRemoved})
		s.emit(s.spawnEvent(pet))
	}
	s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID,
		Text: fmt.Sprintf("%s已生效：%d分钟内幻化为%s", item.Name, def.DurationSec/60, def.TargetName)})
	s.log.Info("使用宠物幻化书", "char", owner.Name, "item", item.Name,
		"pet", inst.ID, "model", def.Model, "seconds", def.DurationSec)
}

func (s *Scene) useExperienceBoostItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	boost := item.ExperienceBoost
	playerPct, petPct := boost.PlayerBonusPct, boost.PetBonusPct
	if p.Player.ExperienceBonusPct(s.tick) > playerPct {
		playerPct = 0
	}
	if p.Player.PetExperienceBonusPct(s.tick) > petPct {
		petPct = 0
	}
	if playerPct == 0 && petPct == 0 {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "当前已有更强的经验增益"})
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	duration := domain.Ticks(int(boost.DurationSec) * 1000)
	p.Player.StartExperienceBoost(playerPct, petPct, s.tick, duration)
	p.Player.MarkDirty()
	startCooldown()
	s.pushInventory(p)
	s.emitTo(p.ID, s.buffSnapshot(p))
	parts := make([]string, 0, 2)
	if playerPct > 0 {
		parts = append(parts, fmt.Sprintf("人物经验×%g", float64(playerPct+100)/100))
	}
	if petPct > 0 {
		parts = append(parts, fmt.Sprintf("宠物经验×%g", float64(petPct+100)/100))
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: fmt.Sprintf("%s已生效%d分钟", strings.Join(parts, "、"), boost.DurationSec/60)})
	s.log.Info("使用经验增益物品", "char", p.Name, "item", item.Name,
		"playerPct", playerPct, "petPct", petPct, "seconds", boost.DurationSec)
}

func (s *Scene) useTitleUnlockItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	ch := p.Player.Char
	if ch.OwnsTitle(item.UseTitle) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "该称号已经解锁"})
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	if !ch.UnlockTitle(item.UseTitle) {
		reject(event.RejectInvalid)
		return
	}
	p.Player.MarkDirty()
	startCooldown()
	if ch.Appear.Title == "" {
		ch.Appear.Title = item.UseTitle
		p.Look.Appearance.Title = item.UseTitle
		s.emit(event.AppearanceChanged{Who: p.ID, Appearance: p.Look.Appearance})
	}
	s.pushInventory(p)
	s.pushTitleSnapshot(p)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: fmt.Sprintf("获得称号「%s」", item.UseTitle)})
	s.log.Info("使用称号石", "char", ch.Name, "item", item.Name, "title", item.UseTitle)
}

func (s *Scene) useFixedRewardItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	bag := p.Player.Bag.Clone()
	if bag == nil || !bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	names := make([]string, 0, len(item.UseRewards))
	for _, reward := range item.UseRewards {
		def, ok := s.itemDef(reward.Item)
		if !ok || reward.Qty <= 0 {
			reject(event.RejectInvalid)
			return
		}
		if bag.Add(def, reward.Qty) != 0 {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "背包空间不足，无法打开礼包"})
			return
		}
		names = append(names, fmt.Sprintf("%s×%d", def.Name, reward.Qty))
	}
	p.Player.Bag = bag
	p.Player.MarkDirty()
	startCooldown()
	s.pushInventory(p)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: "打开成功，获得：" + strings.Join(names, "、")})
	s.log.Info("打开固定礼包", "char", p.Name, "item", item.Name, "rewards", len(item.UseRewards))
}

// useGachaBoxItem 打开概率礼包：先做背包前置门槛检查，再在克隆背包上扣礼包
// 本体与钥匙，用场景随机流逐条独立掷概率、在数量区间内随机，全部奖励预演
// 放入成功后才替换权威背包。背包放不下或钥匙不足时整体不生效，与固定礼包
// 同一原子口径。
func (s *Scene) useGachaBoxItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	box := item.GachaBox
	if box == nil {
		reject(event.RejectInvalid)
		return
	}
	bag := p.Player.Bag.Clone()
	if bag == nil {
		reject(event.RejectInvalid)
		return
	}
	if box.MinFreeSlots > 0 && bag.FreeSlots() < int(box.MinFreeSlots) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
			Text: fmt.Sprintf("背包剩余空间不足%d格，无法打开礼包", box.MinFreeSlots)})
		return
	}
	if !bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	if box.KeyItem != 0 {
		if bag.UsableCountOf(box.KeyItem) < box.KeyQuantity {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "钥匙不足，无法打开礼包"})
			return
		}
		if !bag.Remove(box.KeyItem, box.KeyQuantity) {
			reject(event.RejectNoItem)
			return
		}
	}
	gained := make([]domain.RewardItem, 0, len(box.Rewards))
	for _, reward := range box.Rewards {
		if s.rng.Intn(10000) >= int(reward.ChanceBP) {
			continue
		}
		def, ok := s.itemDef(reward.Item)
		if !ok || reward.MinQty <= 0 || reward.MaxQty < reward.MinQty {
			reject(event.RejectInvalid)
			return
		}
		qty := reward.MinQty
		if span := reward.MaxQty - reward.MinQty; span > 0 {
			qty += int32(s.rng.Intn(int(span) + 1))
		}
		if bag.Add(def, qty) != 0 {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "背包空间不足，无法打开礼包"})
			return
		}
		gained = append(gained, domain.RewardItem{Item: reward.Item, Qty: qty})
	}
	p.Player.Bag = bag
	p.Player.MarkDirty()
	startCooldown()
	s.pushInventory(p)
	if len(gained) == 0 {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "打开礼包，什么也没有获得"})
	} else {
		names := make([]string, 0, len(gained))
		for _, g := range gained {
			if def, ok := s.itemDef(g.Item); ok {
				names = append(names, fmt.Sprintf("%s×%d", def.Name, g.Qty))
			}
		}
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
			Text: "打开成功，获得：" + strings.Join(names, "、")})
	}
	s.log.Info("打开概率礼包", "char", p.Name, "item", item.Name,
		"rewards", len(gained), "key", box.KeyItem, "slots", box.MinFreeSlots)
}

func (s *Scene) useCharacterStatResetItem(p *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	ch := p.Player.Char
	natural := domain.NaturalBaseAtLevel(ch.Race, ch.Level)
	current := ch.EffectiveBase()
	returned := int32(0)
	if item.StatReset == domain.StatResetAll {
		if p.Player.Worn != nil && p.Player.Worn.Count() > 0 {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "使用重生水晶前请脱下全部装备"})
			return
		}
		for _, delta := range [...]int32{
			current.STR - natural.STR, current.VIT - natural.VIT,
			current.INT - natural.INT, current.SPI - natural.SPI,
			current.AGI - natural.AGI, current.DEX - natural.DEX,
		} {
			if delta > 0 {
				returned += delta
			}
		}
		if returned <= 0 {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "当前没有已分配的属性点"})
			return
		}
	} else {
		var value, floor int32
		switch item.StatReset {
		case domain.StatResetSTR:
			value, floor = current.STR, natural.STR
		case domain.StatResetVIT:
			value, floor = current.VIT, natural.VIT
		case domain.StatResetINT:
			value, floor = current.INT, natural.INT
		case domain.StatResetSPI:
			value, floor = current.SPI, natural.SPI
		case domain.StatResetAGI:
			value, floor = current.AGI, natural.AGI
		case domain.StatResetDEX:
			value, floor = current.DEX, natural.DEX
		default:
			reject(event.RejectInvalid)
			return
		}
		if value-floor < 3 {
			s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "该属性没有3点可退回的自由属性"})
			return
		}
		returned = 3
	}
	if ch.FreePoints > int32(^uint32(0)>>1)-returned {
		reject(event.RejectInvalid)
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	ch.Base = current
	if item.StatReset == domain.StatResetAll {
		ch.Base = natural
	} else {
		switch item.StatReset {
		case domain.StatResetSTR:
			ch.Base.STR -= returned
		case domain.StatResetVIT:
			ch.Base.VIT -= returned
		case domain.StatResetINT:
			ch.Base.INT -= returned
		case domain.StatResetSPI:
			ch.Base.SPI -= returned
		case domain.StatResetAGI:
			ch.Base.AGI -= returned
		case domain.StatResetDEX:
			ch.Base.DEX -= returned
		}
	}
	ch.FreePoints += returned
	p.Player.MarkDirty()
	startCooldown()
	s.refreshStats(p)
	s.pushInventory(p)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: fmt.Sprintf("已返还%d点自由属性", returned)})
	s.log.Info("使用人物洗点物品", "char", ch.Name, "item", item.Name, "returned", returned)
}

func (s *Scene) usePetResetItem(owner *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if (item.UsePetReset != domain.PetResetKeepInnate && item.UsePetReset != domain.PetResetRehatch) ||
		s.petSkills == nil {
		reject(event.RejectInvalid)
		return
	}
	inst, petDef, ok := s.activePetInstance(owner)
	if !ok {
		reject(event.RejectNoPet)
		return
	}
	if !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	identity := struct {
		ID      domain.PetInstID
		Slot    int32
		Name    string
		ItemUID int64
	}{inst.ID, inst.Slot, inst.Name, inst.ItemUID}
	prefix, trust, starve := inst.Prefix, inst.Trust, inst.Starve
	s.recallPet(owner, event.PetRecalledByPlayer)
	fresh := domain.NewPetInstance(petDef)
	fresh.ID, fresh.Slot, fresh.Name = identity.ID, identity.Slot, identity.Name
	fresh.ItemUID = identity.ItemUID
	// 单只宠物的PP终身上限不因洗髓重新开放；已投入的PP点归还为自由点。
	fresh.PPAiUsed, fresh.FreePoints = inst.PPAiUsed, inst.PPAiUsed
	if item.UsePetReset == domain.PetResetKeepInnate {
		fresh.Hatched = true
		fresh.Gender = inst.Gender
		fresh.Prefix = prefix
		fresh.Trust, fresh.Starve = trust, starve
		s.assignInitialPetSkills(&fresh, petDef)
	} else {
		fresh.Hatched = false
		fresh.Prefix = domain.NoPetPrefix
	}
	*inst = fresh
	owner.Player.MarkDirty()
	startCooldown()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID,
		Text: fmt.Sprintf("%s已重置为1级", petDef.Name)})
	s.log.Info("使用宠物洗髓物品", "char", owner.Name, "item", item.Name,
		"pet", inst.ID, "rehatch", item.UsePetReset == domain.PetResetRehatch)
}

func (s *Scene) usePetResourceRestoreItem(owner *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if owner.Player.Pet == 0 {
		reject(event.RejectNoPet)
		return
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		reject(event.RejectNoPet)
		return
	}
	if !pet.Alive() {
		reject(event.RejectPetDead)
		return
	}
	hpAmount := item.UsePetRestore.HPAmount(pet.MaxHP)
	mpAmount := item.UsePetRestore.MPAmount(pet.MaxMP)
	hpNeeded := hpAmount > 0 && pet.HP < pet.MaxHP
	mpNeeded := mpAmount > 0 && pet.MP < pet.MaxMP
	if !hpNeeded && !mpNeeded {
		s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: "当前宠物生命和法力无需恢复"})
		return
	}
	if !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	if hpNeeded {
		s.emit(combat.Restore(owner, pet, hpAmount, 0))
	}
	if mpNeeded {
		pet.MP += mpAmount
		if pet.MP > pet.MaxMP {
			pet.MP = pet.MaxMP
		}
	}
	pet.Pet.Inst.HP, pet.Pet.Inst.MP = pet.HP, pet.MP
	owner.Player.MarkDirty()
	startCooldown()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	s.requestNewbieTip(owner.ID, 5)
	s.log.Debug("使用宠物恢复物品", "char", owner.Name, "item", item.Name,
		"pet", pet.Pet.Inst.ID, "hp", hpAmount, "mp", mpAmount)
}

func (s *Scene) usePetExperienceItem(owner *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Alive() || s.petLvls == nil {
		reject(event.RejectNoPet)
		return
	}
	inst := pet.Pet.Inst
	rule := item.PetExperience
	experience := rule.Experience(inst.Level)
	if experience <= 0 {
		reject(event.RejectInvalid)
		return
	}
	if inst.Level > owner.Player.Char.Level+rule.MaxAboveOwner {
		s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID,
			Text: fmt.Sprintf("宠物等级超过人物等级%d级，不能使用宝宝经验珠", rule.MaxAboveOwner)})
		return
	}
	cap := s.petLvls.MaxLevel()
	if max := pet.Pet.Def.MaxLevel; max > 0 && cap > max {
		cap = max
	}
	if inst.Level >= cap {
		s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID,
			Text: "当前宠物已达到最高等级"})
		return
	}
	if !item.ConsumeOnUse || !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	beforeLevel := inst.Level
	s.addPetExperience(owner, pet, experience, cap)
	startCooldown()
	s.pushInventory(owner)
	s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID,
		Text: fmt.Sprintf("%s获得%d经验（%d级→%d级）", pet.Pet.Def.Name, experience, beforeLevel, inst.Level)})
	s.log.Info("使用宝宝经验珠", "char", owner.Name, "item", item.ID,
		"pet", inst.ID, "experience", experience, "from_level", beforeLevel, "to_level", inst.Level)
}

func (s *Scene) useSkillBook(p *entity.Entity, bagSlot int, item domain.ItemDef,
	reject func(event.RejectReason)) {
	def, nextLv, reason, ok := s.skillUpgradeTarget(p, item.UseSkillBook)
	if !ok {
		reject(reason)
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	ch := p.Player.Char
	if ch.Skills == nil {
		ch.Skills = make(domain.Learned)
	}
	ch.Skills[item.UseSkillBook] = nextLv
	p.Player.MarkDirty()
	if def.Kind == domain.SkillPassive {
		s.refreshStats(p)
	}
	s.emitTo(p.ID, s.skillSnapshot(p))
	s.pushInventory(p)
	if nextLv > 1 {
		s.requestNewbieTip(p.ID, 9)
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID,
		Text: fmt.Sprintf("%s提升到%d级", def.Name, nextLv)})
	s.log.Info("使用职业技能书", "char", ch.Name, "item", item.Name,
		"skill", def.Name, "level", nextLv)
}

func (s *Scene) useReturnItem(p *entity.Entity, bagSlot int, def domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if !s.home.Enabled() || s.home.Scene != s.id && s.router == nil {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "当前场景无法使用回城物品"})
		return
	}
	if def.ConsumeOnUse && !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	p.Player.MarkDirty()
	startCooldown()
	if def.ConsumeOnUse {
		s.pushInventory(p)
	}
	name := p.Player.Char.Name
	if !s.onTeleport(Teleport{ID: p.ID, To: s.home.Scene, At: s.home.Pos}) {
		s.log.Error("回城物品传送未执行", "char", name, "item", def.ID)
		return
	}
	s.log.Info("使用回城物品", "char", name, "item", def.Name,
		"consume", def.ConsumeOnUse, "to", s.home.Scene)
}

func (s *Scene) useStatusRemovalItem(p *entity.Entity, bagSlot int, def domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if p.Status == nil {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "当前没有可解除的异常状态"})
		return
	}
	removeSet := make(map[domain.StatusID]struct{}, len(def.UseRemoveStatuses))
	for _, id := range def.UseRemoveStatuses {
		removeSet[id] = struct{}{}
	}
	removed := p.Status.RemoveWhere(func(status domain.StatusDef) bool {
		if def.UseClearStatuses {
			return true
		}
		_, ok := removeSet[status.ID]
		return ok
	})
	if len(removed) == 0 {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "当前没有可解除的异常状态"})
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		s.log.Error("物品状态已解除但背包扣除失败", "char", p.Player.Char.Name,
			"item", def.ID, "slot", bagSlot)
		return
	}
	p.Player.MarkDirty()
	startCooldown()
	s.statusesRemoved(p, removed)
	s.pushInventory(p)
	s.requestNewbieTip(p.ID, 5)
	s.log.Debug("使用解除状态物品", "char", p.Player.Char.Name, "item", def.Name,
		"removed", removed)
}

func (s *Scene) useResourceRestoreItem(p *entity.Entity, bagSlot int, def domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	hpAmount := def.UseRestore.HPAmount(p.MaxHP)
	mpAmount := def.UseRestore.MPAmount(p.MaxMP)
	hpNeeded := hpAmount > 0 && p.HP < p.MaxHP
	mpNeeded := mpAmount > 0 && p.MP < p.MaxMP
	if !hpNeeded && !mpNeeded {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "当前生命和法力无需恢复"})
		return
	}
	if !p.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	if hpNeeded {
		s.emit(combat.Restore(p, p, hpAmount, 0))
	}
	if mpNeeded {
		p.MP += mpAmount
		if p.MP > p.MaxMP {
			p.MP = p.MaxMP
		}
	}
	p.Player.MarkDirty()
	startCooldown()
	s.emitAttributesIfPlayer(p)
	s.pushInventory(p)
	s.requestNewbieTip(p.ID, 5)
	s.log.Debug("使用即时恢复物品", "char", p.Player.Char.Name, "item", def.Name,
		"hp", hpAmount, "mp", mpAmount)
}

func (s *Scene) activePetInstance(owner *entity.Entity) (*domain.PetInstance, domain.PetDef, bool) {
	if owner == nil || owner.Player == nil || owner.Player.Pet == 0 {
		return nil, domain.PetDef{}, false
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Pet.Inst.Hatched {
		return nil, domain.PetDef{}, false
	}
	return pet.Pet.Inst, pet.Pet.Def, true
}

func (s *Scene) usePetLearningItem(owner *entity.Entity, bagSlot int,
	item domain.PetLearningItem, reject func(event.RejectReason)) {
	inst, def, ok := s.activePetInstance(owner)
	if !ok {
		reject(event.RejectNoPet)
		return
	}
	skillDef, exists := s.petSkills[item.Skill]
	if !exists || item.ChanceBP <= 0 || item.ChanceBP > 10000 ||
		!inst.CanLearn(def, item.Skill) {
		reject(event.RejectPetCannotLearn)
		return
	}
	// 官方规则允许从 29 级开始为 30 级技能吃丸；同一级连续吃时，最后一颗
	// 覆盖前一颗并照常消耗，不把第一颗错误地锁成不可替换状态。
	if skillDef.LearnLevel > inst.Level+1 {
		reject(event.RejectLevelTooLow)
		return
	}
	if !s.petSkillSpace(inst, item.Skill) {
		if skillDef.Fight {
			reject(event.RejectPetFightSkillsFull)
		} else {
			reject(event.RejectPetLifeSkillsFull)
		}
		return
	}
	if !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	inst.PendingLearn = domain.PetLearningBoost{
		Item: item.Item, Skill: item.Skill, ChanceBP: item.ChanceBP,
	}
	owner.Player.MarkDirty()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	s.log.Info("宠物使用领悟道具", "char", owner.Name, "宠实例", inst.ID,
		"道具", item.Name, "目标技能", item.Skill, "领悟万分比", item.ChanceBP)
}

func (s *Scene) usePetFood(owner *entity.Entity, bagSlot int, food domain.PetFood,
	reject func(event.RejectReason)) {
	inst, _, ok := s.activePetInstance(owner)
	if !ok {
		reject(event.RejectNoPet)
		return
	}
	if !food.Usable(inst.Starve) {
		reject(event.RejectWrongFoodTier)
		return
	}
	dStarve, dTrust, ok := inst.Feed(food)
	if !ok {
		reject(event.RejectWrongFoodTier)
		return
	}
	if !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		s.log.Error("宠物食物效果已施加但背包扣除失败", "char", owner.Name,
			"item", food.Item, "slot", bagSlot)
		return
	}
	owner.Player.MarkDirty()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	s.emitTo(owner.ID, event.PetFed{Who: owner.ID, Inst: inst.ID,
		Starve: inst.Starve, Trust: inst.Trust,
		DeltaStarve: dStarve, DeltaTrust: dTrust})
	s.log.Debug("喂宠物", "char", owner.Name, "食物", food.Name,
		"饥饿", inst.Starve, "信赖", inst.Trust)
}
