package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// PlayerEmoted 使用客户端内置的 96 个表情编号，不携带服务端自定义资源。
type PlayerEmoted struct {
	Who  domain.EntityID
	Face int32
}

func (e PlayerEmoted) Subject() domain.EntityID { return e.Who }

// MonsterSpoke 是怪物从 ov_dialog 原文中说出的一句台词。
type MonsterSpoke struct {
	Who  domain.EntityID
	Text string
}

func (e MonsterSpoke) Subject() domain.EntityID { return e.Who }

// 实体在场与移动。这些事件对应下行的出场包/移动包, 但本文件不写那些 opcode ——
// opcode 是 protocol 层的事, 写在这里就是把协议漏进游戏层了。

// EntitySpawned 一个实体进入了观察者的视野。
//
// 它同时表达"我刚进图看到了它"和"它走进了我的视野"两种情形 —— 对观察者而言
// 这两件事没有区别, 都是"这个实体现在看得见了"。
type EntitySpawned struct {
	Stall *StallOwnerChanged
	ID    domain.EntityID
	Kind  domain.EntityKind
	// Owner 仅对宠物等从属实体有意义；0 表示没有主人。
	Owner domain.EntityID
	Name  string
	Level int32
	HP    int32
	MaxHP int32
	Pos   domain.Pos
	Dir   uint8
	Look  domain.Look
	// Riding/MountModel 只对玩家出场有意义，使后来进入地图的观察者也能直接
	// 看到正确坐骑，而不依赖曾经错过的增量事件。
	Riding              bool
	MountModel          int32
	RideAnchor          domain.EntityID
	RideSeat, RideSeats uint8
	// Invisible 只对远端玩家出场有意义。后来进入场景的观察者必须从完整
	// 出场快照直接得到隐身状态，不能依赖自己此前没有收到过的增量事件。
	Invisible bool
	// Fresh 只在怪物刚从刷怪点重生的那次广播为 true。普通进图补快照不能继承它，
	// 否则后来进图的玩家会把早已存在的怪再播一遍刷新动画。
	Fresh bool
	Elite bool // 光圈精英怪；占用 0x800f eliteAndFresh 字段的低位

	// NPC 非 nil 时表示这是个 NPC, 里面是它专有的展示数据。
	//
	// 用指针而不是把字段摊平进来: 出场事件是最热的事件之一(进图一次几百条),
	// 而这几个字段只有 NPC 用得上 —— 摊平就是给 99% 的事件白背几十字节。
	NPC *NPCView

	// Ground 非 nil 时表示这是地面掉落物。归属人是游戏事实，但“当前观察者
	// 是否就是归属人”是接收者相关的展示结果，不能在场景广播前预先算成 bool。
	Ground *GroundItemView

	// Trap 非 nil 时走客户端 0x800f 的 entityType=5 分支。
	Trap *TrapView

	// StatusIcons 是该实体在出场这一刻的完整可见状态；运行期变化另走
	// EntityStatusSnapshot，避免重发实体生命周期包。
	StatusIcons []StatusIconView
}

type TrapView struct {
	Skill  domain.SkillID
	Radius int32
}

type StatusIconView struct {
	Status     int32
	Icon       int32
	Beneficial bool
	RemainMS   int32
}

// EntityStatusSnapshot 刷新一个已在场实体头顶的状态图标列表（0x8064）。
type EntityStatusSnapshot struct {
	Who   domain.EntityID
	Icons []StatusIconView
}

func (e EntityStatusSnapshot) Subject() domain.EntityID { return e.Who }

// EntityStatusEffect 刷新世界中一个实体身上的状态特效。
// DurationMS 为 0 时表示提前结束该特效；正常到期仍由客户端按剩余时长自行清理。
type EntityStatusEffect struct {
	Who          domain.EntityID
	Effect       string
	PositionMode uint8
	DurationMS   int32
}

func (e EntityStatusEffect) Subject() domain.EntityID { return e.Who }

// GroundItemView 是地面掉落物对观察者公开的数据。
//
// ProtectionMS 是事件产生这一刻尚余的归属保护毫秒数；场景用逻辑帧计算，
// 协议出口再结合接收者与 Owner 生成客户端的 owned 位。
type GroundItemView struct {
	Item         domain.ItemID
	Count        int32
	Owner        domain.EntityID
	ProtectionMS int32
	Quality      uint8
}

// NPCView 是 NPC 落位包(0x8046)要的展示数据。
//
// **不放进 domain.Look**: Look 是"长什么样"的通用描述, 而这几个是
// 客户端 NPC 系统专有的资源名(对话脚本、立绘、名牌)。
type NPCView struct {
	Sprite   string // "npc601" 这种资源名
	Script   string // 对话脚本号
	Portrait string // 立绘资源名; 来自 PostgreSQL，未配置时为空
	Label    string // 名牌; 无可靠配置时为空
	Sell     int32  // 商店表 id
	Trans    int32  // 传送表 id
}

func (e EntitySpawned) Subject() domain.EntityID { return e.ID }

// EntityDespawned 一个实体离开了观察者的视野(走远、下线、死亡消失、被拾取)。
type EntityDespawned struct {
	ID     domain.EntityID
	Kind   domain.EntityKind
	Reason DespawnReason
}

func (e EntityDespawned) Subject() domain.EntityID { return e.ID }

// DespawnReason 让协议层与客户端知道该播什么表现: 走出视野是静默移除,
// 死亡要播倒地动画, 被拾取要播拾取特效。
type DespawnReason uint8

const (
	DespawnOutOfSight DespawnReason = iota
	DespawnLogout
	DespawnDeath
	DespawnPickedUp
	DespawnTimeout
	// DespawnRemoved 被主动摘掉了: 抓走的怪、收回的宠。
	// 与 DespawnDeath 分开 —— 客户端该播的是"消失"而不是"死亡"动画。
	DespawnRemoved
)

// EntityMoved 实体移动到了新位置。
//
// 每帧每实体最多一条(场景在帧末合并), 不是每收到一个移动包转发一条 ——
// 客户端 10Hz 上报, 原样转发就是 N² 的广播风暴。
type EntityMoved struct {
	ID   domain.EntityID
	To   domain.Pos
	Dir  uint8 // 朝向, 0~7
	Stop bool  // true = 到达终点停下, 客户端切回站立动作
	// Snap 表示立即校正到 To 并清空客户端剩余路径。客户端不会重新计算朝向，
	// 因而也用于清理怪物攻击前的转向提示路径。
	Snap bool
	// Speed 是移动速度(客户端按每秒像素用)。普通移动的 0 表示“事件未提供”，
	// 协议层会补默认值；Stop 会在线上明确写 0，让客户端保留现有速度并在目标点
	// 结束路径。负数只用于 Snap，客户端会立即校正坐标并清空旧路径。
	Speed int32
}

func (e EntityMoved) Subject() domain.EntityID { return e.ID }

// RidingChanged 是玩家开始或结束骑乘。协议层按观察者身份把同一个领域事实
// 翻译成本人状态包或远端玩家状态包。
type RidingChanged struct {
	Who         domain.EntityID
	Riding      bool
	MountModel  int32
	Anchor      domain.EntityID
	Seat, Seats uint8
}

func (e RidingChanged) Subject() domain.EntityID { return e.Who }

// PlayerVisibilityChanged 是远端玩家进入或退出隐身。本人由 Buff 快照表现，
// 旁观者走客户端原生 RemotePlayer.SetInvis 入口。
type PlayerVisibilityChanged struct {
	Who       domain.EntityID
	Invisible bool
}

func (e PlayerVisibilityChanged) Subject() domain.EntityID { return e.Who }

// PlayerPKStateChanged 是其他玩家名牌/战斗选择使用的公开 PK 状态。
// 当前 PVP 未开放，业务只会发布中性状态 0。
type PlayerPKStateChanged struct {
	Who   domain.EntityID
	State uint8
}

func (e PlayerPKStateChanged) Subject() domain.EntityID { return e.Who }

// PlayerRenamed 是在线角色改名后的公开事实。本人由 0x8057 更新，旁观者由
// 0x8056 更新现有 RemotePlayer，不能通过 despawn/spawn 伪造重进场。
type PlayerRenamed struct {
	Who  domain.EntityID
	Name string
}

func (e PlayerRenamed) Subject() domain.EntityID { return e.Who }

// AppearanceChanged 是玩家穿脱装备或修改展示偏好后的公开外观快照。
// 它对本人和 AOI 内其他玩家都可见：本人用它刷新自己的 PlayerAvatar，旁观者用它
// 刷新 RemotePlayer。所有可公开特效都来自 Appearance，协议层不再覆盖领域值。
type AppearanceChanged struct {
	Who        domain.EntityID
	Appearance domain.Appearance
}

func (e AppearanceChanged) Subject() domain.EntityID { return e.Who }

// HairChanged 是理发店确认后的发型/发色完整快照。Appearance 同时提供给
// 会话在线索引刷新队伍头像，线上 0x803f 只写其中的 Head/Hair。
type HairChanged struct {
	Who        domain.EntityID
	Appearance domain.Appearance
}

func (e HairChanged) Subject() domain.EntityID { return e.Who }

// EntityOwnerChanged synchronizes an existing entity's actual owner.
type EntityOwnerChanged struct{ Who, Owner domain.EntityID }

func (e EntityOwnerChanged) Subject() domain.EntityID { return e.Who }

type EntityEffectRequested struct {
	Who    domain.EntityID
	Target uint8
	Effect string
}

func (e EntityEffectRequested) Subject() domain.EntityID { return e.Who }

type HornDialogOpened struct {
	Who        domain.EntityID
	Item       domain.ItemID
	Tier, Skin uint8
}

func (e HornDialogOpened) Subject() domain.EntityID { return e.Who }
