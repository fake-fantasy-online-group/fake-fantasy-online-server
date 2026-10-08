package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 组队在场景里的两件事：药师能奶队友、击杀经验按队分。
//
// 这两件事都只靠比较实体身上抄来的 PartyID —— 场景不认识名册包。

// ── 选目标 ──

// 治愈术 target_team=1。**没组队时只能给自己**：
// 以前 validTarget 对任何玩家都放行，等于药师是全服公共设施。
func TestHealCannotTargetAStranger(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	join(t, s, 2, 200, "路人", 110, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	s.step()
	sink.take()

	伤 := s.entities[2]
	伤.MaxHP, 伤.HP = 1000, 300

	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: 2})
	s.step()

	if _, ok := firstOf[event.HealDone](sink.take()); ok {
		t.Fatal("给陌生人加上血了 —— 队伍技能不该对任何人生效")
	}
	if 伤.HP != 300 {
		t.Fatalf("陌生人的血从 300 变成了 %d", 伤.HP)
	}
}

func TestHealWorksOnAPartyMember(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	join(t, s, 2, 200, "队友", 110, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	s.step()
	sink.take()

	// 会话层把队伍号抄进来 —— 场景自己不管名册
	s.exec(SetParty{ID: 1, Party: 7})
	s.exec(SetParty{ID: 2, Party: 7})

	伤 := s.entities[2]
	伤.MaxHP, 伤.HP = 1000, 300

	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: 2})
	s.step()

	ev, ok := firstOf[event.HealDone](sink.take())
	if !ok {
		t.Fatal("给队友加不上血 —— 药师的 80 个技能就全废了")
	}
	if ev.Amount != 200 { // 上限 1000 的 10% + 100
		t.Fatalf("回了 %d, 该是 200", ev.Amount)
	}
	if 伤.HP != 500 {
		t.Fatalf("队友的血是 %d, 该从 300 涨到 500", 伤.HP)
	}
}

// 0 号队伍不算队友。漏掉这个判断的话，全服散人会互相成为队友。
func TestPartyZeroIsNotATeam(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	join(t, s, 2, 200, "另一个散人", 110, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	s.step()
	sink.take()

	// 两个人的 PartyID 都是 0(默认值)
	if s.entities[1].Player.Party != 0 || s.entities[2].Player.Party != 0 {
		t.Fatal("测试前提不成立: 默认队伍号不是 0")
	}
	伤 := s.entities[2]
	伤.MaxHP, 伤.HP = 1000, 300

	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: 2})
	s.step()

	if _, ok := firstOf[event.HealDone](sink.take()); ok {
		t.Fatal("两个 0 号队伍的散人被判成了队友")
	}
}

func TestLeavingPartyStopsTheHeal(t *testing.T) {
	s, _ := skillScene(t)
	sink := join(t, s, 1, 100, "药师", 100, 100)
	join(t, s, 2, 200, "前队友", 110, 100)
	caster(s, domain.Learned{治愈术.ID: 1})
	s.step()
	s.exec(SetParty{ID: 1, Party: 7})
	s.exec(SetParty{ID: 2, Party: 7})
	s.flush() // 先冲刷: 组队那两条事件本来就该发给各自本人
	sink.take()

	// 退队: 会话层把队伍号清成 0
	s.exec(SetParty{ID: 2, Party: 0})
	s.flush()
	// 2 号的队伍变更只发给 2 号, 1 号(本 sink)不该收到
	if _, ok := firstOf[event.PartyChanged](sink.take()); ok {
		t.Fatal("2 号的队伍变更事件发到了 1 号那里")
	}

	伤 := s.entities[2]
	伤.MaxHP, 伤.HP = 1000, 300
	s.exec(UseSkill{ID: 1, Skill: 治愈术.ID, Target: 2})
	s.step()

	if _, ok := firstOf[event.HealDone](sink.take()); ok {
		t.Fatal("退队之后还能被奶")
	}
}

// ── 分经验 ──

// partyKillScene 建一个能打死怪的场景，两个满配玩家在一起。
func partyKillScene(t *testing.T) (*Scene, *pair) {
	t.Helper()
	s, _ := petScene(t) // 有海龟怪、有物品表、有经验曲线
	join(t, s, 2, 200, "队友", 140, 100)
	s.step()

	for _, id := range []domain.EntityID{1, 2} {
		p := s.entities[id]
		p.Player.Char.Level = 30
		p.Player.Char.Base = domain.Base{STR: 200, VIT: 100, INT: 20, SPI: 20, AGI: 20, DEX: 20}
		s.refreshStats(p)
		p.HP = p.MaxHP
	}
	return s, &pair{a: s.entities[1], b: s.entities[2]}
}

// pair 只是把两个玩家实体捆在一起, 省得每个测试都写两遍取实体。
type pair struct{ a, b *entity.Entity }

// killTurtle 打死场上那只海龟，返回它值多少经验。
func killTurtle(t *testing.T, s *Scene, by domain.EntityID) int64 {
	t.Helper()
	id := *theTurtle(t, s)
	m := s.monsters[id]
	m.HP = 1
	for i := 0; i < 10 && m.Alive(); i++ {
		s.attacks = append(s.attacks, pendingAttack{src: by, dst: id})
		s.stepCombat()
	}
	if m.Alive() {
		t.Fatal("没打死")
	}
	return 海龟怪.Exp
}

func TestSoloKillGivesFullExp(t *testing.T) {
	s, e := partyKillScene(t)
	before := e.a.Player.Char.Exp
	exp := killTurtle(t, s, 1)

	if got := e.a.Player.Char.Exp - before; got != exp {
		t.Fatalf("单人拿到 %d, 该是整份 %d", got, exp)
	}
	if e.b.Player.Char.Exp != 0 {
		t.Fatalf("没组队的旁人拿到了 %d 经验", e.b.Player.Char.Exp)
	}
}

func TestPartyKillSplitsExp(t *testing.T) {
	s, e := partyKillScene(t)
	s.exec(SetParty{ID: 1, Party: 7})
	s.exec(SetParty{ID: 2, Party: 7})

	exp := killTurtle(t, s, 1)
	each := domain.PartyExpShare(exp, 2)

	if got := e.a.Player.Char.Exp; got != each {
		t.Fatalf("击杀者拿到 %d, 该是 %d", got, each)
	}
	if got := e.b.Player.Char.Exp; got != each {
		t.Fatalf("队友拿到 %d, 该是 %d —— 没参与最后一击也该分", got, each)
	}
	// 组队总量比单人多(有加成), 但人均比单人少
	if each*2 <= exp {
		t.Fatalf("两人总共 %d, 该比单人 %d 多(10%% 加成)", each*2, exp)
	}
	if each >= exp {
		t.Fatalf("人均 %d 不该超过单人 %d", each, exp)
	}
}

// 同场景队友不受距离限制；只有换到另一个场景才不分经验。
func TestFarAwayMemberInSameSceneSharesExp(t *testing.T) {
	s, e := partyKillScene(t)
	s.exec(SetParty{ID: 1, Party: 7})
	s.exec(SetParty{ID: 2, Party: 7})
	e.b.Pos.X = 10_000

	exp := killTurtle(t, s, 1)
	each := domain.PartyExpShare(exp, 2)
	if got := e.a.Player.Char.Exp; got != each {
		t.Fatalf("击杀者拿到 %d, 该是 %d", got, each)
	}
	if got := e.b.Player.Char.Exp; got != each {
		t.Fatalf("同场景远处队友拿到 %d, 该是 %d", got, each)
	}
}

// 等级差太大分不到。没有这条，满级号带 1 级号几分钟就能拉到几十级。
func TestPowerLevelingIsBlocked(t *testing.T) {
	s, e := partyKillScene(t)
	s.exec(SetParty{ID: 1, Party: 7})
	s.exec(SetParty{ID: 2, Party: 7})
	// 击杀者不能设成 60(等级上限) —— 满级之后经验不再累积,
	// 那样测出来的"拿到 0"是封顶导致的, 跟组队没关系
	e.a.Player.Char.Level = 50
	e.b.Player.Char.Level = 1 // 差 49 级

	exp := killTurtle(t, s, 1)
	if e.b.Player.Char.Exp != 0 {
		t.Fatalf("1 级号被 60 级带飞, 拿到了 %d 经验", e.b.Player.Char.Exp)
	}
	// 只剩一个够格 → 走单人那条路, 不打 10% 加成的折
	if got := e.a.Player.Char.Exp; got != exp {
		t.Fatalf("只剩自己够格时该拿整份 %d, 实得 %d", exp, got)
	}
}

// **先筛人再算加成。** 反过来的话挂四个小号就能给自己加 40% 经验。
func TestBonusCountsOnlyEligibleMembers(t *testing.T) {
	s, e := partyKillScene(t)
	join(t, s, 3, 300, "小号", 140, 100)
	s.step()
	小号 := s.entities[3]
	小号.Player.Char.Level = 1
	小号.Player.Char.Base = domain.Base{VIT: 10, SPI: 10}
	s.refreshStats(小号)
	小号.HP = 小号.MaxHP

	for _, id := range []domain.EntityID{1, 2, 3} {
		s.exec(SetParty{ID: id, Party: 7})
	}
	e.a.Player.Char.Level = 30
	e.b.Player.Char.Level = 30

	exp := killTurtle(t, s, 1)
	// 够格的只有两个人, 加成该按 2 算而不是 3
	want := domain.PartyExpShare(exp, 2)
	if got := e.a.Player.Char.Exp; got != want {
		t.Fatalf("拿到 %d, 该按 2 人算 = %d。按 3 人算是 %d —— "+
			"那就成了挂小号给自己加经验", got, want, domain.PartyExpShare(exp, 3))
	}
	if 小号.Player.Char.Exp != 0 {
		t.Fatalf("超出等级差的小号拿到了 %d", 小号.Player.Char.Exp)
	}
}

func TestDeadMemberGetsNothing(t *testing.T) {
	s, e := partyKillScene(t)
	s.exec(SetParty{ID: 1, Party: 7})
	s.exec(SetParty{ID: 2, Party: 7})
	e.b.HP = 0 // 躺了

	exp := killTurtle(t, s, 1)
	if e.b.Player.Char.Exp != 0 {
		t.Fatalf("死人拿到了 %d 经验", e.b.Player.Char.Exp)
	}
	if got := e.a.Player.Char.Exp; got != exp {
		t.Fatalf("队友躺了, 击杀者该拿整份 %d, 实得 %d", exp, got)
	}
}
