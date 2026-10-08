package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 打工。
//
// 节奏见 docs/MVP边界.md「打工与耐力」——**这一块是刻意改过的，不是还原原作**：
// 原版 100 点耐力能挂 8.3 小时，我们压缩进 100 分钟，单次收益与总量都不变。
//
// 两个计时器各走各的，不能合成一个：
//
//	结算  每 UnitSec 秒一次（打工 36 秒、钓鱼 12 秒、挖矿 3 秒）
//	耐力  每分钟 −1 点
//
// 合成一个就得把耐力按结算次数摊成小数（36 秒 = 0.6 点），
// 而整数属性上的小数迟早会被取整吃掉 —— 那正是"挂了一天发现耐力没扣完"的来源。

// onStartWork 开工。
func (s *Scene) onStartWork(cmd StartWork) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil || p.Player.Char == nil {
		return
	}
	if p.Player.Stall != nil {
		s.stallNotice(p, "摆摊中不能开始打工，请先结束摊位")
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "StartWork", Reason: r})
	}
	if !p.Alive() {
		reject(event.RejectTargetDead)
		return
	}
	if !canAct(p) {
		reject(event.RejectStunned)
		return
	}
	if s.works == nil {
		reject(event.RejectUnknown)
		return
	}
	ch := p.Player.Char
	def, ok := s.works.ResolveStart(cmd.Work, ch.Skills)
	if !ok {
		reject(event.RejectNoTarget)
		return
	}
	if s.refillStamina(ch) {
		p.Player.MarkDirty()
		s.emitTo(p.ID, s.attributeSnapshot(p))
	}
	skillLearned := def.RequiredSkill == 0
	if def.RequiredSkill != 0 {
		_, skillLearned = ch.Skills[def.RequiredSkill]
	}

	switch domain.CanWork(def, ch.Level, ch.Stamina, skillLearned, p.Player.Work != nil) {
	case domain.WorkSkillNotLearned:
		reject(event.RejectSkillNotLearned)
		return
	case domain.WorkLevelTooLow:
		reject(event.RejectLevelTooLow)
		return
	case domain.WorkLevelTooHigh:
		reject(event.RejectLevelTooHigh)
		return
	case domain.WorkNoStamina:
		reject(event.RejectStaminaEmpty)
		return
	case domain.WorkAlreadyWorking:
		reject(event.RejectAlreadyWorking)
		return
	}
	if reason := s.workRequirementFailure(p, def); reason != event.RejectUnknown {
		reject(reason)
		return
	}

	p.Player.Work = &domain.WorkSession{
		Def:          def,
		NextSettleAt: s.tick + s.workSettleTicks(p, def),
		NextDrainAt:  s.tick + s.stamina.StaminaDrainTicks(),
	}
	p.Player.StopResting()
	s.emit(event.WorkStarted{Who: p.ID, Work: int32(def.ID), Title: def.Title})
	s.log.Debug("开工", "char", ch.Name, "活", def.Title, "耐力", ch.Stamina)
}

// onStopWork 收工。
func (s *Scene) onStopWork(cmd StopWork) {
	p, ok := s.players[cmd.ID]
	if !ok {
		return
	}
	s.stopWork(p, event.WorkStoppedByPlayer)
}

// onGatherWork 消费正式客户端 0x1025。客户端每 3 秒调用一次；服务端只在
// 自己的结算时刻已经到达时结算一次，因此重放/加速发包不会增加收益。
func (s *Scene) onGatherWork(cmd GatherWork) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Work == nil || !p.Alive() {
		return
	}
	w := p.Player.Work
	expected := w.Def.ID
	if w.Def.RequiredSkill != 0 {
		expected = domain.WorkID(w.Def.RequiredSkill)
	}
	if cmd.Token != expected || w.NextSettleAt <= 0 || s.tick < w.NextSettleAt {
		return
	}
	w.NextSettleAt = s.tick + s.workSettleTicks(p, w.Def)
	w.Settled++
	s.settleWork(p, w.Def)
}

// stopWork 停掉一个人的打工并广播。已经没在打工时什么都不做。
func (s *Scene) stopWork(p *entity.Entity, why event.WorkStopReason) {
	if p.Player == nil || p.Player.Work == nil {
		return
	}
	w := p.Player.Work
	p.Player.Work = nil
	ev := event.WorkStopped{Who: p.ID, Work: int32(w.Def.ID),
		Reason: why, Settled: w.Settled}
	// 旁观者按收工发生时的旧 AOI 格接收；本人必须定向接收。移动/踩门会在
	// 同一帧内先改变 AOI 位置甚至离场，若也靠旧格广播，本人可能在 flush 时
	// 已不在旧九宫格而漏掉收工状态。
	s.emitExcept(ev, p.ID)
	s.emitTo(p.ID, ev)
	s.log.Debug("收工", "char", p.Name, "活", w.Def.Title, "结算次数", w.Settled)
}

// stepWork 推进所有人的打工。
//
// 排在帧序的**状态之后、AI 之前**：打工的人身上也可能有状态（比如中毒），
// 那一跳该掉的血要先算；而打工本身不参与战斗，排在 AI 前后都行，
// 放前面是为了让"耐力耗尽自动收工"这件事在同一帧内立刻生效。
func (s *Scene) stepWork() {
	for _, p := range s.players {
		w := p.Player.Work
		if w == nil {
			continue
		}
		if p.Player.TradeBusy {
			// Preserve both clocks and pending yield while inventory is reserved.
			// No stamina is spent on a yield that cannot yet be granted.
			continue
		}
		if !p.Alive() {
			s.stopWork(p, event.WorkStoppedByDeath)
			continue
		}
		// 生活技能熟练度会在产出后增长。跨过 100/400/... 阈值时，下一次
		// 结算必须立刻切到新档，保留全部旧产出并加入刚解锁的矿/鱼；不能把
		// 开工时选中的 WorkDef 固定到玩家手动收工为止。
		if w.Def.RequiredSkill != 0 {
			if current, ok := s.works.ResolveStart(domain.WorkID(w.Def.RequiredSkill), p.Player.Char.Skills); ok {
				w.Def = current
			}
		}
		if reason := s.workRequirementFailure(p, w.Def); reason != event.RejectUnknown {
			s.stopWork(p, workStopReason(reason))
			continue
		}

		// 扣耐力。耗尽就收工 —— 100 点 × 每分钟 1 点 = 100 分钟
		if w.NextDrainAt > 0 && s.tick >= w.NextDrainAt {
			ch := p.Player.Char
			ch.Stamina -= s.stamina.CostPerMin
			if ch.Stamina < 0 {
				ch.Stamina = 0
			}
			p.Player.MarkDirty()
			s.emitTo(p.ID, s.attributeSnapshot(p))
			w.NextDrainAt = s.tick + s.stamina.StaminaDrainTicks()
			if ch.Stamina <= 0 {
				s.stopWork(p, event.WorkStoppedNoStamina)
				continue
			}
		}

		// 生产场景收益只由正式客户端 0x1025 Gather 触发；纯领域测试可关闭该
		// 外部边界，用时钟直接验证结算公式。
		if !s.requireWorkGather && w.NextSettleAt > 0 && s.tick >= w.NextSettleAt {
			w.NextSettleAt = s.tick + s.workSettleTicks(p, w.Def)
			w.Settled++
			s.settleWork(p, w.Def)
		}
	}
}

// workSettleTicks 将 ov_work.pet_skill_add 解释为工作速度百分比。
// add=100 时每轮耗时减半；只认当前出战宠物真实学会的对应辅助技能。
func (s *Scene) workSettleTicks(owner *entity.Entity, d domain.WorkDef) domain.Tick {
	base := d.SettleTicks()
	if base <= 0 || d.PetSkill == 0 || d.PetSkillAddPct <= 0 ||
		s.petWorkEfficiencyPct(owner, d.PetSkill) <= 0 {
		return base
	}
	denominator := int64(100 + d.PetSkillAddPct)
	ticks := (int64(base)*100 + denominator - 1) / denominator
	if ticks < 1 {
		ticks = 1
	}
	return domain.Tick(ticks)
}

// settleWork 结算一次打工收益。
//
// 经验与名誉必给；**钱要掷** —— coin_prob 是千分比（800 = 80%）。
// 掷的是钱不是经验，这是原表的口径，不要顺手把经验也做成概率。
func (s *Scene) settleWork(p *entity.Entity, d domain.WorkDef) {
	ch := p.Player.Char
	inventoryChanged := false
	if d.Honor > 0 {
		ch.Honor += d.Honor
		inventoryChanged = true
	}
	if d.Coin > 0 && (d.CoinProb >= 1000 || int32(s.rng.Intn(1000)) < d.CoinProb) {
		// 打工给的是铜币。三种面额分开存, 不折算(见 domain/quest.go 的说明)
		ch.Money.Copper += d.Coin
		inventoryChanged = true
	}
	itemsChanged, practiseGain, stopReason, shouldStop := s.settleWorkItems(p, d)
	inventoryChanged = itemsChanged || inventoryChanged
	if practiseGain > 0 && d.RequiredSkill != 0 {
		current := ch.Skills.LevelOf(d.RequiredSkill)
		next := current + practiseGain
		if next > 10000 {
			next = 10000
		}
		ch.Skills[d.RequiredSkill] = next
		s.emitTo(p.ID, s.lifeSkillSnapshot(p))
		s.log.Debug("生活技能熟练度增长", "char", ch.Name, "skill", d.RequiredSkill,
			"原熟练度", current, "增加", practiseGain, "当前熟练度", next)
	}
	p.Player.MarkDirty()
	if inventoryChanged {
		if err := s.pushInventory(p); err != nil {
			s.log.Error("打工结算后推送背包失败", "char", ch.Name, "work", d.ID, "err", err)
		}
	}
	s.emitTo(p.ID, event.WorkSettled{Who: p.ID, Work: int32(d.ID),
		Exp: d.Exp, Honor: d.Honor, Stamina: ch.Stamina})

	// 经验放最后 —— 它可能触发升级并重算属性, 让那件事发生在其余都稳定之后
	if d.Exp > 0 {
		s.grantExp(p, d.Exp)
	}
	if shouldStop {
		s.stopWork(p, stopReason)
	}
}

// settleWorkItems 发一次打工的物品产出。
//
// **物品是打工的产出大头**：88 条产出条目里 81 条是物品，钓鱼更是只给物品 ——
// 不发的话钓鱼这个 MVP 内的正经工种结算下来什么都没有。
//
// 掷法(逐条独立 / 按权重单选)由 domain.RollItems 里的 WeightedPick 决定，
// 这里不关心自己在钓鱼还是在管杂务。
func (s *Scene) settleWorkItems(p *entity.Entity, d domain.WorkDef) (bool, int32, event.WorkStopReason, bool) {
	got := d.RollItems(func(n int32) int32 {
		if n <= 0 {
			return 0
		}
		return int32(s.rng.Intn(int(n)))
	})
	if len(got) == 0 {
		return false, 0, 0, false
	}

	// 有逐次消耗品的工种先在背包副本上完成“扣消耗品 + 放入全部产出”。
	// 这样最后一个鱼饵腾出的格子可以接住这条鱼；若仍放不下，也不会出现
	// 鱼饵已扣但鱼丢失的半笔结算。
	if d.Requirements.Consumable != 0 && d.Requirements.ConsumablePerYield > 0 {
		next := p.Player.Bag.Clone()
		if next == nil || !next.Remove(d.Requirements.Consumable, d.Requirements.ConsumablePerYield) {
			return false, 0, event.WorkStoppedNoConsumable, true
		}
		for _, it := range got {
			def, ok := s.itemDef(it.Item)
			if !ok {
				s.log.Warn("打工产出不在物品表里", "活", d.Title, "物品", it.Item)
				return false, 0, 0, false
			}
			if left := next.Add(def, it.Qty); left > 0 {
				s.log.Debug("背包满, 打工中止", "char", p.Name, "物品", def.Name)
				return false, 0, event.WorkStoppedBagFull, true
			}
		}
		p.Player.Bag = next
		practiseGain := s.rollWorkPractiseGain(got)
		broken := s.consumeWorkToolDurability(p, d)
		if broken {
			return true, practiseGain, event.WorkStoppedToolBroken, true
		}
		if next.UsableCountOf(d.Requirements.Consumable) < d.Requirements.ConsumablePerYield {
			return true, practiseGain, event.WorkStoppedNoConsumable, true
		}
		return true, practiseGain, 0, false
	}

	granted := false
	var practiseGain int32
	for _, it := range got {
		def, ok := s.itemDef(it.Item)
		if !ok {
			s.log.Warn("打工产出不在物品表里", "活", d.Title, "物品", it.Item)
			continue
		}
		if left := p.Player.Bag.Add(def, it.Qty); left > 0 {
			// 背包满了就停工 —— 继续挂着只会把后面的产出一件件扔掉,
			// 而玩家看不见任何提示, 回来发现白挂了两小时
			s.log.Debug("背包满, 打工中止", "char", p.Name, "物品", def.Name)
			if left < it.Qty {
				practiseGain += s.rollWorkItemPractiseGain(it)
			}
			return granted || left < it.Qty, practiseGain, event.WorkStoppedBagFull, true
		}
		granted = true
		practiseGain += s.rollWorkItemPractiseGain(it)
	}
	if granted && s.consumeWorkToolDurability(p, d) {
		return true, practiseGain, event.WorkStoppedToolBroken, true
	}
	return granted, practiseGain, 0, false
}

func (s *Scene) rollWorkPractiseGain(items []domain.WorkItem) int32 {
	var total int32
	for _, it := range items {
		total += s.rollWorkItemPractiseGain(it)
	}
	return total
}

func (s *Scene) rollWorkItemPractiseGain(it domain.WorkItem) int32 {
	if it.PractiseGainMax <= 0 {
		return 0
	}
	return int32(s.rng.Intn(int(it.PractiseGainMax))) + 1
}

// workRequirementFailure 对实际穿戴槽与权威背包做准入检查。客户端可能在发送
// BeginWork 前本地换装，但这只是便利操作，不能代替服务端裁决。
func (s *Scene) workRequirementFailure(p *entity.Entity, d domain.WorkDef) event.RejectReason {
	if p == nil || p.Player == nil {
		return event.RejectUnknown
	}
	if req, ok := d.Requirements.EquipmentOf(domain.WorkRequirementOutfit); ok {
		if p.Player.Worn == nil {
			return event.RejectWorkOutfitMissing
		}
		if _, ok := equippedRequirement(p.Player.Worn, req); !ok {
			return event.RejectWorkOutfitMissing
		}
	}
	if req, ok := d.Requirements.EquipmentOf(domain.WorkRequirementTool); ok {
		if p.Player.Worn == nil {
			return event.RejectWorkToolMissing
		}
		st, ok := equippedRequirement(p.Player.Worn, req)
		if !ok {
			return event.RejectWorkToolMissing
		}
		if st.Durability <= 0 {
			return event.RejectWorkToolBroken
		}
	}
	if d.Requirements.Consumable != 0 && d.Requirements.ConsumablePerYield > 0 {
		if p.Player.Bag == nil ||
			p.Player.Bag.UsableCountOf(d.Requirements.Consumable) < d.Requirements.ConsumablePerYield {
			return event.RejectWorkConsumableMissing
		}
	}
	return event.RejectUnknown
}

func equippedRequirement(worn *domain.EquipSet, req domain.WorkEquipmentRequirement) (domain.Stack, bool) {
	st := worn.At(req.Slot)
	if st.Empty() {
		return domain.Stack{}, false
	}
	for _, item := range req.Items {
		if st.Item == item {
			return st, true
		}
	}
	return domain.Stack{}, false
}

func workStopReason(reason event.RejectReason) event.WorkStopReason {
	switch reason {
	case event.RejectWorkOutfitMissing:
		return event.WorkStoppedMissingOutfit
	case event.RejectWorkToolMissing:
		return event.WorkStoppedMissingTool
	case event.RejectWorkToolBroken:
		return event.WorkStoppedToolBroken
	case event.RejectWorkConsumableMissing:
		return event.WorkStoppedNoConsumable
	default:
		return event.WorkStoppedByAction
	}
}

// consumeWorkToolDurability 把 ov_arm.attack_consume 的原始消耗累计到工具实例。
// 不足一个展示点的余数也会落盘，因此重登不会重置工具的磨损进度。
func (s *Scene) consumeWorkToolDurability(p *entity.Entity, d domain.WorkDef) bool {
	req, ok := d.Requirements.EquipmentOf(domain.WorkRequirementTool)
	if !ok {
		return false
	}
	st, ok := equippedRequirement(p.Player.Worn, req)
	if !ok || st.Durability <= 0 {
		return false
	}
	def, ok := s.itemDef(st.Item)
	if !ok || def.Equip == nil || def.Equip.AttackDurabilityCostRaw <= 0 {
		return false
	}
	changed, _, broken := domain.ConsumeDurabilityRaw(&st, def, def.Equip.AttackDurabilityCostRaw)
	if !changed {
		return false
	}
	p.Player.Worn.Set(req.Slot, st)
	p.Player.MarkDirty()
	return broken
}

// refillStamina 按"每天回满一次"补耐力。
//
// 在**开工时**补而不是每帧检查: 每帧给几百人算日期是白烧 CPU,
// 而玩家只有在要用耐力的那一刻才关心它满不满。
func (s *Scene) refillStamina(ch *domain.Character) bool {
	if s.now == nil {
		return false
	}
	if ch.RefillStamina(s.stamina, domain.DayOf(s.now())) {
		s.log.Debug("耐力回满", "char", ch.Name, "耐力", ch.Stamina)
		return true
	}
	return false
}

// interruptWork 只保留给场景传送这种会重建玩家运行态的生命周期边界。
// 正式客户端会在打工期间继续上报自身坐标；移动、攻击、技能、换装等普通
// 操作都不是收工协议，不能把它们擅自解释成 0x1028 EndWork。
func (s *Scene) interruptWork(p *entity.Entity) {
	if p.Player != nil && p.Player.Work != nil {
		s.stopWork(p, event.WorkStoppedByAction)
	}
}
