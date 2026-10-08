// Package entity 定义场景里的实体。
//
// 生命周期约定: 一个 Entity 从进场景到离场景, **只被它所在场景的那个 goroutine 读写**。
// 因此本包所有方法都不加锁, 也不允许把 *Entity 交给场景外的任何人 ——
// 要往外传就转成 id 或值拷贝(见 docs/架构/00-不可逆决策.md 第五条)。
package entity

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Entity 是场景里一个可见的东西: 玩家、怪物、NPC、地上的掉落物。
//
// 用一个结构体带可选分支, 而不是接口 —— 四种实体的共有部分(位置/血量/外观)占了九成,
// 且每帧都要遍历全部实体, 接口派发在这里只有开销没有收益。
type Entity struct {
	ID    domain.EntityID
	Kind  domain.EntityKind
	Name  string
	Level int32
	HP    int32
	MaxHP int32
	// MP 当前法力。与 HP 一样**不落盘** —— 新在线生命周期补满，跨场景保留。
	// 存盘意义不大(下线回满是常规做法), 存了反而要处理"上限变小了怎么办"。
	MP    int32
	MaxMP int32
	Pos   domain.Pos
	Dir   uint8
	Look  domain.Look
	Stats domain.Stats
	// Status 是身上的 buff/debuff。玩家和怪都有。
	Status *domain.StatusSet

	// 按 Kind 分支, 其余为 nil。取用前先判 Kind, 不要判 nil ——
	// 判 Kind 的代码在读的时候意图明确。
	Player  *Player
	Monster *Monster
	Drop    *Drop
	Pet     *Pet
	Trap    *Trap
	// NPC 是 NPC 专有的展示数据(对话脚本、立绘资源名)。
	// 客户端的 NPC 落位包(0x8046)要它, 普通实体的出场包给不了这些字段。
	NPC *NPCInfo
}

// NPCInfo 是 NPC 的展示数据。
type NPCInfo struct {
	Sprite   string
	Script   string
	Portrait string
	Label    string
	Sell     int32
	Trans    int32
	Role     domain.NPCRole
	Greeting string
}

// Pet 是宠物实体的专有部分。
//
// 宠物既不是玩家也不是怪：不占连接、不由刷怪器管、死了不重生、跟着主人走。
// 混进 monsters 那张表的话，stepAI 会顺手把它当敌人索敌。
type Pet struct {
	// Owner 是主人的实体 id。主人离场时宠物跟着一起摘除 ——
	// 留在场上就成了一只没人管、谁都打不过的野怪。
	Owner domain.EntityID
	// Inst 是那只宠的权威数据。**指向玩家身上那一份**，不是拷贝 ——
	// 拷贝的话打怪涨的经验会随召回一起丢掉。
	Inst *domain.PetInstance
	// Def 是种族定义。拷进来免得每帧回查配置表。
	Def domain.PetDef

	// Target/nextAt 仅保留旧测试夹具的结构兼容；生产运行路径从不读写它们。
	// 辅助宠物不索敌、不攻击，也不进入技能结算。
	Target domain.EntityID
	nextAt domain.Tick

	// NextHungerAt/NextTrustAt 只在宠物出战实体存在期间推进。召回后实体被摘除，
	// 因而离场时间不会改变饥饿或信赖。ZeroTrustRecallAt 是信赖为 0 时
	// 留给玩家喂无尽淳的出战宽限截止帧；信赖恢复后会取消。
	NextHungerAt      domain.Tick
	NextTrustAt       domain.Tick
	ZeroTrustRecallAt domain.Tick
	// NextPetSkillAt 是回魂这类持续主动技能的下一次结算时刻。
	NextPetSkillAt domain.Tick
	TrustBand      uint8

	// NextStarveAt 下次改饥渴度的帧。
	//
	// **换状态时要重排**(见 scene 里的 retimeStarve): 打架档和跟随档速率不同,
	// 不重排的话在两个状态之间反复横跳可以一直把计时器推后, 变成永远不饿。
	NextStarveAt domain.Tick
	// StarveState 上一帧算出来的状态, 用来发现状态变了。
	StarveState domain.PetState
	// BagWarned 已经因为背包满报过一次了。**不报第二次** ——
	// 宠物每帧都会去够那件捡不起来的东西, 不去重的话日志和客户端都会被刷爆。
	BagWarned bool
}

// Drop 是地上一件掉落物的专有部分。
type Drop struct {
	Item  domain.ItemID
	Count int32
	// Stack 非空表示这是玩家从背包丢出的真实实例。拾取时必须原样恢复 UID、
	// 耐久与绑定状态，不能按模板重新创建一件满耐久装备。
	Stack domain.Stack
	// Owner 是有优先拾取权的人(打死怪的那个)。0 = 谁都能捡。
	Owner domain.EntityID
	// OwnerUntil 是归属保护到期的帧。过了之后谁都能捡。
	OwnerUntil domain.Tick
}

// Trap 是刺客放置在场景中的一次性陷阱。
//
// Def 保存施法时已经套用被动修正后的技能副本；触发时不再回读玩家当前技能
// 等级。VisibleTo 是服务端的发现事实：主人天然可见，其他玩家只有侦测成功后
// 才会收到 0x800f/SpawnTrap。
type Trap struct {
	Owner             domain.EntityID
	Def               domain.SkillDef
	Radius            int32
	StatusDurationPct int32
	VisibleTo         map[domain.EntityID]struct{}
}

func (t *Trap) VisibleToPlayer(player domain.EntityID) bool {
	if t == nil {
		return false
	}
	if t.Owner == player {
		return true
	}
	_, ok := t.VisibleTo[player]
	return ok
}

// Alive 报告实体是否存活。掉落物与 NPC 恒为 true(它们不参与生死)。
func (e *Entity) Alive() bool {
	switch e.Kind {
	case domain.KindPlayer, domain.KindMonster, domain.KindPet:
		return e.HP > 0
	}
	return true
}

// Spawned 生成本实体的出场事件。给别人看的那份, 不含任何私有数据。
func (e *Entity) Spawned() event.EntitySpawned {
	ev := event.EntitySpawned{
		ID: e.ID, Kind: e.Kind, Name: e.Name, Level: e.Level,
		HP: e.HP, MaxHP: e.MaxHP, Pos: e.Pos, Dir: e.Dir, Look: e.Look,
	}
	if e.Kind == domain.KindPlayer && e.Player != nil {
		ev.Riding = e.Player.Riding
		ev.MountModel = e.Player.MountModel
	}
	if e.Kind == domain.KindPet && e.Pet != nil {
		ev.Owner = e.Pet.Owner
	}
	if e.Kind == domain.KindTrap && e.Trap != nil {
		ev.Owner = e.Trap.Owner
		ev.Trap = &event.TrapView{Skill: e.Trap.Def.ID, Radius: e.Trap.Radius}
	}
	if e.Kind == domain.KindMonster && e.Monster != nil {
		ev.Elite = e.Monster.EliteRing
	}
	if e.NPC != nil {
		ev.NPC = &event.NPCView{
			Sprite: e.NPC.Sprite, Script: e.NPC.Script,
			Portrait: e.NPC.Portrait, Label: e.NPC.Label,
			Sell: e.NPC.Sell, Trans: e.NPC.Trans,
		}
	}
	return ev
}

// SnapshotSaver 是玩家完整生命周期结束时的最终存档出口。
// 实现只允许排队并立即返回；数据库提交确认由实现自己异步处理。
type SnapshotSaver interface {
	Save(domain.Snapshot)
}

// PlayerRuntime 是玩家跨场景时必须随所有权一起交接的非持久化运行态。
//
// 状态里的 deadline 仍属于 FromTick 所在场景的时间轴；恢复时必须重基准。
// 攻击冷却与复活保护则直接保存剩余帧，避免目标场景 Tick 不同导致绕过冷却
// 或丢掉保护期。打工在跨图前停止；出战宠物则把完整运行态一起交接。
type PlayerRuntime struct {
	HP, MP         int32
	Dir            uint8
	Status         *domain.StatusSet
	FromTick       domain.Tick
	AttackReadyIn  domain.Tick
	SkillReadyIn   map[domain.SkillID]domain.Tick
	ItemReadyIn    map[int32]domain.Tick
	ProtectedFor   domain.Tick
	NianliReadyIn  domain.Tick
	GMGod          bool
	GMOneShot      bool
	Pet            *PetRuntime
	Riding         bool
	MountModel     int32
	RideEpoch      uint64
	DeathLoss      domain.DeathLoss
	ExpBonusPct    int32
	ExpBonusFor    domain.Tick
	PetExpBonusPct int32
	PetExpBonusFor domain.Tick
}

// PetRuntime 是出战宠物跨场景必须无损交接的运行态。各 deadline
// 保存为剩余帧，避免目标场景使用不同 Tick 时重置饥饿/信赖进度。
type PetRuntime struct {
	Inst               domain.PetInstID
	HP, MP             int32
	Dir                uint8
	Status             *domain.StatusSet
	FromTick           domain.Tick
	AttackReadyIn      domain.Tick
	HungerReadyIn      domain.Tick
	TrustReadyIn       domain.Tick
	ZeroTrustReadyIn   domain.Tick
	PetSkillReadyIn    domain.Tick
	StarveReadyIn      domain.Tick
	HungerScheduled    bool
	TrustScheduled     bool
	ZeroTrustScheduled bool
	PetSkillScheduled  bool
	StarveScheduled    bool
	Target             domain.EntityID
	TrustBand          uint8
	StarveState        domain.PetState
	BagWarned          bool
}

// CapturePlayerRuntime 取得一份可跨场景传递的玩家运行态值。
func (e *Entity) CapturePlayerRuntime(now domain.Tick) PlayerRuntime {
	if e == nil {
		return PlayerRuntime{FromTick: now}
	}
	r := PlayerRuntime{
		HP:       e.HP,
		MP:       e.MP,
		Dir:      e.Dir,
		Status:   e.Status.Clone(),
		FromTick: now,
	}
	if e.Player != nil {
		r.AttackReadyIn = remainingTicks(e.Player.nextAt, now)
		r.SkillReadyIn = remainingSkillCooldowns(e.Player.skillReady, now)
		r.ItemReadyIn = remainingItemCooldowns(e.Player.itemReady, now)
		r.ProtectedFor = remainingTicks(e.Player.safeUntil, now)
		r.GMGod = e.Player.GMGod
		r.GMOneShot = e.Player.GMOneShot
		r.Riding = e.Player.Riding
		r.MountModel = e.Player.MountModel
		r.RideEpoch = e.Player.RideEpoch
		r.DeathLoss = e.Player.DeathLoss
		r.NianliReadyIn = remainingTicks(e.Player.nextNianliAt, now)
		r.ExpBonusPct = e.Player.ExperienceBonusPct(now)
		r.ExpBonusFor = remainingTicks(e.Player.expBonusUntil, now)
		r.PetExpBonusPct = e.Player.PetExperienceBonusPct(now)
		r.PetExpBonusFor = remainingTicks(e.Player.petExpBonusUntil, now)
	}
	return r
}

// RestorePlayerRuntime 把跨场景运行态安装到已经算好属性上限的新实体。
func (e *Entity) RestorePlayerRuntime(r PlayerRuntime, now domain.Tick) {
	if e == nil {
		return
	}
	e.HP = clampCurrent(r.HP, e.MaxHP)
	e.MP = clampCurrent(r.MP, e.MaxMP)
	e.Dir = r.Dir
	if r.Status != nil {
		e.Status = r.Status.Clone()
	} else {
		e.Status = domain.NewStatusSet()
	}
	if e.Status != nil {
		e.Status.Rebase(r.FromTick, now)
	}
	if e.Player != nil {
		e.Player.nextAt = now + r.AttackReadyIn
		e.Player.skillReady = restoreSkillCooldowns(r.SkillReadyIn, now)
		e.Player.itemReady = restoreItemCooldowns(r.ItemReadyIn, now)
		e.Player.safeUntil = now + r.ProtectedFor
		e.Player.GMGod = r.GMGod
		e.Player.GMOneShot = r.GMOneShot
		e.Player.Riding = r.Riding
		e.Player.MountModel = r.MountModel
		e.Player.RideEpoch = r.RideEpoch
		e.Player.DeathLoss = r.DeathLoss
		e.Player.nextNianliAt = now + r.NianliReadyIn
		e.Player.expBonusPct = r.ExpBonusPct
		e.Player.expBonusUntil = now + r.ExpBonusFor
		e.Player.petExpBonusPct = r.PetExpBonusPct
		e.Player.petExpBonusUntil = now + r.PetExpBonusFor
	}
}

// CapturePetRuntime 在源场景摘除宠物实体前捕获所有可变运行态。
func (e *Entity) CapturePetRuntime(now domain.Tick) *PetRuntime {
	if e == nil || e.Pet == nil || e.Pet.Inst == nil {
		return nil
	}
	return &PetRuntime{
		Inst:               e.Pet.Inst.ID,
		HP:                 e.HP,
		MP:                 e.MP,
		Dir:                e.Dir,
		Status:             e.Status.Clone(),
		FromTick:           now,
		AttackReadyIn:      remainingTicks(e.Pet.nextAt, now),
		HungerReadyIn:      remainingTicks(e.Pet.NextHungerAt, now),
		TrustReadyIn:       remainingTicks(e.Pet.NextTrustAt, now),
		ZeroTrustReadyIn:   remainingTicks(e.Pet.ZeroTrustRecallAt, now),
		PetSkillReadyIn:    remainingTicks(e.Pet.NextPetSkillAt, now),
		StarveReadyIn:      remainingTicks(e.Pet.NextStarveAt, now),
		HungerScheduled:    e.Pet.NextHungerAt != 0,
		TrustScheduled:     e.Pet.NextTrustAt != 0,
		ZeroTrustScheduled: e.Pet.ZeroTrustRecallAt != 0,
		PetSkillScheduled:  e.Pet.NextPetSkillAt != 0,
		StarveScheduled:    e.Pet.NextStarveAt != 0,
		Target:             e.Pet.Target,
		TrustBand:          e.Pet.TrustBand,
		StarveState:        e.Pet.StarveState,
		BagWarned:          e.Pet.BagWarned,
	}
}

// RestorePetRuntime 把交接值安装到目标场景新分配的宠物实体上。
func (e *Entity) RestorePetRuntime(r PetRuntime, now domain.Tick) {
	if e == nil || e.Pet == nil {
		return
	}
	e.HP = clampCurrent(r.HP, e.MaxHP)
	e.MP = clampCurrent(r.MP, e.MaxMP)
	e.Dir = r.Dir
	e.Status = r.Status.Clone()
	if e.Status != nil {
		e.Status.Rebase(r.FromTick, now)
	}
	e.Pet.nextAt = now + r.AttackReadyIn
	e.Pet.NextHungerAt = rebaseOptionalDeadline(now, r.HungerReadyIn, r.HungerScheduled)
	e.Pet.NextTrustAt = rebaseOptionalDeadline(now, r.TrustReadyIn, r.TrustScheduled)
	e.Pet.ZeroTrustRecallAt = rebaseOptionalDeadline(now, r.ZeroTrustReadyIn, r.ZeroTrustScheduled)
	e.Pet.NextPetSkillAt = rebaseOptionalDeadline(now, r.PetSkillReadyIn, r.PetSkillScheduled)
	e.Pet.NextStarveAt = rebaseOptionalDeadline(now, r.StarveReadyIn, r.StarveScheduled)
	e.Pet.Target = r.Target
	e.Pet.TrustBand = r.TrustBand
	e.Pet.StarveState = r.StarveState
	e.Pet.BagWarned = r.BagWarned
}

func rebaseOptionalDeadline(now, remaining domain.Tick, scheduled bool) domain.Tick {
	if !scheduled {
		return 0
	}
	return now + remaining
}

func remainingTicks(until, now domain.Tick) domain.Tick {
	if until <= now {
		return 0
	}
	return until - now
}

func remainingSkillCooldowns(src map[domain.SkillID]domain.Tick, now domain.Tick) map[domain.SkillID]domain.Tick {
	var out map[domain.SkillID]domain.Tick
	for id, until := range src {
		if remaining := remainingTicks(until, now); remaining > 0 {
			if out == nil {
				out = make(map[domain.SkillID]domain.Tick)
			}
			out[id] = remaining
		}
	}
	return out
}

func remainingItemCooldowns(src map[int32]domain.Tick, now domain.Tick) map[int32]domain.Tick {
	var out map[int32]domain.Tick
	for group, until := range src {
		if remaining := remainingTicks(until, now); remaining > 0 {
			if out == nil {
				out = make(map[int32]domain.Tick)
			}
			out[group] = remaining
		}
	}
	return out
}

func restoreSkillCooldowns(src map[domain.SkillID]domain.Tick, now domain.Tick) map[domain.SkillID]domain.Tick {
	if len(src) == 0 {
		return nil
	}
	out := make(map[domain.SkillID]domain.Tick, len(src))
	for id, remaining := range src {
		if id != 0 && remaining > 0 {
			out[id] = now + remaining
		}
	}
	return out
}

func restoreItemCooldowns(src map[int32]domain.Tick, now domain.Tick) map[int32]domain.Tick {
	if len(src) == 0 {
		return nil
	}
	out := make(map[int32]domain.Tick, len(src))
	for group, remaining := range src {
		if group > 0 && remaining > 0 {
			out[group] = now + remaining
		}
	}
	return out
}

func clampCurrent(current, maximum int32) int32 {
	if current <= 0 || maximum <= 0 {
		return 0
	}
	if current > maximum {
		return maximum
	}
	return current
}

// Player 是玩家实体的专有部分。
type Player struct {
	refineReadyAt   domain.Tick
	Char            *domain.Character // 权威角色数据。存档时拷贝出去, 不把指针交出场景
	Bag             *domain.Bag       // 背包。与角色行一起存, 不能分开
	Worn            *domain.EquipSet  // 身上穿的
	ChangeSet       *domain.ChangeSet // 1.5.8 快速换装面板托管的备用装备
	Warehouse       *domain.Warehouse // 角色独立个人仓库；跨图携带，存档与背包同事务
	Wardrobe        *domain.Wardrobe  // 永久外观收藏；与背包消费同一角色快照落盘
	Stall           *domain.Stall     // 摆摊托管物权；正常离场归还，崩溃后由登录恢复
	StallBusy       bool              // 正在提交上架/下架/成交事务，禁止并发改摊位
	StallMoveWarned bool              // 本次摆摊已提示过移动受限，避免客户端位置心跳刷屏
	TradeBusy       bool              // 双方确认后正在提交原子交易，禁止重复报价/确认
	MailBusy        bool              // 邮件快照事务未完成，不能开始双角色交易
	FamilyBusy      bool              // 正在提交建族/家族邮件事务，禁止重复扣费或耗材
	RackBusy        bool              // 正在提交货架购买/退货事务，禁止重复扣物或彩玉
	WarehouseOpen   bool              // 本场景生命周期内已打开仓库界面
	RackOpen        bool              // 本场景生命周期内已打开神奇货架，购买包本身不带上下文
	// GMGod/GMOneShot 是本次在线生命周期的 GM 战斗状态。它们随跨图运行态
	// 交接，但不进入角色快照；登出后新实体从零值开始。
	GMGod     bool
	GMOneShot bool
	// QuestNPC 是最近一次成功打开任务列表的 NPC。接取和完成任务包
	// 都只带任务号，服务端必须保留这份对话上下文，不信客户端自报 NPC。
	// 它不落盘，跨图/重登自然清空。
	QuestNPC string
	// TransportList 是最近一次在本图成功打开的 NPC 传送列表。0x1072 只带
	// 列表号与索引，必须用这份短生命周期上下文阻止客户端跳过打开步骤伪造选择。
	TransportList int32
	// ShopID 是最近一次在本图成功打开的商店。购买/出售包都不再携带商店号，
	// 必须靠这份短生命周期上下文防止客户端绕过 NPC 直接交易。
	ShopID int32
	// RepairQuote 是最后一次由服务端根据权威实例状态生成的修理报价。确认包本身
	// 不回传费用，因此必须保留实例指纹，防止报价后换物/磨损再按旧价结算。
	RepairQuote *RepairQuote
	// CraftType 是最近一次成功打开的 ComposeDlg.show_type。0x1023 只回传
	// 类型和产物号，服务端用这份上下文选择玩家实际满足的配方。
	CraftType   uint8
	WashPending *WashPending
	// Work 是正在进行的打工。**不落盘** —— 下线就停工,
	// 存了反而要处理"离线期间算不算工时", 而那个问题的任何答案都会被拿去挂机。
	Work *domain.WorkSession
	// Pet 是放出来的那只宠的**实体 id**, 0 = 没放。
	// 只存 id 不存指针, 与场景内其余引用一致(见 00-不可逆决策.md 第五条)。
	Pet domain.EntityID
	// Riding/MountModel 是本次在线生命周期的骑乘展示态。普通召回、死亡
	// 或重登会结束；切图是无损运行态交接，不会让宠物下马。
	Riding     bool
	MountModel int32
	// 同乘关系仅在当前场景有效，不写入角色或宠物存档。
	RideAnchor domain.EntityID
	RideSeat   uint8
	RideEpoch  uint64
	// DeathLoss 保存这具尸体本次已经实际扣掉、尚可由复活技能取回的经验和
	// 随身金钱。普通回城复活会清空它但不返还。
	DeathLoss domain.DeathLoss
	// Party 是所在队伍的号, 0 = 没组队。
	//
	// **这是抄过来的一份, 权威名册在 game/party 里。**
	// 抄一份的理由: 场景每帧都要判"是不是队友"(技能选目标、分经验),
	// 而队伍是跨场景的共享状态 —— 每次都去查带锁的名册, 就等于把锁塞进了帧循环。
	// 抄成一个值之后, 同场景内判队友就是比两个整数, 场景 Actor 模型一点没动。
	// 名册变动时由会话层投一条 SetParty 命令过来刷新。
	Party domain.PartyID
	Sink  event.Sink // 该玩家的下行出口
	// FinalSaver 随实体跨场景转移。正常 Leave、场景异常停止和传送失败都必须
	// 经它提交最后快照，避免 Session 看见“场景已停”时猜测哪次写入才是最终版。
	FinalSaver SnapshotSaver

	dirty  bool        // 有未落盘的变更
	nextAt domain.Tick // 下次可攻击的帧
	// dungeonEvictAt 只在副本内失去组队资格后有值。恢复组队即清除；
	// 在线到期由场景传送到出口，中途下线则由存档边界直接落出口。
	dungeonEvictAt domain.Tick
	// skillReady 是每个主动技能自己的冷却结束帧。它与 nextAt 的全局出手间隔
	// 分开，否则法宝 6 分钟冷却会错误地把普通攻击也锁住 6 分钟。
	skillReady map[domain.SkillID]domain.Tick
	// itemReady 按 ov_itemcool.item_type 记录共享物品冷却。不能按 item id
	// 分开，否则同组的不同生命药能绕过客户端快捷栏的整组冷却。
	itemReady        map[int32]domain.Tick
	expBonusPct      int32
	expBonusUntil    domain.Tick
	petExpBonusPct   int32
	petExpBonusUntil domain.Tick
	// resting/nextRestHealAt 是客户端 0x1011 坐下动作对应的服务端运行态。
	// 它不落盘也不跨图；重登或切图后必须重新坐下。
	resting        bool
	nextRestHealAt domain.Tick
	// nextNianliAt 是在线念力恢复节拍；跨图保存剩余帧，但下线后不累计。
	nextNianliAt domain.Tick

	// safeUntil 是复活保护到期的帧。这段时间内怪不会主动索敌到他身上。
	//
	// 没有它就会出现死亡循环: 在原地复活的玩家紧挨着刚打死他的怪,
	// 复活的那一帧就会被重新盯上、再打死 —— 实测就是这样(帧71复活, 帧71挨打, 帧91再死)。
	safeUntil domain.Tick
}

type WashPending struct {
	TargetKind uint8
	EquipSlot  uint8
	Tab        uint8
	BagSlot    int32
	UID        int64
	Item       domain.ItemID
	Quality    uint8
	Count      uint8
	Affixes    [4]domain.Affix
}

// RepairQuote/RepairQuoteItem 只保存短生命周期的值拷贝，不把 Bag/EquipSet 指针
// 泄出场景 actor。TargetKind: 0=背包，1=已穿戴；mode: 0=普通、1=特殊、2=全部。
type RepairQuote struct {
	Mode, TargetKind, Tab uint8
	Slot                  int32
	ShopID                int32
	Fee                   int64
	Nianli                int32
	Items                 []RepairQuoteItem
}

type RepairQuoteItem struct {
	TargetKind        uint8
	Tab               uint8
	Slot              int32
	UID               int64
	Item              domain.ItemID
	Durability        int32
	MaxDurability     int32
	DurabilityWearRaw int32
}

// SetResting 切换坐下休息状态。重复的 on=true 不重排首跳，避免同一动作的
// 重复上报把治疗时间不断向后推。
func (p *Player) SetResting(on bool, now, every domain.Tick) {
	if !on || every == 0 {
		p.resting = false
		p.nextRestHealAt = 0
		return
	}
	if p.resting {
		return
	}
	p.resting = true
	p.nextRestHealAt = now + every
}

// StopResting 终止坐下休息。
func (p *Player) StopResting() { p.SetResting(false, 0, 0) }

// SkillReadyAt 报告指定技能是否已结束独立冷却。
func (p *Player) SkillReadyAt(id domain.SkillID, now domain.Tick) bool {
	return p == nil || p.skillReady == nil || now >= p.skillReady[id]
}

// StartSkillCooldown 记录技能独立冷却。零冷却不创建运行态表。
func (p *Player) StartSkillCooldown(id domain.SkillID, now, cooldown domain.Tick) {
	if p == nil || id == 0 || cooldown == 0 {
		return
	}
	if p.skillReady == nil {
		p.skillReady = make(map[domain.SkillID]domain.Tick)
	}
	p.skillReady[id] = now + cooldown
}

// ItemReadyAt 报告物品共享冷却组是否已经结束。
func (p *Player) ItemReadyAt(group int32, now domain.Tick) bool {
	return p == nil || group <= 0 || p.itemReady == nil || now >= p.itemReady[group]
}

// StartItemCooldown 记录一个物品共享冷却组的结束帧。
func (p *Player) StartItemCooldown(group int32, now, cooldown domain.Tick) {
	if p == nil || group <= 0 || cooldown == 0 {
		return
	}
	if p.itemReady == nil {
		p.itemReady = make(map[int32]domain.Tick)
	}
	p.itemReady[group] = now + cooldown
}

func (p *Player) ExperienceBonusPct(now domain.Tick) int32 {
	if p == nil || p.expBonusPct <= 0 || now >= p.expBonusUntil {
		if p != nil {
			p.expBonusPct, p.expBonusUntil = 0, 0
		}
		return 0
	}
	return p.expBonusPct
}

// ExpireExperienceBoost 在场景帧边界清理到期的经验卡运行态。
// 返回任一增益是否刚到期；调用方据此保存剩余状态并刷新客户端显示。
func (p *Player) ExpireExperienceBoost(now domain.Tick) bool {
	if p == nil {
		return false
	}
	playerExpired := p.expBonusPct > 0 && now >= p.expBonusUntil
	if playerExpired {
		p.expBonusPct, p.expBonusUntil = 0, 0
	}
	petExpired := p.petExpBonusPct > 0 && now >= p.petExpBonusUntil
	if petExpired {
		p.petExpBonusPct, p.petExpBonusUntil = 0, 0
	}
	return playerExpired || petExpired
}

func (p *Player) PetExperienceBonusPct(now domain.Tick) int32 {
	if p == nil || p.petExpBonusPct <= 0 || now >= p.petExpBonusUntil {
		if p != nil {
			p.petExpBonusPct, p.petExpBonusUntil = 0, 0
		}
		return 0
	}
	return p.petExpBonusPct
}

func (p *Player) StartExperienceBoost(playerPct, petPct int32, now, duration domain.Tick) {
	if p == nil || duration == 0 {
		return
	}
	if playerPct > 0 {
		p.expBonusPct, p.expBonusUntil = playerPct, now+duration
	}
	if petPct > 0 {
		p.petExpBonusPct, p.petExpBonusUntil = petPct, now+duration
	}
}

// SaveExperienceBoost 将两个独立的经验卡计时器转换为离线暂停的剩余帧。
func (p *Player) SaveExperienceBoost(c *domain.Character, now domain.Tick) {
	if p == nil || c == nil {
		return
	}
	c.ExpBonusPct = p.ExperienceBonusPct(now)
	c.PetExpBonusPct = p.PetExperienceBonusPct(now)
	c.ExpBonusRemainingTicks = int64(remainingTicks(p.expBonusUntil, now))
	c.PetExpBonusRemainingTicks = int64(remainingTicks(p.petExpBonusUntil, now))
}

// RestoreExperienceBoost 只供新在线生命周期使用；跨图继续传 PlayerRuntime。
func (p *Player) RestoreExperienceBoost(c *domain.Character, now domain.Tick) {
	if p == nil || c == nil {
		return
	}
	if c.ExpBonusPct > 0 && c.ExpBonusRemainingTicks > 0 {
		p.expBonusPct = c.ExpBonusPct
		p.expBonusUntil = now + domain.Tick(c.ExpBonusRemainingTicks)
	}
	if c.PetExpBonusPct > 0 && c.PetExpBonusRemainingTicks > 0 {
		p.petExpBonusPct = c.PetExpBonusPct
		p.petExpBonusUntil = now + domain.Tick(c.PetExpBonusRemainingTicks)
	}
}

// RestHealDue 报告本帧是否该触发一跳，并排好下一跳。
func (p *Player) RestHealDue(now, every domain.Tick) bool {
	if !p.resting || every == 0 || p.nextRestHealAt == 0 || now < p.nextRestHealAt {
		return false
	}
	p.nextRestHealAt = now + every
	return true
}

// StartNianliRecovery 初始化新的在线生命周期恢复节拍。
func (p *Player) StartNianliRecovery(now, every domain.Tick) {
	if p != nil && every > 0 {
		p.nextNianliAt = now + every
	}
}

// NianliRecoveryDue 报告本帧是否到达恢复点，并保持固定的在线周期。
func (p *Player) NianliRecoveryDue(now, every domain.Tick) bool {
	if p == nil || every == 0 {
		return false
	}
	if p.nextNianliAt == 0 {
		p.nextNianliAt = now + every
		return false
	}
	if now < p.nextNianliAt {
		return false
	}
	p.nextNianliAt = now + every
	return true
}

// Protect 给一段复活保护。
func (p *Player) Protect(until domain.Tick) { p.safeUntil = until }

// Protected 报告现在是否处于复活保护中。
func (p *Player) Protected(now domain.Tick) bool { return now < p.safeUntil }

// MarkDirty 标记有未落盘变更。移动、升级、掉血都该调它。
func (p *Player) MarkDirty() { p.dirty = true }

// TakeDirty 取出并清除脏标记。只有真的准备写盘时才调 ——
// 清了标记又没写成功, 这次变更就永远丢了。
func (p *Player) TakeDirty() bool {
	d := p.dirty
	p.dirty = false
	return d
}

// Dirty 只读地看一眼脏标记。
func (p *Player) Dirty() bool { return p.dirty }

// StartDungeonEviction 只记录首次失去资格的截止帧，重复退队不能续期。
func (p *Player) StartDungeonEviction(at domain.Tick) {
	if p != nil && p.dungeonEvictAt == 0 {
		p.dungeonEvictAt = at
	}
}

func (p *Player) ClearDungeonEviction() {
	if p != nil {
		p.dungeonEvictAt = 0
	}
}

func (p *Player) DungeonEvictionDue(now domain.Tick) bool {
	return p != nil && p.dungeonEvictAt > 0 && now >= p.dungeonEvictAt
}

// Monster 是怪物实体的专有部分。
type Monster struct {
	TypeID    domain.MonsterID   // 配置表主键: 这是"哪种怪"
	Kind      domain.MonsterKind // 普通/精英/BOSS/场景物件/采集物。决定战斗类别与重生间隔
	EliteRing bool               // 是否由 EliteID 动态转换而来；只有该来源会下发脚下光圈
	// ColorProfile 是这只怪掉出的随机装备使用的染色（属性条数）档位。它从模板
	// 拷进来，因此光圈精英拿到的是精英模板的档位，而不是被替换掉的原普通怪档位。
	ColorProfile domain.ColorProfile
	SpawnID      int32      // 来自哪个刷怪点, 死后由它负责重生
	Home         domain.Pos // 出生点记录。怪物脱战后不会据此回位或重置

	Target     domain.EntityID // 当前仇恨目标, 0 = 无
	TauntPower int32           // 当前目标由嘲讽建立时的覆盖优先级；普通仇恨为 0
	nextAt     domain.Tick     // 下次可攻击的帧
	// CombatPathActive 表示客户端收过一条追击/拉距路径；脱战时必须显式
	// 清掉它，避免服务端已停住而客户端仍沿旧路径插值。
	CombatPathActive bool
	// ChasePath 是 MASK 局部寻路算出的少量拐点。它只存在于本次仇恨期间，
	// 不持久化；目标移动较远、脱战或重生都会丢弃并重新计算。
	ChasePath       []domain.Pos
	ChasePathNext   int
	ChasePathTarget domain.Pos
	// ChasePathRetryAt 缓存一次局部寻路失败，避免一群怪隔着大片水面时每 500ms
	// 重复铺满搜索圆。目标明显移动后可立即重试，否则最多等 2 秒。
	ChasePathRetryAt     domain.Tick
	ChasePathRetryTarget domain.Pos

	// RoamTo/NextRoamAt 是无目标时的游荡状态。每轮都从当前位置选短途目的地；
	// 追到传送门附近的怪会从那里继续活动，不会回出生点重置。
	RoamTo     domain.Pos
	Roaming    bool
	NextRoamAt domain.Tick
	// 每次出生，每种台词场景至多说一句。避免每次攻击/受击都刷满屏幕。
	SpokenScenes uint8

	// 行为参数。从 MonsterDef 拷进来, 免得每帧回查配置表。
	Aggressive    bool  // 是否主动找人打
	NoAttack      bool  // 完全不能攻击；受击后也不挂仇恨
	NoBasicAttack bool  // 技能照常，禁止普通攻击
	ViewDist      int32 // 索敌半径
	TraceDist     int32 // 当前目标距离超过它时放弃追击；不触发回出生点
	AI            domain.MonsterAIProfile

	// 技能运行态。定义留在 MonsterDef；实体这里只保存这一次出生的冷却与施法状态。
	NextSkillCheck domain.Tick
	NextSkillAt    domain.Tick
	SkillReady     map[domain.SkillID]domain.Tick
	SkillCasts     map[domain.SkillID]int32
	CastingSkill   domain.SkillID
	CastingUntil   domain.Tick
	CastSerial     uint64

	// Summoner 非零表示它由技能临时召唤，不占刷怪点、不给击杀奖励，也不重生。
	Summoner domain.EntityID
	ExpireAt domain.Tick
}

// ReadyToAttack 报告实体在 now 帧能否出手。
func (e *Entity) ReadyToAttack(now domain.Tick) bool {
	switch e.Kind {
	case domain.KindPlayer:
		return e.Player != nil && now >= e.Player.nextAt
	case domain.KindMonster:
		return e.Monster != nil && now >= e.Monster.nextAt
	case domain.KindPet:
		return e.Pet != nil && now >= e.Pet.nextAt
	}
	return false
}

// DidAttack 记下技能等使用属性攻速的出手，排下一次可出手帧。
func (e *Entity) DidAttack(now domain.Tick) {
	e.setNextAttack(now + e.Stats.AttackIntervalTicks())
}

const (
	// 客户端 CombatWorld 的动画节拍是近战 1 秒、远程 0.5 秒，但服务端准入
	// 必须给网络抖动与 100ms 逻辑帧留余量。基础周期与客户端一致，百分比攻速按同一属性值计算。
	// 100ms逻辑帧向下对齐提供不足一帧的量化容差，不额外打折。
	// RemotePlayer.get_IsRangedWeapon 硬编码为攻击距离 > 150。
	playerMeleeAttackMS  = 1000
	playerRangedAttackMS = 500
	playerRangedAtkDist  = 150
)

// UsesRangedWeapon 与正式客户端 RemotePlayer.get_IsRangedWeapon 使用同一门槛。
func (e *Entity) UsesRangedWeapon() bool {
	return e != nil && e.Kind == domain.KindPlayer && e.Look.AtkDist > playerRangedAtkDist
}

// DidBasicAttack 记录普通攻击。玩家采用略短于动画的准入间隔，避免准时到达的
// 下一刀因网络/帧量化落在边界前而被拒；怪物和宠物仍使用各自配置的 AtkSpeedMS。
func (e *Entity) DidBasicAttack(now domain.Tick) {
	interval := e.Stats.AttackIntervalTicks()
	if e.Kind == domain.KindPlayer {
		baseMS := int64(playerMeleeAttackMS)
		if e.UsesRangedWeapon() || e.Look.WeaponCType == 8 {
			baseMS = playerRangedAttackMS
		}
		interval = domain.Tick(baseMS * 100 / int64(e.Stats.PlayerAttackSpeedPercent()) / domain.TickMS)
		if interval < 1 {
			interval = 1
		}
	}
	e.setNextAttack(now + interval)
}

func (e *Entity) setNextAttack(next domain.Tick) {
	switch e.Kind {
	case domain.KindPlayer:
		if e.Player != nil {
			e.Player.nextAt = next
		}
	case domain.KindMonster:
		if e.Monster != nil {
			e.Monster.nextAt = next
		}
	case domain.KindPet:
		if e.Pet != nil {
			e.Pet.nextAt = next
		}
	}
}

// DelayAttackUntil 只会把全局出手时间向后推。技能自己的冷却与普通攻击间隔
// 取较晚者，不能因一个较短的技能全局冷却反向缩短已有攻速限制。
func (e *Entity) DelayAttackUntil(until domain.Tick) {
	switch e.Kind {
	case domain.KindPlayer:
		if e.Player != nil && e.Player.nextAt < until {
			e.Player.nextAt = until
		}
	case domain.KindMonster:
		if e.Monster != nil && e.Monster.nextAt < until {
			e.Monster.nextAt = until
		}
	case domain.KindPet:
		if e.Pet != nil && e.Pet.nextAt < until {
			e.Pet.nextAt = until
		}
	}
}

// RefineReadyAt serializes confirmation retries through the configured high-refinement window.
func (p *Player) RefineReadyAt(now domain.Tick) bool { return p == nil || now >= p.refineReadyAt }
func (p *Player) StartRefineCooldown(now, span domain.Tick) {
	if p != nil && span > 0 {
		p.refineReadyAt = now + span
	}
}
