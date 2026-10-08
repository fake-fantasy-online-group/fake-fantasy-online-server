package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 任务：接取与交付。
//
// **NPC 是实体**，不是一批静态的落位包。任务操作只认当前地图里实际存在、
// 名字匹配的 NPC。客户端既然已经打开了对话框，服务端就不能再额外要求距离。

// onNpcTasks 回「这个 NPC 身上有什么任务」。
//
// 任务列表请求是完成包唯一可用的 NPC 对话上下文；在这里确认 NPC 属于当前地图并记住目标。
func (s *Scene) onNpcTasks(cmd NpcTasks) {
	p, ok := s.players[cmd.ID]
	if !ok || s.quests == nil {
		return
	}
	npc := s.npcOnMap(cmd.NPC)
	if npc == "" {
		p.Player.QuestNPC = ""
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "NpcTasks", Reason: event.RejectNoTarget})
		return
	}
	p.Player.QuestNPC = npc
	ch := p.Player.Char
	if ch.Quests == nil {
		ch.Quests = domain.QuestLog{}
	}
	offers := domain.OffersOf(s.quests, npc, ch, func(it domain.ItemID) int32 {
		return p.Player.Bag.CountOf(it)
	})
	// NPC 对话内容与职能菜单全部由客户端原生逻辑负责；服务端这里只刷新任务列表。
	// 不下发 NpcDialogText/ShowSay，避免清空客户端 BuildMenu 生成的职能菜单。
	// **列表为空也要回。** 不回的话客户端的对话框会一直等,
	// 而"点了 NPC 没反应"与"这个 NPC 没任务"在界面上是一回事, 查起来很痛苦。
	s.emitTo(p.ID, event.NpcTasks{Who: p.ID, NPC: npc, Offers: offers})
	s.log.Debug("NPC 任务列表", "char", ch.Name, "npc", npc, "条数", len(offers))
}

// onAcceptQuest 接任务。
func (s *Scene) onAcceptQuest(cmd AcceptQuest) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "AcceptQuest", Reason: r})
	}
	if s.quests == nil {
		reject(event.RejectUnknown)
		return
	}
	if s.trial != nil && cmd.Quest == s.trial.Quest {
		s.acceptTrial(p)
		return
	}
	def, ok := s.quests[cmd.Quest]
	if !ok {
		reject(event.RejectNoTarget)
		return
	}
	// **发布者以任务定义为准, 不信客户端。**
	//
	// 上行的接任务包(0x1033)只带一个任务号, 根本没有 NPC 名 —— 客户端也不需要说,
	// 因为"这个任务归谁发"是配置里写死的。cmd.NPC 只有测试会填,
	// 留着是为了能构造"站在错误的 NPC 面前"这种场景。
	want := cmd.NPC
	if want == "" {
		// 真实 0x1033 只有任务号。发布者来自之前成功打开的
		// 0x1032 对话；客户端已经允许这次交互，接取时不再重复判距离。
		want = p.Player.QuestNPC
	}
	npc := s.npcOnMap(want)
	if npc == "" {
		reject(event.RejectNoTarget)
		return
	}
	ch := p.Player.Char
	if ch.Quests == nil {
		ch.Quests = domain.QuestLog{}
	}
	if why := ch.Quests.CanAccept(def, ch.Level, npc); why != domain.AcceptOK {
		reject(acceptReject(why))
		return
	}
	if !ch.AllowsCareerQuest(def.ID) {
		reject(event.RejectWrongProf)
		return
	}
	planned, changed, ok := s.planQuestItems(p.Player.Bag, nil, def.AcceptGive, nil)
	if !ok {
		reject(event.RejectBagFull)
		return
	}
	ch.Quests.Accept(def)
	if changed {
		p.Player.Bag = planned
		s.pushInventory(p)
	}
	p.Player.MarkDirty()
	s.emitTo(p.ID, event.QuestAccepted{Who: p.ID, Quest: int32(def.ID), Name: def.Name})
	s.pushQuestLog(p)
	s.pushQuestOffers(p, npc)
	s.log.Debug("接任务", "char", ch.Name, "任务", def.Name)
}

// onAbandonQuest 放弃任务。
//
// 与接/交不同,**不需要站在 NPC 面前** —— 放弃是背包界面上的操作,
// 客户端的 0x1034 也只带一个任务号。
func (s *Scene) onAbandonQuest(cmd AbandonQuest) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	ch := p.Player.Char
	if ch.Quests == nil || !ch.Quests.Abandon(cmd.Quest) {
		// 没接过或已经做完时仍需发送拒绝回执。
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "AbandonQuest",
			Reason: event.RejectNoTarget})
		return
	}
	p.Player.MarkDirty()
	s.pushQuestLog(p) // 任务本是整份覆盖, 放弃之后必须重发, 否则客户端还挂着
	s.pushQuestOffers(p, p.Player.QuestNPC)
	name := ""
	if def, ok := s.quests[cmd.Quest]; ok {
		name = def.Name
	}
	s.log.Debug("放弃任务", "char", ch.Name, "任务", name, "号", cmd.Quest)
}

// onCompleteQuest 交任务。
func (s *Scene) onCompleteQuest(cmd CompleteQuest) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "CompleteQuest", Reason: r})
	}
	if s.quests == nil {
		reject(event.RejectUnknown)
		return
	}
	if s.trial != nil && cmd.Quest == s.trial.Quest {
		s.completeTrial(p)
		return
	}
	def, ok := s.quests[cmd.Quest]
	if !ok {
		reject(event.RejectNoTarget)
		return
	}
	ch := p.Player.Char
	if !ch.Quests.Active(def.ID) {
		reject(event.RejectQuestNotActive)
		return
	}
	if !ch.AllowsCareerQuest(def.ID) {
		reject(event.RejectWrongProf)
		return
	}
	if def.Exchange != nil {
		s.completeExchangeQuest(p, def, reject)
		return
	}
	if len(def.Steps) > 0 {
		s.completeQuestStep(p, def, reject)
		return
	}
	// 旧式定义只用于兼容尚未迁移的构造方；运行时数据库任务全部有 Steps。
	if s.npcOnMap(def.NPC) == "" {
		reject(event.RejectNoTarget)
		return
	}

	// 交物任务: **先确认收得下奖励, 再扣要交的东西**。
	// 反过来的话, 扣完发现背包塞不下奖励, 玩家就白白丢了任务物品。
	bagCount := p.Player.Bag.CountOf(def.GoalItem)
	if !ch.Quests.Deliverable(def, bagCount) {
		reject(event.RejectQuestNotDone)
		return
	}
	freed := int32(0)
	if def.NeedsItem() {
		freed = def.GoalQty
	}
	if !s.canHoldReward(p, def.Reward, def.GoalItem, freed) {
		reject(event.RejectBagFull)
		return
	}
	if def.NeedsItem() && !p.Player.Bag.Remove(def.GoalItem, def.GoalQty) {
		reject(event.RejectQuestNotDone)
		return
	}

	employed := ch.CompleteCareerQuest(def.ID)
	ch.Quests.Finish(def.ID)
	s.grantQuestReward(p, def)
	p.Player.MarkDirty()
	if employed {
		p.Look.Race = domain.Race(ch.ClientRace())
		// CompleteCareerQuest 已洗掉就职前加点；属性、技能书和快捷栏都要立即
		// 覆盖客户端，不能等到重登才看到职业状态。
		s.refreshStats(p)
		s.emitTo(p.ID, s.skillSnapshot(p))
		s.emitTo(p.ID, s.hotbarSnapshot(p))
	}
	s.pushInventory(p)
	s.emitTo(p.ID, s.questCompletedEvent(p.ID, def, def.Reward, ""))
	npc := p.Player.QuestNPC
	if npc == "" {
		npc = def.NPC
	}
	s.pushQuestOffers(p, npc)
	p.Player.QuestNPC = ""
	s.log.Info("完成任务", "char", ch.Name, "任务", def.Name,
		"经验", def.Reward.Exp, "名誉", def.Reward.Honor, "就职", employed)
}

// completeQuestStep 提交结构化任务的当前一步。中间步骤只推进游标并发任务物品；
// 最后一步才标完成和发奖励。
func (s *Scene) completeQuestStep(p *entity.Entity, def domain.QuestDef, reject func(event.RejectReason)) {
	ch := p.Player.Char
	target := ch.Quests.StepTarget(def)
	// TaskComplete 只带任务号。之前成功打开对应 NPC 的任务对话框已经
	// 建立了这次提交的上下文；提交时不再重复做距离校验。
	if target == "" || p.Player.QuestNPC != target {
		reject(event.RejectNoTarget)
		return
	}
	if !ch.Quests.DeliverableAt(def, target, p.Player.Bag.CountOf) {
		reject(event.RejectQuestNotDone)
		return
	}
	step, ok := ch.Quests.CurrentStep(def)
	if !ok {
		reject(event.RejectQuestNotDone)
		return
	}
	final := int(ch.Quests[def.ID].Progress)+1 >= len(def.Steps)
	var rewardItems []domain.RewardItem
	if final {
		rewardItems = def.Reward.Items
	}
	planned, bagChanged, ok := s.planQuestItems(
		p.Player.Bag, step.RequiredItems(), step.Give, rewardItems)
	if !ok {
		reject(event.RejectBagFull)
		return
	}
	if final && len(def.ProfessionRewards) > 0 {
		reward, found := def.ProfessionRewards[ch.Race]
		item, known := s.itemDef(reward.Item)
		if !found || !known || item.Equip == nil {
			reject(event.RejectWrongProf)
			return
		}
		instance := domain.NewStack(item, 1)
		instance.RefineLevel = reward.RefineLevel
		if planned.AddStack(item, instance) != 0 {
			reject(event.RejectBagFull)
			return
		}
		bagChanged = true
		// 只为本次完成回执追加，不能改写共享任务模板或重复发放物品。
		def.Reward.Items = append(append([]domain.RewardItem(nil), def.Reward.Items...),
			domain.RewardItem{Item: reward.Item, Qty: 1})
	}
	if bagChanged {
		p.Player.Bag = planned
	}

	if !final {
		ch.Quests.Advance(def)
		p.Player.MarkDirty()
		if bagChanged {
			s.pushInventory(p)
		} else {
			s.pushQuestLog(p)
		}
		s.pushQuestOffers(p, target)
		p.Player.QuestNPC = ""
		s.log.Info("推进任务", "char", ch.Name, "任务", def.Name,
			"步骤", ch.Quests[def.ID].Progress+1, "总步骤", len(def.Steps))
		return
	}

	employed := ch.CompleteCareerQuest(def.ID)
	ch.Quests.Finish(def.ID)
	// 奖励物品已经在 planned 背包里原子预演并提交；这里只结算钱、名誉和经验。
	s.grantQuestRewardValues(p, def.Reward)
	p.Player.MarkDirty()
	if employed {
		p.Look.Race = domain.Race(ch.ClientRace())
		s.refreshStats(p)
		s.emitTo(p.ID, s.skillSnapshot(p))
		s.emitTo(p.ID, s.hotbarSnapshot(p))
	}
	// 钱和名誉也在背包快照中，即使没有物品变化也必须刷新。
	s.pushInventory(p)
	s.emitTo(p.ID, s.questCompletedEvent(p.ID, def, def.Reward, ""))
	s.pushQuestOffers(p, target)
	p.Player.QuestNPC = ""
	s.log.Info("完成任务", "char", ch.Name, "任务", def.Name,
		"经验", def.Reward.Exp, "名誉", def.Reward.Honor, "就职", employed)
}

func (s *Scene) questCompletedEvent(who domain.EntityID, def domain.QuestDef,
	reward domain.QuestReward, title string) event.QuestCompleted {
	items := make([]event.QuestRewardItem, 0, len(reward.Items))
	for _, itemReward := range reward.Items {
		item, ok := s.itemDef(itemReward.Item)
		if !ok || itemReward.Qty <= 0 {
			continue
		}
		items = append(items, event.QuestRewardItem{Name: item.Name, Qty: itemReward.Qty})
	}
	return event.QuestCompleted{
		Who: who, Quest: int32(def.ID), Name: def.Name,
		Exp: reward.Exp, Honor: reward.Honor, Money: reward.Money, Items: items, Title: title,
	}
}

// planQuestItems 在背包副本上预演一次任务物品事务。任何目标不足、物品定义缺失
// 或背包放不下都返回 false，权威背包保持不变。
func (s *Scene) planQuestItems(bag *domain.Bag, take, give []domain.QuestItemGoal,
	reward []domain.RewardItem) (*domain.Bag, bool, bool) {
	if bag == nil {
		return nil, false, false
	}
	planned := bag.Clone()
	changed := false
	for _, item := range take {
		if item.Item == 0 || item.Qty <= 0 || !planned.Remove(item.Item, item.Qty) {
			return nil, false, false
		}
		changed = true
	}
	for _, item := range give {
		def, ok := s.itemDef(item.Item)
		if !ok || item.Qty <= 0 || planned.Add(def, item.Qty) != 0 {
			return nil, false, false
		}
		changed = true
	}
	for _, item := range reward {
		def, ok := s.itemDef(item.Item)
		if !ok || item.Qty <= 0 || planned.Add(def, item.Qty) != 0 {
			return nil, false, false
		}
		changed = true
	}
	return planned, changed, true
}

// canHoldReward 判断背包装不装得下奖励物品。
//
// freed 是交任务会腾出来的格子数(交掉的东西), 要算进可用空间。
func (s *Scene) canHoldReward(p *entity.Entity, r domain.QuestReward, goal domain.ItemID, freed int32) bool {
	if len(r.Items) == 0 {
		return true
	}
	free := p.Player.Bag.FreeSlots()
	// 交掉的东西如果正好清空了格子, 那些格子能用来装奖励。
	// 保守估计: 只有当交的数量正好等于持有量时才算腾出格子。
	if freed > 0 && p.Player.Bag.CountOf(goal) == freed {
		free++
	}
	return free >= len(r.Items)
}

// grantQuestReward 发奖励。
//
// 顺序: 钱与名誉 → 物品 → 经验。**经验放最后**, 因为它可能触发升级,
// 而升级会重算属性并广播 —— 让那件事发生在背包已经稳定之后, 事件序才是干净的。
func (s *Scene) grantQuestReward(p *entity.Entity, def domain.QuestDef) {
	r := def.Reward
	if len(r.Items) > 0 {
		for _, it := range r.Items {
			d, ok := s.itemDef(it.Item)
			if !ok {
				s.log.Warn("任务奖励物品不在物品表里", "任务", def.Name, "物品", it.Item)
				continue
			}
			if left := p.Player.Bag.Add(d, it.Qty); left > 0 {
				// canHoldReward 过了还装不下, 说明估算偏乐观。宁可报出来也别静默吞掉。
				s.log.Error("任务奖励装不下", "任务", def.Name, "物品", d.Name, "剩余", left)
			}
		}
	}
	s.grantQuestRewardValues(p, r)
}

func (s *Scene) grantQuestRewardValues(p *entity.Entity, r domain.QuestReward) {
	ch := p.Player.Char
	if !r.Money.Empty() || r.Honor != 0 {
		ch.Money = ch.Money.Add(r.Money)
		ch.Honor += r.Honor
	}
	if r.Exp > 0 {
		s.grantExp(p, r.Exp)
	}
}

// completeExchangeQuest 结算“交多少算多少”的单步任务。客户端提交包没有数量
// 字段，因此权威语义是一次交付当前背包中的全部可收购物品。
func (s *Scene) completeExchangeQuest(p *entity.Entity, def domain.QuestDef,
	reject func(event.RejectReason)) {
	ch := p.Player.Char
	step, ok := ch.Quests.CurrentStep(def)
	if !ok || step.Kind != domain.QuestStepExchange || step.To == "" ||
		p.Player.QuestNPC != step.To {
		reject(event.RejectNoTarget)
		return
	}
	reward, take, title, total, ok := def.Exchange.ExchangeReward(func(item domain.ItemID) int32 {
		return p.Player.Bag.CountOf(item)
	})
	if !ok {
		reject(event.RejectQuestNotDone)
		return
	}
	planned, _, ok := s.planQuestItems(p.Player.Bag, take, nil, reward.Items)
	if !ok {
		reject(event.RejectBagFull)
		return
	}

	p.Player.Bag = planned
	ch.Quests.Finish(def.ID)
	titleUnlocked := ch.UnlockTitle(title)
	awardedTitle := ""
	if titleUnlocked {
		awardedTitle = title
	}
	s.grantQuestRewardValues(p, reward)
	p.Player.MarkDirty()
	s.pushInventory(p)
	if titleUnlocked {
		s.pushTitleSnapshot(p)
	}
	s.emitTo(p.ID, s.questCompletedEvent(p.ID, def, reward, awardedTitle))
	s.pushQuestOffers(p, step.To)
	p.Player.QuestNPC = ""
	s.log.Info("完成按量兑换任务", "char", ch.Name, "任务", def.Name,
		"种类", len(take), "总数量", total, "经验", reward.Exp,
		"名誉", reward.Honor, "铜币", reward.Money.Copper, "称号", title)
}

// pushQuestLog 把整份任务本发给某个玩家。
//
// **每次任务本变化都要发。** 客户端那边是整份覆盖不是增量 ——
// 不重发的话界面上还留着刚交掉的任务, 而玩家再点一次会被服务端拒(已经交过了),
// 表现是"任务卡在界面上点不掉"。
func (s *Scene) pushQuestLog(p *entity.Entity) {
	if s.quests == nil || p.Player == nil {
		return
	}
	ch := p.Player.Char
	list := domain.ActiveQuestsOf(s.quests, ch, func(it domain.ItemID) int32 {
		return p.Player.Bag.CountOf(it)
	})
	list = s.trialQuestLog(p, list)
	// 已完成的也要发: 客户端拿它判断 NPC 头顶显示什么、任务能不能再接。
	// 不发的话做过的任务会重新显示成"可接"。
	s.emitTo(p.ID, event.ActiveQuests{Who: p.ID, List: list, Done: ch.Quests.FinishedIDs()})
}

// pushQuestOffers 把当前 NPC 的任务列表与任务本一起刷新。客户端把 0x8027
// 和 0x8028 分别存进 TaskClient 的状态/候选列表；只更新前者会让刚完成的任务
// 仍残留在 NPC 对话上下文里，也看不到同一 NPC 被新解锁的后续任务。
func (s *Scene) pushQuestOffers(p *entity.Entity, npc string) {
	if s.quests == nil || p == nil || p.Player == nil || p.Player.Char == nil ||
		p.Player.Bag == nil || npc == "" {
		return
	}
	ch := p.Player.Char
	offers := domain.OffersOf(s.quests, npc, ch, func(it domain.ItemID) int32 {
		return p.Player.Bag.CountOf(it)
	})
	s.emitTo(p.ID, event.NpcTasks{Who: p.ID, NPC: npc, Offers: offers})
}

// recordQuestKill 只接受已经由战斗系统判定死亡的怪物。客户端攻击包不能直接改任务
// 进度；宠物补刀归到主人，和经验归属保持一致。
func (s *Scene) recordQuestKill(dead *entity.Entity, killer domain.EntityID) {
	if dead == nil || dead.Monster == nil || killer == 0 || s.quests == nil {
		return
	}
	if pet, ok := s.pets[killer]; ok && pet.Pet != nil {
		killer = pet.Pet.Owner
	}
	p, ok := s.players[killer]
	if !ok || p.Player == nil || p.Player.Char == nil {
		return
	}
	changed := p.Player.Char.Quests.RecordKill(s.quests, dead.Monster.TypeID)
	if len(changed) == 0 {
		return
	}
	p.Player.MarkDirty()
	s.pushQuestLog(p)
	s.log.Debug("任务击杀进度", "char", p.Player.Char.Name,
		"monster", dead.Monster.TypeID, "tasks", changed)
}

// npcOnMap 找当前场景里的 NPC。
//
// want 非空时只认那一个名字。这里故意遍历场景实体而不是 AOI：AOI 本身带空间范围，
// 用它会把已经删除的距离校验以另一种形式带回来。
func (s *Scene) npcOnMap(want string) string {
	for _, o := range s.entities {
		if o.Kind != domain.KindNPC {
			continue
		}
		if want == "" || o.Name == want {
			return o.Name
		}
	}
	return ""
}

// acceptReject 把 domain 的拒绝原因翻成对外事件的原因。
func acceptReject(r domain.AcceptReject) event.RejectReason {
	switch r {
	case domain.AcceptWrongNPC:
		return event.RejectNoTarget
	case domain.AcceptLevelTooLow:
		return event.RejectLevelTooLow
	case domain.AcceptAlreadyActive:
		return event.RejectQuestActive
	case domain.AcceptAlreadyDone:
		return event.RejectQuestDone
	case domain.AcceptPrerequisite:
		return event.RejectQuestNotDone
	}
	return event.RejectUnknown
}
