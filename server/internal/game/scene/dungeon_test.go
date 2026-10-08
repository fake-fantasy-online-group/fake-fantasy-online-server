package scene

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 塔狱的真实定义(pworld.def, GBK 解出来核对过):
//
//	进 (20502, 4582, 4477)   出 (507, 393, 621)   限时 900 秒
var 塔狱 = domain.DungeonDef{
	ID: 205, Name: "塔狱",
	Enter:        domain.Pos{MapID: 20502, X: 4582, Y: 4477},
	Exit:         domain.Pos{MapID: 507, X: 393, Y: 621},
	TimeoutSec:   900,
	MaxInstances: 2, // 测试收紧到 2, 好验上限
}

// dungeonRig 起一个 Router: 龙城(7) 常驻, 塔狱(20502) 按需开实例。
//
// frames 要加锁: Builder 在 Router 的锁里跑(调 Ensure 的那个 goroutine),
// 测试自己又要读它 —— 不加锁就是一处竞态(实测被 -race 抓到过)。
type dungeonRig struct {
	r  *Router
	mu sync.Mutex
	fr map[domain.SceneID]chan time.Time
}

func (rig *dungeonRig) put(id domain.SceneID, ch chan time.Time) {
	rig.mu.Lock()
	defer rig.mu.Unlock()
	rig.fr[id] = ch
}

func (rig *dungeonRig) get(id domain.SceneID) chan time.Time {
	rig.mu.Lock()
	defer rig.mu.Unlock()
	return rig.fr[id]
}

// instances 返回已经建出来的副本实例 id。
func (rig *dungeonRig) instances(mapID int32) []domain.SceneID {
	rig.mu.Lock()
	defer rig.mu.Unlock()
	var out []domain.SceneID
	for id := range rig.fr {
		if id.MapID == mapID && !id.Persistent() {
			out = append(out, id)
		}
	}
	return out
}

func dungeonSetup(t *testing.T) *dungeonRig {
	t.Helper()
	rig := &dungeonRig{fr: map[domain.SceneID]chan time.Time{}}
	tbl := domain.DungeonTable{塔狱.Enter.MapID: 塔狱}
	rig.r = NewRouter(func(id domain.SceneID) (*Scene, error) {
		ch := make(chan time.Time)
		rig.put(id, ch)
		cfg := Config{ID: id, SaveEvery: 10_000, Seed: 5, Frames: ch,
			Dungeons: tbl, Alloc: domain.NewEntityAlloc(),
			NPCs: []domain.NPCSpawn{{Name: "通天塔守卫", Sprite: 1,
				Pos: domain.Pos{MapID: id.MapID, X: 120, Y: 100}}}}
		if !id.Persistent() {
			d := 塔狱
			cfg.Dungeon = &d
		}
		return New(cfg), nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	rig.r.Start(ctx)
	t.Cleanup(cancel)
	return rig
}

// step 给某个场景喂帧直到它处理完一条 Inspect。
// step 推帧直到 fn 跑过, **并且那一帧真的跑完了**。
//
// ⚠️ 第二个 Inspect 不是多余的。一帧的顺序是
// `drain()`(排 mailbox, Inspect 在这里关 done) → `step()`(末尾才 flush)。
// 只等第一个 Inspect 的话, 函数返回时**本帧的 flush 还没发生** ——
// 紧接着 sink.take() 就会读到一个还没送出事件的 sink。
//
// 平时窗口很小所以看不出来, 全量测试并行跑(机器有负载)时才会输 ——
// 这条正是那样被抓到的: 单跑一千次都过, 全量 -count=2 挂一次。
//
// 第二个 Inspect 必然在**后一帧**被排到, 所以它跑完就说明前一帧连 flush 都结束了。
func (rig *dungeonRig) step(t *testing.T, s *Scene, fn func(*Scene)) {
	t.Helper()
	frames := rig.get(s.ID())
	if frames == nil {
		t.Fatalf("场景 %v 没有节拍通道", s.ID())
	}
	rig.inspectOnce(t, s, frames, fn)              // 跑用户那一格
	rig.inspectOnce(t, s, frames, func(*Scene) {}) // 栅栏: 确认上一帧已经冲刷完
}

// inspectOnce 投一个 Inspect 并泵帧直到它跑完。
func (rig *dungeonRig) inspectOnce(t *testing.T, s *Scene, frames chan time.Time, fn func(*Scene)) {
	t.Helper()
	done := make(chan struct{})
	if !s.Post(Inspect{Fn: fn, Done: done}) {
		t.Fatal("投递 Inspect 失败")
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-done:
			return
		case frames <- time.Now():
		case <-deadline:
			t.Fatal("场景没响应")
		}
	}
}

// pump 给某个场景喂 n 帧(不等回执)。
func (rig *dungeonRig) pump(t *testing.T, id domain.SceneID, n int) {
	t.Helper()
	frames := rig.get(id)
	for i := 0; i < n; i++ {
		select {
		case frames <- time.Now():
		case <-time.After(time.Second):
			return // 场景已经停了
		}
	}
}

// **同一张图能同时存在多个实例** —— 这是"副本是实例"那条决策唯一真正被用到的地方。
func TestOpenInstanceCreatesSeparateScenes(t *testing.T) {
	rig := dungeonSetup(t)

	a, err := rig.r.OpenInstance(塔狱, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.MapID != 20502 || a.Instance == 0 {
		t.Fatalf("应开出一个塔狱实例, 实际 %+v", a)
	}
	if a.Persistent() {
		t.Fatal("副本实例不该被当成常驻图")
	}

	// 同一个实例还开着时, 再进的人应该复用它 —— 否则副本没法组队打
	b, err := rig.r.OpenInstance(塔狱, 1)
	if err != nil {
		t.Fatal(err)
	}
	if b != a {
		t.Fatalf("应复用已有实例, 实际开了新的: %+v vs %+v", a, b)
	}
}

// 实例数有上限 —— 不封顶的话一张图能被开出无数个实例把内存吃光。
func TestInstanceCapEnforced(t *testing.T) {
	rig := dungeonSetup(t)

	a, _ := rig.r.OpenInstance(塔狱, 1)
	sa, _ := rig.r.Ensure(a)

	// 把第一个实例停掉, 再开就会是新的
	sa.Stop()
	time.Sleep(20 * time.Millisecond)

	b, err := rig.r.OpenInstance(塔狱, 1)
	if err != nil {
		t.Fatal(err)
	}
	if b == a {
		t.Fatal("已经停掉的实例不该被复用")
	}
	rig.r.Ensure(b)

	// 反复开只会一直复用那一个 —— 这本身就是"不会无限增长"的保证。
	// 真正会新开实例的路径只有"已有的都在关闭中", 所以把它们全停掉再开,
	// 就能把实例数顶到上限。
	for i := 0; i < int(塔狱.InstanceCap())+3; i++ {
		id, err := rig.r.OpenInstance(塔狱, 1)
		if err != nil {
			// 到上限了。此时活着的实例数不该超过上限
			live := 0
			rig.r.mu.RLock()
			for sid := range rig.r.scenes {
				if sid.MapID == 20502 && !sid.Persistent() {
					live++
				}
			}
			rig.r.mu.RUnlock()
			if int32(live) > 塔狱.InstanceCap() {
				t.Fatalf("活着 %d 个实例, 超过上限 %d", live, 塔狱.InstanceCap())
			}
			return
		}
		s, _ := rig.r.Ensure(id)
		s.Stop() // 停掉, 逼下一次去开新的
		time.Sleep(15 * time.Millisecond)
	}
}

// 进副本: 站在 NPC 面前 → 被传送到实例里。
func TestEnterDungeonTeleportsIntoInstance(t *testing.T) {
	rig := dungeonSetup(t)
	town, _ := rig.r.Ensure(domain.SceneID{MapID: 7})

	sink := &fakeSink{}
	town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: sink, Party: 1,
		Char: &domain.Character{ID: 100, Name: "甲", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}})
	rig.step(t, town, func(*Scene) {})

	town.Post(EnterDungeon{ID: 1, MapID: 20502, NPC: "通天塔守卫"})
	rig.step(t, town, func(*Scene) {}) // 处理 EnterDungeon, 它会再投一条 Teleport
	rig.step(t, town, func(s *Scene) {
		if s.PlayerCount() != 0 {
			t.Error("人应该已经离开主城")
		}
	})

	// 找到那个实例
	insts := rig.instances(20502)
	if len(insts) == 0 {
		t.Fatal("没开出副本实例")
	}
	inst := insts[0]
	is, _ := rig.r.Ensure(inst)
	rig.step(t, is, func(s *Scene) {
		e := s.EntityAt(1)
		if e == nil {
			t.Fatal("人没到副本里")
		}
		if e.Pos.MapID != 20502 || e.Pos.X != 4582 || e.Pos.Y != 4477 {
			t.Errorf("落点应是 pworld.def 里的 InitPos (20502,4582,4477), 实际 %+v", e.Pos)
		}
	})
}

// 不校验距离：入口 NPC 在当前地图里时，隔得再远也能进副本。
func TestEnterDungeonDoesNotCheckDistance(t *testing.T) {
	rig := dungeonSetup(t)
	town, _ := rig.r.Ensure(domain.SceneID{MapID: 7})

	sink := &fakeSink{}
	town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: sink, Party: 1,
		Char: &domain.Character{ID: 100, Name: "甲", Pos: domain.Pos{MapID: 7, X: 9000, Y: 9000}}})
	rig.step(t, town, func(*Scene) {})
	sink.take()

	town.Post(EnterDungeon{ID: 1, MapID: 20502, NPC: "通天塔守卫"})
	rig.step(t, town, func(*Scene) {})
	rig.step(t, town, func(s *Scene) {
		if s.PlayerCount() != 0 {
			t.Error("距离不应拦截进副本，玩家应该已经离开主城")
		}
	})

	insts := rig.instances(20502)
	if len(insts) == 0 {
		t.Fatal("距离不应拦截创建副本实例")
	}
	is, _ := rig.r.Ensure(insts[0])
	rig.step(t, is, func(s *Scene) {
		if s.EntityAt(1) == nil {
			t.Fatal("距离不应拦截玩家进入副本")
		}
	})
	if rej, ok := firstOf[event.Rejected](sink.take()); ok {
		t.Fatalf("不应因距离拒绝进副本, 实际 %+v", rej)
	}
}

// 不存在的副本进不去。
func TestEnterUnknownDungeon(t *testing.T) {
	rig := dungeonSetup(t)
	town, _ := rig.r.Ensure(domain.SceneID{MapID: 7})
	sink := &fakeSink{}
	town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: sink,
		Char: &domain.Character{ID: 100, Name: "甲", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}})
	rig.step(t, town, func(*Scene) {})
	sink.take()

	town.Post(EnterDungeon{ID: 1, MapID: 99999, NPC: "通天塔守卫"})
	rig.step(t, town, func(*Scene) {})
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectNoTarget {
		t.Fatalf("不存在的副本应被拒, 实际 %+v", rej)
	}
}

// 空实例过了宽限期就自己关掉。
//
// 宽限期存在的理由: **队伍会分批进** —— 第一个人进去、第二个人还在读条,
// 中间那一瞬间实例是空的。立刻关掉会让后进的人掉进一个新实例。
func TestEmptyInstanceClosesAfterGrace(t *testing.T) {
	rig := dungeonSetup(t)
	id, _ := rig.r.OpenInstance(塔狱, 1)

	// 宽限期内不该关
	grace := int(domain.Ticks(domain.EmptyInstanceGraceSec * 1000))
	rig.pump(t, id, grace/2)
	if rig.r.Count() == 0 {
		t.Fatal("宽限期内不该关掉实例")
	}

	// 过了宽限期应当关掉并从路由表里摘掉
	rig.pump(t, id, grace+5)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := lookup(rig.r, id); err != nil {
			return // 已经摘掉了
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("空实例过了宽限期还没关")
}

// 常驻大陆图**永远不会**因为没人就关掉 —— 它们要一直在那儿等人来。
func TestPersistentSceneNeverCloses(t *testing.T) {
	rig := dungeonSetup(t)
	town, _ := rig.r.Ensure(domain.SceneID{MapID: 7})

	grace := int(domain.Ticks(domain.EmptyInstanceGraceSec * 1000))
	rig.pump(t, town.ID(), grace+20)
	rig.step(t, town, func(*Scene) {}) // 还能响应就说明还活着
}

// 限时到了要把人送回出口, 而不是断线。
func TestDungeonTimeoutEvictsToExit(t *testing.T) {
	// 收紧限时, 免得测试要跑 9000 帧
	short := 塔狱
	short.TimeoutSec = 2 // 20 帧

	rig := &dungeonRig{fr: map[domain.SceneID]chan time.Time{}}
	tbl := domain.DungeonTable{short.Enter.MapID: short}
	rig.r = NewRouter(func(id domain.SceneID) (*Scene, error) {
		ch := make(chan time.Time)
		rig.put(id, ch)
		cfg := Config{ID: id, SaveEvery: 10_000, Seed: 5, Frames: ch,
			Dungeons: tbl, Alloc: domain.NewEntityAlloc()}
		if !id.Persistent() {
			d := short
			cfg.Dungeon = &d
		}
		return New(cfg), nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	rig.r.Start(ctx)
	t.Cleanup(cancel)

	// 先把出口图建出来
	exit, _ := rig.r.Ensure(domain.SceneID{MapID: 507})

	id, _ := rig.r.OpenInstance(short, 1)
	inst, _ := rig.r.Ensure(id)
	sink := &fakeSink{}
	inst.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: sink,
		Char: &domain.Character{ID: 100, Name: "甲", Pos: short.Enter}})
	rig.step(t, inst, func(s *Scene) {
		if s.PlayerCount() != 1 {
			t.Fatal("人没进副本")
		}
	})

	// 跑过限时
	rig.pump(t, id, 30)

	// 人应该出现在出口图上, 而且落点是 pworld.def 的 ExpPos
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		rig.step(t, exit, func(s *Scene) {
			if e := s.EntityAt(1); e != nil {
				found = true
				if e.Pos.MapID != 507 || e.Pos.X != 393 || e.Pos.Y != 621 {
					t.Errorf("落点应是出口 (507,393,621), 实际 %+v", e.Pos)
				}
			}
		})
		if found {
			if sink.isClosed() {
				t.Error("限时到应该是被请出来, 不是被断线")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("限时到了人没被送回出口")
}

// lookup 查一个场景在不在路由表里, 不建新的。
func lookup(r *Router, id domain.SceneID) (*Scene, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.scenes[id]; ok {
		return s, nil
	}
	return nil, errNotFound
}

var errNotFound = errNotFoundType{}

type errNotFoundType struct{}

func (errNotFoundType) Error() string { return "not found" }
