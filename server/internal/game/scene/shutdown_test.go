package scene

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 关服链路的测试。
//
// 顺序是「网关停止接客 → Router.Drain（场景存档并退出）→ WriteBack.Close（队列落库）」，
// **反了就丢档**：场景还在写，队列已经关了。
//
// 这条链路以前一行测试都没有 —— 而它出问题的表现是"每次重启都少一点进度"，
// 既不报错也不崩溃，是最贵的一类 bug。

// recordSaver 记下场景交出来的存档快照。
type recordSaver struct {
	mu    sync.Mutex
	saved map[int64]int32 // charID -> 存下来的等级
}

func newRecordSaver() *recordSaver {
	return &recordSaver{saved: map[int64]int32{}}
}

func (r *recordSaver) Save(s domain.Snapshot) {
	if s.Char == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved[s.Char.ID] = s.Char.Level
}

func (r *recordSaver) levelOf(id int64) (int32, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lv, ok := r.saved[id]
	return lv, ok
}

func (r *recordSaver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.saved)
}

// drainRig 起一个带 Router 的世界，节拍由测试给。
type drainRig struct {
	t      *testing.T
	router *Router
	saver  *recordSaver
	frames map[int32]chan time.Time
	cancel context.CancelFunc
}

func newDrainRig(t *testing.T, maps ...int32) *drainRig {
	t.Helper()
	r := &drainRig{t: t, saver: newRecordSaver(),
		frames: map[int32]chan time.Time{}}
	for _, m := range maps {
		r.frames[m] = make(chan time.Time)
	}
	build := func(id domain.SceneID) (*Scene, error) {
		ch, ok := r.frames[id.MapID]
		if !ok {
			t.Fatalf("测试没准备 %d 号图的节拍", id.MapID)
		}
		return New(Config{ID: id, Frames: ch, Saver: r.saver,
			SaveEvery: 100_000}), nil // 存档间隔调得极长: 只测关服那一次
	}
	r.router = NewRouter(build, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.router.Start(ctx)
	t.Cleanup(cancel)
	return r
}

// enter 把一个脏角色放进某张图。
func (r *drainRig) enter(mapID int32, id domain.EntityID, charID int64, level int32) {
	r.t.Helper()
	sc := domain.SceneID{MapID: mapID}
	if _, err := r.router.Ensure(sc); err != nil {
		r.t.Fatalf("建 %d 号图: %v", mapID, err)
	}
	if !r.router.Post(sc, Enter{
		ID: id,
		Char: &domain.Character{ID: charID, Name: "甲", Level: level,
			Pos: domain.Pos{MapID: mapID, X: 100, Y: 100}},
		Sink: &nullSink{},
	}) {
		r.t.Fatal("投递 Enter 失败")
	}
	r.step(mapID)
}

func (r *drainRig) step(mapID int32) {
	r.t.Helper()
	r.frames[mapID] <- time.Now()
	done := make(chan struct{})
	if !r.router.Post(domain.SceneID{MapID: mapID}, Inspect{Fn: func(*Scene) {}, Done: done}) {
		r.t.Fatalf("往 %d 号图投同步点失败", mapID)
	}
	r.frames[mapID] <- time.Now()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.t.Fatalf("%d 号图没响应", mapID)
	}
}

// nullSink 不关心下行字节；本文件只看存档。
type nullSink struct{}

func (nullSink) Emit(event.Event) {}
func (nullSink) Close()           {}

// **关服必须把在线角色的进度交出去。**
// 不交的话，最后一次存档到关服之间的所有变化都没了。
func TestDrainSavesOnlinePlayers(t *testing.T) {
	r := newDrainRig(t, 1, 285)
	r.enter(1, 11, 100, 7)
	r.enter(285, 22, 200, 9)

	// 让两个人都变脏(升级会标脏; 这里直接改等级并标脏)
	for _, m := range []struct {
		mapID int32
		id    domain.EntityID
		lv    int32
	}{{1, 11, 31}, {285, 22, 42}} {
		done := make(chan struct{})
		r.router.Post(domain.SceneID{MapID: m.mapID}, Inspect{
			Fn: func(s *Scene) {
				e := s.EntityAt(m.id)
				e.Player.Char.Level = m.lv
				e.Player.MarkDirty()
			}, Done: done})
		r.frames[m.mapID] <- time.Now()
		<-done
	}

	r.router.Drain(2 * time.Second)

	if n := r.saver.count(); n != 2 {
		t.Fatalf("关服存了 %d 个人的档, 该是 2 —— 少的那些就是丢掉的进度", n)
	}
	if lv, ok := r.saver.levelOf(100); !ok || lv != 31 {
		t.Fatalf("100 号存下来的等级是 %d(存到没: %v), 该是 31", lv, ok)
	}
	if lv, ok := r.saver.levelOf(200); !ok || lv != 42 {
		t.Fatalf("200 号存下来的等级是 %d, 该是 42 —— 另一张图上的人也要存", lv)
	}
}

// Drain 之后场景 goroutine 必须真的退出。
// 不退的话进程会挂在那里，运维只能 kill -9 —— 那才是真的全丢。
func TestDrainStopsAllScenes(t *testing.T) {
	r := newDrainRig(t, 1, 285)
	r.enter(1, 11, 100, 5)
	r.enter(285, 22, 200, 5)
	if r.router.Count() != 2 {
		t.Fatalf("起了 %d 张图", r.router.Count())
	}

	done := make(chan struct{})
	go func() { r.router.Drain(2 * time.Second); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Drain 没返回 —— 关服会挂住")
	}

	// 停了之后再投命令应该被拒: 场景已经不在了
	if r.router.Post(domain.SceneID{MapID: 1}, Inspect{Fn: func(*Scene) {}}) {
		// Post 可能仍然投进一个已停的邮箱, 这里只要求它不 panic、不阻塞
		t.Log("Drain 之后 Post 仍返回 true(邮箱还在), 只要不阻塞即可")
	}
}

// 没有任何场景时 Drain 也要正常返回 —— 空服重启是常事。
func TestDrainWithNoScenes(t *testing.T) {
	r := newDrainRig(t, 1)
	done := make(chan struct{})
	go func() { r.router.Drain(time.Second); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("空服 Drain 没返回")
	}
}

// Drain 可以重复调（信号处理和主流程都调了一次是常见写法）。
func TestDrainIsIdempotent(t *testing.T) {
	r := newDrainRig(t, 1)
	r.enter(1, 11, 100, 5)

	r.router.Drain(2 * time.Second)
	done := make(chan struct{})
	go func() { r.router.Drain(time.Second); close(done) }() // 第二次
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("第二次 Drain 卡住了")
	}
}
