package domain

import "testing"

// 海龟：ov_petgrow pet_id=1006 的真值。捕捉等级 10、成功率 100→160。
// 它是 460 只"严格符合 HP=体质×9"里的一只。
var 海龟 = PetDef{
	ID: 1006, Name: "海龟",
	CaptureLevel: 10, CaptureTool: 3382, BaseCaptureRate: 100, MaxCaptureRate: 160,
	Init:      Base{STR: 3, DEX: 2, AGI: 2, VIT: 6, INT: 6, SPI: 8},
	InitHP:    54, // = 6 × 9
	InitSP:    72, // = 8 × 9
	GrowthOdd: Base{STR: 0, VIT: 3, DEX: 1, AGI: 1, INT: 2, SPI: 3},
	// 真表里海龟奇偶相同; 这里故意让偶数级少长 1 点体质,
	// 好让"必须按奇偶取值"这件事在测试里真的有区别可测
	GrowthEven: Base{STR: 0, VIT: 2, DEX: 1, AGI: 1, INT: 2, SPI: 3},
	MaxLevel:   80,
	InitTrust:  50, InitStarve: 50,
}

// 刑天·物：44 个例外之一。体质 42 但生命 393，不是 42×9=378。
var 刑天物 = PetDef{
	ID: 9001, Name: "刑天·物",
	Init: Base{STR: 21, DEX: 7, AGI: 7, VIT: 42, INT: 7, SPI: 5},
	// 手调值: 393 比公式多 15; 50 比公式(45)多 5
	InitHP: 393, InitSP: 50,
	GrowthOdd:  Base{VIT: 5, SPI: 1},
	GrowthEven: Base{VIT: 5, SPI: 1},
	MaxLevel:   80,
}

func TestPetHPUsesTheSameNineAsCharacters(t *testing.T) {
	// 460/504 只宠满足 init_hp = init_vit × 9, 这是 HPPerVIT 的第三个独立来源
	if 海龟.InitHP != 海龟.Init.VIT*HPPerVIT {
		t.Fatalf("初始生命 %d ≠ 体质 %d × %d", 海龟.InitHP, 海龟.Init.VIT, HPPerVIT)
	}
	if 海龟.InitSP != 海龟.Init.SPI*MPPerSPI {
		t.Fatalf("初始法力 %d ≠ 精神 %d × %d", 海龟.InitSP, 海龟.Init.SPI, MPPerSPI)
	}
	// 符合公式的宠, 每一级都该恒等于 体质×9
	for lv := int32(1); lv <= 60; lv++ {
		want := 海龟.BaseAt(lv).VIT * HPPerVIT
		if got := 海龟.MaxHPAt(lv); got != want {
			t.Fatalf("%d 级生命 %d, 按体质算该是 %d", lv, got, want)
		}
	}
}

func TestSpecialPetKeepsItsHandTunedOffset(t *testing.T) {
	// 那 44 只的手调差值必须一路带上去 ——
	// 写成"体质×9"的话, 1 级的区别升一级就没了
	offset := 刑天物.InitHP - 刑天物.Init.VIT*HPPerVIT
	if offset == 0 {
		t.Fatal("测试样本选错了, 这只并不是例外")
	}
	for lv := int32(1); lv <= 60; lv++ {
		got := 刑天物.MaxHPAt(lv)
		byFormula := 刑天物.BaseAt(lv).VIT * HPPerVIT
		if got-byFormula != offset {
			t.Fatalf("%d 级: 生命 %d, 公式值 %d, 差值 %d 应恒为 %d",
				lv, got, byFormula, got-byFormula, offset)
		}
	}
}

func TestPetGrowthAlternatesByLevelParity(t *testing.T) {
	// 1→2 走偶数级那套(+2 体质), 2→3 走奇数级那套(+3)
	if got := 海龟.BaseAt(1).VIT; got != 6 {
		t.Fatalf("1 级体质 %d, 该是初值 6", got)
	}
	if got := 海龟.BaseAt(2).VIT; got != 6+2 {
		t.Fatalf("2 级体质 %d, 该是 6+偶数级成长 2", got)
	}
	if got := 海龟.BaseAt(3).VIT; got != 6+2+3 {
		t.Fatalf("3 级体质 %d, 该是 6+2+3", got)
	}
	// 取平均会算成 6 + 2×2.5 = 11, 而正确值是 11 —— 5 级才拉得开差距
	if got := 海龟.BaseAt(5).VIT; got != 6+2+3+2+3 {
		t.Fatalf("5 级体质 %d, 该是 16", got)
	}
}

func TestPetLevelIsCappedByOwner(t *testing.T) {
	// 宠物不能超过主人
	if got := PetCapFor(海龟, 30); got != 30 {
		t.Fatalf("主人 30 级时宠物上限 %d, 该是 30", got)
	}
	// 主人到顶了也不能超过 60, 哪怕表里写着 80
	if got := PetCapFor(海龟, 60); got != PetLevelCap {
		t.Fatalf("主人满级时宠物上限 %d, 该是 %d", got, PetLevelCap)
	}
	if 海龟.MaxLevel <= PetLevelCap {
		t.Fatal("样本的表内上限该比 MVP 上限高, 否则这条测不出东西")
	}
	// 表内上限更低时以表为准
	矮宠 := 海龟
	矮宠.MaxLevel = 20
	if got := PetCapFor(矮宠, 60); got != 20 {
		t.Fatalf("表内上限 20 时算出 %d", got)
	}
}

func TestCaptureNeedsAWeakenedTarget(t *testing.T) {
	if r := CanCapture(海龟, 10, true, true, 1.0); r != CaptureTargetNotWeak {
		t.Fatalf("满血就该抓不动, 得到 %v", r)
	}
	if r := CanCapture(海龟, 10, true, true, 0.2); r != CaptureOK {
		t.Fatalf("残血该能抓, 得到 %v", r)
	}
	if r := CanCapture(海龟, 9, true, true, 0.2); r != CaptureLevelTooLow {
		t.Fatalf("等级不够该拒, 得到 %v", r)
	}
	if r := CanCapture(海龟, 10, false, true, 0.2); r != CaptureNoTool {
		t.Fatalf("没工具该拒, 得到 %v", r)
	}
	if r := CanCapture(海龟, 10, true, false, 0.2); r != CaptureBagFull {
		t.Fatalf("背包满该拒, 得到 %v", r)
	}
	// 486/504 只压根抓不到
	不可捕 := 海龟
	不可捕.CaptureLevel = 0
	if r := CanCapture(不可捕, 60, true, true, 0.0); r != CaptureNotCapturable {
		t.Fatalf("不可捕捉的宠该拒, 得到 %v", r)
	}
}

func TestCaptureRateStaysInsideTheMeasuredRange(t *testing.T) {
	// 值域两端是实证的, 中间怎么走是服务端定的 —— 但无论怎么走都不能出界
	for _, hp := range []float64{1.0, 0.9, 0.5, 0.3, 0.1, 0.0, -0.5} {
		got := CaptureRate(海龟, hp)
		if got < 海龟.BaseCaptureRate || got > 海龟.MaxCaptureRate {
			t.Fatalf("血量比例 %.2f 时成功率 %d, 越出值域 [%d,%d]",
				hp, got, 海龟.BaseCaptureRate, 海龟.MaxCaptureRate)
		}
	}
	// 门槛处取下端, 血空取上端
	if got := CaptureRate(海龟, CaptureHPThreshold); got != 海龟.BaseCaptureRate {
		t.Fatalf("刚到门槛该是基础值 %d, 得到 %d", 海龟.BaseCaptureRate, got)
	}
	if got := CaptureRate(海龟, 0); got != 海龟.MaxCaptureRate {
		t.Fatalf("血空该是上限 %d, 得到 %d", 海龟.MaxCaptureRate, got)
	}
	// 越残血成功率越高, 不能有回头
	prev := int32(0)
	for i := 10; i >= 0; i-- {
		got := CaptureRate(海龟, float64(i)/10)
		if got < prev {
			t.Fatalf("血量 %.1f 时成功率 %d 反而低于上一档 %d", float64(i)/10, got, prev)
		}
		prev = got
	}
}

func TestPetExpTableIgnoresTheSaturatedAccumulator(t *testing.T) {
	// exp_total 从 59 级起是 int32 饱和值, 所以表里只放 exp_need
	tab := NewPetLevelTable(map[int32]int64{1: 300, 2: 1100, 3: 2700})
	if got := tab.Need(1); got != 300 {
		t.Fatalf("1 级所需 %d", got)
	}
	if got := tab.MaxLevel(); got != 3 {
		t.Fatalf("最高级 %d", got)
	}
	if got := tab.Need(99); got != 0 {
		t.Fatalf("表外该返回 0, 得到 %d", got)
	}
	var nilTab *PetLevelTable
	if got := nilTab.Need(1); got != 0 {
		t.Fatalf("空表该返回 0, 得到 %d", got)
	}
}

func TestRideableAndCapturableAreIndependent(t *testing.T) {
	// 18 只可捕捉的里只有 7 只能骑 —— 两件事没有蕴含关系
	if 海龟.Rideable() {
		t.Fatal("海龟骑速为 0, 不该判定为坐骑")
	}
	独角兽 := PetDef{CaptureLevel: 35, HorseBaseSpeed: 215}
	if !独角兽.Capturable() || !独角兽.Rideable() {
		t.Fatal("独角兽既可捕捉又可骑")
	}
}
