package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 取自 game_work 真值(SQL 核对过):
//
//	0 打扫卫生中的  1~10级   36秒/次  58经验  1名誉  10钱(80%)
//	3 贩卖物品中的  20~30级  36秒/次  1827经验 1名誉 45钱(50%)
var (
	打扫卫生 = domain.WorkDef{ID: 0, Title: "打扫卫生中的", LevelMin: 1, LevelMax: 10,
		UnitSec: 36, Exp: 58, Honor: 1, Coin: 10, CoinProb: 800}
	贩卖物品 = domain.WorkDef{ID: 3, Title: "贩卖物品中的", LevelMin: 20, LevelMax: 30,
		UnitSec: 36, Exp: 1827, Honor: 1, Coin: 45, CoinProb: 500}
	// 下面两个是测试专用的宽等级段工种。
	// 真实的 game_work 每个工种都卡在一个 10 级的窗口里(打扫卫生 1~10、贩卖物品 20~30),
	// 而长跑测试要把角色设成 60 级去掉升级噪音 —— 那就必须有一个 60 级还能干的活。
	必给钱的活 = domain.WorkDef{ID: 90, Title: "必给钱", LevelMin: 1, LevelMax: 60,
		UnitSec: 36, Exp: 10, Honor: 1, Coin: 7, CoinProb: 1000}
	八成给钱的活 = domain.WorkDef{ID: 91, Title: "八成给钱", LevelMin: 1, LevelMax: 60,
		UnitSec: 36, Exp: 10, Honor: 1, Coin: 10, CoinProb: 800}
)

func workTable(defs ...domain.WorkDef) domain.WorkTable {
	t := domain.WorkTable{}
	for _, d := range defs {
		t[d.ID] = d
	}
	return t
}

// 固定"今天"，免得测试在跨日那一秒偶发失败。
func fixedDay() func() time.Time {
	t := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func workScene(t *testing.T) (*Scene, *fakeSink) {
	t.Helper()
	defs := spawn.MapDefs{技能怪.ID: 技能怪}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 技能怪.ID, Pos: domain.Pos{MapID: 7, X: 140, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{
		ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 42,
		Works:   workTable(打扫卫生, 贩卖物品, 必给钱的活, 八成给钱的活),
		Stamina: domain.StaminaRule{Max: 100, RefillDaily: true, CostPerMin: 1},
		Now:     fixedDay(), Levels: rewardLevels(),
		Spawner: sp, Defs: defs, Skills: skillTable(火弹术),
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	return s, sink
}

// 开工: 耐力自动回满, 状态广播出去。
func TestStartWork(t *testing.T) {
	s, sink := workScene(t)
	p := s.entities[1]

	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()

	if p.Player.Work == nil {
		t.Fatal("应该开工了")
	}
	if p.Player.Char.Stamina != 100 {
		t.Fatalf("开工时应把耐力补满到 100, 实际 %d", p.Player.Char.Stamina)
	}
	ev, ok := firstOf[event.WorkStarted](sink.take())
	if !ok {
		t.Fatal("开工要广播 —— 周围的人要看到「打扫卫生中的」")
	}
	if ev.Title != "打扫卫生中的" {
		t.Errorf("状态名不对: %s", ev.Title)
	}
}

// 等级不对干不了这活。
func TestWorkLevelBand(t *testing.T) {
	s, sink := workScene(t)
	s.entities[1].Player.Char.Level = 1 // 贩卖物品要 20 级

	s.exec(StartWork{ID: 1, Work: 贩卖物品.ID})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectLevelTooLow {
		t.Fatalf("等级不够应被拒, 实际 %+v", rej)
	}
	if s.entities[1].Player.Work != nil {
		t.Fatal("没开成不该有打工状态")
	}
}

// 已经在打工了不能再开。
func TestStartWorkTwice(t *testing.T) {
	s, sink := workScene(t)
	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()
	sink.take()

	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectAlreadyWorking {
		t.Fatalf("重复开工应被拒, 实际 %+v", rej)
	}
}

// **结算节奏**: 36 秒一次 = 360 帧。
func TestWorkSettlesOnSchedule(t *testing.T) {
	s, sink := workScene(t)
	p := s.entities[1]
	p.Player.Char.Level = 3 // 3 级升 4 级要 270 经验, 58 点不会连升
	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()
	sink.take()

	// 359 帧还没到
	for i := 0; i < 358; i++ {
		s.step()
	}
	if n := countOf[event.WorkSettled](sink.take()); n != 0 {
		t.Fatalf("36 秒没到就结算了 %d 次", n)
	}

	// 第 360 帧结算
	for i := 0; i < 3; i++ {
		s.step()
	}
	evs := sink.take()
	if n := countOf[event.WorkSettled](evs); n != 1 {
		t.Fatalf("应结算 1 次, 实际 %d", n)
	}
	ev, _ := firstOf[event.WorkSettled](evs)
	if ev.Exp != 58 || ev.Honor != 1 {
		t.Errorf("单次收益应是 58 经验 1 名誉, 实际 %d/%d", ev.Exp, ev.Honor)
	}
	if p.Player.Char.Honor != 1 {
		t.Errorf("名誉应到账, 实际 %d", p.Player.Char.Honor)
	}
	if p.Player.Char.Exp != 58 {
		t.Errorf("经验应到账, 实际 %d", p.Player.Char.Exp)
	}
}

// **耐力节奏**: 每分钟 −1 点 = 600 帧一次。
func TestStaminaDrainsPerMinute(t *testing.T) {
	s, _ := workScene(t)
	p := s.entities[1]
	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()
	if p.Player.Char.Stamina != 100 {
		t.Fatal("开工时应满耐力")
	}

	// 一分钟 = 600 帧
	for i := 0; i < 601; i++ {
		s.step()
	}
	if p.Player.Char.Stamina != 99 {
		t.Fatalf("一分钟应扣 1 点耐力, 实际剩 %d", p.Player.Char.Stamina)
	}

	for i := 0; i < 600; i++ {
		s.step()
	}
	if p.Player.Char.Stamina != 98 {
		t.Fatalf("两分钟应扣 2 点, 实际剩 %d", p.Player.Char.Stamina)
	}
}

// **这是整个改动的核心断言**: 100 点耐力正好撑 100 分钟, 期间结算 166 次。
//
// 与原版 500 分钟 ÷ 180 秒 = 166 次 完全一致 —— 总量与单次收益都没变,
// 只是把 8.3 小时压缩进 100 分钟。
func TestWorkLastsExactly100Minutes(t *testing.T) {
	s, sink := workScene(t)
	p := s.entities[1]
	p.Player.Char.Level = 60                 // 顶级, 免得升级事件干扰计数
	s.exec(StartWork{ID: 1, Work: 必给钱的活.ID}) // 打扫卫生只到 10 级, 60 级干不了
	s.step()
	sink.take()

	settled := 0
	stopped := false
	// 100 分钟 = 60000 帧, 多跑一点确认它真的停了
	for i := 0; i < 60_200 && !stopped; i++ {
		s.step()
		for _, ev := range sink.take() {
			switch e := ev.(type) {
			case event.WorkSettled:
				settled++
			case event.WorkStopped:
				stopped = true
				if e.Reason != event.WorkStoppedNoStamina {
					t.Fatalf("应该是耐力耗尽停的, 实际原因 %v", e.Reason)
				}
				if e.Settled != int32(settled) {
					t.Errorf("事件里报的结算次数 %d 与实际 %d 对不上", e.Settled, settled)
				}
			}
		}
	}
	if !stopped {
		t.Fatal("100 分钟到了应该自动收工")
	}
	if p.Player.Char.Stamina != 0 {
		t.Errorf("耐力应耗尽, 实际剩 %d", p.Player.Char.Stamina)
	}
	// 6000 秒 ÷ 36 秒 = 166.67 → 166 次
	if settled != 166 {
		t.Fatalf("100 分钟应结算 166 次(与原版 500 分钟÷180 秒一致), 实际 %d 次", settled)
	}
}

// 钱是按概率给的, 经验和名誉必给 —— 别把经验也做成概率。
func TestCoinIsProbabilisticExpIsNot(t *testing.T) {
	s, sink := workScene(t)
	p := s.entities[1]
	p.Player.Char.Level = 60
	s.exec(StartWork{ID: 1, Work: 必给钱的活.ID}) // CoinProb=1000, 必给
	s.step()
	sink.take()

	for i := 0; i < 361; i++ {
		s.step()
	}
	if p.Player.Char.Money.Copper != 7 {
		t.Fatalf("1000 千分比应必给 7 钱, 实际 %d", p.Player.Char.Money.Copper)
	}

	// 换成 80% 的活, 跑很多次, 应该既不是 0 次也不是每次都给
	s.exec(StopWork{ID: 1})
	s.step()
	p.Player.Char.Money.Copper = 0
	s.exec(StartWork{ID: 1, Work: 八成给钱的活.ID})
	s.step()
	sink.take()

	settled := 0
	for i := 0; i < 360*40; i++ {
		s.step()
		settled += countOf[event.WorkSettled](sink.take())
	}
	got := p.Player.Char.Money.Copper
	if settled == 0 {
		t.Fatal("应该结算了很多次")
	}
	if got == 0 {
		t.Fatal("80%% 概率跑了几十次一次都没给钱")
	}
	if got == int64(settled)*八成给钱的活.Coin {
		t.Fatalf("80%% 概率却每次都给了 —— 概率没生效(结算 %d 次, 拿了 %d 钱)", settled, got)
	}
}

// 普通操作不是 0x1028 EndWork。正式客户端在打工时仍会周期性上报位置，
// 服务端不能把移动、攻击或技能包擅自解释成主动收工。
func TestWorkContinuesAfterAction(t *testing.T) {
	cases := []struct {
		name string
		act  func(s *Scene, mid domain.EntityID)
	}{
		{"移动", func(s *Scene, _ domain.EntityID) {
			s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 200, Y: 200}})
		}},
		{"攻击", func(s *Scene, mid domain.EntityID) {
			s.exec(Attack{ID: 1, Target: mid})
		}},
		{"放技能", func(s *Scene, mid domain.EntityID) {
			s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mid})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, sink := workScene(t)
			p := s.entities[1]
			p.Player.Char.Skills = domain.Learned{火弹术.ID: 1}
			p.MaxMP, p.MP = 1000, 1000
			p.Stats.Hit = 1 << 20
			var mid domain.EntityID
			for id, e := range s.entities {
				if e.Kind == domain.KindMonster {
					mid = id
				}
			}
			s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
			s.step()
			sink.take()

			c.act(s, mid)
			s.step()

			if p.Player.Work == nil {
				t.Fatalf("%s 之后不应该自动收工", c.name)
			}
			if ev, ok := firstOf[event.WorkStopped](sink.take()); ok {
				t.Fatalf("%s 之后不应产生收工事件, 实际 %+v", c.name, ev)
			}
		})
	}
}

// 主动收工。
func TestStopWork(t *testing.T) {
	s, sink := workScene(t)
	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()
	sink.take()

	s.exec(StopWork{ID: 1})
	s.step()
	ev, ok := firstOf[event.WorkStopped](sink.take())
	if !ok || ev.Reason != event.WorkStoppedByPlayer {
		t.Fatalf("主动收工的原因应是 ByPlayer, 实际 %+v", ev)
	}
	if s.entities[1].Player.Work != nil {
		t.Fatal("应该已经收工")
	}
}

// 死了要收工。
func TestWorkStoppedByDeath(t *testing.T) {
	s, sink := workScene(t)
	p := s.entities[1]
	s.exec(StartWork{ID: 1, Work: 打扫卫生.ID})
	s.step()
	sink.take()

	p.HP = 0
	s.step()
	ev, ok := firstOf[event.WorkStopped](sink.take())
	if !ok || ev.Reason != event.WorkStoppedByDeath {
		t.Fatalf("死了应收工, 实际 %+v", ev)
	}
}

// 打工要标脏 —— 涨了经验和钱不落盘, 重启就白打了。
func TestWorkMarksDirty(t *testing.T) {
	s, _ := workScene(t)
	p := s.entities[1]
	p.Player.Char.Level = 60
	s.exec(StartWork{ID: 1, Work: 必给钱的活.ID}) // 打扫卫生只到 10 级
	s.step()
	p.Player.TakeDirty()

	for i := 0; i < 361; i++ {
		s.step()
	}
	if !p.Player.Dirty() {
		t.Fatal("结算了却没标脏 —— 重启就白打了")
	}
}

// 没配打工表的场景不该崩。
func TestNoWorkTableIsSafe(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000})
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	s.exec(StartWork{ID: 1, Work: 0})
	s.exec(StopWork{ID: 1})
	s.step() // 不崩就算过
}

// ── 打工的物品产出 ──
//
// 真值取自 game_work_items：钓鱼第 10 档（practise 8100~10000）十条鱼，
// 概率正好加满 1000。杂务管理合计 1910，是另一种掷法。
var (
	小鱼 = domain.ItemDef{ID: 4401, Name: "小鱼", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
	草鱼 = domain.ItemDef{ID: 4402, Name: "草鱼", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
	鲶鱼 = domain.ItemDef{ID: 4403, Name: "鲶鱼", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}

	// 钓鱼: 权重单选, 三条加满 1000
	钓鱼 = domain.WorkDef{ID: 10, Title: "钓鱼中的", LevelMin: 12, LevelMax: 150,
		UnitSec: 12, WeightedPick: true,
		Items: []domain.WorkItem{
			{Item: 4401, Qty: 1, Prob: 500},
			{Item: 4402, Qty: 1, Prob: 300},
			{Item: 4403, Qty: 1, Prob: 200},
		}}
	// 杂务管理: 逐条独立掷, 合计 1300 > 1000
	杂务 = domain.WorkDef{ID: 11, Title: "杂务管理中的", LevelMin: 1, LevelMax: 60,
		UnitSec: 36, Exp: 5,
		Items: []domain.WorkItem{
			{Item: 4401, Qty: 1, Prob: 1000}, // 必给
			{Item: 4402, Qty: 1, Prob: 300},
		}}
)

func fishScene(t *testing.T) (*Scene, *fakeSink) {
	t.Helper()
	defs := spawn.MapDefs{}
	s := New(Config{
		ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 42,
		Works:   workTable(钓鱼, 杂务),
		Stamina: domain.StaminaRule{Max: 100, RefillDaily: true, CostPerMin: 1},
		Now:     fixedDay(), Levels: rewardLevels(), Defs: defs,
		Items: map[domain.ItemID]domain.ItemDef{4401: 小鱼, 4402: 草鱼, 4403: 鲶鱼},
	})
	sink := join(t, s, 1, 100, "渔夫", 100, 100)
	s.step()
	sink.take()
	return s, sink
}

// 钓鱼是 MVP 内的正经工种(12 级起), 而它**只给物品** ——
// 不发物品的话结算一次什么都没有, 这正是上一轮漏掉的洞。
func TestFishingActuallyYieldsFish(t *testing.T) {
	s, _ := fishScene(t)
	p := s.players[1]
	p.Player.Char.Level = 30

	s.exec(StartWork{ID: 1, Work: 钓鱼.ID})
	for i := 0; i < 12*10*5+10; i++ { // 12 秒一次, 跑够 5 次结算
		s.step()
	}
	total := int32(0)
	for _, id := range []domain.ItemID{4401, 4402, 4403} {
		total += p.Player.Bag.CountOf(id)
	}
	if total == 0 {
		t.Fatal("钓了半天一条鱼都没有 —— 物品产出没接上")
	}
	// 权重单选: 每次结算恰好一条, 所以总数就等于结算次数
	if w := p.Player.Work; w != nil && total != w.Settled {
		t.Fatalf("结算 %d 次却得到 %d 条鱼, 权重单选该是一次一条", w.Settled, total)
	}
}

func TestWeightedPickReturnsExactlyOne(t *testing.T) {
	// 加满 1000 的档, 每掷必中且只中一条
	for x := int32(0); x < 1000; x += 37 {
		got := 钓鱼.RollItems(func(n int32) int32 { return x % n })
		if len(got) != 1 {
			t.Fatalf("取数 %d 时掷出 %d 条, 权重单选必须恰好一条", x, len(got))
		}
	}
	// 落点决定选哪条: 0~499 小鱼, 500~799 草鱼, 800~999 鲶鱼
	for _, c := range []struct {
		roll int32
		want domain.ItemID
	}{{0, 4401}, {499, 4401}, {500, 4402}, {799, 4402}, {800, 4403}, {999, 4403}} {
		got := 钓鱼.RollItems(func(int32) int32 { return c.roll })
		if len(got) != 1 || got[0].Item != c.want {
			t.Fatalf("取数 %d 该选 %d, 实得 %+v", c.roll, c.want, got)
		}
	}
}

func TestIndependentRollsCanGiveMoreThanOne(t *testing.T) {
	// 合计 1300 的档是逐条独立掷, 可能一次拿两件
	got := 杂务.RollItems(func(int32) int32 { return 0 }) // 取数恒 0, 两条都中
	if len(got) != 2 {
		t.Fatalf("独立掷且取数恒 0 时该拿到 2 件, 实得 %d", len(got))
	}
	// 取数恒 999: 只有 1000‰ 那条中(必给), 300‰ 那条不中
	got = 杂务.RollItems(func(int32) int32 { return 999 })
	if len(got) != 1 || got[0].Item != 4401 {
		t.Fatalf("取数恒 999 时该只拿到必给的那件, 实得 %+v", got)
	}
	// 独立掷时 1000‰ 的条目必给, 不能因为合计超 1000 就被摊薄
	空手 := domain.WorkDef{Items: []domain.WorkItem{{Item: 4402, Qty: 1, Prob: 300}}}
	if got := 空手.RollItems(func(int32) int32 { return 999 }); len(got) != 0 {
		t.Fatalf("300‰ 的条目在取数 999 时该不中, 实得 %d 件", len(got))
	}
	if 杂务.TotalItemWeight() != 1300 {
		t.Fatalf("权重合计 %d", 杂务.TotalItemWeight())
	}
}

// 背包满了必须停工。继续挂着只会把产出一件件扔掉,
// 而玩家回来只看到"还在钓鱼", 不知道白挂了两小时。
func TestWorkStopsWhenBagIsFull(t *testing.T) {
	s, sink := fishScene(t)
	p := s.players[1]
	p.Player.Char.Level = 30
	// 把背包塞满: 一格的包, 里面放一件不可叠的东西, 鱼就再也放不进去
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(domain.ItemDef{ID: 9999, Name: "石头", InventoryTab: 1, InventoryTabKnown: true}, 1)

	s.exec(StartWork{ID: 1, Work: 钓鱼.ID})
	sink.take()
	for i := 0; i < 12*10+5; i++ {
		s.step()
	}
	if p.Player.Work != nil {
		t.Fatal("背包满了还在钓 —— 产出会被一件件丢掉")
	}
	found := false
	for _, e := range sink.take() {
		if ws, ok := e.(event.WorkStopped); ok && ws.Reason == event.WorkStoppedBagFull {
			found = true
		}
	}
	if !found {
		t.Fatal("停工了却没说是因为背包满")
	}
}

// 钓鱼靠熟练度分档, 不是靠等级 —— 10 档的等级段全是 12~150。
func TestFishingTiersAreKeyedByPractiseNotLevel(t *testing.T) {
	低档 := domain.WorkDef{PractiseMin: 0, PractiseMax: 99}
	高档 := domain.WorkDef{PractiseMin: 8100, PractiseMax: 10000}
	if !低档.PractiseFits(50) || 低档.PractiseFits(100) {
		t.Fatal("低档的熟练度区间判错")
	}
	if 高档.PractiseFits(50) || !高档.PractiseFits(9000) {
		t.Fatal("高档的熟练度区间判错")
	}
	// 不分档的工种恒真, 别让打扫卫生被熟练度挡住
	if !打扫卫生.PractiseFits(0) || !打扫卫生.PractiseFits(9999) {
		t.Fatal("不分档的工种不该被熟练度挡")
	}
}
