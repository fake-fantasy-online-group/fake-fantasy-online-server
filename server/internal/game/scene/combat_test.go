package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 战斗在**场景**这一层要保证的事，和在 combat 包里保证的事不一样:
// combat 管「这一刀打多少」，场景管「什么时候算、算完谁收得到、死了之后怎么收场」。

// putMonster 直接往场景里塞一只怪。刷怪系统接进来之前的脚手架。
func putMonster(s *Scene, id domain.EntityID, hp int32, st domain.Stats, x, y float64) *entity.Entity {
	e := &entity.Entity{
		ID: id, Kind: domain.KindMonster, Name: "小蜗牛怪", Level: 1,
		HP: hp, MaxHP: hp, Stats: st,
		Pos:     domain.Pos{MapID: 7, X: x, Y: y},
		Monster: &entity.Monster{TypeID: 338},
	}
	s.entities[id] = e
	s.aoi.Enter(e)
	return e
}

// 必中的攻击者: 命中拉满, 伤害固定, 便于断言具体数字。
var sureHit = domain.Stats{MinAtk: 7, MaxAtk: 7, Hit: 1 << 20}

func newCombatScene(t *testing.T) *Scene {
	t.Helper()
	return New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 12345})
}

// 进图的玩家必须是活的。
//
// 这条是**回归测试**: onEnter 曾经拿零值六维去推导, 得到「最大生命 = 0 × 9 = 0」——
// 玩家一进图就是尸体, 而且死得极隐蔽: 不报错、不打日志, 只是所有攻击都因为
// 「攻击者已死」被静静跳过。是写战斗测试时才撞出来的。
func TestPlayerEntersAlive(t *testing.T) {
	s := newCombatScene(t)
	for race, name := range map[domain.Race]string{
		domain.Warrior: "战士", domain.Swordsman: "剑客", domain.Assassin: "刺客",
		domain.Healer: "药师", domain.Warlock: "术士",
	} {
		id := domain.EntityID(race) + 1
		s.exec(Enter{ID: id, Char: &domain.Character{
			ID: int64(id), Name: name, Race: race, Level: 1,
			Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
		}, Sink: &fakeSink{}})
		e := s.entities[id]
		if e == nil {
			t.Fatalf("%s 没进去", name)
		}
		if e.MaxHP <= 0 || e.HP <= 0 {
			t.Errorf("%s 进图时血量是 %d/%d —— 生下来就是尸体", name, e.HP, e.MaxHP)
		}
		if !e.Alive() {
			t.Errorf("%s 进图即死", name)
		}
	}
}

// 攻击是**排队到帧末结算**的, 不是收到命令就算。
// 这是公平性的来源: 同一帧内谁的包先到不影响结果。
func TestAttackResolvesInStepNotOnCommand(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = sureHit
	m := putMonster(s, 900, 30, domain.Stats{}, 110, 110)
	s.step()

	s.exec(Attack{ID: 1, Target: 900})
	if m.HP != 30 {
		t.Fatalf("命令刚进来时不该已经扣血, 实际剩 %d", m.HP)
	}
	s.step()
	if m.HP != 23 {
		t.Fatalf("这一帧结算后应剩 23(30−7), 实际 %d", m.HP)
	}
}

// 打怪的中间伤害只发给攻击者 —— 旁观者不需要每一跳的飘字；
// 致命那一击仍要广播，客户端靠它播死亡表现。
//
// 依据 7f2756c「adopt reviewed gameplay rules」收窄的全图实体广播边界：
// 玩家之间的伤害不受影响，只有“打怪”的中间结果从全图广播改成回攻击者。
func TestDamageBroadcastToWatchers(t *testing.T) {
	s := newCombatScene(t)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150) // 旁观者, 同格
	s.entities[1].Stats = sureHit
	putMonster(s, 900, 30, domain.Stats{}, 110, 110)
	s.step()
	sinkA.take()
	sinkB.take()

	s.exec(Attack{ID: 1, Target: 900})
	s.step()

	ev, ok := firstOf[event.DamageDealt](sinkA.take())
	if !ok {
		t.Fatal("攻击者应收到伤害事件")
	}
	if ev.Src != 1 || ev.Dst != 900 || ev.Amount != 7 || ev.DstHP != 23 {
		t.Errorf("攻击者收到的伤害事件不对: %+v", ev)
	}
	if _, leaked := firstOf[event.DamageDealt](sinkB.take()); leaked {
		t.Error("打怪的中间伤害不该广播给旁观者")
	}
}

// 致命一击是广播边界的例外：同图玩家都要收到，否则看不到怪的死亡表现。
func TestFatalDamageIsBroadcastToWatchers(t *testing.T) {
	s := newCombatScene(t)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 150, 150)
	s.entities[1].Stats = sureHit
	putMonster(s, 900, 7, domain.Stats{}, 110, 110) // 7 点血, 一刀致死
	s.step()
	sinkA.take()
	sinkB.take()

	s.exec(Attack{ID: 1, Target: 900})
	s.step()

	if _, ok := firstOf[event.DamageDealt](sinkB.take()); !ok {
		t.Fatal("致命一击应广播给同图玩家")
	}
}

// 攻击冷却按**帧**算。攻速 1500ms = 15 帧, 期间再打要被拒。
func TestAttackCooldownInTicks(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	e := s.entities[1]
	e.Stats = sureHit
	e.Stats.AtkSpeedMS = 1500 // 15 帧
	putMonster(s, 900, 100000, domain.Stats{}, 110, 110)
	s.step()
	sink.take()

	s.exec(Attack{ID: 1, Target: 900})
	s.step()
	sink.take()

	// 冷却里再打: 收到拒绝, 不产生伤害
	s.exec(Attack{ID: 1, Target: 900})
	s.step()
	evs := sink.take()
	rej, ok := firstOf[event.Rejected](evs)
	if !ok || rej.Reason != event.RejectOnCooldown {
		t.Fatalf("冷却中再打应被拒, 实际 %+v", evs)
	}
	if countOf[event.DamageDealt](evs) != 0 {
		t.Fatal("冷却中不该产生伤害")
	}

	// 推到第 15 帧之后再打, 应该成功
	for i := 0; i < 15; i++ {
		s.step()
	}
	sink.take()
	s.exec(Attack{ID: 1, Target: 900})
	s.step()
	if countOf[event.DamageDealt](sink.take()) != 1 {
		t.Fatal("冷却过了应该能打")
	}
}

// 打不存在的目标 / 打死人, 都要给回执 —— 不给回执客户端会一直卡着等。
func TestAttackRejections(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = sureHit
	dead := putMonster(s, 901, 30, domain.Stats{}, 110, 110)
	dead.HP = 0
	s.step()
	sink.take()

	s.exec(Attack{ID: 1, Target: 12345}) // 不存在
	s.step()
	if rej, ok := firstOf[event.Rejected](sink.take()); !ok || rej.Reason != event.RejectNoTarget {
		t.Error("打不存在的目标应回 RejectNoTarget")
	}

	s.exec(Attack{ID: 1, Target: 901}) // 已经死了
	s.step()
	if rej, ok := firstOf[event.Rejected](sink.take()); !ok || rej.Reason != event.RejectTargetDead {
		t.Error("打死人应回 RejectTargetDead")
	}
}

// 打死之后: 先 EntityDied(战斗结果), 尸体停留若干帧, 再 EntityDespawned(视野事件)。
// 这两件事分开是有意的 —— 死亡要结算经验掉落, 消失只影响渲染。
func TestDeathThenCorpseRemoval(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = sureHit
	putMonster(s, 900, 7, domain.Stats{}, 110, 110)
	s.step()
	sink.take()

	s.exec(Attack{ID: 1, Target: 900})
	s.step()

	evs := sink.take()
	dmg, ok := firstOf[event.DamageDealt](evs)
	if !ok || !dmg.Flag.Has(event.DamageFatal) {
		t.Fatalf("这一击应致死, 实际 %+v", dmg)
	}
	died, ok := firstOf[event.EntityDied](evs)
	if !ok || died.ID != 900 || died.Killer != 1 {
		t.Fatalf("应发出死亡事件且带凶手, 实际 %+v", died)
	}
	if countOf[event.EntityDespawned](evs) != 0 {
		t.Fatal("尸体应停留一会儿, 不能死了立刻消失")
	}
	if _, still := s.entities[900]; !still {
		t.Fatal("尸体还没到时间就被移走了")
	}

	// 停留期满后才消失
	for i := 0; i < corpseTicks; i++ {
		s.step()
	}
	ds, ok := firstOf[event.EntityDespawned](sink.take())
	if !ok || ds.Reason != event.DespawnDeath {
		t.Fatalf("尸体到期应发出死亡消失, 实际 %+v", ds)
	}
	if _, still := s.entities[900]; still {
		t.Fatal("尸体到期后应从场上移除")
	}
}

// 目标死了要清掉攻击者的交战状态。不清的话下次打新目标会被当成同一场交战,
// 累加器不重置、首击标志也不再出现。
func TestKillResetsHitTracker(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = sureHit
	putMonster(s, 900, 7, domain.Stats{}, 110, 110)
	s.step()

	s.exec(Attack{ID: 1, Target: 900})
	s.step()

	if tr := s.hits[1]; tr == nil || tr.Target() != 0 {
		t.Fatalf("打死目标后交战状态应清空, 实际锁着 %v", tr.Target())
	}
}

// 玩家离开时要把交战状态一起清掉, 否则那个 map 会一直涨。
func TestLeaveClearsHitTracker(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = sureHit
	putMonster(s, 900, 100000, domain.Stats{}, 110, 110)
	s.step()

	s.exec(Attack{ID: 1, Target: 900})
	s.step()
	if s.hits[1] == nil {
		t.Fatal("打过之后应有交战状态")
	}

	s.exec(Leave{ID: 1, Reason: "登出"})
	if _, ok := s.hits[1]; ok {
		t.Fatal("人走了交战状态还留着 —— 这个 map 会一直涨")
	}
}

// 目标在同一帧里被别人打死了, 后到的攻击应当安静地作废, 不能打尸体。
func TestAttackOnEntityKilledSameFrame(t *testing.T) {
	s := newCombatScene(t)
	sinkA := join(t, s, 1, 100, "甲", 100, 100)
	join(t, s, 2, 200, "乙", 120, 120)
	s.entities[1].Stats = sureHit
	s.entities[2].Stats = sureHit
	putMonster(s, 900, 7, domain.Stats{}, 110, 110)
	s.step()
	sinkA.take()

	// 同一帧两个人都打, 甲那一下就能致死
	s.exec(Attack{ID: 1, Target: 900})
	s.exec(Attack{ID: 2, Target: 900})
	s.step()

	evs := sinkA.take()
	if n := countOf[event.DamageDealt](evs); n != 1 {
		t.Fatalf("目标已经死了, 本帧只该结算一次伤害, 实际 %d 次", n)
	}
	if n := countOf[event.EntityDied](evs); n != 1 {
		t.Fatalf("只该死一次, 实际发出 %d 条死亡事件", n)
	}
}

// 场景给定种子后, 同样的命令序列必须打出同样的结果 —— 出了问题能重放。
func TestSceneCombatIsReproducible(t *testing.T) {
	run := func() []int32 {
		s := New(Config{ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 777})
		sink := &fakeSink{}
		s.exec(Enter{ID: 1, Char: &domain.Character{ID: 100, Name: "甲",
			Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: sink})
		s.entities[1].Stats = domain.Stats{MinAtk: 4, MaxAtk: 7, Hit: 100, CritRate: 500}
		putMonster(s, 900, 1_000_000, domain.Stats{Def: 100}, 110, 110)
		s.step()
		sink.take()

		var out []int32
		for i := 0; i < 40; i++ {
			s.exec(Attack{ID: 1, Target: 900})
			s.step()
			for _, ev := range sink.take() {
				if d, ok := ev.(event.DamageDealt); ok {
					out = append(out, d.Amount)
				}
			}
		}
		return out
	}
	x, y := run(), run()
	if len(x) == 0 {
		t.Fatal("一次伤害都没打出来, 测试本身有问题")
	}
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("同种子第 %d 次结果不同: %d vs %d", i, x[i], y[i])
		}
	}
}

func TestPlayerAttackUsesExactRequestTimeHitDelay(t *testing.T) {
	received := time.Unix(1_700_000_000, 0)
	now := received.Add(100 * time.Millisecond) // 模拟命令等到下一个 100ms 主帧才被消费
	s := New(Config{
		ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 12345,
		PlayerAttackHitDelay: 350 * time.Millisecond,
		Now:                  func() time.Time { return now },
	})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.entities[1].Stats = sureHit
	putMonster(s, 900, 100, domain.Stats{}, 110, 110)
	s.step()
	sink.take()

	s.exec(Attack{ID: 1, Target: 900, ReceivedAt: received})
	if len(s.attacks) != 1 {
		t.Fatalf("攻击应被受理并等待命中，实际队列 %d", len(s.attacks))
	}
	want := received.Add(350 * time.Millisecond)
	if got := s.attacks[0].resolveAtWall; !got.Equal(want) {
		t.Fatalf("命中时刻应按收包时间 +350ms：实际 %v，期望 %v", got, want)
	}

	now = received.Add(349 * time.Millisecond)
	s.stepCombat()
	if n := countOf[event.DamageDealt](sink.take()); n != 0 {
		t.Fatalf("350ms 前不应回伤害，实际 %d 条", n)
	}
	now = want
	s.stepCombat()
	s.flush()
	if n := countOf[event.DamageDealt](sink.take()); n != 1 {
		t.Fatalf("350ms 命中点应回一条伤害，实际 %d 条", n)
	}
}

// 法系技能走的是另一条路: 不判命中, 按魔攻和魔防算。
func TestMagicBlowThroughScene(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	e := s.entities[1]
	e.Level = 60
	e.Stats = domain.Stats{MAtk: 100, Hit: 0} // 命中 0: 物理必 miss
	putMonster(s, 900, 100000, domain.Stats{Def: 1 << 20, MDef: 600}, 110, 110)
	s.step()
	sink.take()

	s.exec(Attack{ID: 1, Target: 900, Blow: combat.Blow{Magic: true, SkillID: 42, SkillPct: 100}})
	s.step()

	ev, ok := firstOf[event.DamageDealt](sink.take())
	if !ok {
		t.Fatal("法系技能应打出伤害")
	}
	if ev.Flag.Has(event.DamageMiss) {
		t.Fatal("法系技能不该 miss, 哪怕命中是 0")
	}
	if ev.SkillID != 42 {
		t.Errorf("技能 id 应透传, 实际 %d", ev.SkillID)
	}
	// 固定减伤：100 - 600×0.63 < 1，命中伤害按下限收敛为 1。
	if ev.Amount != 1 {
		t.Errorf("魔攻100、魔防600 应按固定减伤打 1, 实际 %d", ev.Amount)
	}
}

func TestAttackStatusDamageReductionReflectionAndNextHit(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.entities[1]
	// 状态消耗会重走正式属性管线，因此把命中与攻击写进角色真值，不能只改缓存 Stats。
	p.Player.Char.Base = domain.Base{STR: 14, VIT: 100, SPI: 20, DEX: 2 << 20}
	s.refreshStats(p)
	p.HP = p.MaxHP
	p.Status.Apply(domain.StatusDef{
		ID: domain.StatusKillingIntent, DurationSec: 30, NextBasicAttackPct: 100,
	}, s.tick, p.ID)
	m := putMonster(s, 900, 100, domain.Stats{}, 110, 110)
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{
		ID: 1037, DurationSec: 30,
		PhysicalDamageReductionPct: 50,
		PhysicalReflectPct:         50,
	}, s.tick, m.ID)
	beforeHP := p.HP
	s.step()
	sink.take()

	s.exec(Attack{ID: p.ID, Target: m.ID})
	s.step()
	events := sink.take()
	var dealt, reflected *event.DamageDealt
	for _, raw := range events {
		d, ok := raw.(event.DamageDealt)
		if !ok {
			continue
		}
		if d.Dst == m.ID {
			copy := d
			dealt = &copy
		}
		if d.Dst == p.ID {
			copy := d
			reflected = &copy
		}
	}
	if dealt == nil || dealt.Amount != 7 { // 7 × 杀意 200% × 火盾减伤 50%
		t.Fatalf("主伤害应为 7，实际 %+v", dealt)
	}
	if reflected == nil || reflected.Amount != 3 || p.HP != beforeHP-3 {
		t.Fatalf("应反射最终伤害的 50%%（整数为3），事件=%+v HP=%d→%d", reflected, beforeHP, p.HP)
	}
	if p.Status.Has(domain.StatusKillingIntent) {
		t.Fatal("杀意应在这次普通攻击起手时消耗")
	}
}

func TestFixedDamageShieldIsConsumedByRealAttack(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.entities[1]
	p.Stats = domain.Stats{MinAtk: 200, MaxAtk: 200, Hit: 1 << 20}
	m := putMonster(s, 900, 500, domain.Stats{}, 110, 110)
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{
		ID: domain.StatusDamageShield, DurationSec: 30, DamageShield: 100,
	}, s.tick, m.ID)
	s.step()
	sink.take()

	s.exec(Attack{ID: p.ID, Target: m.ID})
	s.step()
	ev, ok := firstOf[event.DamageDealt](sink.take())
	if !ok || ev.Amount != 100 || ev.DstHP != 400 {
		t.Fatalf("200 点攻击经过 100 点护盾应造成 100，实际 %+v", ev)
	}
	if m.Status.Has(domain.StatusDamageShield) {
		t.Fatal("固定伤害盾耗尽后应移除")
	}
}
