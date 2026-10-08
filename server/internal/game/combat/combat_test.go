package combat

import (
	"math"
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 这些测试的作用是把 docs/战斗公式.md 里那些**考据出来的**结论钉住。
// 每一条都标了出处；改动这里之前先去改文档。

// ── 命中率 ──

// 四条玩家实测约束。公式 命中/(命中+防御) 必须同时满足这四条,
// 而且**不需要任何上下限** —— 加了上下限反而会破坏第一条。
func TestHitChanceSatisfiesFieldConstraints(t *testing.T) {
	t.Run("永远不到100%命中", func(t *testing.T) {
		// 出处: 贴吧「不存在100%命中」
		for _, hit := range []int32{100, 1000, 10000, 1 << 20} {
			if c := HitChance(hit, 1); c >= 1 {
				t.Errorf("命中 %d 对防御 1 算出 %.4f, 只要防御>0 就不该到 1", hit, c)
			}
		}
	})

	t.Run("命中约等于防御时恰好50%", func(t *testing.T) {
		// 出处: 贴吧「1000命中对1000防御很难命中」
		if c := HitChance(1000, 1000); math.Abs(c-0.5) > 1e-9 {
			t.Errorf("1000 vs 1000 应是 50%%, 实际 %.4f", c)
		}
	})

	t.Run("边际递减", func(t *testing.T) {
		// 出处: 贴吧「1400 和 600 体感一致」。怪防御按 47 算。
		lo, hi := HitChance(600, 47), HitChance(1400, 47)
		if lo < 0.92 || hi > 0.97 {
			t.Errorf("600→%.3f 1400→%.3f, 期望落在 92.7%%~96.7%%", lo, hi)
		}
		if hi-lo > 0.05 {
			t.Errorf("命中从 600 翻到 1400 只该多 4 个点, 实际多了 %.1f 个点", (hi-lo)*100)
		}
	})

	t.Run("高防御难打", func(t *testing.T) {
		// 出处: 贴吧「打 270 防御的法系要全身灵巧灵」
		if HitChance(300, 270) >= HitChance(300, 47) {
			t.Error("防御涨了命中率却没降")
		}
	})
}

func TestHitChanceEdges(t *testing.T) {
	if HitChance(100, 0) != 1 {
		t.Error("守方没有防御就没有闪避可言, 应必中")
	}
	if HitChance(0, 100) != 0 {
		t.Error("一点命中都没有就该打不中")
	}
}

// ── 步进器 ──

// 步进器的长期命中率必须和 命中/(命中+防御) 一致。
// 这一条是整个设计成立的前提: 换成步进器只改分布, 不改期望。
func TestTrackerMatchesHitChanceLongRun(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cases := []struct{ hit, def int32 }{
		{100, 100}, // 50%
		{300, 100}, // 75%
		{600, 60},  // 91%
		{47, 300},  // 13.5%
	}
	const n = 20000
	for _, c := range cases {
		var tr HitTracker
		tr.Engage(1, c.hit+c.def, rng)
		hits := 0
		for i := 0; i < n; i++ {
			if tr.Roll(c.hit, c.def) {
				hits++
			}
		}
		got := float64(hits) / n
		want := HitChance(c.hit, c.def)
		if math.Abs(got-want) > 0.001 {
			t.Errorf("命中%d 防御%d: 步进器 %.4f vs 公式 %.4f, 期望必须一致",
				c.hit, c.def, got, want)
		}
	}
}

// 50% 时步进器就是精确的「打一下 miss 一下」—— 这是它存在的全部理由。
// 项目方的原话: 「刚开始、包括很长一段练级时光, 都是很稳定的, 打一下 miss 一下」。
func TestTrackerAlternatesAtFiftyPercent(t *testing.T) {
	var tr HitTracker
	tr.Engage(1, 200, nil) // nil rng: 起点固定为 0, 便于看序列

	var seq []bool
	for i := 0; i < 20; i++ {
		seq = append(seq, tr.Roll(100, 100))
	}
	for i := 1; i < len(seq); i++ {
		if seq[i] == seq[i-1] {
			t.Fatalf("50%% 命中率下应严格交替, 第 %d 下和上一下一样: %v", i, seq)
		}
	}
}

// 关键判据: 步进器**几乎不出现连续 miss**(命中率 ≥50% 时)。
// 纯随机在 50% 下会出现 3 连 miss —— 这正是两种模型可以被抓包区分的地方。
func TestTrackerNoConsecutiveMisses(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, c := range []struct{ hit, def int32 }{{100, 100}, {300, 100}, {600, 60}} {
		var tr HitTracker
		tr.Engage(1, c.hit+c.def, rng)
		streak, worst := 0, 0
		for i := 0; i < 5000; i++ {
			if tr.Roll(c.hit, c.def) {
				streak = 0
				continue
			}
			streak++
			if streak > worst {
				worst = streak
			}
		}
		if worst > 1 {
			t.Errorf("命中%d 防御%d: 出现了 %d 连 miss, 命中率≥50%%时步进器不该连续未命中",
				c.hit, c.def, worst)
		}
	}
}

// 换目标要重置, 并且起点是随机的 —— 否则每次开打的命中序列一模一样,
// 玩家能数着拍子躲。
func TestTrackerResetsOnTargetSwitch(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	var tr HitTracker

	if !tr.Engage(10, 200, rng) {
		t.Fatal("第一次锁定目标应报告为首击")
	}
	if tr.Engage(10, 200, rng) {
		t.Fatal("同一目标不该重复报首击")
	}
	if !tr.Engage(20, 200, rng) {
		t.Fatal("换目标应报告为新一次交战的首击")
	}
	if tr.Target() != 20 {
		t.Fatalf("当前目标应是 20, 实际 %d", tr.Target())
	}

	// 起点确实是随机的: 多个不同种子应该给出不止一个起点
	seen := map[int32]bool{}
	for seed := int64(0); seed < 30; seed++ {
		var x HitTracker
		x.Engage(1, 200, rand.New(rand.NewSource(seed)))
		seen[x.acc] = true
	}
	if len(seen) < 5 {
		t.Fatalf("交战起点几乎不变(只有 %d 种), 命中序列会完全可预测", len(seen))
	}
}

func TestTrackerReset(t *testing.T) {
	var tr HitTracker
	tr.Engage(9, 200, nil)
	tr.Roll(100, 100)
	tr.Reset()
	if tr.Target() != 0 || tr.acc != 0 {
		t.Fatal("Reset 后应回到没在打谁的状态")
	}
}

// ── 物理伤害 ──

// 均匀分布, 闭区间。上限必须取得到 —— 少这一格就是系统性削弱。
func TestRollPhysicalIsUniformOverClosedRange(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	counts := map[int32]int{}
	const n = 60000
	for i := 0; i < n; i++ {
		counts[RollPhysical(4, 7, 0, rng)]++
	}
	for v := int32(4); v <= 7; v++ {
		if counts[v] == 0 {
			t.Fatalf("伤害 %d 一次都没出现过 —— 闭区间的端点必须取得到", v)
		}
		if f := float64(counts[v]) / n; math.Abs(f-0.25) > 0.02 {
			t.Errorf("伤害 %d 占比 %.3f, 均匀分布应约 0.25", v, f)
		}
	}
	if len(counts) != 4 {
		t.Fatalf("4~7 应只出现 4 种值, 实际 %d 种: %v", len(counts), counts)
	}
}

// 抓包实测: 我方面板攻击 5, 武器小剑 1~5, 打小蜗牛怪得到 4,4,4,5,6,7 —— 范围 4~7。
// 怪没有武器所以打我恒为 4。浮动来自武器, 不来自属性。
func TestRollPhysicalNoSpreadWhenMinEqualsMax(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for i := 0; i < 100; i++ {
		if d := RollPhysical(4, 4, 0, rng); d != 4 {
			t.Fatalf("上下限相同时应恒为 4(怪没武器就是这样), 实际 %d", d)
		}
	}
}

// 伤害抗性是固定值减法, 且伤害至少 1 点。
func TestRollPhysicalResistIsFlatSubtraction(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	if d := RollPhysical(100, 100, 30, rng); d != 70 {
		t.Fatalf("100 伤害扣 30 抗性应剩 70, 实际 %d", d)
	}
	if d := RollPhysical(10, 10, 9999, rng); d != MinDamage {
		t.Fatalf("抗性再高也要掉 1 点血, 实际 %d —— 否则客户端表现成'打了没反应'", d)
	}
}

// 防御**不参与**伤害计算。这是本作与仙境传说最大的不同,
// 单独钉一个测试, 免得将来有人"顺手"把防御加进减伤。
func TestDefenseDoesNotReduceDamage(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	a := &entity.Entity{ID: 1, Kind: domain.KindPlayer, Level: 10,
		Stats: domain.Stats{MinAtk: 50, MaxAtk: 50, Hit: 1 << 20}}
	mk := func(def int32) *entity.Entity {
		return &entity.Entity{ID: 2, Kind: domain.KindMonster, HP: 100000,
			Stats: domain.Stats{Def: def}}
	}
	var tr1, tr2 HitTracker
	lo := Strike(a, mk(0), &tr1, Blow{}, rng)
	hi := Strike(a, mk(9999), &tr2, Blow{}, rng)
	if lo.Flag.Has(event.DamageMiss) || hi.Flag.Has(event.DamageMiss) {
		t.Skip("这一击 miss 了, 换个种子")
	}
	if lo.Amount != hi.Amount {
		t.Fatalf("防御 0 打出 %d, 防御 9999 打出 %d —— 防御不该影响伤害, 它只影响命不命中",
			lo.Amount, hi.Amount)
	}
}

// ── 魔法 ──

// 魔防减伤的量级: 60 级、四件中位数装备时应落在 22~28%, 与物理侧同量级。
// 这是选定「攻方等级 × 10」而不是 ×1 / ×2 的**主要依据**。
func TestMagicMitigationMagnitude(t *testing.T) {
	// docs/战斗公式.md 检验一那张表给的是**减伤百分比**(60 级 22.2 / 25.5 / 28.2%),
	// 没给对应的魔防值。下面这三个魔防是**从那三个百分比反解出来的**,
	// 不是从装备表查的 —— 它们的作用是钉住公式形状, 不是钉住装备数值。
	for _, c := range []struct {
		level, mdef int32
		lo, hi      float64
	}{
		{60, 171, 0.20, 0.24}, // ≈22.2%
		{60, 205, 0.24, 0.27}, // ≈25.5%
		{60, 236, 0.27, 0.30}, // ≈28.2%
	} {
		got := MagicMitigation(c.level, c.mdef)
		if got < c.lo || got > c.hi {
			t.Errorf("等级%d 魔防%d 减伤 %.3f, 期望 %.2f~%.2f", c.level, c.mdef, got, c.lo, c.hi)
		}
	}
}

// 上限 75%, 且 60 级以内够不着(要 1800 魔防)。
func TestMagicMitigationCap(t *testing.T) {
	if got := MagicMitigation(60, 1_000_000); got != MaxMagicMitigation {
		t.Fatalf("减伤应封顶在 75%%, 实际 %.4f", got)
	}
	// 触到 75% 需要 魔防 = 3 × 等级 × 10
	if got := MagicMitigation(60, 1800); math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("60 级 1800 魔防应正好触到 75%%, 实际 %.4f", got)
	}
	if MagicMitigation(60, 1799) >= MaxMagicMitigation {
		t.Fatal("1799 魔防不该已经封顶 —— 60 级以内本就够不着")
	}
}

func TestMagicMitigationEdges(t *testing.T) {
	if MagicMitigation(60, 0) != 0 {
		t.Error("没有魔防就没有减伤")
	}
	if got := MagicMitigation(0, 100); got > MaxMagicMitigation {
		t.Errorf("等级 0 会让分母塌掉, 必须兜住, 实际 %.4f", got)
	}
}

// 魔法侧两条减伤线是独立的: 魔防按比例, 魔抗按固定值。
func TestRollMagicTwoIndependentReductions(t *testing.T) {
	// 魔攻 100, 100% 倍率, 60 级打魔防 600 → 减伤 600/(600+600) = 50% → 50
	if d := RollMagic(100, 100, 60, 600, 0); d != 50 {
		t.Fatalf("按比例减伤应剩 50, 实际 %d", d)
	}
	// 再扣 20 点魔法抗性(固定值)
	if d := RollMagic(100, 100, 60, 600, 20); d != 30 {
		t.Fatalf("再扣 20 点固定魔抗应剩 30, 实际 %d", d)
	}
	// 技能倍率是乘法: 130% 伤害
	if d := RollMagic(100, 130, 60, 600, 0); d != 65 {
		t.Fatalf("130%% 倍率应打 65, 实际 %d", d)
	}
	// 没写倍率按 100% 算
	if RollMagic(100, 0, 60, 600, 0) != RollMagic(100, 100, 60, 600, 0) {
		t.Error("倍率为 0 应当作 100%")
	}
	if d := RollMagic(10, 100, 60, 600, 9999); d != MinDamage {
		t.Fatalf("魔抗再高也要掉 1 点, 实际 %d", d)
	}
}

// ── 暴击 ──

// 暴击率的单位是**万分比**(attr23 口径), 别当成百分比。
func TestCritRateIsBasisPoints(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	const n = 40000
	hits := 0
	for i := 0; i < n; i++ {
		if Crit(500, rng) { // 500 万分比 = 5%
			hits++
		}
	}
	if f := float64(hits) / n; math.Abs(f-0.05) > 0.005 {
		t.Fatalf("500(万分比) 应是 5%%, 实际 %.4f —— 当成百分比就会变成 500%%", f)
	}
	if Crit(0, rng) {
		t.Error("0 暴击率不该暴")
	}
	if !Crit(critScale, rng) {
		t.Error("10000 万分比 = 100%, 应必暴")
	}
}

// ── 整体结算 ──

func mkAttacker(id domain.EntityID, s domain.Stats) *entity.Entity {
	return &entity.Entity{ID: id, Kind: domain.KindPlayer, Level: 10, Stats: s,
		HP: 1000, MaxHP: 1000, Player: &entity.Player{}}
}

func mkTarget(id domain.EntityID, hp int32, s domain.Stats) *entity.Entity {
	return &entity.Entity{ID: id, Kind: domain.KindMonster, HP: hp, MaxHP: hp, Stats: s,
		Monster: &entity.Monster{}}
}

// 未命中的伤害必须是 0, 且血量不变。抓包 22/22 如此。
func TestStrikeMissDealsNothing(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	a := mkAttacker(1, domain.Stats{MinAtk: 10, MaxAtk: 10, Hit: 0}) // 命中 0 = 必 miss
	d := mkTarget(2, 100, domain.Stats{Def: 100})
	var tr HitTracker

	ev := Strike(a, d, &tr, Blow{}, rng)
	if !ev.Flag.Has(event.DamageMiss) {
		t.Fatal("命中 0 打防御 100 必然 miss")
	}
	if ev.Amount != 0 {
		t.Fatalf("未命中的伤害必须是 0, 实际 %d", ev.Amount)
	}
	if d.HP != 100 {
		t.Fatalf("miss 不该扣血, 实际剩 %d", d.HP)
	}
}

// 暴击是「必中」的三种例外之一 —— 出处「除非你暴击不然全MISS」。
// 所以暴击的掷点必须排在命中判定**前面**。
func TestCritAlwaysHitsEvenWithZeroAccuracy(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	a := mkAttacker(1, domain.Stats{MinAtk: 10, MaxAtk: 10, Hit: 0, CritRate: critScale})
	d := mkTarget(2, 1000, domain.Stats{Def: 1 << 20})
	var tr HitTracker

	ev := Strike(a, d, &tr, Blow{}, rng)
	if ev.Flag.Has(event.DamageMiss) {
		t.Fatal("暴击必中, 命中 0 也一样")
	}
	if !ev.Flag.Has(event.DamageCrit) {
		t.Fatal("暴击标志没打上")
	}
	if ev.Amount != 20 {
		t.Fatalf("暴击固定 2 倍: 10 应打 20, 实际 %d", ev.Amount)
	}
}

// 「无视防御」词缀 = 绝对命中率。这条恰好是「防御=闪避」最硬的一条反证:
// 如果防御是减伤, 这个词缀就该叫「无视减伤」。
func TestIgnoreDefenseAlwaysHits(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	a := mkAttacker(1, domain.Stats{MinAtk: 10, MaxAtk: 10, Hit: 1})
	d := mkTarget(2, 1000, domain.Stats{Def: 1 << 20})
	var tr HitTracker

	ev := Strike(a, d, &tr, Blow{IgnoreDefense: true}, rng)
	if ev.Flag.Has(event.DamageMiss) {
		t.Fatal("「无视防御」触发时应绝对命中")
	}
}

// 法系技能 100% 命中(法杖平砍算物理, 照样 miss)。
func TestMagicSkillNeverMisses(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	a := mkAttacker(1, domain.Stats{MAtk: 100, Hit: 0})
	d := mkTarget(2, 1000, domain.Stats{Def: 1 << 20, MDef: 0})
	var tr HitTracker

	ev := Strike(a, d, &tr, Blow{Magic: true, SkillPct: 100}, rng)
	if ev.Flag.Has(event.DamageMiss) {
		t.Fatal("法系技能不判命中")
	}
	if !ev.Flag.Has(event.DamageMagic) {
		t.Fatal("法术伤害标志没打上")
	}
	if ev.Amount != 100 {
		t.Fatalf("魔攻 100、无魔防, 应打 100, 实际 %d", ev.Amount)
	}
}

// 致死: 血归零, 打上致死标志, 且伤害可以大于剩余血(溢出斩杀)。
// 抓包实测最后一击伤害 7 > 剩余 4。
func TestStrikeFatalOverkill(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	a := mkAttacker(1, domain.Stats{MinAtk: 7, MaxAtk: 7, Hit: 1 << 20})
	d := mkTarget(2, 4, domain.Stats{})
	var tr HitTracker

	ev := Strike(a, d, &tr, Blow{}, rng)
	if !ev.Flag.Has(event.DamageFatal) {
		t.Fatal("血被打光应打上致死标志")
	}
	if ev.Amount != 7 {
		t.Fatalf("伤害应是完整的 7(不被剩余血截断), 实际 %d", ev.Amount)
	}
	if ev.DstHP != 0 {
		t.Fatalf("剩余血应归 0 不能为负, 实际 %d", ev.DstHP)
	}
	if d.HP != 0 {
		t.Fatalf("实体血量也应归 0, 实际 %d", d.HP)
	}
}

// 交战第一击带首击标志, 后续不带。客户端靠它起战斗姿态。
func TestStrikeOpeningFlagOnlyOnFirstBlow(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	a := mkAttacker(1, domain.Stats{MinAtk: 1, MaxAtk: 1, Hit: 1 << 20})
	d := mkTarget(2, 1000, domain.Stats{})
	var tr HitTracker

	if !Strike(a, d, &tr, Blow{}, rng).Flag.Has(event.DamageOpening) {
		t.Fatal("第一击应带首击标志")
	}
	for i := 0; i < 5; i++ {
		if Strike(a, d, &tr, Blow{}, rng).Flag.Has(event.DamageOpening) {
			t.Fatalf("第 %d 击不该再带首击标志", i+2)
		}
	}
}

// 同样的种子 + 同样的输入 = 同样的结果。出了「这一刀怎么打出这个数」的问题能重放。
func TestStrikeIsReproducible(t *testing.T) {
	run := func() []int32 {
		rng := rand.New(rand.NewSource(99))
		a := mkAttacker(1, domain.Stats{MinAtk: 4, MaxAtk: 7, Hit: 100, CritRate: 1000})
		d := mkTarget(2, 1_000_000, domain.Stats{Def: 100})
		var tr HitTracker
		var out []int32
		for i := 0; i < 50; i++ {
			out = append(out, Strike(a, d, &tr, Blow{}, rng).Amount)
		}
		return out
	}
	x, y := run(), run()
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("同种子第 %d 击结果不同: %d vs %d —— 战斗必须可复现", i, x[i], y[i])
		}
	}
}
