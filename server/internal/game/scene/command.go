package scene

import (
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Command 是从场景外投进来的一个请求。命名一律用祈使式("请求做"),
// 与事件的过去式("已发生")对称。
//
// 命令是**纯数据**, 不带行为 —— 处理逻辑集中在 dispatch.go 的那个 switch 里。
// 这样"外面能让场景做哪些事"是一份可以一眼读完的清单, 而不是散落各处的方法。
type Command interface{ CmdName() string }

// Enter 玩家进入本场景。
//
// EntityID 由**调用方**在投递前分配(Router.NewPlayerID), 不是场景分配的。
// 这样会话层立刻就知道自己的实体 id, 后续命令可以直接带上, 不需要等回调。
type Enter struct {
	ID   domain.EntityID
	Char *domain.Character
	// Runtime 非 nil 表示这是跨场景所有权交接，不是一次新的登录。它只携带
	// 玩家运行态的值快照，不把 *Entity 暴露给另一个场景 goroutine。
	Runtime *entity.PlayerRuntime
	// ClientMapReady 表示客户端已经自行加载了目标地图。这类交接只需要让服务端
	// 追上场景所有权并下发目标图实体，不能再发 Teleported 让客户端重复清图。
	ClientMapReady bool
	// Party 是跨场景名册当前给这个角色的队伍号。场景只保存这个判定值，
	// 不持有全服名册；传送交接时必须原样带走。
	Party domain.PartyID
	// DungeonElapsed 是多层副本从首层创建起已经消耗的帧数。目标层用它
	// 对齐自己的限时基准，跨层不能把 15 分钟倒计时重新计时。
	DungeonElapsed domain.Tick
	// Bag 是这个角色的背包, Worn 是身上穿的。nil 则场景建空的 ——
	// 那只在测试里发生, 生产路径一定从存档读出来。
	Bag       *domain.Bag
	Worn      *domain.EquipSet
	ChangeSet *domain.ChangeSet
	Warehouse *domain.Warehouse
	Wardrobe  *domain.Wardrobe
	Stall     *domain.Stall
	// SeenTips 是服务端持久化的 NewbieTipBox 已读编号。只在初次进世界时下发，
	// 且必须排在 SelfEntered 之后、技能/宠物快照之前。
	SeenTips []int32
	Sink     event.Sink
	// FinalSaver 随玩家进入并跨场景转移，只在整个在线生命周期真正结束时使用。
	// 周期存档仍走 Scene.saver，不能提前兑现角色租约。
	FinalSaver Saver
}

func (Enter) CmdName() string { return "Enter" }

type WardrobeStore struct {
	ID   domain.EntityID
	Tab  uint8
	Slot int32
}

func (WardrobeStore) CmdName() string { return "WardrobeStore" }

type WardrobeWear struct {
	ID   domain.EntityID
	Item domain.ItemID
}

func (WardrobeWear) CmdName() string { return "WardrobeWear" }

type WardrobeRemove struct {
	ID   domain.EntityID
	Item domain.ItemID
}

func (WardrobeRemove) CmdName() string { return "WardrobeRemove" }

type WardrobeMove struct {
	ID   domain.EntityID
	Item domain.ItemID
	To   int32
}

func (WardrobeMove) CmdName() string { return "WardrobeMove" }

type CreateStall struct {
	ID    domain.EntityID
	Type  domain.StallType
	Name  string
	Reply chan<- StallReserveResult
}

func (CreateStall) CmdName() string { return "CreateStall" }

type ReserveStallAdd struct {
	ID    domain.EntityID
	Tab   uint8
	Slot  int32
	Item  domain.ItemID
	Count int32
	Price int64
	Reply chan<- StallReserveResult
}

func (ReserveStallAdd) CmdName() string { return "ReserveStallAdd" }

type ReserveStallDel struct {
	ID    domain.EntityID
	Index int32
	Reply chan<- StallReserveResult
}

func (ReserveStallDel) CmdName() string { return "ReserveStallDel" }

type ReserveStallEnd struct {
	ID    domain.EntityID
	Reply chan<- StallReserveResult
}

func (ReserveStallEnd) CmdName() string { return "ReserveStallEnd" }

type BrowseStall struct {
	ID    domain.EntityID
	Owner domain.EntityID
}

func (BrowseStall) CmdName() string { return "BrowseStall" }

type ReserveStallDeal struct {
	ID            domain.EntityID
	Owner         domain.EntityID
	Index         int32
	Count         int32
	Tab           uint8
	Slot          int32
	ExpectedItem  domain.ItemID
	ExpectedPrice int64
	Reply         chan<- StallReserveResult
}

func (ReserveStallDeal) CmdName() string { return "ReserveStallDeal" }

type StallPlayerReservation struct {
	IncomingPetUID int64
	PetsKnown      bool
	BeforePets     []domain.PetInstance
	ID             domain.EntityID
	BeforeBag      *domain.Bag
	BeforeMoney    domain.Money
	BeforeStall    *domain.Stall
}

type StallReservation struct {
	Players []StallPlayerReservation
	Owner   domain.EntityID
	Create  bool
	Close   bool
}

type StallReserveResult struct {
	Reservation  StallReservation
	Snapshots    []domain.Snapshot
	Reason       string
	ActorMessage string
	OwnerMessage string
}

type FinalizeStallReservation struct {
	Reservation  StallReservation
	Commit       bool
	ActorMessage string
	OwnerMessage string
	Done         chan<- struct{}
}

func (FinalizeStallReservation) CmdName() string { return "FinalizeStallReservation" }

type ChangeHair struct {
	ID     domain.EntityID
	Mode   domain.HairChangeMode
	Target uint8
}

func (ChangeHair) CmdName() string { return "ChangeHair" }

type ReserveFamilyCreate struct {
	ID    domain.EntityID
	Reply chan<- FamilyReserveResult
}

func (ReserveFamilyCreate) CmdName() string { return "ReserveFamilyCreate" }

type ReserveFamilyMail struct {
	ID    domain.EntityID
	Reply chan<- FamilyReserveResult
}

func (ReserveFamilyMail) CmdName() string { return "ReserveFamilyMail" }

type FamilyReservation struct {
	ID          domain.EntityID
	BeforeBag   *domain.Bag
	BeforeMoney domain.Money
	BeforeCaiyu int64
}

type FamilyReserveResult struct {
	Reservation FamilyReservation
	Snapshot    domain.Snapshot
	Reason      string
	Consumed    string
	Caiyu       int64
	Credited    int64
	Stack       domain.Stack
}

type ReserveFamilyStashDeposit struct {
	ID      domain.EntityID
	BagTab  uint8
	BagSlot int32
	Reply   chan<- FamilyReserveResult
}

func (ReserveFamilyStashDeposit) CmdName() string { return "ReserveFamilyStashDeposit" }

type ReserveFamilyStashWithdraw struct {
	ID    domain.EntityID
	Stack domain.Stack
	Reply chan<- FamilyReserveResult
}

func (ReserveFamilyStashWithdraw) CmdName() string { return "ReserveFamilyStashWithdraw" }

type ShowFamilyStash struct {
	ID    domain.EntityID
	Stash domain.FamilyStash
}

func (ShowFamilyStash) CmdName() string { return "ShowFamilyStash" }

type FinalizeFamilyReservation struct {
	Reservation FamilyReservation
	Commit      bool
	Done        chan<- struct{}
}

func (FinalizeFamilyReservation) CmdName() string { return "FinalizeFamilyReservation" }

// Leave 玩家离开本场景(下线/断线/传送走)。
type Leave struct {
	ID     domain.EntityID
	Reason string
	// FinalSaver 非 nil 时接管这次离场的最终快照。生产会话传入的是角色租约：
	// 它仍然只做内存排队、立即返回，但要等 WriteBack 真正提交成功才开放重进。
	// nil 保留原有 Saver 行为，供关服、内部调用和未接 gate 的测试路径使用。
	FinalSaver Saver
}

func (Leave) CmdName() string { return "Leave" }

// MoveTo 玩家移动到某坐标。
//
// 服务端只做**合法性**判断(速度上限、是否可通行), 不做逐帧插值 ——
// 客户端容差常量已知(Snap 260px / Tol 40px / 追赶 1.6×), 逐帧重算是白费算力。
type MoveTo struct {
	ID   domain.EntityID
	To   domain.Pos
	Dir  uint8
	Stop bool
	// ReceivedAt 是会话层收到真实 0x1017 的时刻，用于按速度校验两次上报的位移。
	// 零值仅供场景内部的可信移动与旧测试夹具，不走客户端限速。
	ReceivedAt time.Time
}

func (MoveTo) CmdName() string { return "MoveTo" }

// Attack 玩家对目标出手。是否够得着、冷却好没好由场景判定。
//
// 命令只说「要打谁、怎么打」,不带任何结算结果 —— 客户端说了不算,
// 它连自己这一下中没中都不知道。
type Attack struct {
	ID         domain.EntityID
	Target     domain.EntityID
	Blow       combat.Blow // 零值 = 普通物理平砍
	ReceivedAt time.Time   // 会话收到 0x100c 的时刻；用于消除 100ms 主帧带来的相位抖动
}

func (Attack) CmdName() string { return "Attack" }

// QueryTargetStatus 请求刷新目标栏的 buff/debuff 列表。客户端切换目标时立即查，
// 同一目标约每秒轮询一次；它不是攻击，也不改变任何世界状态。
type QueryTargetStatus struct {
	ID     domain.EntityID
	Target domain.EntityID
}

func (QueryTargetStatus) CmdName() string { return "QueryTargetStatus" }

// RequestNewbieTip 将一个已由上游入口精确判定的客户端操作带入场景顺序点。
// 场景仍会确认玩家实体存在；它不接受客户端自报的 tipId，Tip 只能由服务端
// 固定映射构造。
type RequestNewbieTip struct {
	ID  domain.EntityID
	Tip int32
}

func (RequestNewbieTip) CmdName() string { return "RequestNewbieTip" }

// SetResting 接收客户端已有的坐下/起身动作。客户端只声明动作状态；回血的
// 时间与数值完全由场景中的数据库规则结算。
type SetResting struct {
	ID domain.EntityID
	On bool
}

func (SetResting) CmdName() string { return "SetResting" }

// ShowEmote 请求播放客户端内置表情。Face 是现有 EmotePicker 的零基索引。
type ShowEmote struct {
	ID   domain.EntityID
	Face int32
}

func (ShowEmote) CmdName() string { return "ShowEmote" }

// InspectPlayer 请求查看同场景玩家的公开角色属性与已穿装备。
type InspectPlayer struct {
	ID     domain.EntityID
	Target domain.EntityID
}

func (InspectPlayer) CmdName() string { return "InspectPlayer" }

// ValidateTradeOffer resolves client tab/slot/count references inside the scene
// actor that exclusively owns the bag. The reply contains value copies only.
type ValidateTradeOffer struct {
	ID    domain.EntityID
	Money int64
	Items []TradeOfferRef
	Reply chan<- TradeOfferResult
}

func (ValidateTradeOffer) CmdName() string { return "ValidateTradeOffer" }

type TradeOfferRef struct {
	Tab   uint8
	Slot  int32
	Count int32
}

type TradeOfferItem struct {
	Pet     *domain.PetItemInfo
	Tab     uint8
	Slot    int32
	Stack   domain.Stack
	Name    string
	Info    string
	Quality uint8
}

type TradeOfferResult struct {
	OK    bool
	Money int64
	Items []TradeOfferItem
}

// ReserveTradeSettlement 在同一场景 actor 内重新核验双方已经展示的报价，
// 并把两边背包/金钱切到候选状态。数据库提交完成前 TradeBusy 会阻止第二次结算。
type ReserveTradeSettlement struct {
	A, B   domain.EntityID
	AOffer TradeSettlementOffer
	BOffer TradeSettlementOffer
	Reply  chan<- TradeSettlementResult
	Cancel <-chan struct{} // caller timed out before accepting the reservation
}

func (ReserveTradeSettlement) CmdName() string { return "ReserveTradeSettlement" }

type TradeSettlementOffer struct {
	Money int64
	Items []TradeOfferItem
}

type TradeSettlementPlayer struct {
	BeforePets  []domain.PetInstance
	ID          domain.EntityID
	BeforeBag   *domain.Bag
	BeforeMoney domain.Money
}

type TradeSettlementReservation struct {
	Players []TradeSettlementPlayer
	// Result is a dedicated one-shot completion path. It remains usable when the
	// scene mailbox is full or the router has begun draining during shutdown.
	Result chan<- bool
	Done   <-chan struct{}
}

type TradeSettlementResult struct {
	Reservation TradeSettlementReservation
	Snapshots   []domain.Snapshot
	Reason      string
}

type FinalizeTradeSettlement struct {
	Reservation TradeSettlementReservation
	Commit      bool
	Done        chan<- struct{}
}

func (FinalizeTradeSettlement) CmdName() string { return "FinalizeTradeSettlement" }

// OpenShop 打开 NPC SellList 对应的服务端库存。
type OpenShop struct {
	ID   domain.EntityID
	Shop int32
}

func (OpenShop) CmdName() string { return "OpenShop" }

// BuyItem 从最近成功打开的商店购买物品。
type BuyItem struct {
	ID    domain.EntityID
	Item  domain.ItemID
	Count int32
}

func (BuyItem) CmdName() string { return "BuyItem" }

// SellItem 把权威背包指定格的物品卖给最近成功打开的商店。
type SellItem struct {
	ID        domain.EntityID
	Tab       uint8
	Slot      int
	Count     int32
	Item      domain.ItemID
	Confirmed bool
}

func (SellItem) CmdName() string { return "SellItem" }

// MoveBagItem 把当前分页的一格物品移到另一格。Tab 只用于复核来源物品，
// 格号和物品内容仍以场景内权威背包为准。
type MoveBagItem struct {
	ID       domain.EntityID
	Tab      uint8
	From, To int
}

func (MoveBagItem) CmdName() string { return "MoveBagItem" }

// SortBag 整理客户端当前显示的背包分页。
type SortBag struct {
	ID  domain.EntityID
	Tab uint8
}

func (SortBag) CmdName() string { return "SortBag" }

type OpenWarehouse struct{ ID domain.EntityID }

func (OpenWarehouse) CmdName() string { return "OpenWarehouse" }

type WarehouseMove struct {
	ID           domain.EntityID
	Dir          uint8
	BagTab       uint8
	FromSlot     int32
	WarehouseTab uint8
	Count        int32
	ToSlot       int32
}

func (WarehouseMove) CmdName() string { return "WarehouseMove" }

type WarehouseMoney struct {
	ID     domain.EntityID
	Mode   uint8
	Amount int64
}

type ExpandWarehouse struct{ ID domain.EntityID }

func (ExpandWarehouse) CmdName() string { return "ExpandWarehouse" }

type OpenRack struct{ ID domain.EntityID }

func (OpenRack) CmdName() string { return "OpenRack" }

type RackReservation struct {
	ID          domain.EntityID
	BeforeBag   *domain.Bag
	BeforeCaiyu int64
	AddedPets   []domain.PetInstID
}

type RackReserveResult struct {
	Reservation RackReservation
	Snapshot    domain.Snapshot
	Reason      string
	Item        domain.ItemID
	Name        string
	UnitPrice   int32
	Bundle      int32
	Shares      int32
	Total       int64
}

type ReserveRackPurchase struct {
	ID     domain.EntityID
	Item   domain.ItemID
	Source uint8
	Count  int32
	Reply  chan<- RackReserveResult
}

func (ReserveRackPurchase) CmdName() string { return "ReserveRackPurchase" }

type ReserveRackRefund struct {
	ID     domain.EntityID
	Offer  domain.RackRefundOffer
	Shares int32
	Credit int64
	Reply  chan<- RackReserveResult
}

func (ReserveRackRefund) CmdName() string { return "ReserveRackRefund" }

type FinalizeRackReservation struct {
	Reservation    RackReservation
	Commit         bool
	RefreshCatalog bool
	Message        string
	Done           chan<- struct{}
}

func (FinalizeRackReservation) CmdName() string { return "FinalizeRackReservation" }

type ShowRackRefunds struct {
	ID     domain.EntityID
	Rule   domain.RackRefundRule
	Offers []domain.RackRefundOffer
}

func (ShowRackRefunds) CmdName() string { return "ShowRackRefunds" }

type OpenFitting struct {
	ID      domain.EntityID
	Kind    uint8
	Part    string
	Search  string
	Page    int32
	PerPage int32
	Paged   bool
}

func (OpenFitting) CmdName() string { return "OpenFitting" }

type FittingDetails struct {
	ID    domain.EntityID
	Items []domain.ItemID
}

func (FittingDetails) CmdName() string { return "FittingDetails" }

type ReserveDeposit struct {
	ID     domain.EntityID
	Amount int32
	Reply  chan<- FamilyReserveResult
}

func (ReserveDeposit) CmdName() string { return "ReserveDeposit" }

func (WarehouseMoney) CmdName() string { return "WarehouseMoney" }

type WarehouseSplit struct {
	ID    domain.EntityID
	Tab   uint8
	Slot  int32
	Count int32
}

func (WarehouseSplit) CmdName() string { return "WarehouseSplit" }

type WarehouseRearrange struct {
	ID               domain.EntityID
	FromTab, ToTab   uint8
	FromSlot, ToSlot int32
}

func (WarehouseRearrange) CmdName() string { return "WarehouseRearrange" }

type SortWarehouse struct {
	ID  domain.EntityID
	Tab uint8
}

func (SortWarehouse) CmdName() string { return "SortWarehouse" }

// DropBagItem 把一格权威背包实例放到角色附近的地面。
type DropBagItem struct {
	ID        domain.EntityID
	Tab       uint8
	Slot      int
	Item      domain.ItemID
	At        domain.Pos
	Confirmed bool // 客户端是否已经走过本地确认框；不作为服务端授权依据
}

func (DropBagItem) CmdName() string { return "DropBagItem" }

// SplitBagItem 从可堆叠物品中拆出 Count 个，占用一个新的空格。
type SplitBagItem struct {
	ID    domain.EntityID
	Tab   uint8
	Slot  int
	Count int32
}

func (SplitBagItem) CmdName() string { return "SplitBagItem" }

// SetItemLock 只携带已经由会话层完成账号安全校验的目标引用。场景仍必须按
// tab/slot/item 三元组复核权威背包，不能信任会话看到的客户端参数。
type SetItemLock struct {
	ID   domain.EntityID
	Tab  uint8
	Slot int32
	Item domain.ItemID
	On   bool
}

func (SetItemLock) CmdName() string { return "SetItemLock" }

// Repair 请求服务端为单件普通/特殊修理或身上全部普通修理生成报价。
type Repair struct {
	ID                    domain.EntityID
	Mode, TargetKind, Tab uint8
	Slot                  int32
}

func (Repair) CmdName() string { return "Repair" }

// RepairConfirm 确认最近一次同参数报价；费用与物品状态不由客户端回传。
type RepairConfirm struct {
	ID                    domain.EntityID
	Mode, TargetKind, Tab uint8
	Slot                  int32
}

func (RepairConfirm) CmdName() string { return "RepairConfirm" }

// StartWork 开工。
type StartWork struct {
	ID   domain.EntityID
	Work domain.WorkID
}

func (StartWork) CmdName() string { return "StartWork" }

// StopWork 收工。
type StopWork struct {
	ID domain.EntityID
}

func (StopWork) CmdName() string { return "StopWork" }

// EnterDungeon 进副本。
//
// MapID 是副本那张图的 id; NPC 非空时服务端会确认玩家确实站在那个 NPC 面前。
// 实例由服务端挑或新开, 客户端指定不了 —— 否则它能把自己塞进别人的实例。
type EnterDungeon struct {
	ID    domain.EntityID
	MapID int32
	NPC   string
}

func (EnterDungeon) CmdName() string { return "EnterDungeon" }

// AcceptQuest 接任务。
//
// NPC 名字由客户端带上, 但服务端会**自己确认玩家确实站在那个 NPC 旁边** ——
// 不确认的话客户端可以在任何地方把全世界的任务一口气接完, 而奖励是经验和钱。
type AcceptQuest struct {
	ID    domain.EntityID
	Quest domain.QuestID
	NPC   string
}

func (AcceptQuest) CmdName() string { return "AcceptQuest" }

// NpcTasks 问「这个 NPC 身上有什么任务」。客户端点 NPC 时发。
//
// 带 NPC **名字**而不是实体 id: 上行包(0x1032)给的就是名字,
// 而任务表也是按名字关联 NPC 的(game_tasks.npc 存的是名字不是 id)。
type NpcTasks struct {
	ID  domain.EntityID
	NPC string
}

func (NpcTasks) CmdName() string { return "NpcTasks" }

// AbandonQuest 放弃任务。上行包(0x1034)只带一个任务号, 不需要 NPC ——
// 放弃是任务界面上的操作, 不是对着 NPC 做的。
type AbandonQuest struct {
	ID    domain.EntityID
	Quest domain.QuestID
}

func (AbandonQuest) CmdName() string { return "AbandonQuest" }

// CompleteQuest 交任务。交给谁由任务定义决定, 客户端说了不算。
type CompleteQuest struct {
	ID    domain.EntityID
	Quest domain.QuestID
}

func (CompleteQuest) CmdName() string { return "CompleteQuest" }

// UseSkill 释放技能。
//
// Target 为 0 表示"对自己/以自己为中心"。打谁由服务端按技能的目标类型判 ——
// 让客户端指定等于让它决定"用治疗术奶怪"。
type UseSkill struct {
	ID     domain.EntityID
	Skill  domain.SkillID
	Target domain.EntityID
	// ReceivedAt 是网关收到 0x1013 的时刻。弹道命中从这个释放时刻计，不能把
	// 命令等到下一个 100ms 场景帧的排队时间额外算进飞行时间。
	ReceivedAt time.Time
	// At 非 nil 表示客户端通过 UseSkillAt 明确给了落点；范围技能以此为中心。
	// nil 是普通 UseSkill，由目标实体或施法者位置决定中心。
	At *domain.Pos
}

func (UseSkill) CmdName() string { return "UseSkill" }

// UseItem 使用普通自用物品。背包双击带精确 Tab/BagSlot；快捷栏按客户端
// 协议带 Tab=0xff、BagSlot=-1，由场景从背包第一格同类物品解析实际槽位。
type UseItem struct {
	ID      domain.EntityID
	Item    domain.ItemID
	Tab     uint8
	BagSlot int
}

func (UseItem) CmdName() string { return "UseItem" }

type ApplyAvatar struct {
	ID         domain.EntityID
	BagTab     uint8
	BagSlot    int32
	TargetSlot int32
	Kind       uint8
	TargetKind uint8
	TargetTab  uint8
}

func (ApplyAvatar) CmdName() string { return "ApplyAvatar" }

// Equip 把背包第 BagSlot 格的装备穿上。目标槽位由装备自己决定, 不由客户端指定 ——
// 让客户端指定槽位等于让它决定"把剑戴在头上"。
type Equip struct {
	ID      domain.EntityID
	BagSlot int
}

func (Equip) CmdName() string { return "Equip" }

// Unequip 脱下某个槽位的装备, 放回背包。
type Unequip struct {
	ID   domain.EntityID
	Slot domain.EquipSlot
}

func (Unequip) CmdName() string { return "Unequip" }

type ChangeSetItem struct {
	ID      domain.EntityID
	Put     bool
	BagTab  uint8
	BagSlot int
	Cell    domain.EquipSlot
}

func (ChangeSetItem) CmdName() string { return "ChangeSetItem" }

type ChangeSetSwap struct{ ID domain.EntityID }

func (ChangeSetSwap) CmdName() string { return "ChangeSetSwap" }

// PickUp 拾取地上的东西。
type PickUp struct {
	ID     domain.EntityID
	Target domain.EntityID // 掉落物实体
}

func (PickUp) CmdName() string { return "PickUp" }

// OpenTransport 请求打开 NPC 的传送目的地列表。
type OpenTransport struct {
	ID   domain.EntityID
	List int32
}

func (OpenTransport) CmdName() string { return "OpenTransport" }

// ChooseTransport 选择先前已成功打开的 NPC 传送列表中的零基目的地。
type ChooseTransport struct {
	ID    domain.EntityID
	List  int32
	Index int32
}

func (ChooseTransport) CmdName() string { return "ChooseTransport" }

// Teleport 把玩家送到另一个场景。
//
// 由场景自己在踩到传送门时投, 也可以由复活流程/GM 直接投。
// 目标场景不存在会被懒加载建出来。
type Teleport struct {
	ID             domain.EntityID
	To             domain.SceneID
	At             domain.Pos
	Dir            uint8
	ClientMapReady bool
}

func (Teleport) CmdName() string { return "Teleport" }

// Inspect 在场景 goroutine 内执行一段只读逻辑, 完成后关闭 Done。
//
// ⚠️ 这是唯一能从外部看到场景内部状态的口子, 存在的理由只有两个: 测试与 GM 工具。
// **业务代码不许用** —— 一旦用它读状态, "场景独占实体"这条铁律就等于没有了。
// 回调里不许改任何东西, 也不许把 *Scene 或 *entity.Entity 存到外面。
type Inspect struct {
	Fn   func(*Scene)
	Done chan struct{}
}

func (Inspect) CmdName() string { return "Inspect" }

// CapturePet 对一只怪下手抓。
type CapturePet struct {
	ID     domain.EntityID
	Target domain.EntityID
	Tool   domain.ItemID // 客户端 0x102b 的 itemId；场景仍以宠物定义和权威背包复核
}

func (CapturePet) CmdName() string { return "CapturePet" }

// SummonPet 把一只宠放出来。
type SummonPet struct {
	ID   domain.EntityID
	Inst domain.PetInstID
}

func (SummonPet) CmdName() string { return "SummonPet" }

// SummonPetAt 按**宠物栏槽位**放出一只宠。
//
// 与 SummonPet 分开是因为来源不同：客户端手上只有 0x800b 发下去的那份列表，
// 它回报的是那份列表的下标；实例号只有服务端知道。把翻译放在场景里，
// 会话层就不需要认识角色的宠物栏。
type SummonPetAt struct {
	ID   domain.EntityID
	Slot int32
}

func (SummonPetAt) CmdName() string { return "SummonPetAt" }

// TogglePetAt 是宠物栏双击的真实入口：未出战时放出该槽，正在出战同一槽时收回。
type TogglePetAt struct {
	ID   domain.EntityID
	Slot int32
}

func (TogglePetAt) CmdName() string { return "TogglePetAt" }

// RecallPet 把放出来的宠收回去。
type RecallPet struct {
	ID domain.EntityID
}

func (RecallPet) CmdName() string { return "RecallPet" }

// HatchPetAt 按客户端宠物栏槽位孵化未孵化宠物。
type HatchPetAt struct {
	ID   domain.EntityID
	Slot int32
}

func (HatchPetAt) CmdName() string { return "HatchPetAt" }

// SetPetSkill 设置当前出战宠物唯一启用的辅助技能。效果本期不结算。
type SetPetSkill struct {
	ID    domain.EntityID
	Skill domain.SkillID
}

func (SetPetSkill) CmdName() string { return "SetPetSkill" }

// UsePetSkill 是宠物技能面板双击与快捷栏点击的真实入口。普通辅助技能只切换
// 当前启用项；坐骑技能还会切换骑乘状态。
type UsePetSkill struct {
	ID    domain.EntityID
	Skill domain.SkillID
}

func (UsePetSkill) CmdName() string { return "UsePetSkill" }

// AddPetPoint consumes one authoritative free point on the current deployed pet.
type AddPetPoint struct {
	ID            domain.EntityID
	AttributeCode int32
}

func (AddPetPoint) CmdName() string { return "AddPetPoint" }

// RenamePet changes the current deployed pet's display name.
type RenamePet struct {
	ID   domain.EntityID
	Name string
}

func (RenamePet) CmdName() string { return "RenamePet" }

// SetPetShown applies the source petShow checkbox. False recalls an ordinary shown pet; true
// dismounts and reveals the current riding pet.
type SetPetShown struct {
	ID   domain.EntityID
	Show bool
}

func (SetPetShown) CmdName() string { return "SetPetShown" }

// SwapPetSlots reorders two visible pet-book slots.
type SwapPetSlots struct {
	ID       domain.EntityID
	From, To int32
}

func (SwapPetSlots) CmdName() string { return "SwapPetSlots" }

// SetParty 更新一个玩家的队伍号。
//
// **由会话层在名册变动后投进来。** 场景自己不管名册 ——
// 队伍是跨场景的，而场景只拥有自己这张图上的实体。
type SetParty struct {
	ID    domain.EntityID
	Party domain.PartyID
}

func (SetParty) CmdName() string { return "SetParty" }

// RefreshParty 不改变场景判定值，只要求该成员的会话重建 0x8033（队友血量、
// 等级、外观或队长变化时使用）。
type RefreshParty struct {
	ID domain.EntityID
}

func (RefreshParty) CmdName() string { return "RefreshParty" }

// FeedPet 喂宠物。
type FeedPet struct {
	ID   domain.EntityID
	Inst domain.PetInstID
	Item domain.ItemID
}

func (FeedPet) CmdName() string { return "FeedPet" }

// AllocPoints 加点(分配自由属性点到六维)。
//
// 客户端发的是**六维增量**(EquipDlg.pend[] 累加数组), 不是绝对值。服务端验证:
// ① 六项增量都 >= 0(不能洗点) ② 消耗 = 六项增量之和, 且 <= FreePoints。
// Slot5 是 wire 第 6 个字段(d7[5]), 阶段 1 审计未证明业务含义, 原样保留不猜。
type AllocPoints struct {
	ID                           domain.EntityID
	STR, VIT, INT, SPI, AGI, DEX int32 // 六维增量
	Slot5                        int32 // d7[5]: 语义未证明(positional)
}

func (AllocPoints) CmdName() string { return "AllocPoints" }

// UpgradeSkill 升级技能(消耗技能点)。
type UpgradeSkill struct {
	ID    domain.EntityID
	Skill domain.SkillID
}

// LearnLife 学习生活技能面板九项之一。钓鱼/采矿以 0 熟练度起步，其余加工技能以 1 级起步。
type LearnLife struct {
	ID    domain.EntityID
	Skill domain.SkillID
}

type MailItemRef struct {
	Tab   uint8
	Slot  int32
	Count int32
}

type ChatShareRef struct {
	Kind uint8
	Tab  uint8
	Slot int32
}

type ChatShareResult struct {
	Shares []domain.ChatShare
	Reason string
}

type ResolveChatShares struct {
	ID    domain.EntityID
	Refs  []ChatShareRef
	Reply chan ChatShareResult
}

func (ResolveChatShares) CmdName() string { return "ResolveChatShares" }

type MailReservation struct {
	BeforeBag   *domain.Bag
	BeforeMoney domain.Money
	BeforeCaiyu int64
}

type MailReserveResult struct {
	Snapshot    domain.Snapshot
	Attachments []domain.MailAttachment
	Reservation MailReservation
	Reason      string
}

type MailAttachmentDisplay struct {
	Name, Desc string
}

type MailAttachmentDisplayResult struct {
	Items []MailAttachmentDisplay
	OK    bool
}

// RenderMailAttachments projects hydrated attachment instances through the
// same item presentation rules used by the recipient's inventory.
type RenderMailAttachments struct {
	ID     domain.EntityID
	Stacks []domain.Stack
	Reply  chan<- MailAttachmentDisplayResult
}

func (RenderMailAttachments) CmdName() string { return "RenderMailAttachments" }

type ReserveMailSend struct {
	ID    domain.EntityID
	Money int64
	Items []MailItemRef
	Reply chan MailReserveResult
}

func (ReserveMailSend) CmdName() string { return "ReserveMailSend" }

type ReserveMailClaim struct {
	ID    domain.EntityID
	Mail  domain.Mail
	Reply chan MailReserveResult
}

func (ReserveMailClaim) CmdName() string { return "ReserveMailClaim" }

type FinalizeMailReservation struct {
	ID          domain.EntityID
	Reservation MailReservation
	Commit      bool
	Done        chan struct{}
}

func (FinalizeMailReservation) CmdName() string { return "FinalizeMailReservation" }

func (LearnLife) CmdName() string { return "LearnLife" }

// GatherWork 是客户端工作循环每 3 秒发来的真实结算触发。Token 对生活工作
// 是技能号，对普通工作是 work_id；服务端仍以 NextSettleAt 限速。
type GatherWork struct {
	ID    domain.EntityID
	Token domain.WorkID
}

func (GatherWork) CmdName() string { return "GatherWork" }

type OpenFurnace struct {
	ID       domain.EntityID
	MakeType uint8
}

func (OpenFurnace) CmdName() string { return "OpenFurnace" }

type CraftItem struct {
	ID       domain.EntityID
	MakeType uint8
	Product  domain.ItemID
}

func (CraftItem) CmdName() string { return "CraftItem" }

type RefineItem struct {
	ID        domain.EntityID
	Mode      uint8 // 0=背包，1=已穿戴
	EquipSlot uint8
	Execute   bool
	Tab       uint8
	BagSlot   int32
	Protect   bool
}

func (RefineItem) CmdName() string { return "RefineItem" }

type DrillItem struct {
	ID         domain.EntityID
	TargetKind uint8
	EquipSlot  uint8
	Tab        uint8
	BagSlot    int32
	Execute    bool
	Protect    bool
}

func (DrillItem) CmdName() string { return "DrillItem" }

type InlayItem struct {
	ID          domain.EntityID
	TargetKind  uint8
	EquipSlot   uint8
	Tab         uint8
	BagSlot     int32
	Hole        uint8
	RuneTab     uint8
	RuneBagSlot int32
}

func (InlayItem) CmdName() string { return "InlayItem" }

type UseItemOn struct {
	ID         domain.EntityID
	Tool       domain.ItemID
	TargetKind uint8
	Tab        uint8
	Slot       int32
}

func (UseItemOn) CmdName() string { return "UseItemOn" }

type WashAffix struct {
	ID    domain.EntityID
	Kind  uint8
	Tab   uint8
	Slot  int32
	Index int32
}

func (WashAffix) CmdName() string { return "WashAffix" }

func (UpgradeSkill) CmdName() string { return "UpgradeSkill" }

// SaveHotbar 保存 1.5.8 客户端完整上报的 30 格。
type SaveHotbar struct {
	ID       domain.EntityID
	Slots    [domain.HotbarSlotCount]domain.HotbarSlot
	Expanded bool
}

func (SaveHotbar) CmdName() string { return "SaveHotbar" }

type SetSmartCast struct {
	ID domain.EntityID
	On bool
}

func (SetSmartCast) CmdName() string { return "SetSmartCast" }

type SetPetView struct {
	ID   domain.EntityID
	Mask int32
}

func (SetPetView) CmdName() string { return "SetPetView" }

type SetGlowMode struct {
	ID   domain.EntityID
	Mode uint8
}

func (SetGlowMode) CmdName() string { return "SetGlowMode" }

// SetPKMode 保存客户端 0..4 的 PK 模式偏好；是否允许玩家互伤仍由场景规则决定。
type SetPKMode struct {
	ID   domain.EntityID
	Mode uint8
}

func (SetPKMode) CmdName() string { return "SetPKMode" }

// RenamePlayer 把已经成功落库的新名字应用到场景实体，并用 Done 回报是否命中
// 当前实体租约。改名是持久化后必须完成的同步，不能静默丢进普通日志。
type RenamePlayer struct {
	ID   domain.EntityID
	Name string
	Done chan<- bool
}

func (RenamePlayer) CmdName() string { return "RenamePlayer" }

type SetTitle struct {
	ID    domain.EntityID
	Title string
}

func (SetTitle) CmdName() string { return "SetTitle" }

type SetEquipFXMask struct {
	ID   domain.EntityID
	Mask int32
}

func (SetEquipFXMask) CmdName() string { return "SetEquipFXMask" }

// Revive 主动复活(死亡状态下)。
type Revive struct {
	ID         domain.EntityID
	ReviveType uint8 // 0=回城, 1=原地(需要复活卷轴)
}

func (Revive) CmdName() string { return "Revive" }

// TrialAction carries only the native menu verb, not player-supplied outcomes.
type TrialAction struct {
	ID     domain.EntityID
	Action string
}

func (TrialAction) CmdName() string { return "TrialAction" }
