package session

import (
	"log/slog"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// eventSink 是游戏层与客户端之间的翻译器: 收游戏事件, 编成字节, 交给连接发出。
//
// 它是**依赖倒置的那个接头**。场景只知道 event.Sink 这个接口, 完全不知道下面有
// opcode、有加密、有 TCP。要验证这一点很简单: game 包里 grep 不到任何 0x8xxx。
//
// 只被场景 goroutine 调用, 因此内部无需加锁; 底层 Send 必须是非阻塞的
// (网关连接内部有发送队列+背压), 否则一个慢客户端就能卡住整张图。
type eventSink struct {
	out Sink
	log *slog.Logger
	// observer 是这条连接在场景里的实体号。部分下行展示是接收者相关的：
	// 例如同一件掉落对击杀者 owned=1，对路人 owned=0。
	observer domain.EntityID
	// sess 是回指。**只用于跟随场景变化** —— 玩家被传送之后, 会话得知道
	// 自己现在在哪张图, 否则后续的移动/攻击会投给原来那个场景, 石沉大海。
	sess *Session

	// 1.5.8 的背包、仓库和宠物增量包都以客户端已经收到一份完整快照为前提。
	// 缓存放在每连接的接缝层：场景继续只产出权威完整视图，协议差分不会渗进
	// 购买、拾取、换装等每一条业务路径。
	inventory *event.InventorySnapshot
	warehouse *event.WarehouseSnapshot
	pet       *event.PetSnapshot
}

func (s *eventSink) Emit(ev event.Event) {
	if _,ok := ev.(event.EndgameMessage); ok && (s.sess == nil || !s.sess.endgameEnabled()) { return }
	var pkts [][]byte
	var refreshParty bool
	var entered bool
	followTipIDs := inferredNewbieTips(s.observer, ev)
	var persistTipID int32
	var persistTipCharID int64
	if e, ok := ev.(event.NewbieTipRequested); ok {
		// 先确认这个 ID 在服务端文案表里能编码，再占用一次性状态；否则一个
		// 尚未接入的编号会被错误地永久标成已读。
		pkts = protocol.Encode(s.observer, e)
		if len(pkts) == 0 {
			s.log.Debug("忽略无文案的新手提示", "tip", e.ID)
			return
		}
		if s.sess != nil {
			charID, claimed := s.sess.claimServerTip(e.ID)
			if !claimed {
				return
			}
			persistTipID, persistTipCharID = e.ID, charID
		}
	}
	// SelfEntered 只表示首次登录入场：接缝层在改绑初始场景后构造登录批次。
	// 跨图落地由独立的 Teleported 生命周期处理，不能再发一份 EnterWorld 快照。
	if e, ok := ev.(event.SelfEntered); ok {
		entered = true
		// 同一连接返回选角后可以进入另一角色；新角色必须重新以完整快照打底。
		s.inventory, s.warehouse, s.pet = nil, nil, nil
		if s.sess != nil {
			s.sess.rebind(e.ID, e.Scene)
		}
		s.observer = e.ID
		partyPkt := protocol.EmptyPartySnapshot()
		if s.sess != nil {
			partyPkt = s.sess.partyPacket()
		}
		pkts = [][]byte{partyPkt, protocol.EnterWorld(e.ID, e.Char)}
		if s.sess != nil {
			if packet := protocol.Announcements(s.sess.deps.Announcements, false); packet != nil {
				pkts = append(pkts, packet)
			}
		}
	}
	if e, ok := ev.(event.StatsChanged); ok && s.sess != nil {
		s.sess.updateOnlineStats(e)
		refreshParty = true
	}
	if e, ok := ev.(event.AppearanceChanged); ok && s.sess != nil && e.Who == s.observer {
		s.sess.updateOnlineAppearance(e.Appearance)
		refreshParty = true
	}
	if e, ok := ev.(event.HairChanged); ok && s.sess != nil && e.Who == s.observer {
		s.sess.updateOnlineAppearance(e.Appearance)
		refreshParty = true
	}
	if _, ok := ev.(event.PartyChanged); ok && s.sess != nil {
		pkts = [][]byte{s.sess.partyPacket()}
	}
	// Teleported 必须先更新服务端路由与观察者身份，再按普通事件编码 0x803a；
	// 否则紧随落地包而来的移动/攻击仍可能投向旧场景。
	if e, ok := ev.(event.Teleported); ok {
		if s.sess != nil {
			s.sess.rebind(e.Who, e.Scene)
		}
		s.observer = e.Who
	}
	// 拒绝回执单独打, **必须带原因**。
	//
	// 日志记录拒绝原因，便于区分距离、目标、存活状态和冷却时间等条件。
	if r, isReject := ev.(event.Rejected); isReject {
		s.log.Debug("请求被拒", "cmd", r.Cmd, "原因", r.Reason)
		// 所有业务拒绝继续交给协议层显示系统文本。StartWork 仍由协议层选择
		// CancelWork 模式，普通拒绝使用聊天区系统提示，不能再静默丢弃。
	}
	stateEncoded := false
	switch e := ev.(type) {
	case event.InventorySnapshot:
		pkts, stateEncoded = s.inventoryPackets(e), true
	case event.WarehouseSnapshot:
		pkts, stateEncoded = s.warehousePackets(e), true
	case event.PetSnapshot:
		pkts, stateEncoded = s.petPackets(e), true
	}
	if pkts == nil && !stateEncoded {
		pkts = protocol.Encode(s.observer, ev)
	}
	if refreshParty && s.sess != nil {
		if pkt := s.sess.partyPacketIfJoined(); pkt != nil {
			pkts = append(pkts, pkt)
			s.sess.refreshPartyPeers()
		}
	}
	if len(pkts) == 0 {
		// 不产生下行包是正常的(尚未取得可靠映射或纯内部记账), 但记一笔 ——
		// 静默丢弃会让"客户端没反应"变成无从下手的问题。
		s.log.Debug("事件无对应下行包", "event", eventName(ev))
		for _, tipID := range followTipIDs {
			s.Emit(event.NewbieTipRequested{Who: s.observer, ID: tipID})
		}
		return
	}
	for _, pkt := range pkts {
		// 调试日志记录发出的操作码和包长度。
		s.log.Debug("下行", "op", logHex(uint16(pkt[0])|uint16(pkt[1])<<8),
			"字节", len(pkt), "event", eventName(ev))
		s.out.Send(pkt)
	}
	if entered && s.sess != nil {
		// 好友快照要查 PostgreSQL，不能阻塞场景 goroutine。EnterWorld 已经先进入
		// 同一连接的 FIFO 发送队列，因此异步刷新不会越过进世界主包。
		go func() {
			s.sess.refreshFriendList()
			s.sess.refreshBlockList()
			s.sess.refreshSystemRequests()
			s.sess.refreshMailIndicator()
			s.sess.refreshFamily()
			s.sess.refreshApprentice()
		}()
	}
	if persistTipID != 0 && s.sess != nil {
		s.sess.persistServerTip(persistTipCharID, persistTipID)
	}
	for _, tipID := range followTipIDs {
		s.Emit(event.NewbieTipRequested{Who: s.observer, ID: tipID})
	}
}

func (s *eventSink) Close() { s.out.Close() }

// TransferScene 把“目标 Enter 已入队”与“会话改绑到目标场景”做成同一个
// Session 锁内顺序点。若断线发生在传送交接中，OnClose 只能看见源场景且目标
// 尚未接收，或看见目标场景且 Enter 已经排在 Leave 前；不会产生目标图幽灵实体。
func (s *eventSink) TransferScene(id domain.EntityID, from, to domain.SceneID,
	handoff func() bool) bool {
	if s.sess == nil {
		return handoff()
	}
	return s.sess.transferScene(id, from, to, handoff)
}

// eventName 取事件的类型名, 只用于日志。
func eventName(ev event.Event) string {
	switch ev.(type) {
	case event.SelfEntered:
		return "SelfEntered"
	case event.ExpGained:
		return "ExpGained"
	case event.LevelUp:
		return "LevelUp"
	case event.StatsChanged:
		return "StatsChanged"
	case event.RackCatalogSnapshot:
		return "RackCatalogSnapshot"
	case event.FittingCatalogSnapshot:
		return "FittingCatalogSnapshot"
	case event.FittingDetailsSnapshot:
		return "FittingDetailsSnapshot"
	case event.RackRefundSnapshot:
		return "RackRefundSnapshot"
	case event.FamilyStashSnapshot:
		return "FamilyStashSnapshot"
	case event.AppearanceChanged:
		return "AppearanceChanged"
	case event.HairChanged:
		return "HairChanged"
	case event.Rejected:
		return "Rejected"
	case event.EntityDied:
		return "EntityDied"
	case event.PlayerRevived:
		return "PlayerRevived"
	case event.TargetStatusSnapshot:
		return "TargetStatusSnapshot"
	case event.NpcTasks:
		return "NpcTasks"
	case event.NpcChatter:
		return "NpcChatter"
	case event.NpcDialogText:
		return "NpcDialogText"
	case event.QuestAccepted:
		return "QuestAccepted"
	case event.QuestItemObtained:
		return "QuestItemObtained"
	case event.QuestItemBlocked:
		return "QuestItemBlocked"
	case event.QuestCompleted:
		return "QuestCompleted"
	case event.EntitySpawned:
		return "EntitySpawned"
	case event.EntityMoved:
		return "EntityMoved"
	case event.RidingChanged:
		return "RidingChanged"
	case event.EntityDespawned:
		return "EntityDespawned"
	case event.DamageDealt:
		return "DamageDealt"
	case event.SkillCastChanged:
		return "SkillCastChanged"
	case event.SkillHitEffect:
		return "SkillHitEffect"
	case event.InventorySnapshot:
		return "InventorySnapshot"
	case event.ChangeSetSnapshot:
		return "ChangeSetSnapshot"
	case event.WarehouseSnapshot:
		return "WarehouseSnapshot"
	case event.WardrobeSnapshot:
		return "WardrobeSnapshot"
	case event.StallContents:
		return "StallContents"
	case event.StallOwnerChanged:
		return "StallOwnerChanged"
	case event.StallClosed:
		return "StallClosed"
	case event.SkillSnapshot:
		return "SkillSnapshot"
	case event.LifeSkillSnapshot:
		return "LifeSkillSnapshot"
	case event.NewbieTipSeed:
		return "NewbieTipSeed"
	case event.NewbieTipRequested:
		return "NewbieTipRequested"
	case event.HotbarSnapshot:
		return "HotbarSnapshot"
	case event.BuffSnapshot:
		return "BuffSnapshot"
	case event.StatusApplied:
		return "StatusApplied"
	case event.EntityStatusSnapshot:
		return "EntityStatusSnapshot"
	case event.EntityStatusEffect:
		return "EntityStatusEffect"
	case event.PetSnapshot:
		return "PetSnapshot"
	case event.Teleported:
		return "Teleported"
	case event.ActiveQuests:
		return "ActiveQuests"
	case event.TitleSnapshot:
		return "TitleSnapshot"
	case event.WorkStarted:
		return "WorkStarted"
	case event.WorkStopped:
		return "WorkStopped"
	case event.WorkSettled:
		return "WorkSettled"
	}
	return "?"
}

// inferredNewbieTips 只从已经完成的游戏事实推导提示，不从请求或 UI 猜测。
// 因为同一事件会广播给旁观者，所有本人提示都必须再核对 observer。
func inferredNewbieTips(observer domain.EntityID, ev event.Event) []int32 {
	switch e := ev.(type) {
	case event.LevelUp:
		if e.Who != observer {
			return nil
		}
		ids := []int32{3}
		if e.Level >= 10 {
			ids = append(ids, 33)
		}
		if e.Level >= 11 {
			ids = append(ids, 42)
		}
		// 1.3.4 没有这些 ID 的可达固定 Push 调用；这里按同版本
		// ov_petgrow 的权威 capture_level 阶梯补齐服务端通知。每个门槛都
		// 对应文案中的捕捉工具与至少一只仍存在于客户端表里的宠物。
		if e.Level >= 20 {
			ids = append(ids, 64) // 老君壶：山狼/大蜗牛怪等
		}
		if e.Level >= 30 {
			ids = append(ids, 43) // 玉净瓶：1.3.4 表中为雪女/风精灵
		}
		if e.Level >= 35 {
			ids = append(ids, 65) // 捆仙索：独角兽/苍狼皇
		}
		if e.Level >= 50 {
			ids = append(ids, 44) // 捆仙索：火鸟/大闸蟹/毒蜘蛛
		}
		return ids
	case event.InventorySnapshot:
		if e.Who == observer && hasStartingEquipment(e.Items) {
			return []int32{30}
		}
	case event.QuestCompleted:
		if e.Who == observer {
			return []int32{19}
		}
	case event.WorkStarted:
		if e.Who == observer {
			return []int32{35}
		}
	case event.PetCaptured:
		if e.Who == observer {
			return []int32{45}
		}
	case event.PetExpGained:
		if e.Who == observer && e.Ups > 0 {
			return []int32{50}
		}
	case event.DamageDealt:
		if e.Dst == observer && e.Amount > 0 {
			return []int32{47}
		}
	}
	return nil
}

// hasStartingEquipment 在下行背包已经完整通过场景校验之后核对出生套装。
// 两件都仍在背包才提示玩家去穿；只靠一级或建角成功会把缺件旧存档也说成
// “已经为你准备好”，而把已穿上的装备也算进去又会给出过时操作提示。
func hasStartingEquipment(items []event.InventoryItemView) bool {
	var body, sword bool
	for _, item := range items {
		if item.Count <= 0 {
			continue
		}
		body = body || domain.IsStartingBody(item.Item)
		sword = sword || domain.IsStartingSword(item.Item)
	}
	return body && sword
}
