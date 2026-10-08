package data

import (
	"context"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 静态配置加载的集成测试。**必须打真库** —— 这些 SQL 唯一会出错的方式就是
// 列名/类型跟表对不上, 而那种错只有真查一次才会暴露, mock 一个 Querier 测不出来。
//
// 默认连接专用测试库，避免并行联调正在修改 fantasy 开发库时让静态数据断言
// 随外部状态漂移。连不上就 Skip(CI 无库时不阻塞)。
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("FANTASY_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:dev@127.0.0.1:55432/fantasy_test?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("连不上 Postgres: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("连不上 Postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestLoadMonsters(t *testing.T) {
	ctx := context.Background()
	defs, err := LoadMonsters(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) < 1000 {
		t.Fatalf("怪物模板只有 %d 种, 太少了 —— 导入可能没跑", len(defs))
	}

	// 龙城南郊的树妖：等级/生命沿用原模板，攻防按当前项目数值定案。
	// L14 标准普通怪的攻击中心为 28，上下限按 80%/120% 四舍五入为 22/34。
	d, ok := defs[1001]
	if !ok {
		t.Fatal("找不到树妖(1001)")
	}
	if d.Name != "树妖" || d.Level != 14 || d.HP != 246 {
		t.Errorf("树妖数值不对: %s L%d %d血", d.Name, d.Level, d.HP)
	}
	if d.Stats.MinAtk != 22 || d.Stats.MaxAtk != 34 || d.Stats.Def != 14 || d.Stats.Hit != 42 {
		t.Errorf("树妖攻防命中不对: 攻%d~%d 防%d 命中%d",
			d.Stats.MinAtk, d.Stats.MaxAtk, d.Stats.Def, d.Stats.Hit)
	}
	if d.Kind != domain.MonsterNormal {
		t.Errorf("树妖应是普通怪, 实际 %v", d.Kind)
	}

	// 不该混进 0 级 / 0 血的占位行 —— 那种怪刷出来生下来就是死的
	for id, d := range defs {
		if d.Level <= 0 || d.HP <= 0 {
			t.Fatalf("模板 %d(%s) 等级 %d 血 %d, 不该被装进来", id, d.Name, d.Level, d.HP)
		}
	}

	// 染色档位必须真的跟着类别走：精英与 BOSS 用原档，普通怪不能落到原档上，
	// 否则普通怪又会掉出 3 条以上的紫/绿/黄装备。
	for id, d := range defs {
		switch d.Kind {
		case domain.MonsterElite, domain.MonsterBoss:
			if d.ColorProfile != domain.DefaultColorProfile {
				t.Fatalf("精英/BOSS %d(%s) 的染色档应为 %d, 实际 %d",
					id, d.Name, domain.DefaultColorProfile, d.ColorProfile)
			}
		case domain.MonsterNormal:
			if d.ColorProfile == domain.DefaultColorProfile {
				t.Fatalf("普通怪 %d(%s) 不该使用精英染色档", id, d.Name)
			}
		}
	}
}

// 染色档位必须按档分组加载：精英/BOSS 原档覆盖 0..5，普通档只到 2 条。
func TestLoadEquipmentRollTableColorProfiles(t *testing.T) {
	ctx := context.Background()
	table, err := LoadEquipmentRollTable(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	if !table.Enabled() {
		t.Fatal("装备随机属性矩阵未就绪")
	}
	if got := len(table.Counts[domain.DefaultColorProfile]); got != 6 {
		t.Fatalf("精英/BOSS 档应为 6 个条数档, 实际 %d", got)
	}
	ordinary := table.Counts[2]
	if len(ordinary) == 0 {
		t.Fatal("普通怪/副本小怪的染色档没有配置")
	}
	for _, option := range ordinary {
		if option.Count > 2 {
			t.Fatalf("普通档配了 %d 条, 上限应是 2 条", option.Count)
		}
	}
}

// 配方表的分层硬约束：gamedata 里只放客户端原始行，我们自己的配方与规则都放业务表。
// 这条测试同时锁住"两边真的被合并加载"—— 漏读业务表会让锋刃卡升阶配方凭空消失，
// 也会让高强化保护道具规则失效。
func TestRecipesBlendBusinessTables(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	items, err := LoadItems(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}

	craft, err := LoadCraftRecipes(ctx, pool, items)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, list := range craft.ByType {
		total += len(list)
	}
	// 客户端原始 4782 条 + 业务补充 12 条
	if total != 4794 {
		t.Fatalf("合成配方应为 4794 条（原始 4782 + 业务 12），实得 %d", total)
	}
	extra := craft.ProductRecipes(0, 7685)
	if len(extra) != 1 || extra[0].RowNo != 4900 {
		t.Fatalf("业务补充配方 4900(→7685) 没被加载: %+v", extra)
	}
	if extra[0].SuccessRate != 85 || extra[0].DisappearRate != 15 {
		t.Fatalf("锋刃卡升阶应是 85%% 成功 / 15%% 材料消失, 实得 %+v", extra[0])
	}

	refines, err := LoadRefineRecipes(ctx, pool, items)
	if err != nil {
		t.Fatal(err)
	}
	withProtect := 0
	for _, r := range refines {
		switch {
		case r.CurrentLevel >= 7 && r.ProtectItem != 4478:
			t.Fatalf("7 级以上应用保护道具 4478, row=%d 实得 %d", r.RowNo, r.ProtectItem)
		case r.CurrentLevel < 7 && r.ProtectItem != 0:
			t.Fatalf("7 级以下不该用保护道具, row=%d 实得 %d", r.RowNo, r.ProtectItem)
		}
		if r.ProtectItem != 0 {
			withProtect++
		}
	}
	// 当前规则是"7 级以上才用保护道具"，与 gamedata 客户端原值无关。
	if withProtect != 1418 {
		t.Fatalf("按业务规则带保护道具的精炼配方应为 1418 条，实得 %d", withProtect)
	}
}

func TestLoadSpawns(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	defs, err := LoadMonsters(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	table, st, err := LoadSpawns(ctx, pool, defs)
	if err != nil {
		t.Fatal(err)
	}

	if st.Total < 40000 {
		t.Fatalf("刷怪点总数只有 %d, 期望 4 万以上", st.Total)
	}
	if st.Loaded+st.NoMonster+st.NoMap != st.Total {
		t.Errorf("统计对不上: 装入%d + 缺模板%d + 缺地图%d ≠ 总数%d",
			st.Loaded, st.NoMonster, st.NoMap, st.Total)
	}
	if st.Loaded < 20000 {
		t.Fatalf("只装进 %d 个点位, 覆盖率过低", st.Loaded)
	}

	// 龙城南郊(mapID=14): 树妖 101 + 白兔 57 + 火鸟蛋 44 = 202, SQL 核对过。
	// 表里其实是 211 个点位, 另外 9 个引用的怪没有模板, 会被过滤掉。
	pts := table[14]
	if len(pts) != 202 {
		t.Fatalf("龙城南郊应有 202 个可用刷怪点, 实际 %d", len(pts))
	}
	byMonster := map[domain.MonsterID]int{}
	for _, p := range pts {
		byMonster[p.Monster]++
		if p.Pos.MapID != 14 {
			t.Errorf("刷怪点的地图 id 应是 14, 实际 %d", p.Pos.MapID)
		}
		if p.Pos.X == 0 && p.Pos.Y == 0 {
			t.Errorf("刷怪点 %d 坐标是原点, 多半没读到 init_pos", p.ID)
		}
		if _, ok := defs[p.Monster]; !ok {
			t.Errorf("刷怪点 %d 引用了没有模板的怪 %d —— 应该已经被过滤掉", p.ID, p.Monster)
		}
	}
	if byMonster[1001] != 101 {
		t.Errorf("树妖应有 101 个点位, 实际 %d", byMonster[1001])
	}
}

// 一个地图文件可以对应**多个地图 id** —— pw23800(荒雷岛) 有 6 个, qw0060(幻想小岛) 有 3 个。
// 那不是脏数据, 是同一张图被多个副本/分线复用。每个 id 都该拿到自己那份刷怪点,
// 所以 LoadSpawns 的行数比 map_monster_spawns 的行数多是**正确的**。
// 钉一个测试, 免得将来有人把这个"重复"当 bug 修掉。
func TestOneMapFileCanServeManyMapIDs(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defs, err := LoadMonsters(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	table, _, err := LoadSpawns(ctx, pool, defs)
	if err != nil {
		t.Fatal(err)
	}
	// 荒雷岛的六个 id 各自都该有怪
	got := 0
	for _, id := range []int32{23800, 23900, 24000, 24100, 24200, 24300} {
		if len(table[id]) > 0 {
			got++
		}
	}
	if got < 2 {
		t.Fatalf("荒雷岛的 6 个地图 id 只有 %d 个拿到了刷怪点 —— "+
			"同一 map_file 服务多个地图 id 的情形被漏掉了", got)
	}
}

func TestLoadTeleports(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	dungeons, err := LoadDungeons(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	table, st, err := LoadTeleports(ctx, pool, dungeons)
	if err != nil {
		t.Fatal(err)
	}

	if st.Total < 900 {
		t.Fatalf("传送门总数只有 %d, 期望 995 左右", st.Total)
	}
	if st.Loaded+st.NoMap+st.NoDest+st.NoArea != st.Total {
		t.Errorf("统计对不上: 装入%d + 无起点%d + 无目标%d + 无区域%d ≠ 总数%d",
			st.Loaded, st.NoMap, st.NoDest, st.NoArea, st.Total)
	}
	if st.Loaded == 0 {
		t.Fatal("一个传送门都没装进来")
	}

	// 龙城(7) 那扇通往南郊(14) 的门: proc_id 9, 落点 (9768,1982), SQL 核对过
	var 南郊 *domain.Portal
	for i, p := range table[7] {
		if p.ProcID == 9 {
			南郊 = &table[7][i]
		}
	}
	if 南郊 == nil {
		t.Fatalf("龙城应有通往南郊的门(proc_id=9), 实际只有 %d 扇门", len(table[7]))
	}
	if 南郊.To.MapID != 14 || 南郊.To.X != 9768 || 南郊.To.Y != 1982 {
		t.Errorf("南郊门的目标不对: %+v", 南郊.To)
	}
	if len(南郊.Area) != 4 {
		t.Errorf("南郊门应是四边形, 实际 %d 个顶点", len(南郊.Area))
	}
	// 站在多边形中心就该踩中
	if !南郊.Hit(domain.Pos{MapID: 7, X: 5442, Y: 6986}) {
		t.Error("站在门中心却没踩中")
	}

	// 装进来的门必须都是可用的: 有区域、目标地图非零
	for mapID, ports := range table {
		for _, p := range ports {
			if len(p.Area) < 3 {
				t.Fatalf("图 %d 的门 %d 没有区域却被装进来了", mapID, p.ProcID)
			}
			if p.To.MapID == 0 {
				t.Fatalf("图 %d 的门 %d 目标地图是 0", mapID, p.ProcID)
			}
		}
	}
}

// ── domain 里那两张硬编码快照 vs 客户端真表 ──
//
// domain.StartingBase / domain.GrowthAt 是从 ov_levelup 抄进代码的快照
// (domain 不能依赖数据库, 所以只能抄)。抄了就有走样的风险,
// 所以在**有库可查的这一层**逐格对回去。这是那两张表唯一的防线。

func TestStartingBaseMatchesOvLevelup(t *testing.T) {
	ctx := context.Background()
	rows, err := testPool(t).Query(ctx, `
		SELECT type, init_str, init_vit, init_int, init_spi, init_agi, init_dex,
		       init_hp, init_sp, point_add
		  FROM gamedata.ov_levelup WHERE prof = 0 ORDER BY type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var typ int32
		var str, vit, intel, spi, agi, dex, hp, sp, pt int64
		if err := rows.Scan(&typ, &str, &vit, &intel, &spi, &agi, &dex, &hp, &sp, &pt); err != nil {
			t.Fatal(err)
		}
		// type = Race + 1, 五个一一对应(见 docs/属性体系.md 的更正说明)
		race := domain.Race(typ - 1)
		got := domain.StartingBase(race)
		want := domain.Base{STR: int32(str), VIT: int32(vit), INT: int32(intel),
			SPI: int32(spi), AGI: int32(agi), DEX: int32(dex)}
		if got != want {
			t.Errorf("职业 %d(ov_levelup.type=%d) 初始六维走样了\n  代码里 %+v\n  表里   %+v",
				race, typ, got, want)
		}
		// ×9 也顺手验一遍
		s := domain.DeriveFromBase(got)
		if int64(s.MaxHP) != hp || int64(s.MaxMP) != sp {
			t.Errorf("职业 %d: 推导出 %d血/%d法, 表里 %d/%d", race, s.MaxHP, s.MaxMP, hp, sp)
		}
		if pt != int64(domain.FreePointsPerLevel) {
			t.Errorf("每级自由点应是 %d, 表里 %d", domain.FreePointsPerLevel, pt)
		}
		seen++
	}
	if seen != 5 {
		t.Fatalf("应查到 5 个职业模板, 实际 %d", seen)
	}
}

func TestGrowthMatchesOvLevelup(t *testing.T) {
	ctx := context.Background()
	rows, err := testPool(t).Query(ctx, `
		SELECT type,
		       str_odd, vit_odd, int_odd, spi_odd, agi_odd, dex_odd,
		       str_even, vit_even, int_even, spi_even, agi_even, dex_even
		  FROM gamedata.ov_levelup WHERE prof = 0 ORDER BY type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var typ int32
		var o [6]int64
		var e [6]int64
		if err := rows.Scan(&typ, &o[0], &o[1], &o[2], &o[3], &o[4], &o[5],
			&e[0], &e[1], &e[2], &e[3], &e[4], &e[5]); err != nil {
			t.Fatal(err)
		}
		race := domain.Race(typ - 1)
		mk := func(v [6]int64) domain.Base {
			return domain.Base{STR: int32(v[0]), VIT: int32(v[1]), INT: int32(v[2]),
				SPI: int32(v[3]), AGI: int32(v[4]), DEX: int32(v[5])}
		}
		// 奇数级用 odd, 偶数级用 even
		if got, want := domain.GrowthAt(race, 3), mk(o); got != want {
			t.Errorf("职业 %d 奇数级成长走样\n  代码里 %+v\n  表里   %+v", race, got, want)
		}
		if got, want := domain.GrowthAt(race, 4), mk(e); got != want {
			t.Errorf("职业 %d 偶数级成长走样\n  代码里 %+v\n  表里   %+v", race, got, want)
		}
		seen++
	}
	if seen != 5 {
		t.Fatalf("应查到 5 个职业模板, 实际 %d", seen)
	}
}

func TestLoadLevels(t *testing.T) {
	ctx := context.Background()
	tb, err := LoadLevels(ctx, testPool(t), domain.LevelCap)
	if err != nil {
		t.Fatal(err)
	}
	if tb.MaxLevel() != domain.LevelCap {
		t.Fatalf("应加载到 %d 级, 实际 %d", domain.LevelCap, tb.MaxLevel())
	}
	// 前几级的真值, SQL 核对过
	for lv, want := range map[int32]int64{1: 30, 2: 110, 3: 270, 4: 546, 5: 875, 6: 1350} {
		if got := tb.Need(lv); got != want {
			t.Errorf("%d 级升级所需经验应是 %d, 实际 %d", lv, want, got)
		}
	}
	// 曲线必须单调递增 —— 出现回落说明读串了列
	for lv := int32(1); lv < domain.LevelCap; lv++ {
		if tb.Need(lv) >= tb.Need(lv+1) {
			t.Fatalf("%d→%d 级的经验需求没有递增: %d vs %d",
				lv, lv+1, tb.Need(lv), tb.Need(lv+1))
		}
	}
	// 不该把飞升后的等级混进来
	if tb.Need(61) != 0 {
		t.Errorf("61 级属于飞升后, 不该加载, 实际 %d", tb.Need(61))
	}
}

// exp_accum 在 1~60 级确实是 exp 的累加, 61 级起变成了 exp 的副本。
// 这条是"为什么只加载到 60 级"的依据, 钉住免得将来有人顺手把上限调到 150。
func TestExpAccumOnlyValidBeforeAscension(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	var mismatch int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM game_levels l
		  LEFT JOIN LATERAL (SELECT sum(exp) total FROM game_levels x WHERE x.level < l.level) s ON true
		 WHERE l.level <= 60 AND l.exp_accum <> COALESCE(s.total, 0)`).Scan(&mismatch); err != nil {
		t.Fatal(err)
	}
	if mismatch != 0 {
		t.Errorf("1~60 级的 exp_accum 应是 exp 的累加, 有 %d 行对不上", mismatch)
	}

	var same int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM game_levels WHERE level > 60 AND exp_accum = exp`).Scan(&same); err != nil {
		t.Fatal(err)
	}
	if same == 0 {
		t.Error("61 级起 exp_accum 应等于 exp(飞升后经验另起一套账) —— 这个前提变了")
	}
}

func TestLoadItems(t *testing.T) {
	ctx := context.Background()
	items, err := LoadItems(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 10000 {
		t.Fatalf("物品模板只有 %d 件, 期望上万", len(items))
	}

	// 树枝(4103): 树妖 31.4% 掉的那个, 可堆叠
	if d, ok := items[4103]; !ok {
		t.Error("找不到树枝(4103)")
	} else if !d.Stackable {
		t.Error("树枝应当可堆叠(ov_item.can_pile)")
	}

	// 装备一律不可堆叠, 且必须带 Equip
	equips, stackableEquips := 0, 0
	for _, d := range items {
		if !d.IsEquip() {
			continue
		}
		equips++
		if d.Stackable {
			stackableEquips++
		}
	}
	if equips < 5000 {
		t.Fatalf("装备只有 %d 件, 期望上千", equips)
	}
	if stackableEquips != 0 {
		t.Fatalf("有 %d 件装备被标成可堆叠 —— 每件耐久不同, 堆起来就分不出谁是谁", stackableEquips)
	}

	for id, want := range map[domain.ItemID]domain.EquipAppearance{
		2875: {Part: domain.AppearanceBody, Model: 143, Known: true},
		2876: {Part: domain.AppearanceBody, Model: 144, Known: true},
		1011: {Part: domain.AppearanceWeaponR, Model: 1, Known: true,
			AtkVariant: 1, WeaponCType: 2, AtkDist: 75, AttackKnown: true},
	} {
		got := items[id]
		if got.Equip == nil || got.Equip.Appearance != want {
			t.Errorf("装备 %d 外观 = %+v, 期望 %+v", id, got.Equip, want)
		}
	}

	// can_pile 是真数据: 两种都该有, 不能全 true 或全 false
	var pile, noPile int
	for _, d := range items {
		if d.IsEquip() {
			continue
		}
		if d.Stackable {
			pile++
		} else {
			noPile++
		}
	}
	if pile == 0 || noPile == 0 {
		t.Errorf("can_pile 应当两种都有, 实际 可堆叠%d / 不可堆叠%d —— 多半是没关联上", pile, noPile)
	}
}

func TestParseEquipAppearanceStrictPrefixes(t *testing.T) {
	tests := []struct {
		name   string
		id     domain.ItemID
		slot   int32
		avatar string
		part   domain.AppearancePart
		model  uint16
		known  bool
	}{
		{"A", 2875, 8, "A143", domain.AppearanceBody, 143, true},
		{"body", 1, 8, "body004", domain.AppearanceBody, 4, true},
		{"M", 1, 8, "M12", domain.AppearanceBody, 12, true},
		{"N", 1, 8, "N12", domain.AppearanceBody, 12, true},
		{"m", 1, 8, "m12", domain.AppearanceBody, 12, true},
		{"cap", 1, 2, "cap001", domain.AppearanceCap, 1, true},
		{"cape", 1, 2, "cape1", domain.AppearanceCap, 1, true},
		{"backpack", 1, 10, "backpack827", domain.AppearanceBackpack, 827, true},
		{"face+Unicode空白", 1, 1, "\u3000face971\u3000", domain.AppearanceFace, 971, true},
		{"weaponr单手槽", 1, 4, "weaponr001", domain.AppearanceWeaponR, 1, true},
		{"盾牌落左手外形槽", 1, 5, "weaponr402", domain.AppearanceWeaponL, 402, true},
		{"双手武器落右手外形槽", 1, 13, "weaponr058", domain.AppearanceWeaponR, 58, true},
		{"旧表face错槽不推广", 1, 10, "face814", 0, 0, false},
		{"部位与槽位必须同时吻合", 1, 1, "backpack827", 0, 0, false},
		{"大小写严格", 1, 8, "a143", 0, 0, false},
		{"必须整串匹配", 1, 8, "A143x", 0, 0, false},
		{"模型零无效", 1, 8, "A0", 0, 0, false},
		{"超过U16", 1, 8, "A65536", 0, 0, false},
		{"非法装备槽", 1, 99, "A143", 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseEquipAppearance(tc.id, tc.slot, tc.avatar, 0, 0, 0, 0)
			if got.Known != tc.known || got.Part != tc.part || got.Model != tc.model {
				t.Fatalf("parse = %+v", got)
			}
		})
	}

	starter := parseEquipAppearance(1011, 4, "weaponr001", 4, 2, 75, 0)
	if !starter.AttackKnown || starter.AtkVariant != 1 || starter.WeaponCType != 2 || starter.AtkDist != 75 {
		t.Fatalf("1011 攻击表现未闭合: %+v", starter)
	}
	other := parseEquipAppearance(1012, 4, "weaponr002", 4, 2, 75, 0)
	if !other.Known || !other.AttackKnown || other.AtkVariant != 1 ||
		other.WeaponCType != 2 || other.AtkDist != 75 {
		t.Fatalf("武器攻击外观应按 type/atk_dist 落位: %+v", other)
	}
}

func TestLoadDrops(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	items, err := LoadItems(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	table, st, err := LoadDrops(ctx, pool, items)
	if err != nil {
		t.Fatal(err)
	}

	if st.Total < 16000 {
		t.Fatalf("掉落表总行数只有 %d, 期望 16887 左右", st.Total)
	}
	if st.Loaded+st.ByKind+st.NoItem+st.BadRate != st.Total {
		t.Errorf("统计对不上: 装入%d + 只给类别%d + 物品不存在%d + 概率非法%d ≠ 总数%d",
			st.Loaded, st.ByKind, st.NoItem, st.BadRate, st.Total)
	}
	// 只给类别那批占了大头(10708 行), 现在做不了, 但必须被计数而不是静默丢掉
	if st.ByKind < 10000 {
		t.Errorf("只给类别的行应有一万多, 实际 %d", st.ByKind)
	}

	// 树妖(1001) 掉树枝(4103), 31.4%
	entries := table[1001]
	if len(entries) == 0 {
		t.Fatal("树妖应该有掉落")
	}
	var 树枝 *domain.DropEntry
	for i := range entries {
		if entries[i].Item == 4103 {
			树枝 = &entries[i]
		}
	}
	if 树枝 == nil {
		t.Fatal("树妖应该掉树枝(4103)")
	}
	if 树枝.RatePct < 31 || 树枝.RatePct > 32 {
		t.Errorf("树枝掉率应约 31.4%%, 实际 %.2f%%", 树枝.RatePct)
	}

	// 装进来的每一条都必须指向真实存在的物品, 且概率合法
	for mid, es := range table {
		for _, e := range es {
			if _, ok := items[e.Item]; !ok {
				t.Fatalf("怪 %d 掉的物品 %d 不存在 —— 应该已经被过滤", mid, e.Item)
			}
			if e.RatePct <= 0 || e.RatePct > 100 {
				t.Fatalf("怪 %d 的掉率 %.4f 不合法", mid, e.RatePct)
			}
		}
	}
}

// 掉率精度: 最低到 0.00045%, 用百分比整数掷会被全部抹成 0(永远不掉)。
func TestDropRatePrecision(t *testing.T) {
	e := domain.DropEntry{Item: 1, RatePct: 0.00045}
	// 用一个确定的"总是返回 0"的随机源: 只要阈值 > 0 就该命中
	if !e.Roll(func(int) int { return 0 }) {
		t.Fatal("0.00045% 的掉率被抹成了 0 —— 精度不够, 那批稀有物永远掉不出来")
	}
	// 返回最大值时不该命中
	if e.Roll(func(n int) int { return n - 1 }) {
		t.Fatal("随机取到最大值时不该命中")
	}
	// 100% 必掉, 0% 必不掉
	if !(domain.DropEntry{Item: 1, RatePct: 100}).Roll(func(n int) int { return n - 1 }) {
		t.Error("100% 应必掉")
	}
	if (domain.DropEntry{Item: 1, RatePct: 0}).Roll(func(int) int { return 0 }) {
		t.Error("0% 应必不掉")
	}
}

// 词条必须挂上去, 而且只挂**有名字的**那批。
//
// 无名字的 352 条(18 种 attr_id, 含 attr_id=0 / 772 / 1405 这种明显不是属性号的,
// 266 条值还是 0)是导入器没解出来的残渣。带名字的 17365 条覆盖 24 种 attr_id,
// 与 domain 里的字典一一对上 —— 所以"有没有名字"就是这份数据自带的质量信号。
func TestLoadAffixes(t *testing.T) {
	ctx := context.Background()
	items, err := LoadItems(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}

	var withAffix, total int
	seen := map[int32]int{}
	for _, d := range items {
		if !d.IsEquip() || len(d.Equip.Affixes) == 0 {
			continue
		}
		withAffix++
		for _, a := range d.Equip.Affixes {
			total++
			seen[a.Attr]++
			if a.Mode != domain.ModeAbsolute && a.Mode != domain.ModePercent {
				t.Fatalf("装备 %d 的词条 mode=%d, 只该有 0/1", d.ID, a.Mode)
			}
		}
	}
	if withAffix < 5000 {
		t.Fatalf("带词条的装备只有 %d 件, 期望 5729 左右 —— 词条多半没挂上", withAffix)
	}
	if total < 17000 {
		t.Fatalf("词条总数只有 %d, 期望 17365 左右", total)
	}

	// 装进来的 attr_id 必须全部是 domain 认识的那 24 种
	known := map[int32]bool{
		domain.AttrSTR: true, domain.AttrINT: true, domain.AttrVIT: true,
		domain.AttrAGI: true, domain.AttrDEX: true, domain.AttrSPI: true,
		domain.AttrDef: true, domain.AttrMAtk: true, domain.AttrMDef: true,
		domain.AttrHit: true, domain.AttrCrit: true, domain.AttrAtkSpeed: true,
		domain.AttrMoveSpeed: true, domain.AttrMinAtk: true, domain.AttrMaxAtk: true,
		domain.AttrMaxHP: true, domain.AttrMaxMP: true, domain.AttrHPRegen: true,
		domain.AttrMaxWeight: true, domain.AttrAtk: true, domain.AttrMCrit: true,
		domain.AttrPhysRes: true, domain.AttrMagicRes: true, domain.AttrStatusRes: true,
	}
	for attr, n := range seen {
		if !known[attr] {
			t.Errorf("装进来了 domain 不认识的 attr_id %d(%d 条) —— "+
				"要么字典漏了一种, 要么无名字的残渣混进来了", attr, n)
		}
	}
	if len(seen) != len(known) {
		t.Errorf("实际用到 %d 种属性, 字典里有 %d 种", len(seen), len(known))
	}

	// 暴击率/抗性这些**只从词条来**的属性必须真的有装备带
	for _, attr := range []int32{domain.AttrCrit, domain.AttrPhysRes, domain.AttrStatusRes} {
		if seen[attr] == 0 {
			t.Errorf("没有任何装备带 attr%d —— 那个属性永远会是零", attr)
		}
	}
}

// 穿戴门槛要从 ov_arm 读出来。9089 件装备全部对得上 id。
func TestEquipRequirementsLoaded(t *testing.T) {
	ctx := context.Background()
	items, err := LoadItems(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	// 钢剑(1002): 6 级, 力量14 敏捷14 (SQL 核对过)
	d, ok := items[1002]
	if !ok || d.Equip == nil {
		t.Fatal("找不到钢剑(1002)")
	}
	need := d.Equip.Need
	if need.Level != 6 || need.Base.STR != 14 || need.Base.AGI != 14 {
		t.Fatalf("钢剑门槛应是 6级/力量14/敏捷14, 实际 %+v", need)
	}

	// 门槛是**属性门槛不是职业限制**: 应该有相当一批装备带六维要求
	var withStatNeed, withSexLimit int
	for _, d := range items {
		if !d.IsEquip() {
			continue
		}
		if d.Equip.Need.Base != (domain.Base{}) {
			withStatNeed++
		}
		if d.Equip.Need.Sex != 0 {
			withSexLimit++
		}
	}
	if withStatNeed < 100 {
		t.Errorf("只有 %d 件装备带六维门槛, 多半没读到 ov_arm", withStatNeed)
	}
	if withSexLimit < 100 {
		t.Errorf("只有 %d 件装备带性别限制, 期望 569 左右", withSexLimit)
	}
}

func TestLoadSkills(t *testing.T) {
	ctx := context.Background()
	tb, st, err := LoadSkills(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}

	// 五个职业 × 30 技能 × 5 级 = 750 行
	if st.Total != 750 {
		t.Fatalf("应读到 750 行(5职业×30技能×5级), 实际 %d", st.Total)
	}
	if st.Loaded != st.Total {
		t.Errorf("应全部装入, 实际 %d/%d", st.Loaded, st.Total)
	}
	usable := st.Loaded - st.Passive - st.NoEffect
	if usable < 250 {
		t.Fatalf("可用技能只有 %d 行, 期望 275 左右 —— desc 解析多半退化了", usable)
	}

	// 火弹术(10401) 一级: 208% 伤害, 31 法力, 距离 450, 火系
	d, ok := tb.Get(10401, 1)
	if !ok {
		t.Fatal("找不到火弹术(10401) 一级")
	}
	if d.Name != "火弹术" || d.Prof != 5 {
		t.Errorf("火弹术应属于术士(prof=5), 实际 %s prof=%d", d.Name, d.Prof)
	}
	if d.Effect.Kind != domain.EffectDamage || d.Effect.DamagePct != 208 {
		t.Errorf("火弹术应是 208%% 伤害, 实际 %+v", d.Effect)
	}
	if d.MPCost != 31 {
		t.Errorf("火弹术应耗 31 法力, 实际 %d —— sp_chg_start 的位解码可能错了", d.MPCost)
	}
	if d.Dist != 450 || d.HurtType != domain.HurtFire {
		t.Errorf("火弹术应是 450 距离的火系技能, 实际 距离%d 类型%d", d.Dist, d.HurtType)
	}

	// 治愈术: 恢复上限10% + 100
	var heal domain.SkillDef
	for _, s := range tb {
		if s.Name == "治愈术" && s.Level == 1 {
			heal = s
		}
	}
	if heal.Effect.Kind != domain.EffectHeal {
		t.Fatalf("治愈术应解析成治疗, 实际 %+v", heal.Effect)
	}
	if heal.Effect.HealPctOfMax != 10 || heal.Effect.HealFlat != 100 {
		t.Errorf("治愈术一级应是 上限10%%+100, 实际 %+v", heal.Effect)
	}
	if heal.Prof != 4 {
		t.Errorf("治愈术应属于药师(prof=4), 实际 %d", heal.Prof)
	}

	// 祝福术复用一枚“强化”状态，但技能文本还明确给出魔攻 +5/10/15/20/25%。
	// 不能为了补数值再塞一枚“增幅”，否则客户端会显示两个状态。
	bless, ok := tb.Get(10307, 1)
	if !ok || len(bless.Statuses) != 1 {
		t.Fatalf("祝福术一级应只有一个状态实例: %+v", bless.Statuses)
	}
	app := bless.Statuses[0]
	if app.ID != 1014 || app.Level != 1 || app.DurationSec != 300 ||
		len(app.ExtraAffixes) != 1 || app.ExtraAffixes[0] != (domain.Affix{
		Attr: domain.AttrMAtk, Value: 5, Mode: domain.ModePercent,
	}) || app.Description != "攻击力上升10%；魔法攻击力上升5%。" {
		t.Fatalf("祝福术一级应为强化 +10%%、魔攻 +5%%，持续 300 秒: %+v", app)
	}

	// 法力消耗随等级递增(连环枪法 10002: 8/11/14/17/20。
	// 10001 是枪法修炼, 那是被动, 不耗法力)
	var prev int32
	for lv := int32(1); lv <= 5; lv++ {
		d, ok := tb.Get(10002, lv)
		if !ok {
			continue
		}
		if lv > 1 && d.MPCost <= prev {
			t.Errorf("连环枪法 %d 级的法力 %d 没有比 %d 级的 %d 高", lv, d.MPCost, lv-1, prev)
		}
		prev = d.MPCost
	}

	// 被动技能不该有法力消耗, 也不该可用
	for _, d := range tb {
		if d.Kind == domain.SkillPassive {
			if d.MPCost != 0 {
				t.Errorf("被动技能 %s 不该耗法力, 实际 %d", d.Name, d.MPCost)
			}
			if d.Usable() {
				t.Errorf("被动技能 %s 不该是可用的主动技能", d.Name)
			}
		}
	}

	// 可用技能的倍率必须落在合理区间 —— 解析出个 0 或者上万就是解错了
	for _, d := range tb {
		if !d.Usable() || d.Effect.Kind != domain.EffectDamage {
			continue
		}
		if d.Effect.DamagePct < 50 || d.Effect.DamagePct > 1000 {
			t.Errorf("%s %d级 倍率 %d%% 不合常理", d.Name, d.Level, d.Effect.DamagePct)
		}
	}
}

// 五个职业都要有可用技能。少了一个说明 prof 映射又出问题了。
func TestEveryProfessionHasUsableSkills(t *testing.T) {
	ctx := context.Background()
	tb, _, err := LoadSkills(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	byProf := map[int32]int{}
	for _, d := range tb {
		if d.Usable() {
			byProf[d.Prof]++
		}
	}
	names := map[int32]string{1: "战士", 2: "剑客", 3: "刺客", 4: "药师", 5: "术士"}
	for p := int32(1); p <= 5; p++ {
		if byProf[p] == 0 {
			t.Errorf("%s(prof=%d) 一个可用技能都没有", names[p], p)
		}
	}
	// 药师应当有治疗技能, 术士应当有火/冰技能 —— 这是 prof 4/5 不搞反的第二道防线
	var healerHeals, warlockElemental int
	for _, d := range tb {
		if d.Prof == 4 && d.Effect.Kind == domain.EffectHeal {
			healerHeals++
		}
		if d.Prof == 5 && (d.HurtType == domain.HurtFire || d.HurtType == domain.HurtIce) {
			warlockElemental++
		}
	}
	if healerHeals == 0 {
		t.Error("药师应当有治疗技能 —— prof 4/5 可能又搞反了")
	}
	if warlockElemental == 0 {
		t.Error("术士应当有火/冰技能 —— prof 4/5 可能又搞反了")
	}
}

func TestLoadStatuses(t *testing.T) {
	ctx := context.Background()
	tb, st, err := LoadStatuses(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	if st.Statuses < 150 {
		t.Fatalf("只装入 %d 个状态, 期望 199 左右", st.Statuses)
	}

	// 逐条与状态自己的说明文字对: 这五条是"效果表解读正确"的全部依据
	cases := []struct {
		id       domain.StatusID
		name     string
		duration int32
		interval int32
		attr     int32
		mode     int32
		value    int32
		why      string
	}{
		{domain.StatusPoison, "中毒", 10, 2, domain.AttrHPRegen, domain.ModeAbsolute, -10,
			"说明「每两秒损失10点生命值」"},
		{domain.StatusWeaken, "衰弱", 60, 0, domain.AttrAtk, domain.ModePercent, -10,
			"说明「攻击力下降10%」"},
		{domain.StatusDampen, "降幅", 60, 0, domain.AttrMAtk, domain.ModePercent, -5,
			"说明「魔法攻击力下降5%」"},
		{domain.StatusBlind, "致盲", 60, 0, domain.AttrHit, domain.ModePercent, -5,
			"说明「命中下降5%」"},
		{domain.StatusSlow, "减速", 60, 0, domain.AttrMoveSpeed, domain.ModePercent, -10,
			"说明「移动速度降低10%」"},
	}
	for _, c := range cases {
		d, ok := tb.Get(c.id, 1)
		if !ok {
			t.Errorf("找不到 %s(%d) 一级", c.name, c.id)
			continue
		}
		if d.Name != c.name {
			t.Errorf("%d 的名字应是 %s, 实际 %s", c.id, c.name, d.Name)
		}
		if d.DurationSec != c.duration || d.IntervalSec != c.interval {
			t.Errorf("%s 应持续 %ds 周期 %ds, 实际 %ds/%ds",
				c.name, c.duration, c.interval, d.DurationSec, d.IntervalSec)
		}
		var found bool
		for _, a := range d.Affixes {
			if a.Attr == c.attr && a.Mode == c.mode && a.Value == c.value {
				found = true
			}
		}
		if !found {
			t.Errorf("%s 应有 [attr%d mode%d %d] (%s), 实际 %+v",
				c.name, c.attr, c.mode, c.value, c.why, d.Affixes)
		}
	}

	// 控制类状态**没有属性效果行** —— 「不能动」不是一个数值变化
	for _, id := range []domain.StatusID{domain.StatusStun, domain.StatusSilence,
		domain.StatusFreeze, domain.StatusPetrify} {
		d, ok := tb.Get(id, 1)
		if !ok {
			t.Errorf("找不到控制状态 %d", id)
			continue
		}
		if d.Control == domain.ControlNone {
			t.Errorf("%s(%d) 应是控制类", d.Name, id)
		}
		if len(d.Affixes) != 0 {
			t.Errorf("%s 是控制类, 不该有属性效果, 实际 %+v", d.Name, d.Affixes)
		}
		if d.DurationSec <= 0 {
			t.Errorf("%s 应有持续时间", d.Name)
		}
	}

	// 装进来的每一条词条都必须是 domain 认识的属性、mode 合法
	for k, d := range tb {
		for _, a := range d.Affixes {
			if a.Mode != domain.ModeAbsolute && a.Mode != domain.ModePercent {
				t.Fatalf("状态 %d 级%d 的 mode=%d 非法 —— 过滤没起作用", k.ID, k.Level, a.Mode)
			}
			if !knownAttr(a.Attr) {
				t.Fatalf("状态 %d 级%d 用了字典外的 attr%d", k.ID, k.Level, a.Attr)
			}
		}
		if d.DurationSec <= 0 {
			t.Errorf("状态 %d 级%d 没有持续时间", k.ID, k.Level)
		}
	}
}

func TestLoadQuests(t *testing.T) {
	ctx := context.Background()
	qt, err := LoadQuests(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(qt) != 366 {
		t.Fatalf("应加载 366 个任务, 实际 %d", len(qt))
	}

	// 蜗壳笛(24): 小叶发布, 1~10 级, 交蜗牛壳(4161)×20 (SQL 核对过)
	d, ok := qt[24]
	if !ok {
		t.Fatal("找不到任务 24")
	}
	if d.Name != "蜗壳笛" || d.NPC != "小叶" {
		t.Errorf("任务 24 应是小叶的蜗壳笛, 实际 %s / %s", d.Name, d.NPC)
	}
	if d.GoalItem != 4161 || d.GoalQty != 20 {
		t.Errorf("应交蜗牛壳(4161)×20, 实际 %d×%d", d.GoalItem, d.GoalQty)
	}
	if d.LevelMin != 1 || d.LevelMax != 10 {
		t.Errorf("等级区间应是 1~10, 实际 %d~%d", d.LevelMin, d.LevelMax)
	}

	// 奖励要真的解出来了。**不能用 game_tasks.exp** —— 那一列全表只有 4 个取值
	var withExp, withMoney, withItems int
	for _, d := range qt {
		if d.Reward.Exp > 0 {
			withExp++
		}
		if !d.Reward.Money.Empty() {
			withMoney++
		}
		if len(d.Reward.Items) > 0 {
			withItems++
		}
	}
	if withExp < 200 {
		t.Fatalf("只有 %d 个任务解出了经验, 期望 225 左右 —— 奖励解析多半退化了", withExp)
	}
	if withMoney < 200 || withItems < 80 {
		t.Errorf("有钱的 %d 个、有物品的 %d 个, 期望 237 / 99", withMoney, withItems)
	}

	// 经验必须来自奖励表而不是 game_tasks.exp: 后者只有 4 个取值
	distinct := map[int64]bool{}
	for _, d := range qt {
		if d.Reward.Exp > 0 {
			distinct[d.Reward.Exp] = true
		}
	}
	if len(distinct) < 20 {
		t.Fatalf("经验只有 %d 种取值 —— 多半读到了 game_tasks.exp 那个按等级段填的占位列",
			len(distinct))
	}

	// 奖励物品必须都指向真实存在的物品
	items, err := LoadItems(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	for id, d := range qt {
		for _, it := range d.Reward.Items {
			if _, ok := items[it.Item]; !ok {
				t.Errorf("任务 %d 的奖励物品 %d 不在物品表里", id, it.Item)
			}
			if it.Qty <= 0 {
				t.Errorf("任务 %d 的奖励物品数量是 %d", id, it.Qty)
			}
		}
		if d.LevelMax > 0 && d.LevelMin > d.LevelMax {
			t.Errorf("任务 %d 的等级区间反了: %d~%d", id, d.LevelMin, d.LevelMax)
		}
	}
}

func TestLoadNPCs(t *testing.T) {
	ctx := context.Background()
	nt, skipped, err := LoadNPCs(ctx, testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for mapID, list := range nt {
		total += len(list)
		for _, n := range list {
			if n.Name == "" {
				t.Errorf("图 %d 有个没名字的 NPC", mapID)
			}
			if n.Pos.MapID != mapID {
				t.Errorf("NPC %s 的坐标地图 id 是 %d, 应是 %d", n.Name, n.Pos.MapID, mapID)
			}
			if n.Pos.X == 0 && n.Pos.Y == 0 {
				t.Errorf("NPC %s 在原点, 多半没读到坐标", n.Name)
			}
		}
	}
	// 当前客户端解包数据共有 859 条可见 NPC，其中 37 条所在地图还没有 map id，
	// 因而可装入 822 条。旧断言写死“981 左右”，已经高于源表本身的总行数。
	if total < 800 {
		t.Fatalf("只装入 %d 个 NPC, 当前资源应有 822 左右", total)
	}
	// 跳过的是"地图文件在 map_defs 里没有 id"那批, 与刷怪点/传送门同一类缺口
	if skipped == 0 {
		t.Error("应有一批 NPC 因为地图无 id 被跳过 —— 那个统计不该是 0")
	}
	// 龙城(7) 该有 NPC, 而且能覆盖到任务发布者
	if len(nt[7]) == 0 {
		t.Fatal("龙城(7) 应该有 NPC")
	}
	qt, _ := LoadQuests(ctx, testPool(t))
	names := map[string]bool{}
	for _, n := range nt[7] {
		names[n.Name] = true
	}
	givers := 0
	for _, d := range qt {
		if names[d.NPC] {
			givers++
		}
	}
	if givers == 0 {
		t.Error("龙城的 NPC 里一个任务发布者都没有 —— 任务与 NPC 按名字对不上了")
	}
}

// TestLoadWorkItems 锁住打工物品产出的加载。
//
// 0022 只落了经验/名誉/钱, 把物品整个漏了。现在钓鱼 20 档与从正式服
// 3,760 次记录恢复的采矿 10 档都必须有物品产出。
func TestLoadWorkItems(t *testing.T) {
	works, _, err := LoadWork(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	var 带物品的 int
	for _, d := range works {
		if len(d.Items) > 0 {
			带物品的++
		}
	}
	if 带物品的 != 30 {
		t.Fatalf("有物品产出的工种 %d 个, 该是钓鱼20档+采矿10档=30", 带物品的)
	}

	// 钓鱼第 10 档(work_id=32): 十条鱼, 权重单选, 概率正好加满 1000
	钓鱼10, ok := works[32]
	if !ok {
		t.Fatal("找不到钓鱼第 10 档(work_id=32)")
	}
	if 钓鱼10.Title != "钓鱼中的" {
		t.Fatalf("work_id=32 是 %q", 钓鱼10.Title)
	}
	if !钓鱼10.WeightedPick {
		t.Fatal("钓鱼该是权重单选 —— 逐条独立掷会钓出 0 条或 3 条")
	}
	if len(钓鱼10.Items) != 10 {
		t.Fatalf("钓鱼第 10 档该有 10 条鱼, 实得 %d", len(钓鱼10.Items))
	}
	if w := 钓鱼10.TotalItemWeight(); w != 1000 {
		t.Fatalf("权重合计 %d, 加不满 1000 会掷出空手", w)
	}
	// 熟练度分档: 等级段十档全是 12~150, 区分维度在 practise
	if 钓鱼10.PractiseMin != 8100 || 钓鱼10.PractiseMax != 10000 {
		t.Fatalf("第 10 档熟练度区间 %d~%d, 该是 8100~10000",
			钓鱼10.PractiseMin, 钓鱼10.PractiseMax)
	}
	if 钓鱼10.LevelMin != 12 || 钓鱼10.LevelMax != 150 {
		t.Fatalf("钓鱼等级段 %d~%d, 该是 12~150", 钓鱼10.LevelMin, 钓鱼10.LevelMax)
	}

	// 钓鱼使用千分权重并必须加满 1000；采矿来自正式服相对权重，铁矿为 2、
	// 其余已解锁矿为 1，因此总权重是品种数+1。
	for _, d := range works {
		if !d.WeightedPick {
			continue
		}
		want := int32(1000)
		if d.WorkType == 10 {
			want = int32(len(d.Items) + 1)
		}
		if d.TotalItemWeight() != want {
			t.Fatalf("%s(id=%d) 是权重单选但合计 %d", d.Title, d.ID, d.TotalItemWeight())
		}
	}
}

// TestMiningUsesRecoveredProductionRules 锁住正式服记录恢复出的十档采矿规则。
// 原客户端 151/151 与空奖励是被清空的服务端数据，不能再当成死内容。
func TestMiningUsesRecoveredProductionRules(t *testing.T) {
	works, _, err := LoadWork(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	wantItems := map[domain.WorkID]int{
		9: 1, 15: 2, 17: 3, 19: 4, 21: 5,
		23: 6, 25: 7, 27: 8, 29: 9, 31: 10,
	}
	for id, itemCount := range wantItems {
		d, ok := works[id]
		if !ok {
			t.Fatalf("找不到采矿档 work_id=%d", id)
		}
		if d.Title != "挖矿中的" || d.WorkType != 10 || d.RequiredSkill != 11007 {
			t.Fatalf("采矿档 %d 身份字段错误: %+v", id, d)
		}
		if d.LevelMin != 10 || d.LevelMax != 150 || !d.Available(60) {
			t.Fatalf("采矿档 %d 等级范围 %d~%d / lv60可用=%v", id,
				d.LevelMin, d.LevelMax, d.Available(60))
		}
		if !d.WeightedPick || len(d.Items) != itemCount || d.TotalItemWeight() != int32(itemCount+1) {
			t.Fatalf("采矿档 %d 应累计解锁 %d 种矿、权重 %d，实得 items=%d weight=%d mode=%v",
				id, itemCount, itemCount+1, len(d.Items), d.TotalItemWeight(), d.WeightedPick)
		}
		if _, ok := d.Requirements.EquipmentOf(domain.WorkRequirementOutfit); !ok {
			t.Fatalf("采矿档 %d 缺矿工服要求", id)
		}
		if _, ok := d.Requirements.EquipmentOf(domain.WorkRequirementTool); !ok {
			t.Fatalf("采矿档 %d 缺采矿工具要求", id)
		}
	}
}

func TestCompleteKnownStatusSemantics(t *testing.T) {
	steel := domain.StatusDef{
		ID: domain.StatusIronSkin, Level: 1,
		Affixes: []domain.Affix{{Attr: domain.AttrPhysRes, Value: 10, Mode: domain.ModePercent}},
	}
	completeKnownStatusSemantics(&steel)
	if steel.PhysicalDamageReductionPct != 10 || len(steel.Affixes) != 0 {
		t.Fatalf("钢铁应成为 10%% 物理减伤而非面板抗性词条: %+v", steel)
	}

	fire := domain.StatusDef{
		ID: domain.StatusFireShield, Level: 2,
		Affixes: []domain.Affix{{Attr: domain.AttrPhysRes, Value: 35, Mode: domain.ModePercent}},
	}
	completeKnownStatusSemantics(&fire)
	if fire.PhysicalDamageReductionPct != 35 || fire.PhysicalReflectPct != 10 {
		t.Fatalf("二级火盾应减伤 35%%、反射 10%%: %+v", fire)
	}

	poison := domain.StatusDef{ID: domain.StatusPoisonHit, Level: 5}
	completeKnownStatusSemantics(&poison)
	if poison.PoisonOnHitChance != 50 || poison.PoisonOnHitLevel != 25 {
		t.Fatalf("五级淬毒应 50%% 施加 25 级中毒: %+v", poison)
	}
}

func TestLoadSpecialStatusMechanics(t *testing.T) {
	statuses, _, err := LoadStatuses(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	get := func(id domain.StatusID) domain.StatusDef {
		d, ok := statuses.Get(id, 1)
		if !ok {
			t.Fatalf("缺少状态 %d/1", id)
		}
		return d
	}
	if d := get(1055); !d.ClearHarmful {
		t.Fatal("咒法·褪应驱散全部不良状态")
	}
	if d := get(1068); d.OutgoingMagicDamageFlat != -230 || d.MagicDamageReductionFlat != 230 {
		t.Fatalf("佛光多宝舍利双向法术修正错误: %+v", d)
	}
	if d := get(1095); d.DamageShield != 1700 {
		t.Fatalf("神光璧应吸收 1700 点，实际 %d", d.DamageShield)
	}
	if d := get(1178); !d.Invulnerable || !d.CannotAttack {
		t.Fatalf("金甲之佑应无敌且不能攻击: %+v", d)
	}
	if d := get(1181); d.MaxHPFloor != 15000 {
		t.Fatalf("提升生命应把生命上限提至 15000: %+v", d)
	}
	if d := get(1208); d.PhysicalDamageReductionPct != 50 {
		t.Fatalf("大地状态应提供 50%% 物理减伤: %+v", d)
	}
	if d := get(1209); d.MagicDamageReductionPct != 50 {
		t.Fatalf("天佑状态应提供 50%% 法术减伤: %+v", d)
	}
	if d := get(1212); d.ExperienceBonusPct != 100 {
		t.Fatalf("双倍经验状态应额外增加 100%%: %+v", d)
	}
}
