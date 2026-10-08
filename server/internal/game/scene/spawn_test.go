package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 场景这一层要保证的是「怪什么时候出现、什么时候消失、什么时候回来」，
// 以及这三件事有没有让附近的玩家看见。怪本身长什么样是 spawn 包的事。

// 取自 game_monsters 的真实行(SQL 核对过)。
var 树妖 = domain.MonsterDef{
	ID: 1001, Name: "树妖", Kind: domain.MonsterNormal, Level: 14, HP: 246,
	Exp: 100, Sprite: 201,
	Stats: domain.NewMonsterStats(14, 6, 42, 0, 0, 2000, 100),
}

var 金星草 = domain.MonsterDef{
	ID: 2074, Name: "金星草", Kind: domain.MonsterProp, Level: 1, HP: 8,
	Sprite: 572,
	Stats:  domain.NewMonsterStats(2, 1, 3, 0, 0, 1500, 0),
}

// sceneWithSpawns 建一个带刷怪器的场景。怪全部落在 (100,100) 附近, 与测试玩家同格。
func sceneWithSpawns(t *testing.T, defs spawn.MapDefs, points []domain.SpawnPoint) *Scene {
	t.Helper()
	sp, skipped := spawn.New(points, defs, domain.NewEntityAlloc())
	if skipped != 0 {
		t.Fatalf("测试数据里有 %d 个点位引用不到模板", skipped)
	}
	return New(Config{
		ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 4242, Spawner: sp,
	})
}

func point(id int32, m domain.MonsterID, x, y float64) domain.SpawnPoint {
	return domain.SpawnPoint{ID: id, Monster: m, Pos: domain.Pos{MapID: 14, X: x, Y: y}, Dir: 67}
}

// 场景一建好怪就该在场上, 不是等第一个玩家进来才刷。
// 否则先进图的人会看到空场, 重生节奏也会跟着"谁先进图"漂移。
func TestSceneIsPopulatedAtCreation(t *testing.T) {
	s := sceneWithSpawns(t, spawn.MapDefs{树妖.ID: 树妖}, []domain.SpawnPoint{
		point(1, 树妖.ID, 110, 110), point(2, 树妖.ID, 120, 120), point(3, 树妖.ID, 130, 130),
	})
	if len(s.entities) != 3 {
		t.Fatalf("建场景时就该把 3 个点铺满, 实际 %d 只", len(s.entities))
	}
	if s.Tick() != 0 {
		t.Fatal("刷怪不该消耗逻辑帧")
	}
}

// 进图的玩家要看到已经在场的怪。
func TestPlayerSeesExistingMonsters(t *testing.T) {
	s := sceneWithSpawns(t, spawn.MapDefs{树妖.ID: 树妖}, []domain.SpawnPoint{
		point(1, 树妖.ID, 110, 110), point(2, 树妖.ID, 120, 120),
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()

	evs := sink.take()
	if n := countOf[event.EntitySpawned](evs); n != 2 {
		t.Fatalf("玩家应看到视野里的 2 只怪, 实际 %d 条出场", n)
	}
	sp, _ := firstOf[event.EntitySpawned](evs)
	if sp.Name != "树妖" || sp.Kind != domain.KindMonster {
		t.Errorf("出场的应是树妖, 实际 %+v", sp)
	}
	if sp.HP != 246 || sp.MaxHP != 246 {
		t.Errorf("血量应满, 实际 %d/%d", sp.HP, sp.MaxHP)
	}
}

// 怪物进图一次性全量下发；AOI 格只做服务端查询索引，不是客户端实体边界。
func TestAllMapMonstersSentOnEnter(t *testing.T) {
	s := sceneWithSpawns(t, spawn.MapDefs{树妖.ID: 树妖}, []domain.SpawnPoint{
		point(1, 树妖.ID, 110, 110),   // 同格
		point(2, 树妖.ID, 9000, 9000), // 远在视野外
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()

	if n := countOf[event.EntitySpawned](sink.take()); n != 2 {
		t.Fatalf("应看到全图 2 只怪, 实际 %d", n)
	}
}

// 完整的一轮: 打死 → 尸体停留 → 消失 → 隔一段时间在同一个点重生, 并广播出场。
func TestMonsterRespawnCycle(t *testing.T) {
	s := sceneWithSpawns(t, spawn.MapDefs{树妖.ID: 树妖}, []domain.SpawnPoint{
		point(1, 树妖.ID, 110, 110),
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = domain.Stats{MinAtk: 9999, MaxAtk: 9999, Hit: 1 << 20}
	s.step()

	// 找到那只怪
	var monsterID domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			monsterID = id
		}
	}
	if monsterID == 0 {
		t.Fatal("场上没有怪")
	}
	sink.take()

	// 一击致死
	s.exec(Attack{ID: 1, Target: monsterID})
	s.step()
	if _, ok := firstOf[event.EntityDied](sink.take()); !ok {
		t.Fatal("应打死它")
	}
	if !s.spawner.Occupied(1) {
		t.Fatal("尸体还在场上, 刷怪点不该这么早释放")
	}

	// 尸体停留期
	for i := 0; i < corpseTicks; i++ {
		s.step()
	}
	if _, ok := firstOf[event.EntityDespawned](sink.take()); !ok {
		t.Fatal("尸体到期应消失")
	}
	if s.spawner.Occupied(1) {
		t.Fatal("尸体清掉后刷怪点应释放, 否则这个点永远不再出怪")
	}
	if len(s.entities) != 1 { // 只剩玩家
		t.Fatalf("场上应只剩玩家, 实际 %d 个实体", len(s.entities))
	}

	// 重生间隔里不该有东西冒出来
	for i := 0; i < int(spawn.RespawnNormal)-1; i++ {
		s.step()
	}
	if n := countOf[event.EntitySpawned](sink.take()); n != 0 {
		t.Fatalf("重生时间还没到就出怪了(%d 条)", n)
	}

	// 到点重生, 且要广播 —— 不广播的话这只怪对附近玩家是隐形的
	s.step()
	sp, ok := firstOf[event.EntitySpawned](sink.take())
	if !ok {
		t.Fatal("到点应重生并广播出场")
	}
	if sp.Name != "树妖" || sp.HP != 246 {
		t.Errorf("重生的怪应是满血树妖, 实际 %+v", sp)
	}
	if !sp.Fresh {
		t.Error("重生广播必须带客户端 fresh 位，否则只会突然出现而没有刷新动画")
	}
	if sp.ID == monsterID {
		t.Error("重生后应是新的运行时 id —— 抓包实测同一只怪重生后 0x800f 里的 id 变了")
	}
	if !s.spawner.Occupied(1) {
		t.Error("重生后刷怪点应重新占用")
	}
}

// 场景物件不重生。让 5023 个摆设参与生死轮回没有意义, 还会白烧定时器。
func TestPropsDoNotRespawn(t *testing.T) {
	s := sceneWithSpawns(t, spawn.MapDefs{金星草.ID: 金星草}, []domain.SpawnPoint{
		point(1, 金星草.ID, 110, 110),
	})
	join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = domain.Stats{MinAtk: 9999, MaxAtk: 9999, Hit: 1 << 20}
	s.step()

	var propID domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			propID = id
		}
	}
	s.exec(Attack{ID: 1, Target: propID})
	s.step()

	for i := 0; i < corpseTicks+int(spawn.RespawnNormal)+10; i++ {
		s.step()
	}
	if len(s.entities) != 1 {
		t.Fatalf("摆设被打掉后不该重生, 场上却有 %d 个实体", len(s.entities))
	}
	if s.spawner.Occupied(1) {
		t.Error("点位应保持释放状态")
	}
}

// 没有刷怪器的图(主城)照常工作, 不该崩。
func TestSceneWithoutSpawnerWorks(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000})
	if len(s.entities) != 0 {
		t.Fatal("没有刷怪器就不该有怪")
	}
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	if s.PlayerCount() != 1 {
		t.Fatal("玩家应能正常进图")
	}
}

// 怪被打死之后, 攻击者的交战状态要清掉; 重生出来的是**新实体**,
// 不该被当成同一场交战继续打。
func TestRespawnedMonsterIsANewEngagement(t *testing.T) {
	s := sceneWithSpawns(t, spawn.MapDefs{树妖.ID: 树妖}, []domain.SpawnPoint{
		point(1, 树妖.ID, 110, 110),
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = domain.Stats{MinAtk: 9999, MaxAtk: 9999, Hit: 1 << 20}
	s.step()

	var first domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			first = id
		}
	}
	s.exec(Attack{ID: 1, Target: first})
	s.step()
	sink.take()

	for i := 0; i < corpseTicks+int(spawn.RespawnNormal)+1; i++ {
		s.step()
	}
	var second domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			second = id
		}
	}
	if second == 0 || second == first {
		t.Fatalf("应重生出一只新实体, first=%d second=%d", first, second)
	}
	sink.take()

	// 打新怪应当重新算作交战首击
	s.exec(Attack{ID: 1, Target: second})
	s.step()
	dmg, ok := firstOf[event.DamageDealt](sink.take())
	if !ok {
		t.Fatal("应能打到重生出来的怪")
	}
	if !dmg.Flag.Has(event.DamageOpening) {
		t.Error("打新目标应重新算首击 —— 说明上一场交战的状态没清干净")
	}
}
