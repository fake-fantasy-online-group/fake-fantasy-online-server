package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 全部取自 ov_exceptdesc + ov_exceptdetail 真值(SQL 核对过):
//
//	中毒 1001  10秒 每2秒  attr38(生命恢复) 绝对 -10
//	昏迷 1002  12秒        控制类, 没有属性效果行
//	衰弱 1006  60秒        attr47(攻击) 百分比 -10
//	致盲 1008  60秒        attr19(命中) 百分比 -5
//	狂暴 1013  30秒        attr47(攻击) 百分比 +10
var (
	中毒 = domain.StatusDef{ID: domain.StatusPoison, Level: 1, Name: "中毒",
		DurationSec: 10, IntervalSec: 2,
		Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
	昏迷 = domain.StatusDef{ID: domain.StatusStun, Level: 1, Name: "昏迷",
		DurationSec: 12, Control: domain.ControlStun}
	封印 = domain.StatusDef{ID: domain.StatusSilence, Level: 1, Name: "封印",
		DurationSec: 10, Control: domain.ControlSilence}
	衰弱 = domain.StatusDef{ID: domain.StatusWeaken, Level: 1, Name: "衰弱",
		DurationSec: 60,
		Affixes:     []domain.Affix{{Attr: domain.AttrAtk, Value: -10, Mode: domain.ModePercent}}}
	狂暴 = domain.StatusDef{ID: domain.StatusBerserk, Level: 1, Name: "狂暴",
		DurationSec: 30,
		Affixes:     []domain.Affix{{Attr: domain.AttrAtk, Value: 10, Mode: domain.ModePercent}}}
)

func statusTable(defs ...domain.StatusDef) domain.StatusTable {
	t := domain.StatusTable{}
	for _, d := range defs {
		t[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	return t
}

// 抗性护符: 状态抗性只能从**装备**来 —— 直接写 p.Stats 会被 refreshStats 重算掉,
// 那正是"属性只有一个真相来源"这条设计在起作用。
var 抗性护符 = domain.ItemDef{ID: 8001, Name: "抗性护符", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{
	Slot:    int32(domain.SlotNeck),
	Affixes: []domain.Affix{{Attr: domain.AttrStatusRes, Value: 90}},
}}

// 壮实角色: 六维给全。只给力量的话体质是 0, 推导出来血上限也是 0,
// 角色一进来就是死的, 后面什么状态都加不上。
func beefUp(s *Scene) *entity.Entity {
	p := s.entities[1]
	p.Player.Char.Base = domain.Base{STR: 200, VIT: 100, INT: 20, SPI: 20, AGI: 20, DEX: 20}
	s.refreshStats(p)
	p.HP = p.MaxHP
	return p
}

// statusScene 建一个能施加状态的场景, 一只怪在 (140,100)。
func statusScene(t *testing.T) (*Scene, domain.EntityID, *fakeSink) {
	t.Helper()
	defs := spawn.MapDefs{技能怪.ID: 技能怪}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 技能怪.ID, Pos: domain.Pos{MapID: 14, X: 140, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 21,
		Spawner: sp, Defs: defs,
		Statuses: statusTable(中毒, 昏迷, 封印, 衰弱, 狂暴),
		Skills:   skillTable(火弹术),
		Items:    map[domain.ItemID]domain.ItemDef{抗性护符.ID: 抗性护符}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()

	var mid domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			mid = id
		}
	}
	return s, mid, sink
}

// 施加状态: 事件广播, 属性立刻变。
func TestApplyStatusChangesStats(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	before := p.Stats.MinAtk
	sink.take()

	if !s.applyStatus(p, domain.StatusWeaken, 1, 0) {
		t.Fatal("应能施加衰弱")
	}
	s.step()

	if p.Stats.MinAtk != before-before/10 {
		t.Fatalf("衰弱应让攻击 -10%%: %d → %d", before, p.Stats.MinAtk)
	}
	ev, ok := firstOf[event.StatusApplied](sink.take())
	if !ok {
		t.Fatal("施加状态要广播")
	}
	if ev.Status != int32(domain.StatusWeaken) || ev.DurationMS != 60_000 {
		t.Errorf("事件内容不对: %+v", ev)
	}
}

// 增益与减益走同一条路径, 只是符号相反。
func TestBuffAndDebuffSamePipeline(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	base := p.Stats.MinAtk

	s.applyStatus(p, domain.StatusBerserk, 1, 0)
	s.step()
	if p.Stats.MinAtk <= base {
		t.Fatalf("狂暴应加攻击: %d → %d", base, p.Stats.MinAtk)
	}
	buffed := p.Stats.MinAtk

	s.applyStatus(p, domain.StatusWeaken, 1, 0)
	s.step()
	if p.Stats.MinAtk >= buffed {
		t.Fatalf("再中衰弱应把攻击拉回去: %d → %d", buffed, p.Stats.MinAtk)
	}
}

func TestClearHarmfulAndMaxHPFloor(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	baseMax := p.MaxHP
	bad := domain.StatusDef{
		ID: 1991, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 60,
		Affixes: []domain.Affix{{Attr: domain.AttrAtk, Value: -50, Mode: domain.ModePercent}},
	}
	if !s.applyStatusDef(p, bad, 0) || !p.Status.Has(bad.ID) {
		t.Fatal("测试减益应先成功施加")
	}
	clear := domain.StatusDef{
		ID: 1055, Level: 1, TypeKnown: true, Type: domain.StatusGood,
		DurationSec: 5, ClearHarmful: true,
	}
	if !s.applyStatusDef(p, clear, p.ID) || p.Status.Has(bad.ID) {
		t.Fatal("咒法·褪应移除已有减益")
	}

	maxHP := domain.StatusDef{
		ID: 1181, Level: 1, TypeKnown: true, Type: domain.StatusGood,
		DurationSec: 180, MaxHPFloor: 15000,
	}
	if !s.applyStatusDef(p, maxHP, p.ID) || p.MaxHP != 15000 {
		t.Fatalf("提升生命后上限应为 15000，实际 %d", p.MaxHP)
	}
	p.Status.Remove(maxHP.ID)
	s.refreshEntityStats(p)
	if p.MaxHP != baseMax {
		t.Fatalf("状态移除后应回到基础上限 %d，实际 %d", baseMax, p.MaxHP)
	}
}

func TestManaToHealthTick(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	p.HP -= 100
	p.MP = 10
	sink.take()
	s.tickStatus(p, domain.Active{Def: domain.StatusDef{
		ID: 1070, Level: 1, ManaToHealthRate: 3,
	}})
	s.flush()
	if p.MP != 9 || p.HP != p.MaxHP-97 {
		t.Fatalf("每跳应消耗 1 法力恢复 3 生命，当前 HP/MP=%d/%d", p.HP, p.MP)
	}
	if _, ok := firstOf[event.HealDone](sink.take()); !ok {
		t.Fatal("法力转生命应发治疗事件")
	}
}

func TestStatusPositionModeMatchesClientAcceptance(t *testing.T) {
	orbit := []domain.StatusID{
		domain.StatusPoison, domain.StatusSilence, domain.StatusWeaken, domain.StatusDampen,
		domain.StatusBlind, domain.StatusBerserk, 1010, 1011, domain.StatusIronSkin,
		1014, 1015, 1017, 1019, 1029, 1182, 1208, 1209, 1210, 1211, 1212, 1213, 1214,
	}
	for _, id := range orbit {
		if got := statusPositionMode(domain.StatusDef{ID: id}); got != domain.StatusPositionOrbit {
			t.Errorf("状态 %d 应环绕人物，实际 mode=%d", id, got)
		}
	}

	ground := []domain.StatusID{
		domain.StatusStun, domain.StatusFreeze, domain.StatusPetrify, domain.StatusSlow,
		1016, 1018, 1020, 1021, 1022, 1023, 1024, 1025, 1026, 1027,
		1028, 1032, 1034, 1036, 1037, 1038, 1039, 1055, 1068, 1070, 1074, 1081, 1095, 1106,
		1116, 1145, 1154, 1158, 1159, 1174, 1175, 1176, 1178, 1180, 1181, 1184,
		1188, 1206, 1207,
	}
	for _, id := range ground {
		if got := statusPositionMode(domain.StatusDef{ID: id}); got != 0 {
			t.Errorf("状态 %d 应落在脚底，实际 mode=%d", id, got)
		}
	}

	// 没有特效预览、也没有得到人工结论的状态必须保留旧分类行为。
	if got := statusPositionMode(domain.StatusDef{
		ID: 1030, TypeKnown: true, Type: domain.StatusBad,
	}); got != domain.StatusPositionOrbit {
		t.Errorf("未确认状态不应被擅自改成脚底，实际 mode=%d", got)
	}
}

// 状态到期要还原属性并广播。
func TestStatusExpires(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	before := p.Stats.MinAtk

	s.applyStatus(p, domain.StatusWeaken, 1, 0)
	s.step()
	sink.take()
	if p.Stats.MinAtk == before {
		t.Fatal("衰弱没生效, 后面的到期检查没意义")
	}

	// 60 秒 = 600 帧
	for i := 0; i < 601; i++ {
		s.step()
	}
	if p.Stats.MinAtk != before {
		t.Fatalf("到期后攻击应还原到 %d, 实际 %d", before, p.Stats.MinAtk)
	}
	if p.Status.Has(domain.StatusWeaken) {
		t.Error("到期的状态应被清掉")
	}
	if _, ok := firstOf[event.StatusRemoved](sink.take()); !ok {
		t.Error("状态到期要广播")
	}
}

// 中毒: 每 2 秒掉 10 点血, 持续 10 秒 = 掉 5 次。
func TestPoisonTicks(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	s.applyStatus(p, domain.StatusPoison, 1, 0)
	full := p.HP // 施加之后再记 —— applyStatus 会重算属性
	sink.take()

	// 跑满 10 秒
	for i := 0; i < 101; i++ {
		s.step()
	}
	// 每 2 秒一跳, 10 秒内跳 5 次 × 10 点 = 50
	if p.HP != full-50 {
		t.Fatalf("中毒 10 秒应掉 50 点血(从 %d 到 %d), 实际剩 %d", full, full-50, p.HP)
	}
	n := countOf[event.DamageDealt](sink.take())
	if n != 5 {
		t.Fatalf("应跳 5 次伤害, 实际 %d 次", n)
	}
	if p.Status.Has(domain.StatusPoison) {
		t.Error("10 秒后中毒应已到期")
	}
}

// 中毒能把人毒死, 而且要走完整的死亡流程。
func TestPoisonCanKill(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	s.applyStatus(p, domain.StatusPoison, 1, 0)
	p.HP = 5 // 施加之后再压血, 否则会被属性重算覆盖
	sink.take()

	for i := 0; i < 25; i++ {
		s.step()
	}
	evs := sink.take()
	dmg, ok := firstOf[event.DamageDealt](evs)
	if !ok || !dmg.Flag.Has(event.DamageFatal) {
		t.Fatal("中毒应能致死")
	}
	if _, ok := firstOf[event.EntityDied](evs); !ok {
		t.Fatal("毒死也要走死亡流程")
	}
}

// ── 控制 ──

// 昏迷: 不能攻击、不能移动、不能放技能。
func TestStunBlocksEverything(t *testing.T) {
	s, mid, sink := statusScene(t)
	p := beefUp(s)
	p.Player.Char.Skills = domain.Learned{火弹术.ID: 1}
	p.MP = p.MaxMP
	s.applyStatus(p, domain.StatusStun, 1, 0)
	s.step()
	sink.take()
	startPos := p.Pos

	// 攻击被拒
	s.exec(Attack{ID: 1, Target: mid})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectStunned {
		t.Fatalf("昏迷中攻击应回 RejectStunned, 实际 %+v", rej)
	}

	// 技能被拒
	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mid})
	s.step()
	rej, ok = firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectStunned {
		t.Fatalf("昏迷中放技能应回 RejectStunned, 实际 %+v", rej)
	}

	// 移动无效(移动不给回执 —— 客户端每秒发好几次, 回执只会刷屏)
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 14, X: 500, Y: 500}})
	s.step()
	if p.Pos != startPos {
		t.Fatalf("昏迷中不该能移动, 从 %v 挪到了 %v", startPos, p.Pos)
	}
}

// 封印: 放不了技能, 但能普通攻击、能走。
func TestSilenceOnlyBlocksSkills(t *testing.T) {
	s, mid, sink := statusScene(t)
	p := beefUp(s)
	p.Player.Char.Skills = domain.Learned{火弹术.ID: 1}
	p.MP = p.MaxMP
	s.applyStatus(p, domain.StatusSilence, 1, 0)
	s.entities[1].Stats.Hit = 1 << 20 // 保证普通攻击必中(施加状态之后再设)
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mid})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectSilenced {
		t.Fatalf("封印中放技能应回 RejectSilenced, 实际 %+v", rej)
	}

	// 普通攻击照常
	s.exec(Attack{ID: 1, Target: mid})
	s.step()
	if _, ok := firstOf[event.DamageDealt](sink.take()); !ok {
		t.Fatal("封印不该挡住普通攻击")
	}

	// 移动照常
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 14, X: 200, Y: 200}})
	s.step()
	if p.Pos.X != 200 {
		t.Fatal("封印不该挡住移动")
	}
}

// 昏迷的怪站着不动、也不还手。
func TestStunnedMonsterDoesNothing(t *testing.T) {
	defs := spawn.MapDefs{主动树妖.ID: 主动树妖}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 主动树妖.ID, Pos: domain.Pos{MapID: 14, X: 300, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 3,
		Spawner: sp, Defs: defs, Statuses: statusTable(昏迷)})
	join(t, s, 1, 100, "甲", 100, 100)
	var mid domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			mid = id
		}
	}
	m := s.entities[mid]
	s.applyStatus(m, domain.StatusStun, 1, 0)
	start := m.Pos

	for i := 0; i < 40; i++ {
		s.step()
	}
	if m.Pos != start {
		t.Fatalf("昏迷的怪不该移动, 从 %v 挪到了 %v", start, m.Pos)
	}
}

// 状态到期后恢复行动。
func TestStunWearsOff(t *testing.T) {
	s, mid, sink := statusScene(t)
	p := beefUp(s)
	s.applyStatus(p, domain.StatusStun, 1, 0)
	p.Stats.Hit = 1 << 20 // 施加之后再设, 否则被属性重算覆盖

	// 昏迷 12 秒 = 120 帧
	for i := 0; i < 121; i++ {
		s.step()
	}
	sink.take()
	if p.Status.Has(domain.StatusStun) {
		t.Fatal("12 秒后昏迷应已解除")
	}
	s.exec(Attack{ID: 1, Target: mid})
	s.step()
	if _, ok := firstOf[event.DamageDealt](sink.take()); !ok {
		t.Fatal("昏迷解除后应能攻击")
	}
}

// ── 抗性 ──

// **不良状态抗性第一次真的起作用。** 1404 条装备词条带这个属性,
// 在此之前没有任何东西会施加状态, 所以那个数是死的。
func TestStatusResistBlocksDebuff(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	// 抗性来自装备。直接写 p.Stats 会被 refreshStats 重算掉 ——
	// 那正是"属性只有一个真相来源"这条设计在起作用。
	p.Player.Worn.Set(domain.SlotNeck, domain.Stack{Item: 抗性护符.ID, Count: 1})
	s.refreshStats(p)
	if p.Stats.StatusResist != 90 {
		t.Fatalf("护符应给 90 抗性, 实际 %d", p.Stats.StatusResist)
	}
	sink.take()

	blocked := 0
	for i := 0; i < 50; i++ {
		if !s.applyStatus(p, domain.StatusWeaken, 1, 0) {
			blocked++
		}
		p.Status.Remove(domain.StatusWeaken)
	}
	if blocked < 30 {
		t.Fatalf("90%% 抗性下 50 次只挡住 %d 次, 抗性没生效", blocked)
	}
	s.step()
	if _, ok := firstOf[event.StatusResisted](sink.take()); !ok {
		t.Error("被抗性挡下要通知本人")
	}
}

// 抗性只挡 debuff, 不挡自己身上的增益 —— 挡了就成了负面效果。
func TestResistDoesNotBlockBuffs(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	p.Player.Worn.Set(domain.SlotNeck, domain.Stack{Item: 抗性护符.ID, Count: 1})
	s.refreshStats(p)

	for i := 0; i < 20; i++ {
		if !s.applyStatus(p, domain.StatusBerserk, 1, 0) {
			t.Fatal("抗性不该挡住增益状态")
		}
	}
}

// 没有抗性时必中。
func TestNoResistAlwaysApplies(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	for i := 0; i < 20; i++ {
		if !s.applyStatus(p, domain.StatusWeaken, 1, 0) {
			t.Fatal("没有抗性时不该被挡")
		}
	}
}

// 重复施加是刷新时长, 不是叠加。
func TestReapplyRefreshes(t *testing.T) {
	s, _, _ := statusScene(t)
	p := beefUp(s)
	base := p.Stats.MinAtk

	s.applyStatus(p, domain.StatusWeaken, 1, 0)
	s.step()
	once := p.Stats.MinAtk

	s.applyStatus(p, domain.StatusWeaken, 1, 0)
	s.applyStatus(p, domain.StatusWeaken, 1, 0)
	s.step()
	if p.Stats.MinAtk != once {
		t.Fatalf("重复施加应只刷新时长而不叠加: 一次 %d, 三次 %d (裸 %d)",
			once, p.Stats.MinAtk, base)
	}
	if p.Status.Count() != 1 {
		t.Fatalf("同一个状态只该占一个位置, 实际 %d 个", p.Status.Count())
	}
}

// 没配状态表的场景不该崩。
func TestNoStatusTableIsSafe(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000})
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	if s.applyStatus(s.entities[1], domain.StatusStun, 1, 0) {
		t.Fatal("没有状态表时不该加得上")
	}
	s.step() // 不崩就算过
}
