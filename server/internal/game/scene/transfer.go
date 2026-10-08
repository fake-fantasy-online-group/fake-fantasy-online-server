package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// transferSink 是会话接缝可选实现的原子场景交接能力。生产 eventSink 实现它，
// 测试假 Sink 不实现时仍走普通投递；接口留在 scene 包，避免 event 层认识路由。
type transferSink interface {
	TransferScene(id domain.EntityID, from, to domain.SceneID, handoff func() bool) bool
}

// 跨场景传送 —— Actor 模型里最容易写错的一段。
//
// 铁律: **实体的所有权随消息转移, 同一时刻只有一个场景持有它。**
//
// 所以交接必须是这个顺序, 一步都不能调:
//
//	1. 广播消失（这时它还在 AOI 里, 广播范围才算得对）
//	2. 立刻冲刷（下一步就要把它摘掉, 留到帧末就发不出去了）
//	3. 从本场景**彻底摘除**（entities / players / AOI / 交战状态）
//	4. 改坐标, 把 Character 交给目标场景
//
// 第 3 步做完之后, **本场景再也不许碰这个 *Character**。
// 它已经属于另一个 goroutine 了 —— 这里少删一个 map, 就是一处数据竞争。

// onTeleport 把玩家送去别的场景。返回值表示这条命令是否已经消费：
// true 既包括成功落地，也包括摘除后交接失败并保存/断线；调用方看到 true 后
// 绝不能继续触碰原实体。只有玩家不存在或尚未改状态就发现无路由时返回 false。
func (s *Scene) onTeleport(cmd Teleport) bool {
	if s.deferTradeTeleport(cmd) {
		return true
	}
	e, ok := s.players[cmd.ID]
	if !ok {
		return false
	}
	if loading := s.mapLoads[cmd.ID]; loading != nil {
		queued := cmd
		loading.next = &queued
		s.log.Info("客户端仍在加载，保留最新传送目标", "entity", cmd.ID, "to", cmd.To, "epoch", loading.epoch)
		return true
	}
	// 所有切图路径在此按数据库识别副本，包括 NPC 传送、普通门、GM 和
	// 客户端先切图后上报位置。任何入口都不能把副本送入 Instance=0。
	if def, dungeon := s.dungeons[cmd.To.MapID]; dungeon && cmd.To.Persistent() {
		if e.Player == nil || e.Player.Char == nil || s.router == nil {
			return false
		}
		owner := def.Owner(domain.CharID(e.Player.Char.ID), e.Player.Party)
		reject := func(reason event.RejectReason) bool {
			s.emitTo(e.ID, event.Rejected{Who: e.ID, Cmd: "EnterDungeon", Reason: reason})
			// 客户端已经本地切图时，拒绝后必须送回服务端原场景，恢复双方一致。
			if cmd.ClientMapReady {
				return s.onTeleport(Teleport{ID: e.ID, To: s.id, At: e.Pos})
			}
			return false
		}
		if !owner.Valid() {
			return reject(event.RejectPartyRequired)
		}
		if !e.Alive() {
			return reject(event.RejectInvalid)
		}
		if s.dungeon != nil && def.ID == s.dungeon.ID && !s.id.Persistent() {
			cmd.To.Instance = s.id.Instance
			if err := s.router.EnsureDungeonFloor(cmd.To, owner); err != nil {
				s.log.Error("副本跨层失败", "to", cmd.To, "err", err)
				return reject(event.RejectUnknown)
			}
		} else {
			// 活动配置中的 0,0 是未提供通用入口的哨兵，不能当出生点开放。
			if def.Enter.X <= 0 || def.Enter.Y <= 0 {
				return reject(event.RejectNoTarget)
			}
			inst, err := s.openPlayerDungeon(e, def)
			if err != nil {
				s.trialNotice(e, err.Error())
				s.log.Warn("副本入口创建实例失败", "map", cmd.To.MapID, "err", err)
				return reject(event.RejectInstanceFull)
			}
			cmd.To = inst
			def = s.dungeons[inst.MapID]
			if cmd.At != def.Enter {
				cmd.At = def.Enter
				cmd.ClientMapReady = false
			}
		}
	}
	// 传送会关闭客户端 NPC 对话；商店上下文不能跟着旧 NPC/旧坐标继续使用。
	if e.Player != nil {
		e.Player.ShopID = 0
		s.closeStallForExit(e)
	}
	// 同场景只在服务端挪位置；客户端收到 Teleported 仍会重建，因此也要等待确认。
	if cmd.To == s.id {
		// 服务端瞬移是一次战斗不连续点：本帧前半段排进去、尚未结算的动作
		// 不能拿旧位置完成准入后，又在新坐标跨远距离命中。出战宠物也属于
		// 同一份战斗上下文，必须一起解除。
		s.interruptWork(e)
		combatants := []domain.EntityID{e.ID}
		if e.Player != nil && e.Player.Pet != 0 {
			combatants = append(combatants, e.Player.Pet)
		}
		s.cancelPendingCombat(combatants...)
		// WorkStopped 是按产生时的源 AOI 格广播；若等 force move 后才在帧末
		// flush，本人跨格后反而不在旧格视野里，会漏掉收工事件。传送本来就是
		// 生命周期边界，先把此前事件按旧坐标冲刷完，再下发权威落点。
		s.flush()
		if !s.beginMapLoad(e, cmd.ClientMapReady) {
			return true
		}
		if s.mapLoads[e.ID] != nil {
			s.emitExcept(event.EntityDespawned{ID: e.ID, Kind: e.Kind, Reason: event.DespawnTimeout}, e.ID)
			s.flush()
		}
		// 只有落点真的处于门区才上锁；落在普通位置不能吞掉下一次正常入门。
		s.blockPortalLanding(e.ID, cmd.At)
		// 本人也必须收到权威落点；普通 EntityMoved 刻意排除了本人，只改服务端
		// 会让同图门后的客户端仍停在旧坐标。0x803a 对同图同样是精确定义的传送事件。
		s.emitTo(e.ID, event.Teleported{Who: e.ID, Scene: s.id, To: cmd.At})
		// 不再次检查传送门；否则落点仍在门区时会同步递归。force=true 让眩晕
		// 只限制主动移动，不阻止服务端传送。
		s.move(MoveTo{ID: cmd.ID, To: cmd.At, Dir: cmd.Dir, Stop: true}, false, true)
		return true
	}
	if s.router == nil {
		s.log.Error("场景没接路由, 无法传送", "id", cmd.ID)
		return false
	}
	// Character.Pos 在线期间不随每次移动更新，源场景的权威位置是实体位置。
	// 交接失败时玩家只能断线重登，届时必须从这个源坐标恢复，不能把尚未成功
	// 到达的目标落点写进存档。
	sourcePos := e.Pos
	s.endSharedRide(e)

	// 0. 切图必然打断打工。踩门路径已由 onMove 打断，但副本进入、限时踢出和
	// 死亡回城会直达这里；统一放在交接入口，才不会静默丢掉 Work。
	s.interruptWork(e)
	combatants := []domain.EntityID{e.ID}
	if e.Player != nil && e.Player.Pet != 0 {
		combatants = append(combatants, e.Player.Pet)
	}
	s.cancelPendingCombat(combatants...)

	runtime := e.CapturePlayerRuntime(s.tick)
	// 0.5 宠物跟着主人做无损运行态交接。源场景只摘除旧实体，
	// 不走 recallPet，因为召回会清当前技能、结束骑乘并改变客户端宠物栏。
	runtime.Pet = s.detachPetForTransfer(e)
	s.removeOwnerTraps(e.ID)
	// 传送命令携带的是目标落地方向；不能让运行态快照里的旧朝向把它覆盖掉。
	runtime.Dir = cmd.Dir

	// 1~2. 让周围的人看到他走了
	s.emitExcept(event.EntityDespawned{ID: e.ID, Kind: e.Kind, Reason: event.DespawnTimeout}, e.ID)
	s.flush()

	// 3. 彻底摘除。漏掉任何一个 map 都会留下悬空引用
	s.aoi.Leave(e)
	delete(s.entities, e.ID)
	delete(s.players, e.ID)
	delete(s.hits, e.ID)
	delete(s.portalBlocked, e.ID)
	// 正在打他的怪要脱战, 否则会追着一个已经不在这张图的 id
	for _, m := range s.monsters {
		if m.Monster != nil && m.Monster.Target == e.ID {
			s.dropTarget(m)
		}
	}

	// 4. 改坐标后交出去。**从这一行往后, 本场景不再拥有 char**
	char := e.Player.Char
	char.Pos = cmd.At
	char.Pos.MapID = cmd.To.MapID
	char.SceneInstance = cmd.To.Instance

	bag := e.Player.Bag   // 背包跟着人走, 不能留在原场景
	worn := e.Player.Worn // 身上穿的同理
	changeSet := e.Player.ChangeSet
	warehouse := e.Player.Warehouse
	wardrobe := e.Player.Wardrobe
	stall := e.Player.Stall
	sink := e.Player.Sink
	finalSaver := e.Player.FinalSaver
	name := e.Name
	partyID := e.Player.Party
	id := e.ID // 实体 id 跟着人走 —— 是同一个实体换了地方, 不是新生成一个

	handoff := func() bool {
		return s.router.PostHandoff(cmd.To, Enter{
			ID: id, Char: char, Runtime: &runtime, Bag: bag, Worn: worn, Warehouse: warehouse,
			ChangeSet: changeSet,
			Wardrobe:  wardrobe,
			Stall:     stall,
			Party:     partyID, DungeonElapsed: s.transferDungeonElapsed(cmd.To),
			Sink: sink, ClientMapReady: cmd.ClientMapReady,
			FinalSaver: finalSaver,
		})
	}
	handedOff := false
	if owner, ok := sink.(transferSink); ok {
		handedOff = owner.TransferScene(id, s.id, cmd.To, handoff)
	} else {
		handedOff = handoff()
	}
	if !handedOff {
		// 目标场景建不出来/满了。人已经从这边摘掉了, 不能装作没事 ——
		// 断开连接让他重登, 好过留在一个谁都不拥有的悬空状态。
		s.log.Error("传送失败, 目标场景不可用", "char", name, "to", cmd.To)
		char.Pos = sourcePos
		char.SceneInstance = s.id.Instance
		if s.dungeon != nil {
			char.Pos = s.dungeon.Exit
			char.SceneInstance = 0
		}
		if finalSaver != nil {
			finalSaver.Save(domain.Snapshot{Char: char, Bag: bag, Worn: worn, Warehouse: warehouse,
				ChangeSet: changeSet, Wardrobe: wardrobe, Stall: stall}.Clone())
		} else if s.saver != nil {
			s.saver.Save(domain.Snapshot{Char: char, Bag: bag, Worn: worn, Warehouse: warehouse,
				ChangeSet: changeSet, Wardrobe: wardrobe, Stall: stall}.Clone())
		}
		sink.Close()
		s.closeDungeonIfEmpty()
		return true
	}
	s.log.Info("传送", "char", name, "从", s.id, "到", cmd.To, "落点", cmd.At)
	s.closeDungeonIfEmpty()
	return true
}

// checkPortals 看玩家有没有踩到传送门。移动之后调。
//
// 返回 true 表示已经把他送走了, 调用方不要再碰这个实体。
func (s *Scene) checkPortals(e *domain.Pos, id domain.EntityID) bool {
	if len(s.portals) == 0 {
		delete(s.portalBlocked, id)
		return false
	}
	portal := s.portalAt(*e)
	if _, blocked := s.portalBlocked[id]; blocked {
		if portal < 0 {
			// 已完全离开所有门区。从下一次重新进入开始，门才恢复触发。
			delete(s.portalBlocked, id)
		}
		return false
	}
	if portal < 0 || s.router == nil {
		return false
	}
	p := &s.portals[portal]
	if s.dungeon != nil {
		if target, ok := s.dungeons[p.To.MapID]; ok && target.ID == s.dungeon.ID {
			if s.enterDungeonFloor(id, p.To, uint8(p.ToDir)) {
				return true
			}
			s.portalBlocked[id] = struct{}{}
			return false
		}
	}
	if p.Dungeon {
		if s.onEnterDungeon(EnterDungeon{ID: id, MapID: p.To.MapID}) {
			return true
		}
		// 拒绝后仍让角色走到门区，但在完全离开之前不重复弹同一条提示。
		s.portalBlocked[id] = struct{}{}
		return false
	}
	dst := domain.SceneID{MapID: p.To.MapID}
	if !s.onTeleport(Teleport{ID: id, To: dst, At: p.To, Dir: uint8(p.ToDir)}) {
		s.log.Warn("传送门触发未执行, 玩家留在原场景", "id", id,
			"proc", p.ProcID, "to", dst)
		return false
	}
	return true
}

// portalAt 返回包含坐标的第一扇门。配置顺序就是重叠门区的确定性优先级。
func (s *Scene) portalAt(pos domain.Pos) int {
	for i := range s.portals {
		if s.portals[i].Hit(pos) {
			return i
		}
	}
	return -1
}

// blockPortalLanding 只在服务端权威落点位于门区时抑制门触发。抑制不是
// “跳过下一包”，而是一直持续到玩家完全离开所有门区，避免小步移动反复弹图。
func (s *Scene) blockPortalLanding(id domain.EntityID, pos domain.Pos) {
	if s.portalAt(pos) >= 0 {
		s.portalBlocked[id] = struct{}{}
		return
	}
	delete(s.portalBlocked, id)
}

// cancelPendingCombat 取消一次服务端瞬移两侧尚未结算的战斗关系。普通攻击整条
// 删除；技能若只是某个 AOE 目标被传走，则保留同一技能的其他目标。
func (s *Scene) cancelPendingCombat(ids ...domain.EntityID) {
	if len(ids) == 0 {
		return
	}
	cancelled := make(map[domain.EntityID]struct{}, len(ids))
	for _, id := range ids {
		if id != 0 {
			cancelled[id] = struct{}{}
		}
	}
	if len(cancelled) == 0 {
		return
	}

	attacks := s.attacks[:0]
	for _, attack := range s.attacks {
		_, srcGone := cancelled[attack.src]
		_, dstGone := cancelled[attack.dst]
		if !srcGone && !dstGone {
			attacks = append(attacks, attack)
		}
	}
	for i := len(attacks); i < len(s.attacks); i++ {
		s.attacks[i] = pendingAttack{}
	}
	s.attacks = attacks

	casts := s.casts[:0]
	for _, cast := range s.casts {
		if _, srcGone := cancelled[cast.src]; srcGone {
			continue
		}
		targets := cast.targets[:0]
		for _, target := range cast.targets {
			if _, gone := cancelled[target]; !gone {
				targets = append(targets, target)
			}
		}
		for i := len(targets); i < len(cast.targets); i++ {
			cast.targets[i] = 0
		}
		if len(targets) == 0 && len(cast.targets) > 0 {
			continue
		}
		cast.targets = targets
		casts = append(casts, cast)
	}
	for i := len(casts); i < len(s.casts); i++ {
		s.casts[i] = pendingCast{}
	}
	s.casts = casts

	for attacker, tracker := range s.hits {
		if _, gone := cancelled[attacker]; gone {
			delete(s.hits, attacker)
			continue
		}
		if tracker != nil {
			if _, gone := cancelled[tracker.Target()]; gone {
				tracker.Reset()
			}
		}
	}
	for _, monster := range s.monsters {
		if monster.Monster == nil {
			continue
		}
		if _, gone := cancelled[monster.Monster.Target]; gone {
			s.dropTarget(monster)
		}
	}
}

// Router 回填。由 Router 在建完场景后调用 ——
// 场景要靠它把人送去别的图, 也要靠它的分配器造掉落物实体。
func (s *Scene) attachRouter(r *Router) {
	s.router = r
	if s.alloc == nil { // Config 里显式给过就不覆盖
		s.alloc = r.Alloc()
	}
}
