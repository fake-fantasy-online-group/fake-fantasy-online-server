package scene

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 跨场景传送是 Actor 模型里最容易写错的一段: 实体的所有权要从一个 goroutine
// 交到另一个。这些测试盯的就是"交接有没有交干净"。

// 龙城(7) 那扇通往南郊(14) 的门, 用真实数据:
//
//	proc_id 9 | map_to 14 | to (9768,1982) | poly {5442,6906, 5577,6986, 5442,7066, 5307,6986}
var 南郊门 = domain.NewPortal(domain.Teleport{
	ProcID: 9,
	To:     domain.Pos{MapID: 14, X: 9768, Y: 1982},
	ToDir:  67,
	Area: domain.Polygon{
		{X: 5442, Y: 6906}, {X: 5577, Y: 6986}, {X: 5442, Y: 7066}, {X: 5307, Y: 6986},
	},
})

// twoScenes 起一个真的 Router, 里面两张图: 龙城(7) 有门, 南郊(14) 没有。
//
// **每张图一条自己的节拍**。共用一条的话, 喂进去的那一帧会被两张图里
// 随便哪个抢走 —— 测试就变成了掷骰子(实测在 -count=2 下偶发失败)。
type sceneRig struct {
	r      *Router
	frames map[int32]chan time.Time
}

func twoScenes(t *testing.T) (*Router, *sceneRig) {
	t.Helper()
	rig := &sceneRig{frames: map[int32]chan time.Time{
		7: make(chan time.Time), 14: make(chan time.Time),
	}}
	rig.r = NewRouter(func(id domain.SceneID) (*Scene, error) {
		ch, ok := rig.frames[id.MapID]
		if !ok {
			ch = make(chan time.Time)
			rig.frames[id.MapID] = ch
		}
		cfg := Config{ID: id, SaveEvery: 10_000, Seed: 7, Frames: ch,
			Items: map[domain.ItemID]domain.ItemDef{
				场景钢剑.ID: 场景钢剑,
				掉落树枝.ID: 掉落树枝,
			}}
		if id.MapID == 7 {
			cfg.Portals = []domain.Portal{南郊门}
		}
		return New(cfg), nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	rig.r.Start(ctx)
	t.Cleanup(cancel)
	return rig.r, rig
}

// inspect 推帧直到 fn 跑过, **并且那一帧真的跑完了**。
//
// ⚠️ 第二个 Inspect 是栅栏, 不是多余的。一帧的顺序是
// `drain()`(排 mailbox, Inspect 在这里关 done) → `step()`(末尾才 flush)。
// 只等第一个的话, 函数返回时**本帧的 flush 还没发生** ——
// 紧接着 sink.take() 就会读到一个还没送出事件的 sink。
//
// 平时窗口极小所以看不出来, 只有全量测试并行跑(机器有负载)时才会输。
// dungeon_test 的 rig 犯过同一个错, 两处是同一个原因。
func inspect(t *testing.T, s *Scene, rig *sceneRig, fn func(*Scene)) {
	t.Helper()
	frames := rig.frames[s.ID().MapID]
	if frames == nil {
		t.Fatalf("图 %d 没有节拍通道", s.ID().MapID)
	}
	inspectPump(t, s, frames, fn)
	inspectPump(t, s, frames, func(*Scene) {}) // 栅栏: 确认上一帧已经冲刷完
}

func inspectPump(t *testing.T, s *Scene, frames chan time.Time, fn func(*Scene)) {
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
			t.Fatal("场景没响应 Inspect")
		}
	}
}

// 完整的一次传送: 甲从龙城被送到南郊, 两边的账本都要对。
func TestTeleportMovesOwnership(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})

	sink := &fakeSink{}
	char := &domain.Character{ID: 100, Name: "甲", Level: 3,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}
	src.Post(Enter{ID: 1, Char: char, Sink: sink})
	var wantHP, wantMP int32
	inspect(t, src, rig, func(s *Scene) {
		if s.PlayerCount() != 1 {
			t.Errorf("甲没进龙城")
		}
		e := s.EntityAt(1)
		wantHP, wantMP = e.MaxHP-1, e.MaxMP-1
		e.HP, e.MP = wantHP, wantMP
		e.Dir = 11 // 旧朝向；落地必须由 Teleport.Dir 覆盖。
		e.Status.Apply(domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: 10}, s.tick, e.ID)
		e.Stats.AtkSpeedMS = 1500
		e.DidAttack(s.tick)
		e.Player.Protect(s.tick + 20)
		e.Player.Work = &domain.WorkSession{Def: domain.WorkDef{ID: 1}}
	})

	src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14},
		At: domain.Pos{MapID: 14, X: 9768, Y: 1982}, Dir: 67})
	inspect(t, src, rig, func(s *Scene) {
		if s.PlayerCount() != 0 {
			t.Error("源场景应该已经把人交出去了")
		}
		if s.EntityAt(1) != nil {
			t.Error("源场景还留着实体 —— 这就是两个 goroutine 同时持有同一个对象")
		}
	})
	inspect(t, dst, rig, func(s *Scene) {
		if s.PlayerCount() != 1 {
			t.Fatalf("目标场景应收到人, 实际在线 %d", s.PlayerCount())
		}
		e := s.EntityAt(1)
		if e == nil {
			t.Fatal("目标场景里找不到实体")
		}
		if e.Name != "甲" {
			t.Errorf("名字对不上: %s", e.Name)
		}
		// 实体 id 跟着人走 —— 是同一个实体换了地方, 不是新造一个
		if e.ID != 1 {
			t.Errorf("实体 id 应保持不变, 实际 %d", e.ID)
		}
		if e.Pos.MapID != 14 || e.Pos.X != 9768 || e.Pos.Y != 1982 {
			t.Errorf("落点不对: %+v", e.Pos)
		}
		if e.HP != wantHP || e.MP != wantMP {
			t.Errorf("跨图不应补满 HP/MP: got (%d,%d), want (%d,%d)", e.HP, e.MP, wantHP, wantMP)
		}
		if e.Dir != 67 {
			t.Errorf("落地方向应采用传送命令: got %d, want 67", e.Dir)
		}
		if !e.Status.Has(domain.StatusSlow) {
			t.Error("跨图不应清空状态")
		}
		if e.ReadyToAttack(s.tick) {
			t.Error("跨图不应清空攻击冷却")
		}
		if !e.Player.Protected(s.tick) {
			t.Error("跨图不应清空复活保护")
		}
		if e.Player.Work != nil {
			t.Error("跨图必须显式停止打工")
		}
	})

	// 目标场景创建/投递失败时，源场景已经摘除了实体，只能保存后断线。
	// 保存位置必须回到失败前的源坐标；否则玩家重登会落在这次根本没成功的目标图。
	failedFrames := make(chan time.Time)
	failedSaver := newFakeSaver()
	fallbackSaver := newFakeSaver()
	failedSink := &fakeSink{}
	failedRouter := NewRouter(func(id domain.SceneID) (*Scene, error) {
		if id.MapID == 14 {
			return nil, errors.New("目标场景不可用")
		}
		return New(Config{ID: id, Saver: fallbackSaver, SaveEvery: 10_000, Frames: failedFrames}), nil
	}, nil)
	failedCtx, failedCancel := context.WithCancel(context.Background())
	failedRouter.Start(failedCtx)
	t.Cleanup(failedCancel)
	failedSrc, err := failedRouter.Ensure(domain.SceneID{MapID: 7})
	if err != nil {
		t.Fatalf("创建失败传送的源场景: %v", err)
	}
	failedRig := &sceneRig{r: failedRouter, frames: map[int32]chan time.Time{7: failedFrames}}
	failedChar := &domain.Character{ID: 101, Name: "乙", Level: 3,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}
	failedSrc.Post(Enter{ID: 2, Char: failedChar, Sink: failedSink, FinalSaver: failedSaver})
	inspect(t, failedSrc, failedRig, func(*Scene) {})

	// Character.Pos 在线期间不会随移动更新，故意先移动一次，确保回滚取的是
	// 源实体的权威位置，而不是 Character 里仍为 (100,100) 的旧值。
	sourcePos := domain.Pos{MapID: 7, X: 222, Y: 333}
	failedSrc.Post(MoveTo{ID: 2, To: sourcePos})
	inspect(t, failedSrc, failedRig, func(*Scene) {})
	failedSrc.Post(Teleport{ID: 2, To: domain.SceneID{MapID: 14},
		At: domain.Pos{MapID: 14, X: 9768, Y: 1982}, Dir: 67})
	inspect(t, failedSrc, failedRig, func(s *Scene) {
		if s.EntityAt(2) != nil {
			t.Error("失败交接后源场景不应继续持有实体")
		}
	})

	if !failedSink.isClosed() {
		t.Error("失败交接后应断开连接，等待玩家从源坐标重登")
	}
	if got := failedSaver.count(failedChar.ID); got != 1 {
		t.Fatalf("失败交接应保存一次最终快照，实际 %d 次", got)
	}
	failedSaver.mu.Lock()
	saved := failedSaver.last[failedChar.ID]
	failedSaver.mu.Unlock()
	if saved == nil {
		t.Fatal("失败交接没有保存角色快照")
	}
	if saved.Pos != sourcePos {
		t.Fatalf("失败交接保存了未成功的目标落点: got %+v, want source %+v", saved.Pos, sourcePos)
	}
	if got := fallbackSaver.count(failedChar.ID); got != 0 {
		t.Fatalf("有 FinalSaver 时不应重复走普通 saver，实际 %d 次", got)
	}

	// 没有 FinalSaver 的测试/降级路径同样必须先回滚源坐标，再交给场景 saver。
	fallbackSink := &fakeSink{}
	fallbackChar := &domain.Character{ID: 102, Name: "丙", Level: 3,
		Pos: domain.Pos{MapID: 7, X: 400, Y: 500}}
	failedSrc.Post(Enter{ID: 3, Char: fallbackChar, Sink: fallbackSink})
	inspect(t, failedSrc, failedRig, func(*Scene) {})
	fallbackSourcePos := domain.Pos{MapID: 7, X: 444, Y: 555}
	failedSrc.Post(MoveTo{ID: 3, To: fallbackSourcePos})
	inspect(t, failedSrc, failedRig, func(*Scene) {})
	failedSrc.Post(Teleport{ID: 3, To: domain.SceneID{MapID: 14},
		At: domain.Pos{MapID: 14, X: 8765, Y: 1234}, Dir: 19})
	inspect(t, failedSrc, failedRig, func(*Scene) {})
	if !fallbackSink.isClosed() {
		t.Error("普通 saver 降级路径也应在交接失败后断开连接")
	}
	if got := fallbackSaver.count(fallbackChar.ID); got != 1 {
		t.Fatalf("普通 saver 应保存一次失败交接快照，实际 %d 次", got)
	}
	fallbackSaver.mu.Lock()
	fallbackSaved := fallbackSaver.last[fallbackChar.ID]
	fallbackSaver.mu.Unlock()
	if fallbackSaved == nil || fallbackSaved.Pos != fallbackSourcePos {
		t.Fatalf("普通 saver 保存位置不对: got %+v, want source %+v", fallbackSaved, fallbackSourcePos)
	}
}

// 落地只下发专用 Teleported —— 会话先据此改绑场景，再编码 0x803a。
// SelfEntered 是完整初次入场 bootstrap，跨图重发会清空客户端运行态。
func TestTeleportEmitsDedicatedTeleported(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})
	targetLanding := domain.Pos{MapID: 14, X: 9768, Y: 1982}
	returnPoint := domain.Pos{MapID: 7, X: 300, Y: 300}
	// 目标图的落点本身也位于一扇门里，覆盖跨图 Enter 对 portalBlocked 的初始化。
	returnPortal := domain.NewPortal(domain.Teleport{
		ProcID: 100, To: returnPoint, ToDir: 11,
		Area: domain.Polygon{
			{X: 9718, Y: 1982}, {X: 9768, Y: 1932},
			{X: 9818, Y: 1982}, {X: 9768, Y: 2032},
		},
	})
	inspect(t, dst, rig, func(s *Scene) {
		s.portals = append(s.portals, returnPortal)
	})

	sink := &fakeSink{}
	src.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲",
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: sink})
	inspect(t, src, rig, func(*Scene) {})
	sink.take()

	src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14}, At: targetLanding, Dir: 67})
	inspect(t, src, rig, func(*Scene) {})
	inspect(t, dst, rig, func(*Scene) {})

	events := sink.take()
	te, ok := firstOf[event.Teleported](events)
	if !ok {
		t.Fatal("传送落地应下发专用 Teleported")
	}
	if te.Scene != (domain.SceneID{MapID: 14}) {
		t.Fatalf("Teleported 场景不对: %+v", te.Scene)
	}
	if te.To != targetLanding {
		t.Fatalf("Teleported 落点不对: %+v", te.To)
	}
	if _, ok := firstOf[event.SelfEntered](events); ok {
		t.Fatal("跨图不应重发初次入场 SelfEntered")
	}

	// 第一条门内移动不能立刻弹回；完全离开后重新进入，门才再次触发。
	if !dst.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 14, X: 9769, Y: 1982}}) {
		t.Fatal("投递目标门区内移动失败")
	}
	inspect(t, dst, rig, func(s *Scene) {
		if s.EntityAt(1) == nil {
			t.Error("跨图落在门区后，第一条门内移动不应弹回")
		}
		if _, blocked := s.portalBlocked[1]; !blocked {
			t.Error("目标图仍在门区时必须保持阻断")
		}
	})
	if _, ok := firstOf[event.Teleported](sink.take()); ok {
		t.Fatal("目标图门区内的小步移动不应再次传送")
	}
	if !dst.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 14, X: 9900, Y: 2150}}) {
		t.Fatal("投递离开目标门区移动失败")
	}
	inspect(t, dst, rig, func(s *Scene) {
		if _, blocked := s.portalBlocked[1]; blocked {
			t.Error("离开目标图全部门区后应重新武装")
		}
	})
	if !dst.Post(MoveTo{ID: 1, To: targetLanding}) {
		t.Fatal("投递重新进入目标门区移动失败")
	}
	inspect(t, dst, rig, func(s *Scene) {
		if s.EntityAt(1) != nil {
			t.Error("重新进入目标门区后应传回 map7")
		}
	})
	inspect(t, src, rig, func(s *Scene) {
		if e := s.EntityAt(1); e == nil || e.Pos != returnPoint {
			t.Errorf("重新入门后应回到 map7 权威落点，实际 %+v", e)
		}
	})
}

// 走出去的人, 旁观者要看到他消失。
func TestTeleportBroadcastsDespawnToWatchers(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	_, _ = r.Ensure(domain.SceneID{MapID: 14})

	watcher := &fakeSink{}
	src.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲",
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: &fakeSink{}})
	src.Post(Enter{ID: 2, Char: &domain.Character{ID: 200, Name: "乙",
		Pos: domain.Pos{MapID: 7, X: 150, Y: 150}}, Sink: watcher})
	inspect(t, src, rig, func(*Scene) {})
	watcher.take()

	src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14},
		At: domain.Pos{MapID: 14, X: 9768, Y: 1982}})
	inspect(t, src, rig, func(*Scene) {})

	if _, ok := firstOf[event.EntityDespawned](watcher.take()); !ok {
		t.Fatal("乙应看到甲消失, 否则屏幕上会留下一个不动的幽灵")
	}
}

// 同图传送不走所有权交接，但仍要给本人专用 Teleported 权威落点；普通移动包
// 刻意不发给本人。它也不受眩晕影响，并且是一个战斗/门区边沿的不连续点。
func TestTeleportWithinSameSceneIsJustAMove(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})
	landing := domain.Pos{MapID: 7, X: 5442, Y: 6986}
	trigger := domain.Pos{MapID: 7, X: 2000, Y: 2000}
	// 这扇测试门在同一张图内，但落点位于现有跨图门的多边形中心。
	// 从 (100,100) 走进触发区会跨多个 AOI 格，专门覆盖真实 MoveTo 踩门时
	// WorkStopped 不能因位置已变化而漏给本人的回归。
	sameMapPortal := domain.NewPortal(domain.Teleport{
		ProcID: 99, To: landing, ToDir: 19,
		Area: domain.Polygon{
			{X: 1900, Y: 2000}, {X: 2000, Y: 1900},
			{X: 2100, Y: 2000}, {X: 2000, Y: 2100},
		},
	})
	inspect(t, src, rig, func(s *Scene) {
		s.portals = append(s.portals, sameMapPortal)
	})

	sink := &fakeSink{}
	oldWatcher := &fakeSink{}
	triggerWatcher := &fakeSink{}
	landingWatcher := &fakeSink{}
	src.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲",
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: sink})
	src.Post(Enter{ID: 2, Char: &domain.Character{ID: 200, Name: "起点旁观者",
		Pos: domain.Pos{MapID: 7, X: 110, Y: 100}}, Sink: oldWatcher})
	src.Post(Enter{ID: 3, Char: &domain.Character{ID: 300, Name: "门点旁观者",
		Pos: trigger}, Sink: triggerWatcher})
	src.Post(Enter{ID: 5, Char: &domain.Character{ID: 500, Name: "落点旁观者",
		Pos: landing}, Sink: landingWatcher})
	inspect(t, src, rig, func(*Scene) {})
	sink.take()
	oldWatcher.take()
	triggerWatcher.take()
	landingWatcher.take()

	// 把“排队动作 → 同帧传送”放进同一次 drain；若先推进一帧，攻击和技能
	// 已经结算，恰好绕过了这条回归。Inspect 只负责在 actor 内布置队列，后面的
	// 真正的 MoveTo 踩门与断言 Inspect 按 mailbox 顺序紧随其后。
	staged := make(chan struct{})
	if !src.Post(Inspect{Done: staged, Fn: func(s *Scene) {
		e := s.EntityAt(1)
		e.Status.Apply(domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: 10}, s.tick, e.ID)
		e.Stats.AtkSpeedMS = 1500
		e.DidAttack(s.tick)
		e.Player.Work = &domain.WorkSession{Def: domain.WorkDef{ID: 1}}

		// 出战宠物虽是辅助单位，防御性清理仍要移除旧版本可能遗留的攻击、
		// 施法和命中队列。它本身不应再持有战斗目标。
		const petID domain.EntityID = 4
		e.Player.Pet = petID
		pet := &entity.Entity{ID: petID, Kind: domain.KindPet,
			Pet: &entity.Pet{Owner: e.ID}}
		s.entities[petID] = pet
		monster := &entity.Entity{ID: 20, Kind: domain.KindMonster,
			Monster: &entity.Monster{Target: e.ID}}
		s.entities[monster.ID] = monster
		s.monsters[monster.ID] = monster

		s.attacks = []pendingAttack{
			{src: e.ID, dst: monster.ID}, {src: monster.ID, dst: e.ID},
			{src: petID, dst: monster.ID}, {src: monster.ID, dst: petID},
			{src: monster.ID, dst: 21}, // 无关 control，必须保留。
		}
		s.casts = []pendingCast{
			{src: e.ID, targets: []domain.EntityID{monster.ID}},
			{src: petID, targets: []domain.EntityID{monster.ID}},
			{src: monster.ID, targets: []domain.EntityID{e.ID, 21}},
			{src: monster.ID, targets: []domain.EntityID{petID}},
			{src: 22, targets: []domain.EntityID{23}}, // 无关 control。
		}
		for attacker, target := range map[domain.EntityID]domain.EntityID{
			e.ID: monster.ID, petID: monster.ID, monster.ID: e.ID,
		} {
			tracker := &combat.HitTracker{}
			tracker.Engage(target, 1, s.rng)
			s.hits[attacker] = tracker
		}
	}}) {
		t.Fatal("投递战斗队列布置失败")
	}

	// 用真实移动踩入同图门，而不是直接投 Teleport；同图门的权威落点故意在
	// 现有跨图门区中心，后续门内小步移动必须被 portalBlocked 压住。
	if !src.Post(MoveTo{ID: 1, To: trigger}) {
		t.Fatal("投递同图门移动失败")
	}
	inspect(t, src, rig, func(s *Scene) {
		if s.EntityAt(1) == nil {
			t.Error("人应该还在这张图")
		}
		e := s.EntityAt(1)
		if e == nil || e.Pos != landing {
			t.Errorf("应就地挪过去, 实际 %+v", e)
		}
		if e != nil {
			if !e.Status.Has(domain.StatusSlow) {
				t.Error("同图传送不应清空状态")
			}
			if e.ReadyToAttack(s.tick) {
				t.Error("同图传送不应清空攻击冷却")
			}
			if e.Player.Work != nil {
				t.Error("同图传送必须先停止打工")
			}
		}
		if len(s.attacks) != 1 || s.attacks[0].src != 20 || s.attacks[0].dst != 21 {
			t.Errorf("只应保留无关普通攻击: %+v", s.attacks)
		}
		if len(s.casts) != 2 || s.casts[0].src != 20 ||
			len(s.casts[0].targets) != 1 || s.casts[0].targets[0] != 21 ||
			s.casts[1].src != 22 || len(s.casts[1].targets) != 1 || s.casts[1].targets[0] != 23 {
			t.Errorf("技能应只剔除传送相关目标并保留 control: %+v", s.casts)
		}
		if s.hits[1] != nil || s.hits[4] != nil {
			t.Error("本人和出战宠物的命中步进器应清除")
		}
		if tracker := s.hits[20]; tracker == nil || tracker.Target() != 0 {
			t.Errorf("以传送者为目标的命中步进器应重置: %+v", tracker)
		}
		if pet := s.entities[4]; pet == nil || pet.Pet == nil || pet.Pet.Target != 0 {
			t.Errorf("出战宠物应脱战: %+v", pet)
		}
		if monster := s.monsters[20]; monster == nil || monster.Monster.Target != 0 {
			t.Errorf("以传送者为目标的怪物应脱战: %+v", monster)
		}
		if _, blocked := s.portalBlocked[1]; !blocked {
			t.Error("落点在门区时必须保持门触发阻断")
		}
	})
	select {
	case <-staged:
	default:
		t.Fatal("战斗队列布置命令未执行")
	}
	events := sink.take()
	if _, ok := firstOf[event.SelfEntered](events); ok {
		t.Fatal("同图传送不该重发 SelfEntered —— 客户端会白重载一次场景")
	}
	te, ok := firstOf[event.Teleported](events)
	if !ok {
		t.Fatal("同图传送也必须给本人下发权威 Teleported")
	}
	if te.Scene != (domain.SceneID{MapID: 7}) || te.To != landing {
		t.Fatalf("同图 Teleported 落点不对: %+v", te)
	}
	workAt, teleportAt := -1, -1
	for i, ev := range events {
		switch ev.(type) {
		case event.WorkStopped:
			if workAt < 0 {
				workAt = i
			}
		case event.Teleported:
			if teleportAt < 0 {
				teleportAt = i
			}
		}
	}
	if workAt < 0 || teleportAt < 0 || workAt >= teleportAt {
		t.Fatalf("应先停工再下发权威落点: WorkStopped=%d Teleported=%d events=%T", workAt, teleportAt, events)
	}
	workCount := 0
	for _, ev := range events {
		if _, ok := ev.(event.WorkStopped); ok {
			workCount++
		}
	}
	if workCount != 1 {
		t.Fatalf("本人应恰好收到一次 WorkStopped，实际 %d: %+v", workCount, events)
	}

	oldEvents := oldWatcher.take()
	oldWorkAt, oldMoveAt := -1, -1
	for i, ev := range oldEvents {
		switch typed := ev.(type) {
		case event.WorkStopped:
			if typed.Who == 1 && oldWorkAt < 0 {
				oldWorkAt = i
			}
		case event.EntityMoved:
			if typed.ID == 1 && oldMoveAt < 0 {
				oldMoveAt = i
			}
		case event.EntityDespawned:
			if typed.ID == 1 {
				t.Errorf("同图全图同步不应因传送距离远而消失: %+v", typed)
			}
		}
	}
	if oldWorkAt < 0 || oldMoveAt < 0 || oldWorkAt >= oldMoveAt {
		t.Fatalf("起点旁观者应先看到收工再看到同图移动: WorkStopped=%d Move=%d events=%+v",
			oldWorkAt, oldMoveAt, oldEvents)
	}
	triggerEvents := triggerWatcher.take()
	if mv, ok := firstOf[event.EntityMoved](triggerEvents); !ok || mv.ID != 1 || mv.To != landing {
		t.Fatalf("门点旁观者应收到同图权威移动: %+v", triggerEvents)
	}
	landingEvents := landingWatcher.take()
	landingMove := 0
	for _, ev := range landingEvents {
		switch typed := ev.(type) {
		case event.EntityMoved:
			if typed.ID == 1 {
				landingMove++
			}
		case event.EntitySpawned:
			if typed.ID == 1 {
				t.Errorf("同图玩家已全图出场，传送时不应重复 spawn: %+v", typed)
			}
		case event.EntityDespawned:
			if typed.ID == 1 {
				t.Errorf("落点旁观者不应在同次传送里收到玩家消失: %+v", typed)
			}
		}
	}
	if landingMove != 1 {
		t.Fatalf("落点旁观者应恰好收到一次玩家移动，实际 %d: %+v", landingMove, landingEvents)
	}

	// 主动移动受眩晕限制，但服务端直接传送必须 force 到权威落点。仍留在同一
	// 门区内也不得同步或在下一包重入。
	stunnedLanding := domain.Pos{MapID: 7, X: 5443, Y: 6986}
	inspect(t, src, rig, func(s *Scene) {
		e := s.EntityAt(1)
		e.Status.Apply(domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 10}, s.tick, e.ID)
	})
	if !src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 7}, At: stunnedLanding}) {
		t.Fatal("投递眩晕中的服务端同图传送失败")
	}
	inspect(t, src, rig, func(s *Scene) {
		e := s.EntityAt(1)
		if e == nil || e.Pos != stunnedLanding || !e.Status.Has(domain.StatusStun) {
			t.Errorf("眩晕中也应保留状态并到达权威落点: %+v", e)
		}
	})
	stunnedEvents := sink.take()
	if te, ok := firstOf[event.Teleported](stunnedEvents); !ok || te.To != stunnedLanding {
		t.Fatalf("眩晕中的同图传送也必须下发 Teleported: %+v", stunnedEvents)
	}

	// 先移除眩晕，才能证明下一条门内移动由 portalBlocked 抑制，而不是恰好
	// 被 canAct 吞掉。
	inspect(t, src, rig, func(s *Scene) {
		s.EntityAt(1).Status.Remove(domain.StatusStun)
	})
	if !src.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 5444, Y: 6986}}) {
		t.Fatal("投递门区内移动失败")
	}
	inspect(t, src, rig, func(s *Scene) {
		if s.EntityAt(1) == nil {
			t.Error("传送落点仍在门区时，下一条移动不得再次切图")
		}
		if _, blocked := s.portalBlocked[1]; !blocked {
			t.Error("仍在任意门区内时不能提前重新武装")
		}
	})
	if _, ok := firstOf[event.Teleported](sink.take()); ok {
		t.Fatal("门区内小步移动不应再发 Teleported")
	}

	// 完整离开后，本次移动只负责解除阻断；随后重新进入才恢复正常触门。
	if !src.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 5442, Y: 6800}}) {
		t.Fatal("投递离开门区移动失败")
	}
	inspect(t, src, rig, func(s *Scene) {
		if _, blocked := s.portalBlocked[1]; blocked {
			t.Error("完全离开全部门区后应重新武装")
		}
	})
	if !src.Post(MoveTo{ID: 1, To: landing}) {
		t.Fatal("投递重新入门移动失败")
	}
	inspect(t, src, rig, func(s *Scene) {
		if s.EntityAt(1) != nil {
			t.Error("离区后重新进门应正常跨图")
		}
	})
	inspect(t, dst, rig, func(s *Scene) {
		if e := s.EntityAt(1); e == nil || e.Pos.X != 9768 || e.Pos.Y != 1982 {
			t.Errorf("重新入门后应落到 map14 目标点, 实际 %+v", e)
		}
	})
}

// ── 传送门触发 ──

// 走进门的多边形就该被传走。
func TestSteppingOnPortalTeleports(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})

	src.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲",
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: &fakeSink{}})
	inspect(t, src, rig, func(*Scene) {})

	// 走到门的正中心(菱形中心 5442,6986)
	src.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 5442, Y: 6986}})
	inspect(t, src, rig, func(*Scene) {})
	inspect(t, src, rig, func(s *Scene) {
		if s.PlayerCount() != 0 {
			t.Error("踩到门就该被传走")
		}
	})
	inspect(t, dst, rig, func(s *Scene) {
		e := s.EntityAt(1)
		if e == nil {
			t.Fatal("没到南郊")
		}
		// 落点取自客户端 .link 的 map_to_pos
		if e.Pos.X != 9768 || e.Pos.Y != 1982 {
			t.Errorf("落点应是 (9768,1982), 实际 (%.0f,%.0f)", e.Pos.X, e.Pos.Y)
		}
	})
}

// 从门旁边走过不该被吸进去。
func TestWalkingNearPortalDoesNotTeleport(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})

	src.Post(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲",
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: &fakeSink{}})
	inspect(t, src, rig, func(*Scene) {})

	// 外接矩形之外一点点
	src.Post(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 5442, Y: 6800}})
	inspect(t, src, rig, func(*Scene) {})
	inspect(t, src, rig, func(s *Scene) {
		if s.PlayerCount() != 1 {
			t.Error("没踩到门不该被传走")
		}
	})
}

// 怪不走传送门。它们要是能跟着穿图, 一张图的怪会漏到另一张图去。
func TestMonstersDoNotUsePortals(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000,
		Portals: []domain.Portal{南郊门}})
	m := putMonster(s, 900, 100, domain.Stats{}, 5442, 6986) // 直接站在门中心
	s.step()
	if _, still := s.entities[900]; !still {
		t.Fatal("怪不该走传送门")
	}
	if m.Pos.MapID != 7 {
		t.Fatal("怪被传走了")
	}
}
