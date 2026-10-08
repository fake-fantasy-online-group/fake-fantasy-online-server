package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 主动怪: 树妖的真实数值, 但把 aggressive 打开(真实数据里 14 级的树妖不主动)。
var 主动树妖 = domain.MonsterDef{
	ID: 1001, Name: "树妖", Kind: domain.MonsterNormal, Level: 14, HP: 246,
	Sprite: 201, Stats: domain.NewMonsterStats(14, 6, 42, 0, 0, 2000, 110),
	Aggressive: true, ViewDist: 300, TraceDist: 600,
}

// 被动怪: 同样数值但不主动。挨打才还手。
var 被动树妖 = func() domain.MonsterDef {
	d := 主动树妖
	d.Aggressive = false
	return d
}()

// 不会动的怪(食人花那类, 移速 0)。
var 食人花 = domain.MonsterDef{
	ID: 1002, Name: "食人花", Kind: domain.MonsterNormal, Level: 10, HP: 146,
	Sprite: 202, Stats: domain.NewMonsterStats(12, 5, 30, 0, 0, 2000, 0),
	Aggressive: true, ViewDist: 300, TraceDist: 600,
}

// aiScene 建一个只有一只怪的场景, 怪在 (mx,my)。
func aiScene(t *testing.T, def domain.MonsterDef, mx, my float64) (*Scene, domain.EntityID) {
	t.Helper()
	sp, skipped := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: def.ID, Pos: domain.Pos{MapID: 14, X: mx, Y: my}}},
		spawn.MapDefs{def.ID: def}, domain.NewEntityAlloc())
	if skipped != 0 {
		t.Fatal("测试数据有问题")
	}
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 99, Spawner: sp})
	var id domain.EntityID
	for eid, e := range s.entities {
		if e.Kind == domain.KindMonster {
			id = eid
		}
	}
	if id == 0 {
		t.Fatal("没刷出怪")
	}
	return s, id
}

// runFrames 推 n 帧。
func runFrames(s *Scene, n int) {
	for i := 0; i < n; i++ {
		s.step()
	}
}

// 主动怪看到玩家会追过来。
func TestAggressiveMonsterChasesPlayer(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 300, 100)
	join(t, s, 1, 100, "甲", 100, 100) // 距离 200, 在视野 300 内
	m := s.entities[mid]
	startX := m.Pos.X

	runFrames(s, aiScanEvery*2) // 给足够帧数让它扫到目标并挪几步

	if m.Monster.Target != 1 {
		t.Fatalf("怪应盯上玩家, 实际目标 %d", m.Monster.Target)
	}
	if m.Pos.X >= startX {
		t.Fatalf("怪应朝玩家(X=100)移动, X 从 %.0f 变成 %.0f", startX, m.Pos.X)
	}
}

// 视野外的玩家不该被盯上。
func TestMonsterIgnoresPlayerOutOfView(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 100, 100)
	join(t, s, 1, 100, "甲", 600, 100) // 距离 500 > 视野 300
	runFrames(s, aiScanEvery*3)

	if tgt := s.entities[mid].Monster.Target; tgt != 0 {
		t.Fatalf("视野外的玩家不该被盯上, 实际目标 %d", tgt)
	}
}

// 不主动的怪不会来找你 —— 但你打它一下, 它一定还手。
func TestPassiveMonsterOnlyFightsBack(t *testing.T) {
	s, mid := aiScene(t, 被动树妖, 140, 100)
	sink := join(t, s, 1, 100, "甲", 100, 100) // 距离 40, 在近战距离内
	s.entities[1].Stats = domain.Stats{MinAtk: 1, MaxAtk: 1, Hit: 1 << 20, AtkSpeedMS: 1500}

	runFrames(s, aiScanEvery*3)
	if tgt := s.entities[mid].Monster.Target; tgt != 0 {
		t.Fatalf("不主动的怪不该主动上仇恨, 实际目标 %d", tgt)
	}
	sink.take()

	// 打它一下
	s.exec(Attack{ID: 1, Target: mid})
	s.step()
	if tgt := s.entities[mid].Monster.Target; tgt != 1 {
		t.Fatalf("挨打之后应该还手, 实际目标 %d", tgt)
	}

	// 它确实会打回来
	runFrames(s, 30)
	var hitBack bool
	for _, ev := range sink.take() {
		if d, ok := ev.(event.DamageDealt); ok && d.Src == mid && d.Dst == 1 {
			hitBack = true
		}
	}
	if !hitBack {
		t.Fatal("被动怪挨打后应该反击")
	}
}

// 够得着就打, 而且按攻速节奏打 —— 不能每帧一下。
func TestMonsterAttacksAtItsOwnSpeed(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 140, 100) // 距离 40 < 近战 75
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats.MaxHP = 1 << 20
	s.entities[1].HP = 1 << 20

	const frames = 100 // 10 秒
	runFrames(s, aiScanEvery+frames)

	hits := 0
	for _, ev := range sink.take() {
		if d, ok := ev.(event.DamageDealt); ok && d.Src == mid {
			hits++
		}
	}
	// 攻速 2000ms = 20 帧, 100 帧里最多 5~6 下
	if hits == 0 {
		t.Fatal("怪应该在打人")
	}
	if hits > 8 {
		t.Fatalf("100 帧里打了 %d 下, 攻速 2000ms 最多 5~6 下 —— 冷却没生效", hits)
	}
}

func TestStationaryAttackDoesNotSendMovement(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 140, 100)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.entities[1]
	p.MaxHP, p.HP = 1<<20, 1<<20
	m := s.entities[mid]
	m.Monster.Target = 1
	sink.take()

	start := m.Pos
	s.step()
	evs := sink.take()
	if n := countOf[event.DamageDealt](evs); n != 1 {
		t.Fatalf("攻击帧应直接正常出手，实际 %d 条伤害", n)
	}
	for _, ev := range evs {
		if mv, ok := ev.(event.EntityMoved); ok && mv.ID == mid {
			t.Fatalf("原地攻击不应下发怪物移动事件，实际 %+v", mv)
		}
	}
	if m.Pos != start {
		t.Fatalf("原地攻击不能改变服务端位置，起点 %+v，实际 %+v", start, m.Pos)
	}
}

func TestWanderDoesNotCrossBlockedTerrain(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 100, 100)
	m := s.entities[mid]
	// x=120..140 模拟一条水带；另一岸本身可走，用来证明实现校验了整段而非只验终点。
	s.walkable = func(p domain.Pos) bool { return p.X < 120 || p.X > 140 }
	m.Monster.NextRoamAt = 1

	for i := 0; i < 500; i++ {
		s.step()
		if m.Pos.X >= 120 {
			t.Fatalf("游荡路径越过了不可走水带，怪物位置 %+v", m.Pos)
		}
	}
}

// 玩家传送离开或跑出追击距离后，怪物原地脱战，不回出生点重置。
// 这保证主动怪可以被带到传送门附近并在那里聚集。
func TestMonsterDropsDistantTargetWithoutGoingHome(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 100, 100)
	join(t, s, 1, 100, "甲", 2000, 100)
	m := s.entities[mid]
	s.aoi.Move(m, domain.Pos{MapID: 14, X: 800, Y: 100})
	m.Monster.Target = 1
	s.step()

	if m.Monster.Target != 0 {
		t.Fatalf("玩家跑出追击半径后应脱战, 实际仍锁着 %d", m.Monster.Target)
	}
	dropPos := m.Pos
	runFrames(s, 10) // 小于最短游走停顿 20 帧
	if m.Pos != dropPos {
		t.Fatalf("脱战后不应朝出生点回位, 从 %v 移到了 %v", dropPos, m.Pos)
	}
}

// 怪被牵得离出生点很远，只要玩家还在身边就继续交战。
func TestMonsterKeepsTargetWhenDraggedFarFromSpawn(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 100, 100)
	m := s.entities[mid]
	m.Monster.Target = 1 // 直接给它挂上仇恨
	// 玩家贴着怪, 但怪被挪到离家 700(>600) 的地方
	join(t, s, 1, 100, "甲", 810, 100)
	s.aoi.Move(m, domain.Pos{MapID: 14, X: 800, Y: 100})

	s.step()
	if m.Monster.Target != 1 {
		t.Fatal("出生点不应触发脱战；玩家贴脸时应继续交战")
	}
}

// 天生不会动的怪够不着就干瞪眼, 不该原地抽搐。
func TestStationaryMonsterDoesNotMove(t *testing.T) {
	s, mid := aiScene(t, 食人花, 250, 100)
	join(t, s, 1, 100, "甲", 100, 100) // 距离 150: 在视野内, 但够不着
	m := s.entities[mid]
	start := m.Pos

	runFrames(s, 50)
	if m.Monster.Target != 1 {
		t.Fatal("食人花也会盯上人, 只是过不去")
	}
	if m.Pos != start {
		t.Fatalf("移速 0 的怪不该移动, 从 %v 挪到了 %v", start, m.Pos)
	}
}

// 怪移动要广播给看得见它的玩家, 否则客户端上它就是个瞬移的鬼影。
func TestMonsterMovementIsBroadcast(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 280, 100)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	runFrames(s, aiScanEvery*2)

	var moves int
	for _, ev := range sink.take() {
		if mv, ok := ev.(event.EntityMoved); ok && mv.ID == mid {
			moves++
		}
	}
	if moves == 0 {
		t.Fatal("怪在移动却没广播 —— 客户端看到的会是瞬移")
	}
}

// 目标下线之后要清目标, 不能对着一个不存在的 id 追。
func TestTargetClearedWhenPlayerLeaves(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 200, 100)
	join(t, s, 1, 100, "甲", 100, 100)
	runFrames(s, aiScanEvery*2)
	if s.entities[mid].Monster.Target == 0 {
		t.Fatal("怪应先盯上玩家")
	}

	s.exec(Leave{ID: 1, Reason: "登出"})
	s.step()
	if tgt := s.entities[mid].Monster.Target; tgt != 0 {
		t.Fatalf("玩家离场后应清目标, 实际仍锁着 %d", tgt)
	}
}

// 玩家被打死: 不能像怪那样从场上抹掉(那个实体还连着一条 TCP 连接),
// 必须等客户端复活指令才能站起来。
func TestPlayerDeathWaitsForRevive(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 140, 100)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.entities[1]
	p.MaxHP, p.HP = 20, 20 // 一两下就死

	runFrames(s, aiScanEvery+40)

	if _, ok := firstOf[event.EntityDied](sink.take()); !ok {
		t.Fatal("玩家应被怪打死")
	}
	if _, still := s.players[1]; !still {
		t.Fatal("玩家死了不能从场景里抹掉 —— 那个实体还连着一条连接")
	}
	if s.entities[mid].Monster.Target != 0 {
		t.Error("目标死了怪要脱战, 否则会一直对着尸体挥空")
	}

	runFrames(s, 200)
	if p.Alive() || p.HP != 0 {
		t.Fatalf("没有客户端复活指令时应保持死亡, 实际 %d/%d", p.HP, p.MaxHP)
	}

	s.exec(Revive{ID: p.ID, ReviveType: ReviveTypeRespawn})
	s.step()
	if !p.Alive() || p.HP != p.MaxHP {
		t.Fatalf("客户端发来复活指令后玩家应满血复活, 实际 %d/%d", p.HP, p.MaxHP)
	}

	// 复活保护: 原地复活必须配保护, 否则就是死亡循环 ——
	// 玩家在打死他的怪旁边站起来, 复活那一帧就会被重新盯上。
	// (实测过没保护时的样子: 帧 71 复活、帧 71 挨打、帧 91 再死。)
	if !p.Player.Protected(s.Tick()) {
		t.Fatal("刚复活的玩家应处于保护中")
	}
	before := p.HP
	runFrames(s, 3) // 保护窗口内
	if !p.Player.Protected(s.Tick()) {
		t.Fatal("测试推得太远了, 已经出了保护窗口")
	}
	if p.HP != before {
		t.Fatalf("保护期内不该挨打, 血从 %d 变成 %d", before, p.HP)
	}
	if s.entities[mid].Monster.Target == 1 {
		t.Fatal("保护期内怪不该重新盯上他")
	}
}

// 复活保护挡索敌, 而且**是一段时间不是永久免疫**。
//
// 直接给保护再推帧, 不走"被打死再复活"那条路 —— 那条路上玩家会反复死、
// 反复拿到新的保护, 测出来的东西不确定。
func TestReviveGraceBlocksAcquisitionThenExpires(t *testing.T) {
	s, mid := aiScene(t, 主动树妖, 200, 100)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.entities[1]
	m := s.entities[mid]

	p.Player.Protect(s.Tick() + reviveGraceTicks)
	runFrames(s, aiScanEvery*2)
	if m.Monster.Target != 0 {
		t.Fatalf("保护期内不该被盯上, 实际目标 %d", m.Monster.Target)
	}

	runFrames(s, reviveGraceTicks)
	if p.Player.Protected(s.Tick()) {
		t.Fatal("保护到期了却还生效 —— 那就成了永久免疫")
	}
	runFrames(s, aiScanEvery*2)
	if m.Monster.Target != 1 {
		t.Fatal("保护到期后怪应该重新盯上来")
	}
}

// 索敌是按帧错开的, 不是每只怪每帧都扫九宫格。
func TestScanIsStaggered(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000})
	// 连续的实体 id 应该落在不同的扫描帧上
	seen := map[uint64]bool{}
	for id := domain.EntityID(1); id <= aiScanEvery; id++ {
		for tick := domain.Tick(0); tick < aiScanEvery; tick++ {
			s.tick = tick
			if s.shouldScan(id) {
				seen[uint64(tick)] = true
			}
		}
	}
	if len(seen) != aiScanEvery {
		t.Fatalf("%d 个实体应均匀分布在 %d 个扫描帧上, 实际只用了 %d 帧",
			aiScanEvery, aiScanEvery, len(seen))
	}
}

// 参数取自实体, 而实体的参数来自配置。少一环整个 AI 的距离判断就全错。
func TestAIParamsFromEntity(t *testing.T) {
	_, _ = aiScene(t, 主动树妖, 100, 100)
	m := &entity.Entity{
		Kind: domain.KindMonster, Stats: domain.NewMonsterStats(0, 0, 0, 0, 0, 1500, 110),
		Monster: &entity.Monster{Aggressive: true, ViewDist: 300, TraceDist: 600},
	}
	p := aiParams(m)
	if p.ViewDist != 300 || p.TraceDist != 600 || p.MoveSpeed != 110 || !p.Aggressive {
		t.Fatalf("参数没从实体上取全: %+v", p)
	}
	if p.AttackDist != ai.MeleeRange {
		t.Errorf("攻击距离应是武器表的近战众数 %d, 实际 %v", ai.MeleeRange, p.AttackDist)
	}

	// 没配的话要有兜底, 否则视野 0 的怪永远不动
	bare := &entity.Entity{Kind: domain.KindMonster, Monster: &entity.Monster{}}
	bp := aiParams(bare)
	if bp.ViewDist <= 0 || bp.TraceDist <= 0 {
		t.Fatalf("缺配置时应有兜底值, 实际 %+v", bp)
	}
}
