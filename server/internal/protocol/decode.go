package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
)

// 上行解码。
//
// 出站已经有一层了（`encode.go`：游戏事件 → 字节）。入站一直没有 ——
// `session.OnPacket` 是一个裸 switch，认 6 个 opcode，其余全落进 `default` 悄悄丢掉。
// 8 大系统里除了移动，**没有一条指令能从客户端进来**。
//
// 这里补上对称的那一半：
//
//	出站   game/event   →  protocol.Encode  →  字节
//	入站   字节          →  protocol.Decode  →  protocol.Request  →  session 翻成 scene.Command
//
// 中间那个 `Request` 不能直接是 `scene.Command`：
// **协议层不许认识游戏层**（`internal/arch` 的红线之一，游戏层也不许认识协议层）。
// 所以入站在这里落成一套中立的请求结构，由 session 翻过去 —— 它两边都认识。
//
// 解码器按服务端已实现的业务入口逐项注册；业务尚未实现的请求交给 UnknownLog。

// Op 是协议号。
type Op uint16

// AllowedDuringMapLoad keeps connection control and map acknowledgement available
// while the client rebuilds its world. Gameplay packets cannot mutate that world.
func AllowedDuringMapLoad(op Op) bool {
	e, ok := decoders[op]
	if !ok {
		return false
	}
	switch e.Kind {
	case ReqPing, ReqSavePos, ReqMove, ReqLeaveWorld, ReqServerStat, ReqGMCommand:
		return true
	}
	return false
}

const (
	OpApplyAvatar Op = 0x1036
	OpBlockAdd    Op = 0x108d
	OpBlockDel    Op = 0x108e
	OpClientCaps  Op = 0x1091
)

// Request 是一条解好的上行请求。
//
// 用一个带 Kind 的结构而不是接口：请求种类是封闭集合，字段高度重叠，
// 而 session 那边要做的就是一次 switch —— 接口在这里只增加分配，没有收益。
type Request struct {
	Kind ReqKind

	// 通用载荷。哪个字段有效由 Kind 决定，各 Kind 的含义写在 decoders 表里。
	X, Y         int32
	MapID        int32
	TargetID     uint32
	ID32         uint32 // 技能号 / 物品号 / 任务号 / 宠物实例号 …
	RequestID    int32  // SystemInfoBar 回传的交互请求号
	TransList    int32  // NPC 传送列表号
	TransIndex   int32  // NPC 传送目的地零基索引
	TickMS       int64
	Money        int64 // TradeOffer 的客户端货币报价
	Price        int64 // 摆摊单价
	Caiyu        int64
	MailID       int64
	CharID       int64 // 持久化角色号；1.5.8 屏蔽删除使用 I64
	Slot         int32
	FromSlot     int32
	ToSlot       int32
	FromTab      uint8
	ToTab        uint8
	WarehouseTab uint8
	Count        int32
	UnitPrice    int32
	Bundle       int32
	Day          int32
	RefundShares int32
	// EquipFxMask 是客户端本地的装备特效隐藏位图。它不是物品/实体 ID，
	// 单列出来可避免会话层把 0x1082 的位图误当成 ID 使用。
	EquipFxMask   int32
	Flag          uint8
	U8            uint8 // 通用单字节参数 (复活类型等)
	Tab           uint8 // 背包分页；与 mode/targetKind 等通用字节参数分开保存
	Mode          uint8
	EquipSlot     uint8
	TargetKind    uint8
	Hole          uint8
	RuneTab       uint8
	RuneBagSlot   int32
	Index         int32
	Race          uint8
	Gender        uint8
	Head          uint8
	Hair          uint8
	S1, S2        string
	S3, S4        string
	DeviceID      string  // Login 的客户端设备标识；认证暂不使用，但协议层不得丢字段
	ClientVersion string  // Login 固定版本字段；当前只接受 1.6.0
	ClientVM      uint8   // 1.5.8 VmDetect 结果，只接受 0/1
	ClientDebug   uint8   // 1.5.8 DebugDetect 结果，只接受 0/1
	ClientAuthTag []byte  // 1.5.8 HMAC-SHA256 tag，会话层使用本连接握手密钥校验
	Vals          []int32 // 通用 int32 数组; AllocPoints 用它按 wire 原序装 7 个增量
	Hotbar        [HotbarSlotCount]HotbarSlot
	Expanded      bool
	Execute       bool
	Protect       bool
	TradeItems    []TradeItemRef
	MailItems     []MailItemRef
	Shares        []ShareRef
}

// MailItemRef 是 0x1020 发信附件的背包引用，必须由场景对照权威背包解析。
type MailItemRef struct {
	Tab   uint8
	Slot  int32
	Count int32
}

// TradeItemRef is the exact 0x1041 client reference tuple. It is not yet an item
// fact: the scene actor must resolve tab/slot/count against the authoritative bag.
type TradeItemRef struct {
	Tab   uint8
	Slot  int32
	Count int32
}

// ShareRef 是 SendChat/Gossip 的精确分享引用三元组。它只是客户端背包引用，
// 业务层必须再对照权威背包解析，不能把其中任何值直接当成物品事实。
type ShareRef struct {
	Kind uint8
	Tab  uint8
	Slot int32
}

const (
	PrimaryHotbarSlotCount = 20
	HotbarSlotCount        = 30
)

// HotbarSlot 是 0x1014 上报的一格。图标不由客户端回传，服务端从静态定义派生。
type HotbarSlot struct {
	ID   int32
	Kind uint8
}

// ReqKind 是上行请求的种类。
//
// 与 `scene.Command` 一一对应，但**故意不共用类型** —— 共用就等于协议层认识游戏层。
type ReqKind uint8

const (
	ReqUnknown ReqKind = iota
	ReqLogin
	ReqRegister
	ReqResetPassword
	ReqPing
	ReqServerStat
	ReqCreateChar
	ReqEnterGame
	ReqLeaveWorld
	ReqMove
	ReqSavePos
	ReqOpenTransport
	ReqChooseTransport
	ReqOpenShop
	ReqBuyItem
	ReqSellItem
	ReqRepair
	ReqRepairConfirm
	ReqNpcTasks
	ReqAttack
	ReqQueryTargetStatus
	ReqUseSkill
	ReqUseItem
	ReqApplyAvatar
	ReqMoveBagItem
	ReqSortBag
	ReqDropItem
	ReqSplitItem
	ReqPickUp
	ReqEquip
	ReqUnequip
	ReqEquipFxMask
	ReqChangeSetItem
	ReqChangeSetSwap
	ReqAcceptQuest
	ReqSubmitQuest
	ReqAbandonQuest
	ReqAllocPoints
	ReqUpgradeSkill
	ReqLearnLife
	ReqGatherWork
	ReqOpenFurnace
	ReqCraftItem
	ReqRefineItem
	ReqDrillItem
	ReqInlayItem
	ReqUseItemOn
	ReqWashAffix
	ReqOpenWarehouse
	ReqWarehouseMove
	ReqWarehouseMoney
	ReqWarehouseSplit
	ReqWarehouseRearrange
	ReqSortWarehouse
	ReqSetItemLock
	ReqChangePassword
	ReqChangeSecurityCode
	ReqRevive
	ReqEnterDungeon
	ReqStartWork
	ReqStopWork
	ReqCapturePet
	ReqSummonPet
	ReqRecallPet
	ReqTogglePet
	ReqHatchPet
	ReqSetPetSkill
	ReqUsePetSkill
	ReqPetOther
	ReqDeleteChar
	ReqSaveHotbar
	ReqSetSmartCast
	ReqSetPetView
	ReqSetGlowMode
	ReqSetPKMode
	ReqSetTitle
	ReqInspectPlayer
	ReqRenameCharacter
	ReqTradeRequest
	ReqTradeOffer
	ReqTradeLock
	ReqTradeConfirm
	ReqTradeCancel
	ReqPartyLeave
	ReqPartyKick
	ReqPartyLeader
	ReqPartyCreate
	ReqPartyDismiss
	ReqPartyInvite
	ReqPartyJoin
	ReqSysMsgRespond
	ReqFriendRequest
	ReqFriendDelete
	ReqGossip
	ReqEmote
	ReqHelp
	ReqSit
	ReqGMCommand
	ReqTipSeen
	ReqOpenMail
	ReqSendMail
	ReqMailAction
	ReqWardrobeStore
	ReqWardrobeWear
	ReqWardrobeRemove
	ReqWardrobeMove
	ReqStallCreate
	ReqStallEnd
	ReqStallAdd
	ReqStallDel
	ReqStallBrowse
	ReqStallDeal
	ReqChangeHair
	ReqFamilyInvite
	ReqFamilyLeave
	ReqFamilyCreate
	ReqFamilyList
	ReqFamilyApply
	ReqFamilyManage
	ReqFamilyMail
	ReqFamilyPositions
	ReqFamilyRefresh
	ReqFamilyPosName
	ReqFamilyStash
	ReqFamilyStashPerm
	ReqApprenticeRequest
	ReqApprenticeList
	ReqExpandWarehouse
	ReqOpenRack
	ReqBuyRack
	ReqFittingCatalog
	ReqFittingDesc
	ReqRackRefundList
	ReqRackRefund
	ReqDeposit
	ReqBlockAdd
	ReqBlockDel
	ReqClientCaps
)

var reqNames = map[ReqKind]string{
	ReqUnknown: "Unknown", ReqLogin: "Login", ReqRegister: "Register", ReqResetPassword: "ResetPassword", ReqPing: "Ping",
	ReqServerStat: "ServerStat", ReqEnterGame: "EnterGame", ReqLeaveWorld: "LeaveWorld",
	ReqCreateChar: "CreateChar",
	ReqMove:       "Move", ReqSavePos: "SavePos", ReqOpenTransport: "OpenTransport",
	ReqChooseTransport: "ChooseTransport", ReqOpenShop: "OpenShop",
	ReqBuyItem: "BuyItem", ReqSellItem: "SellItem", ReqRepair: "Repair",
	ReqRepairConfirm: "RepairConfirm", ReqNpcTasks: "NpcTasks",
	ReqAttack: "Attack", ReqQueryTargetStatus: "QueryTargetStatus",
	ReqUseSkill: "UseSkill", ReqUseItem: "UseItem", ReqApplyAvatar: "ApplyAvatar", ReqMoveBagItem: "MoveBagItem",
	ReqSortBag: "SortBag", ReqDropItem: "DropItem", ReqSplitItem: "SplitItem", ReqPickUp: "PickUp",
	ReqEquip: "Equip", ReqUnequip: "Unequip", ReqEquipFxMask: "EquipFxMask",
	ReqChangeSetItem: "ChangeSetItem", ReqChangeSetSwap: "ChangeSetSwap",
	ReqAcceptQuest: "AcceptQuest", ReqSubmitQuest: "SubmitQuest",
	ReqAbandonQuest: "AbandonQuest", ReqAllocPoints: "AllocPoints", ReqUpgradeSkill: "UpgradeSkill", ReqLearnLife: "LearnLife", ReqGatherWork: "GatherWork", ReqOpenFurnace: "OpenFurnace", ReqCraftItem: "CraftItem", ReqRefineItem: "RefineItem", ReqDrillItem: "DrillItem", ReqInlayItem: "InlayItem", ReqUseItemOn: "UseItemOn", ReqWashAffix: "WashAffix",
	ReqOpenWarehouse: "OpenWarehouse", ReqWarehouseMove: "WarehouseMove", ReqWarehouseMoney: "WarehouseMoney",
	ReqWarehouseSplit: "WarehouseSplit", ReqWarehouseRearrange: "WarehouseRearrange", ReqSortWarehouse: "SortWarehouse",
	ReqSetItemLock: "SetItemLock", ReqChangePassword: "ChangePassword", ReqChangeSecurityCode: "ChangeSecurityCode", ReqRevive: "Revive",
	ReqEnterDungeon: "EnterDungeon",
	ReqStartWork:    "StartWork", ReqStopWork: "StopWork",
	ReqCapturePet: "CapturePet", ReqSummonPet: "SummonPet", ReqRecallPet: "RecallPet", ReqTogglePet: "TogglePet",
	ReqHatchPet: "HatchPet", ReqSetPetSkill: "SetPetSkill", ReqUsePetSkill: "UsePetSkill",
	ReqPetOther: "PetOther", ReqDeleteChar: "DeleteChar",
	ReqSaveHotbar: "SaveHotbar", ReqSetSmartCast: "SetSmartCast",
	ReqSetPetView: "SetPetView", ReqSetGlowMode: "SetGlowMode", ReqSetPKMode: "SetPKMode", ReqSetTitle: "SetTitle",
	ReqInspectPlayer:   "InspectPlayer",
	ReqRenameCharacter: "RenameCharacter",
	ReqTradeRequest:    "TradeRequest", ReqTradeOffer: "TradeOffer", ReqTradeLock: "TradeLock",
	ReqTradeConfirm: "TradeConfirm", ReqTradeCancel: "TradeCancel",
	ReqPartyLeave: "PartyLeave", ReqPartyKick: "PartyKick", ReqPartyLeader: "PartyLeader",
	ReqPartyCreate: "PartyCreate", ReqPartyDismiss: "PartyDismiss",
	ReqPartyInvite: "PartyInvite", ReqPartyJoin: "PartyJoin", ReqSysMsgRespond: "SysMsgRespond",
	ReqFriendRequest: "FriendRequest", ReqFriendDelete: "FriendDelete", ReqGossip: "Gossip",
	ReqEmote: "Emote", ReqHelp: "Help", ReqSit: "Sit",
	ReqGMCommand: "GMCommand", ReqTipSeen: "TipSeen",
	ReqOpenMail: "OpenMail", ReqSendMail: "SendMail", ReqMailAction: "MailAction",
	ReqWardrobeStore: "WardrobeStore", ReqWardrobeWear: "WardrobeWear",
	ReqWardrobeRemove: "WardrobeRemove", ReqWardrobeMove: "WardrobeMove",
	ReqStallCreate: "StallCreate", ReqStallEnd: "StallEnd", ReqStallAdd: "StallAdd",
	ReqStallDel: "StallDel", ReqStallBrowse: "StallBrowse", ReqStallDeal: "StallDeal",
	ReqChangeHair:   "ChangeHair",
	ReqFamilyInvite: "FamilyInvite", ReqFamilyLeave: "FamilyLeave",
	ReqFamilyCreate: "FamilyCreate", ReqFamilyList: "FamilyList", ReqFamilyApply: "FamilyApply",
	ReqFamilyManage: "FamilyManage", ReqFamilyMail: "FamilyMail",
	ReqFamilyPositions: "FamilyPositions", ReqFamilyRefresh: "FamilyRefresh",
	ReqFamilyPosName: "FamilyPosName",
	ReqFamilyStash:   "FamilyStash", ReqFamilyStashPerm: "FamilyStashPerm",
	ReqApprenticeRequest: "ApprenticeRequest", ReqApprenticeList: "ApprenticeList",
	ReqExpandWarehouse: "ExpandWarehouse",
	ReqOpenRack:        "OpenRack", ReqBuyRack: "BuyRack",
	ReqFittingCatalog: "FittingCatalog", ReqFittingDesc: "FittingDesc", ReqDeposit: "Deposit",
	ReqRackRefundList: "RackRefundList", ReqRackRefund: "RackRefund",
	ReqBlockAdd: "BlockAdd", ReqBlockDel: "BlockDel",
	ReqClientCaps: "ClientCaps",
}

func (k ReqKind) String() string {
	if n, ok := reqNames[k]; ok {
		return n
	}
	return fmt.Sprintf("ReqKind(%d)", uint8(k))
}

// decoder 把一个包的载荷解成请求。
type decoder func(payload []byte) (Request, bool)

// entry 是注册表里的一行。
type entry struct {
	Kind ReqKind
	Dec  decoder
	// Proven 表示这个 opcode 与载荷格式已有依据；false 的条目不进入生产解码。
	Proven bool
}

// subCommandKinds 列出**子命令包**除表里那个之外还会解出的种类。
//
// `0x100b` 一个号底下挂着 11 个客户端方法，首字节才是真正的动作，解码器按子
// 命令自己定 Kind。把全集列在这里，"每个注册的 Kind 都得有处理分支"那道检查
// 才查得到它们 —— 否则子命令会绕过审计，收到包只打一行警告然后什么都不做。
var subCommandKinds = map[Op][]ReqKind{
	0x100b: {ReqSummonPet, ReqRecallPet, ReqTogglePet, ReqHatchPet, ReqSetPetSkill, ReqUsePetSkill},
}

// decoders 是 opcode → 解码器 的注册表。
//
// 只注册载荷格式已知且服务端有对应业务入口的请求。其他已知协议操作由 UnknownLog
// 记录，等待接入对应业务处理。
var decoders = map[Op]entry{
	0x1002: {ReqLogin, decLogin, true},
	0x105d: {ReqRegister, decRegister, true},
	0x105e: {ReqResetPassword, decResetPassword, true},
	0x1001: {ReqPing, decPing, true},
	0x107d: {ReqServerStat, decEmpty(ReqServerStat), true},
	0x1003: {ReqCreateChar, decCreateChar, true},
	0x1004: {ReqEnterGame, decEnterGame, true},
	0x1049: {ReqLeaveWorld, decEmpty(ReqLeaveWorld), true},
	0x1017: {ReqMove, decMoveXY, true},  // PlayerMove(x,y): 两个小端 i32
	0x100a: {ReqSavePos, decMove, true}, // SavePos: i32 模式 + i32 X + i32 Y + u8 标志
	0x1071: {ReqOpenTransport, decOpenTransport, true},
	0x1072: {ReqChooseTransport, decChooseTransport, true},
	0x1019: {ReqOpenShop, decOpenShop, true},
	0x101a: {ReqBuyItem, decBuyItem, true},
	0x101b: {ReqSellItem, decSellItem, true},
	0x101d: {ReqRepair, decRepair(ReqRepair), true},
	0x101e: {ReqRepairConfirm, decRepair(ReqRepairConfirm), true},
	0x101f: {ReqOpenMail, decEmpty(ReqOpenMail), true},
	0x1020: {ReqSendMail, decSendMail, true},
	0x1021: {ReqMailAction, decMailAction, true},

	// 生活技能请求。
	0x1027: {ReqStartWork, decStartWork, true},
	0x1028: {ReqStopWork, decEmpty(ReqStopWork), true},

	// 单实体操作与技能请求。
	0x100c: {ReqAttack, decOneID, true},            // Attack(0x22334455) -> 55 44 33 22
	0x1081: {ReqQueryTargetStatus, decOneID, true}, // QueryTargetStatus(targetId): I32
	0x1013: {ReqUseSkill, decUseSkill, true},       // skillId, mode, x, y, targetId
	0x1015: {ReqUseItem, decUseItem, true},         // itemId:I32, tab:U8, slot:I32
	0x100d: {ReqMoveBagItem, decMoveBagItem, true}, // tab:U8, from:I32, to:I32
	0x1078: {ReqSortBag, decSortBag, true},         // tab:U8
	0x100f: {ReqDropItem, decDropItem, true},       // tab:U8, slot:I32, x:I32, y:I32, itemId:I32, confirmed:U8
	0x1016: {ReqSplitItem, decSplitItem, true},     // tab:U8, slot:I32, count:I32
	0x1010: {ReqPickUp, decOneID, true},            // PickItem(0x44556677) -> 77 66 55 44
	0x1007: {ReqEquip, decOneID, true},             // EquipItem(0x11) -> 11 00 00 00
	0x1008: {ReqUnequip, decOneID, true},           // UnequipItem(0x33) -> 33 00 00 00
	0x1082: {ReqEquipFxMask, decEquipFxMask, true}, // SendEquipFx: CharData.EquipFxHideMask() 的 i32 位图
	0x1014: {ReqSaveHotbar, decHotbar, true},       // 1.3.4:20 格；1.5.8:20 格+expanded+10 格
	0x107f: {ReqSetSmartCast, decBool(ReqSetSmartCast), true},
	0x1085: {ReqSetPetView, decI32(ReqSetPetView), true},
	0x107b: {ReqSetGlowMode, decGlowMode, true},
	0x1075: {ReqSetPKMode, decPKMode, true},
	0x1005: {ReqSetTitle, decOneString(ReqSetTitle, true), true},
	0x1073: {ReqInspectPlayer, decOneID, true},
	0x1079: {ReqRenameCharacter, decOneString(ReqRenameCharacter, false), true},
	0x102b: {ReqCapturePet, decCapture, true}, // Capture(a,b) -> 两个小端 i32
	// 0x100b 是子命令包: 首字节才是动作。Kind 由 decPetCmd 按子命令定,
	// 表里这个 ReqPetOther 只兜住"还没接的那些子命令"。
	0x100b: {ReqPetOther, decPetCmd, true},

	// NPC、任务、技能与仓库请求。
	0x1032: {ReqNpcTasks, decOneStr, true},           // TaskNpcList(NPC 名): Str
	0x1033: {ReqAcceptQuest, decOneID, true},         // TaskAccept(任务号): I32
	0x1035: {ReqSubmitQuest, decOneID, true},         // TaskComplete(任务号): I32
	0x1034: {ReqAbandonQuest, decOneID, true},        // TaskAbandon(任务号): I32
	0x1009: {ReqAllocPoints, decAllocPoints, true},   // AllocPoints: I32×7 增量, wire 序见 decAllocPoints
	0x1012: {ReqUpgradeSkill, decUpgradeSkill, true}, // UpgradeSkill(技能号): I32
	0x1026: {ReqLearnLife, decI32(ReqLearnLife), true},
	0x1025: {ReqGatherWork, decI32(ReqGatherWork), true},
	0x1022: {ReqOpenFurnace, decOneU8(ReqOpenFurnace), true},
	0x1023: {ReqCraftItem, decCraftItem, true},
	0x1024: {ReqRefineItem, decRefineItem, true},
	0x1029: {ReqDrillItem, decDrillItem, true},
	0x102a: {ReqInlayItem, decInlayItem, true},
	0x105b: {ReqUseItemOn, decUseItemOn, true},
	0x105c: {ReqWashAffix, decWashAffix, true},
	0x102c: {ReqOpenWarehouse, decEmpty(ReqOpenWarehouse), true},
	0x102d: {ReqWarehouseMove, decWarehouseMove, true},
	0x102e: {ReqWarehouseMoney, decWarehouseMoney, true},
	0x1030: {ReqWarehouseSplit, decWarehouseSplit, true},
	0x1031: {ReqWarehouseRearrange, decWarehouseRearrange, true},
	0x1087: {ReqSortWarehouse, decOneU8(ReqSortWarehouse), true},
	0x1080: {ReqSetItemLock, decSetItemLock, true},
	0x1088: {ReqChangePassword, decChangePassword, true},
	0x1089: {ReqChangeSecurityCode, decChangeSecurityCode, true},
	0x1018: {ReqRevive, decRevive, true}, // Revive(类型): U8, 0=回城 1=原地
	0x105f: {ReqDeleteChar, decOneStr, true},
	0x1040: {ReqTradeRequest, decOneID, true},
	0x1041: {ReqTradeOffer, decTradeOffer, true},
	0x1042: {ReqTradeLock, decBool(ReqTradeLock), true},
	0x1043: {ReqTradeConfirm, decEmpty(ReqTradeConfirm), true},
	0x1044: {ReqTradeCancel, decEmpty(ReqTradeCancel), true},
	0x1046: {ReqPartyLeave, decEmpty(ReqPartyLeave), true},
	0x104d: {ReqPartyKick, decOneID, true},
	0x104e: {ReqPartyLeader, decOneID, true},
	0x1051: {ReqPartyCreate, decOneString(ReqPartyCreate, false), true},
	0x1052: {ReqPartyDismiss, decEmpty(ReqPartyDismiss), true},
	0x1045: {ReqPartyInvite, decTargetName(ReqPartyInvite), true},
	0x1053: {ReqPartyJoin, decTargetName(ReqPartyJoin), true},
	0x1054: {ReqWardrobeStore, decWardrobeStore, true},
	0x1055: {ReqWardrobeWear, decI32(ReqWardrobeWear), true},
	0x1056: {ReqWardrobeRemove, decI32(ReqWardrobeRemove), true},
	0x104a: {ReqSysMsgRespond, decSysMsgRespond, true},
	0x104f: {ReqFriendRequest, decOneString(ReqFriendRequest, false), true},
	0x1050: {ReqFriendDelete, decOneString(ReqFriendDelete, false), true},
	0x104c: {ReqGossip, decGossip, true},
	0x1057: {ReqEmote, decI32(ReqEmote), true},
	0x107c: {ReqWardrobeMove, decWardrobeMove, true},
	0x1060: {ReqStallCreate, decStallCreate, true},
	0x1061: {ReqStallEnd, decEmpty(ReqStallEnd), true},
	0x1062: {ReqStallAdd, decStallAdd, true},
	0x1063: {ReqStallDel, decStallDel, true},
	0x1064: {ReqStallBrowse, decOneID, true},
	0x1065: {ReqStallDeal, decStallDeal, true},
	0x1066: {ReqChangeHair, decChangeHair, true},
	// HelpWinDlg.Toggle 与 PlayerController.Update 的 1.3.4 调用链分别写出
	// HelpReq() 和 Sit(on)。两条都只表示客户端本人刚完成对应 UI 操作。
	0x1077: {ReqHelp, decEmpty(ReqHelp), true},
	0x1011: {ReqSit, decBool(ReqSit), true},
	// NewbieTipBox.Push(id) 首次展示固定提示后调用 NetClient.TipSeen(id)。
	// 服务端持久化该编号，并在下次进世界时通过 0x8054 种回客户端。
	0x1076: {ReqTipSeen, decI32(ReqTipSeen), true},
	// ChatUI 对以 '/' 开头的输入去掉斜杠后调用 GM 命令；载荷为
	// 一个 Str。0x1006 也承载少量普通玩法菜单命令，会话层只拦截明确白名单。
	0x1006: {ReqGMCommand, decOneString(ReqGMCommand, false), true},
}

// ProvenUnwired 是**已经实证、但服务端还没接**的上行 opcode。
//
// 放在这里而不是注释里, 是因为它们是**查证过的事实**, 不该只活在散文里 ——
// 对应业务实现后，可将请求加入 decoders 注册表。
//
// 采集与验证方式同上。载荷长度是实测的。
var ProvenUnwired = map[Op]struct {
	Method string // 客户端方法名
	Len    int    // 实测载荷长度
	Note   string
}{
	0x1068: {"FamilyList", 0, "家族列表"},
	0x106f: {"ApprenticeList", 0, "师徒列表"},
}

func FamilyOpcode(op Op) bool {
	switch op {
	case 0x1047, 0x1048, 0x1067, 0x1068, 0x1069, 0x106a, 0x106b, 0x106c, 0x106d, 0x106e,
		0x1092, 0x1093:
		return true
	}
	return false
}

// DecodeFamily 解码已经闭合、但为保持通用注册表历史审计基线而独立路由的家族包。
func DecodeFamily(op Op, payload []byte) (Request, bool) {
	switch op {
	case 0x1047:
		return decFamilyInvite(payload)
	case 0x1048:
		return decEmpty(ReqFamilyLeave)(payload)
	case 0x1067:
		return decFamilyCreate(payload)
	case 0x1068:
		return decEmpty(ReqFamilyList)(payload)
	case 0x1069:
		return decOneString(ReqFamilyApply, false)(payload)
	case 0x106a:
		return decFamilyManage(payload)
	case 0x106b:
		return decFamilyMail(payload)
	case 0x106c:
		return decEmpty(ReqFamilyPositions)(payload)
	case 0x106d:
		return decEmpty(ReqFamilyRefresh)(payload)
	case 0x106e:
		return decFamilyPosName(payload)
	case 0x1092:
		r := newReader(payload)
		action, tab, slot, index := r.u8(), r.u8(), r.i32(), r.i32()
		if !r.done() || action > 2 ||
			action == 0 && (tab != 0 || slot != 0 || index != 0) ||
			action == 1 && (slot < 0 || index != 0) ||
			action == 2 && (tab != 0 || slot != 0 || index < 0) {
			return Request{Kind: ReqFamilyStash}, false
		}
		return Request{Kind: ReqFamilyStash, Mode: action, FromTab: tab, FromSlot: slot, Index: index}, true
	case 0x1093:
		r := newReader(payload)
		position := r.u8()
		if !r.done() || position < 1 || position > 10 {
			return Request{Kind: ReqFamilyStashPerm}, false
		}
		return Request{Kind: ReqFamilyStashPerm, U8: position}, true
	}
	return Request{}, false
}

func ApprenticeOpcode(op Op) bool { return op == 0x104b || op == 0x106f }

func DecodeApprentice(op Op, payload []byte) (Request, bool) {
	switch op {
	case 0x104b:
		r := newReader(payload)
		target, role, name := r.i32(), r.u8(), r.str()
		if !r.done() {
			return Request{}, false
		}
		return Request{Kind: ReqApprenticeRequest, TargetID: uint32(target), U8: role, S1: name}, true
	case 0x106f:
		return decEmpty(ReqApprenticeList)(payload)
	}
	return Request{}, false
}

func WarehouseExpansionOpcode(op Op) bool { return op == 0x102f }

func DecodeWarehouseExpansion(payload []byte) (Request, bool) {
	return decEmpty(ReqExpandWarehouse)(payload)
}

func CommerceOpcode(op Op) bool {
	return op == 0x1059 || op == 0x105a || op == 0x1070 || op == 0x107a ||
		op == 0x1094 || op == 0x1095 || op == 0x1098
}

func DecodeCommerce(op Op, payload []byte) (Request, bool) {
	switch op {
	case 0x1059:
		return decEmpty(ReqOpenRack)(payload)
	case 0x105a:
		r := newReader(payload)
		item, source, count := r.u32(), r.u8(), r.i32()
		if !r.done() || item == 0 || count <= 0 {
			return Request{}, false
		}
		return Request{Kind: ReqBuyRack, ID32: item, U8: source, Count: count}, true
	case 0x1070:
		r := newReader(payload)
		chain, amount := r.str(), r.i32()
		if !r.done() {
			return Request{}, false
		}
		return Request{Kind: ReqDeposit, S1: chain, X: amount}, true
	case 0x107a:
		if len(payload) == 0 {
			return Request{Kind: ReqFittingCatalog}, true
		}
		r := newReader(payload)
		kind, part, search, page, perPage := r.u8(), r.str(), r.str(), r.i32(), r.i32()
		if !r.done() || kind > 2 || page < 0 || perPage <= 0 || perPage > 100 {
			return Request{Kind: ReqFittingCatalog}, false
		}
		return Request{Kind: ReqFittingCatalog, U8: kind, S1: part, S2: search,
			X: page, Count: perPage, Mode: 1}, true
	case 0x1098:
		r := newReader(payload)
		count := int(r.u16())
		if count == 0 || count > 48 {
			return Request{Kind: ReqFittingDesc}, false
		}
		ids := make([]int32, count)
		for i := range ids {
			ids[i] = r.i32()
			if ids[i] <= 0 {
				return Request{Kind: ReqFittingDesc}, false
			}
		}
		if !r.done() {
			return Request{Kind: ReqFittingDesc}, false
		}
		return Request{Kind: ReqFittingDesc, Vals: ids}, true
	case 0x1094:
		return decEmpty(ReqRackRefundList)(payload)
	case 0x1095:
		r := newReader(payload)
		item, unitPrice, bundle, day, shares := r.i32(), r.i32(), r.i32(), r.i32(), r.i32()
		if !r.done() || item <= 0 || unitPrice <= 0 || bundle <= 0 || day < 0 || shares <= 0 {
			return Request{Kind: ReqRackRefund}, false
		}
		return Request{Kind: ReqRackRefund, ID32: uint32(item), UnitPrice: unitPrice,
			Bundle: bundle, Day: day, RefundShares: shares}, true
	}
	return Request{}, false
}

func BlockOpcode(op Op) bool { return op == OpBlockAdd || op == OpBlockDel }

// DecodeBlock 解码 1.5.8 的单向屏蔽操作。0x108d 按名字添加并保留
// scope=0/1；0x108e 按持久化角色号删除，不使用会话实体号。
func DecodeBlock(op Op, payload []byte) (Request, bool) {
	r := newReader(payload)
	switch op {
	case OpBlockAdd:
		name, scope := r.str(), r.u8()
		if !r.done() || name == "" || scope > 1 {
			return Request{Kind: ReqBlockAdd}, false
		}
		return Request{Kind: ReqBlockAdd, S1: name, U8: scope}, true
	case OpBlockDel:
		charID := r.i64()
		if !r.done() || charID <= 0 {
			return Request{Kind: ReqBlockDel}, false
		}
		return Request{Kind: ReqBlockDel, CharID: charID}, true
	default:
		return Request{}, false
	}
}

func CapabilityOpcode(op Op) bool { return op == OpClientCaps }

func DecodeClientCaps(payload []byte) (Request, bool) {
	r := newReader(payload)
	r.i32()
	if !r.done() {
		return Request{Kind: ReqClientCaps}, false
	}
	// 0x1091 是 1.5.8 客户端固定发送的版本标记。服务端只消费它以保持
	// 包边界闭合，不保存、不回协商，也不让它改变任何业务或下行路径。
	return Request{Kind: ReqClientCaps}, true
}

func ChangeSetOpcode(op Op) bool { return op == 0x109a || op == 0x109b }

// DecodeChangeSet 解码 1.5.8 快速换装请求。
func DecodeChangeSet(op Op, payload []byte) (Request, bool) {
	switch op {
	case 0x109a:
		return decChangeSetItem(payload)
	case 0x109b:
		return decEmpty(ReqChangeSetSwap)(payload)
	default:
		return Request{}, false
	}
}

func decFamilyInvite(payload []byte) (Request, bool) {
	r := newReader(payload)
	target, name := r.i32(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqFamilyInvite, TargetID: uint32(target), S1: name}, true
}

func decFamilyCreate(payload []byte) (Request, bool) {
	r := newReader(payload)
	name, proclaim := r.str(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqFamilyCreate, S1: name, S2: proclaim}, true
}

func decFamilyManage(payload []byte) (Request, bool) {
	r := newReader(payload)
	action, target, arg, body := r.u8(), r.str(), r.i32(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqFamilyManage, U8: action, S1: target, X: arg, S2: body}, true
}

func decFamilyMail(payload []byte) (Request, bool) {
	r := newReader(payload)
	title, body := r.str(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqFamilyMail, S1: title, S2: body}, true
}

func decFamilyPosName(payload []byte) (Request, bool) {
	r := newReader(payload)
	position, name := r.i32(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqFamilyPosName, X: position, S1: name}, true
}

func decSendMail(payload []byte) (Request, bool) {
	r := newReader(payload)
	to, title, body := r.str(), r.str(), r.str()
	money, caiyu, mailType := r.i64(), r.i64(), r.u8()
	n := int(r.u8())
	items := make([]MailItemRef, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, MailItemRef{Tab: r.u8(), Slot: r.i32(), Count: r.i32()})
	}
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqSendMail, S1: to, S2: title, S3: body,
		Money: money, Caiyu: caiyu, U8: mailType, MailItems: items}, true
}

func decMailAction(payload []byte) (Request, bool) {
	r := newReader(payload)
	action, id := r.u8(), r.i64()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqMailAction, U8: action, MailID: id}, true
}

// Decode 解一条上行包。
//
// 第二个返回值 false 表示**这个 opcode 还没接**（而不是包坏了）——
// 调用方该把它交给 Unknown 记下来，那是补映射表的原始材料。
func Decode(op Op, payload []byte) (Request, bool) {
	e, ok := decoders[op]
	if !ok {
		return Request{}, false
	}
	req, ok := e.Dec(payload)
	if !ok {
		return Request{Kind: e.Kind}, false
	}
	// 子命令包的解码器自己定了更具体的 Kind, 不覆盖它。
	if req.Kind == ReqUnknown {
		req.Kind = e.Kind
	}
	return req, true
}

// RegisteredKinds 返回一个 opcode 可能解出的**全部**种类。
// 子命令包一个号对多个种类，审计要按这个查，不能只看 RegisteredKind。
func RegisteredKinds(op Op) []ReqKind {
	e, ok := decoders[op]
	if !ok {
		return nil
	}
	return append([]ReqKind{e.Kind}, subCommandKinds[op]...)
}

// Registered 报告某个 opcode 认不认得。
func Registered(op Op) bool {
	_, ok := decoders[op]
	return ok
}

// RegisteredKind 返回已接线 opcode 对应的请求种类，供启动审计与会话层自检使用。
func RegisteredKind(op Op) (ReqKind, bool) {
	e, ok := decoders[op]
	if !ok {
		return ReqUnknown, false
	}
	return e.Kind, true
}

// ProvenOps 返回全部已实证的 opcode。给测试与启动日志用。
func ProvenOps() []Op {
	var out []Op
	for op, e := range decoders {
		if e.Proven {
			out = append(out, op)
		}
	}
	return out
}

// ── 各包的解码 ──

func decEmpty(k ReqKind) decoder {
	return func(payload []byte) (Request, bool) {
		return Request{Kind: k}, len(payload) == 0
	}
}

// decPing 解 0x1001。载荷是客户端 Ping(Int64 t) 写出的一个 I64。
//
// 那个时间戳必须**原样回显**在 0x8001 里, 客户端拿它算延迟:
//
//	rtt = Environment.TickCount - 回显值
//	if ((uint)rtt >> 5 <= 0x752) RttMs = rtt    // 即 rtt 在 [0, 60s) 才采信
//
// 不回显的话 rtt 是个天文数字, 被那个上界拦掉, 客户端的延迟显示**永远不更新**。
func decPing(payload []byte) (Request, bool) {
	r := newReader(payload)
	v := r.i64()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqPing, TickMS: v}, true
}

// decLogin 只接受 1.6.0：Str acc/pwd/device/version + U8 vm/debug + 32B authTag。
// authTag 是客户端 HMAC-SHA256 的原始 32 字节，没有长度前缀；会话层依据
// 已恢复的 NetCrypto 算法与本连接握手密钥验证，协议层只负责完整读取。
func decLogin(payload []byte) (Request, bool) {
	r := newReader(payload)
	acc, pwd, deviceID := r.str(), r.str(), r.str()
	clientVersion, vm, debug := r.str(), r.u8(), r.u8()
	if !r.ok() || acc == "" || clientVersion != "1.6.0" || vm > 1 || debug > 1 || r.remaining() != 32 {
		return Request{}, false
	}
	tag := r.raw(32)
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqLogin, S1: acc, S2: pwd, DeviceID: deviceID,
		ClientVersion: clientVersion, ClientVM: vm, ClientDebug: debug,
		ClientAuthTag: tag}, true
}

// decRegister 解 1.5.8 的 0x105d：四段账号 Str + 设备 Str + VmDetect U8。
// 字段是否为空、长度与字符约束属于账号业务规则，由 session 返回可见的 0x8009；
// 协议层只保证完整新布局存在且载荷被精确消费。
func decRegister(payload []byte) (Request, bool) {
	r := newReader(payload)
	acc, pwd, sec, invite := r.str(), r.str(), r.str(), r.str()
	device, vm := r.str(), r.u8()
	if !r.done() || vm > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqRegister, S1: acc, S2: pwd, S3: sec, S4: invite,
		DeviceID: device, ClientVM: vm}, true
}

func decResetPassword(payload []byte) (Request, bool) {
	r := newReader(payload)
	account, security, password := r.str(), r.str(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqResetPassword, S1: account, S2: security, S3: password}, true
}

// decSetItemLock 解 0x1080：tab:U8, slot:I32, on:U8, secCode:Str, itemId:I32。
// on 只接受客户端布尔写法 0/1；物品、分页与格号由场景再次按权威背包复核。
func decSetItemLock(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot, on := r.u8(), r.i32(), r.u8()
	security, item := r.str(), r.i32()
	if !r.done() || on > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqSetItemLock, Tab: tab, Slot: slot, Flag: on,
		S1: security, ID32: uint32(item)}, true
}

func decChangePassword(payload []byte) (Request, bool) {
	r := newReader(payload)
	oldPassword, newPassword := r.str(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqChangePassword, S1: oldPassword, S2: newPassword}, true
}

func decChangeSecurityCode(payload []byte) (Request, bool) {
	r := newReader(payload)
	password, oldSecurity, newSecurity := r.str(), r.str(), r.str()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqChangeSecurityCode, S1: password, S2: oldSecurity, S3: newSecurity}, true
}

func decWarehouseMove(payload []byte) (Request, bool) {
	r := newReader(payload)
	dir, fromTab, fromSlot := r.u8(), r.u8(), r.i32()
	warehouseTab, count, toSlot := r.u8(), r.i32(), r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqWarehouseMove, Flag: dir, FromTab: fromTab,
		FromSlot: fromSlot, WarehouseTab: warehouseTab, Count: count, ToSlot: toSlot}, true
}

func decWarehouseMoney(payload []byte) (Request, bool) {
	r := newReader(payload)
	mode, amount := r.u8(), r.i64()
	if !r.done() || mode > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqWarehouseMoney, U8: mode, Money: amount}, true
}

func decWarehouseSplit(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot, count := r.u8(), r.i32(), r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqWarehouseSplit, WarehouseTab: tab, Slot: slot, Count: count}, true
}

func decWarehouseRearrange(payload []byte) (Request, bool) {
	r := newReader(payload)
	fromTab, fromSlot, toTab, toSlot := r.u8(), r.i32(), r.u8(), r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqWarehouseRearrange, FromTab: fromTab, FromSlot: fromSlot,
		ToTab: toTab, ToSlot: toSlot}, true
}

func decWardrobeStore(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot := r.u8(), r.i32()
	if !r.done() || slot < 0 {
		return Request{}, false
	}
	return Request{Kind: ReqWardrobeStore, Tab: tab, Slot: slot}, true
}

func decWardrobeMove(payload []byte) (Request, bool) {
	r := newReader(payload)
	item, index := r.u32(), r.i32()
	if !r.done() || item == 0 || index < 0 {
		return Request{}, false
	}
	return Request{Kind: ReqWardrobeMove, ID32: item, Index: index}, true
}

func decStallCreate(payload []byte) (Request, bool) {
	r := newReader(payload)
	typ, name := r.u8(), r.str()
	if !r.done() || typ > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqStallCreate, U8: typ, S1: name}, true
}

func decStallAdd(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot, item, count, price := r.u8(), r.i32(), r.u32(), r.i32(), r.i64()
	if !r.done() || slot < 0 || item == 0 || count <= 0 || price <= 0 {
		return Request{}, false
	}
	return Request{Kind: ReqStallAdd, Tab: tab, Slot: slot, ID32: item, Count: count, Price: price}, true
}

func decStallDeal(payload []byte) (Request, bool) {
	r := newReader(payload)
	owner, index, count, tab, slot := r.u32(), r.i32(), r.i32(), r.u8(), r.i32()
	expectedItem, expectedPrice := r.u32(), r.i64()
	if !r.done() || owner == 0 || index < 0 || count <= 0 || expectedItem == 0 || expectedPrice <= 0 {
		return Request{}, false
	}
	return Request{Kind: ReqStallDeal, TargetID: owner, Index: index, Count: count,
		Tab: tab, Slot: slot, ID32: expectedItem, Price: expectedPrice}, true
}

func decStallDel(payload []byte) (Request, bool) {
	r := newReader(payload)
	index := r.i32()
	if !r.done() || index < 0 {
		return Request{}, false
	}
	return Request{Kind: ReqStallDel, Index: index}, true
}

func decChangeHair(payload []byte) (Request, bool) {
	r := newReader(payload)
	mode, target := r.u8(), r.u8()
	if !r.done() || mode > 1 || target == 0 {
		return Request{}, false
	}
	return Request{Kind: ReqChangeHair, Mode: mode, U8: target}, true
}

// decEnterGame 解 0x1004：uint16 长度 + 角色名。
func decEnterGame(payload []byte) (Request, bool) {
	r := newReader(payload)
	name := r.str()
	if !r.done() || name == "" {
		return Request{}, false
	}
	return Request{S1: name}, true
}

// decCreateChar 解 0x1003：
//
//	Str name | U8 race | U8 gender | U8 head | U8 hair
//
// 字段顺序遵循客户端建角请求。这里仅验证线格式完整；空名字、
// 非法职业等是领域规则，必须交给建角流程返回 0x8005，不能伪装成“载荷解析失败”。
func decCreateChar(payload []byte) (Request, bool) {
	r := newReader(payload)
	name := r.str()
	race := r.u8()
	gender := r.u8()
	head := r.u8()
	hair := r.u8()
	if !r.done() {
		return Request{}, false
	}
	return Request{S1: name, Race: race, Gender: gender, Head: head, Hair: hair}, true
}

// decStartWork 解 0x1027：**小端 i32 的工作入口号**。
//
// 布局是实测的：调 `BeginWork(0x11223344)`，服务端收到 `44 33 22 11`。
// 普通打工传 game_work.work_id；钓鱼/挖矿传生活技能 id 11006/11007，
// 场景层再按角色熟练度选择 game_work 中的实际档位。
func decStartWork(payload []byte) (Request, bool) {
	r := newReader(payload)
	id := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{ID32: uint32(id)}, true
}

// decOneID 解「一个小端 i32」这种最常见的载荷。
//
// Attack / PickItem / EquipItem / UnequipItem 都是这个形状 ——
// 实测: 传 0x22334455 收到 `55 44 33 22`。
func decOneID(payload []byte) (Request, bool) {
	r := newReader(payload)
	v := r.u32()
	if !r.done() {
		return Request{}, false
	}
	return Request{ID32: v, TargetID: v}, true
}

// decUseItem 解普通物品使用分支 0x1015：itemId:I32, tab:U8, slot:I32。
// 捕捉道具走同一客户端方法的另一分支 0x102b，不会到这里。
func decUseItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	item := r.u32()
	tab := r.u8()
	slot := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqUseItem, ID32: item, U8: tab, Slot: slot}, true
}

func decApplyAvatar(payload []byte) (Request, bool) {
	r := newReader(payload)
	bagTab, bagSlot, targetSlot := r.u8(), r.i32(), r.i32()
	kind, targetKind, targetTab := r.u8(), r.u8(), r.u8()
	// 正式1.5.8的SuitFxDB.IsDragon分支以kind=2调用ApplyAvatar。
	if !r.done() || kind > 2 || targetKind > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqApplyAvatar, FromTab: bagTab, FromSlot: bagSlot,
		ToSlot: targetSlot, Mode: kind, TargetKind: targetKind, Tab: targetTab}, true
}

// decChangeSetItem 解 1.5.8 快速换装托管操作：
//
//	放入 op=0, bagTab:U8, bagSlot:I32, cell:I32
//	取出 op=1, 0:U8, 0:I32, cell:I32
//
// cell 是客户端装备部位号 1..11，不是 UI 数组下标。物品是否真属于该部位
// 仍由场景用 PostgreSQL 装备定义复核。
func decChangeSetItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	op, tab, slot, cell := r.u8(), r.u8(), r.i32(), r.i32()
	if !r.done() || op > 1 || cell < 1 || cell > 11 ||
		op == 0 && slot < 0 || op == 1 && (tab != 0 || slot != 0) {
		return Request{Kind: ReqChangeSetItem}, false
	}
	return Request{Kind: ReqChangeSetItem, Mode: op, Tab: tab, Slot: slot,
		EquipSlot: uint8(cell)}, true
}

// DecodeApplyAvatar 单独解已闭合但尚未纳入旧通用注册表基线的0x1036。
func DecodeApplyAvatar(payload []byte) (Request, bool) { return decApplyAvatar(payload) }

// decMoveBagItem 解 0x100d：客户端当前分页、来源格与目标格。
// 格号只是客户端引用；实际容量、来源分页和物品状态由场景内权威背包复核。
func decMoveBagItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, from, to := r.u8(), r.i32(), r.i32()
	if !r.done() || from < 0 || to < 0 {
		return Request{}, false
	}
	return Request{Kind: ReqMoveBagItem, Tab: tab, FromSlot: from, ToSlot: to}, true
}

// decSortBag 解 0x1078：客户端只上报当前分页；排序结果完全由服务端生成。
func decSortBag(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab := r.u8()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqSortBag, Tab: tab}, true
}

// decDropItem 解 0x100f：分页、格号、世界落点、物品号与客户端确认位。
func decDropItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot := r.u8(), r.i32()
	x, y, item := r.i32(), r.i32(), r.u32()
	confirmed := r.u8()
	if !r.done() || confirmed > 1 {
		return Request{}, false
	}
	return Request{
		Kind: ReqDropItem, Tab: tab, Slot: slot, X: x, Y: y, ID32: item, Flag: confirmed,
	}, true
}

// decSplitItem 解 0x1016：分页、来源格和拆出的数量。
func decSplitItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot, count := r.u8(), r.i32(), r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqSplitItem, Tab: tab, Slot: slot, Count: count}, true
}

// decOpenShop 解 OpenShop(shopId)。商店号来自 NPC 的 SellList。
func decOpenShop(payload []byte) (Request, bool) {
	r := newReader(payload)
	shop := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqOpenShop, ID32: uint32(shop)}, true
}

// decBuyItem 解 BuyItem(itemId,count)。商店号不在包内，由场景保存最近一次
// 成功 OpenShop 的短生命周期上下文。
func decBuyItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	item, count := r.u32(), r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqBuyItem, ID32: item, Count: count}, true
}

// decSellItem 解 SellItem(tab,slot,count,itemId,confirmed)。tab/slot/itemId 都是
// 客户端引用，场景会对照权威背包逐项复核。
func decSellItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	tab, slot, count := r.u8(), r.i32(), r.i32()
	item, confirmed := r.u32(), r.u8()
	if !r.done() || confirmed > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqSellItem, U8: tab, Slot: slot, Count: count,
		ID32: item, Flag: confirmed}, true
}

// decRepair 解 Repair/RepairConfirm(mode,slot,targetKind,tab)。客户端枚举已由
// RepairDlg.PickSlot/PickBag 与三个商店按钮闭合：mode 0普通/1特殊/2全部，
// targetKind 0背包/1已穿戴。
func decRepair(kind ReqKind) decoder {
	return func(payload []byte) (Request, bool) {
		r := newReader(payload)
		mode, slot, targetKind, tab := r.u8(), r.i32(), r.u8(), r.u8()
		if !r.done() || mode > 2 || targetKind > 1 || mode != 2 && slot < 0 {
			return Request{}, false
		}
		return Request{Kind: kind, U8: mode, Slot: slot, Flag: targetKind, Tab: tab}, true
	}
}

// decTradeOffer decodes 0x1041. The item list is a nullable tail: money alone is
// a valid packet, while a present tail starts with U8 count followed by
// (tab:U8, slot:I32, count:I32) rows.
func decTradeOffer(payload []byte) (Request, bool) {
	r := newReader(payload)
	money := r.i64()
	if !r.ok() || money < 0 {
		return Request{}, false
	}
	if r.p == len(r.b) {
		return Request{Kind: ReqTradeOffer, Money: money}, true
	}
	count := int(r.u8())
	items := make([]TradeItemRef, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, TradeItemRef{Tab: r.u8(), Slot: r.i32(), Count: r.i32()})
	}
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqTradeOffer, Money: money, TradeItems: items}, true
}

// decEquipFxMask 解 0x1082。客户端 SendEquipFx 发送一个 I32，
// 值直接来自 CharData.EquipFxHideMask()；这是显示偏好位图，不是装备 ID。
func decEquipFxMask(payload []byte) (Request, bool) {
	r := newReader(payload)
	mask := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{EquipFxMask: mask}, true
}

// decHotbar 只接受 1.5.8 的 151 字节：前 20 格、expanded、后 10 格。
func decHotbar(payload []byte) (Request, bool) {
	const payloadBytes = HotbarSlotCount*5 + 1
	if len(payload) != payloadBytes {
		return Request{}, false
	}
	r := newReader(payload)
	var slots [HotbarSlotCount]HotbarSlot
	for i := 0; i < PrimaryHotbarSlotCount; i++ {
		slots[i] = HotbarSlot{ID: r.i32(), Kind: r.u8()}
	}
	expanded := r.u8()
	for i := PrimaryHotbarSlotCount; i < HotbarSlotCount; i++ {
		slots[i] = HotbarSlot{ID: r.i32(), Kind: r.u8()}
	}
	if !r.done() || expanded > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqSaveHotbar, Hotbar: slots, Expanded: expanded == 1}, true
}

func decBool(kind ReqKind) decoder {
	return func(payload []byte) (Request, bool) {
		r := newReader(payload)
		value := r.u8()
		if !r.done() || value > 1 {
			return Request{}, false
		}
		return Request{Kind: kind, U8: value}, true
	}
}

func decI32(kind ReqKind) decoder {
	return func(payload []byte) (Request, bool) {
		r := newReader(payload)
		value := r.i32()
		if !r.done() {
			return Request{}, false
		}
		return Request{Kind: kind, ID32: uint32(value)}, true
	}
}

func decOneU8(kind ReqKind) decoder {
	return func(payload []byte) (Request, bool) {
		r := newReader(payload)
		value := r.u8()
		if !r.done() {
			return Request{}, false
		}
		return Request{Kind: kind, U8: value}, true
	}
}

func decCraftItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	makeType, product := r.u8(), r.u32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqCraftItem, U8: makeType, ID32: product}, true
}

func decRefineItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	mode, equipSlot, execute := r.u8(), r.u8(), r.u8()
	tab, bagSlot, protect := r.u8(), r.i32(), r.u8()
	if !r.done() || mode > 1 || execute > 1 || protect > 1 {
		return Request{}, false
	}
	return Request{
		Kind: ReqRefineItem, Mode: mode, EquipSlot: equipSlot, Execute: execute == 1,
		Tab: tab, Slot: bagSlot, Protect: protect == 1,
	}, true
}

func decDrillItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	equipSlot, execute, protect := r.u8(), r.u8(), r.u8()
	targetKind, tab, bagSlot := r.u8(), r.u8(), r.i32()
	if !r.done() || execute > 1 || protect > 1 || targetKind > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqDrillItem, EquipSlot: equipSlot, Execute: execute == 1,
		Protect: protect == 1, TargetKind: targetKind, Tab: tab, Slot: bagSlot}, true
}

func decInlayItem(payload []byte) (Request, bool) {
	r := newReader(payload)
	equipSlot, hole, runeTab := r.u8(), r.u8(), r.u8()
	runeSlot := r.i32()
	targetKind, tab, bagSlot := r.u8(), r.u8(), r.i32()
	if !r.done() || targetKind > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqInlayItem, EquipSlot: equipSlot, Hole: hole,
		RuneTab: runeTab, RuneBagSlot: runeSlot, TargetKind: targetKind, Tab: tab, Slot: bagSlot}, true
}

func decUseItemOn(payload []byte) (Request, bool) {
	r := newReader(payload)
	tool, targetKind, tab, slot := r.u32(), r.u8(), r.u8(), r.i32()
	if !r.done() || targetKind > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqUseItemOn, ID32: tool, TargetKind: targetKind, Tab: tab, Slot: slot}, true
}

func decWashAffix(payload []byte) (Request, bool) {
	r := newReader(payload)
	kind, tab, slot, index := r.u8(), r.u8(), r.i32(), r.i32()
	if !r.done() || kind > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqWashAffix, TargetKind: kind, Tab: tab, Slot: slot, Index: index}, true
}

func decGlowMode(payload []byte) (Request, bool) {
	r := newReader(payload)
	mode := r.u8()
	if !r.done() || mode > 16 {
		return Request{}, false
	}
	return Request{Kind: ReqSetGlowMode, U8: mode}, true
}

func decPKMode(payload []byte) (Request, bool) {
	r := newReader(payload)
	mode := r.u8()
	if !r.done() || mode > 4 {
		return Request{}, false
	}
	return Request{Kind: ReqSetPKMode, U8: mode}, true
}

func decOneString(kind ReqKind, allowEmpty bool) decoder {
	return func(payload []byte) (Request, bool) {
		r := newReader(payload)
		value := r.str()
		if !r.done() || (!allowEmpty && value == "") {
			return Request{}, false
		}
		return Request{Kind: kind, S1: value}, true
	}
}

// decTargetName 解组队邀请等“在线实体号 + 角色名”请求。服务端会同时校验两项，
// 防止客户端拿旧实体号对另一个名字发起交互。
func decTargetName(kind ReqKind) decoder {
	return func(payload []byte) (Request, bool) {
		r := newReader(payload)
		target := r.u32()
		name := r.str()
		if !r.done() || target == 0 || name == "" {
			return Request{}, false
		}
		return Request{Kind: kind, TargetID: target, S1: name}, true
	}
}

func decSysMsgRespond(payload []byte) (Request, bool) {
	r := newReader(payload)
	id := r.i32()
	accept := r.u8()
	if !r.done() || id <= 0 || accept > 1 {
		return Request{}, false
	}
	return Request{Kind: ReqSysMsgRespond, RequestID: id, U8: accept}, true
}

// decGossip 解私聊正文和最多四个分享坐标。当前业务只开放纯文本，但必须把分享
// 尾完整读完，才能把“格式正确但功能未开放”与坏包区分开。
func decGossip(payload []byte) (Request, bool) {
	r := newReader(payload)
	to := r.str()
	message := r.str()
	count := r.u8()
	if count > 4 {
		return Request{}, false
	}
	shares := make([]ShareRef, 0, count)
	for i := uint8(0); i < count; i++ {
		shares = append(shares, ShareRef{Kind: r.u8(), Tab: r.u8(), Slot: r.i32()})
	}
	if !r.done() || to == "" || message == "" {
		return Request{}, false
	}
	return Request{Kind: ReqGossip, S1: to, S2: message, ID32: uint32(count), Shares: shares}, true
}

// 0x100b 的子命令号。
//
// 子命令号 sub=2..12 分别对应宠物操作；当前未定义 sub=1。
const (
	petSubAddPoint = 2  // PetAddPoint(attrIdx)
	petSubRename   = 3  // PetRename(name)
	petSubSetSkill = 4  // PetSetSkill(skillId)   选择当前启用的辅助技能
	petSubSwitch   = 5  // CharData.SwitchPet(idx) 双击同一格切换出战/收回
	petSubUseSkill = 6  // PetUseSkill(skillId)
	petSubRecall   = 7  // PetRecall(slot)
	petSubDeploy   = 8  // PetDeploy(storeIdx)
	petSubHatch    = 9  // PetHatch(slot)         孵化
	petSubShow     = 10 // PetShow(show)
	petSubAwaken   = 11 // PetAwaken()            非 Win05.petdlg 可达分支
	petSubSwap     = 12 // PetSwap(from,to)
)

// decPetCmd 解 0x100b：`sub:U8 arg:I32 text:Str`。
//
// 一个 opcode 底下挂着 11 个动作，首字节才是真正要干什么。Win05.petdlg 与背包
// 宠物入口当前接入：
//
//	2/3        单点加属性 / 改名
//	4/6        清除当前技能 / 使用或切换技能
//	5/7/8/9    切换 / 收回 / 放出 / 孵化
//	10         显示切换（false 复用 RecallPet，true 由 ReqPetOther 精确转发）
//	12         宠物栏排序
//
// 2/3/10(true)/12 继续保留 ReqPetOther 这个既有解码种类，由 session 按已经验证的
// sub 精确翻译为 scene 命令；这样不改变其余请求枚举的稳定编号。11 PetAwaken 属于
// 独立 PetArousalDlg，仍只记录。
//
// ⚠️ `PetShow(true)` 本身不携带槽位；当前没有宠物出战时，服务端不能凭空猜该放哪只。
// 放出走 SwitchPet/PetDeploy，`PetShow(false)` 则可以无歧义地表达收回。
func decPetCmd(payload []byte) (Request, bool) {
	r := newReader(payload)
	sub := r.u8()
	arg := r.i32()
	text := r.str()
	if !r.done() {
		return Request{}, false
	}
	req := Request{Slot: arg, Flag: sub, S1: text}
	switch sub {
	case petSubSetSkill:
		req.Kind = ReqSetPetSkill
	case petSubUseSkill:
		req.Kind = ReqUsePetSkill
	case petSubSwitch:
		req.Kind = ReqTogglePet
	case petSubDeploy:
		req.Kind = ReqSummonPet
	case petSubRecall:
		req.Kind = ReqRecallPet
	case petSubShow:
		if arg == 0 {
			req.Kind = ReqRecallPet
		}
	case petSubHatch:
		req.Kind = ReqHatchPet
	case petSubSwap:
		to, err := strconv.Atoi(text)
		if err != nil || to < 0 || to > 9 {
			return Request{}, false
		}
		req.FromSlot = arg
		req.ToSlot = int32(to)
	}
	return req, true
}

// decCapture 解 0x102b：**两个**小端 i32 —— 目标实体、捕捉道具。
//
// 实测: `Capture(0x0A0B0C0D, 0x0E0F1011)` 收到
// `0d 0c 0b 0a 11 10 0f 0e`，两个值一字不差。
//
// ⚠️ 这个号**两条路径共用**：`NetClient.Capture` 和 `NetClient.UseItem` 在
// 用捕捉道具时（itemId ∈ {0x0d36,0x0d37,0x0d38,0x0d3c,0x0d3d}，或 0x182c3 且
// CaptureTargetId≠0）都发它，两边 wire 都是 `targetId:I32 + itemId:I32`，
// 所以同一个解码器就够（见 c2s_map.json 的 UseItem.variants）。
func decCapture(payload []byte) (Request, bool) {
	r := newReader(payload)
	target := r.u32()
	item := r.u32()
	if !r.done() {
		return Request{}, false
	}
	return Request{TargetID: target, ID32: item}, true
}

// decOneStr 解「就一个字符串」这种载荷。
//
// 实测 0x1032(TaskNpcList): 站在「主持人王小丫」旁边时客户端发的是
// `12 00` + 那 6 个汉字的 UTF-8 —— u16 长度 + 字节, 与 Str 写方法一致。
func decOneStr(payload []byte) (Request, bool) {
	r := newReader(payload)
	v := r.str()
	if !r.done() || v == "" {
		return Request{}, false
	}
	return Request{S1: v}, true
}

// decMoveXY 解 0x1017(PlayerMove): **就两个小端 i32, 没别的**。
//
// 实证: 角色站在 (9000,5101) 时客户端发的是
//
//	28 23 00 00 ed 13 00 00
//	└─ 9000 ─┘ └─ 5101 ─┘
//
// 与 `NetClient.PlayerMove(Int32, Int32)` 的签名一致。
// 这是客户端走路时**每 0.6 秒一次**的位置上报, 是真正的移动包。
func decMoveXY(payload []byte) (Request, bool) {
	r := newReader(payload)
	x := r.i32()
	y := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{X: x, Y: y}, true
}

// decMove 解 0x100a(SavePos)：i32 mapId + i32 X + i32 Y + u8 entered。
func decMove(payload []byte) (Request, bool) {
	r := newReader(payload)
	mapID := r.i32()
	x := r.i32()
	y := r.i32()
	entered := r.u8()
	if !r.done() {
		return Request{}, false
	}
	return Request{MapID: mapID, X: x, Y: y, Flag: entered}, true
}

// decOpenTransport 解 0x1071：NPC 落位包中的 TransList 编号。
func decOpenTransport(payload []byte) (Request, bool) {
	r := newReader(payload)
	list := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{TransList: list}, true
}

// decChooseTransport 解 0x1072：TransList 编号与 TransportUI.sel 零基索引。
func decChooseTransport(payload []byte) (Request, bool) {
	r := newReader(payload)
	list := r.i32()
	index := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{TransList: list, TransIndex: index}, true
}

// decUseSkill 解共享的 0x1013：
//
//	I32 skillId | U8 mode | I32 x | I32 y | I32 targetId
//
// UseSkill 固定写 mode=0、x=y=0；UseSkillAt 固定写 mode=1 并带目标点。
func decUseSkill(payload []byte) (Request, bool) {
	r := newReader(payload)
	skillID := r.u32()
	mode := r.u8()
	x := r.i32()
	y := r.i32()
	targetID := r.u32()
	if !r.done() || mode > 1 {
		return Request{}, false
	}
	return Request{ID32: skillID, Flag: mode, X: x, Y: y, TargetID: targetID}, true
}

// ── 未知 opcode 的观测 ──

// UnknownLog 记下协议已定义、但服务端业务尚未接线的 opcode，以及真正异常的包。
// 运行期样本用于判断客户端实际触发了哪条未实现入口，并保留畸形载荷的诊断证据。
type UnknownLog struct {
	mu   sync.Mutex
	seen map[Op]*UnknownStat
}

// UnknownStat 是一个未知 opcode 的观测记录。
type UnknownStat struct {
	Op    Op
	Count int
	// MinLen/MaxLen 载荷长度区间。定长包两者相等 —— 那是个很强的线索,
	// 说明载荷里没有变长字符串。
	MinLen, MaxLen int
	// Sample 第一次见到时的载荷前若干字节。留着人工比对用。
	Sample []byte
}

// sampleBytes 每个未知包留多少字节样本。
//
// 32 字节足够看出结构(几个 i32、有没有 u16 长度前缀), 又不至于把日志撑爆。
const sampleBytes = 32

// UnknownSampleFingerprint 只暴露样本摘要，避免未知包错位时把账号、口令或聊天
// 原文写进日志。原始短样本仍只在进程内用于协议诊断，不落日志。
func UnknownSampleFingerprint(sample []byte) string {
	sum := sha256.Sum256(sample)
	return hex.EncodeToString(sum[:8])
}

// NewUnknownLog 建一个观测器。
func NewUnknownLog() *UnknownLog {
	return &UnknownLog{seen: map[Op]*UnknownStat{}}
}

// Note 记一次未知 opcode。返回 true 表示这是**第一次**见到它 ——
// 调用方可以只在第一次打日志，避免刷屏。
//
// 加锁是因为多个连接会并发写同一个观测器。
func (u *UnknownLog) Note(op Op, payload []byte) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	st, seen := u.seen[op]
	if !seen {
		n := len(payload)
		if n > sampleBytes {
			n = sampleBytes
		}
		st = &UnknownStat{Op: op, MinLen: len(payload), MaxLen: len(payload),
			Sample: append([]byte(nil), payload[:n]...)}
		u.seen[op] = st
	}
	st.Count++
	if len(payload) < st.MinLen {
		st.MinLen = len(payload)
	}
	if len(payload) > st.MaxLen {
		st.MaxLen = len(payload)
	}
	return !seen
}

// Snapshot 取一份观测结果，按 opcode 升序。
func (u *UnknownLog) Snapshot() []UnknownStat {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]UnknownStat, 0, len(u.seen))
	for _, st := range u.seen {
		out = append(out, *st)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Op < out[j-1].Op; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Len 返回见过多少种未知 opcode。
func (u *UnknownLog) Len() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.seen)
}

func decAllocPoints(payload []byte) (Request, bool) {
	// AllocPoints 发 7 个 I32, 全是**增量**(不是绝对值): 客户端 EquipDlg.pend[]
	// 是一个 7 元增量数组, 加点界面把点数累加进去, 确认时整份发过来。
	//
	// 依据阶段 1 资产 c2s_map.json / 上行包定义.md 的 AllocPoints.variants[].schema
	// (语义级 verified, 来源 EquipDlg.OnGUI 唯一 caller + CharData.attrs 下标):
	//
	//   wire[0] strDelta (attrs[10])   wire[1] dexDelta (attrs[21])
	//   wire[2] conDelta (attrs[11]=体质/VIT)  wire[3] agiDelta (attrs[12])
	//   wire[4] intDelta (attrs[13])   wire[5] d7[5] 语义未证明(positional, 不猜)
	//   wire[6] spiDelta (attrs[14])
	//
	// 没有"总消耗"字段 —— 消耗 = 六项已知增量之和, 服务端自己算并校验。
	// 这里保持 wire 原序不重排, 命名映射交给 session 层, 避免在解码层臆造语义。
	r := newReader(payload)
	d := [7]int32{}
	for i := range d {
		d[i] = r.i32()
	}
	if !r.done() {
		return Request{}, false
	}
	return Request{
		Kind: ReqAllocPoints,
		Vals: []int32{d[0], d[1], d[2], d[3], d[4], d[5], d[6]},
	}, true
}

func decUpgradeSkill(payload []byte) (Request, bool) {
	// UpgradeSkill 发 1 个 I32: 技能号
	r := newReader(payload)
	skillID := r.i32()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqUpgradeSkill, ID32: uint32(skillID)}, true
}

func decRevive(payload []byte) (Request, bool) {
	// Revive 发 1 个 U8: 复活类型(0=回城, 1=原地)
	// 当前阶段只支持回城, 原地复活需要道具(复活卷轴)
	r := newReader(payload)
	reviveType := r.u8()
	if !r.done() {
		return Request{}, false
	}
	return Request{Kind: ReqRevive, U8: reviveType}, true
}

// DecodeRideTarget 解正式1.5.8的RideTogether(0x107e)/RideInvite(0x108c)。
func DecodeRideTarget(payload []byte) (int32, bool) {
	r := newReader(payload)
	target := r.i32()
	return target, r.done() && target >= 0
}

const (
	OpRideTogether Op = 0x107e
	OpRideInvite   Op = 0x108c
)
