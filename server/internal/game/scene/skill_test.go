package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 全部取自 ov_skilldesc 真值(SQL 核对过):
//
//	火弹术 10401  术士  10级  208%伤害  31法力  距离450  火(8)
//	火球术 10402  术士  20级  195%伤害  64法力  距离450  半径100  火(8)
//	治愈术 10301  药师  10级  恢复上限10%+100  23法力  距离450  圣(12)
//	连环枪法 10002 战士  1级   130%伤害  8法力   物理(1)  (10001 是枪法修炼, 被动)
var (
	火弹术 = domain.SkillDef{
		ID: 10401, Level: 1, Name: "火弹术", Prof: 5, Kind: domain.SkillSingle,
		LevelNeed: 10, HurtType: domain.HurtFire, MPCost: 31, Dist: 450,
		Effect:      domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 208},
		TargetEnemy: true,
	}
	火球术 = domain.SkillDef{
		ID: 10402, Level: 1, Name: "火球术", Prof: 5, Kind: domain.SkillArea,
		LevelNeed: 20, HurtType: domain.HurtFire, MPCost: 64, Dist: 450, Radius: 100,
		Area: domain.SkillAreaTargeting{
			Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: 100,
		},
		Effect:      domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 195},
		TargetEnemy: true,
	}
	治愈术 = domain.SkillDef{
		ID: 10301, Level: 1, Name: "治愈术", Prof: 4, Kind: domain.SkillSingle,
		LevelNeed: 10, HurtType: domain.HurtHoly, MPCost: 23, Dist: 450,
		Effect:     domain.SkillEffect{Kind: domain.EffectHeal, HealPctOfMax: 10, HealFlat: 100},
		TargetSelf: true, TargetTeam: true,
	}
	祝福术 = domain.SkillDef{
		ID: 10307, Level: 1, Name: "祝福术", Prof: 4, Kind: domain.SkillArea,
		LevelNeed: 15, MPCost: 35, Radius: 200,
		Area: domain.SkillAreaTargeting{
			Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 200,
		},
		Statuses: []domain.StatusApplication{{
			ID: 1014, Level: 1, DurationSec: 300,
			ExtraAffixes: []domain.Affix{{
				Attr: domain.AttrMAtk, Value: 5, Mode: domain.ModePercent,
			}},
			Description: "攻击力上升10%；魔法攻击力上升5%。",
		}},
		TargetSelf: true, TargetTeam: true,
	}
	连环枪法 = domain.SkillDef{
		ID: 10002, Level: 1, Name: "连环枪法", Prof: 1, Kind: domain.SkillSingle,
		LevelNeed: 1, HurtType: domain.HurtPhysical, MPCost: 8, Dist: 75,
		Effect:      domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 130},
		TargetEnemy: true,
	}
	// 隐身术那一类: 定义在, 但 desc 解不出效果(等状态系统)
	隐身术 = domain.SkillDef{
		ID: 10203, Level: 1, Name: "隐身术", Prof: 3, Kind: domain.SkillSingle,
		LevelNeed: 30, MPCost: 40, Effect: domain.SkillEffect{Kind: domain.EffectNone},
		TargetSelf: true,
	}
	剑术修炼 = domain.SkillDef{
		ID: 10101, Level: 1, Name: "剑术修炼", Prof: 2, Kind: domain.SkillPassive,
		Effect: domain.SkillEffect{Kind: domain.EffectNone},
	}
)

func skillTable(defs ...domain.SkillDef) domain.SkillTable {
	t := domain.SkillTable{}
	for _, d := range defs {
		t[domain.SkillKey{ID: d.ID, Level: d.Level}] = d
	}
	return t
}

var 技能怪 = domain.MonsterDef{
	ID: 1001, Name: "树妖", Kind: domain.MonsterNormal, Level: 14, HP: 2000, Exp: 58,
	Stats: domain.NewMonsterStats(14, 6, 42, 0, 0, 2000, 110),
}

// skillScene 建一个能放技能的场景, 一只怪在 (140,100)。
func skillScene(t *testing.T, monsters ...domain.Pos) (*Scene, []domain.EntityID) {
	t.Helper()
	if len(monsters) == 0 {
		monsters = []domain.Pos{{MapID: 14, X: 140, Y: 100}}
	}
	defs := spawn.MapDefs{技能怪.ID: 技能怪}
	var pts []domain.SpawnPoint
	for i, p := range monsters {
		pts = append(pts, domain.SpawnPoint{ID: int32(i + 1), Monster: 技能怪.ID, Pos: p})
	}
	sp, _ := spawn.New(pts, defs, domain.NewEntityAlloc())
	statuses := domain.StatusTable{
		{ID: 1014, Level: 1}: {
			ID: 1014, Level: 1, Name: "强化", DurationSec: 60,
			Affixes: []domain.Affix{{
				Attr: domain.AttrAtk, Value: 10, Mode: domain.ModePercent,
			}},
		},
	}
	s := New(Config{ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 11,
		Spawner: sp, Defs: defs,
		Skills:   skillTable(火弹术, 火球术, 治愈术, 祝福术, 连环枪法, 隐身术, 剑术修炼),
		Statuses: statuses})

	var ids []domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			ids = append(ids, id)
		}
	}
	if len(ids) != len(monsters) {
		t.Fatalf("应刷出 %d 只怪, 实际 %d", len(monsters), len(ids))
	}
	return s, ids
}

// caster 让玩家学会技能并给足法力。
func caster(s *Scene, learned domain.Learned) {
	p := s.entities[1]
	p.Player.Char.Skills = learned
	p.Stats.MAtk = 100
	p.Stats.MinAtk, p.Stats.MaxAtk = 50, 50
	p.Stats.Hit = 1 << 20
	p.MaxMP, p.MP = 1000, 1000
}

// 法系技能: 不判命中, 按魔攻 × 倍率算。
func TestUseMagicSkill(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()

	ev, ok := firstOf[event.DamageDealt](sink.take())
	if !ok {
		t.Fatal("应打出伤害")
	}
	if ev.SkillID != int32(火弹术.ID) {
		t.Errorf("技能 id 应透传, 实际 %d", ev.SkillID)
	}
	if !ev.Flag.Has(event.DamageMagic) {
		t.Error("火系技能应标记为法术伤害")
	}
	if ev.Flag.Has(event.DamageMiss) {
		t.Error("法系技能不该 miss")
	}
	// 魔攻100 × 208% = 208, 怪没有魔防
	if ev.Amount != 208 {
		t.Fatalf("魔攻100 × 208%% 应打 208, 实际 %d", ev.Amount)
	}
	if s.entities[1].MP != 1000-31 {
		t.Fatalf("应扣 31 法力, 实际剩 %d", s.entities[1].MP)
	}
}

func TestBlessingUsesOneStatusAndAppliesBothAttackBonuses(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	p := s.entities[1]
	p.Player.Char.Skills = domain.Learned{祝福术.ID: 1}
	p.MaxMP, p.MP = 1000, 1000
	s.step()
	sink.take()
	baseAtk, baseMAtk := p.Stats.MaxAtk, p.Stats.MAtk

	s.exec(UseSkill{ID: 1, Skill: 祝福术.ID})
	s.step()

	if !p.Status.Has(1014) || p.Status.Count() != 1 {
		t.Fatalf("祝福术应只施加一枚强化状态，实际状态数 %d", p.Status.Count())
	}
	if want := baseAtk + baseAtk*10/100; p.Stats.MaxAtk != want {
		t.Fatalf("祝福术物攻应按基础值 +10%%：want=%d got=%d", want, p.Stats.MaxAtk)
	}
	if want := baseMAtk + baseMAtk*5/100; p.Stats.MAtk != want {
		t.Fatalf("祝福术魔攻应按基础值 +5%%：want=%d got=%d", want, p.Stats.MAtk)
	}
	if got := len((p.Status.Affixes())); got != 2 {
		t.Fatalf("同一强化状态应包含物攻与魔攻两项，实际 %d", got)
	}
	var active domain.Active
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1014 {
			active = a
		}
	})
	if active.Def.ID == 0 || active.Def.Desc != "攻击力上升10%；魔法攻击力上升5%。" {
		t.Fatalf("祝福术状态说明没有展示完整效果: %+v", active.Def)
	}
}

// 物理技能照常判命中, 而且吃倍率。
func TestUsePhysicalSkill(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "战士", 100, 100)
	caster(s, domain.Learned{连环枪法.ID: 1})
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 连环枪法.ID, Target: mons[0]})
	s.step()

	ev, ok := firstOf[event.DamageDealt](sink.take())
	if !ok {
		t.Fatal("应打出伤害")
	}
	if ev.Flag.Has(event.DamageMagic) {
		t.Error("物理技能不该标记为法术")
	}
	// 攻击 50 × 130% = 65
	if ev.Amount != 65 {
		t.Fatalf("攻击50 × 130%% 应打 65, 实际 %d", ev.Amount)
	}
}

// **治疗**: HealDone 事件至今没有任何代码发出过, 这是第一次。
func TestHealSkill(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	p := s.entities[1]
	p.MaxHP, p.HP = 1000, 300
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: 1})
	s.step()

	ev, ok := firstOf[event.HealDone](sink.take())
	if !ok {
		t.Fatal("应发出治疗事件")
	}
	// 上限 1000 的 10% + 100 = 200
	if ev.Amount != 200 {
		t.Fatalf("上限10%%+100 应回 200, 实际 %d", ev.Amount)
	}
	if p.HP != 500 {
		t.Fatalf("血量应从 300 涨到 500, 实际 %d", p.HP)
	}
	if ev.DstHP != 500 {
		t.Errorf("事件里的剩余血应是 500, 实际 %d", ev.DstHP)
	}
}

// 治疗不能超上限, 而且报的是**实际回了多少** —— 否则客户端飘的数和血条对不上。
func TestHealCappedAtMaxHP(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	p := s.entities[1]
	p.MaxHP, p.HP = 1000, 950
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: 1})
	s.step()

	ev, _ := firstOf[event.HealDone](sink.take())
	if p.HP != 1000 {
		t.Fatalf("应补到满 1000, 实际 %d", p.HP)
	}
	if ev.Amount != 50 {
		t.Fatalf("只回得了 50 点, 事件里却报 %d —— 客户端飘的数会和血条对不上", ev.Amount)
	}
}

// 范围技能打到半径内的所有怪。
func TestAreaSkillHitsMultiple(t *testing.T) {
	s, mons := skillScene(t,
		domain.Pos{MapID: 14, X: 200, Y: 100}, // 中心
		domain.Pos{MapID: 14, X: 250, Y: 100}, // 距中心 50, 半径 100 内
		domain.Pos{MapID: 14, X: 500, Y: 100}) // 距中心 300, 圈外
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火球术.ID: 1})
	s.step()
	sink.take()

	// 以第一只怪为中心
	var center domain.EntityID
	for _, id := range mons {
		if s.entities[id].Pos.X == 200 {
			center = id
		}
	}
	s.exec(UseSkill{ID: 1, Skill: 火球术.ID, Target: center})
	s.step()

	n := countOf[event.DamageDealt](sink.take())
	if n != 2 {
		t.Fatalf("半径100内应打到 2 只怪, 实际 %d 只", n)
	}
}

// 没学的技能放不出来。
func TestSkillNotLearned(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	caster(s, domain.Learned{}) // 什么都没学
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectSkillNotLearned {
		t.Fatalf("没学的技能应回 RejectSkillNotLearned, 实际 %+v", rej)
	}
}

// 被动技能与效果还没做的技能都放不出来 ——
// 放一个什么都不发生的技能只会白扣法力。
func TestUnusableSkillsRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  domain.SkillDef
	}{
		{"被动技能", 剑术修炼},
		{"效果待做的(buff/位移)", 隐身术},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := skillScene(t)
			sink := join(t, s, 1, 100, "甲", 100, 100)
			caster(s, domain.Learned{tc.def.ID: 1})
			mpBefore := s.entities[1].MP
			s.step()
			sink.take()

			s.exec(UseSkill{ID: 1, Skill: tc.def.ID, Target: 1})
			s.step()

			rej, ok := firstOf[event.Rejected](sink.take())
			if !ok || rej.Reason != event.RejectSkillNotUsable {
				t.Fatalf("应回 RejectSkillNotUsable, 实际 %+v", rej)
			}
			if s.entities[1].MP != mpBefore {
				t.Error("被拒的技能不该扣法力")
			}
		})
	}
}

// 法力不够放不出来。
func TestNotEnoughMP(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.entities[1].MP = 5 // 火弹术要 31
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectNotEnoughMP {
		t.Fatalf("法力不够应回 RejectNotEnoughMP, 实际 %+v", rej)
	}
	if s.entities[1].MP != 5 {
		t.Error("被拒时不该扣法力")
	}
}

// 同一帧连发两次: 第二次要被法力/冷却拦住, 不能两个都放出去。
// 法力**在收到命令时就扣**, 就是为了拦住这个。
func TestDoubleCastSameFrameBlocked(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.entities[1].MP = 40 // 只够放一次(31)
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()

	evs := sink.take()
	if n := countOf[event.DamageDealt](evs); n != 1 {
		t.Fatalf("法力只够放一次, 实际放出了 %d 次", n)
	}
	if s.entities[1].MP != 9 {
		t.Fatalf("应只扣一次 31, 剩 9, 实际 %d", s.entities[1].MP)
	}
}

// 够不着放不出来 —— 不判距离的话客户端能隔着半张图放技能。
func TestSkillOutOfRange(t *testing.T) {
	s, mons := skillScene(t, domain.Pos{MapID: 14, X: 700, Y: 100}) // 距离 600 > 450
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()
	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectNoTarget {
		t.Fatalf("够不着应被拒, 实际 %+v", rej)
	}
}

// 治疗技能不能奶怪 —— 目标合法性由服务端判, 客户端说了不算。
func TestHealCannotTargetMonster(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	s.entities[mons[0]].HP = 100
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: mons[0]})
	s.step()

	if _, ok := firstOf[event.HealDone](sink.take()); ok {
		t.Fatal("治疗技能不该能奶怪")
	}
	if s.entities[mons[0]].HP != 100 {
		t.Fatal("怪的血被治疗了")
	}
}

// 伤害技能不能打自己。
func TestDamageSkillCannotTargetSelf(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	p := s.entities[1]
	p.MaxHP, p.HP = 1000, 1000
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: 1})
	s.step()

	if _, ok := firstOf[event.DamageDealt](sink.take()); ok {
		t.Fatal("伤害技能不该能打自己")
	}
	if p.HP != 1000 {
		t.Fatal("自己被自己打了")
	}
}

// 技能与普通攻击是**两条独立队列**：受理技能既不重置普攻计时，普攻也不会被
// 技能冷却拒绝；技能自己仍按 CooldownMS 冷却。
//
// 依据 d8f7a88「decouple player skill admission from basic attack cooldown」：
// 客户端 RequestSkill 与普攻各按自己的节拍发，服务端不能拿上一刀的 nextAt
// 去拒绝客户端已经正常发出的技能请求，反过来也一样。
func TestSkillCooldownDoesNotBlockNormalAttack(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.entities[1].Stats.AtkSpeedMS = 1500 // 15 帧
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()
	sink.take()

	// 技能刚放完就出普攻：应该照常受理，不能被技能冷却挡下
	s.exec(Attack{ID: 1, Target: mons[0]})
	s.step()
	evs := sink.take()
	if rej, ok := firstOf[event.Rejected](evs); ok && rej.Reason == event.RejectOnCooldown {
		t.Fatal("技能不该阻塞普攻：技能准入已与普攻冷却解耦")
	}
	if _, ok := firstOf[event.DamageDealt](evs); !ok {
		t.Fatal("技能之后普攻应被受理并造成伤害")
	}
}

// 技能打死怪要走完整的死亡流程(经验、掉落、脱战、重生)。
func TestSkillKillTriggersDeath(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.entities[1].Stats.MAtk = 100000 // 一发秒杀
	s.step()
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()

	evs := sink.take()
	dmg, _ := firstOf[event.DamageDealt](evs)
	if !dmg.Flag.Has(event.DamageFatal) {
		t.Fatal("应打死它")
	}
	if _, ok := firstOf[event.EntityDied](evs); !ok {
		t.Fatal("技能击杀也要走死亡流程")
	}
}

// 死人放不了技能。
func TestDeadCannotCast(t *testing.T) {
	s, mons := skillScene(t)
	sink := join(t, s, 1, 100, "术士", 100, 100)
	caster(s, domain.Learned{火弹术.ID: 1})
	s.step()
	s.entities[1].HP = 0
	sink.take()

	s.exec(UseSkill{ID: 1, Skill: 火弹术.ID, Target: mons[0]})
	s.step()
	if n := countOf[event.DamageDealt](sink.take()); n != 0 {
		t.Fatal("死人不该放得出技能")
	}
}

// 没配技能表的场景不该崩。
func TestNoSkillTableIsSafe(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000})
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	s.exec(UseSkill{ID: 1, Skill: 10401, Target: 0})
	s.step() // 不崩就算过
}
