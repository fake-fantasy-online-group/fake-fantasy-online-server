package event

import "fmt"

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// 只发给当事人的事件。场景在广播时按 Subject 判断: 这些事件不进 AOI, 直投本人。

// SelfEntered 你自己进入了一个场景。必须在任何其他实体的出场事件之前送达 ——
// 客户端要先有"我", 才能把别人摆到我周围。
type SelfEntered struct {
	ID    domain.EntityID
	Char  *domain.Character // 本人角色数据; 只读, 不得由协议层修改
	Scene domain.SceneID
}

func (e SelfEntered) Subject() domain.EntityID { return e.ID }

// NewbieTipSeed 是当前角色已经看过的固定新手提示编号。
// 客户端用它初始化 NewbieTipBox.shownIds，后续技能/宠物快照即使再次
// 触发同一 tipId，也不会重复展示。
type NewbieTipSeed struct {
	Who    domain.EntityID
	TipIDs []int32
}

func (e NewbieTipSeed) Subject() domain.EntityID { return e.Who }

// NewbieTipRequested 是服务端根据已经成功的权威玩法事件，请求向本人展示的一条
// 固定新手提示。ID 对应正式客户端 NewbieTipBox 的内置字典；跨登录去重由会话层
// 在真正下发 0x8008 mode=5 时完成，场景层不接触协议和存储。
type NewbieTipRequested struct {
	Who domain.EntityID
	ID  int32
}

func (e NewbieTipRequested) Subject() domain.EntityID { return e.Who }

// StatsChanged 自身属性快照变了(受伤、升级、换装、上下状态)。
//
// Current 与 Total 是领域层已经组好的权威数组。前七项已验证为
// level/exp/expToNext/hp/maxHp/mp/maxMp；其余位置在语义完全闭合前只保留
// 角色已有兼容值，不由协议层猜测或重新排列。
type StatsChanged struct {
	Who            domain.EntityID
	Current, Total []int32
}

func (e StatsChanged) Subject() domain.EntityID { return e.Who }

// PKStateSnapshot 是本人 PK 面板的完整状态。模式偏好可以保存，但在 PVP
// 玩法开放前 MapSafe 恒真、State 恒 0，明确阻止客户端进入可互伤表现。
type PKStateSnapshot struct {
	Who          domain.EntityID
	Mode         uint8
	Karma        int64
	State        uint8
	MapSafe      bool
	ProtectLevel int32
}

func (e PKStateSnapshot) Subject() domain.EntityID { return e.Who }

// SkillSnapshot 是玩家当前已学技能与未分配技能点的完整快照。
//
// Skills 的顺序由构造快照的游戏层决定；协议层只忠实编码，不从 map 迭代顺序
// 推导客户端顺序。技能点与角色六维自由点不是同一个概念，调用方不得混用。
type SkillSnapshot struct {
	DamageBonuses map[domain.SkillID]int32
	Who           domain.EntityID
	SkillPoints   int32
	Skills        []SkillView
}

func (e SkillSnapshot) Subject() domain.EntityID { return e.Who }

// SkillView 是技能快照中的一项。ID 与 Level 都是客户端直接消费的 I32。
type SkillView struct {
	ID    domain.SkillID
	Level int32
}

// LifeSkillSnapshot 是客户端生活技能面板的完整九项列表。
type LifeSkillSnapshot struct {
	Who    domain.EntityID
	Skills []LifeSkillView
}

func (e LifeSkillSnapshot) Subject() domain.EntityID { return e.Who }

type LifeSkillView struct {
	ID       domain.SkillID
	Level    int32
	Name     string
	CanLearn bool
}

type CraftRecipeSnapshot struct {
	Who      domain.EntityID
	MakeType uint8
	Recipes  []CraftRecipeView
}

func (e CraftRecipeSnapshot) Subject() domain.EntityID { return e.Who }

type CraftRecipeView struct {
	Product   domain.ItemID
	Name      string
	Cost      int32
	CanMake   bool
	Materials []CraftMaterialView
}

type CraftMaterialView struct {
	Item domain.ItemID
	Need int32
	Have int32
	Name string
}

type RefineInfo struct {
	Who          domain.EntityID
	Mode         uint8
	EquipSlot    uint8
	CurrentLevel int32
	Cost         int32
	CanDo        bool
	SuccessRate  int32
	Tab          uint8
	BagSlot      int32
	Item         domain.ItemID
	Materials    []CraftMaterialView
	SafeLevel    int32
	FailDestroys bool
}

func (e RefineInfo) Subject() domain.EntityID { return e.Who }

type SocketInfo struct {
	Who          domain.EntityID
	Item         domain.ItemID
	SocketCount  uint8
	Sockets      []domain.ItemID
	NextCost     int32
	NextSuccess  int32
	CanDrill     bool
	Materials    []CraftMaterialView
	SocketNames  []SocketNameView
	MaxHoles     uint8
	Reason       string
	FailDestroys bool
}

func (e SocketInfo) Subject() domain.EntityID { return e.Who }

type SocketNameView struct {
	Item domain.ItemID
	Name string
}

type AttrWashInfo struct {
	AtMax   []bool
	Who     domain.EntityID
	Kind    uint8
	Tab     uint8
	Slot    int32
	Item    domain.ItemID
	Quality uint8
	Affixes []string
}

func (e AttrWashInfo) Subject() domain.EntityID { return e.Who }

// ServerNotice 是普通聊天区文本，不触发任务/NPC/打工的客户端动作。
type ServerNotice struct {
	Who  domain.EntityID
	Text string
}

func (e ServerNotice) Subject() domain.EntityID { return e.Who }

// HotbarSlotCount包含20格主栏与10格扩展栏，expanded位于两段之间。
const HotbarSlotCount = domain.HotbarSlotCount

// HotbarSnapshot 是完整快捷栏快照。固定长度数组把“完整30格”的协议不变量
// 提前放进类型系统，调用方无法误发少一格或多一格的载荷。
type HotbarSnapshot struct {
	Who      domain.EntityID
	Slots    [HotbarSlotCount]HotbarSlotView
	Expanded bool
}

func (e HotbarSnapshot) Subject() domain.EntityID { return e.Who }

// HotbarSlotView 是一格快捷栏的协议无关展示值。Kind 决定 ID 的业务类别，
// Icon 是客户端 LoadHotbar 直接使用的图标号。
type HotbarSlotView struct {
	ID   int32
	Kind uint8
	Icon int32
}

// PetSnapshot 是宠物栏的完整快照。只发给本人。
//
// 什么时候发: 进世界时发一次, 之后**每次宠物栏变化都要重发** ——
// 抓到新宠、放出、收回都算。客户端那边是整份覆盖, 没有增量。
//
// 协议的固定段无论有没有宠都必须照发, 所以"没有宠"要用 ActiveSlot = NoActivePet
// 明确表达, 不能靠不发这个事件来表示。
type PetSnapshot struct {
	FightCap, LifeCap int32
	// SharedMountModel 是乘客当前坐骑的显示模型，不是其拥有或出战的宠物。
	// 正式1.5.8的UpdateAnim先检查宠物快照模型，ComposeMount再读取骑乘模型。
	SharedMountModel int32
	PPAiCap          int32
	Who              domain.EntityID
	// Active 是当前放出来那只的完整状态。ActiveSlot == NoActivePet 时它保持零值。
	Active     PetView
	ActiveSlot int
	// ActiveBound mirrors the current active pet's carrier binding state.
	ActiveBound bool
	// Pets 是宠物栏里的全部宠, **下标即槽位号**。客户端随后的"放出第 N 只"
	// 就是拿这个下标回报的, 因此顺序必须与持久化顺序一致、每次快照都相同。
	Pets []PetSlotView
	// Show 是"宠物要不要显示在场景里"的当前权威状态。
	Show bool
	// Ridable 表示当前出战宠物可通过已学坐骑技能或当前鞍具骑乘。
	Ridable bool
}

// NoActivePet 表示当前没有宠在外面。
const NoActivePet = -1

func (e PetSnapshot) Subject() domain.EntityID { return e.Who }

// PetView 是一只宠的完整状态。
//
// Gender is assigned on hatch and persisted; Habit comes from species food configuration.
// FreePoints here is the active pet's remaining allocatable points.
type PetView struct {
	PPAiUsed    int32
	Model       int32 // 客户端模型号；不是宠物种族号（海龟为 206，不是 1006）
	Species     string
	Prename     string
	Name        string // 玩家改的名字; 空表示用种族名
	Gender      uint8
	Habit       uint8
	Level       int32
	Exp         int32
	ExpToNext   int32
	Starve      int32
	Trust       int32
	FreePoints  int32
	HP, MaxHP   int32
	MP, MaxMP   int32
	Base        domain.Base
	MinAtk      int32
	MaxAtk      int32
	Def         int32
	MAtk        int32
	MDef        int32
	Hit         int32
	ActiveSkill int32
	Skills      []PetSkillView
}

type PetSkillView struct {
	ID          int32
	Name        string
	Icon        int32
	Description string
	Fight       bool
	Active      bool
	Level       int32
	MaxLevel    int32
}

// PetSlotView 是宠物栏里的一行。Slot 是这一行的槽位号, 与 PetSnapshot.Pets
// 的下标相同 —— 两边都发是因为客户端两处都读, 不是冗余。
type PetSlotView struct {
	PPAiUsed   int32
	Model      int32
	Slot       int32
	Name       string
	Prename    string
	Species    string
	Gender     uint8
	Habit      uint8
	Level      int32
	Base       domain.Base
	Starve     int32
	Trust      int32
	Opened     bool
	Bound      bool
	Locked     bool
	FreePoints int32
	UsedPoints int32
	Skills     []PetSkillView
}

// BuffSnapshot 是玩家本人当前可见 Buff/Debuff 的完整快照。空切片明确清空客户端。
type BuffSnapshot struct {
	Who       domain.EntityID
	Buffs     []BuffView
	DoubleExp bool
}

func (e BuffSnapshot) Subject() domain.EntityID { return e.Who }

type BuffView struct {
	SkillID      int32
	Icon         int32
	RemainSec    int32
	Name, Desc   string
	Beneficial   bool
	PositionMode uint8
	Invisible    bool
	Immobile     uint8
}

// Teleported 表示玩家已经跨场景落到 To。它与初次进入世界不同：客户端有独立
// 的 teleport 回调，不能用另一份 EnterWorld 快照冒充这个生命周期事件。
// Scene 是服务端路由所需的完整场景标识；To 只承载客户端 0x803a 所需的地图与坐标。
type Teleported struct {
	Who   domain.EntityID
	Scene domain.SceneID
	To    domain.Pos
}

func (e Teleported) Subject() domain.EntityID { return e.Who }

// TransportDestinations 是玩家打开 NPC 传送菜单时看到的权威目的地列表。
// 只发给请求者；不同 TransList 的索引、落点和价格均来自 PostgreSQL。
type TransportDestinations struct {
	Who          domain.EntityID
	List         int32
	Destinations []domain.TransportDestination
}

func (e TransportDestinations) Subject() domain.EntityID { return e.Who }

// InventorySnapshot 是玩家当前背包、穿戴与货币的完整公开视图。
//
// 它是一个游戏事件，不是 0x8006 的字节镜像：切片顺序由场景构建者确定，
// 协议层只按顺序写出；宠物物品等尚未进入领域模型的内容也不会用占位数据冒充。
type InventorySnapshot struct {
	Who                 domain.EntityID
	Money, Caiyu, Honor int64
	Weight, MaxWeight   int32
	Items               []InventoryItemView
	Equipped            []EquippedItemView
}

// ItemIconMap 是客户端特殊物品小图标覆盖表。初次进世界发送一次，跨图不重发。
type ItemIconMap struct {
	Who   domain.EntityID
	Pairs []domain.ItemIconMapping
}

func (e ItemIconMap) Subject() domain.EntityID { return e.Who }

func (e InventorySnapshot) Subject() domain.EntityID { return e.Who }

type WarehouseSnapshot struct {
	Who       domain.EntityID
	PageCount uint8
	Money     int64
	Items     []WarehouseItemView
}

func (e WarehouseSnapshot) Subject() domain.EntityID { return e.Who }

type WardrobeSnapshot struct {
	Who      domain.EntityID
	Capacity int32
	Items    []WardrobeItemView
}

func (e WardrobeSnapshot) Subject() domain.EntityID { return e.Who }

type WardrobeItemView struct {
	Item       domain.ItemID
	Category   domain.WardrobeCategory
	Worn       bool
	Blocked    bool
	Name, Desc string
}

type StallContents struct {
	Owner     domain.EntityID
	OwnerName string
	Type      domain.StallType
	Name      string
	Rows      []StallRowView
	Sales     []StallSaleView
}

func (e StallContents) Subject() domain.EntityID { return e.Owner }

type StallRowView struct {
	Pet     *domain.PetItemInfo
	Item    domain.ItemID
	Count   int32
	Price   int64
	Name    string
	Desc    string
	Blocked bool
	Quality uint8
}

type StallSaleView struct {
	Time  int64
	Item  string
	Count int32
	Money int64
}

type StallOwnerChanged struct {
	Owner domain.EntityID
	Type  domain.StallType
	Name  string
}

func (e StallOwnerChanged) Subject() domain.EntityID { return e.Owner }

type StallClosed struct{ Owner domain.EntityID }

func (e StallClosed) Subject() domain.EntityID { return e.Owner }

type WarehouseItemView struct {
	Tab               uint8
	Slot              int32
	Item              domain.ItemID
	Count             int32
	Name, Desc        string
	CurrentDurability int32
	MaxDurability     int32
	Blocked           bool
	Quality           uint8
}

// InventoryItemView 是背包里一格物品的展示数据。
//
// Slot 是客户端与物品定义共同使用的装备槽号；-1 表示非装备。协议层会在写出时
// 做客户端要求的 +1 变换，因此这里不出现 slotPlusOne 这种线协议字段名。
type InventoryItemView struct {
	Pet                              *domain.PetItemInfo
	BagIndex                         int32
	Item                             domain.ItemID
	Count                            int32
	Tab                              uint8
	Name, Desc                       string
	Sell                             int64
	CurrentDurability                int32
	MaxDurability                    int32
	Avatar                           int32
	Blocked, Bound, Locked, Cosmetic bool
	Quality                          uint8
	Slot                             domain.EquipSlot
	FusedDesc                        string
}

// ItemLockChanged 是 0x1080 经场景权威背包裁决后的结果。字段原样对应
// 0x8066 的 item-lock 分支；On=true 为锁定(kind=1)，false 为解锁(kind=2)。
type ItemLockChanged struct {
	Who     domain.EntityID
	On      bool
	OK      bool
	Message string
	Tab     uint8
	Slot    int32
	Item    domain.ItemID
}

func (e ItemLockChanged) Subject() domain.EntityID { return e.Who }

// ShopItems 是一个 NPC 商店的完整库存快照。Blocked 只影响客户端展示；
// 服务端购买仍按物品存在性、金额与背包容量独立裁决。
type ShopItems struct {
	Who   domain.EntityID
	Shop  int32
	Items []ShopItemView
}

func (e ShopItems) Subject() domain.EntityID { return e.Who }

type ShopItemView struct {
	Item    domain.ItemID
	Price   int64
	Name    string
	Desc    string
	Blocked bool
}

type RackCatalogSnapshot struct {
	Who        domain.EntityID
	Categories []domain.RackCategory
	Goods      []RackGoodView
	Holiday    bool
}

func (e RackCatalogSnapshot) Subject() domain.EntityID { return e.Who }

type RackGoodView struct {
	Item    domain.ItemID
	Name    string
	Desc    string
	Weight  int32
	Group   uint8
	Price   int64
	Flags   uint8
	Remain  int32
	Part    string
	Blocked bool
}

// FittingCatalogSnapshot 是 0x8059 的试衣目录。Paged=false 对应旧的完整目录；
// Paged=true 是 1.5.8 的分页精简目录，描述留给 0x806e 按需补取。
type FittingCatalogSnapshot struct {
	Who         domain.EntityID
	Items       []FittingItemView
	Total, Page int32
	Paged       bool
}

func (e FittingCatalogSnapshot) Subject() domain.EntityID { return e.Who }

type FittingItemView struct {
	Item       domain.ItemID
	Name, Part string
	Kind       uint8
	Price      int32
	Model      int32
	Desc       string
}

type FittingDetailsSnapshot struct {
	Who   domain.EntityID
	Items []FittingDetailView
}

func (e FittingDetailsSnapshot) Subject() domain.EntityID { return e.Who }

type FittingDetailView struct {
	Item  domain.ItemID
	Name  string
	Price int32
	Desc  string
}

type RackRefundSnapshot struct {
	Who                 domain.EntityID
	Percent, WindowDays int32
	Categories          string
	UsedOK              bool
	Rows                []RackRefundView
}

func (e RackRefundSnapshot) Subject() domain.EntityID { return e.Who }

type RackRefundView struct {
	Item                           domain.ItemID
	Name                           string
	UnitPrice, Bundle, Day, Shares int32
	Have, Gain, LeftDays           int32
}

type FamilyStashSnapshot struct {
	Who      domain.EntityID
	Capacity int32
	TakePos  uint8
	CanTake  bool
	IsLeader bool
	Entries  []FamilyStashView
}

func (e FamilyStashSnapshot) Subject() domain.EntityID { return e.Who }

type FamilyStashView struct {
	Item                    domain.ItemID
	Count, CurDur, MaxDur   int32
	Quality                 int32
	Name, Desc, DepositedBy string
}

// ChangeSetSnapshot 是 1.5.8 快速换装面板的完整备用套装。空列表必须下发，
// 用来清掉同一客户端上一角色残留的数据。
type ChangeSetSnapshot struct {
	Who   domain.EntityID
	Items []ChangeSetItemView
}

func (e ChangeSetSnapshot) Subject() domain.EntityID { return e.Who }

type ChangeSetItemView struct {
	Cell                   domain.EquipSlot
	Item                   domain.ItemID
	Name, Desc             string
	CurDur, MaxDur, Refine int32
	Quality                uint8
	Blocked                bool
}

// RepairQuoted 是 0x801d 所需的服务端权威修理报价。Mode/Slot/TargetKind/Tab
// 原样回显用于客户端确认，费用与数量来自场景内实例状态。
type RepairQuoted struct {
	Who                   domain.EntityID
	Mode, TargetKind, Tab uint8
	Slot                  int32
	Fee                   int64
	Count                 int32
	Item                  domain.ItemID
	Info                  string
}

func (e RepairQuoted) Subject() domain.EntityID { return e.Who }

// EquippedItemView 是一件已穿戴物品的展示数据。
type EquippedItemView struct {
	Slot                             domain.EquipSlot
	Item                             domain.ItemID
	Name, Desc                       string
	CurrentDurability, MaxDurability int32
	Avatar, SoulAvatar               int32
	Quality                          uint8
}

// PlayerEquipmentInspected 是查看另一玩家装备的公开快照。Who 是请求者，
// Target 是被查看者；Attrs 保留客户端既有的位置数组，不在游戏层重命名未知下标。
type PlayerEquipmentInspected struct {
	Who, Target domain.EntityID
	Name        string
	Level       int32
	Race        domain.Race
	Gender      uint8
	Head        uint8
	Hair        uint8
	Title       string
	Honor       int64
	Appearance  domain.Appearance
	Attrs       []int32
	MaxAtk      int32
	MinAtk      int32
	Def         int32
	MAtk        int32
	MDef        int32
	Hit         int32
	Equipped    []EquippedItemView
}

func (e PlayerEquipmentInspected) Subject() domain.EntityID { return e.Who }

// StatusApplied 身上加了一个状态。它表达领域事实；客户端展示由随后推送的
// BuffSnapshot/EntityStatusSnapshot 完整快照负责。
type StatusApplied struct {
	Who        domain.EntityID
	Status     int32
	Level      int32
	DurationMS int64
}

func (e StatusApplied) Subject() domain.EntityID { return e.Who }

// StatusRemoved 状态到期或被驱散。
type StatusRemoved struct {
	Who    domain.EntityID
	Status int32
}

func (e StatusRemoved) Subject() domain.EntityID { return e.Who }

// StatusResisted 状态被抗性挡下来了。只发给本人 ——
// 「抵抗！」这种飘字是给被打的人看的。
type StatusResisted struct {
	Who    domain.EntityID
	Status int32
}

func (e StatusResisted) Subject() domain.EntityID { return e.Who }

// TargetStatusSnapshot 是某个观察者请求的目标状态栏快照。
// Who 是接收者，Target 是被观察实体；二者不能混为一个 Subject，否则未来跨 AOI
// 或目标刚消失时，清空目标栏的回包会被场景按错误位置过滤。
type TargetStatusSnapshot struct {
	Who      domain.EntityID
	Target   domain.EntityID
	Statuses []TargetStatusView
}

func (e TargetStatusSnapshot) Subject() domain.EntityID { return e.Who }

// TargetStatusView 是目标栏展示所需的协议无关值。字段语义均由客户端
// GameHud.TgSt 的构造参数确定；场景没有证据的字段不得用状态号等值冒充。
type TargetStatusView struct {
	Icon       int32
	Beneficial bool
	RemainMS   int32
	Name       string
	Desc       string
}

// QuestAccepted 接了一个任务。只发给本人。
type QuestAccepted struct {
	Who   domain.EntityID
	Quest int32
	Name  string
}

func (e QuestAccepted) Subject() domain.EntityID { return e.Who }

// QuestItemObtained 是任务进行期间由指定怪物直接放入背包的专属收集品。
// 它不会生成地面实体；Have/Need 用于客户端任务浮层提示本次真实进度。
type QuestItemObtained struct {
	Who        domain.EntityID
	Quest      domain.QuestID
	Item       domain.ItemID
	Name       string
	Have, Need int32
}

func (e QuestItemObtained) Subject() domain.EntityID { return e.Who }

// QuestItemBlocked 表示任务专属物品本应获得，但权威背包没有空位。
type QuestItemBlocked struct {
	Who   domain.EntityID
	Quest domain.QuestID
	Item  domain.ItemID
	Name  string
}

func (e QuestItemBlocked) Subject() domain.EntityID { return e.Who }

// NpcTasks 是某个 NPC 对这个玩家展示的任务列表(客户端点了 NPC 之后要它)。
//
// 只发给本人: 同一个 NPC 对不同玩家展示的内容不一样 ——
// 有人还没接、有人做到一半、有人可以交了。
type NpcTasks struct {
	Who    domain.EntityID
	NPC    string
	Offers []domain.Offer
}

func (e NpcTasks) Subject() domain.EntityID { return e.Who }

// ActiveQuests 是玩家的任务本(进行中的全部任务)。只发给本人。
//
// 什么时候发: 进世界时发一次, 之后**每次任务本变化都要重发** ——
// 接了、交了、或者交物任务的物品数变了。客户端那边是整份覆盖, 不是增量。
type ActiveQuests struct {
	Who  domain.EntityID
	List []domain.ActiveQuest
	// Done 是**已完成**的任务号。
	//
	// 出处不是猜的: 0x8027 的 handler 收完两段之后尾调用
	// `TaskClient.SetState(第一段, 第二段, 0)`, 而 TaskClient 的字段就是
	//
	//	Active   List<ActiveTask>        ← 第一段
	//	Done     HashSet<System.Int32>   ← 第二段
	//
	// 客户端拿 Done 判断 NPC 头顶显示什么、任务能不能再接。
	// 不发的话做过的任务会重新显示成"可接"。
	Done []int32
}

func (e ActiveQuests) Subject() domain.EntityID { return e.Who }

// TitleSnapshot 是当前佩戴称号与已解锁称号的完整快照。客户端收到后整份覆盖
// CharData.title/ownedTitles，因此登录、解锁和切换称号后都必须发送。
type TitleSnapshot struct {
	Who     domain.EntityID
	Current string
	Owned   []string
}

func (e TitleSnapshot) Subject() domain.EntityID { return e.Who }

// QuestRewardItem 是完成提示里展示的一项实际奖励物品。
type QuestRewardItem struct {
	Name string
	Qty  int32
}

// QuestCompleted 交了一个任务。只发给本人。
// 经验、钱和物品仍由各自的权威事件/快照结算；这里携带同一笔结算的展示数据，
// 避免协议层重新解析 game_tasks.reward 的自然语言文本。
type QuestCompleted struct {
	Who   domain.EntityID
	Quest int32
	Name  string
	Exp   int64
	Honor int64
	Money domain.Money
	Items []QuestRewardItem
	Title string
}

func (e QuestCompleted) Subject() domain.EntityID { return e.Who }

// WorkStopReason 是收工的原因。客户端据此决定提示什么。
type WorkStopReason uint8

const (
	WorkStoppedByPlayer      WorkStopReason = iota // 自己停的
	WorkStoppedByAction                            // 动了/打了/放技能了
	WorkStoppedNoStamina                           // 耐力耗尽
	WorkStoppedByDeath                             // 死了
	WorkStoppedBagFull                             // 背包满了 —— 再挂下去产出只会被一件件丢掉
	WorkStoppedByCapture                           // 去抓宠物了
	WorkStoppedMissingOutfit                       // 中途脱下/更换了工作服
	WorkStoppedMissingTool                         // 中途脱下/更换了工作工具
	WorkStoppedToolBroken                          // 工作工具耐久耗尽
	WorkStoppedNoConsumable                        // 钓鱼等工种的逐次消耗品耗尽
)

// WorkStarted 开工。广播 —— 周围的人要看到「打扫卫生中的」这个状态。
type WorkStarted struct {
	Who   domain.EntityID
	Work  int32
	Title string
}

func (e WorkStarted) Subject() domain.EntityID { return e.Who }

// WorkStopped 收工。
type WorkStopped struct {
	Who     domain.EntityID
	Work    int32
	Reason  WorkStopReason
	Settled int32 // 这一轮结算了几次
}

func (e WorkStopped) Subject() domain.EntityID { return e.Who }

// WorkSettled 打工结算了一次。只发给本人 ——
// 经验另有 ExpGained 事件, 这条带的是"这次给了什么"和剩余耐力。
type WorkSettled struct {
	Who     domain.EntityID
	Work    int32
	Exp     int64
	Honor   int64
	Stamina int32
}

func (e WorkSettled) Subject() domain.EntityID { return e.Who }

// Rejected 是"你刚才那个请求不成立"。
//
// 存在的理由: 游戏层拒绝一个命令时不能直接扔 error 就算完 ——
// 客户端还在等回应, 不给回应它就一直卡着。所有"背包满了/等级不够/距离太远"
// 都走这条, 由协议层翻成对应的错误码或提示框。
type Rejected struct {
	Who    domain.EntityID
	Cmd    string // 被拒的命令名, 便于排查
	Reason RejectReason
}

func (e Rejected) Subject() domain.EntityID { return e.Who }

// RejectReason 是拒绝原因的枚举。用枚举不用字符串, 因为客户端的提示文本
// 是它自己的资源, 我们只能给码。
type RejectReason uint16

const (
	RejectUnknown RejectReason = iota
	RejectNotInGame
	RejectTooFar
	RejectLevelTooLow
	RejectBagFull
	RejectNoTarget
	RejectTargetDead
	RejectOnCooldown
	RejectNotEnoughMP
	RejectStaminaEmpty
	RejectNotEquippable   // 这东西不是装备
	RejectStatTooLow      // 六维不够(装备是属性门槛, 不是职业限制)
	RejectWrongSex        // 性别不符
	RejectSkillNotLearned // 没学这个技能
	RejectSkillNotUsable  // 技能存在但效果还没做(被动/buff/位移)
	RejectStunned         // 昏迷/冰冻/石化中, 动不了
	RejectSilenced        // 封印中, 放不了技能
	RejectLevelTooHigh    // 等级超过了任务上限
	RejectQuestActive     // 这个任务已经在进行中
	RejectQuestDone       // 这个任务已经做过了(且不可重复)
	RejectQuestNotActive  // 没接这个任务
	RejectQuestNotDone    // 任务条件还没达成
	RejectInstanceFull    // 这个副本的实例开满了
	RejectPartyRequired   // 第一版副本必须先组队
	RejectDungeonNotClear // 当前楼层仍有可战斗怪物
	RejectAlreadyWorking  // 已经在打工了
	RejectNotCapturable   // 这只怪抓不了(504 只宠里只有 18 只标了捕捉等级)
	RejectTargetNotWeak   // 目标血太满, 打残了才抓得动
	RejectNoCaptureTool   // 没有对应的捕捉道具
	RejectPetSlotsFull    // 宠物栏满了
	RejectNoPet           // 没有这只宠
	RejectPetAlreadyOut   // 已经放了一只出来
	RejectPetDead         // 这只宠还没救活
	RejectNotFood         // 这东西不是宠物食物
	RejectNoItem          // 背包里没有这件东西
	RejectWrongFoodTier   // 食物档位与当前饥渴度不符(0~50/51~75/76~100 三档)
	RejectPetStarving     // 宠物饿到不肯出战了
	RejectInvalid         // 参数非法(比如加点时试图减某个属性)
	RejectNotEnough       // 消耗品不够(比如自由点不足、金钱不足)
	RejectMaxLevel        // 已经满级(技能/装备强化等)
	RejectWrongProf       // 职业不符(这个技能不是你的职业)
	RejectAlreadyAlive    // 已经是活着的状态(复活时)
	RejectPetUnhatched    // 未孵化宠物不能出战
	RejectPetAlreadyHatched
	RejectPetNoTrust            // 信赖为 0，必定不出战
	RejectPetRefused            // 低信赖时本次拒绝出战
	RejectPetLearningPending    // 已有一枚领悟道具等待升级结算
	RejectPetCannotLearn        // 该宠物不能领悟道具指定的技能
	RejectWorkOutfitMissing     // 未实际穿着对应生活工作服
	RejectWorkToolMissing       // 未实际装备对应生活工具
	RejectWorkToolBroken        // 生活工具耐久为 0
	RejectWorkConsumableMissing // 缺少鱼饵等逐次消耗品
	RejectRiding                // 骑乘状态下不能执行该操作
	RejectRequiresInvisible     // 技能要求先进入隐身状态
	RejectSkillPrerequisite     // 尚未学习技能定义要求的前置技能
	RejectPetLifeSkillsFull     // 生活技能栏已满
	RejectPetFightSkillsFull    // 战斗技能栏已满
)

// rejectNames 让日志说人话。
//
// 日志使用稳定的原因名称，便于查看拒绝条件。
var rejectNames = map[RejectReason]string{
	RejectUnknown:               "Unknown",
	RejectNotInGame:             "NotInGame",
	RejectTooFar:                "TooFar",
	RejectLevelTooLow:           "LevelTooLow",
	RejectBagFull:               "BagFull",
	RejectNoTarget:              "NoTarget",
	RejectTargetDead:            "TargetDead",
	RejectOnCooldown:            "OnCooldown",
	RejectNotEnoughMP:           "NotEnoughMP",
	RejectStaminaEmpty:          "StaminaEmpty",
	RejectNotEquippable:         "NotEquippable",
	RejectStatTooLow:            "StatTooLow",
	RejectWrongSex:              "WrongSex",
	RejectSkillNotLearned:       "SkillNotLearned",
	RejectSkillNotUsable:        "SkillNotUsable",
	RejectStunned:               "Stunned",
	RejectSilenced:              "Silenced",
	RejectLevelTooHigh:          "LevelTooHigh",
	RejectQuestActive:           "QuestActive",
	RejectQuestDone:             "QuestDone",
	RejectQuestNotActive:        "QuestNotActive",
	RejectQuestNotDone:          "QuestNotDone",
	RejectInstanceFull:          "InstanceFull",
	RejectPartyRequired:         "PartyRequired",
	RejectDungeonNotClear:       "DungeonNotClear",
	RejectAlreadyWorking:        "AlreadyWorking",
	RejectNotCapturable:         "NotCapturable",
	RejectTargetNotWeak:         "TargetNotWeak",
	RejectNoCaptureTool:         "NoCaptureTool",
	RejectPetSlotsFull:          "PetSlotsFull",
	RejectNoPet:                 "NoPet",
	RejectPetAlreadyOut:         "PetAlreadyOut",
	RejectPetDead:               "PetDead",
	RejectNotFood:               "NotFood",
	RejectNoItem:                "NoItem",
	RejectWrongFoodTier:         "WrongFoodTier",
	RejectPetStarving:           "PetStarving",
	RejectInvalid:               "Invalid",
	RejectNotEnough:             "NotEnough",
	RejectMaxLevel:              "MaxLevel",
	RejectWrongProf:             "WrongProf",
	RejectAlreadyAlive:          "AlreadyAlive",
	RejectPetUnhatched:          "PetUnhatched",
	RejectPetAlreadyHatched:     "PetAlreadyHatched",
	RejectPetNoTrust:            "PetNoTrust",
	RejectPetRefused:            "PetRefused",
	RejectPetLearningPending:    "PetLearningPending",
	RejectPetCannotLearn:        "PetCannotLearn",
	RejectPetLifeSkillsFull:     "PetLifeSkillsFull",
	RejectPetFightSkillsFull:    "PetFightSkillsFull",
	RejectWorkOutfitMissing:     "WorkOutfitMissing",
	RejectWorkToolMissing:       "WorkToolMissing",
	RejectWorkToolBroken:        "WorkToolBroken",
	RejectWorkConsumableMissing: "WorkConsumableMissing",
	RejectRiding:                "Riding",
	RejectRequiresInvisible:     "RequiresInvisible",
	RejectSkillPrerequisite:     "SkillPrerequisite",
}

func (r RejectReason) String() string {
	if n, ok := rejectNames[r]; ok {
		return n
	}
	return fmt.Sprintf("RejectReason(%d)", uint16(r))
}

// ── 宠物 ──

// PetCaptured 捕捉成功。
type PetCaptured struct {
	Who  domain.EntityID
	Inst domain.PetInstID
	Pet  int32  // 种族号
	Name string // 显示名
}

func (e PetCaptured) Subject() domain.EntityID { return e.Who }

// PetCaptureFailed 捕捉掷失败了。**目标没死也没跑** ——
// 失败只是这一次没抓住, 不是把怪打没了。
type PetCaptureFailed struct {
	Who    domain.EntityID
	Target domain.EntityID
	Rate   int32 // 这次的成功率(百分比), 给客户端显示"差一点"用
}

func (e PetCaptureFailed) Subject() domain.EntityID { return e.Who }

// PetSummoned 宠物放出来了。
type PetSummoned struct {
	Who    domain.EntityID // 主人
	Entity domain.EntityID // 宠物在场上的实体 id
	Inst   domain.PetInstID
}

func (e PetSummoned) Subject() domain.EntityID { return e.Who }

// PetRecalled 宠物收回去了。
type PetRecalled struct {
	Who    domain.EntityID
	Entity domain.EntityID
	Reason PetRecallReason
}

func (e PetRecalled) Subject() domain.EntityID { return e.Who }

// PetRecallReason 是收回宠物的原因。
type PetRecallReason uint8

const (
	PetRecalledByPlayer     PetRecallReason = iota // 自己收的
	PetRecalledByDeath                             // 宠物被打死了
	PetRecalledByOwnerLeft                         // 主人离场/换图
	PetRecalledByOwnerDeath                        // 主人死了
	PetRecalledByStarve                            // 饿到头了, 不肯出战
	PetRecalledByDistrust                          // 出战中信赖降到 0
	PetRecalledByEscape                            // 逃逸技能在体力低于 10% 时主动回栏
)

// PetExpGained 宠物涨经验。**不从主人那份里扣** ——
// 扣的话带宠物练级更慢, 那就没人带宠物了。
type PetExpGained struct {
	Who   domain.EntityID // 主人
	Inst  domain.PetInstID
	Exp   int64
	Level int32
	Ups   int32 // 这次升了几级
}

func (e PetExpGained) Subject() domain.EntityID { return e.Who }

// PartyChanged 队伍号变了(加入/退出/被踢/解散)。
//
// 只带号不带名册: 名册是会话层的东西, 场景手上没有跨图的成员列表。
// 客户端要的完整队伍面板由会话层单独下发。
type PartyChanged struct {
	Who   domain.EntityID
	Party uint32
}

func (e PartyChanged) Subject() domain.EntityID { return e.Who }

// PetFed 喂了一次宠物。
type PetFed struct {
	Who  domain.EntityID
	Inst domain.PetInstID
	// Starve/Trust 是喂完之后的值; Delta* 是这一次变了多少。
	// 两个都给: 客户端要飘"-3"也要刷面板上的当前值。
	Starve, Trust           int32
	DeltaStarve, DeltaTrust int32
}

func (e PetFed) Subject() domain.EntityID { return e.Who }

// PetStarving 宠物饿到头了。
type PetStarving struct {
	Who    domain.EntityID
	Inst   domain.PetInstID
	Starve int32
}

func (e PetStarving) Subject() domain.EntityID { return e.Who }

// PetPickedUp 宠物替主人捡了一件东西。
//
// 发给主人而不是广播: 别人不需要知道你的宠捡了什么,
// 而"谁捡走了我看中的那件"恰恰是最容易吵起来的事。
type PetPickedUp struct {
	Who  domain.EntityID // 主人
	Pet  domain.EntityID
	Item int32
	Name string
}

func (e PetPickedUp) Subject() domain.EntityID { return e.Who }

// NewbieTextNotice displays dynamic text in the native bottom-right NewbieTipBox.
type NewbieTextNotice struct {
	Who  domain.EntityID
	Text string
}

func (e NewbieTextNotice) Subject() domain.EntityID { return e.Who }
