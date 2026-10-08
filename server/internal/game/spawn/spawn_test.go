package spawn

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 全部取自 game_monsters 的真实行(SQL 核对过), 免得测试里的数字和线上对不上。
var (
	树妖 = domain.MonsterDef{ // 龙城南郊主力, 101 个点位
		ID: 1001, Name: "树妖", Kind: domain.MonsterNormal, Level: 14, HP: 246,
		Exp: 100, Sprite: 201,
		Stats:    domain.NewMonsterStats(14, 6, 42, 0, 0, 2000, 100),
		ViewDist: 300, TraceDist: 600,
	}
	刑天 = domain.MonsterDef{ // 31 级 BOSS
		ID: 1011, Name: "刑天", Kind: domain.MonsterBoss, Level: 31, HP: 41000,
		Sprite: 211,
		Stats:  domain.NewMonsterStats(198, 16, 93, 0, 0, 1800, 120),
	}
	金星草 = domain.MonsterDef{ // 场景物件, 全服 5023 个点位里的一种
		ID: 2074, Name: "金星草", Kind: domain.MonsterProp, Level: 1, HP: 8,
		Sprite: 572,
		Stats:  domain.NewMonsterStats(2, 1, 3, 0, 0, 1500, 0),
	}
)

func defs() MapDefs {
	return MapDefs{树妖.ID: 树妖, 刑天.ID: 刑天, 金星草.ID: 金星草}
}

func pts(n int, m domain.MonsterID) []domain.SpawnPoint {
	out := make([]domain.SpawnPoint, n)
	for i := range out {
		out[i] = domain.SpawnPoint{
			ID: int32(i + 1), Monster: m,
			Pos: domain.Pos{MapID: 14, X: float64(1000 + i*50), Y: 2000},
			Dir: 67, // 45416/45635 个刷怪点都是这个值
		}
	}
	return out
}

func newSpawner(t *testing.T, points []domain.SpawnPoint) *Spawner {
	t.Helper()
	s, _ := New(points, defs(), domain.NewEntityAlloc())
	return s
}

// 开场把所有点铺满, 每个点一只。
func TestInitialFillsEveryPoint(t *testing.T) {
	s := newSpawner(t, pts(101, 树妖.ID)) // 南郊真实的树妖点位数
	got := s.Initial()
	if len(got) != 101 {
		t.Fatalf("101 个刷怪点应出 101 只怪, 实际 %d", len(got))
	}
	seen := map[domain.EntityID]bool{}
	for _, e := range got {
		if seen[e.ID] {
			t.Fatalf("实体 id %d 重复", e.ID)
		}
		seen[e.ID] = true
		if e.ID.SegKind() != domain.KindMonster {
			t.Errorf("怪的 id %d 不在怪物段里", e.ID)
		}
	}
}

// 铺满之后再铺一次, 不该重复出怪 —— 场景重建时会再调一次 Initial。
func TestInitialIsIdempotent(t *testing.T) {
	s := newSpawner(t, pts(10, 树妖.ID))
	s.Initial()
	if again := s.Initial(); len(again) != 0 {
		t.Fatalf("点位都占着, 不该再出怪, 实际又出了 %d 只", len(again))
	}
}

// 造出来的怪必须和模板一致 —— 这是"配置 → 实体"这一步唯一会出错的地方。
func TestSpawnedEntityMatchesDef(t *testing.T) {
	s := newSpawner(t, pts(1, 树妖.ID))
	e := s.Spawn(1)
	if e == nil {
		t.Fatal("没出怪")
	}
	if e.Name != "树妖" || e.Level != 14 {
		t.Errorf("名字/等级不对: %s L%d", e.Name, e.Level)
	}
	if e.HP != 246 || e.MaxHP != 246 {
		t.Errorf("血量应是满的 246/246, 实际 %d/%d", e.HP, e.MaxHP)
	}
	if e.Look.ModelID != 201 {
		t.Errorf("模型号应透传给出场包, 实际 %d", e.Look.ModelID)
	}
	if e.Kind != domain.KindMonster || e.Monster == nil {
		t.Fatal("应当是怪物实体")
	}
	if e.Monster.TypeID != 1001 || e.Monster.SpawnID != 1 {
		t.Errorf("模板 id / 刷怪点 id 不对: %+v", e.Monster)
	}
	if e.Monster.Home != e.Pos {
		t.Error("出生点应记成家, 追人追远了要回来")
	}
	if !e.Alive() {
		t.Error("刚刷出来的怪应该是活的")
	}
}

// 怪的攻击**没有上下限浮动** —— 浮动来自武器, 怪没有武器。
// 抓包印证: 小蜗牛怪打我恒为 4。
func TestMonsterHasNoWeaponSpread(t *testing.T) {
	s := newSpawner(t, pts(1, 树妖.ID))
	e := s.Spawn(1)
	if e.Stats.MinAtk != e.Stats.MaxAtk {
		t.Fatalf("怪的攻击不该有区间, 实际 %d~%d", e.Stats.MinAtk, e.Stats.MaxAtk)
	}
	if e.Stats.MinAtk != 14 {
		t.Fatalf("树妖攻击应是 14, 实际 %d", e.Stats.MinAtk)
	}
	if e.Stats.Hit != 42 || e.Stats.Def != 6 {
		t.Errorf("命中/防御应透传, 实际 命中%d 防御%d", e.Stats.Hit, e.Stats.Def)
	}
}

// 引用不到模板的刷怪点要被丢掉**并计数** ——
// 静默跳过会变成"这张图怎么没怪", 而且完全查不出原因。
func TestUnknownMonsterIsSkippedAndCounted(t *testing.T) {
	points := append(pts(3, 树妖.ID),
		domain.SpawnPoint{ID: 90, Monster: 99999},
		domain.SpawnPoint{ID: 91, Monster: 88888})
	s, skipped := New(points, defs(), domain.NewEntityAlloc())
	if skipped != 2 {
		t.Fatalf("应报告丢了 2 个点位, 实际 %d", skipped)
	}
	if s.Count() != 3 {
		t.Fatalf("应只剩 3 个有效点位, 实际 %d", s.Count())
	}
	if len(s.Initial()) != 3 {
		t.Fatal("只该出 3 只怪")
	}
}

// 占用与释放。死了要 Release, 否则那个点永远不再出怪。
func TestOccupyAndRelease(t *testing.T) {
	s := newSpawner(t, pts(1, 树妖.ID))

	if s.Occupied(1) {
		t.Fatal("还没刷就被占了")
	}
	s.Spawn(1)
	if !s.Occupied(1) {
		t.Fatal("刷了之后应标记为占用")
	}
	if s.Spawn(1) != nil {
		t.Fatal("占着的点不该再出怪 —— 否则一个点会堆出无数只")
	}

	s.Release(1)
	if s.Occupied(1) {
		t.Fatal("释放后不该还占着")
	}
	if s.Spawn(1) == nil {
		t.Fatal("释放后应能再出怪")
	}
}

func TestSpawnUnknownPoint(t *testing.T) {
	s := newSpawner(t, pts(1, 树妖.ID))
	if s.Spawn(999) != nil {
		t.Fatal("不存在的刷怪点不该出怪")
	}
}

// 重生间隔按类别分档, 且**摆设与采集物不重生**。
// 一张图里 5023 个点位是场景物件, 让它们参与生死轮回没有意义。
func TestRespawnTicksByKind(t *testing.T) {
	cases := []struct {
		kind domain.MonsterKind
		want domain.Tick
		why  string
	}{
		{domain.MonsterNormal, RespawnNormal, "普通怪 15 秒"},
		{domain.MonsterElite, RespawnElite, "精英 1 分钟"},
		{domain.MonsterBoss, RespawnBoss, "BOSS 30 分钟"},
		{domain.MonsterProp, 0, "场景物件不重生"},
		{domain.MonsterGather, 0, "采集物不重生"},
	}
	for _, c := range cases {
		if got := RespawnTicks(c.kind); got != c.want {
			t.Errorf("%s: 期望 %d 帧, 实际 %d", c.why, c.want, got)
		}
	}
	// 间隔用帧表达, 换算回秒要对得上
	if RespawnNormal.Millis() != 15_000 {
		t.Errorf("普通怪重生应是 15 秒, 实际 %dms", RespawnNormal.Millis())
	}
	if RespawnBoss.Millis() != 30*60*1000 {
		t.Errorf("BOSS 重生应是 30 分钟, 实际 %dms", RespawnBoss.Millis())
	}
}

// 类别要跟着实体走 —— 场景靠它决定重生间隔。
func TestKindCarriedOntoEntity(t *testing.T) {
	points := []domain.SpawnPoint{
		{ID: 1, Monster: 树妖.ID}, {ID: 2, Monster: 刑天.ID}, {ID: 3, Monster: 金星草.ID},
	}
	s := newSpawner(t, points)
	want := map[int32]domain.MonsterKind{
		1: domain.MonsterNormal, 2: domain.MonsterBoss, 3: domain.MonsterProp,
	}
	for _, e := range s.Initial() {
		if got := e.Monster.Kind; got != want[e.Monster.SpawnID] {
			t.Errorf("刷怪点 %d 的类别应是 %v, 实际 %v",
				e.Monster.SpawnID, want[e.Monster.SpawnID], got)
		}
	}
}

// 摆设不该主动打人。一张图 5023 个场景物件要是会追人, 主城就成修罗场了。
func TestPropsAreNotHostile(t *testing.T) {
	if domain.MonsterProp.Hostile() || domain.MonsterGather.Hostile() {
		t.Fatal("场景物件与采集物不该有敌意")
	}
	for _, k := range []domain.MonsterKind{domain.MonsterNormal, domain.MonsterElite, domain.MonsterBoss} {
		if !k.Hostile() {
			t.Errorf("%v 应当有敌意", k)
		}
	}
}
