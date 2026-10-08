package domain

import (
	"fmt"
	"sort"
	"strings"
)

// 任务。
//
// 数据出处：
//
//	game_tasks              366 个任务（名字/NPC/地图/等级区间/交物目标）
//	game_task_rewards       经验/名誉/金银铜，依据任务奖励展示文本整理
//	game_task_reward_items  奖励物品，168 件全部经名称印证命中 ov_item/ov_arm
//
// ⚠️ **奖励的结构化数值在客户端里根本不存在**，只有一段给人看的文本
// （"雄鹰之翼；33000点经验值；5点名誉值；22银币"）。此前把 214 条经验的数字
// 逐个去客户端记录里搜，最高命中 3/214 —— 确实不在。所以只能从文本解，
// 而物品段落必须能在物品表里查到名字才算数，查不到的不猜。
//
// ⚠️ **`game_tasks.exp` 那一列不能用。** 全表只有 4 个取值（10000/20000/32000/8000），
// 是按等级段填的占位，不是每个任务的经验。真值在奖励文本里（225/366 解得出）。

// QuestID 是任务号（game_tasks.id）。
type QuestID int32

// QuestState 是一个任务在角色身上的状态。
type QuestState uint8

const (
	QuestActive   QuestState = iota // 进行中
	QuestDeliver                    // 条件够了，可以交付
	QuestFinished                   // 已完成
)

// Money 是三种面额的钱。
//
// 正式客户端背包与商店口径已闭合为 1 金=1000 银、1 银=1000 铜。
// 存档仍分三列，进入交易结算点时才规范化为统一铜币，避免各玩法重复换算。
type Money struct {
	Gold, Silver, Copper int64
}

// Add 把另一份钱加进来。
func (m Money) Add(o Money) Money {
	return Money{m.Gold + o.Gold, m.Silver + o.Silver, m.Copper + o.Copper}
}

// Empty 报告是不是一分钱都没有。
func (m Money) Empty() bool { return m.Gold == 0 && m.Silver == 0 && m.Copper == 0 }

// QuestReward 是完成任务给的东西。
type QuestReward struct {
	Exp   int64
	Honor int64
	Money Money
	Items []RewardItem
}

// QuestExchangeTier 是“交多少算多少”任务达到一个数量门槛后的额外奖励。
// 门槛按 MinQty 升序；结算时只取已达到的最高一档，不累计低档奖励。
type QuestExchangeTier struct {
	MinQty int32
	Reward QuestReward
	Title  string
}

// QuestExchangeItem 是按量收购任务中的一种可交付物品及其单件价格。
type QuestExchangeItem struct {
	Item       ItemID
	ItemName   string
	UnitReward QuestReward
}

// QuestExchangeRule 描述没有固定目标数量的兑换任务。玩家一次交付当前
// 背包中所有可收购物品，每种物品分别按单价结算，再按交付总数叠加最高档奖励。
type QuestExchangeRule struct {
	MinTotalQty int32
	Items       []QuestExchangeItem
	Tiers       []QuestExchangeTier
}

// TotalQty 返回当前背包中所有可收购物品的总数。
func (r *QuestExchangeRule) TotalQty(itemCount func(ItemID) int32) int32 {
	if r == nil {
		return 0
	}
	var total int32
	for _, item := range r.Items {
		total += countOf(itemCount, item.Item)
	}
	return total
}

func addScaledQuestReward(out *QuestReward, reward QuestReward, qty int32) {
	out.Exp += reward.Exp * int64(qty)
	out.Honor += reward.Honor * int64(qty)
	out.Money = out.Money.Add(Money{
		Gold:   reward.Money.Gold * int64(qty),
		Silver: reward.Money.Silver * int64(qty),
		Copper: reward.Money.Copper * int64(qty),
	})
	for _, item := range reward.Items {
		out.Items = append(out.Items, RewardItem{Item: item.Item, Qty: item.Qty * qty})
	}
}

// ExchangeReward 返回当前可交付物品、完整奖励、命中称号与交付总数。
func (r *QuestExchangeRule) ExchangeReward(itemCount func(ItemID) int32) (
	QuestReward, []QuestItemGoal, string, int32, bool,
) {
	if r == nil || r.MinTotalQty <= 0 || len(r.Items) == 0 {
		return QuestReward{}, nil, "", 0, false
	}
	var out QuestReward
	take := make([]QuestItemGoal, 0, len(r.Items))
	var total int32
	for _, item := range r.Items {
		qty := countOf(itemCount, item.Item)
		if item.Item == 0 || item.ItemName == "" || qty <= 0 {
			continue
		}
		take = append(take, QuestItemGoal{Item: item.Item, Qty: qty, Name: item.ItemName})
		total += qty
		addScaledQuestReward(&out, item.UnitReward, qty)
	}
	if total < r.MinTotalQty {
		return QuestReward{}, nil, "", total, false
	}
	var selected *QuestExchangeTier
	for _, tier := range r.Tiers {
		if total < tier.MinQty {
			break
		}
		copy := tier
		selected = &copy
	}
	if selected == nil {
		return out, take, "", total, true
	}
	out.Exp += selected.Reward.Exp
	out.Honor += selected.Reward.Honor
	out.Money = out.Money.Add(selected.Reward.Money)
	out.Items = append(out.Items, selected.Reward.Items...)
	return out, take, selected.Title, total, true
}

// RewardItem 是一件奖励物品。
type RewardItem struct {
	Item ItemID
	Qty  int32
}

// QuestEquipmentReward 是按建角职业路线发放的一件装备实例奖励。
type QuestEquipmentReward struct {
	Item        ItemID
	RefineLevel int32
}

// QuestStepKind 是一步任务的触发方式。字符串与数据库任务步骤的 kind 完全一致；
// Objective 只给 ov_task 的扁平目标兜底，Unknown 用来 fail-closed。
type QuestStepKind string

const (
	QuestStepUnknown   QuestStepKind = "unknown"
	QuestStepObjective QuestStepKind = "objective"
	QuestStepTalk      QuestStepKind = "talk"
	QuestStepKill      QuestStepKind = "kill"
	QuestStepCollect   QuestStepKind = "collect"
	QuestStepDeliver   QuestStepKind = "deliver"
	QuestStepExchange  QuestStepKind = "exchange"
	QuestStepTurnIn    QuestStepKind = "turnin"
)

// QuestItemGoal 是一步里要检查、扣除或给予的物品。
type QuestItemGoal struct {
	Item ItemID
	Qty  int32
	Name string
	// Sources 是“仅在该任务当前步骤进行时，由这些怪物直接放入背包”的
	// 任务专属来源。空切片表示这件物品必须来自接取给予、前一步 NPC 给予，
	// 或普通玩法，不能凭任务击杀自动生成。
	Sources []MonsterID
}

// QuestDropTier 是任务专属掉落的可审计分档。概率最终仍由 RatePct 决定；
// 分档保留下来，是为了让启动校验能阻止“唯一怪却不是必掉”或“大量收集却
// 配成低掉率”这类数据漂移。
type QuestDropTier string

const (
	QuestDropGuaranteed QuestDropTier = "guaranteed"
	QuestDropHigh       QuestDropTier = "high"
	QuestDropMedium     QuestDropTier = "medium"
	QuestDropLow        QuestDropTier = "low"
)

// QuestDropRule 表示当前步骤击杀一种怪物时，任务专属物品组的总掉率。
// 一次命中后只会从尚未收齐的匹配物品中选一件，不会逐项独立掉落。
type QuestDropRule struct {
	Monster MonsterID
	RatePct int32
	Tier    QuestDropTier
	Reason  string
}

// QuestMonsterGoal 是一步里的击杀目标。
type QuestMonsterGoal struct {
	Monster MonsterID
	Qty     int32
	Name    string
}

// QuestStep 是客户端结构化任务流程的一步。
//
// Collect 与 Take 都是完成这一步时要从背包扣掉的物品：Collect 表示这一步负责
// 收集，Take 表示玩家此前已拿到、现在交付。Give 是完成中间步骤后发给玩家、供
// 后续步骤继续使用的任务物品。
type QuestStep struct {
	Kind QuestStepKind
	To   string
	Text string
	Say  string

	Collect []QuestItemGoal
	Kill    []QuestMonsterGoal
	Take    []QuestItemGoal
	Give    []QuestItemGoal
	Drops   []QuestDropRule
}

// DropRule 返回这种怪在当前步骤的任务专属物品组规则。
func (s QuestStep) DropRule(monster MonsterID) (QuestDropRule, bool) {
	for _, rule := range s.Drops {
		if rule.Monster == monster {
			return rule, true
		}
	}
	return QuestDropRule{}, false
}

// Known 报告这一步能否由当前运行时权威判定。
func (s QuestStep) Known() bool {
	switch s.Kind {
	case QuestStepObjective, QuestStepTalk, QuestStepKill, QuestStepCollect,
		QuestStepDeliver, QuestStepExchange, QuestStepTurnIn:
		return true
	default:
		return false
	}
}

// RequiredItems 返回完成这一步时既要检查又要扣除的全部物品目标。
func (s QuestStep) RequiredItems() []QuestItemGoal {
	out := make([]QuestItemGoal, 0, len(s.Collect)+len(s.Take))
	out = append(out, s.Collect...)
	out = append(out, s.Take...)
	return out
}

// QuestDef 是一个任务的定义。
type QuestDef struct {
	ID   QuestID
	Name string
	// NPC 是发布任务的 NPC **名字**。game_tasks 里存的就是名字不是 id，
	// 所以匹配也按名字来（1736 个 NPC 里 130/160 个任务 NPC 能对上）。
	NPC string
	Map string

	LevelMin int32
	// LevelMax 是客户端展示用的推荐等级上界，不是接取上限。
	LevelMax int32
	// Prerequisites 是全部前置任务。有多个时是 AND 关系：必须全部完成。
	Prerequisites []QuestID
	// GoalItem / GoalQty 是交物目标。0 表示这个任务不用交东西
	// （366 个里 139 个有交物目标，其余是纯对话任务）。
	GoalItem ItemID
	GoalQty  int32

	// AcceptGive 是接任务当场发的任务物品；Steps 是权威流程。由 ov_task 的
	// 5×物品/5×怪物槽构造兜底，再由数据库中的分步定义覆盖。
	AcceptGive []QuestItemGoal
	Steps      []QuestStep

	Repeatable bool
	Reward     QuestReward
	// 使用 Character.Race（建角路线），不使用就职前统一为初行者的 ClientRace。
	ProfessionRewards map[Race]QuestEquipmentReward
	// Exchange 非 nil 时，本任务按当前持有数量兑换，不使用固定 GoalQty/Reward 结算。
	Exchange *QuestExchangeRule

	// Desc / RewardText 是给玩家看的原文, 直接来自 game_tasks 的 descr/reward 列。
	// 客户端的任务对话框要它们 —— 只给数值的话玩家看到的是一条没有正文的任务。
	Desc       string
	RewardText string
}

// NeedsItem 报告这是不是交物任务。
func (d QuestDef) NeedsItem() bool { return d.GoalItem != 0 && d.GoalQty > 0 }

// QuestTable 是全部任务定义。
type QuestTable map[QuestID]QuestDef

// ByNPC 返回某个 NPC 发布的全部任务。
func (t QuestTable) ByNPC(npc string) []QuestDef {
	var out []QuestDef
	for _, d := range t {
		if d.NPC == npc {
			out = append(out, d)
		}
	}
	return out
}

// QuestEntry 是一个任务在角色身上的进度。
type QuestEntry struct {
	State QuestState
	// Progress 是当前步骤下标。旧实现从未推进这个字段，已有存档均为 0，
	// 因此可以无损升级为步骤游标。
	Progress int32
	// Kills 是当前步骤按怪物类型累计的击杀数。进入下一步时清空。
	Kills map[MonsterID]int32
}

// QuestLog 是一个角色的任务本：任务号 → 进度。
//
// **已完成的也留着** —— 不留就没法拦住"同一个非重复任务再接一次"。
type QuestLog map[QuestID]QuestEntry

// Clone 做一份独立副本。存档时用。
func (l QuestLog) Clone() QuestLog {
	if l == nil {
		return nil
	}
	out := make(QuestLog, len(l))
	for k, v := range l {
		if v.Kills != nil {
			v.Kills = cloneKillProgress(v.Kills)
		}
		out[k] = v
	}
	return out
}

func cloneKillProgress(in map[MonsterID]int32) map[MonsterID]int32 {
	if in == nil {
		return nil
	}
	out := make(map[MonsterID]int32, len(in))
	for id, n := range in {
		out[id] = n
	}
	return out
}

// AcceptReject 是接任务被拒的原因。
type AcceptReject uint8

const (
	AcceptOK AcceptReject = iota
	AcceptUnknownQuest
	AcceptWrongNPC
	AcceptLevelTooLow
	AcceptAlreadyActive
	AcceptAlreadyDone
	AcceptPrerequisite
)

// CanAccept 判断角色能不能接这个任务。
func (l QuestLog) CanAccept(d QuestDef, level int32, npc string) AcceptReject {
	if npc != "" && d.NPC != npc {
		return AcceptWrongNPC
	}
	if level < d.LevelMin {
		return AcceptLevelTooLow
	}
	for _, prerequisite := range d.Prerequisites {
		if prerequisite != 0 && !l.Done(prerequisite) {
			return AcceptPrerequisite
		}
	}
	switch e, ok := l[d.ID]; {
	case !ok:
		return AcceptOK
	case e.State != QuestFinished:
		return AcceptAlreadyActive
	case !d.Repeatable:
		return AcceptAlreadyDone
	}
	return AcceptOK
}

// Accept 把任务记进任务本。调用方先用 CanAccept 校验。
func (l QuestLog) Accept(d QuestDef) {
	l[d.ID] = QuestEntry{State: QuestActive, Kills: map[MonsterID]int32{}}
}

// Deliverable 报告这个任务现在能不能交。
//
// 交物任务看背包里够不够 —— **进度不单独记账**。
// 记账的话就要在每次背包变动时同步, 少同步一处就会出现"东西没了任务还显示能交"。
// 直接问背包永远不会不同步。
func (l QuestLog) Deliverable(d QuestDef, bagCount int32) bool {
	e, ok := l[d.ID]
	if !ok || e.State == QuestFinished {
		return false
	}
	if !d.NeedsItem() {
		return true // 纯对话任务, 接了就能交
	}
	return bagCount >= d.GoalQty
}

// CurrentStep 返回任务当前进行到的结构化步骤。没有 Steps 表示调用方构造的是
// 旧式单目标定义，继续走 Deliverable 兼容路径。
func (l QuestLog) CurrentStep(d QuestDef) (QuestStep, bool) {
	e, ok := l[d.ID]
	if !ok || e.State == QuestFinished || len(d.Steps) == 0 || e.Progress < 0 || int(e.Progress) >= len(d.Steps) {
		return QuestStep{}, false
	}
	return d.Steps[e.Progress], true
}

// StepTarget 返回当前应该交互的 NPC。旧式定义仍返回发布者。
func (l QuestLog) StepTarget(d QuestDef) string {
	if step, ok := l.CurrentStep(d); ok && step.To != "" {
		return step.To
	}
	return d.NPC
}

// DeliverableAt 报告当前步骤是否已达到提交条件，并校验正在对话的 NPC。
// itemCount 直接读取当前背包，避免物品进度与真实持有量脱节。
func (l QuestLog) DeliverableAt(d QuestDef, npc string, itemCount func(ItemID) int32) bool {
	if len(d.Steps) == 0 {
		if npc != "" && npc != d.NPC {
			return false
		}
		return l.Deliverable(d, countOf(itemCount, d.GoalItem))
	}
	e, ok := l[d.ID]
	if !ok || e.State == QuestFinished {
		return false
	}
	step, ok := l.CurrentStep(d)
	if !ok || !step.Known() || step.To == "" || (npc != "" && npc != step.To) {
		return false
	}
	if d.Exchange != nil {
		return step.Kind == QuestStepExchange && d.Exchange.MinTotalQty > 0 &&
			d.Exchange.TotalQty(itemCount) >= d.Exchange.MinTotalQty
	}
	for _, goal := range step.RequiredItems() {
		if goal.Item == 0 || goal.Qty <= 0 || countOf(itemCount, goal.Item) < goal.Qty {
			return false
		}
	}
	for _, goal := range step.Kill {
		if goal.Monster == 0 || goal.Qty <= 0 || e.Kills[goal.Monster] < goal.Qty {
			return false
		}
	}
	// kill/collect/objective 没有任何可判定目标通常意味着数据残缺；不能降级成
	// “对话即完成”。talk/deliver/turnin 则允许没有数值目标。
	switch step.Kind {
	case QuestStepKill:
		return len(step.Kill) > 0
	case QuestStepCollect:
		return len(step.Collect) > 0
	case QuestStepObjective:
		return len(step.Kill)+len(step.Collect)+len(step.Take) > 0
	}
	return true
}

// RecordKill 把一次真实怪物死亡计入所有关心该怪物的活跃任务。
func (l QuestLog) RecordKill(all QuestTable, monster MonsterID) []QuestID {
	var changed []QuestID
	for id, entry := range l {
		if entry.State == QuestFinished {
			continue
		}
		def, ok := all[id]
		if !ok || len(def.Steps) == 0 || entry.Progress < 0 || int(entry.Progress) >= len(def.Steps) {
			continue
		}
		step := def.Steps[entry.Progress]
		for _, goal := range step.Kill {
			if goal.Monster != monster || goal.Qty <= 0 || entry.Kills[monster] >= goal.Qty {
				continue
			}
			if entry.Kills == nil {
				entry.Kills = map[MonsterID]int32{}
			}
			entry.Kills[monster]++
			l[id] = entry
			changed = append(changed, id)
			break
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i] < changed[j] })
	return changed
}

// Advance 完成当前步骤。返回 true 表示已经越过最后一步，应当结算任务。
func (l QuestLog) Advance(d QuestDef) bool {
	e, ok := l[d.ID]
	if !ok || e.State == QuestFinished || len(d.Steps) == 0 {
		return false
	}
	if int(e.Progress)+1 >= len(d.Steps) {
		return true
	}
	e.Progress++
	e.Kills = map[MonsterID]int32{}
	l[d.ID] = e
	return false
}

// Finish 把任务标成已完成。
func (l QuestLog) Finish(id QuestID) {
	e := l[id]
	e.State = QuestFinished
	e.Progress = 0
	e.Kills = nil
	l[id] = e
}

// Abandon 放弃一个进行中的任务：整条记录删掉，之后可以重新接。
//
// **不能标成已完成** —— 标完成的话，一个不可重复任务就白白少了一次机会，
// 而且 NPC 头顶会显示成"做过了"。放弃的语义是"当没接过"。
//
// 已经完成的任务放弃不了：那条记录是"做过"的唯一证据，
// 删掉就等于把不可重复任务的奖励变成可以反复领。
func (l QuestLog) Abandon(id QuestID) bool {
	e, ok := l[id]
	if !ok || e.State == QuestFinished {
		return false
	}
	delete(l, id)
	return true
}

// Active 报告某个任务是不是进行中。
func (l QuestLog) Active(id QuestID) bool {
	e, ok := l[id]
	return ok && e.State != QuestFinished
}

// Done 报告某个任务是不是已完成过。
func (l QuestLog) Done(id QuestID) bool {
	e, ok := l[id]
	return ok && e.State == QuestFinished
}

// NPCSpawn 是一个 NPC 在地图上的落位。
//
// NPC 在场景里是**实体**, 不是一批静态包。它们不动、不打人、不会死,
// 但要能被看见、被靠近 —— 接任务得判"你站在那个 NPC 面前"。
type NPCSpawn struct {
	Name     string
	Sprite   int32
	Pos      Pos
	Dir      int32
	Sell     int32
	Trans    int32
	Portrait string // 已核实的对话立绘资源名
	// Script 对话脚本号。客户端的 NPC 落位包要它 ——
	// 没有它 NPC 站在那儿但点不出对话。
	Script string
	// Role/Greeting 是服务端下发给 NpcChatter 的稳定环境闲聊配置。
	Role     NPCRole
	Greeting string
}

// OfferState 是一个任务在**某个玩家眼里**的状态, 决定 NPC 对话框里那一条长什么样。
type OfferState uint8

const (
	OfferAvailable OfferState = iota // 可以接
	OfferActive                      // 已接, 条件还没达成
	OfferReady                       // 已接, 条件达成, 可以交了
)

// Offer 是 NPC 对某个玩家展示的一条任务。字段与客户端的 `Offer` 类一一对应。
type Offer struct {
	ID    QuestID
	State OfferState
	Title string
	Desc  string
	Award string
}

// OffersOf 列出某个 NPC 对这个玩家能展示的全部任务。
//
// 三类都要列, 不能只列"可接的":
//   - 可接的     不列玩家就不知道这儿有任务
//   - 进行中的   不列玩家找不到回来交任务的地方
//   - 可交付的   同上, 而且这条最要紧
//
// 只列可接的那种做法, 表现是"接完任务 NPC 就没反应了"。
//
// bagCount 是"玩家背包里有多少个该任务要交的物品", 由调用方查 ——
// 领域层不认识背包的存储方式。
func OffersOf(all QuestTable, npc string, ch *Character, bagCount func(ItemID) int32) []Offer {
	if npc == "" || ch == nil {
		return nil
	}
	var out []Offer
	for _, d := range all {
		e, has := ch.Quests[d.ID]
		switch {
		case has && e.State != QuestFinished:
			// 已接任务挂在当前步骤的目标 NPC 身上，不再永远挂回发布者。
			if ch.Quests.StepTarget(d) != npc {
				continue
			}
			st := OfferActive
			if ch.Quests.DeliverableAt(d, npc, bagCount) {
				st = OfferReady
			}
			out = append(out, offerOf(d, st))
		case d.NPC == npc && ch.AllowsCareerQuest(d.ID) && ch.Quests.CanAccept(d, ch.Level, npc) == AcceptOK:
			out = append(out, offerOf(d, OfferAvailable))
		}
	}
	// **按任务号排序。** map 的遍历顺序每次都不一样, 不排的话玩家每次打开
	// 同一个 NPC 看到的顺序都在变。
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func countOf(f func(ItemID) int32, it ItemID) int32 {
	if f == nil || it == 0 {
		return 0
	}
	return f(it)
}

func offerOf(d QuestDef, st OfferState) Offer {
	return Offer{ID: d.ID, State: st, Title: d.Name, Desc: d.Desc, Award: d.RewardText}
}

// ActiveQuest 是任务本里一条**进行中**的任务, 字段与客户端的 `ActiveTask` 类
// (id/remainSec/step/total/text/progress/to/ready)一一对应。
type ActiveQuest struct {
	ID        QuestID
	RemainSec int32  // 限时任务的剩余秒数; 0 = 不限时
	Step      int32  // 当前剧情步骤下标（从 0 开始）
	Total     int32  // 剧情总步骤数
	Text      string // 任务正文
	Progress  string // 进度描述, 界面上跟在正文后面
	To        string // 交给谁
	Ready     bool   // 条件达成, 可以交了
}

// ActiveQuestsOf 列出角色任务本里全部进行中的任务。
//
// **不含已完成的** —— 那是另一段(客户端的包里第二段是一串任务号)。
//
// bagCount 由调用方查: 领域层不认识背包的存储方式。
func ActiveQuestsOf(all QuestTable, ch *Character, bagCount func(ItemID) int32) []ActiveQuest {
	if ch == nil {
		return nil
	}
	var out []ActiveQuest
	for id, e := range ch.Quests {
		if e.State == QuestFinished {
			continue
		}
		d, ok := all[id]
		if !ok {
			continue // 任务表换了版本, 旧存档里可能有已经不存在的任务号
		}
		q := activeQuestOf(d, e, ch.Quests, bagCount)
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func activeQuestOf(d QuestDef, e QuestEntry, log QuestLog, itemCount func(ItemID) int32) ActiveQuest {
	if len(d.Steps) == 0 {
		have := countOf(itemCount, d.GoalItem)
		// 旧式定义在客户端状态机里按单步任务表达。step/total 是剧情步骤，
		// 不能塞目标数量；客户端用 step >= total-1 区分“对话”和“领取奖励”。
		q := ActiveQuest{ID: d.ID, Total: 1, Text: d.Desc, To: d.NPC,
			Ready: log.Deliverable(d, have)}
		if d.NeedsItem() {
			if have > d.GoalQty {
				have = d.GoalQty
			}
			q.Progress = questItemProgress("", have, d.GoalQty)
		}
		return q
	}

	step, ok := log.CurrentStep(d)
	if !ok {
		return ActiveQuest{ID: d.ID, Text: d.Desc, To: d.NPC}
	}
	q := ActiveQuest{ID: d.ID, Step: e.Progress, Total: int32(len(d.Steps)),
		Text: step.Text, To: step.To,
		Ready: log.DeliverableAt(d, step.To, itemCount)}
	if q.Text == "" {
		q.Text = d.Desc
	}
	if d.Exchange != nil {
		progress := make([]string, 0, len(d.Exchange.Items))
		for _, item := range d.Exchange.Items {
			progress = append(progress, fmt.Sprintf("已获得%s%d个", item.ItemName,
				countOf(itemCount, item.Item)))
		}
		q.Progress = strings.Join(progress, "\n")
		return q
	}
	progress := make([]string, 0, len(step.RequiredItems())+len(step.Kill))
	for _, goal := range step.RequiredItems() {
		have := countOf(itemCount, goal.Item)
		if have > goal.Qty {
			have = goal.Qty
		}
		progress = append(progress, questItemProgress(goal.Name, have, goal.Qty))
	}
	for _, goal := range step.Kill {
		have := e.Kills[goal.Monster]
		if have > goal.Qty {
			have = goal.Qty
		}
		progress = append(progress, questGoalProgress("已消灭", goal.Name, have, goal.Qty))
	}
	// progress 是客户端直接展示的目标文本，不是给服务端看的步骤游标。
	// 目标必须逐项带名字；把多目标合并成裸 "2/10" 会丢掉玩家究竟在打谁、
	// 收集什么。纯对话步骤没有数量目标，保持为空，正文和 To 已足够表达当前步骤。
	q.Progress = strings.Join(progress, "\n")
	return q
}

func questItemProgress(name string, have, total int32) string {
	return fmt.Sprintf("已获得%s%d/%d个", name, have, total)
}

func questGoalProgress(prefix, name string, have, total int32) string {
	label := prefix + name
	if label == "" {
		return fmt.Sprintf("%d/%d", have, total)
	}
	return fmt.Sprintf("%s %d/%d", label, have, total)
}

// FinishedIDs 返回全部已完成的任务号(升序)。
//
// 客户端的 `TaskClient.Done` 就是它 —— 拿来判断 NPC 头顶显示什么、
// 任务能不能再接。升序是因为 map 遍历顺序每次都不一样, 而这份列表会被整份下发,
// 不排的话每次发出去的字节都不同, 抓包对比时全是噪声。
func (l QuestLog) FinishedIDs() []int32 {
	var out []int32
	for id, e := range l {
		if e.State == QuestFinished {
			out = append(out, int32(id))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
