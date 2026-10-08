package domain

import "testing"

// game_levels 前几级的真值(SQL 核对过): 30 / 110 / 270 / 546 / 875 / 1350 …
func testLevels() *LevelTable {
	return NewLevelTable(map[int32]int64{
		1: 30, 2: 110, 3: 270, 4: 546, 5: 875, 6: 1350, 7: 2016, 8: 2838,
	})
}

func newChar(race Race, level int32) *Character {
	return &Character{ID: 1, Name: "测试", Race: race, Level: level, Base: StartingBase(race)}
}

func TestLevelTableNeed(t *testing.T) {
	tb := testLevels()
	if got := tb.Need(1); got != 30 {
		t.Errorf("1 级升 2 级应需 30 经验, 实际 %d", got)
	}
	if got := tb.Need(8); got != 2838 {
		t.Errorf("8 级应需 2838, 实际 %d", got)
	}
	if tb.MaxLevel() != 8 {
		t.Errorf("最高等级应是 8, 实际 %d", tb.MaxLevel())
	}
	// 越界不能崩, 也不能返回垃圾
	for _, lv := range []int32{-1, 0, 9, 999} {
		if got := tb.Need(lv); got != 0 {
			t.Errorf("Need(%d) 应返回 0, 实际 %d", lv, got)
		}
	}
	var nilT *LevelTable
	if nilT.Need(1) != 0 || nilT.MaxLevel() != 1 {
		t.Error("nil 表也不能崩")
	}
}

// 经验存的是**当前等级内的进度**, 不是累计总量。
func TestAddExpProgressWithinLevel(t *testing.T) {
	tb, c := testLevels(), newChar(Warrior, 1)

	res := c.AddExp(tb, 10, LevelCap)
	if res.Levels != 0 || c.Level != 1 || c.Exp != 10 {
		t.Fatalf("不够升级时应只涨进度: 等级%d 经验%d 升了%d 级", c.Level, c.Exp, res.Levels)
	}
	res = c.AddExp(tb, 20, LevelCap) // 凑够 30
	if res.Levels != 1 || c.Level != 2 || c.Exp != 0 {
		t.Fatalf("刚好凑够应升一级且进度归零: 等级%d 经验%d", c.Level, c.Exp)
	}
}

// 溢出的经验要带进下一级, 不能丢。
func TestAddExpCarriesOverflow(t *testing.T) {
	tb, c := testLevels(), newChar(Warrior, 1)
	c.AddExp(tb, 50, LevelCap) // 30 升级 + 20 带过去
	if c.Level != 2 || c.Exp != 20 {
		t.Fatalf("应升到 2 级并带 20 点过去, 实际 等级%d 经验%d", c.Level, c.Exp)
	}
}

// 一口气给一大笔经验要能连升好几级。
func TestAddExpMultiLevel(t *testing.T) {
	tb, c := testLevels(), newChar(Warrior, 1)
	// 30+110+270 = 410 正好升到 4 级
	res := c.AddExp(tb, 410, LevelCap)
	if c.Level != 4 || res.Levels != 3 {
		t.Fatalf("410 经验应从 1 级连升到 4 级, 实际 等级%d 升了%d 级", c.Level, res.Levels)
	}
	if c.Exp != 0 {
		t.Errorf("应正好用完, 剩余 %d", c.Exp)
	}
	if res.FromLevel != 1 || res.ToLevel != 4 {
		t.Errorf("结果里的起止等级不对: %d→%d", res.FromLevel, res.ToLevel)
	}
}

// 到了上限就不再累积, 而且零头要清掉 —— 留一条永远满不了的经验条只会让人困惑。
func TestAddExpStopsAtCap(t *testing.T) {
	tb := testLevels()
	c := newChar(Warrior, 5)
	res := c.AddExp(tb, 1_000_000, 5)
	if c.Level != 5 {
		t.Fatalf("已在上限不该再升, 实际 %d 级", c.Level)
	}
	if c.Exp != 0 {
		t.Errorf("上限之后经验应清零, 实际 %d", c.Exp)
	}
	if !res.Overflowed {
		t.Error("应报告溢出")
	}

	// 升到上限的那一次也要清零头
	c2 := newChar(Warrior, 4)
	res2 := c2.AddExp(tb, 999_999, 5)
	if c2.Level != 5 || c2.Exp != 0 || !res2.Overflowed {
		t.Fatalf("升到顶应清零头: 等级%d 经验%d 溢出%v", c2.Level, c2.Exp, res2.Overflowed)
	}
}

func TestAddExpIgnoresNonPositive(t *testing.T) {
	tb, c := testLevels(), newChar(Warrior, 3)
	before := c.Exp
	for _, g := range []int64{0, -5} {
		if res := c.AddExp(tb, g, LevelCap); res.Levels != 0 {
			t.Errorf("给 %d 经验不该升级", g)
		}
	}
	if c.Exp != before {
		t.Errorf("经验被改动了: %d → %d", before, c.Exp)
	}
}

// ── 成长 ──

// 奇偶交替是用来做小数成长的。剑客的精神 odd=1 / even=2, 平均 1.5。
// **必须按等级奇偶取值, 不能取平均** —— 属性是整数, 取平均那半点会永久丢失。
func TestGrowthOddEven(t *testing.T) {
	if odd, even := GrowthAt(Swordsman, 3).SPI, GrowthAt(Swordsman, 4).SPI; odd != 1 || even != 2 {
		t.Fatalf("剑客精神成长应是 奇1/偶2, 实际 奇%d/偶%d", odd, even)
	}
	if odd, even := GrowthAt(Healer, 3).STR, GrowthAt(Healer, 4).STR; odd != 1 || even != 2 {
		t.Fatalf("药师力量成长应是 奇1/偶2, 实际 奇%d/偶%d", odd, even)
	}
	// 战士奇偶相同
	if GrowthAt(Warrior, 3) != GrowthAt(Warrior, 4) {
		t.Error("战士的奇偶成长应当相同")
	}
	// 非法职业不能返回零成长(那样角色永远不长)
	if GrowthAt(Race(200), 2) == (Base{}) {
		t.Error("非法职业应有兜底成长")
	}
}

// 连升多级要**逐级**取奇偶, 不能按级数乘一次。
func TestApplyLevelUpsAccumulatesPerLevel(t *testing.T) {
	c := newChar(Swordsman, 1)
	start := c.Base

	g := c.ApplyLevelUps(1, 3) // 升到 2(偶) 和 3(奇)
	wantSPI := GrowthAt(Swordsman, 2).SPI + GrowthAt(Swordsman, 3).SPI
	if wantSPI != 3 {
		t.Fatalf("测试前提错了: 剑客升 2、3 级的精神应共长 3, 算出 %d", wantSPI)
	}
	if g.Base.SPI != 3 {
		t.Errorf("两级共长精神应是 3(偶2+奇1), 实际 %d —— 按级数乘会得到 2 或 4", g.Base.SPI)
	}
	if c.Base.SPI != start.SPI+3 {
		t.Errorf("成长没加到角色身上: %d → %d", start.SPI, c.Base.SPI)
	}
	if g.FreePoints != 2*FreePointsPerLevel || c.FreePoints != 2*FreePointsPerLevel {
		t.Errorf("两级应给 %d 自由点, 实际 %d", 2*FreePointsPerLevel, c.FreePoints)
	}
}

// 老存档没有六维, 升级时要先补出**升级前那一级应有的**六维再加成长,
// 不能在零值上累加, 也不能只退回一级初始值(那会让高等级角色一直是一级战力)。
func TestApplyLevelUpsOnLegacyCharacter(t *testing.T) {
	c := &Character{ID: 1, Name: "老存档", Race: Assassin, Level: 5}
	// 期望值要在改动**之前**算 —— ApplyLevelUps 会把 c.Base 填上,
	// 之后再调 BaseAtLevel 拿到的是新值, 期望就跟着结果跑了
	want := c.BaseAtLevel(5).Add(GrowthAt(Assassin, 6))
	c.ApplyLevelUps(5, 6)

	if c.Base != want {
		t.Fatalf("应在 5 级应有的六维上加成长\n  实际 %+v\n  期望 %+v", c.Base, want)
	}
	// 6 级的六维必须严格高于 1 级初始值
	if start := StartingBase(Assassin); c.Base.VIT <= start.VIT {
		t.Fatalf("6 级体质 %d 没超过一级初始 %d", c.Base.VIT, start.VIT)
	}
	if DeriveFromBase(c.Base).MaxHP <= 0 {
		t.Error("升完级还是 0 血")
	}
}

// 升级后血量上限必须真的涨 —— 战士体质每级 +5, 就是 +45 血。
func TestLevelUpRaisesMaxHP(t *testing.T) {
	c := newChar(Warrior, 1)
	before := DeriveFromBase(c.Base).MaxHP
	c.ApplyLevelUps(1, 2)
	after := DeriveFromBase(c.Base).MaxHP

	if d := after - before; d != 5*HPPerVIT {
		t.Fatalf("战士升一级应涨 %d 血(体质+5 × 9), 实际涨了 %d", 5*HPPerVIT, d)
	}
}
