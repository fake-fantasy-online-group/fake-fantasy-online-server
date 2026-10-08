package scene

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 测试直接驱动 drain/step, 不跑 Run —— 逻辑帧的意义就在这: "过了 1.5 秒"等于
// step 15 次, 不需要 sleep, 也不会因为机器忙就偶发失败。
// 只有 TestRunLoopShutdown 例外, 它要验的就是 Run 那个循环本身。

// ── 测试替身 ──

type fakeSink struct {
	mu     sync.Mutex
	evs    []event.Event
	closed bool
}

func (f *fakeSink) Emit(e event.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evs = append(f.evs, e)
}
func (f *fakeSink) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// take 取出并清空已收事件。
func (f *fakeSink) take() []event.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.evs
	f.evs = nil
	return out
}

func (f *fakeSink) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

type fakeSaver struct {
	mu      sync.Mutex
	saves   map[int64]int
	last    map[int64]*domain.Character
	lastBag map[int64]*domain.Bag
}

func newFakeSaver() *fakeSaver {
	return &fakeSaver{saves: map[int64]int{}, last: map[int64]*domain.Character{},
		lastBag: map[int64]*domain.Bag{}}
}

func (s *fakeSaver) Save(snap domain.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves[snap.Char.ID]++
	s.last[snap.Char.ID] = snap.Char
	s.lastBag[snap.Char.ID] = snap.Bag
}

func (s *fakeSaver) count(id int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves[id]
}

// ── 辅助 ──

func newTestScene(t *testing.T, saver Saver) *Scene {
	t.Helper()
	return New(Config{ID: domain.SceneID{MapID: 7}, Saver: saver, SaveEvery: 10})
}

// join 让一个玩家进场景并推进一帧, 返回它的 sink。
func join(t *testing.T, s *Scene, id domain.EntityID, charID int64, name string, x, y float64) *fakeSink {
	t.Helper()
	sink := &fakeSink{}
	s.exec(Enter{
		ID: id,
		Char: &domain.Character{
			ID: charID, Name: name, Level: 1,
			Pos: domain.Pos{MapID: 7, X: x, Y: y},
		},
		Sink: sink,
	})
	return sink
}

// countOf 数某类事件有几条。
func countOf[T event.Event](evs []event.Event) int {
	n := 0
	for _, e := range evs {
		if _, ok := e.(T); ok {
			n++
		}
	}
	return n
}

func firstOf[T event.Event](evs []event.Event) (T, bool) {
	for _, e := range evs {
		if v, ok := e.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// ── 进出场景 ──

// 进场景时下发顺序必须是"先我, 后别人": 客户端要先有自己的实体, 才能把别人摆到周围。
func TestEnterSelfBeforeOthers(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150)
	s.step()

	evs := sinkB.take()
	if len(evs) < 2 {
		t.Fatalf("乙应至少收到 SelfEntered + 甲的出场, 实际 %d 条", len(evs))
	}
	if _, ok := evs[0].(event.SelfEntered); !ok {
		t.Fatalf("第一条必须是 SelfEntered, 实际 %T —— 客户端没有'我'就摆不了别人", evs[0])
	}
	sp, ok := firstOf[event.EntitySpawned](evs)
	if !ok || sp.Name != "甲" {
		t.Fatalf("乙应看到甲出场, 实际 %+v", evs)
	}
}

// 后进来的人要被已在场的人看到 —— 视野是双向的。
func TestEnterIsMutuallyVisible(t *testing.T) {
	s := newTestScene(t, nil)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sinkA.take() // 清掉甲自己的进场事件

	join(t, s, 2, 200, "乙", 150, 150)
	s.step()

	sp, ok := firstOf[event.EntitySpawned](sinkA.take())
	if !ok || sp.Name != "乙" {
		t.Fatal("甲没看到乙进来 —— 视野必须是双向的")
	}
}

// 同图实体全图同步，距离再远也要互相可见。
func TestEnterFarApartStillVisibleAcrossMap(t *testing.T) {
	s := newTestScene(t, nil)
	sinkA := join(t, s, 1, 100, "甲", 100, 100) // 格 (0,0)
	s.step()
	sinkA.take()

	sinkB := join(t, s, 2, 200, "乙", 5000, 5000) // 格 (16,16), 远在视野外
	s.step()

	if sp, ok := firstOf[event.EntitySpawned](sinkA.take()); !ok || sp.ID != 2 {
		t.Fatalf("甲应看到同图 5000 单位外的乙, 实际 %+v", sp)
	}
	if sp, ok := firstOf[event.EntitySpawned](sinkB.take()); !ok || sp.ID != 1 {
		t.Fatalf("乙应看到同图远处的甲, 实际 %+v", sp)
	}
}

// ── 移动与 AOI ──

// 格内移动: 广播给同区的人, **但不发给自己** —— 客户端已经本地走过了,
// 再收一条自己的移动会把画面拽回去。
func TestMoveWithinCell(t *testing.T) {
	s := newTestScene(t, nil)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150)
	s.step()
	sinkA.take()
	sinkB.take()

	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 120, Y: 120}})
	s.step()

	if n := countOf[event.EntityMoved](sinkB.take()); n != 1 {
		t.Fatalf("乙应收到 1 条甲的移动, 实际 %d", n)
	}
	if n := countOf[event.EntityMoved](sinkA.take()); n != 0 {
		t.Fatalf("甲不该收到自己的移动(会把客户端画面拽回去), 实际 %d 条", n)
	}
}

// 跨格走远仍在同一张图：只广播移动，不触发消失。
func TestMoveFarAwayStaysVisibleAcrossMap(t *testing.T) {
	s := newTestScene(t, nil)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150)
	s.step()
	sinkA.take()
	sinkB.take()

	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 3000, Y: 100}}) // 格 (0,0) -> (10,0)
	s.step()

	if evs := sinkA.take(); countOf[event.EntityMoved](evs) != 0 || countOf[event.EntityDespawned](evs) != 0 {
		t.Fatalf("甲不应收到自己的移动或任何消失事件: %+v", evs)
	}
	if evs := sinkB.take(); countOf[event.EntityMoved](evs) != 1 || countOf[event.EntityDespawned](evs) != 0 {
		t.Fatalf("乙应只收到甲的远距离移动，实体仍常驻同图: %+v", evs)
	}
}

// 原本相距很远的同图玩家早已互相出场，走近时只需要移动包。
func TestMoveNearSendsMoveWithoutRespawn(t *testing.T) {
	s := newTestScene(t, nil)
	sinkA := join(t, s, 1, 100, "甲", 3000, 100) // 格 (10,0)
	sinkB := join(t, s, 2, 200, "乙", 100, 100)  // 格 (0,0), 互不可见
	s.step()
	sinkA.take()
	sinkB.take()

	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 150, Y: 100}})
	s.step()

	if evs := sinkA.take(); countOf[event.EntityMoved](evs) != 0 || countOf[event.EntitySpawned](evs) != 0 {
		t.Fatalf("甲不应收到自己的移动或重复出场: %+v", evs)
	}
	if evs := sinkB.take(); countOf[event.EntityMoved](evs) != 1 || countOf[event.EntitySpawned](evs) != 0 {
		t.Fatalf("乙应只收到甲走近的移动，不重复出场: %+v", evs)
	}
}

// 跨格但仍互相可见(相邻格): 只发移动, 不该重发出场 ——
// 重发出场会让客户端把实体瞬移重建, 表现为闪烁。
func TestMoveCrossCellStillVisibleSendsMoveOnly(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 290, 100) // 格 (0,0)
	sinkB := join(t, s, 2, 200, "乙", 100, 100)
	s.step()
	sinkB.take()

	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 310, Y: 100}}) // 跨到格 (1,0), 仍相邻
	s.step()

	evs := sinkB.take()
	if n := countOf[event.EntitySpawned](evs); n != 0 {
		t.Fatalf("仍在视野内不该重发出场(会让客户端闪烁), 实际 %d 条", n)
	}
	if n := countOf[event.EntityMoved](evs); n != 1 {
		t.Fatalf("应只发 1 条移动, 实际 %d", n)
	}
}

// 离开场景要广播给周围的人。
func TestLeaveBroadcastsDespawn(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150)
	s.step()
	sinkB.take()

	s.exec(Leave{ID: 1, Reason: "登出"})
	s.step()

	ds, ok := firstOf[event.EntityDespawned](sinkB.take())
	if !ok || ds.ID != 1 || ds.Reason != event.DespawnLogout {
		t.Fatalf("乙应收到甲登出消失, 实际 %+v", ds)
	}
	if s.PlayerCount() != 1 {
		t.Fatalf("在线数应为 1, 实际 %d", s.PlayerCount())
	}
}

// ── 存档 ──

// 进图标了脏, 一个存档周期后应落盘。
func TestPeriodicSave(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	join(t, s, 1, 100, "存档测试", 100, 100)

	for i := 0; i < 10; i++ { // SaveEvery = 10 帧
		s.step()
	}
	if got := sv.count(100); got != 1 {
		t.Fatalf("一个周期后应存 1 次, 实际 %d", got)
	}
}

// 没变更就不该反复存 —— 否则几百人在线时数据库全是无意义的写。
func TestSaveOnlyDirty(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	join(t, s, 1, 100, "只存一次", 100, 100)

	for i := 0; i < 50; i++ { // 跨 5 个存档周期
		s.step()
	}
	if got := sv.count(100); got != 1 {
		t.Fatalf("只在进图时脏过一次, 应只存 1 次, 实际 %d", got)
	}
}

// 移动会标脏, 下个周期应再存一次。
func TestMoveMarksDirty(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	join(t, s, 1, 100, "移动标脏", 100, 100)
	for i := 0; i < 10; i++ {
		s.step()
	}

	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 500, Y: 500}})
	for i := 0; i < 10; i++ {
		s.step()
	}
	if got := sv.count(100); got != 2 {
		t.Fatalf("移动后应再存一次(共 2), 实际 %d", got)
	}
}

// 登出是关键节点, 必须立刻排进存档队列, 不等下个周期。
func TestLeaveSavesImmediately(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	join(t, s, 1, 100, "登出存档", 100, 100)
	s.exec(Leave{ID: 1, Reason: "登出"})

	if got := sv.count(100); got != 1 {
		t.Fatalf("登出应立刻存档, 实际 %d 次", got)
	}
}

// 存出去的必须是**克隆**。传指针的话, 场景下一帧改它就和写库的 goroutine 打架了。
func TestSaveClonesCharacter(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	sink := &fakeSink{}
	orig := &domain.Character{ID: 100, Name: "克隆检查", Level: 1,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}, Attrs: []int32{1, 2, 3}}
	s.exec(Enter{ID: 1, Char: orig, Sink: sink})
	s.exec(Leave{ID: 1, Reason: "登出"})

	sv.mu.Lock()
	saved := sv.last[100]
	sv.mu.Unlock()
	if saved == orig {
		t.Fatal("存档拿到的是同一个指针 —— 场景改它就和写库 goroutine 打架了")
	}
	saved.Attrs[0] = 9
	if orig.Attrs[0] == 9 {
		t.Fatal("Attrs 是浅拷贝, 底层数组还共享着 —— 等于没拷")
	}
}

// 关服无条件存全部在线角色, 不管脏不脏。
func TestShutdownSavesAll(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	join(t, s, 2, 200, "乙", 150, 150)
	for i := 0; i < 10; i++ { // 先让周期存档把脏标记清掉
		s.step()
	}

	s.shutdown("test")

	if sv.count(100) != 2 || sv.count(200) != 2 {
		t.Fatalf("关服应无条件再存一次, 实际 甲=%d 乙=%d", sv.count(100), sv.count(200))
	}
	if !sinkA.isClosed() {
		t.Fatal("关服后应断开玩家连接")
	}
}

// ── 韧性 ──

// 一条命令 panic 只能毁这一帧, 不能带走整张图。
func TestFramePanicIsContained(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "幸存者", 100, 100)
	s.step()

	s.Post(Inspect{Fn: func(*Scene) { panic("模拟业务代码写崩了") }, Done: make(chan struct{})})
	if ok := s.frame(); !ok {
		t.Fatal("单帧 panic 不该让场景放弃 —— 一张图崩了不能带走全服")
	}
	if s.PlayerCount() != 1 {
		t.Fatalf("panic 后玩家应还在场景里, 实际 %d", s.PlayerCount())
	}

	// 崩过之后还得能正常干活
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 200, Y: 200}})
	s.step()
	if s.Tick() == 0 {
		t.Fatal("panic 之后场景应能继续推进")
	}
}

// 连着崩说明状态本身坏了, 这时候该收摊而不是继续把错误写进存档。
func TestRepeatedPanicGivesUp(t *testing.T) {
	s := newTestScene(t, nil)
	boom := func() { s.Post(Inspect{Fn: func(*Scene) { panic("坏了") }, Done: make(chan struct{})}) }
	for i := 1; i < maxPanicsInARow; i++ {
		boom()
		if ok := s.frame(); !ok {
			t.Fatalf("第 %d 次 panic 就放弃了, 太早", i)
		}
	}
	boom()
	if ok := s.frame(); ok {
		t.Fatalf("连续 %d 帧 panic 后应放弃这张图", maxPanicsInARow)
	}
}

// mailbox 满了要丢并返回 false, 不能阻塞投递方 ——
// 一个刷包的客户端不能把整张图的循环拖住。
func TestMailboxFullDropsInsteadOfBlocking(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, Mailbox: 2})
	if !s.Post(MoveTo{ID: 1}) || !s.Post(MoveTo{ID: 1}) {
		t.Fatal("前两条应该进得去")
	}
	done := make(chan bool, 1)
	go func() { done <- s.Post(MoveTo{ID: 1}) }()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("队列满时 Post 应返回 false")
		}
	case <-time.After(time.Second):
		t.Fatal("Post 阻塞了 —— 这会让慢客户端卡死整张图")
	}
}

// Leave 不能沿用普通 Post 的“队列满就丢”。它可以等一个场景帧腾出位置；否则
// 同一客户端重登后，旧玩家会永久留在场景里并以 0x8023 发给新实体。
func TestLifecyclePostWaitsForMailboxSpace(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, Mailbox: 2})
	if !s.Post(MoveTo{ID: 1}) || !s.Post(MoveTo{ID: 1}) {
		t.Fatal("测试前提：mailbox 应已填满")
	}

	done := make(chan bool, 1)
	go func() { done <- s.PostLifecycle(Leave{ID: 1, Reason: "断线"}) }()
	select {
	case <-done:
		t.Fatal("mailbox 仍满时生命周期投递不该返回")
	default:
	}

	// 模拟下一帧排空普通命令；被阻塞的 Leave 随后必须成功入队。
	s.drain()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("场景仍运行且已腾出空间，生命周期投递却失败")
		}
	case <-time.After(time.Second):
		t.Fatal("mailbox 腾出空间后生命周期投递仍未返回")
	}
}

func TestLifecyclePostStopsWaitingWhenSceneStops(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, Mailbox: 1})
	if !s.Post(MoveTo{ID: 1}) {
		t.Fatal("测试前提：mailbox 应已填满")
	}
	done := make(chan bool, 1)
	go func() { done <- s.PostLifecycle(Leave{ID: 1, Reason: "断线"}) }()
	s.Stop()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("场景停止后生命周期命令不该报告已投递")
		}
	case <-time.After(time.Second):
		t.Fatal("场景停止后生命周期投递没有解除等待")
	}
}

// ── 定时器 ──

func TestTimersFireInOrder(t *testing.T) {
	var tm timers
	var got []int
	tm.after(0, 5, func() { got = append(got, 5) })
	tm.after(0, 1, func() { got = append(got, 1) })
	tm.after(0, 3, func() { got = append(got, 3) })

	for tick := domain.Tick(1); tick <= 5; tick++ {
		tm.advance(tick)
	}
	want := []int{1, 3, 5}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("定时器应按帧序触发, 期望 %v 实际 %v", want, got)
	}
}

// 回调里再排定时器不能死循环 —— after 至少 +1 帧, 新的必然落在未来。
func TestTimerRescheduleDoesNotLoop(t *testing.T) {
	var tm timers
	n := 0
	var again func()
	again = func() {
		n++
		if n < 3 {
			tm.after(domain.Tick(n), 0, again) // d=0 会被当成 1 帧
		}
	}
	tm.after(0, 1, again)

	done := make(chan struct{})
	go func() {
		for tick := domain.Tick(1); tick <= 5; tick++ {
			tm.advance(tick)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("定时器回调里重排导致死循环")
	}
	if n != 3 {
		t.Fatalf("应触发 3 次, 实际 %d", n)
	}
}

// ── 循环本身 ──

// Run 的正常路径: 起来、跑帧、收到 ctx 取消后存档退出。
func TestRunLoopShutdown(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	ctx, cancel := context.WithCancel(context.Background())

	stopped := make(chan struct{})
	go func() { s.Run(ctx); close(stopped) }()

	sink := &fakeSink{}
	s.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "循环测试",
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: sink})

	// 等场景真的处理掉 Enter(用 Inspect 同步一下, 比 sleep 可靠)
	waitInspect(t, s, func(sc *Scene) {
		if sc.PlayerCount() != 1 {
			t.Errorf("玩家没进去, 在线 %d", sc.PlayerCount())
		}
	})

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 没有在 ctx 取消后退出")
	}
	if sv.count(100) == 0 {
		t.Fatal("关服未保存在线角色")
	}
}

// waitInspect 投一条 Inspect 并等它执行完。测试里用它做同步点,
// 比 sleep 可靠 —— Inspect 在场景 goroutine 内执行, 回来就说明前面的命令都处理完了。
func waitInspect(t *testing.T, s *Scene, fn func(*Scene)) {
	t.Helper()
	done := make(chan struct{})
	if !s.Post(Inspect{Fn: fn, Done: done}) {
		t.Fatal("投递 Inspect 失败")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("场景没有响应 Inspect —— 循环可能卡住了")
	}
}

// 走过之后, 存档里必须是**走到的地方**, 不是进图时那个坐标。
//
// 曾经全服没有任何一处往 Char.Pos 写: 走路只动 AOI 里的实体(e.Pos),
// 而存档存的是 Char。表现是玩家走一整天、下线再上线还站在原地,
// 服务端日志却一切正常 —— "批量存档 count=1" 每 30 秒照打, 只是存的是旧值。
// 实测靠"客户端走到 9300, 库里一直是 9000"才发现。
func TestSaveKeepsWhereThePlayerWalkedTo(t *testing.T) {
	sv := newFakeSaver()
	s := newTestScene(t, sv)
	join(t, s, 1, 100, "行者", 100, 100)

	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 640, Y: 480}})
	for i := 0; i < 10; i++ { // SaveEvery = 10 帧
		s.step()
	}

	sv.mu.Lock()
	got := sv.last[100]
	sv.mu.Unlock()
	if got == nil {
		t.Fatal("走完之后没有产生存档")
	}
	if got.Pos.X != 640 || got.Pos.Y != 480 {
		t.Fatalf("存档里的位置是 (%v,%v), 该是走到的 (640,480)", got.Pos.X, got.Pos.Y)
	}
}
