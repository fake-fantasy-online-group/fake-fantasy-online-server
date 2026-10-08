package scene

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Router 是"场景 id → 那个场景的 mailbox"的目录。
//
// 它是**唯一**允许持有全部场景引用的东西。别的地方想让某场景做事, 一律
// router.Post(id, cmd) —— 拿不到 *Scene 就不可能绕过 mailbox 直接碰实体,
// 这条约束是靠"只有 Router 有那张表"来保证的, 不是靠自觉。
type Router struct {
	mu               sync.RWMutex
	scenes           map[domain.SceneID]*Scene
	instanceOwners   map[domain.SceneID]domain.DungeonOwner
	trialLevels      map[domain.SceneID]int32
	trialCompleted   map[domain.SceneID]bool
	instanceAdmitted map[domain.SceneID]bool
	// instanceMu 串行化“复用现有实例或创建新实例”的完整决策，避免两个队伍
	// 同时越过数量上限，或同一队伍在并发进入时各开一张图。
	instanceMu sync.Mutex
	// draining 是关服封门位。它与 scenes 共用同一把锁，保证
	// Drain 开始抓取场景列表后，不会再有 Ensure 向 WaitGroup Add。
	draining bool

	build Builder
	alloc *domain.EntityAlloc
	log   *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	nextInst atomic.Uint32
}

// Builder 按场景 id 造一个场景。由上层注入 —— Router 不该认识地图数据、存档、配置表。
type Builder func(domain.SceneID) (*Scene, error)

// NewRouter 建路由。build 不能为 nil。
func NewRouter(build Builder, log *slog.Logger) *Router {
	if log == nil {
		log = slog.Default()
	}
	return &Router{
		scenes:         map[domain.SceneID]*Scene{},
		instanceOwners: map[domain.SceneID]domain.DungeonOwner{},
		trialLevels:    map[domain.SceneID]int32{}, trialCompleted: map[domain.SceneID]bool{}, instanceAdmitted: map[domain.SceneID]bool{},
		build: build,
		alloc: domain.NewEntityAlloc(),
		log:   log,
	}
}

// Start 记下生命周期上下文。必须在任何 Ensure/Post 之前调 ——
// 场景 goroutine 要靠这个 ctx 才知道什么时候该退出。
func (r *Router) Start(ctx context.Context) {
	r.ctx, r.cancel = context.WithCancel(ctx)
}

// NewPlayerID 分配一个玩家实体 id。会话层在投 Enter 之前调它, 这样它立刻就知道
// 自己的实体 id, 不用等场景回调。
func (r *Router) NewPlayerID() domain.EntityID { return r.alloc.Player() }

// Alloc 暴露实体 id 分配器给刷怪、掉落等需要造实体的地方。
func (r *Router) Alloc() *domain.EntityAlloc { return r.alloc }

// Ensure 取场景, 不存在就建并启动。
//
// **懒加载**: 371 张图不会真开 371 个 goroutine 常驻, 只有有人进去才起。
// 这是省资源, 不是省架构 —— 场景模型本身不变。
func (r *Router) Ensure(id domain.SceneID) (*Scene, error) {
	r.mu.RLock()
	if r.draining {
		r.mu.RUnlock()
		return nil, fmt.Errorf("scene: Router 正在关服")
	}
	s, ok := r.scenes[id]
	r.mu.RUnlock()
	if ok {
		return s, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.draining {
		return nil, fmt.Errorf("scene: Router 正在关服")
	}
	if s, ok := r.scenes[id]; ok { // 双检: 拿写锁的路上别人可能已经建好了
		return s, nil
	}
	if r.ctx == nil {
		return nil, fmt.Errorf("scene: Router.Start 还没调用")
	}
	s, err := r.build(id)
	if err != nil {
		return nil, fmt.Errorf("scene: 建场景 %v 失败: %w", id, err)
	}
	// 回填路由: 场景要靠它把人送去别的图。Router 拥有场景, 所以由它来接线,
	// 而不是让每个 Builder 自己记得传。
	s.attachRouter(r)
	r.scenes[id] = s
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		s.Run(r.ctx)
		// 场景自己停了(panic 兜底失败, 或被 Stop): 从表里摘掉, 下次进入会重建。
		r.mu.Lock()
		if cur := r.scenes[id]; cur == s {
			delete(r.scenes, id)
			delete(r.instanceOwners, id)
			delete(r.trialLevels, id)
			delete(r.trialCompleted, id)
			delete(r.instanceAdmitted, id)
		}
		r.mu.Unlock()
	}()
	return s, nil
}

// Post 把命令投给某个场景。场景不存在就先建。
func (r *Router) Post(id domain.SceneID, c Command) bool {
	s, err := r.Ensure(id)
	if err != nil {
		r.log.Error("投递失败", "scene", id, "cmd", c.CmdName(), "err", err)
		return false
	}
	return s.Post(c)
}

// PostHandoff 把跨场景 Enter 投到目标的独立所有权队列。目标允许懒创建；
// 入队本身不等待目标帧，也不受普通玩法 mailbox 容量影响。
func (r *Router) PostHandoff(id domain.SceneID, c Enter) bool {
	s, err := r.Ensure(id)
	if err != nil {
		r.log.Error("交接投递失败", "scene", id, "cmd", c.CmdName(), "err", err)
		return false
	}
	if !s.PostHandoff(c) {
		r.log.Warn("交接投递时目标场景已停止", "scene", id, "cmd", c.CmdName())
		return false
	}
	return true
}

// PostLifecycle 把不能丢的生命周期/持久化同步命令投给一个已经存在的场景。
//
// 与 Post 不同，这里绝不为 Leave/RenamePlayer 懒创建场景：若旧场景已经退出，其中的实体也已由
// shutdown 清空；为了删除一个不存在的实体重建空场景既无意义，还会掩盖地址过期。
// 运行中的场景队列即使暂满也会等待下一帧腾出空间，不能把断线清理静默丢掉。
func (r *Router) PostLifecycle(id domain.SceneID, c Command) bool {
	r.mu.RLock()
	s, ok := r.scenes[id]
	r.mu.RUnlock()
	if !ok {
		r.log.Warn("生命周期投递目标已不存在", "scene", id, "cmd", c.CmdName())
		return false
	}
	if !s.PostLifecycle(c) {
		r.log.Warn("生命周期投递时场景已停止", "scene", id, "cmd", c.CmdName())
		return false
	}
	return true
}

// NewInstance 为一张图开一个新的副本实例号。
//
// 实例号从 1 开始**只增不减** —— 复用实例号会让"上一个副本的延迟消息投进了新副本"
// 这种 bug 变得可能, 而那种 bug 极难复现。
func (r *Router) NewInstance(mapID int32) domain.SceneID {
	return domain.SceneID{MapID: mapID, Instance: r.nextInst.Add(1)}
}

// OpenInstance 给一支队伍挑一个能进的实例, 没有就新开。
//
// 只复用同一 PartyID 已经拥有的实例；不同队伍即使进入同一副本地图，也必须
// 分处不同实例，不能共享怪物、掉落和限时状态。
// 开新的受 MaxInstances 约束: 不封顶的话一张图能被开出无数个实例把内存吃光。
//
// **这个方法不在任何场景的 goroutine 里**。instanceMu 串行整个决策，
// Router.mu 只保护场景与所有权目录；这里不碰任何实体。
func (r *Router) OpenInstance(def domain.DungeonDef, party domain.PartyID) (domain.SceneID, error) {
	return r.OpenDungeonInstance(def, def.Owner(0, party))
}

// OpenDungeonInstance 按数据库配置的队伍或角色所有权开设独立实例。
func (r *Router) OpenDungeonInstance(def domain.DungeonDef, owner domain.DungeonOwner) (domain.SceneID, error) {
	return r.openDungeonAccess(def, owner, true, nil, 0)
}
func (r *Router) openDungeonAccess(def domain.DungeonDef, owner domain.DungeonOwner, canCreate bool, group map[int32][]domain.TrialSpawn, level int32) (domain.SceneID, error) {
	if !owner.Valid() || def.Solo != (owner.Character > 0) {
		return domain.SceneID{}, fmt.Errorf("scene: 副本 %s 的所有权与单人/组队模式不一致", def.Name)
	}
	r.instanceMu.Lock()
	defer r.instanceMu.Unlock()

	r.mu.RLock()
	var existing []*Scene
	for id, s := range r.scenes {
		if (id.MapID == def.Enter.MapID || len(group[id.MapID]) > 0) && !id.Persistent() {
			if r.instanceOwners[id] == owner {
				existing = append(existing, s)
			}
		}
	}
	allForMap := 0
	for id := range r.scenes {
		if id.MapID == def.Enter.MapID && !id.Persistent() {
			allForMap++
		}
	}
	r.mu.RUnlock()

	// 复用: 挑一个还开着的实例。
	//
	// 不按人数挑最空的：人数是每帧抄出来的，慢一帧。同一队伍只会有一张
	// 活跃实例，挑第一张即可。
	for _, s := range existing {
		select {
		case <-s.quit:
			continue // 正在关的实例不要往里塞人
		default:
		}
		r.mu.RLock()
		done := r.trialCompleted[s.ID()]
		admitted := r.instanceAdmitted[s.ID()]
		r.mu.RUnlock()
		if !canCreate && !admitted {
			return domain.SceneID{}, fmt.Errorf("队长正在进入副本，请稍候")
		}
		if done {
			return domain.SceneID{}, fmt.Errorf("本次试炼已结束，请队员全部离开后再进入")
		}
		return s.ID(), nil
	}

	if !canCreate {
		return domain.SceneID{}, fmt.Errorf("请由队长先进入创建副本")
	}
	if int32(allForMap) >= def.InstanceCap() {
		return domain.SceneID{}, fmt.Errorf(
			"scene: 副本 %s 的实例已达上限 %d", def.Name, def.InstanceCap())
	}
	id := r.NewInstance(def.Enter.MapID)
	s, err := r.Ensure(id)
	if err != nil {
		return domain.SceneID{}, err
	}
	r.mu.Lock()
	if r.scenes[id] != s {
		r.mu.Unlock()
		return domain.SceneID{}, fmt.Errorf("scene: 新副本 %v 在绑定队伍前已停止", id)
	}
	r.instanceOwners[id] = owner
	if level > 0 {
		r.trialLevels[id] = level
	}
	r.mu.Unlock()
	return id, nil
}

// EnsureDungeonFloor 为同一副本实例准备另一层场景。实例号全服单调唯一；只要已有
// 任一楼层归该队伍所有，就能把同一个 Instance 扩展到同副本的目标地图。
func (r *Router) EnsureDungeonFloor(id domain.SceneID, owner domain.DungeonOwner) error {
	if id.Persistent() || !owner.Valid() {
		return fmt.Errorf("scene: 多层副本目标或队伍非法 scene=%v owner=%+v", id, owner)
	}
	r.instanceMu.Lock()
	defer r.instanceMu.Unlock()

	r.mu.RLock()
	if existing, ok := r.scenes[id]; ok {
		owned := r.instanceOwners[id] == owner
		select {
		case <-existing.quit:
			owned = false
		default:
		}
		r.mu.RUnlock()
		if !owned {
			return fmt.Errorf("scene: 副本楼层 %v 已归其它队伍或正在关闭", id)
		}
		return nil
	}
	authorized := false
	for sceneID, existingOwner := range r.instanceOwners {
		if sceneID.Instance == id.Instance && existingOwner == owner {
			authorized = true
			break
		}
	}
	r.mu.RUnlock()
	if !authorized {
		return fmt.Errorf("scene: 所有者 %+v 不拥有副本实例 %d", owner, id.Instance)
	}

	s, err := r.Ensure(id)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scenes[id] != s {
		return fmt.Errorf("scene: 副本楼层 %v 在绑定队伍前已停止", id)
	}
	r.instanceOwners[id] = owner
	return nil
}

// Count 返回当前活着的场景数。
func (r *Router) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.scenes)
}

// ActiveForOwner 报告实例仍活着且归指定角色或队伍所有。重登恢复必须检查这条，
// 仅凭实例号活着会让玩家换队后回到旧队伍的副本。
func (r *Router) ActiveForOwner(id domain.SceneID, owner domain.DungeonOwner) bool {
	if id.Persistent() || !owner.Valid() {
		return false
	}
	r.mu.RLock()
	s, ok := r.scenes[id]
	owned := ok && r.instanceOwners[id] == owner
	r.mu.RUnlock()
	if !owned {
		return false
	}
	select {
	case <-s.quit:
		return false
	default:
		return true
	}
}

// PostExistingForOwner 在同一把路由读锁内复核实例所有权并投递重登入场。
// 这样 ActiveForOwner 与真正 Post 之间即使场景退出或目录变化，也不会把角色
// 交给错误队伍的实例。
func (r *Router) PostExistingForOwner(id domain.SceneID, owner domain.DungeonOwner, c Command) bool {
	if id.Persistent() || !owner.Valid() {
		return false
	}
	r.mu.RLock()
	s, ok := r.scenes[id]
	if !ok || r.instanceOwners[id] != owner {
		r.mu.RUnlock()
		return false
	}
	select {
	case <-s.quit:
		r.mu.RUnlock()
		return false
	default:
	}
	ok = s.Post(c)
	r.mu.RUnlock()
	return ok
}

// Drain 停掉所有场景并等它们存完档退出。
//
// 关服顺序里的第二步(第一步是网关停止接客, 第三步是刷写回队列)。
// 顺序反了就会丢档: 场景还在写, 队列已经关了。
// 返回 nil 才表示所有场景都已交出最终快照；超时必须向上传播，调用方不能继续
// 把本次停服报告成成功。
func (r *Router) Drain(timeout time.Duration) error {
	// 先在写锁内封门再抓快照。否则 Ensure 可以在下面 wg.Wait
	// 开始后创建新场景：它既不在 all 里，又与 WaitGroup 并发 Add。
	r.mu.Lock()
	r.draining = true
	all := make([]*Scene, 0, len(r.scenes))
	for _, s := range r.scenes {
		all = append(all, s)
	}
	r.mu.Unlock()

	r.log.Info("停止全部场景", "count", len(all))
	for _, s := range all {
		s.Stop()
	}
	if r.cancel != nil {
		r.cancel()
	}

	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		r.log.Info("全部场景已停止")
		return nil
	case <-time.After(timeout):
		// 超时不是"再等等", 是"有场景卡住了"。必须报出来 —— 这通常意味着
		// 有人在场景 goroutine 里做了阻塞 IO, 违反了第一条纪律。
		remaining := r.Count()
		err := fmt.Errorf("scene: %s 内未排空全部场景，仍有 %d 个场景未停止", timeout, remaining)
		r.log.Error("等待场景退出超时, 可能有场景阻塞在 IO",
			"timeout", timeout, "剩余", remaining)
		return err
	}
}

func (r *Router) trialLevelOf(id domain.SceneID) int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.trialLevels[id]
}
func (r *Router) finishTrial(id domain.SceneID) {
	r.mu.Lock()
	r.trialCompleted[id] = true
	r.mu.Unlock()
}

func (r *Router) admitDungeon(id domain.SceneID) {
	r.mu.Lock()
	r.instanceAdmitted[id] = true
	r.mu.Unlock()
}
