package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 树妖的真实经验值是 58(粉粉兔全量真值)。用它做样本。
var 给经验的树妖 = domain.MonsterDef{
	ID: 1001, Name: "树妖", Kind: domain.MonsterNormal, Level: 14, HP: 246,
	Exp: 58, Sprite: 201, Stats: domain.NewMonsterStats(14, 6, 42, 0, 0, 2000, 110),
}

// game_levels 的真值: 1→2 需 30, 2→3 需 110, 3→4 需 270。
func rewardLevels() *domain.LevelTable {
	return domain.NewLevelTable(map[int32]int64{1: 30, 2: 110, 3: 270, 4: 546, 5: 875})
}

// rewardScene 建一个能结算经验的场景, 里面一只怪。
func rewardScene(t *testing.T, def domain.MonsterDef) (*Scene, domain.EntityID) {
	t.Helper()
	defs := spawn.MapDefs{def.ID: def}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: def.ID, Pos: domain.Pos{MapID: 14, X: 140, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 5,
		Spawner: sp, Levels: rewardLevels(), Defs: defs})
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

// oneShot 让玩家一击必杀。
//
// 只改攻击与命中, **不整块替换 Stats** —— 整块替换会把 MaxHP 抹成 0,
// 造出一个现实中不存在的实体, 测出来的就不是真问题了。
func oneShot(s *Scene) {
	e := s.entities[1]
	e.Stats.MinAtk, e.Stats.MaxAtk, e.Stats.Hit = 99999, 99999, 1<<20
}

// 打死怪要给经验, 而且给的是**模板里的真值**。
func TestKillGrantsExp(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()
	sink.take()
	// 3 级升 4 级要 270 点, 58 点不会触发升级, 好单独看"加经验"这件事
	s.entities[1].Player.Char.Level = 3

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	ev, ok := firstOf[event.ExpGained](sink.take())
	if !ok {
		t.Fatal("打死怪应给经验")
	}
	if ev.Delta != 58 {
		t.Fatalf("树妖的经验是 58, 实际给了 %d", ev.Delta)
	}
	if ev.Who != 1 {
		t.Errorf("经验应给凶手, 实际给了 %d", ev.Who)
	}
	if ch := s.entities[1].Player.Char; ch.Exp != 58 {
		t.Errorf("角色身上的经验应是 58, 实际 %d", ch.Exp)
	}
}

func TestExperienceStatusAppliesAtCentralGrant(t *testing.T) {
	s, _ := rewardScene(t, 给经验的树妖)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.entities[1]
	p.Player.Char.Level = 3
	p.Level = 3
	p.Status.Apply(domain.StatusDef{
		ID: 1212, DurationSec: 900, ExperienceBonusPct: 100,
	}, s.tick, p.ID)
	s.step()
	sink.take()

	s.grantExp(p, 58)
	s.flush()
	ev, ok := firstOf[event.ExpGained](sink.take())
	if !ok || ev.Delta != 116 || p.Player.Char.Exp != 116 {
		t.Fatalf("双倍经验应把 58 变成 116，事件=%+v 总经验=%d", ev, p.Player.Char.Exp)
	}
}

// 经验够了就升级, 而且升级要广播(周围人看得到特效)、属性明细只发本人。
func TestKillLevelsUp(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150) // 旁观者
	oneShot(s)
	s.step()
	sinkA.take()
	sinkB.take()

	ch := s.entities[1].Player.Char
	ch.Exp = 29 // 差 1 点就升级

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	evs := sinkA.take()
	lv, ok := firstOf[event.LevelUp](evs)
	if !ok {
		t.Fatal("经验够了应升级")
	}
	if lv.Level != 2 {
		t.Errorf("应升到 2 级, 实际 %d", lv.Level)
	}
	if ch.Exp != 29+58-30 {
		t.Errorf("溢出的经验应带到下一级, 期望 %d 实际 %d", 29+58-30, ch.Exp)
	}
	if _, ok := firstOf[event.StatsChanged](evs); !ok {
		t.Error("升级后应把新属性发给本人")
	}

	// 旁观者要看到升级(特效), 但不该收到别人的属性明细
	bEvs := sinkB.take()
	if _, ok := firstOf[event.LevelUp](bEvs); !ok {
		t.Error("升级应广播给周围的人")
	}
	if _, ok := firstOf[event.StatsChanged](bEvs); ok {
		t.Error("属性明细不该发给别人")
	}
	if n := countOf[event.ExpGained](bEvs); n != 0 {
		t.Error("经验不该广播给别人")
	}
}

// 升级要把六维成长加上、二级属性重推、血上限真的涨。
func TestLevelUpAppliesGrowth(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()

	p := s.entities[1]
	ch := p.Player.Char
	ch.Race = domain.Warrior
	ch.Base = domain.StartingBase(domain.Warrior)
	p.Stats = domain.DeriveFromBase(ch.Base)
	p.MaxHP, p.HP = p.Stats.MaxHP, p.Stats.MaxHP
	beforeVIT, beforeHP, beforeMax := ch.Base.VIT, p.HP, p.MaxHP
	ch.Exp = 29
	oneShot(s) // 放在重设属性之后

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	if ch.Level != 2 {
		t.Fatalf("应升到 2 级, 实际 %d", ch.Level)
	}
	// 战士每级 +5 体质
	if d := ch.Base.VIT - beforeVIT; d != 5 {
		t.Errorf("战士升一级应长 5 体质, 实际 %d", d)
	}
	if d := p.MaxHP - beforeMax; d != 5*domain.HPPerVIT {
		t.Errorf("血上限应涨 %d, 实际 %d", 5*domain.HPPerVIT, d)
	}
	// 上限涨多少当前血就补多少(服务端定的口径)
	if d := p.HP - beforeHP; d != p.MaxHP-beforeMax {
		t.Errorf("当前血应跟着上限涨同样多, 上限+%d 当前+%d", p.MaxHP-beforeMax, p.HP-beforeHP)
	}
	if p.Level != ch.Level {
		t.Errorf("实体上的等级没跟着角色走: %d vs %d", p.Level, ch.Level)
	}
	if ch.FreePoints != domain.FreePointsPerLevel {
		t.Errorf("升一级应给 %d 自由点, 实际 %d", domain.FreePointsPerLevel, ch.FreePoints)
	}
}

// 血量按上限增量补, **不是回满** —— 否则升级会变成一次免费治疗。
func TestLevelUpDoesNotFullHeal(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()

	p := s.entities[1]
	p.Player.Char.Exp = 29
	p.HP = 1 // 残血

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	if p.HP == p.MaxHP {
		t.Fatal("升级不该回满血 —— 那会让升级变成免费治疗")
	}
	if p.HP <= 1 {
		t.Fatalf("上限涨了, 当前血也该跟着涨一点, 实际 %d", p.HP)
	}
}

// 到了等级上限就不再涨经验。
func TestExpStopsAtLevelCap(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()
	sink.take()

	ch := s.entities[1].Player.Char
	ch.Level = domain.LevelCap

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	if ch.Level != domain.LevelCap {
		t.Fatalf("已在上限不该再升, 实际 %d", ch.Level)
	}
	if ch.Exp != 0 {
		t.Errorf("上限之后经验应保持 0, 实际 %d", ch.Exp)
	}
	if _, ok := firstOf[event.LevelUp](sink.take()); ok {
		t.Error("上限时不该发升级事件")
	}
}

// 没配经验曲线的场景(测试/主城)不该崩, 只是不给经验。
func TestNoLevelTableIsSafe(t *testing.T) {
	defs := spawn.MapDefs{给经验的树妖.ID: 给经验的树妖}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 给经验的树妖.ID, Pos: domain.Pos{MapID: 14, X: 140, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Spawner: sp}) // Levels/Defs 都是 nil

	var mid domain.EntityID
	for eid, e := range s.entities {
		if e.Kind == domain.KindMonster {
			mid = eid
		}
	}
	join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()
	s.exec(Attack{ID: 1, Target: mid})
	s.step() // 不崩就算过

	if ch := s.entities[1].Player.Char; ch.Exp != 0 {
		t.Errorf("没有配置时不该给经验, 实际 %d", ch.Exp)
	}
}

// 凶手已经离场时不能崩, 那笔经验就没人拿。
func TestKillerGoneNoPanic(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()

	s.exec(Attack{ID: 1, Target: mid})
	s.exec(Leave{ID: 1, Reason: "刚好下线"}) // 攻击已排队, 人先走了
	s.step()

	if s.PlayerCount() != 0 {
		t.Fatal("人应该已经离场")
	}
}

// 打怪要标脏 —— 经验涨了不落盘, 重启就白打了。
func TestExpMarksDirty(t *testing.T) {
	sv := newFakeSaver()
	defs := spawn.MapDefs{给经验的树妖.ID: 给经验的树妖}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 给经验的树妖.ID, Pos: domain.Pos{MapID: 14, X: 140, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10, Seed: 5,
		Spawner: sp, Levels: rewardLevels(), Defs: defs, Saver: sv})

	var mid domain.EntityID
	for eid, e := range s.entities {
		if e.Kind == domain.KindMonster {
			mid = eid
		}
	}
	join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	for i := 0; i < 10; i++ { // 先把进图那次脏标记消化掉
		s.step()
	}
	before := sv.count(100)

	s.exec(Attack{ID: 1, Target: mid})
	for i := 0; i < 10; i++ {
		s.step()
	}
	if sv.count(100) <= before {
		t.Fatal("打怪涨了经验却没标脏 —— 重启就白打了")
	}
}
