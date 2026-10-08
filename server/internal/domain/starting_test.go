package domain

import "testing"

// 一级初始六维是从 gamedata.ov_levelup（prof=0 的五行模板）抄进来的快照。
// 抄错一个数字整个职业的手感就毁了，所以逐行钉住，并且用 ×9 不变量交叉验证。

func TestStartingBaseMatchesClientTable(t *testing.T) {
	// 直接来自 SELECT ... FROM gamedata.ov_levelup WHERE prof=0
	want := []struct {
		race                          Race
		name                          string
		ovType                        int
		str, vit, int_, spi, agi, dex int32
		initHP, initSP                int32
	}{
		{Warrior, "战士", 1, 12, 24, 4, 4, 4, 4, 216, 36},
		{Swordsman, "剑客", 2, 8, 20, 6, 6, 8, 6, 180, 54},
		{Assassin, "刺客", 3, 4, 16, 6, 4, 12, 12, 144, 36},
		{Healer, "药师", 4, 6, 16, 6, 12, 4, 4, 144, 108},
		{Warlock, "术士", 5, 6, 12, 12, 16, 4, 4, 108, 144},
	}

	for _, w := range want {
		got := StartingBase(w.race)
		exp := Base{STR: w.str, VIT: w.vit, INT: w.int_, SPI: w.spi, AGI: w.agi, DEX: w.dex}
		if got != exp {
			t.Errorf("%s(ov_levelup.type=%d) 六维不符\n  实际 %+v\n  期望 %+v", w.name, w.ovType, got, exp)
			continue
		}

		// ×9 交叉验证。这十项全中, 是 HPPerVIT / MPPerSPI 的第二个独立来源 ——
		// 也就是说抄错任何一个数字, 这里都会炸。
		s := DeriveFromBase(got)
		if s.MaxHP != w.initHP {
			t.Errorf("%s: 体质%d × 9 = %d, 但 ov_levelup.init_hp 是 %d",
				w.name, w.vit, s.MaxHP, w.initHP)
		}
		if s.MaxMP != w.initSP {
			t.Errorf("%s: 精神%d × 9 = %d, 但 ov_levelup.init_sp 是 %d",
				w.name, w.spi, s.MaxMP, w.initSP)
		}
	}
}

// 职业与 ov_levelup.type / ov_skilldesc.prof 的对应是 type = Race + 1，五个一一对应。
// **[实证]** prof=4 是治愈术/复法术(hurt_type 12 圣) → 药师;
//
//	prof=5 是火弹术/火球术(hurt_type 8 火 / 9 冰) → 术士。
//
// docs/属性体系.md 此前把这两个写反了，钉一个测试免得再翻车。
func TestHealerAndWarlockAreNotSwapped(t *testing.T) {
	healer, warlock := StartingBase(Healer), StartingBase(Warlock)

	// 术士靠智慧吃饭(智慧÷2 = 魔攻), 一级智慧就该是全场最高
	if warlock.INT <= healer.INT {
		t.Errorf("术士智慧 %d 不该低于药师的 %d —— 大概率是 type 4/5 又搞反了",
			warlock.INT, healer.INT)
	}
	for r := Warrior; r <= Warlock; r++ {
		if r != Warlock && StartingBase(r).INT > warlock.INT {
			t.Errorf("职业 %d 的智慧比术士还高", r)
		}
	}
	// 药师比术士耐打(体质高), 术士法力池更大
	if healer.VIT <= warlock.VIT {
		t.Errorf("药师体质 %d 应高于术士的 %d", healer.VIT, warlock.VIT)
	}
	if DeriveFromBase(warlock).MaxMP <= DeriveFromBase(healer).MaxMP {
		t.Error("术士的法力池应比药师大")
	}
}

// 战士最耐打, 刺客最灵巧 —— 五个职业的定位不该互相串。
func TestClassIdentities(t *testing.T) {
	for r := Swordsman; r <= Warlock; r++ {
		if StartingBase(r).VIT > StartingBase(Warrior).VIT {
			t.Errorf("职业 %d 的体质比战士还高", r)
		}
		if StartingBase(r).DEX > StartingBase(Assassin).DEX {
			t.Errorf("职业 %d 的灵巧比刺客还高", r)
		}
	}
}

// 非法职业不能返回零值 —— 零值六维意味着 0 血, 玩家生下来就是尸体。
func TestStartingBaseNeverZero(t *testing.T) {
	for _, r := range []Race{Warrior, Swordsman, Assassin, Healer, Warlock, Race(200)} {
		if b := StartingBase(r); b == (Base{}) {
			t.Errorf("职业 %d 拿到零值六维 —— 会推导出 0 血", r)
		}
	}
}

// EffectiveBase: 老存档没有六维, 必须补出**该等级应有的**六维而不是零值。
//
// 以前这里退回的是一级初始值, 而那会让高等级角色按一级战力打怪
// (见 TestZeroBaseIsFilledToTheCharactersOwnLevel)。
func TestEffectiveBaseFallsBackForLegacyCharacters(t *testing.T) {
	legacy := &Character{ID: 1, Name: "老存档", Race: Assassin, Level: 30}
	got := legacy.EffectiveBase()
	if got == (Base{}) {
		t.Fatal("没有六维的老角色拿到零值 —— 会推导出 0 血")
	}
	if start := StartingBase(Assassin); got.STR <= start.STR || got.VIT <= start.VIT {
		t.Fatalf("30 级角色的六维没比一级高: %+v vs 初始 %+v", got, start)
	}
	if hp := DeriveFromBase(legacy.EffectiveBase()).MaxHP; hp <= 0 {
		t.Fatalf("退回之后血量仍是 %d", hp)
	}

	// 有六维的角色不该被覆盖
	custom := Base{STR: 99, VIT: 99, INT: 99, SPI: 99, AGI: 99, DEX: 99}
	ch := &Character{Race: Assassin, Base: custom}
	if ch.EffectiveBase() != custom {
		t.Fatal("已有六维的角色不该被初始值覆盖")
	}
}

// 建号就该带上初始六维, 不该等到进图才补。
func TestNewCharacterGetsStartingBase(t *testing.T) {
	c, err := NewCharacter(1, 0, "新号", Warlock, Appearance{Head: 1, Hair: 1}, Pos{MapID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if c.Base != StartingBase(Warlock) {
		t.Fatalf("建号应带上术士初始六维, 实际 %+v", c.Base)
	}
	if DeriveFromBase(c.Base).MaxHP != 108 {
		t.Fatalf("术士一级应有 108 血, 实际 %d", DeriveFromBase(c.Base).MaxHP)
	}
}

// 六维是空的角色, 按**它自己的等级**补齐, 不是退回一级。
//
// 从真实抓包导入的角色只有属性数组、没有六维。以前这里退回一级初始值,
// 于是 63 级角色按一级战力打怪 —— 实测打 12 级的白兔(215 血)连砍 8 下打不死,
// 而服务端日志一切正常(伤害确实结算了, 只是每下三十点)。
func TestZeroBaseIsFilledToTheCharactersOwnLevel(t *testing.T) {
	lv1 := &Character{Race: Warrior, Level: 1}
	lv63 := &Character{Race: Warrior, Level: 63}

	if got := lv1.EffectiveBase(); got != StartingBase(Warrior) {
		t.Fatalf("一级角色的六维该是初始值, 得到 %+v", got)
	}
	hi := lv63.EffectiveBase()
	lo := lv1.EffectiveBase()
	if hi == lo {
		t.Fatal("63 级和 1 级的六维一样 —— 等级成长没补上")
	}
	if hi.STR <= lo.STR || hi.VIT <= lo.VIT {
		t.Fatalf("63 级六维没比 1 级高: %+v vs %+v", hi, lo)
	}
	// 与"一路升上来"的角色应当一致 —— 补齐用的就是同一条成长曲线
	grown := &Character{Race: Warrior, Level: 1, Base: StartingBase(Warrior)}
	grown.Level = 63
	grown.ApplyLevelUps(1, 63)
	if grown.Base != hi {
		t.Fatalf("补齐的六维与一路升上来的不一致:\n  补齐 %+v\n  升级 %+v", hi, grown.Base)
	}
}

// 升级不能把成长算两遍。
//
// AddExp 会先把 c.Level 推到升级后的值, ApplyLevelUps 才加成长 ——
// 那里若按"当前等级"补齐零值六维, from+1..to 这段就会被加两次。
func TestLevelUpDoesNotDoubleCountGrowth(t *testing.T) {
	c := &Character{Race: Warrior, Level: 10} // 六维空
	want := (&Character{Race: Warrior, Level: 15}).EffectiveBase()

	c.Level = 15 // AddExp 干的事
	c.ApplyLevelUps(10, 15)

	if c.Base != want {
		t.Fatalf("10→15 级之后六维 %+v, 该是 15 级应有的 %+v", c.Base, want)
	}
}
