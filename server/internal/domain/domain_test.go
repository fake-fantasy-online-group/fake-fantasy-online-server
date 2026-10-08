package domain

import "testing"

// ── 时间 ──

// 毫秒换帧要**向上取整**: 宁可晚一帧, 不能早一帧。
// 早了就是"冷却没到就能放技能", 那是能被玩家发现并利用的。
func TestTicksRoundsUp(t *testing.T) {
	cases := []struct {
		ms   int
		want Tick
		why  string
	}{
		{0, 0, "零就是零"},
		{-5, 0, "负数按零处理"},
		{1, 1, "1ms 也要占满一帧"},
		{100, 1, "正好一帧"},
		{101, 2, "多出 1ms 就要第二帧"},
		{1500, 15, "攻击间隔 1500ms = 15 帧"},
		{900_000, 9000, "副本限时 900 秒 = 9000 帧"},
		{36_000, 360, "打工结算 36 秒 = 360 帧"},
	}
	for _, c := range cases {
		if got := Ticks(c.ms); got != c.want {
			t.Errorf("Ticks(%d) = %d, 期望 %d (%s)", c.ms, got, c.want, c.why)
		}
	}
}

func TestTickMillisRoundTrip(t *testing.T) {
	if got := Tick(15).Millis(); got != 1500 {
		t.Fatalf("15 帧应是 1500ms, 实际 %d", got)
	}
}

// 攻击间隔: 基准 1500ms; 装备提速后不能快到一帧连打。
func TestAttackIntervalTicks(t *testing.T) {
	cases := []struct {
		ms   int32
		want Tick
	}{
		{0, 15},    // 未设置 -> 用 ov_arm 基准 1500ms
		{1500, 15}, // 基准
		{1200, 12}, // 刺客型
		{1800, 18}, // 重型
		{50, 1},    // 快到离谱也至少一帧, 否则伤害数值失控
	}
	for _, c := range cases {
		s := Stats{AtkSpeedMS: c.ms}
		if got := s.AttackIntervalTicks(); got != c.want {
			t.Errorf("攻速 %dms 应是 %d 帧, 实际 %d", c.ms, c.want, got)
		}
	}
}

// ── 实体标识 ──

// 四类实体分段发号, 从 id 一眼能看出这是什么东西, 也保证不会撞号。
func TestEntityAllocSegments(t *testing.T) {
	a := NewEntityAlloc()
	got := map[EntityKind]EntityID{
		KindPlayer:  a.Player(),
		KindNPC:     a.NPC(),
		KindMonster: a.Monster(),
		KindDrop:    a.Drop(),
	}
	for want, id := range got {
		if id.SegKind() != want {
			t.Errorf("id %d 应属于分段 %v, 实际 %v", id, want, id.SegKind())
		}
	}
	// 0 保留为"无实体", 任何分配都不该发出 0
	for k, id := range got {
		if id == 0 {
			t.Errorf("%v 分配出了 0, 但 0 是保留值", k)
		}
	}
}

// 同一段内连续分配不重号。
func TestEntityAllocUnique(t *testing.T) {
	a := NewEntityAlloc()
	seen := map[EntityID]bool{}
	for i := 0; i < 1000; i++ {
		id := a.Player()
		if seen[id] {
			t.Fatalf("重复分配了 id %d", id)
		}
		seen[id] = true
	}
}

// ── 场景 ──

func TestSceneIDPersistent(t *testing.T) {
	if !(SceneID{MapID: 7}).Persistent() {
		t.Error("Instance=0 是常驻大陆图")
	}
	if (SceneID{MapID: 20502, Instance: 1}).Persistent() {
		t.Error("Instance>0 是副本实例, 不是常驻图")
	}
}

// ── 角色 ──

// Clone 必须真拷 Attrs。浅拷贝共享底层数组, 等于没拷 ——
// 场景改它就和写库的 goroutine 打架了。
func TestCharacterCloneIsDeep(t *testing.T) {
	orig := &Character{ID: 1, Name: "甲", Level: 10, Attrs: []int32{1, 2, 3}}
	cp := orig.Clone()

	if cp == orig {
		t.Fatal("Clone 返回了同一个指针")
	}
	cp.Level = 99
	cp.Attrs[0] = 9
	if orig.Level != 10 {
		t.Error("改克隆体影响了原对象的等级")
	}
	if orig.Attrs[0] != 1 {
		t.Error("Attrs 还是共享的底层数组 —— 这正是要避免的那种竞态")
	}
}

func TestCloneNilIsNil(t *testing.T) {
	var c *Character
	if c.Clone() != nil {
		t.Error("nil 的克隆应该还是 nil")
	}
}

// ── 属性推导 ──

// 六条换算系数。改这里之前必须先改 docs/属性体系.md ——
// 这些数是考据出来的, 不是拍的。
func TestDeriveFromBase(t *testing.T) {
	// 用 ov_levelup 那种量级的一组值, 避免整除掩盖 ÷2 的截断行为
	b := Base{STR: 11, VIT: 20, INT: 9, SPI: 14, AGI: 7, DEX: 13}
	s := DeriveFromBase(b)

	cases := []struct {
		name string
		got  int32
		want int32
		src  string
	}{
		{"最大生命 = 体质 × 9", s.MaxHP, 180, "逗游文章 + init_hp/init_vit=9.000 双来源"},
		{"最大法力 = 精神 × 9", s.MaxMP, 126, "驱动法力的是精神不是智慧, 见属性体系.md"},
		{"攻击 = 力量 ÷ 2", s.MinAtk, 5, "单来源(逗游), 抓包不能证实也不能证伪"},
		{"防御 = 敏捷 ÷ 2", s.Def, 3, "本作防御走闪避不减伤"},
		{"命中 = 灵巧 ÷ 2", s.Hit, 6, ""},
		{"魔攻 = 智慧 ÷ 2", s.MAtk, 4, ""},
		{"魔防 = 精神 ÷ 2", s.MDef, 7, ""},
		{"负重 = 500 + 力量 × 10", s.MaxWeight, 610, "真实新战士 STR=12 时面板为 620"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: 期望 %d 实际 %d %s", c.name, c.want, c.got, c.src)
		}
	}
}

// 攻击上下限的**浮动来自武器**, 不来自基础属性 —— 抓包印证:
// 怪没有武器所以打我恒为 4, 我有小剑(1~5)所以打它是 4~7。
func TestDeriveGivesNoWeaponSpread(t *testing.T) {
	s := DeriveFromBase(Base{STR: 20})
	if s.MinAtk != s.MaxAtk {
		t.Fatalf("裸属性推导出的攻击不该有浮动区间, 实际 %d~%d", s.MinAtk, s.MaxAtk)
	}
}

func TestBaseAdd(t *testing.T) {
	got := Base{STR: 1, VIT: 2, INT: 3, SPI: 4, AGI: 5, DEX: 6}.
		Add(Base{STR: 10, VIT: 20, INT: 30, SPI: 40, AGI: 50, DEX: 60})
	want := Base{STR: 11, VIT: 22, INT: 33, SPI: 44, AGI: 55, DEX: 66}
	if got != want {
		t.Fatalf("六维相加错了: %+v", got)
	}
}

func TestStatusMitigationConsumesDamageShield(t *testing.T) {
	set := NewStatusSet()
	set.Apply(StatusDef{
		ID: 1021, Level: 1, DurationSec: 30,
		PhysicalDamageReductionPct: 25,
		DamageShield:               100,
	}, 0, 1)

	got, removed := set.MitigateDamage(false, 200)
	if got != 50 { // 200 先减伤 25% = 150，再由护盾吸收 100
		t.Fatalf("减伤和护盾后的伤害应为 50，实际 %d", got)
	}
	if len(removed) != 1 || removed[0] != 1021 || set.Has(1021) {
		t.Fatalf("护盾耗尽后应移除状态，removed=%v has=%v", removed, set.Has(1021))
	}
}

func TestInvulnerableAndFlatMagicReduction(t *testing.T) {
	set := NewStatusSet()
	set.Apply(StatusDef{ID: 1068, DurationSec: 30, MagicDamageReductionFlat: 230}, 0, 1)
	if got, _ := set.MitigateDamage(true, 500); got != 270 {
		t.Fatalf("500 点法伤减 230 后应为 270，实际 %d", got)
	}
	set.Apply(StatusDef{ID: 1178, DurationSec: 20, Invulnerable: true}, 0, 1)
	if got, _ := set.MitigateDamage(false, 500); got != 0 {
		t.Fatalf("无敌状态下物理伤害应为 0，实际 %d", got)
	}
	if got, _ := set.MitigateDamage(true, 500); got != 0 {
		t.Fatalf("无敌状态下法术伤害应为 0，实际 %d", got)
	}
}

func TestStatusCategoryBlockers(t *testing.T) {
	set := NewStatusSet()
	set.Apply(StatusDef{ID: StatusSpiritLock, DurationSec: 30, BlockHarmful: true}, 0, 1)
	bad := StatusDef{ID: StatusWeaken, TypeKnown: true, Type: StatusBad}
	good := StatusDef{ID: 1014, TypeKnown: true, Type: StatusGood}
	if !set.BlocksStatus(bad) {
		t.Fatal("锁灵应阻止有害状态")
	}
	if set.BlocksStatus(good) {
		t.Fatal("锁灵不应阻止增益状态")
	}

	set.Apply(StatusDef{ID: StatusChaos, DurationSec: 30, BlockBeneficial: true}, 0, 1)
	if !set.BlocksStatus(good) {
		t.Fatal("混沌应阻止增益状态")
	}
}

func TestOneShotStatusOnlyConsumesWhenApplicable(t *testing.T) {
	set := NewStatusSet()
	set.Apply(StatusDef{ID: StatusFocus, DurationSec: 30, NextStatusDurationPct: 50}, 0, 1)

	_, duration, _, removed := set.ConsumeAttackModifiers(true, true, false, false)
	if duration != 0 || len(removed) != 0 || !set.Has(StatusFocus) {
		t.Fatal("没有持续状态的攻击技能不应浪费集中")
	}
	_, duration, _, removed = set.ConsumeAttackModifiers(true, false, true, false)
	if duration != 50 || len(removed) != 1 || set.Has(StatusFocus) {
		t.Fatal("有时限的技能应获得 50% 时长并消耗集中")
	}
}
