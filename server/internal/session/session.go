// Package session 是会话层: 账号 ↔ 连接 ↔ 角色 的状态机, 鉴权, 防顶号。
//
// 职责边界(刻意划得很窄): 它管**这条连接现在处于哪个阶段**, 以及把上行包翻成
// 场景命令投出去。它**不持有任何世界状态**, 也不做玩法判断 —— 那些都在场景里。
//
// 判断一段代码该不该写在这: 问"重连之后它还成立吗"。
// 阶段、账号、当前角色是连接的属性, 归这里; 血量、位置、背包是世界的属性, 不归。
package session

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/party"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/trade"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

// clientBagSlot translates page-local wire coordinates only at the session boundary.
func clientBagSlot(tab uint8, slot int) int {
	if result, ok := domain.ClientBagSlot(tab, slot); ok {
		return result
	}
	return -1
}
func clientBagSlotIfBag(kind uint8, tab uint8, slot int) int {
	if kind != 0 {
		return slot
	}
	return clientBagSlot(tab, slot)
}

// Stage 会话阶段。状态机严格按此推进, 不许跳级。
type Stage int

const (
	StageConnected Stage = iota // 已连接, 未认证
	StageAuthed                 // 已认证(账号确定)
	StageInGame                 // 已进入某个场景
	StageClosed
)

// Sink 是网关连接出口(net.Conn 实现)。会话拿它发字节。
type Sink interface {
	Send(inner []byte)
	Close()
	SessionID() int64
}

// Deps 是会话依赖(构造注入, 便于测试)。
type Deps struct {
	Announcements domain.AnnouncementPool
	Store         store.Store
	Router        *scene.Router
	// HelpTopics 是 PostgreSQL 在开服时加载的普通角色帮助主题。
	HelpTopics      []domain.HelpTopic
	HelpTopicsKnown bool
	WarehouseRule   domain.WarehouseRule
	WardrobeRule    domain.WardrobeRule
	MailRule        domain.MailRule
	ChatRules       domain.ChatChannelRules
	WriteBack       *store.WriteBack
	// AccountSessions 是全服共享的账号在线会话表。所有会话必须使用同一实例；
	// nil 时不提供同账号后登录踢旧连接的保证。
	AccountSessions *AccountSessions
	// CharGate 是全服共享的角色所有权/离场提交闸门。所有会话必须使用同一实例；
	// nil 时不提供跨连接重进保护。
	CharGate store.CharacterGate
	// Items 是只读客户端物品定义。建角要据此构造原服出生背包；nil 或缺定义时
	// 必须拒绝建角，不能只落角色行。
	Items map[domain.ItemID]domain.ItemDef
	// Handshake 用服务端 RSA 私钥解握手包并产出会话密钥。
	Handshake *wire.HandshakeDecryptor
	SpawnPos  func(race domain.Race) domain.Pos
	// Dungeons 用于重登解析已保存的副本实例：原实例失效时只能落配置出口，
	// 不能因为一条过期角色存档自动创建新副本。
	Dungeons domain.DungeonTable
	// GMLanding 返回一张地图经过静态数据验证的安全落点。nil 或 false 时 goto
	// 拒绝执行，不能把角色送到未知坐标再靠重登碰运气。
	GMLanding func(mapID int32) (domain.Pos, bool)
	// Unknown 收集协议已定义、但业务尚未接线的 opcode。**全服共用一个。**
	// 记录客户端触发但尚未接入业务处理的协议操作。
	Unknown *protocol.UnknownLog
	// Party 是全服队伍名册。队伍跨场景, 所以名册不能放进任何一个场景。
	Party *party.Registry
	// Online 是全服在线索引: 角色 → 他现在在哪张图、是哪个实体。
	// 任何"对另一个人做点什么"的功能都要它 —— 会话手上只有自己的地址。
	Online *online.Registry
	// Requests 是待处理的好友请求。**不落盘** ——
	// 加好友是当面同意的事, 存下来只会让人上线时收到一堆早就忘了的请求。
	Requests *social.Requests
	// Trades is the process-wide registry of active two-client handshakes. Inventory
	// references remain scene-owned and enter this registry only after validation.
	Trades *trade.Registry
	Log    *slog.Logger
}

// Session 一个客户端会话。
//
// 字段由该连接的读循环串行访问(网关保证 OnPacket 不并发), 故大部分无需锁;
// 只有 stage/entity 这些要被别的 goroutine 看到的才用锁保护。
type Session struct {
	sink Sink
	deps Deps
	log  *slog.Logger

	mu            sync.Mutex
	stage         Stage
	account       *store.Account
	clientAuthTag []byte
	char          *domain.Character
	// entity 是本会话在场景里的实体 id。**进世界之前分配好**, 这样后续每条命令
	// 都能直接带上它, 不需要等场景回调告诉我们"你是几号"。
	entity domain.EntityID
	scene  domain.SceneID
	// lease 从进世界前一直持有到离场最终快照成功提交。它与 char/entity/scene
	// 同属会话绑定，必须在同一把锁下取出并清空。
	lease store.CharacterLease
	// awaitMapAck 表示"服务端已经把人跨图了, 但客户端还没确认自己到了新图"。
	//
	// 切图后客户端可能继续发送旧地图的坐标。等待目标地图确认后再接受移动，
	// 避免旧坐标覆盖新地图位置或触发额外传送。
	//
	// 只有目标地图的 entered=1 才投递 MapReady；场景完成快照后释放闸门。
	// 普通 entered=0 存点不能作为建图完成，实际耗时不受固定秒数限制。
	awaitMapAck bool
	mapEpoch    uint64
	// mutedUntil 是 GM 对本次在线会话施加的禁言截止时间；不落盘，断线即清。
	mutedUntil time.Time
	// seenTips 是本次在线期间已经展示或由客户端确认展示过的固定新手提示。
	// 它与数据库的 character_seen_tips 同口径，供 0x8008 mode=5 服务端提示
	// 在场景热路径上做 O(1) 去重。
	seenTips map[int32]struct{}
	// blocks 是当前角色的单向屏蔽缓存。进世界前从 PostgreSQL 读入，
	// 添删成功后重读；在线聊天与交互投递只读这张 O(1) 表。
	blocks   map[domain.CharID]uint8
	lastChat map[uint8]time.Time
}

// New 创建会话(网关的 HandlerFactory 调用)。
func New(sink Sink, deps Deps) *Session {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Session{
		sink: sink, deps: deps,
		log:   deps.Log.With("sid", sink.SessionID()),
		stage: StageConnected,
	}
}

// ── net.Handler 实现 ──

func (s *Session) OnHandshake(hs []byte) (wire.Keys, error) {
	// 用服务端 RSA 私钥直接解客户端握手包(260B = 4B 头 + 256B OAEP-SHA1 密文),
	// 还原出 aesKey(32)‖macKey(32)。每连接独立解密, 无旁路的密钥漂移竞态。
	if s.deps.Handshake == nil {
		return wire.Keys{}, ErrNoKeys
	}
	k, err := s.deps.Handshake.Open(hs)
	if err != nil {
		s.log.Warn("握手解密失败", "err", err)
		return wire.Keys{}, err
	}
	s.mu.Lock()
	s.clientAuthTag = wire.ClientAuthTag(k, "1.6.0")
	s.mu.Unlock()
	return k, nil
}

// OnPacket 处理一条上行包。
//
// **解码不在这里做** —— 交给 protocol.Decode，它是一张 opcode → 解码器的表。
// 这里只做"请求 → 场景命令"的翻译，因为只有会话同时认识协议层和游戏层
// (协议层不许认识游戏层，那是 internal/arch 的红线之一)。
//
// 认不出来的 opcode **不再静默丢掉**：记进 UnknownLog。完整协议映射已经闭合；
// 这里的未知项表示对应业务入口尚未接线，样本用于定位客户端实际触发的优先级。
func (s *Session) OnPacket(op uint16, payload []byte) {
	s.mu.Lock()
	loading := s.awaitMapAck
	s.mu.Unlock()
	if loading {
		if !protocol.AllowedDuringMapLoad(protocol.Op(op)) {
			s.log.Debug("加载地图期间忽略玩法请求", "op", logHex(op))
			return
		}
	}
	if protocol.Op(op) == protocol.OpRideTogether || protocol.Op(op) == protocol.OpRideInvite {
		target, ok := protocol.DecodeRideTarget(payload)
		if ok {
			s.onRideInvite(domain.EntityID(target), protocol.Op(op) == protocol.OpRideTogether)
		}
		return
	}
	if protocol.FamilyOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeFamily(protocol.Op(op), payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		s.onFamilyRequest(req)
		return
	}
	if protocol.ApprenticeOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeApprentice(protocol.Op(op), payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		s.onApprenticeRequest(req)
		return
	}
	if protocol.WarehouseExpansionOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeWarehouseExpansion(payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		s.postToScene(scene.ExpandWarehouse{ID: s.entityID()})
		return
	}
	if protocol.CommerceOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeCommerce(protocol.Op(op), payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		s.onCommerceRequest(req)
		return
	}
	if protocol.BlockOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeBlock(protocol.Op(op), payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		s.onBlockRequest(req)
		return
	}
	if protocol.CapabilityOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeClientCaps(payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		// 固定 1.5.8 服务端不做运行时能力协商。该包只需被完整消费，
		// 背包/仓库/试衣/宠物均直接走 1.5.8 标准协议。
		return
	}
	if protocol.ChangeSetOpcode(protocol.Op(op)) {
		req, ok := protocol.DecodeChangeSet(protocol.Op(op), payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		if req.Kind == protocol.ReqChangeSetItem {
			s.postToScene(scene.ChangeSetItem{ID: s.entityID(), Put: req.Mode == 0,
				BagTab: req.Tab, BagSlot: clientBagSlot(req.Tab, int(req.Slot)), Cell: domain.EquipSlot(req.EquipSlot)})
		} else {
			s.postToScene(scene.ChangeSetSwap{ID: s.entityID()})
		}
		return
	}
	if protocol.Op(op) == protocol.OpApplyAvatar {
		req, ok := protocol.DecodeApplyAvatar(payload)
		if !ok {
			s.noteUnknown(op, payload, req.Kind)
			return
		}
		s.postToScene(scene.ApplyAvatar{ID: s.entityID(), BagTab: req.FromTab,
			BagSlot: int32(clientBagSlot(req.FromTab, int(req.FromSlot))), TargetSlot: req.ToSlot, Kind: req.Mode,
			TargetKind: req.TargetKind, TargetTab: req.Tab})
		return
	}
	// OpChat 的协议字段已闭合，但聊天当前只开放地图纯文本这一条窄入口；
	// 单独解码可避免把尚未开放的频道/分享形态混入通用玩法注册表。
	if op == protocol.OpChat {
		req, ok := protocol.DecodeChat(payload)
		if !ok {
			s.noteUnknown(op, payload, protocol.ReqChat)
			return
		}
		s.onChat(req)
		return
	}
	req, ok := protocol.Decode(protocol.Op(op), payload)
	if !ok {
		s.noteUnknown(op, payload, req.Kind)
		return
	}
	switch req.Kind {
	case protocol.ReqLogin:
		// Every production connection completes OnHandshake before OnPacket.
		// Never retain the negotiated encryption keys in the session itself.
		if s.deps.Handshake != nil {
			s.mu.Lock()
			valid := len(s.clientAuthTag) == 32 && hmac.Equal(s.clientAuthTag, req.ClientAuthTag)
			s.mu.Unlock()
			if !valid {
				s.sink.Send(protocol.LoginResult(1, "客户端认证失败，请重新连接"))
				s.sink.Close()
				return
			}
		}
		s.log.Info("识别客户端协议", "version", req.ClientVersion,
			"vm", req.ClientVM, "debug", req.ClientDebug)
		s.onLogin(req)
	case protocol.ReqRegister:
		s.onRegister(req)
	case protocol.ReqResetPassword:
		s.onResetPassword(req)
	case protocol.ReqChangePassword:
		s.onChangePassword(req)
	case protocol.ReqChangeSecurityCode:
		s.onChangeSecurityCode(req)
	case protocol.ReqServerStat:
		s.sink.Send(protocol.OnlineCount(s.onlineCount()))
	case protocol.ReqPing:
		// 第一项原样回显供客户端算 RTT；第二项是定义要求的服务端 Unix 毫秒。
		s.sink.Send(protocol.HeartbeatReply(req.TickMS, time.Now().UnixMilli()))
	case protocol.ReqCreateChar:
		s.onCreateChar(req)
	case protocol.ReqDeleteChar:
		s.onDeleteChar(req)
	case protocol.ReqEnterGame:
		s.onEnter(req)
	case protocol.ReqLeaveWorld:
		// GameBootstrap.ReturnToTitle 会先重建 TitleFlow，再发 0x1049，
		// 并在同一条 TCP 上等待新的 0x8004。只做场景解绑而不刷新
		// 角色列表会让真客户端正确回到选角页却显示空列表。
		if s.leaveWorld("客户端退出世界", StageAuthed) {
			s.sendCharacterList()
		}
	case protocol.ReqMove:
		s.onMove(req)
	case protocol.ReqSavePos:
		// 存点 = 客户端主动说"把我现在的位置存下来"(停下来时发一次)。
		// 与移动走同一条路: 场景更新位置并标脏, 下个存档周期落库。
		s.onMove(req)
	case protocol.ReqOpenTransport:
		s.postToScene(scene.OpenTransport{ID: s.entityID(), List: req.TransList})
	case protocol.ReqChooseTransport:
		s.postToScene(scene.ChooseTransport{
			ID: s.entityID(), List: req.TransList, Index: req.TransIndex,
		})
	case protocol.ReqOpenShop:
		s.postToScene(scene.OpenShop{ID: s.entityID(), Shop: int32(req.ID32)})
	case protocol.ReqBuyItem:
		s.postToScene(scene.BuyItem{
			ID: s.entityID(), Item: domain.ItemID(req.ID32), Count: req.Count,
		})
	case protocol.ReqSellItem:
		s.postToScene(scene.SellItem{
			ID: s.entityID(), Tab: req.U8, Slot: clientBagSlot(req.U8, int(req.Slot)), Count: req.Count,
			Item: domain.ItemID(req.ID32), Confirmed: req.Flag == 1,
		})
	case protocol.ReqRepair:
		s.postToScene(scene.Repair{
			ID: s.entityID(), Mode: req.U8, Slot: int32(clientBagSlotIfBag(req.Flag, req.Tab, int(req.Slot))), TargetKind: req.Flag, Tab: req.Tab,
		})
	case protocol.ReqRepairConfirm:
		s.postToScene(scene.RepairConfirm{
			ID: s.entityID(), Mode: req.U8, Slot: int32(clientBagSlotIfBag(req.Flag, req.Tab, int(req.Slot))), TargetKind: req.Flag, Tab: req.Tab,
		})
	case protocol.ReqOpenMail:
		s.refreshMailList()
	case protocol.ReqSendMail:
		s.onSendMail(req)
	case protocol.ReqMailAction:
		s.onMailAction(req)
	case protocol.ReqWardrobeStore:
		s.postToScene(scene.WardrobeStore{ID: s.entityID(), Tab: req.Tab, Slot: int32(clientBagSlot(req.Tab, int(req.Slot)))})
	case protocol.ReqWardrobeWear:
		s.postToScene(scene.WardrobeWear{ID: s.entityID(), Item: domain.ItemID(req.ID32)})
	case protocol.ReqWardrobeRemove:
		s.postToScene(scene.WardrobeRemove{ID: s.entityID(), Item: domain.ItemID(req.ID32)})
	case protocol.ReqWardrobeMove:
		s.postToScene(scene.WardrobeMove{ID: s.entityID(), Item: domain.ItemID(req.ID32), To: req.Index})
	case protocol.ReqStallCreate:
		s.onStallCreate(req)
	case protocol.ReqStallEnd:
		s.onStallEnd()
	case protocol.ReqStallAdd:
		s.onStallAdd(req)
	case protocol.ReqStallDel:
		s.onStallDel(req.Index)
	case protocol.ReqStallBrowse:
		s.postToScene(scene.BrowseStall{ID: s.entityID(), Owner: domain.EntityID(req.TargetID)})
	case protocol.ReqStallDeal:
		s.onStallDeal(req)
	case protocol.ReqChangeHair:
		s.postToScene(scene.ChangeHair{ID: s.entityID(), Mode: domain.HairChangeMode(req.Mode), Target: req.U8})
	case protocol.ReqStartWork:
		s.postToScene(scene.StartWork{ID: s.entityID(), Work: domain.WorkID(req.ID32)})
	case protocol.ReqStopWork:
		s.postToScene(scene.StopWork{ID: s.entityID()})
	case protocol.ReqAttack:
		s.postToScene(scene.Attack{
			ID: s.entityID(), Target: domain.EntityID(req.TargetID), ReceivedAt: time.Now(),
		})
	case protocol.ReqQueryTargetStatus:
		s.postToScene(scene.QueryTargetStatus{
			ID: s.entityID(), Target: domain.EntityID(req.TargetID),
		})
	case protocol.ReqUseSkill:
		cmd := scene.UseSkill{
			ID: s.entityID(), Skill: domain.SkillID(req.ID32),
			Target: domain.EntityID(req.TargetID), ReceivedAt: time.Now(),
		}
		if req.Flag == 1 {
			cmd.At = &domain.Pos{X: float64(req.X), Y: float64(req.Y)}
		}
		s.postToScene(cmd)
	case protocol.ReqUseItem:
		s.postToScene(scene.UseItem{
			ID: s.entityID(), Item: domain.ItemID(req.ID32), Tab: req.U8, BagSlot: clientBagSlot(req.U8, int(req.Slot)),
		})
	case protocol.ReqApplyAvatar:
		s.postToScene(scene.ApplyAvatar{ID: s.entityID(), BagTab: req.FromTab,
			BagSlot: int32(clientBagSlot(req.FromTab, int(req.FromSlot))), TargetSlot: req.ToSlot, Kind: req.Mode,
			TargetKind: req.TargetKind, TargetTab: req.Tab})
	case protocol.ReqMoveBagItem:
		s.postToScene(scene.MoveBagItem{
			ID: s.entityID(), Tab: req.Tab, From: clientBagSlot(req.Tab, int(req.FromSlot)), To: clientBagSlot(req.Tab, int(req.ToSlot)),
		})
	case protocol.ReqSortBag:
		s.postToScene(scene.SortBag{ID: s.entityID(), Tab: req.Tab})
	case protocol.ReqOpenWarehouse:
		s.postToScene(scene.OpenWarehouse{ID: s.entityID()})
	case protocol.ReqWarehouseMove:
		// fromSlot belongs to the bag only on deposit; on withdrawal it is a warehouse slot.
		fromSlot, toSlot := req.FromSlot, req.ToSlot
		if req.Flag == s.deps.WarehouseRule.MoveDepositDir {
			fromSlot = int32(clientBagSlot(req.FromTab, int(req.FromSlot)))
		}
		s.log.Info("收到个人仓库物品请求", "dir", req.Flag, "bagTab", req.FromTab,
			"fromSlot", req.FromSlot, "warehouseTab", req.WarehouseTab,
			"count", req.Count, "toSlot", req.ToSlot)
		s.postToScene(scene.WarehouseMove{ID: s.entityID(), Dir: req.Flag,
			BagTab: req.FromTab, FromSlot: fromSlot, WarehouseTab: req.WarehouseTab,
			Count: req.Count, ToSlot: toSlot})
	case protocol.ReqWarehouseMoney:
		s.postToScene(scene.WarehouseMoney{ID: s.entityID(), Mode: req.U8, Amount: req.Money})
	case protocol.ReqWarehouseSplit:
		s.log.Info("收到个人仓库拆分请求", "warehouseTab", req.WarehouseTab,
			"slot", req.Slot, "count", req.Count)
		s.postToScene(scene.WarehouseSplit{ID: s.entityID(), Tab: req.WarehouseTab,
			Slot: req.Slot, Count: req.Count})
	case protocol.ReqWarehouseRearrange:
		s.postToScene(scene.WarehouseRearrange{ID: s.entityID(), FromTab: req.FromTab,
			FromSlot: req.FromSlot, ToTab: req.ToTab, ToSlot: req.ToSlot})
	case protocol.ReqSortWarehouse:
		s.postToScene(scene.SortWarehouse{ID: s.entityID(), Tab: req.U8})
	case protocol.ReqDropItem:
		s.postToScene(scene.DropBagItem{
			ID: s.entityID(), Tab: req.Tab, Slot: clientBagSlot(req.Tab, int(req.Slot)), Item: domain.ItemID(req.ID32),
			At: domain.Pos{X: float64(req.X), Y: float64(req.Y)}, Confirmed: req.Flag == 1,
		})
	case protocol.ReqSplitItem:
		s.postToScene(scene.SplitBagItem{
			ID: s.entityID(), Tab: req.Tab, Slot: clientBagSlot(req.Tab, int(req.Slot)), Count: req.Count,
		})
	case protocol.ReqSetItemLock:
		s.onSetItemLock(req)
	case protocol.ReqPickUp:
		s.postToScene(scene.PickUp{ID: s.entityID(), Target: domain.EntityID(req.TargetID)})
	case protocol.ReqEquip:
		s.postToScene(scene.Equip{ID: s.entityID(), BagSlot: clientBagSlot(2, int(req.ID32))})
	case protocol.ReqUnequip:
		s.postToScene(scene.Unequip{ID: s.entityID(), Slot: domain.EquipSlot(req.ID32)})
	case protocol.ReqEquipFxMask:
		s.postToScene(scene.SetEquipFXMask{ID: s.entityID(), Mask: req.EquipFxMask})
	case protocol.ReqSaveHotbar:
		var slots [domain.HotbarSlotCount]domain.HotbarSlot
		for i := range slots {
			slots[i] = domain.HotbarSlot{ID: req.Hotbar[i].ID, Kind: req.Hotbar[i].Kind}
		}
		s.postToScene(scene.SaveHotbar{ID: s.entityID(), Slots: slots, Expanded: req.Expanded})
	case protocol.ReqSetSmartCast:
		s.postToScene(scene.SetSmartCast{ID: s.entityID(), On: req.U8 == 1})
	case protocol.ReqSetPetView:
		s.postToScene(scene.SetPetView{ID: s.entityID(), Mask: int32(req.ID32)})
	case protocol.ReqSetGlowMode:
		s.postToScene(scene.SetGlowMode{ID: s.entityID(), Mode: req.U8})
	case protocol.ReqSetPKMode:
		s.postToScene(scene.SetPKMode{ID: s.entityID(), Mode: req.U8})
	case protocol.ReqSetTitle:
		s.postToScene(scene.SetTitle{ID: s.entityID(), Title: req.S1})
	case protocol.ReqInspectPlayer:
		s.postToScene(scene.InspectPlayer{ID: s.entityID(), Target: domain.EntityID(req.TargetID)})
	case protocol.ReqRenameCharacter:
		s.onRenameCharacter(req.S1)
	case protocol.ReqTradeRequest:
		s.onTradeRequest(domain.EntityID(req.TargetID))
	case protocol.ReqTradeOffer:
		s.onTradeOffer(req.Money, req.TradeItems)
	case protocol.ReqTradeLock:
		s.onTradeLock(req.U8 == 1)
	case protocol.ReqTradeConfirm:
		s.onTradeConfirm()
	case protocol.ReqTradeCancel:
		s.onTradeCancel()
	case protocol.ReqPartyCreate:
		s.onPartyCreate(req.S1)
	case protocol.ReqPartyLeave:
		s.onPartyLeave()
	case protocol.ReqPartyKick:
		s.onPartyKick(domain.EntityID(req.TargetID))
	case protocol.ReqPartyLeader:
		s.onPartyLeader(domain.EntityID(req.TargetID))
	case protocol.ReqPartyDismiss:
		s.onPartyDismiss()
	case protocol.ReqPartyInvite:
		s.onPartyInvite(domain.EntityID(req.TargetID), req.S1)
	case protocol.ReqPartyJoin:
		s.onPartyJoinRequest(domain.EntityID(req.TargetID), req.S1)
	case protocol.ReqSysMsgRespond:
		s.onSystemRequest(req.RequestID, req.U8 == 1)
	case protocol.ReqFriendRequest:
		s.onFriendRequest(req.S1)
	case protocol.ReqFriendDelete:
		s.onFriendDelete(req.S1)
	case protocol.ReqGossip:
		s.onGossipRequest(req)
	case protocol.ReqEmote:
		s.postToScene(scene.ShowEmote{ID: s.entityID(), Face: int32(req.ID32)})
	case protocol.ReqHelp:
		if s.deps.HelpTopicsKnown {
			if pkt := protocol.HelpTopics(s.deps.HelpTopics); pkt != nil {
				s.sink.Send(pkt)
			}
		}
		s.postToScene(scene.RequestNewbieTip{ID: s.entityID(), Tip: 26})
	case protocol.ReqSit:
		on := req.U8 == 1
		s.log.Debug("收到坐下状态", "entity", s.entityID(), "on", on, "raw", req.U8)
		s.postToScene(scene.SetResting{ID: s.entityID(), On: on})
	case protocol.ReqGMCommand:
		s.onGMCommand(req.S1)
	case protocol.ReqTipSeen:
		s.onTipSeen(int32(req.ID32))
	case protocol.ReqNpcTasks:
		s.postToScene(scene.NpcTasks{ID: s.entityID(), NPC: req.S1})
	case protocol.ReqAcceptQuest:
		// 不传 NPC —— 上行包里没有, 发布者由场景照任务定义查
		s.postToScene(scene.AcceptQuest{ID: s.entityID(), Quest: domain.QuestID(req.ID32)})
	case protocol.ReqAbandonQuest:
		s.postToScene(scene.AbandonQuest{ID: s.entityID(), Quest: domain.QuestID(req.ID32)})
	case protocol.ReqAllocPoints:
		vals := req.Vals
		if len(vals) != 7 {
			s.log.Warn("AllocPoints 参数数量不对", "got", len(vals))
			return
		}
		// wire 原序→命名(依据 c2s_map.json AllocPoints.schema):
		// [0]strDelta [1]dexDelta [2]conDelta(VIT) [3]agiDelta [4]intDelta [5]d7[5] [6]spiDelta
		s.postToScene(scene.AllocPoints{
			ID:    s.entityID(),
			STR:   vals[0],
			DEX:   vals[1],
			VIT:   vals[2],
			AGI:   vals[3],
			INT:   vals[4],
			Slot5: vals[5],
			SPI:   vals[6],
		})
	case protocol.ReqUpgradeSkill:
		s.postToScene(scene.UpgradeSkill{ID: s.entityID(), Skill: domain.SkillID(req.ID32)})
	case protocol.ReqLearnLife:
		s.postToScene(scene.LearnLife{ID: s.entityID(), Skill: domain.SkillID(req.ID32)})
	case protocol.ReqGatherWork:
		s.postToScene(scene.GatherWork{ID: s.entityID(), Token: domain.WorkID(req.ID32)})
	case protocol.ReqOpenFurnace:
		s.postToScene(scene.OpenFurnace{ID: s.entityID(), MakeType: req.U8})
	case protocol.ReqCraftItem:
		s.postToScene(scene.CraftItem{ID: s.entityID(), MakeType: req.U8, Product: domain.ItemID(req.ID32)})
	case protocol.ReqRefineItem:
		s.postToScene(scene.RefineItem{
			ID: s.entityID(), Mode: req.Mode, EquipSlot: req.EquipSlot, Execute: req.Execute,
			Tab: req.Tab, BagSlot: int32(clientBagSlotIfBag(req.Mode, req.Tab, int(req.Slot))), Protect: req.Protect,
		})
	case protocol.ReqDrillItem:
		s.postToScene(scene.DrillItem{ID: s.entityID(), TargetKind: req.TargetKind,
			EquipSlot: req.EquipSlot, Tab: req.Tab, BagSlot: int32(clientBagSlotIfBag(req.TargetKind, req.Tab, int(req.Slot))),
			Execute: req.Execute, Protect: req.Protect})
	case protocol.ReqInlayItem:
		s.postToScene(scene.InlayItem{ID: s.entityID(), TargetKind: req.TargetKind,
			EquipSlot: req.EquipSlot, Tab: req.Tab, BagSlot: int32(clientBagSlotIfBag(req.TargetKind, req.Tab, int(req.Slot))),
			Hole: req.Hole, RuneTab: req.RuneTab, RuneBagSlot: int32(clientBagSlot(req.RuneTab, int(req.RuneBagSlot)))})
	case protocol.ReqUseItemOn:
		s.postToScene(scene.UseItemOn{ID: s.entityID(), Tool: domain.ItemID(req.ID32),
			TargetKind: req.TargetKind, Tab: req.Tab, Slot: int32(clientBagSlotIfBag(req.TargetKind, req.Tab, int(req.Slot)))})
	case protocol.ReqWashAffix:
		s.postToScene(scene.WashAffix{ID: s.entityID(), Kind: req.TargetKind, Tab: req.Tab, Slot: int32(clientBagSlotIfBag(req.TargetKind, req.Tab, int(req.Slot))), Index: req.Index})
	case protocol.ReqRevive:
		s.postToScene(scene.Revive{ID: s.entityID(), ReviveType: req.U8})
	case protocol.ReqSubmitQuest:
		s.postToScene(scene.CompleteQuest{ID: s.entityID(), Quest: domain.QuestID(req.ID32)})
	case protocol.ReqCapturePet:
		s.postToScene(scene.CapturePet{
			ID: s.entityID(), Target: domain.EntityID(req.TargetID), Tool: domain.ItemID(req.ID32),
		})
	case protocol.ReqSummonPet:
		// 客户端给的是**宠物栏槽位号**，不是实例号 —— 它手上只有 0x800b 发下去
		// 的那份列表，槽位就是那份列表的下标。翻成实例号是场景的事，会话手上
		// 没有角色的宠物栏。
		s.postToScene(scene.SummonPetAt{ID: s.entityID(), Slot: req.Slot})
	case protocol.ReqRecallPet:
		s.postToScene(scene.RecallPet{ID: s.entityID()})
	case protocol.ReqTogglePet:
		s.postToScene(scene.TogglePetAt{ID: s.entityID(), Slot: req.Slot})
	case protocol.ReqHatchPet:
		s.postToScene(scene.HatchPetAt{ID: s.entityID(), Slot: req.Slot})
	case protocol.ReqSetPetSkill:
		s.postToScene(scene.SetPetSkill{ID: s.entityID(), Skill: domain.SkillID(req.Slot)})
	case protocol.ReqUsePetSkill:
		s.postToScene(scene.UsePetSkill{ID: s.entityID(), Skill: domain.SkillID(req.Slot)})
	case protocol.ReqPetOther:
		switch req.Flag {
		case 2:
			s.postToScene(scene.AddPetPoint{ID: s.entityID(), AttributeCode: req.Slot})
		case 3:
			s.postToScene(scene.RenamePet{ID: s.entityID(), Name: req.S1})
		case 10:
			s.postToScene(scene.SetPetShown{ID: s.entityID(), Show: req.Slot == 1})
		case 12:
			s.postToScene(scene.SwapPetSlots{ID: s.entityID(), From: req.FromSlot, To: req.ToSlot})
		default:
			// sub=11 PetAwaken 不属于 Win05.petdlg 的可达交互；保留日志，
			// 直到其独立养成窗口和服务端消耗规则有完整证据。
			s.log.Debug("宠物子命令暂未接线", "sub", req.Flag, "arg", req.Slot, "text", req.S1)
		}
	default:
		// 表里注册了但这里没接 —— 属于代码问题, 不是客户端问题, 该报出来
		s.log.Warn("已注册的请求没有处理分支", "kind", req.Kind, "op", op)
	}
}

const maxNewbieTipID = 1<<16 - 1

// onTipSeen 持久化客户端已经实际展示过的固定新手提示。
//
// 这条状态不能等场景周期存档：客户端显示提示后可能立刻断线；若上报没有即时落库，
// 下次登录又会收到同一条提示。INSERT 本身按 (角色, tipId) 幂等，重复上报无副作用。
func (s *Session) onTipSeen(tipID int32) {
	s.mu.Lock()
	char, stage := s.char, s.stage
	s.mu.Unlock()
	if stage != StageInGame || char == nil {
		s.log.Debug("未进世界时忽略新手提示已读", "tip", tipID, "stage", stage)
		return
	}
	// 正式客户端当前资源编号远小于这个上界；保留足够扩展空间，同时阻止伪造客户端
	// 用任意 I32 无限膨胀角色的已读集合。
	if tipID <= 0 || tipID > maxNewbieTipID {
		s.log.Warn("忽略非法新手提示编号", "char", char.Name, "tip", tipID)
		return
	}
	if err := s.deps.Store.MarkTipSeen(context.Background(), char.ID, tipID); err != nil {
		s.log.Error("保存新手提示已读失败", "char", char.Name, "tip", tipID, "err", err)
		return
	}
	s.mu.Lock()
	if s.stage == StageInGame && s.char != nil && s.char.ID == char.ID {
		if s.seenTips == nil {
			s.seenTips = make(map[int32]struct{})
		}
		s.seenTips[tipID] = struct{}{}
	}
	s.mu.Unlock()
}

// claimServerTip 在下发服务端推断的新手提示前做在线去重，并返回稳定角色号。
// 这里只碰内存，不能在场景 goroutine 调到的 eventSink 中同步访问数据库。
func (s *Session) claimServerTip(tipID int32) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage != StageInGame || s.char == nil || tipID <= 0 || tipID > maxNewbieTipID {
		return 0, false
	}
	if s.seenTips == nil {
		s.seenTips = make(map[int32]struct{})
	}
	if _, seen := s.seenTips[tipID]; seen {
		return 0, false
	}
	s.seenTips[tipID] = struct{}{}
	return s.char.ID, true
}

// persistServerTip 在提示已经交给非阻塞连接发送队列后异步落库。PushText 协议
// 没有客户端确认包，这是正式客户端边界；因此这里提供 at-most-once 语义，避免
// 为了等数据库把整张场景的事件循环卡住。
func (s *Session) persistServerTip(charID int64, tipID int32) {
	go func() {
		if err := s.deps.Store.MarkTipSeen(context.Background(), charID, tipID); err != nil {
			s.log.Error("保存服务端新手提示已读失败", "charID", charID, "tip", tipID, "err", err)
		}
	}()
}

// 客户端选角页固定只有三个角色位。0x8004 的角色记录本身不带 slot，客户端按
// 数组下标摆放；因此 slot 是服务端持久化约束，创建时取 [0,3) 内第一个空位。
const maxCharacterSlots = 3

func firstFreeCharacterSlot(chars []*domain.Character) (int32, bool) {
	if len(chars) >= maxCharacterSlots {
		return 0, false
	}
	var used [maxCharacterSlots]bool
	for _, c := range chars {
		if c != nil && c.Slot >= 0 && c.Slot < maxCharacterSlots {
			used[c.Slot] = true
		}
	}
	for i := range used {
		if !used[i] {
			return int32(i), true
		}
	}
	return 0, false
}

func createCharacterMessage(err error) string {
	switch {
	case errors.Is(err, domain.ErrNameEmpty):
		return "角色名不能为空"
	case errors.Is(err, domain.ErrNameTooLong):
		return "角色名过长"
	case errors.Is(err, domain.ErrNameBadChar):
		return "角色名含非法字符"
	case errors.Is(err, domain.ErrInvalidRace):
		return "职业参数无效"
	case errors.Is(err, domain.ErrInvalidGender):
		return "性别参数无效"
	case errors.Is(err, domain.ErrInvalidHead):
		return "发型参数无效"
	case errors.Is(err, domain.ErrInvalidHair):
		return "发色参数无效"
	case errors.Is(err, domain.ErrNameTaken):
		return "角色名已被占用"
	case errors.Is(err, domain.ErrSlotTaken):
		return "角色槽位已被占用"
	default:
		return "创建角色失败"
	}
}

func deleteCharacterMessage(err error) string {
	switch {
	case errors.Is(err, domain.ErrCharNotFound):
		return "角色不存在"
	default:
		return "删除角色失败"
	}
}

// onCreateChar 完成“已登录账号 → 建角落库 → 刷新选角页”整条边界。
// 0x8005 只告诉客户端成败，不包含角色数据；成功后必须紧跟新的 0x8004，
// 否则客户端虽然回到选角页，serverChars 仍是创建前的旧数组。
func (s *Session) onCreateChar(req protocol.Request) {
	s.mu.Lock()
	account, stage := s.account, s.stage
	s.mu.Unlock()
	if account == nil || stage != StageAuthed {
		s.sink.Send(protocol.CreateCharacterResult(1, "请先登录"))
		return
	}
	if s.deps.SpawnPos == nil {
		s.log.Error("建角失败: 未配置出生点")
		s.sink.Send(protocol.CreateCharacterResult(1, "服务器配置错误"))
		return
	}

	ctx := context.Background()
	chars, err := s.deps.Store.CharsByAccount(ctx, account.ID)
	if err != nil {
		s.log.Error("建角失败: 读取角色列表", "err", err)
		s.sink.Send(protocol.CreateCharacterResult(1, "读取角色失败"))
		return
	}
	slot, ok := firstFreeCharacterSlot(chars)
	if !ok {
		s.sink.Send(protocol.CreateCharacterResult(1, "角色数量已达上限"))
		return
	}

	race := domain.Race(req.Race)
	c, err := domain.NewCharacter(account.ID, slot, req.S1, race, domain.Appearance{
		Gender: req.Gender,
		Head:   req.Head,
		Hair:   req.Hair,
		// 原服建角/换装样本均为模式 1；0 仍保留给玩家后续主动设置。
		GlowMode: 1, GlowModeKnown: true,
	}, s.deps.SpawnPos(race))
	if err == nil {
		var bag *domain.Bag
		bag, err = domain.NewStartingBag(c.Appear.Gender, s.deps.Items)
		if err == nil {
			// 角色行、出生背包和空穿戴集是一个不可拆分的建角结果。
			// CreateChar 成功而 SaveSnapshot 失败时，PostgreSQL 会整体回滚。
			err = s.deps.Store.WithTx(ctx, func(tx store.Store) error {
				if err := tx.CreateChar(ctx, c); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, domain.Snapshot{
					Char: c, Bag: bag, Worn: domain.NewEquipSet(),
				})
			})
		}
	}
	if err != nil {
		s.log.Info("创建角色被拒", "acc", account.Username, "name", req.S1, "err", err)
		s.sink.Send(protocol.CreateCharacterResult(1, createCharacterMessage(err)))
		return
	}

	chars = append(chars, c)
	sort.Slice(chars, func(i, j int) bool { return chars[i].Slot < chars[j].Slot })
	s.sink.Send(protocol.CreateCharacterResult(0, "创建成功"))
	s.sink.Send(protocol.CharacterList(toBriefs(chars)))
	s.log.Info("创建角色成功", "acc", account.Username, "char", c.Name, "slot", c.Slot)
}

func (s *Session) onDeleteChar(req protocol.Request) {
	name := req.S1

	s.mu.Lock()
	account, stage := s.account, s.stage
	s.mu.Unlock()
	if account == nil || stage != StageAuthed {
		s.sink.Send(protocol.CreateCharacterResult(1, "请先登录"))
		return
	}

	ctx := context.Background()
	char, err := s.deps.Store.CharByName(ctx, name)
	if err != nil || char == nil {
		if err == nil {
			err = domain.ErrCharNotFound
		}
		s.log.Warn("删除角色失败: 角色不存在", "acc", account.Username, "name", name, "err", err)
		s.sink.Send(protocol.CreateCharacterResult(1, deleteCharacterMessage(err)))
		return
	}
	if char.AccountID != account.ID {
		s.log.Warn("删除角色失败: 非法归属", "acc", account.Username, "name", name)
		s.sink.Send(protocol.CreateCharacterResult(1, "无权限删除该角色"))
		return
	}

	if s.deps.Online != nil && s.deps.Online.Online(domain.CharID(char.ID)) {
		s.log.Warn("删除角色失败: 角色在线", "acc", account.Username, "name", name)
		s.sink.Send(protocol.CreateCharacterResult(1, "角色正在游戏中"))
		return
	}

	if err := s.deps.Store.DeleteChar(ctx, char.ID); err != nil {
		s.log.Warn("删除角色失败: 持久化失败", "acc", account.Username, "name", name, "err", err)
		s.sink.Send(protocol.CreateCharacterResult(1, deleteCharacterMessage(err)))
		return
	}

	s.sink.Send(protocol.CreateCharacterResult(0, "删除角色成功"))
	s.sendCharacterList()
	s.log.Info("删除角色成功", "acc", account.Username, "char", name)
}

// noteUnknown 记一个没接的上行包。
//
// req.Kind 非零表示 opcode 认得但**载荷解坏了** —— 那是我们的格式理解错了,
// 比"根本不认识这个号"严重得多, 所以分开报。
func (s *Session) noteUnknown(op uint16, payload []byte, kind protocol.ReqKind) {
	if kind != protocol.ReqUnknown {
		s.log.Warn("载荷解析失败", "kind", kind, "op", op, "长度", len(payload))
		return
	}
	if s.deps.Unknown == nil {
		return
	}
	if first := s.deps.Unknown.Note(protocol.Op(op), payload); first {
		// 只在第一次打, 否则客户端每秒发的那些号会把日志刷爆。未知包可能
		// 是错位的登录/聊天包，只记摘要，绝不把原始字段写进日志。
		s.log.Info("未接入的上行协议", "op", logHex(op),
			"载荷长度", len(payload),
			"样本摘要", protocol.UnknownSampleFingerprint(sample(payload)))
	}
}

func logHex(op uint16) string { return fmt.Sprintf("0x%04x", op) }

// onlineCount 取当前在线人数, 给选服界面用。
//
// 选服是**登录之前**就在问的, 那时还没有在线索引也很正常, 回 0 即可。
func (s *Session) onlineCount() uint16 {
	if s.deps.Online == nil {
		return 0
	}
	n := s.deps.Online.Count()
	if n > 0xffff {
		n = 0xffff
	}
	return uint16(n)
}

// entityID 取本会话的实体 id。没进游戏时是 0。
func (s *Session) entityID() domain.EntityID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entity
}

// postToScene 把一条命令投给本会话所在的场景。
//
// **没进游戏就直接丢掉** —— 客户端本来就可以不按顺序发包,
// 而没有实体的时候投进去只会让场景收到一条指向 0 号实体的命令。
func (s *Session) postToScene(c scene.Command) {
	s.mu.Lock()
	sc, inGame := s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame {
		s.log.Debug("没进游戏, 丢弃命令", "cmd", c.CmdName())
		return
	}
	s.deps.Router.Post(sc, c)
}

// sample 截取载荷前若干字节。32 字节够看出结构(几个 i32、有没有 u16 长度前缀),
// 又不至于把一个大包整行刷进日志。
func sample(p []byte) []byte {
	if len(p) > 32 {
		return p[:32]
	}
	return p
}

func (s *Session) OnClose(reason string) {
	s.leaveWorld(reason, StageClosed)
	s.mu.Lock()
	account := s.account
	s.mu.Unlock()
	if account != nil && s.deps.AccountSessions != nil {
		s.deps.AccountSessions.Release(account.ID, s)
	}
}

// leaveWorld 完成“会话解绑 → 全服索引清理 → 场景移除”的唯一离场路径。
// 0x1049 主动退世界与 TCP 断线必须共用它，否则两条路径迟早会漏掉其中一种清理。
//
// 状态与绑定先在同一把锁下摘掉：这既让重复 0x1049 / OnClose 幂等，也阻止同一
// 连接在清理途中继续把玩法命令投给旧实体。target 只能是 Authed（回选角页）或
// Closed（连接结束），由本包内两个固定调用点保证。
func (s *Session) leaveWorld(reason string, target Stage) bool {
	s.mu.Lock()
	prev, id, sc, ch, lease := s.stage, s.entity, s.scene, s.char, s.lease
	if prev != StageInGame {
		if target == StageClosed {
			s.stage = StageClosed
		}
		s.mu.Unlock()
		return false
	}
	s.stage = target
	s.awaitMapAck = false
	s.mapEpoch++
	s.entity = 0
	s.scene = domain.SceneID{}
	s.char = nil
	s.lease = nil
	s.seenTips = nil
	s.blocks = nil
	s.mu.Unlock()

	if ch != nil {
		// 下线和回选角不等于退队。实例是否仍存活由 Router 单独判定。
		// Active trades are bound to both entity leases. Cancel before removing the
		// online control so the peer's native DealDlg is closed deterministically.
		if s.deps.Trades != nil {
			if views, ok := s.deps.Trades.Cancel(domain.CharID(ch.ID), id); ok {
				s.endTradeViews(views, "对方已离开，交易取消")
			}
		}
		// 通知好友我下线了。**必须在摘索引之前** ——
		// 摘完就查不到自己了, 而 notifyFriends 要读我的好友列表(那个不受影响),
		// 但顺序反了会让"我自己"也可能被当成在线好友通知一遍
		s.notifyFriends(context.Background(), ch, false)
		// 摘索引**带上实体 id**: 掉线重连时新会话已经写进去了,
		// 不比对的话旧会话的 OnClose 会把新会话的条目删掉
		if s.deps.Online != nil {
			s.deps.Online.Leave(domain.CharID(ch.ID), id)
		}
		// 待处理的好友请求跟着人走: 我收到的清空, 我发出去的撤回。
		// 留着的话对方点了同意, 而我早就不在了
		if s.deps.Requests != nil {
			s.deps.Requests.Clear(domain.CharID(ch.ID))
			s.deps.Requests.DropFrom(domain.CharID(ch.ID))
		}
	}
	if s.deps.Router == nil {
		s.log.Error("离场失败: 未配置场景路由", "entity", id, "scene", sc, "reason", reason)
		// lease 已经交给场景实体；这里不能猜某次周期写回就是最终快照。
		// 没有显式最终 Save 时保持 gate 关闭，宁可拒绝重进也不读旧档覆盖。
		return true
	}
	leave := scene.Leave{ID: id, Reason: reason}
	if lease != nil {
		leave.FinalSaver = lease
	}
	if !s.deps.Router.PostLifecycle(sc, leave) {
		// 目标场景若已经停止，其 shutdown 已保存并清空全部实体；仍然必须明确记录，
		// 不能把关键生命周期投递失败伪装成成功。
		s.log.Warn("离场命令未投递: 场景已不存在或停止", "entity", id, "scene", sc,
			"reason", reason)
		// 最终 saver 随玩家实体保存在场景内；shutdown 会先用它排最终快照，
		// 再关闭连接。这里绝不把任意一次周期写回误认成最终提交。
	}
	return true
}

// leaveParty 把一个人从队伍里摘掉, 并把剩下每个人的队伍号刷新到他所在的场景。
//
// **解散时剩下那个人的号也要清。** 漏掉的话他会挂在一个已经不存在的队伍号上,
// 之后所有的"是不是队友"判定都会失败得莫名其妙 —— 而且那个号将来会被复用。
func (s *Session) leaveParty(who domain.CharID) {
	if s.deps.Party == nil {
		return
	}
	after, alive := s.deps.Party.Leave(who)
	// 显式退队时本人仍在场景里；先清本人，不能只刷新“剩下的人”。断线路径查不到
	// 在线地址时自然是 no-op。
	s.pushPartyID(who, 0)
	if !alive {
		// 队伍散了: 名册里已经清干净, 剩下那个人的场景侧也要清
		for _, m := range after.Members {
			s.pushPartyID(m.Char, 0)
		}
		return
	}
	for _, m := range after.Members {
		s.pushPartyID(m.Char, after.ID)
		s.pushPartyRefresh(m.Char)
	}
}

// pushPartyID 把某人的队伍号投到他所在的场景。
//
// 靠在线索引找地址 —— **不再只能刷新本连接自己**。
// 队伍是跨场景的, 队友可能在任何一张图上, 而每个会话只认识自己的
// (场景, 实体 id); 索引是唯一能回答"别人在哪"的地方。
//
// 查不到就是他下线了, 什么都不用做: 他重新登录时会带着新的队伍号进场景。
func (s *Session) pushPartyID(who domain.CharID, pid domain.PartyID) {
	if s.deps.Online == nil {
		return
	}
	loc, ok := s.deps.Online.Find(who)
	if !ok {
		return
	}
	s.deps.Router.Post(loc.Scene, scene.SetParty{ID: loc.Entity, Party: pid})
}

func (s *Session) pushPartyRefresh(who domain.CharID) {
	if s.deps.Online == nil || s.deps.Router == nil {
		return
	}
	if loc, ok := s.deps.Online.Find(who); ok {
		s.deps.Router.Post(loc.Scene, scene.RefreshParty{ID: loc.Entity})
	}
}

// partyPacket 只读全服内存索引，供场景事件出口同步构造 0x8033；这里绝不能查库。
func (s *Session) partyPacket() []byte {
	s.mu.Lock()
	ch := s.char
	s.mu.Unlock()
	if ch == nil || s.deps.Party == nil || s.deps.Online == nil {
		return protocol.EmptyPartySnapshot()
	}
	pid := s.deps.Party.PartyOf(domain.CharID(ch.ID))
	if pid == 0 {
		return protocol.EmptyPartySnapshot()
	}
	p, ok := s.deps.Party.Get(pid)
	if !ok {
		return protocol.EmptyPartySnapshot()
	}
	chars := make([]domain.CharID, 0, len(p.Members))
	for _, m := range p.Members {
		chars = append(chars, m.Char)
	}
	locs := s.deps.Online.Locate(chars)
	byChar := make(map[domain.CharID]online.Location, len(locs))
	for _, loc := range locs {
		byChar[loc.Char] = loc
	}
	view := protocol.PartyView{Name: p.Name}
	if loc, found := byChar[p.Leader]; found {
		view.Leader = loc.Entity
	}
	for _, m := range p.Members {
		loc, onlineNow := byChar[m.Char]
		view.Members = append(view.Members, protocol.PartyMemberView{
			Entity: loc.Entity, Char: m.Char, Name: m.Name,
			Level: loc.Profile.Level, HP: loc.Profile.HP, MaxHP: loc.Profile.MaxHP,
			Gender: loc.Profile.Gender, HairID: loc.Profile.HairID,
			HairColor: loc.Profile.HairColor, Online: onlineNow,
		})
	}
	if pkt := protocol.PartySnapshot(view); pkt != nil {
		return pkt
	}
	return protocol.EmptyPartySnapshot()
}

func (s *Session) partyPacketIfJoined() []byte {
	s.mu.Lock()
	ch := s.char
	s.mu.Unlock()
	if ch == nil || s.deps.Party == nil || s.deps.Party.PartyOf(domain.CharID(ch.ID)) == 0 {
		return nil
	}
	return s.partyPacket()
}

func (s *Session) updateOnlineStats(e event.StatsChanged) {
	if s.deps.Online == nil || len(e.Current) < 5 {
		return
	}
	s.mu.Lock()
	ch, entity := s.char, s.entity
	s.mu.Unlock()
	if ch == nil || entity != e.Who {
		return
	}
	loc, ok := s.deps.Online.Find(domain.CharID(ch.ID))
	if !ok {
		return
	}
	loc.Profile.Level, loc.Profile.HP, loc.Profile.MaxHP = e.Current[0], e.Current[3], e.Current[4]
	s.deps.Online.UpdateProfile(loc.Char, entity, loc.Profile)
}

func (s *Session) updateOnlineAppearance(a domain.Appearance) {
	if s.deps.Online == nil {
		return
	}
	s.mu.Lock()
	ch, entity := s.char, s.entity
	s.mu.Unlock()
	if ch == nil {
		return
	}
	loc, ok := s.deps.Online.Find(domain.CharID(ch.ID))
	if !ok {
		return
	}
	loc.Profile.Gender = a.Gender
	loc.Profile.HairID, loc.Profile.HairColor = int32(a.Head), int32(a.Hair)
	s.deps.Online.UpdateProfile(loc.Char, entity, loc.Profile)
}

func (s *Session) refreshPartyPeers() {
	s.mu.Lock()
	ch := s.char
	s.mu.Unlock()
	if ch == nil || s.deps.Party == nil {
		return
	}
	pid := s.deps.Party.PartyOf(domain.CharID(ch.ID))
	p, ok := s.deps.Party.Get(pid)
	if !ok {
		return
	}
	for _, m := range p.Members {
		if m.Char != domain.CharID(ch.ID) {
			s.pushPartyRefresh(m.Char)
		}
	}
}

// ── 业务处理 ──

func (s *Session) onLogin(req protocol.Request) {
	acc := req.S1
	ctx := context.Background()
	s.mu.Lock()
	stage := s.stage
	s.mu.Unlock()
	// 一条 TCP 只允许完成一次账号认证。换账号必须重连，否则同一个 Session 会在
	// 多个账号在线表项之间漂移，旧登录的迟到清理也会失去稳定的身份依据。
	if stage != StageConnected {
		s.log.Warn("当前阶段拒绝登录", "stage", stage, "acc", acc)
		return
	}

	account, err := s.deps.Store.AccountByName(ctx, acc)
	if err != nil {
		s.log.Error("查账号失败", "err", err)
		s.sink.Send(protocol.LoginResult(1, "服务器错误"))
		return
	}
	if account == nil {
		s.sink.Send(protocol.LoginResult(1, "账号不存在，请先注册"))
		return
	}
	if account.Banned || account.BannedUntil.After(time.Now()) {
		s.sink.Send(protocol.LoginResult(2, "账号已封禁"))
		return
	}
	ok, err := s.checkPassword(ctx, account, req.S2)
	if err != nil {
		s.log.Error("校验账号密码失败", "acc", acc, "err", err)
		s.sink.Send(protocol.LoginResult(1, "服务器错误"))
		return
	}
	if !ok {
		s.sink.Send(protocol.LoginResult(1, "账号或密码错误"))
		return
	}

	chars, err := s.deps.Store.CharsByAccount(ctx, account.ID)
	if err != nil {
		s.log.Error("查角色失败", "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色失败"))
		return
	}

	s.mu.Lock()
	if s.stage != StageConnected {
		s.mu.Unlock()
		return
	}
	s.account = account
	s.stage = StageAuthed
	s.mu.Unlock()

	// Claim 的替换与旧会话 OnClose 的 Release 都以 Session 指针做租约校验：
	// 新会话先成为唯一所有者，再断开旧连接；旧连接随后清理时不能摘掉新绑定。
	if s.deps.AccountSessions != nil {
		if replaced := s.deps.AccountSessions.Claim(account.ID, s); replaced != nil {
			s.log.Info("同账号新登录，顶替旧会话", "acc", acc,
				"oldSid", replaced.sink.SessionID(), "newSid", s.sink.SessionID())
			replaced.disconnectForDuplicateLogin(account.ID, s.sink.SessionID())
		}
	}
	// Publish the binding before rereading credentials: recovery/ban either finds
	// this session to revoke it, or its committed change is seen by this check.
	if _, ok := s.accountForSecurity(); !ok {
		s.revokeAuthentication("登录期间账号状态已变化")
		return
	}

	s.sink.Send(protocol.LoginResult(0, "欢迎回来,"+acc))
	s.sink.Send(protocol.CharacterList(toBriefs(chars)))
	// 0x8065 只种客户端安全码状态。现有资料未给出原服免输入窗口常量，
	// 因此明确使用 0ms，每次解锁都由服务端重新校验安全码。
	s.sink.Send(protocol.AccountSecuritySeed(account.SecHash != "", 0))
	s.log.Info("登录成功", "acc", acc, "chars", len(chars))
}

func (s *Session) disconnectForDuplicateLogin(accountID, newSessionID int64) {
	s.mu.Lock()
	account, stage := s.account, s.stage
	s.mu.Unlock()
	if account == nil || account.ID != accountID || stage == StageClosed {
		return
	}
	s.log.Info("账号已在新会话登录，断开当前会话", "acc", account.Username,
		"newSid", newSessionID, "stage", stage)
	s.revokeAuthentication("账号已在新会话登录")
}

// sendCharacterList 刷新当前已认证账号的选角页。
//
// 0x1049 只把会话从 InGame 退回 Authed，不断 TCP。客户端在发包
// 前已销毁旧世界 UI 并新建 TitleFlow，新实例不继承旧 serverChars，
// 因此服务端必须再发 0x8004。此时场景的最终快照可能还在写回；
// 列表仅用于选角展示，真正重进仍由 CharacterGate 拦到最终提交后，
// 并在拿到 gate 后重读权威角色/背包/装备数据。
func (s *Session) sendCharacterList() {
	s.mu.Lock()
	account, stage := s.account, s.stage
	s.mu.Unlock()
	if account == nil || stage != StageAuthed {
		s.log.Warn("拒绝刷新角色列表: 会话未处于已认证阶段", "stage", stage)
		return
	}

	chars, err := s.deps.Store.CharsByAccount(context.Background(), account.ID)
	if err != nil {
		s.log.Error("退回选角页后刷新角色列表失败", "acc", account.Username, "err", err)
		return
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i].Slot < chars[j].Slot })
	s.sink.Send(protocol.CharacterList(toBriefs(chars)))
	s.log.Debug("退回选角页已刷新角色列表", "acc", account.Username, "chars", len(chars))
}

// onEnter 选角进世界。
//
// 顺序: 校验归属 → 分配实体 id → 投 Enter 命令。
// 下发什么包**不在这里决定** —— 场景会发出 SelfEntered 等事件, 由 eventSink 编码。
// 会话层不再自己拼进世界的包, 那是把玩法逻辑写进连接管理里。
func (s *Session) onEnter(req protocol.Request) {
	name := req.S1
	ctx := context.Background()

	s.mu.Lock()
	account, stage := s.account, s.stage
	s.mu.Unlock()
	if account == nil || stage != StageAuthed {
		s.log.Warn("当前阶段拒绝进游戏", "stage", stage, "name", name)
		return
	}
	// A character-selection connection may outlive a database ban or password
	// recovery. Revalidate before loading any world state; final Stage check below
	// also cancels Enter if an account revocation races with character loading.
	if _, ok := s.accountForSecurity(); !ok {
		s.sink.Send(protocol.LoginResult(2, "账号状态已变化，请重新登录"))
		return
	}

	char, err := s.deps.Store.CharByName(ctx, name)
	if err != nil || char == nil || char.AccountID != account.ID {
		s.log.Warn("进游戏: 角色不属于本账号或不存在", "name", name)
		return
	}

	// 角色 gate 从读取 Bag/Worn **之前**持有，并贯穿整个在线期，直到离场最终快照
	// 真正提交成功。这样旧连接仍在线、主动退世界正在存档、旧 TCP 刚断开的三种
	// 窗口都只能明确拒绝新 Enter，不会拿旧库存起一个会覆盖新档的实体。
	var lease store.CharacterLease
	handedToScene := false
	if s.deps.CharGate != nil {
		var acquired bool
		lease, acquired = s.deps.CharGate.TryEnter(char.ID, account.ID)
		if !acquired {
			s.log.Warn("账号仍有角色在线或离场存档尚未提交, 拒绝进入",
				"char", char.Name, "id", char.ID)
			s.sink.Send(protocol.LoginResult(1, "账号角色正在保存，请稍后重试"))
			return
		}
		defer func() {
			if !handedToScene {
				lease.Release()
			}
		}()

		// 第一次 CharByName 只为得到稳定的角色 id 来抢 gate。它可能恰好与旧会话
		// 最终提交并发而读到旧角色行；拿到 gate 后必须重读，不能把那份旧 Char
		// 与随后新鲜的 Bag/Worn 拼成一个混合快照。
		char, err = s.deps.Store.CharByName(ctx, name)
		if err != nil || char == nil || char.AccountID != account.ID {
			s.log.Warn("取得角色 gate 后重读失败", "name", name, "err", err)
			s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
			return
		}
	}

	char.Touch()
	// 登录时间也是持久化边界：写入失败时不能继续发送入场成功快照。
	// 此时尚未向场景移交角色，返回会由上面的 defer 释放角色租约。
	if err := s.deps.Store.SaveChar(ctx, char); err != nil {
		s.log.Error("保存角色登录状态失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "保存角色登录状态失败，请稍后重试"))
		return
	}

	// 背包与角色行一起读。读不出来就不让他进 —— 顶着一个空背包进游戏,
	// 玩家会以为东西丢了, 然后往里放新东西, 那才是真的把旧的覆盖掉。
	bag, err := s.deps.Store.LoadBag(ctx, char.ID, int(char.BagSlots))
	if err != nil {
		s.log.Error("读背包失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}
	staleStall, err := s.deps.Store.LoadStall(ctx, char.ID)
	if err != nil {
		s.log.Error("读摆摊托管失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}

	worn, err := s.deps.Store.LoadEquips(ctx, char.ID)
	if err != nil {
		s.log.Error("读装备失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}
	changeSet, err := s.deps.Store.LoadChangeSet(ctx, char.ID)
	if err != nil {
		s.log.Error("读快速换装套装失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}
	var warehouse *domain.Warehouse
	if s.deps.WarehouseRule.Valid() {
		warehouse, err = s.deps.Store.LoadWarehouse(ctx, char.ID, s.deps.WarehouseRule)
		if err != nil {
			s.log.Error("读个人仓库失败, 拒绝进入", "char", char.Name, "err", err)
			s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
			return
		}
	}
	var wardrobe *domain.Wardrobe
	if s.deps.WardrobeRule.Valid() {
		wardrobe, err = s.deps.Store.LoadWardrobe(ctx, char.ID, s.deps.WardrobeRule)
		if err != nil {
			s.log.Error("读衣柜失败, 拒绝进入", "char", char.Name, "err", err)
			s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
			return
		}
	}

	if char.Skills, err = s.deps.Store.LoadSkills(ctx, char.ID); err != nil {
		s.log.Error("读技能失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}

	if char.Quests, err = s.deps.Store.LoadQuests(ctx, char.ID); err != nil {
		s.log.Error("读任务本失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}

	// 宠物读不出来同样拒绝进入。放进去再说"你没有宠物"的话,
	// 下一次存档就会把 char_pets 清空 —— 那是真的把宠物删了
	if char.Pets, err = s.deps.Store.LoadPets(ctx, char.ID); err != nil {
		s.log.Error("读宠物失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}
	if staleStall != nil {
		bag, err = s.recoverStaleStall(ctx, char, bag, worn, warehouse, wardrobe, staleStall)
		if err != nil {
			s.log.Error("恢复摆摊托管失败, 拒绝进入", "char", char.Name, "err", err)
			s.sink.Send(protocol.LoginResult(1, "恢复摆摊物品失败，请联系管理员"))
			return
		}
	}

	seenTips, err := s.deps.Store.LoadSeenTips(ctx, char.ID)
	if err != nil {
		s.log.Error("读新手提示已读状态失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}
	seenTipSet := make(map[int32]struct{}, len(seenTips))
	for _, tipID := range seenTips {
		seenTipSet[tipID] = struct{}{}
	}
	blockSet, err := s.loadBlockSet(ctx, char.ID)
	if err != nil {
		s.log.Error("读屏蔽名单失败, 拒绝进入", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "读取角色数据失败"))
		return
	}

	if s.deps.Router == nil {
		s.log.Error("进入场景失败: 未配置场景路由", "char", char.Name)
		s.sink.Send(protocol.LoginResult(1, "服务器配置错误"))
		return
	}
	var partyID domain.PartyID
	if s.deps.Party != nil {
		partyID = s.deps.Party.PartyOf(domain.CharID(char.ID))
	}
	sc, err := s.resolveLoginScene(ctx, char, partyID)
	if err != nil {
		s.log.Error("解析角色重登场景失败", "char", char.Name, "err", err)
		s.sink.Send(protocol.LoginResult(1, "恢复角色位置失败"))
		return
	}
	id := s.deps.Router.NewPlayerID()

	// 告诉在线的好友我上线了。**放在进场景之前**没有意义,
	// 但也不能太晚 —— 索引是在 SelfEntered 之后才写的, 所以真正的通知
	// 由 rebind 触发(见下面)。这里只把列表读一遍热身, 顺便暴露读失败。
	if list := s.friendsOf(ctx, char.ID); len(list) > 0 {
		s.log.Debug("好友列表就绪", "char", char.Name, "好友数", len(list))
	}

	enter := scene.Enter{
		ID: id, Char: char, Bag: bag, Worn: worn, ChangeSet: changeSet,
		Warehouse: warehouse, Wardrobe: wardrobe,
		Party: partyID, SeenTips: seenTips,
		Sink:       &eventSink{out: s.sink, log: s.log, sess: s, observer: id},
		FinalSaver: lease,
	}

	// 会话绑定与 Enter 入场命令必须形成同一个顺序点。若先解锁再 Post，TCP 恰好
	// 在两者之间断开时，OnClose 会把 Leave 排在 Enter 前面，留下无人持有的实体；
	// 持锁完成这次纯内存投递后，OnClose 只能观察到“未进入”或“Enter 已入队”。
	s.mu.Lock()
	if s.stage != StageAuthed || s.account != account {
		stage := s.stage
		s.mu.Unlock()
		s.log.Warn("角色数据加载期间会话状态已变化, 取消进入", "stage", stage,
			"char", char.Name)
		return
	}
	s.char, s.entity, s.scene, s.lease = char, id, sc, lease
	s.seenTips = seenTipSet
	s.blocks = blockSet
	s.stage = StageInGame
	ok := false
	if sc.Persistent() {
		ok = s.deps.Router.Post(sc, enter)
	} else {
		def := s.deps.Dungeons[sc.MapID]
		ok = s.deps.Router.PostExistingForOwner(sc, def.Owner(domain.CharID(char.ID), partyID), enter)
	}
	if !ok {
		s.stage = StageAuthed
		s.char, s.entity, s.scene, s.lease = nil, 0, domain.SceneID{}, nil
		s.seenTips = nil
		s.blocks = nil
	} else {
		handedToScene = true
	}
	s.mu.Unlock()
	if !ok {
		s.log.Error("进入场景失败", "char", char.Name, "scene", sc)
		s.sink.Close()
		return
	}
	s.log.Info("进入世界", "char", char.Name, "entity", id, "scene", sc)
}

// resolveLoginScene 只恢复仍归本角色/队伍所有的原实例。过期实例和旧版本保存的
// 副本公共场景统一落出口；这里不调 Router.Ensure，因此不会重置副本。
func (s *Session) resolveLoginScene(ctx context.Context, char *domain.Character,
	partyID domain.PartyID) (domain.SceneID, error) {
	if char == nil {
		return domain.SceneID{}, fmt.Errorf("角色为空")
	}
	def, dungeon := s.deps.Dungeons[char.Pos.MapID]
	if char.SceneInstance == 0 && !dungeon {
		return domain.SceneID{MapID: char.Pos.MapID}, nil
	}
	saved := domain.SceneID{MapID: char.Pos.MapID, Instance: char.SceneInstance}
	if dungeon && s.deps.Router.ActiveForOwner(saved, def.Owner(domain.CharID(char.ID), partyID)) {
		return saved, nil
	}
	if !dungeon {
		return domain.SceneID{}, fmt.Errorf("副本地图 %d 缺少出口配置", char.Pos.MapID)
	}
	char.Pos = def.Exit
	char.SceneInstance = 0
	if err := s.deps.Store.SaveChar(ctx, char); err != nil {
		return domain.SceneID{}, fmt.Errorf("清理过期副本存档: %w", err)
	}
	s.log.Info("原副本实例已不可恢复，改落出口", "char", char.Name,
		"scene", saved, "party", partyID, "exit", def.Exit)
	return domain.SceneID{MapID: def.Exit.MapID}, nil
}

func (s *Session) onMove(req protocol.Request) {
	s.mu.Lock()
	inGame, id, sc := s.stage == StageInGame, s.entity, s.scene
	if inGame && s.awaitMapAck {
		epoch := s.mapEpoch
		confirmed := req.Kind == protocol.ReqSavePos && req.Flag == 1 &&
			req.MapID > 0 && req.MapID == sc.MapID
		s.mu.Unlock()
		if confirmed {
			// Keep the barrier closed until the scene consumes this lifecycle command.
			if s.deps.Router == nil || !s.deps.Router.PostLifecycle(sc, scene.MapReady{ID: id, Scene: sc, Epoch: epoch}) {
				s.log.Error("地图确认无法投递到目标场景", "entity", id, "scene", sc, "epoch", epoch)
				s.sink.Close()
			}
		} else {
			s.log.Debug("等待地图就绪，忽略位置上报", "entity", id, "expected", sc.MapID, "map", req.MapID, "entered", req.Flag)
		}
		return
	}
	if inGame && req.Kind == protocol.ReqSavePos && req.Flag == 1 &&
		req.MapID > 0 && req.MapID == sc.MapID {
		epoch := s.mapEpoch
		s.mu.Unlock()
		// The client has rebuilt this scene even though its map ID did not change.
		// Coordinates are not a movement command at this lifecycle boundary.
		if s.deps.Router == nil || !s.deps.Router.PostLifecycle(sc, scene.ClientSceneReady{
			ID: id, Scene: sc, Epoch: epoch,
		}) {
			s.log.Error("同图重建确认无法投递到场景", "entity", id, "scene", sc, "epoch", epoch)
			s.sink.Close()
		}
		return
	}
	// 幻想小岛的村庄选择等旧客户端功能会先在本地切图，再用 0x100a 把新地图和
	// 落点报上来；它们不会先发 OpenTeleport/Teleport。切图开始时先报 entered=0，
	// 地图和 CombatWorld 真正建好后才报 entered=1。若在第一包就交接，下发的 NPC、
	// 怪物会被客户端随后完成的建图清空。因此只把 entered=1 当作客户端发起的传送，
	// 此时再让现有场景交接链路追上客户端并下发目标图实体。
	clientTeleport := inGame && req.Kind == protocol.ReqSavePos &&
		req.Flag == 1 && req.MapID > 0 && req.MapID != sc.MapID
	// 0x1017 不带图号(decMoveXY 留 0, 而没有任何一张图的 id 是 0), 0x100a 带。
	// 服务端已经发起跨图时，图号不符的仍是旧图滞后包；没有图号的移动也继续关在闸外。
	drop := inGame && req.MapID != 0 && req.MapID != sc.MapID
	s.mu.Unlock()
	if !inGame {
		return
	}
	if clientTeleport {
		s.deps.Router.Post(sc, scene.Teleport{
			ID:             id,
			To:             domain.SceneID{MapID: req.MapID},
			At:             domain.Pos{MapID: req.MapID, X: float64(req.X), Y: float64(req.Y)},
			ClientMapReady: true,
		})
		return
	}
	if drop {
		// 这条一定要留: 客户端画面在动而服务端不认位置, 是本项目最难查的故障形态。
		s.log.Debug("丢弃跨图后的滞后位置上报", "id", id, "包里的图", req.MapID,
			"当前图", sc.MapID, "x", req.X, "y", req.Y)
		return
	}
	// 记录移动请求，便于在调试日志中查看坐标。
	s.log.Debug("收到移动", "id", id, "x", req.X, "y", req.Y)
	// 合法性校验(速度上限、可通行)在场景里做 —— 会话手上没有地图, 也没有上一帧的位置。
	s.deps.Router.Post(sc, scene.MoveTo{
		ID:         id,
		To:         domain.Pos{MapID: sc.MapID, X: float64(req.X), Y: float64(req.Y)},
		ReceivedAt: time.Now(),
	})
}

// rebind 记下"我现在在哪张图、是哪个实体"。
//
// 由 eventSink 在收到首次入场 SelfEntered 或跨图 Teleported 时调用。
// 不更新的话, 传送之后的移动命令还会投给原来那个场景, 玩家会卡在原地不动,
// 而且没有任何报错。
func (s *Session) rebind(id domain.EntityID, sc domain.SceneID) {
	s.mu.Lock()
	if s.stage != StageInGame {
		stage := s.stage
		s.mu.Unlock()
		s.log.Warn("忽略离场后的场景改绑", "stage", stage, "entity", id, "scene", sc)
		return
	}
	from := s.scene
	changed := s.entity != id || s.scene != sc
	// 只有真的换了图才拉闸: 同图改绑(初次入场、实体号变化)没有滞后上报问题,
	// 拉闸反而会把正常移动挡在外面。
	if from.MapID != sc.MapID {
		s.mapEpoch++
		s.awaitMapAck = true
	}
	s.entity, s.scene = id, sc
	ch := s.char
	s.mu.Unlock()
	if changed {
		s.log.Info("会话改绑场景", "entity", id, "从", from, "到", sc)
	}

	// 在线索引每次都幂等写入。初次进图时 onEnter 已经预先把 entity/scene
	// 放进会话，因此“地址没变”并不等于“在线索引已有记录”；以前在上面提前
	// return，导致所有首次登录者实际上从未在线。Teleported 落地也走这条路 —— 不更新的话,
	// 私聊会投给一个已经没有这个实体的场景, 消息静静消失
	if s.deps.Online != nil && ch != nil {
		profile := online.Profile{Level: ch.Level, Gender: ch.Appear.Gender,
			HairID: int32(ch.Appear.Head), HairColor: int32(ch.Appear.Hair)}
		if old, ok := s.deps.Online.Find(domain.CharID(ch.ID)); ok {
			profile = old.Profile
		}
		_, had := s.deps.Online.Enter(online.Location{
			Char: domain.CharID(ch.ID), Account: ch.AccountID, Name: ch.Name,
			Entity: id, Scene: sc, Profile: profile})
		s.deps.Online.AttachControl(domain.CharID(ch.ID), id, gmSessionControl{sink: s.sink, sess: s})
		// 只在**第一次**进索引时通知好友。传送落地也会走到这里,
		// 不判 had 的话每换一张图好友就收到一次"他上线了"
		if !had {
			s.notifyFriends(context.Background(), ch, true)
		}
	}
}

// transferScene 串行化跨场景 Enter 入队与断线清理。
// handoff 只能做内存投递，不能做 IO；调用时持有会话锁是为了让 OnClose 无法卡进
// “源实体已删、目标 Enter 已排队、会话/在线索引仍指向源场景”的危险窗口。
func (s *Session) transferScene(id domain.EntityID, from, to domain.SceneID,
	handoff func() bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage != StageInGame || s.entity != id || s.scene != from {
		s.log.Warn("传送交接时会话绑定已变化", "stage", s.stage, "entity", s.entity,
			"scene", s.scene, "wantEntity", id, "from", from, "to", to)
		return false
	}
	if !handoff() {
		return false
	}
	s.scene = to
	s.mapEpoch++
	s.awaitMapAck = true
	// 在目标场景的 Teleported 事件 flush 之前，私聊、队伍刷新等路由已经
	// 可能查 Online；因此它必须与会话改绑处在同一个锁内顺序点。Online.Enter
	// 只拿 Registry 自己的内存锁且不回调 Session，锁序固定为 Session.mu → Online.mu。
	// 这里只是换地址，不通知好友“上线”；随后的 rebind 会幂等覆写，had=true
	// 也不会重复通知。交接失败时上面已返回，会话与索引都保持 from。
	if s.deps.Online != nil && s.char != nil {
		profile := online.Profile{Level: s.char.Level, Gender: s.char.Appear.Gender,
			HairID: int32(s.char.Appear.Head), HairColor: int32(s.char.Appear.Hair)}
		if old, ok := s.deps.Online.Find(domain.CharID(s.char.ID)); ok {
			profile = old.Profile
		}
		s.deps.Online.Enter(online.Location{
			Char: domain.CharID(s.char.ID), Account: s.char.AccountID, Name: s.char.Name,
			Entity: id, Scene: to, Profile: profile})
	}
	return true
}

// toBriefs 把领域角色转成协议角色简报。
func toBriefs(chars []*domain.Character) []protocol.CharBrief {
	out := make([]protocol.CharBrief, 0, len(chars))
	for _, c := range chars {
		lastLogin := ""
		if !c.LastLogin.IsZero() {
			lastLogin = c.LastLogin.Format("2006-01-02 15:04")
		}
		out = append(out, protocol.CharBrief{
			Name: c.Name, Race: c.ClientRace(), Gender: c.Appear.Gender,
			// 客户端 head 是发型、hair 是发色，顺序由协议固定。
			Head: c.Appear.Head, Hair: c.Appear.Hair,
			Level: c.Level, MapID: c.Pos.MapID,
			X: float32(c.Pos.X), Y: float32(c.Pos.Y),
			LastLogin: lastLogin,
			Appear:    protocol.ApViewOf(c),
			Attrs:     c.Attrs,
			Honor:     c.Honor,
		})
	}
	return out
}
