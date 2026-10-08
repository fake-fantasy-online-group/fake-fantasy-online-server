package domain

import "testing"

// 定案参数（用户拍板，与 game_level_penalty 默认行一致）：
// 经验：怪高 ≤9 级满；之后每级 −8%；怪低 ≥1 级即 −8%/级，均到 0。
// 掉率：|级差| ≤5 满；之后每级 −8%；50% 封底。
func testPenalty() LevelPenalty {
	return LevelPenalty{
		ExpFullAbove:  9,
		ExpStepBP:     800,
		ExpBelowStart: 1,
		DropFull:      5,
		DropStepBP:    800,
		DropFloorBP:   5000,
	}
}

func TestLevelPenalty_ExpMultBP(t *testing.T) {
	p := testPenalty()
	cases := []struct {
		name string
		pl   int32 // 玩家等级
		ml   int32 // 怪物等级
		want int32
	}{
		{"同等级", 30, 30, 10000},
		{"黄档怪高2", 30, 32, 10000},
		{"红档上限怪高9", 30, 39, 10000},
		{"红档怪高3", 30, 33, 10000},
		{"紫档起点怪高10", 30, 40, 9200},
		{"紫档怪高15", 30, 45, 5200},
		{"紫档怪高20", 30, 50, 1200},
		{"紫档怪高21", 30, 51, 400},
		{"紫档怪高22归零", 30, 52, 0},
		{"紫档怪高30归零", 30, 60, 0},
		{"绿档起点怪低1", 30, 29, 9200},
		{"绿档怪低5", 30, 25, 6000},
		{"绿档怪低9", 30, 21, 2800},
		{"灰档怪低10", 30, 20, 2000},
		{"灰档怪低12", 30, 18, 400},
		{"灰档怪低13归零", 30, 17, 0},
		{"灰档怪低20归零", 30, 10, 0},
	}
	for _, c := range cases {
		if got := p.ExpMultBP(c.pl, c.ml); got != c.want {
			t.Errorf("%s: ExpMultBP(%d,%d) = %d, want %d", c.name, c.pl, c.ml, got, c.want)
		}
	}
}

func TestLevelPenalty_DropMultBP(t *testing.T) {
	p := testPenalty()
	cases := []struct {
		name string
		pl   int32
		ml   int32
		want int32
	}{
		{"同等级", 30, 30, 10000},
		{"怪高2", 30, 32, 10000},
		{"怪高5满区间上限", 30, 35, 10000},
		{"怪低5满区间下限", 30, 25, 10000},
		{"怪高6", 30, 36, 9200},
		{"怪低6", 30, 24, 9200},
		{"怪高8", 30, 38, 7600},
		{"怪高11", 30, 41, 5200},
		{"怪高12封底", 30, 42, 5000},
		{"怪低12封底", 30, 18, 5000},
		{"怪低20封底", 30, 10, 5000},
		{"怪高30封底", 30, 60, 5000},
	}
	for _, c := range cases {
		if got := p.DropMultBP(c.pl, c.ml); got != c.want {
			t.Errorf("%s: DropMultBP(%d,%d) = %d, want %d", c.name, c.pl, c.ml, got, c.want)
		}
	}
}

func TestLevelPenalty_ZeroValueDisabled(t *testing.T) {
	var p LevelPenalty
	if p.Enabled() {
		t.Error("零值 LevelPenalty 应视为未启用")
	}
	// 未启用时倍率恒为满，不因零值字段出现除零或归零。
	if got := p.ExpMultBP(30, 60); got != 10000 {
		t.Errorf("零值 ExpMultBP = %d, want 10000", got)
	}
	if got := p.DropMultBP(30, 60); got != 10000 {
		t.Errorf("零值 DropMultBP = %d, want 10000", got)
	}
}
