package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 副本实例的生命周期。
//
// 这是 `00-不可逆决策.md` 第一条("场景 Actor")和第四条("副本是实例")真正被用到的地方:
// **同一张地图可以同时存在多个场景**，各自独占自己的实体、各自跑自己的帧。
// 在此之前 `SceneID.Instance` 一直是 0，`Router.NewInstance` 是死代码。
//
// 一个实例的一生：
//
//	开     玩家找 NPC 进副本 → Router 找一个没满的实例，没有就新开（受上限约束）
//	跑     和常驻图完全一样 —— 刷怪、AI、战斗、掉落都不用为副本写第二套
//	限时   到点把里面的人送回出口，然后自己停
//	清空   真正进过人后，实际人数归零就立即停；只有首次 Enter 交接前保留短宽限

// onEnterDungeon 把玩家送进副本。
func (s *Scene) onEnterDungeon(cmd EnterDungeon) bool {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return false
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "EnterDungeon", Reason: r})
	}
	if s.router == nil || s.dungeons == nil {
		reject(event.RejectUnknown)
		return false
	}
	def, ok := s.dungeons[cmd.MapID]
	if !ok || def.Enter.X <= 0 || def.Enter.Y <= 0 {
		reject(event.RejectNoTarget)
		return false
	}
	// 只认当前地图里真实存在的入口 NPC，不校验玩家与 NPC 的距离。
	if cmd.NPC != "" && s.npcOnMap(cmd.NPC) == "" {
		reject(event.RejectNoTarget)
		return false
	}
	if p.Player == nil || (!def.Solo && p.Player.Party == 0) {
		reject(event.RejectPartyRequired)
		return false
	}

	inst, err := s.openPlayerDungeon(p, def)
	if err != nil {
		s.trialNotice(p, err.Error())
		s.log.Warn("开副本实例失败", "副本", def.Name, "err", err)
		reject(event.RejectInstanceFull)
		return false
	}
	// 已在场景 goroutine 内，直接消费不可丢的生命周期；再投本场景普通 mailbox
	// 会在并发填满时静默丢掉传送，却仍误记一条成功日志。
	def = s.dungeons[inst.MapID]
	id, name := p.ID, p.Name
	if !s.onTeleport(Teleport{ID: id, To: inst, At: def.Enter}) {
		s.emitTo(id, event.Rejected{Who: id, Cmd: "EnterDungeon", Reason: event.RejectUnknown})
		s.log.Error("进副本传送未执行", "char", name, "副本", def.Name, "实例", inst.Instance)
		return false
	}
	return true
}

// canStayInDungeon 统一判定停留资格：单人副本按角色持有，其余要求组队。
// 退队、断线、重登都只复用这一个事实，不各自发明一套分支。
func (s *Scene) canStayInDungeon(p *entity.Entity) bool {
	return p != nil && p.Player != nil &&
		((s.dungeon != nil && s.dungeon.Solo) || p.Player.Party != 0)
}

func (s *Scene) updateDungeonEligibility(p *entity.Entity) {
	if s.dungeon == nil || p == nil || p.Player == nil {
		return
	}
	if s.canStayInDungeon(p) {
		p.Player.ClearDungeonEviction()
		return
	}
	p.Player.StartDungeonEviction(s.tick + domain.Ticks(domain.DungeonPartyGraceSec*1000))
}

// checkDungeonEligibility 只清退到期且仍无组队资格的在线角色。
func (s *Scene) checkDungeonEligibility() {
	if s.dungeon == nil {
		return
	}
	var expired []domain.EntityID
	for id, p := range s.players {
		if !s.canStayInDungeon(p) && p.Player.DungeonEvictionDue(s.tick) {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		s.log.Info("副本组队资格已超过宽限，清退", "副本", s.dungeon.Name,
			"实例", s.id.Instance, "id", id)
		s.onTeleport(Teleport{
			ID: id, To: domain.SceneID{MapID: s.dungeon.Exit.MapID}, At: s.dungeon.Exit,
		})
	}
}

// persistedPosition 统一决定存档中的场景位置。在副本中仍有组队资格时
// 保留实例号；已失去资格而下线/小退时直接落副本出口。
func (s *Scene) persistedPosition(p *entity.Entity) (domain.Pos, uint32) {
	if s.dungeon == nil {
		return p.Pos, 0
	}
	if s.canStayInDungeon(p) {
		return p.Pos, s.id.Instance
	}
	return s.dungeon.Exit, 0
}

// closeDungeonIfEmpty 是副本销毁的唯一规则：曾经进过人，现在实际人数为零。
func (s *Scene) closeDungeonIfEmpty() bool {
	if s.dungeon == nil || s.id.Persistent() || !s.everOccupied || len(s.players) != 0 {
		return false
	}
	if s.multiFloorDungeon() {
		// 其它楼层仍可能有人，且返回时必须保留本层怪物/掉落状态。多层实例统一
		// 保留到首层开始的总限时，不能按单层人数立即销毁。
		return false
	}
	select {
	case <-s.quit:
		return true
	default:
	}
	s.log.Info("副本无人，立即关闭", "副本", s.dungeon.Name, "实例", s.id.Instance)
	s.Stop()
	return true
}

// checkInstanceLife 每帧检查这个实例该不该收摊。
//
// 只对副本实例生效 —— 常驻大陆图永远不会因为没人就关掉，
// 它们要一直在那儿等人来。
func (s *Scene) checkInstanceLife() {
	if s.dungeon == nil || s.id.Persistent() {
		return
	}
	if s.closeDungeonIfEmpty() {
		return
	}
	// 限时到了: 先把人送回出口, 再停。
	//
	// 顺序不能反 —— 先停的话 shutdown 会直接断掉他们的连接,
	// 玩家看到的是"打副本打到一半掉线", 而不是"时间到了被请出来"。
	if t := s.dungeon.Timeout(); t > 0 && s.dungeonElapsed() >= t {
		s.log.Info("副本限时到", "副本", s.dungeon.Name, "实例", s.id.Instance,
			"在场", len(s.players))
		s.evictAll(event.DespawnTimeout)
		s.Stop()
		return
	}
	if s.multiFloorDungeon() {
		return
	}

	if len(s.players) > 0 {
		s.emptySince = 0
		return
	}
	if s.emptySince == 0 {
		s.emptySince = s.tick
		return
	}
	if s.tick-s.emptySince >= domain.Ticks(domain.EmptyInstanceGraceSec*1000) {
		s.log.Info("副本已空, 关闭", "副本", s.dungeon.Name, "实例", s.id.Instance)
		s.Stop()
	}
}

func (s *Scene) dungeonElapsed() domain.Tick {
	if s == nil || s.dungeon == nil {
		return 0
	}
	return s.dungeonElapsedBase + s.tick
}

// 只有同一副本实例跨层才继承总计时，进入另一个副本从零开始。
func (s *Scene) transferDungeonElapsed(to domain.SceneID) domain.Tick {
	if s.dungeon != nil && !to.Persistent() && to.Instance == s.id.Instance {
		if target, ok := s.dungeons[to.MapID]; ok && target.ID == s.dungeon.ID {
			return s.dungeonElapsed()
		}
	}
	return 0
}

func (s *Scene) multiFloorDungeon() bool {
	if s == nil || s.dungeon == nil {
		return false
	}
	n := 0
	for _, def := range s.dungeons {
		if def.ID == s.dungeon.ID {
			n++
			if n > 1 {
				return true
			}
		}
	}
	return false
}

func (s *Scene) dungeonFloorCleared() bool {
	if s == nil || s.dungeon == nil {
		return false
	}
	for _, monster := range s.monsters {
		if monster != nil && monster.Monster != nil && monster.Alive() && monster.Monster.Kind.Hostile() {
			return false
		}
	}
	return true
}

// enterDungeonFloor 让已有副本实例进入同组目标楼层。普通 link 门和 NPC
// 古代石碑共用这里，避免两条入口对 PartyID/Instance 的处理逐渐分叉。
func (s *Scene) enterDungeonFloor(id domain.EntityID, target domain.Pos, dir uint8) bool {
	if s == nil || s.dungeon == nil || s.router == nil {
		return false
	}
	def, ok := s.dungeons[target.MapID]
	if !ok || def.ID != s.dungeon.ID {
		return false
	}
	player := s.players[id]
	if player == nil || !s.canStayInDungeon(player) {
		s.emitTo(id, event.Rejected{Who: id, Cmd: "DungeonFloor", Reason: event.RejectPartyRequired})
		return false
	}
	dst := domain.SceneID{MapID: target.MapID, Instance: s.id.Instance}
	if err := s.router.EnsureDungeonFloor(dst, def.Owner(domain.CharID(player.Player.Char.ID), player.Player.Party)); err != nil {
		s.log.Error("副本跨层目标未就绪", "from", s.id, "to", dst,
			"party", player.Player.Party, "err", err)
		s.emitTo(id, event.Rejected{Who: id, Cmd: "DungeonFloor", Reason: event.RejectUnknown})
		return false
	}
	return s.onTeleport(Teleport{ID: id, To: dst, At: target, Dir: dir})
}

func (s *Scene) advanceDungeonEncounter(dead *entity.Entity) {
	if s == nil || s.dungeon == nil || dead == nil || dead.Monster == nil || s.alloc == nil || s.defs == nil {
		return
	}
	trigger := dead.Monster.TypeID
	key := domain.DungeonEncounterKey{MapID: s.id.MapID, Trigger: trigger}
	nextID, ok := s.dungeonEncounters[key]
	if !ok || s.encounterDone[trigger] {
		return
	}
	def, ok := s.defs.Def(nextID)
	if !ok {
		s.log.Error("副本下一阶段怪物定义不存在", "scene", s.id, "trigger", trigger, "next", nextID)
		return
	}
	s.encounterDone[trigger] = true
	next := spawn.Encounter(s.alloc.Monster(), dead.Pos, dead.Dir, def)
	s.entities[next.ID] = next
	s.monsters[next.ID] = next
	s.aoi.Enter(next)
	s.emit(s.spawnEvent(next))
	s.log.Info("副本首领进入下一阶段", "scene", s.id, "trigger", dead.Name,
		"next", next.Name, "entity", next.ID, "at", next.Pos)
}

// evictAll 把实例里的人全部送回出口。
//
// 用 Teleport 而不是直接断连接: 那是一次正常的场景交接,
// 玩家会落在出口图上, 背包装备一样不少。
func (s *Scene) evictAll(_ event.DespawnReason) {
	if s.dungeon == nil || s.router == nil {
		return
	}
	ids := make([]domain.EntityID, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	exit := domain.SceneID{MapID: s.dungeon.Exit.MapID}
	for _, id := range ids {
		s.onTeleport(Teleport{ID: id, To: exit, At: s.dungeon.Exit})
	}
}

// All entry paths share this gate. Solo dungeons retain character ownership.
func (s *Scene) openPlayerDungeon(p *entity.Entity, def domain.DungeonDef) (domain.SceneID, error) {
	ch := p.Player.Char
	owner := def.Owner(domain.CharID(ch.ID), p.Player.Party)
	canCreate := def.Solo || s.dungeonParty == nil
	roster := domain.DungeonParty{}
	if !def.Solo && s.dungeonParty != nil {
		roster = s.dungeonParty(domain.CharID(ch.ID))
		if roster.ID == 0 || roster.ID != p.Player.Party {
			return domain.SceneID{}, fmt.Errorf("请先创建或加入队伍")
		}
		canCreate = roster.Leader == domain.CharID(ch.ID)
	}
	level := int32(0)
	var group map[int32][]domain.TrialSpawn
	if s.trial.IsMap(def.Enter.MapID) {
		q, ok := ch.Quests[s.trial.Quest]
		if !ok || q.State != domain.QuestActive {
			return domain.SceneID{}, fmt.Errorf("请先领取尚未完成的试炼任务")
		}
		level = roster.MaxLevel
		if level < ch.Level {
			level = ch.Level
		}
		if level < s.trial.MinLevel || level > s.trial.MaxLevel {
			return domain.SceneID{}, fmt.Errorf("队伍等级不在试炼范围内")
		}
		group = s.trial.Maps
		ids := s.trialMapIDs()
		def = s.dungeons[ids[s.rng.Intn(len(ids))]]
	}
	id, err := s.router.openDungeonAccess(def, owner, canCreate, group, level)
	if err == nil && level > 0 {
		ch.Trial.MonsterLevel = s.router.trialLevelOf(id)
		p.Player.MarkDirty()
	}
	return id, err
}
