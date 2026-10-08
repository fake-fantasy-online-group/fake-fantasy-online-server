package data

import (
	"path/filepath"
	"testing"
)

// 列计划驱动的读取必须与解包工具算出的一致 —— 这里锁住几处已用多重手段确证过的字段,
// 任一环节(列计划生成 / 偏移 / GBK 解码)出错都会立刻暴露。
func TestOvRowsKnownFields(t *testing.T) {
	root := filepath.Join("..", "..", "..", "assets", "client")
	plan, err := LoadOvPlan(filepath.Join(root, "ovplan.json"))
	if err != nil {
		t.Skipf("无列计划, 跳过: %v", err)
	}
	zipbin := filepath.Join(root, "pkg", "zipbin")

	cases := []struct {
		table string
		row   int
		col   string
		want  any
	}{
		{"ov_task", 0, "task_id", int64(1)},
		{"ov_task", 0, "desc_", "守卫的委托：消灭甲虫"},
		{"ov_task", 0, "publisher", "守城卫兵阿蒙"},
		{"ov_task", 0, "recom_level", int64(11)},
		{"ov_arm", 0, "name", "铁剑"},
		{"ov_arm", 0, "max_atk", int32(9)},
	}
	for _, c := range cases {
		tp, ok := plan[c.table]
		if !ok {
			t.Errorf("列计划里没有 %s", c.table)
			continue
		}
		cols := tp.Cols
		rows, err := OvRows(zipbin, tp.File, cols)
		if err != nil {
			t.Fatalf("%s: %v", c.table, err)
		}
		idx := -1
		for i, col := range cols {
			if col.Name == c.col {
				idx = i + 1 // +1 因为首列是 row_no
				break
			}
		}
		if idx < 0 {
			t.Errorf("%s 没有列 %s", c.table, c.col)
			continue
		}
		if got := rows[c.row][idx]; got != c.want {
			t.Errorf("%s[%d].%s = %v(%T), 期望 %v(%T)", c.table, c.row, c.col, got, got, c.want, c.want)
		}
	}
}

// 每张表的行数必须等于文件头声明的 rowCount, 且列数与计划一致。
func TestOvRowsShape(t *testing.T) {
	root := filepath.Join("..", "..", "..", "assets", "client")
	plan, err := LoadOvPlan(filepath.Join(root, "ovplan.json"))
	if err != nil {
		t.Skipf("无列计划, 跳过: %v", err)
	}
	zipbin := filepath.Join(root, "pkg", "zipbin")
	var tables, total int
	for tb, tp := range plan {
		cols := tp.Cols
		rows, err := OvRows(zipbin, tp.File, cols)
		if err != nil {
			t.Errorf("%s: %v", tb, err)
			continue
		}
		for i, r := range rows {
			if len(r) != len(cols)+1 {
				t.Fatalf("%s 第%d行 %d 列, 期望 %d", tb, i, len(r), len(cols)+1)
			}
		}
		tables++
		total += len(rows)
	}
	t.Logf("读通 %d 张表 / %d 行", tables, total)
	if tables < 130 {
		t.Errorf("只读通 %d 张表, 期望 >=130", tables)
	}
}

// TestOvRowsSignedness 锁住 1/2 字节列的符号读法。
//
// 来由(2026-08-12): 这里曾有个波及 37 列的 bug —— 2 字节列按 int16 读, 而 id 空间
// 远超 32767(ov_item 到 43226、技能到 35010), 于是大 id 全部变成负数、JOIN 全失效。
// 最坏的 ov_combine.product_item_id 有 1733/4782 行的产物 id 是负的。
// 修法是"默认无符号 + SIGNED8/SIGNED16 白名单", 本测试就是防它回退:
//
//	正向 —— 大 id 必须读成正数
//	反向 —— 白名单里的列必须保住负值(否则 -1 哨兵和负向成长会被读成 65535/+255)
func TestOvRowsSignedness(t *testing.T) {
	root := filepath.Join("..", "..", "..", "assets", "client")
	plan, err := LoadOvPlan(filepath.Join(root, "ovplan.json"))
	if err != nil {
		t.Skipf("无列计划, 跳过: %v", err)
	}
	zipbin := filepath.Join(root, "pkg", "zipbin")

	// 某列在整表里"有多少行为负" —— 用它同时表达两个方向的期望
	countNeg := func(t *testing.T, table, col string) (neg, total int) {
		t.Helper()
		p, ok := plan[table]
		if !ok {
			t.Fatalf("列计划里没有 %s", table)
		}
		idx := -1
		for i, c := range p.Cols {
			if c.Name == col {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("%s 没有列 %s", table, col)
		}
		rows, err := OvRows(zipbin, p.File, p.Cols)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		for _, r := range rows {
			total++
			switch v := r[idx+1].(type) { // +1: 首列是 row_no
			case int16:
				if v < 0 {
					neg++
				}
			case int32:
				if v < 0 {
					neg++
				}
			case int64:
				if v < 0 {
					neg++
				}
			}
		}
		return neg, total
	}

	// 正向: 这些是 id 列, 一个负数都不该有
	for _, c := range []struct{ table, col string }{
		{"ov_combine", "product_item_id"}, // 修复前 1733 行为负
		{"ov_combine", "target_id"},       // 修复前 1733 行为负
		{"ov_card", "item_id"},            // 修复前 1621 行为负
		{"ov_skilldesc", "skill_id"},      // 修复前 453 行为负
		{"ov_refine", "mat_1id"},          // 修复前 140 行为负
		{"ov_enchase", "mat_1id"},         // 修复前 36 行为负
	} {
		neg, total := countNeg(t, c.table, c.col)
		if neg != 0 {
			t.Errorf("%s.%s: %d/%d 行为负 —— 2 字节 id 列被按有符号读了",
				c.table, c.col, neg, total)
		}
	}

	// 反向: 白名单里的列必须保住负值
	for _, c := range []struct {
		table, col string
		wantNeg    int
	}{
		{"ov_arm", "merge_limit", 382},     // 恒 -1 的"无限制"哨兵
		{"ov_petgrow", "str_nick_add", 18}, // 昵称成长的负向修正
		{"ov_petnickgrow", "str_odd", 1},   // 性格成长: 智灵 力-1
	} {
		neg, _ := countNeg(t, c.table, c.col)
		if neg != c.wantNeg {
			t.Errorf("%s.%s: 负值行数 %d, 期望 %d —— 有符号白名单失效了",
				c.table, c.col, neg, c.wantNeg)
		}
	}
}
