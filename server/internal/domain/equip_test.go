package domain

import "testing"

// 钢剑: game_equipment 真值 —— 6 级, 单手(slot 4), 需要力量14/敏捷14。
var 钢剑 = ItemDef{ID: 4002, Name: "钢剑", Equip: &EquipDef{
	Slot: int32(SlotWeapon), LevelReq: 6, Durable: 100,
	Bonus: Stats{MinAtk: 8, MaxAtk: 14},
	Need:  Requirement{Level: 6, Base: Base{STR: 14, AGI: 14}},
}}

func defsOf(items ...ItemDef) func(ItemID) (ItemDef, bool) {
	m := map[ItemID]ItemDef{}
	for _, d := range items {
		m[d.ID] = d
	}
	return func(id ItemID) (ItemDef, bool) {
		d, ok := m[id]
		return d, ok
	}
}

func wearing(slot EquipSlot, d ItemDef) *EquipSet {
	e := NewEquipSet()
	e.Set(slot, Stack{Item: d.ID, Count: 1})
	return e
}

// 不穿装备时, 属性就是六维推导的结果。
func TestComputeNoEquipment(t *testing.T) {
	b := StartingBase(Warrior)
	gotBase, gotStats := Compute(StatSource{Base: b, Level: 1, Worn: NewEquipSet(),
		Defs: defsOf()})
	if gotBase != b {
		t.Errorf("没装备时六维不该变: %+v", gotBase)
	}
	if gotStats != DeriveFromBase(b) {
		t.Errorf("没装备时二级属性应等于纯推导值")
	}
}

// 装备自带的攻防直接加到二级属性上。
func TestComputeEquipBonus(t *testing.T) {
	b := StartingBase(Warrior)
	_, s := Compute(StatSource{Base: b, Level: 6,
		Worn: wearing(SlotWeapon, 钢剑), Defs: defsOf(钢剑)})

	want := DeriveFromBase(b)
	if s.MinAtk != want.MinAtk+8 || s.MaxAtk != want.MaxAtk+14 {
		t.Fatalf("钢剑应加 8~14 攻击, 实际 %d~%d(裸 %d~%d)",
			s.MinAtk, s.MaxAtk, want.MinAtk, want.MaxAtk)
	}
}

// **这是整个文件最重要的一条**: 装备加的六维必须先进六维、再参与推导,
// 不能当二级属性直接加。战士体质 +10 应该变成 +90 血, 而不是 +10 血。
func TestEquipBaseFeedsDerivation(t *testing.T) {
	armor := ItemDef{ID: 5001, Name: "铁甲", Equip: &EquipDef{
		Slot: int32(SlotBody), Base: Base{VIT: 10},
	}}
	b := StartingBase(Warrior)
	gotBase, s := Compute(StatSource{Base: b, Level: 10,
		Worn: wearing(SlotBody, armor), Defs: defsOf(armor)})

	if gotBase.VIT != b.VIT+10 {
		t.Fatalf("装备的体质应进六维, 实际 %d(裸 %d)", gotBase.VIT, b.VIT)
	}
	want := DeriveFromBase(b).MaxHP + 10*HPPerVIT
	if s.MaxHP != want {
		t.Fatalf("体质+10 应变成 +%d 血, 期望 %d 实际 %d —— 当二级属性加会少算 %d",
			10*HPPerVIT, want, s.MaxHP, want-s.MaxHP)
	}
}

// 词条里的六维同样要先进六维。
func TestAffixBaseFeedsDerivation(t *testing.T) {
	ring := ItemDef{ID: 5002, Name: "力量戒指", Equip: &EquipDef{
		Slot:    int32(SlotRing),
		Affixes: []Affix{{Attr: AttrSTR, Value: 20, Mode: ModeAbsolute}},
	}}
	b := StartingBase(Warrior)
	gotBase, s := Compute(StatSource{Base: b, Level: 10,
		Worn: wearing(SlotRing, ring), Defs: defsOf(ring)})

	if gotBase.STR != b.STR+20 {
		t.Fatalf("词条的力量应进六维, 实际 %d", gotBase.STR)
	}
	// 力量 ÷2 = 攻击, 所以 +20 力量 = +10 攻击
	if want := DeriveFromBase(b).MinAtk + 10; s.MinAtk != want {
		t.Fatalf("力量+20 应变成 +10 攻击, 期望 %d 实际 %d", want, s.MinAtk)
	}
}

// 二级属性词条直接加。这些字段**只从装备来** —— 没有词条它们永远是零。
func TestAffixSecondaryStats(t *testing.T) {
	amulet := ItemDef{ID: 5003, Name: "全能护符", Equip: &EquipDef{
		Slot: int32(SlotNeck),
		Affixes: []Affix{
			{Attr: AttrCrit, Value: 800, Mode: ModeAbsolute},      // 万分比 = 8%
			{Attr: AttrPhysRes, Value: 15, Mode: ModeAbsolute},    // 伤害抗性
			{Attr: AttrMagicRes, Value: 12, Mode: ModeAbsolute},   // 魔法抗性
			{Attr: AttrStatusRes, Value: 20, Mode: ModeAbsolute},  // 不良状态抗性
			{Attr: AttrAtkSpeed, Value: -200, Mode: ModeAbsolute}, // 负数 = 提速
			{Attr: AttrHit, Value: 30, Mode: ModeAbsolute},
		},
	}}
	_, s := Compute(StatSource{Base: StartingBase(Warrior), Level: 10,
		Worn: wearing(SlotNeck, amulet), Defs: defsOf(amulet)})

	cases := []struct {
		name string
		got  int32
		want int32
	}{
		{"爆击率(万分比)", s.CritRate, 800},
		{"伤害抗性", s.PhysResist, 15},
		{"魔法抗性", s.MagicResist, 12},
		{"不良状态抗性", s.StatusResist, 20},
		{"攻速修正", s.AtkSpeedMS, -200},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s 应是 %d, 实际 %d —— 这些字段只从装备来, 不对就是没接上",
				c.name, c.want, c.got)
		}
	}
	if want := DeriveFromBase(StartingBase(Warrior)).Hit + 30; s.Hit != want {
		t.Errorf("命中应是 %d, 实际 %d", want, s.Hit)
	}
}

// attr47「攻击」同时加上下限。
func TestAttrAtkAddsBothBounds(t *testing.T) {
	w := ItemDef{ID: 5004, Equip: &EquipDef{Slot: int32(SlotWeapon),
		Affixes: []Affix{{Attr: AttrAtk, Value: 50, Mode: ModeAbsolute}}}}
	base := StartingBase(Warrior)
	_, s := Compute(StatSource{Base: base, Level: 10,
		Worn: wearing(SlotWeapon, w), Defs: defsOf(w)})

	want := DeriveFromBase(base)
	if s.MinAtk != want.MinAtk+50 || s.MaxAtk != want.MaxAtk+50 {
		t.Fatalf("attr47 应同时加上下限, 实际 %d~%d", s.MinAtk, s.MaxAtk)
	}
}

// 百分比词条最后乘。mode=1 的值域实测是 2~50, 确实是百分比。
func TestPercentAffixAppliedLast(t *testing.T) {
	// 一件加 10 体质(绝对) + 20% 最大生命的衣服
	armor := ItemDef{ID: 5005, Equip: &EquipDef{Slot: int32(SlotBody),
		Affixes: []Affix{
			{Attr: AttrVIT, Value: 10, Mode: ModeAbsolute},
			{Attr: AttrMaxHP, Value: 20, Mode: ModePercent},
		}}}
	base := StartingBase(Warrior)
	_, s := Compute(StatSource{Base: base, Level: 10,
		Worn: wearing(SlotBody, armor), Defs: defsOf(armor)})

	// 先算 (体质+10)×9, 再 ×1.2
	raw := (base.VIT + 10) * HPPerVIT
	want := raw + raw*20/100
	if s.MaxHP != want {
		t.Fatalf("百分比应在绝对值之后乘: 期望 %d 实际 %d\n"+
			"  (先加体质再×1.2 = %d; 先×1.2 再加体质 = %d)",
			want, s.MaxHP, want, base.VIT*HPPerVIT*120/100+10*HPPerVIT)
	}
}

// 六维的百分比词条作用在六维上, 然后才推导。
func TestBasePercentAffix(t *testing.T) {
	ring := ItemDef{ID: 5006, Equip: &EquipDef{Slot: int32(SlotRing),
		Affixes: []Affix{{Attr: AttrVIT, Value: 50, Mode: ModePercent}}}}
	base := StartingBase(Warrior) // 战士体质 24
	gotBase, s := Compute(StatSource{Base: base, Level: 10,
		Worn: wearing(SlotRing, ring), Defs: defsOf(ring)})

	wantVIT := base.VIT + base.VIT*50/100
	if gotBase.VIT != wantVIT {
		t.Fatalf("体质应 ×1.5 = %d, 实际 %d", wantVIT, gotBase.VIT)
	}
	if s.MaxHP != wantVIT*HPPerVIT {
		t.Fatalf("血量应按加成后的体质算, 期望 %d 实际 %d", wantVIT*HPPerVIT, s.MaxHP)
	}
}

func TestStatusPercentUsesBaselineInsteadOfBuffedValue(t *testing.T) {
	base := Base{STR: 200, VIT: 24}
	statuses := NewStatusSet()
	statuses.Apply(StatusDef{ID: 2001, DurationSec: 60, Affixes: []Affix{
		{Attr: AttrAtk, Value: 50, Mode: ModeAbsolute},
	}}, 0, 1)
	statuses.Apply(StatusDef{ID: 2002, DurationSec: 60, Affixes: []Affix{
		{Attr: AttrAtk, Value: 10, Mode: ModePercent},
	}}, 0, 1)
	statuses.Apply(StatusDef{ID: 2003, DurationSec: 60, Affixes: []Affix{
		{Attr: AttrAtk, Value: 20, Mode: ModePercent},
	}}, 0, 1)
	statuses.Apply(StatusDef{ID: 2004, DurationSec: 60, Affixes: []Affix{
		{Attr: AttrVIT, Value: 10, Mode: ModeAbsolute},
		{Attr: AttrVIT, Value: 50, Mode: ModePercent},
	}}, 0, 1)

	gotBase, got := Compute(StatSource{Base: base, Status: statuses})
	// 攻击基线 100；固定 +50；两个百分比只取基线的 10+20，合计 180。
	if got.MinAtk != 180 || got.MaxAtk != 180 {
		t.Fatalf("状态不能互相乘：攻击应为 100+50+10+20=180，实际 %d~%d", got.MinAtk, got.MaxAtk)
	}
	// 体质基线 24；固定 +10；百分比只取基线的 50%=12。
	if gotBase.VIT != 46 || got.MaxHP != 46*HPPerVIT {
		t.Fatalf("六维状态不能互相乘：体质应为 46、生命 %d，实际 %d/%d",
			46*HPPerVIT, gotBase.VIT, got.MaxHP)
	}
}

func TestMonsterStatusPercentUsesTemplateBaseline(t *testing.T) {
	baseline := Stats{MinAtk: 100, MaxAtk: 100, Def: 40}
	got := ApplyStatusStats(baseline, []Affix{
		{Attr: AttrAtk, Value: 50, Mode: ModeAbsolute},
		{Attr: AttrAtk, Value: 10, Mode: ModePercent},
		{Attr: AttrAtk, Value: 20, Mode: ModePercent},
		{Attr: AttrDef, Value: -50, Mode: ModePercent},
	})
	if got.MinAtk != 180 || got.MaxAtk != 180 {
		t.Fatalf("怪物状态不能复利：攻击应为 180，实际 %d~%d", got.MinAtk, got.MaxAtk)
	}
	if got.Def != 20 {
		t.Fatalf("怪物减防应取模板基线：40-20=20，实际 %d", got.Def)
	}
}

// 多件装备的加成要累加。
func TestMultipleEquipsStack(t *testing.T) {
	a := ItemDef{ID: 6001, Equip: &EquipDef{Slot: int32(SlotHead),
		Affixes: []Affix{{Attr: AttrCrit, Value: 300}}}}
	b := ItemDef{ID: 6002, Equip: &EquipDef{Slot: int32(SlotGlove),
		Affixes: []Affix{{Attr: AttrCrit, Value: 500}}}}
	worn := NewEquipSet()
	worn.Set(SlotHead, Stack{Item: a.ID, Count: 1})
	worn.Set(SlotGlove, Stack{Item: b.ID, Count: 1})

	_, s := Compute(StatSource{Base: StartingBase(Warrior), Level: 10,
		Worn: worn, Defs: defsOf(a, b)})
	if s.CritRate != 800 {
		t.Fatalf("两件的暴击率应累加到 800, 实际 %d", s.CritRate)
	}
}

// 认不出的 attr_id 静静跳过, 不能崩也不能乱加。
func TestUnknownAffixIgnored(t *testing.T) {
	odd := ItemDef{ID: 6003, Equip: &EquipDef{Slot: int32(SlotHead),
		Affixes: []Affix{{Attr: 9999, Value: 12345}}}}
	base := StartingBase(Warrior)
	gotBase, s := Compute(StatSource{Base: base, Level: 10,
		Worn: wearing(SlotHead, odd), Defs: defsOf(odd)})
	if gotBase != base || s != DeriveFromBase(base) {
		t.Fatal("认不出的词条不该影响任何属性")
	}
}

// ── 穿戴门槛 ──

func TestRequirementMeet(t *testing.T) {
	need := Requirement{Level: 6, Base: Base{STR: 14, AGI: 14}}
	enough := Base{STR: 14, AGI: 14}
	if r := need.Meet(6, enough, 0); r != RejectNone {
		t.Errorf("刚好达标应能穿, 实际 %v", r)
	}
	if r := need.Meet(5, enough, 0); r != RejectLevelTooLow {
		t.Errorf("等级不够应报等级, 实际 %v", r)
	}
	if r := need.Meet(6, Base{STR: 13, AGI: 14}, 0); r != RejectStatTooLow {
		t.Errorf("六维不够应报属性, 实际 %v", r)
	}
	// 性别限制: 0 不限
	sexed := Requirement{Sex: 1}
	if r := sexed.Meet(1, enough, 0); r != RejectWrongSex {
		t.Errorf("性别不符应被拒, 实际 %v", r)
	}
	if r := sexed.Meet(1, enough, 1); r != RejectNone {
		t.Errorf("性别相符应能穿, 实际 %v", r)
	}
	if r := (Requirement{}).Meet(1, Base{}, 7); r != RejectNone {
		t.Errorf("无限制的装备谁都能穿, 实际 %v", r)
	}
}

// 单手/双手/盾的互斥关系。
func TestConflictSlots(t *testing.T) {
	two := ConflictSlots(SlotTwoHand)
	if len(two) != 2 {
		t.Fatalf("双手武器应与单手和盾冲突, 实际 %v", two)
	}
	if len(ConflictSlots(SlotWeapon)) != 1 || ConflictSlots(SlotWeapon)[0] != SlotTwoHand {
		t.Error("单手武器应与双手冲突")
	}
	if len(ConflictSlots(SlotShield)) != 1 {
		t.Error("盾应与双手冲突")
	}
	// 其余槽位互不干涉
	for _, s := range []EquipSlot{SlotHead, SlotBody, SlotRing, SlotShoe} {
		if len(ConflictSlots(s)) != 0 {
			t.Errorf("槽位 %d 不该与任何槽位冲突", s)
		}
	}
}

// ── EquipSet ──

func TestEquipSetBasics(t *testing.T) {
	e := NewEquipSet()
	if e.Count() != 0 || !e.At(SlotHead).Empty() {
		t.Fatal("新建的应是空的")
	}
	e.Set(SlotHead, Stack{Item: 1, Count: 1})
	if e.Count() != 1 || e.At(SlotHead).Item != 1 {
		t.Fatal("穿上之后应查得到")
	}
	// 写空 = 脱下
	e.Set(SlotHead, Stack{})
	if e.Count() != 0 {
		t.Fatal("写空应当作脱下")
	}
}

func TestEquipSetCloneIsIndependent(t *testing.T) {
	e := NewEquipSet()
	e.Set(SlotHead, Stack{Item: 1, Count: 1})
	cp := e.Clone()
	e.Set(SlotBody, Stack{Item: 2, Count: 1})

	if cp.Count() != 1 {
		t.Fatal("改原对象影响了克隆 —— 写库的 goroutine 会读到半截状态")
	}
	var nilSet *EquipSet
	if nilSet.Clone() != nil {
		t.Error("nil 的克隆应还是 nil")
	}
}

func TestEquipSetNilSafe(t *testing.T) {
	var e *EquipSet
	if e.Count() != 0 || !e.At(SlotHead).Empty() {
		t.Error("nil 的查询应返回零值")
	}
	e.Set(SlotHead, Stack{Item: 1})
	e.Each(func(EquipSlot, Stack) { t.Error("nil 不该有东西") })
}

// Compute 对 nil 的装备集也要能算。
func TestComputeNilWorn(t *testing.T) {
	b := StartingBase(Assassin)
	gotBase, s := Compute(StatSource{Base: b, Level: 1, Defs: defsOf()})
	if gotBase != b || s != DeriveFromBase(b) {
		t.Fatal("没有装备集时应等于纯推导值")
	}
}

func TestAppearanceFromEquipmentStarterSet(t *testing.T) {
	shirt := ItemDef{ID: 2875, Equip: &EquipDef{Slot: int32(SlotBody), Appearance: EquipAppearance{
		Part: AppearanceBody, Model: 143, Known: true,
	}}}
	sword := ItemDef{ID: 1011, Equip: &EquipDef{Slot: int32(SlotWeapon), Appearance: EquipAppearance{
		Part: AppearanceWeaponR, Model: 1, Known: true,
		AtkVariant: 1, WeaponCType: 2, AtkDist: 75, AttackKnown: true,
	}}}
	worn := NewEquipSet()
	worn.Set(SlotBody, Stack{Item: shirt.ID, Count: 1})
	worn.Set(SlotWeapon, Stack{Item: sword.ID, Count: 1})

	base := Appearance{Gender: 1, Hair: 2, Head: 3, EquipView: [6]uint16{999, 9, 9, 9, 9, 9}}
	got := AppearanceFromEquipment(base, worn, defsOf(shirt, sword))
	if got.EquipView != [6]uint16{143, 0, 0, 1, 0, 0} {
		t.Fatalf("初始装外观 = %v", got.EquipView)
	}
	if got.AtkVariant != 1 || got.WeaponCType != 2 || got.AtkDist != 75 {
		t.Fatalf("小剑攻击表现 = %d/%d/%d", got.AtkVariant, got.WeaponCType, got.AtkDist)
	}
	if got.Gender != 1 || got.Hair != 2 || got.Head != 3 {
		t.Fatalf("装备重算不应改建角外观: %+v", got)
	}

	// 脱下后必须从零重算，不能把 characters 行里的旧缓存继续发出去。
	got = AppearanceFromEquipment(got, NewEquipSet(), defsOf(shirt, sword))
	if got.EquipView != [6]uint16{} || got.AtkVariant != 0 || got.WeaponCType != 0 || got.AtkDist != 0 {
		t.Fatalf("脱光后外观/攻击表现未清零: %+v", got)
	}
}
