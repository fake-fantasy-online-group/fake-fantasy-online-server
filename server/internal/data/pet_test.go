package data

import (
	"context"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 宠物加载的集成测试。**必须打真库** —— 下面这几条结论全是从 504 行真数据上
// 数出来的，只有真查一次才守得住。

func TestLoadPets(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	// 504 行里 10944 有两行(同一坐骑的两个皮肤名), 按 pet_id 归并后 503 只
	if len(defs) != 503 {
		t.Fatalf("宠物数 %d, 该是 503(504 行 - 1 个重复 pet_id)", len(defs))
	}
	if _, ok := defs[10944]; !ok {
		t.Fatal("重复 pet_id 的那只被整个丢了")
	}
	海龟, ok := defs[1006]
	if !ok {
		t.Fatal("找不到海龟(pet_id=1006)")
	}
	if 海龟.Name != "海龟" {
		t.Fatalf("1006 的名字是 %q", 海龟.Name)
	}
	if 海龟.CaptureLevel != 10 || 海龟.BaseCaptureRate != 100 || 海龟.MaxCaptureRate != 160 {
		t.Fatalf("海龟捕捉参数读错: 等级 %d 成功率 %d→%d",
			海龟.CaptureLevel, 海龟.BaseCaptureRate, 海龟.MaxCaptureRate)
	}
}

// TestPetHPCoefficientIsNine 是这条系数的**可执行证据**。
//
// `docs/属性体系.md` 里 HPPerVIT/MPPerSPI = 9 原本只有两个来源
// (逗游文章 + ov_levelup 十个非零行)。宠物表是第三个独立来源，
// 而且样本量大两个数量级 —— 460/503 只严格满足 init_hp = init_vit × 9。
//
// 这个测试的作用是：将来谁把 9 改成别的数，它会在这里炸，
// 而不是等到线上发现所有宠物血量都不对。
func TestPetHPCoefficientIsNine(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	var hpHit, spHit, total int
	for _, d := range defs {
		total++
		if d.InitHP == d.Init.VIT*domain.HPPerVIT {
			hpHit++
		}
		if d.InitSP == d.Init.SPI*domain.MPPerSPI {
			spHit++
		}
	}
	// 归并掉的那只(10944)本身符合公式, 所以 460 少一只 = 459
	if hpHit != 459 {
		t.Fatalf("符合 生命=体质×%d 的宠物 %d/%d 只, 该是 459 —— "+
			"系数或数据变了, 先去 docs/属性体系.md 对账", domain.HPPerVIT, hpHit, total)
	}
	if spHit != 459 {
		t.Fatalf("符合 法力=精神×%d 的宠物 %d/%d 只, 该是 459", domain.MPPerSPI, spHit, total)
	}
	// 九成以上命中才算得上"证据"; 掉到这条线以下说明公式选错了
	if float64(hpHit)/float64(total) < 0.9 {
		t.Fatalf("命中率只有 %.1f%%, 撑不起'系数就是 9'这个结论",
			100*float64(hpHit)/float64(total))
	}
}

// TestSpecialPetsAreTwoTemplatesNotHandTuning 锁住那 44 个例外的性质。
//
// 它们不是逐只手调，是「物」「法」两个模板：物 393HP/50SP、法 258HP/370SP。
// 这件事决定了 MaxHPAt 该写成"保留差值"而不是"重算" —— 见 domain/pet.go。
func TestSpecialPetsAreTwoTemplatesNotHandTuning(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	shapes := map[[2]int32]int{}
	for _, d := range defs {
		if d.InitHP != d.Init.VIT*domain.HPPerVIT {
			shapes[[2]int32{d.InitHP, d.InitSP}]++
		}
	}
	if len(shapes) > 6 {
		t.Fatalf("例外的生命/法力组合有 %d 种, 说明不是模板而是逐只手调 —— "+
			"那 MaxHPAt 的'保留差值'写法要重新想", len(shapes))
	}
	// 两个主模板必须在
	for _, want := range [][2]int32{{393, 50}, {258, 370}} {
		if shapes[want] < 10 {
			t.Fatalf("模板 %dHP/%dSP 只出现 %d 次, 该是十几次",
				want[0], want[1], shapes[want])
		}
	}
}

func TestLoadPetLevelsIgnoresSaturatedTotal(t *testing.T) {
	tab, err := LoadPetLevels(context.Background(), testPool(t), domain.PetLevelCap)
	if err != nil {
		t.Fatal(err)
	}
	if got := tab.MaxLevel(); got != domain.PetLevelCap {
		t.Fatalf("曲线最高级 %d, 该是 %d", got, domain.PetLevelCap)
	}
	if got := tab.Need(1); got != 300 {
		t.Fatalf("1→2 级所需经验 %d, 真值是 300", got)
	}
	// 每级所需必须严格递增 —— exp_total 那列在 59 级饱和,
	// 如果哪天有人把 loader 改成读 exp_total, 这条会立刻炸
	var prev int64
	for lv := int32(1); lv <= domain.PetLevelCap; lv++ {
		need := tab.Need(lv)
		if lv < domain.PetLevelCap && need <= prev {
			t.Fatalf("%d 级所需 %d 没有比上一级 %d 大 —— 八成是读到 exp_total 了",
				lv, need, prev)
		}
		prev = need
	}
	// 饱和值绝不该出现在表里
	for lv := int32(1); lv <= domain.PetLevelCap; lv++ {
		if tab.Need(lv) == 2147483647 {
			t.Fatalf("%d 级读到了 int32 饱和值, loader 读错列了", lv)
		}
	}
}

// TestEveryCapturablePetIsInsideMVP 确认捕捉玩法整个落在 60 级以内。
func TestEveryCapturablePetIsInsideMVP(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range defs {
		if !d.Capturable() {
			continue
		}
		n++
		if d.CaptureLevel > domain.LevelCap {
			t.Fatalf("%s 要 %d 级才能抓, 超出 MVP 上限 %d", d.Name, d.CaptureLevel, domain.LevelCap)
		}
		if d.BaseCaptureRate > d.MaxCaptureRate {
			t.Fatalf("%s 基础成功率 %d 高于上限 %d", d.Name, d.BaseCaptureRate, d.MaxCaptureRate)
		}
	}
	if n != 18 {
		t.Fatalf("可捕捉的宠物 %d 只, 该是 18", n)
	}
}

// TestLoadPetStarveTiers 锁住饥渴四档的真值。
//
// 最要紧的一条是**负数还原**: 原表用 u8 存 −1(255)。
// 不还原的话"每 3 分钟饿 255 点", 饥渴度一跳就满 —— 而且极好认，
// 所以这条测试的价值在于它一次都不会误报。
func TestLoadPetStarveTiers(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	海龟, ok := defs[1006]
	if !ok {
		t.Fatal("找不到海龟")
	}
	want := [4]domain.StarveTier{
		{Delta: -1, Interval: 180},
		{Delta: -1, Interval: 360},
		{Delta: +1, Interval: 1800},
		{Delta: +1, Interval: 900},
	}
	if 海龟.Starve.Tiers != want {
		t.Fatalf("四档读成了 %+v\n该是 %+v", 海龟.Starve.Tiers, want)
	}
	if 海龟.Starve.Online != (domain.StarveTier{Delta: +1, Interval: 14400}) {
		t.Fatalf("在线档是 %+v", 海龟.Starve.Online)
	}

	// 全表: 只有两种取值 —— 476 只标准 + 28 只 Delta 全零(归并后 475+28)
	标准, 全零 := 0, 0
	for _, d := range defs {
		switch {
		case d.Starve.Tiers == want:
			标准++
		case d.Starve.Tiers[0].Delta == 0 && d.Starve.Tiers[3].Delta == 0:
			全零++
		default:
			t.Fatalf("%s 出现了第三种四档取值 %+v", d.Name, d.Starve.Tiers)
		}
	}
	if 标准+全零 != len(defs) {
		t.Fatalf("标准 %d + 全零 %d ≠ 总数 %d", 标准, 全零, len(defs))
	}
	if 全零 != 28 {
		t.Fatalf("永不饥饿的宠有 %d 只, 该是 28", 全零)
	}
	// 负数没还原的话这里会是 255
	for _, d := range defs {
		for i, tier := range d.Starve.Tiers {
			if tier.Delta > 1 {
				t.Fatalf("%s 第 %d 档变化量是 %d —— u8 存的负数没还原",
					d.Name, i+1, tier.Delta)
			}
		}
	}
}

// TestLoadPetFoods 锁住食物三档。
//
// 数量与描述文本**双源互证**: eats1~4_qty = 3/2/1/5,
// 与 game_descs 里的"降低饥渴 3/2/1/5 点"482 行逐个吻合。
func TestLoadPetFoods(t *testing.T) {
	foods, err := LoadPetFoods(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	// 三个食性 × 三档 + 无尽淳 = 10 种
	if len(foods) != 10 {
		t.Fatalf("食物 %d 种, 该是 10(三食性×三档 + 无尽淳)", len(foods))
	}
	for _, c := range []struct {
		id       domain.ItemID
		reduce   int32
		min, max int32
		anytime  bool
		trust    int32
		name     string
	}{
		{3020, 3, 0, 50, false, 0, "百叶草"},
		{3025, 2, 51, 75, false, 0, "百花草"},
		{3026, 1, 76, 100, false, 0, "百果草"},
		{3031, 5, 0, 0, true, 2, "无尽淳"},
	} {
		f, ok := foods[c.id]
		if !ok {
			t.Fatalf("找不到食物 %d(%s)", c.id, c.name)
		}
		if f.Name != c.name {
			t.Fatalf("%d 的名字是 %q, 该是 %q", c.id, f.Name, c.name)
		}
		if f.Reduce != c.reduce {
			t.Fatalf("%s 降 %d 点, 该是 %d", c.name, f.Reduce, c.reduce)
		}
		if f.Anytime != c.anytime || f.Trust != c.trust {
			t.Fatalf("%s anytime=%v trust=%d, 该是 %v/%d",
				c.name, f.Anytime, f.Trust, c.anytime, c.trust)
		}
		if !c.anytime && (f.Min != c.min || f.Max != c.max) {
			t.Fatalf("%s 区间 %d~%d, 该是 %d~%d", c.name, f.Min, f.Max, c.min, c.max)
		}
	}
	// 三档必须首尾相接、不留缝: 任何饥渴度都要有一档粮能用
	for starve := int32(0); starve <= domain.MaxStarve; starve++ {
		hit := 0
		for _, f := range foods {
			if !f.Anytime && f.Usable(starve) {
				hit++
			}
		}
		// 每个饥渴度恰好对应三种食性各一档 = 3 种
		if hit != 3 {
			t.Fatalf("饥渴度 %d 有 %d 种分档粮能用, 该是 3(三种食性各一)", starve, hit)
		}
	}
}

// TestPickupFlagIsBooleanNotThreshold 锁住一个**列名骗人**的地方。
//
// `pickup_trust` 看着像信赖阈值，实际 504 行只有 0/1 —— 是布尔标志。
// 当成阈值用的话("信赖 ≥ pickup_trust 才能捡")，
// 所有宠物一出生就满足条件，那一列就等于没写。
func TestPickupFlagIsBooleanNotThreshold(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range defs {
		if d.CanPickUp {
			n++
		}
	}
	// 504 行里 123 个 1。重复的 pet_id 10944 两行**都是 1**,
	// 按 pet_id 归并掉一行之后是 122
	if n != 122 {
		t.Fatalf("会捡东西的宠 %d 只, 该是 122(504 行里 123 个, 归并掉重复的一行)", n)
	}
	// **18 只可捕捉宠全都会捡** —— 这条让功能在 MVP 里立刻有用
	for _, d := range defs {
		if d.Capturable() && !d.CanPickUp {
			t.Fatalf("%s 能抓却不会捡东西 —— 18/18 该全会", d.Name)
		}
	}
}

// TestNonPetRowsAreMountsAndCostumes 锁住"宠物表里混着 28 行不是宠物的东西"。
//
// 三个条件 28/28 完全重合：四档饥渴全零、can_buy=0、can_deal=0。
// 其中 18 行是「金甲幻化书」，其余多为坐骑。
// 不把它们分出来的话，"可捕捉宠共 18 只"这类统计会被它们污染。
func TestNonPetRowsAreMountsAndCostumes(t *testing.T) {
	defs, err := LoadPets(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	real, fake := 0, 0
	for _, d := range defs {
		if d.RealPet {
			real++
			continue
		}
		fake++
		// 不是宠物的那些不该能抓
		if d.Capturable() {
			t.Fatalf("%s 被判成非宠物却标着捕捉等级 %d", d.Name, d.CaptureLevel)
		}
	}
	if fake != 28 {
		t.Fatalf("非宠物行 %d 条, 该是 28", fake)
	}
	if real+fake != len(defs) {
		t.Fatalf("%d + %d ≠ %d", real, fake, len(defs))
	}
}

// TestTradeTrustIsInitMinusTwenty 记录一条**没有实现**的规律。
//
// 玩家间交易不在 MVP 里，但这条规律数出来了就该锁住，免得将来重挖：
// 476/504 行满足 deal_trust = can_deal_trust = init_trust − 20。
// 28 个例外全是上面那批非宠物行。
func TestTradeTrustIsInitMinusTwenty(t *testing.T) {
	rows, err := testPool(t).Query(context.Background(), `
		SELECT init_trust, deal_trust, can_deal_trust, starve_add1
		  FROM gamedata.ov_petgrow`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	hit, exceptions := 0, 0
	for rows.Next() {
		var initT, dealT, canT, starve int32
		if err := rows.Scan(&initT, &dealT, &canT, &starve); err != nil {
			t.Fatal(err)
		}
		if dealT == initT-domain.TradeTrustDrop && canT == initT-domain.TradeTrustDrop {
			hit++
			continue
		}
		exceptions++
		// 例外必须都是非宠物行(四档饥渴全零)
		if starve != 0 {
			t.Fatalf("初始信赖 %d 交易信赖 %d, 差的不是 %d, 而且它是个真宠物",
				initT, dealT, domain.TradeTrustDrop)
		}
	}
	if hit != 476 || exceptions != 28 {
		t.Fatalf("符合 −%d 规律的 %d 行、例外 %d 行, 该是 476/28",
			domain.TradeTrustDrop, hit, exceptions)
	}
}
