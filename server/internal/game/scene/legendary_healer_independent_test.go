package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"math/rand"
	"testing"
	"time"
)

// Each independent row equips only its named power. Conditions are native casts
// and statuses, not synthetic legendary modifier records. Combinations are kept
// in a distinct test and excluded from the independent coverage denominator.
func healerSoloFixture(t *testing.T, id string, learned domain.Learned, on bool) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	powers := []string{}
	if on {
		powers = append(powers, id)
	}
	s, p, m := nativeRepairFixture(t, domain.Healer, learned, powers...)
	s.rng = rand.New(rand.NewSource(1))
	return s, p, m
}
func healerSoloAlly(t *testing.T, s *Scene, p *entity.Entity, id domain.EntityID, x, y float64, hp, maxHP int32) *entity.Entity {
	t.Helper()
	join(t, s, id, int64(100+id), "healer solo ally", x, y)
	a := s.players[id]
	p.Player.Party = 1
	a.Player.Party = 1
	a.HP, a.MaxHP = hp, maxHP
	a.MP, a.MaxMP = 0, 1000
	return a
}
func healerSoloMonster(s *Scene, id domain.EntityID, x, y float64) *entity.Entity {
	m := putMonster(s, id, 100000, domain.Stats{}, x, y)
	m.Status = domain.NewStatusSet()
	s.monsters[m.ID] = m
	return m
}
func healerSoloUndead(s *Scene, m *entity.Entity) {
	s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Kind: domain.MonsterNormal, Family: domain.MonsterFamilyUndead}}
}
func healerSoloStatus(t *testing.T, e *entity.Entity, skill domain.SkillID) domain.Active {
	t.Helper()
	var out domain.Active
	e.Status.EachEffective(func(a domain.Active) {
		if a.Def.SkillSnapshotID == skill || a.Def.SourceSkillID == int32(skill) {
			out = a
		}
	})
	if out.Def.ID == 0 {
		t.Fatalf("missing native status from skill %d on %d", skill, e.ID)
	}
	return out
}
func healerSoloCount(s *Scene, id domain.EntityID) (int32, int) {
	var damage int32
	count := 0
	for _, x := range s.outbox {
		if e, ok := x.ev.(event.DamageDealt); ok && e.Dst == id && e.Amount > 0 {
			damage += e.Amount
			count++
		}
	}
	return damage, count
}
func healerSoloNear(t *testing.T, got, base, pct int32) {
	t.Helper()
	want := base * pct / 100
	if got < want-2 || got > want+2 {
		t.Fatalf("actual amount %d, baseline %d × %d%% = %d", got, base, pct, want)
	}
}
func healerSoloIncoming(s *Scene, p, m *entity.Entity, magic, basic bool) int32 {
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000, Hit: 100000}
	b := combat.Blow{GuaranteedHit: true, Magic: magic}
	if !basic {
		b.SkillID = 900999
	}
	before := p.HP
	ev, raw, full := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
	s.afterStrikeStatuses(m, p, ev, raw, full)
	return before - p.HP
}

func TestHealerIndependentA01(t *testing.T) {
	var base int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A01", domain.Learned{10319: 1}, on)
		others := []*entity.Entity{}
		for i, x := range []float64{140, 170, 220, 260, 311} {
			others = append(others, healerSoloMonster(s, domain.EntityID(901+i), x, 100))
		}
		nativeRepairCast(t, s, p, 10319, m.ID)
		got := 100000 - m.HP
		if !on {
			base = got
		} else if got != base {
			t.Fatalf("primary damage %d want %d", got, base)
		}
		for i, v := range others {
			loss := 100000 - v.HP
			if on && i < 4 {
				healerSoloNear(t, loss, base, 60)
				if _, n := healerSoloCount(s, v.ID); n != 1 {
					t.Fatal("secondary repeated", n)
				}
			} else if loss != 0 {
				t.Fatal("all-in-radius exclusion", i, loss)
			}
			if v.Status.HasEffective(1008) {
				t.Fatal("secondary status generated")
			}
		}
	}
}
func TestHealerIndependentA02(t *testing.T) {
	var base int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A02", domain.Learned{10319: 1}, on)
		m.Stats.MagicResist = 20
		nativeRepairCast(t, s, p, 10319, m.ID)
		loss := 100000 - m.HP
		if !on {
			base = loss
		} else {
			healerSoloNear(t, loss, base, 185)
			_, n := healerSoloCount(s, m.ID)
			if n != 2 {
				t.Fatal("echo recursion/extra hit", n)
			}
		}
	}
	s, p, m := healerSoloFixture(t, "YS-A02", domain.Learned{10319: 1}, true)
	m.HP = 1
	side := healerSoloMonster(s, 901, 140, 100)
	nativeRepairCast(t, s, p, 10319, m.ID)
	if m.Alive() || side.HP != 100000 {
		t.Fatal("dead primary echo transferred")
	}
}
func TestHealerIndependentA03(t *testing.T) {
	var base int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A03", domain.Learned{10319: 1}, on)
		for i := 0; i < 4; i++ {
			d := domain.StatusDef{ID: domain.StatusID(190500 + i), DurationSec: 30, IntervalSec: 1, TypeKnown: true, Type: domain.StatusGood, HPChanges: []domain.ResourceChange{{Value: 10}}}
			if i == 2 {
				d.Permanent = true
			}
			if i == 3 {
				d.Type = domain.StatusBad
			}
			m.Status.Apply(d, s.tick, m.ID)
		}
		nativeRepairCast(t, s, p, 10319, m.ID)
		loss := 100000 - m.HP
		if !on {
			base = loss
		} else {
			healerSoloNear(t, loss, base, 160)
		}
		n := 0
		for _, id := range []domain.StatusID{190500, 190501} {
			if m.Status.HasEffective(id) {
				n++
			}
		}
		want := 2
		if on {
			want = 1
		}
		if n != want || !m.Status.HasEffective(190502) || !m.Status.HasEffective(190503) {
			t.Fatal("HOT removal count/permanent/harmful guard", n)
		}
	}
}
func healerSoloOwnedDebuff(t *testing.T, id string, attack domain.SkillID, debuffs []domain.SkillID, pct int32) {
	t.Helper()
	for _, debuff := range debuffs {
		var base int32
		for _, mode := range []string{"none", "own", "foreign"} {
			s, p, m := healerSoloFixture(t, id, domain.Learned{attack: 1, debuff: 1}, true)
			var before domain.Active
			if mode != "none" {
				nativeRepairCast(t, s, p, debuff, m.ID)
				before = healerSoloStatus(t, m, debuff)
				if mode == "foreign" {
					m.Status.Remove(before.Def.ID)
					m.Status.Apply(before.Def, s.tick, 777)
					before = healerSoloStatus(t, m, debuff)
				}
			}
			hp := m.HP
			s.outbox = nil
			nativeRepairCast(t, s, p, attack, m.ID)
			got := hp - m.HP
			if mode == "none" {
				base = got
			} else if mode == "own" {
				healerSoloNear(t, got, base, pct)
			} else if got != base {
				t.Fatal("foreign native debuff amplified", got, base)
			}
			if mode != "none" {
				after := healerSoloStatus(t, m, debuff)
				if after.Source != before.Source || after.ExpireAt != before.ExpireAt {
					t.Fatal("debuff consumed or refreshed")
				}
			}
		}
	}
}
func TestHealerIndependentA04(t *testing.T) {
	healerSoloOwnedDebuff(t, "YS-A04", 10319, []domain.SkillID{10324, 10325}, 190)
}
func TestHealerIndependentA05(t *testing.T) {
	var base int32
	s, p, m := healerSoloFixture(t, "YS-A05", domain.Learned{10317: 1}, false)
	nativeRepairCast(t, s, p, 10317, m.ID)
	base = 100000 - m.HP
	for _, center := range []string{"self", "ally", "enemy"} {
		s, p, m = healerSoloFixture(t, "YS-A05", domain.Learned{10317: 1}, true)
		target := p
		if center == "ally" {
			target = healerSoloAlly(t, s, p, 2, 650, 100, 100, 1000)
			s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 650, Y: 100})
		}
		if center == "enemy" {
			target = m
		}
		out := healerSoloMonster(s, 901, target.Pos.X+201, 100)
		hp := target.HP
		nativeRepairCast(t, s, p, 10317, target.ID)
		if center != "enemy" {
			healerSoloNear(t, 100000-m.HP, base, 120)
			if target.HP != hp || out.HP != 100000 {
				t.Fatal("friendly center healed or radius leaked")
			}
		} else if 100000-m.HP != base {
			t.Fatal("enemy-center bonus changed")
		}
		if !m.Status.HasEffective(1008) {
			t.Fatal("original blind absent")
		}
	}
	for _, kind := range []string{"far", "stranger"} {
		s, p, _ = healerSoloFixture(t, "YS-A05", domain.Learned{10317: 1}, true)
		a := healerSoloAlly(t, s, p, 2, 701, 100, 100, 1000)
		if kind == "stranger" {
			a.Pos.X = 200
			a.Player.Party = 2
		}
		mp := p.MP
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10317, Target: a.ID})
		if len(s.casts) != 0 || p.MP != mp {
			t.Fatal("illegal friendly center admitted", kind)
		}
	}
}
func TestHealerIndependentA06(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A06", domain.Learned{10317: 1}, false)
	nativeRepairCast(t, s, p, 10317, m.ID)
	base := 100000 - m.HP
	s, p, m = healerSoloFixture(t, "YS-A06", domain.Learned{10317: 1}, true)
	inside := healerSoloMonster(s, 901, 230, 100)
	outer := healerSoloMonster(s, 902, 330, 100)
	out := healerSoloMonster(s, 903, 331, 100)
	nativeRepairCast(t, s, p, 10317, m.ID)
	for _, v := range []*entity.Entity{m, inside, outer} {
		pct := int32(160)
		if v == outer {
			pct = 120
		}
		healerSoloNear(t, 100000-v.HP, base, pct)
		_, n := healerSoloCount(s, v.ID)
		if n != 1 || !v.Status.HasEffective(1008) {
			t.Fatal("per-target hit/blind missing or duplicated", n)
		}
	}
	if out.HP != 100000 {
		t.Fatal("circle boundary leaked")
	}
}
func TestHealerIndependentA08(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A08", domain.Learned{10317: 1}, false)
	nativeRepairCast(t, s, p, 10317, m.ID)
	base := 100000 - m.HP
	s, p, m = healerSoloFixture(t, "YS-A08", domain.Learned{10317: 1}, true)
	s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 550, Y: 100})
	inside := healerSoloMonster(s, 901, 690, 179)
	wide := healerSoloMonster(s, 902, 690, 181)
	long := healerSoloMonster(s, 903, 701, 100)
	nativeRepairCast(t, s, p, 10317, m.ID)
	for _, v := range []*entity.Entity{m, inside} {
		healerSoloNear(t, 100000-v.HP, base, 180)
		_, n := healerSoloCount(s, v.ID)
		if n != 1 || !v.Status.HasEffective(1008) {
			t.Fatal("rectangle/primary/blind", n)
		}
	}
	if wide.HP != 100000 || long.HP != 100000 {
		t.Fatal("rectangle width or length leaked")
	}
}
func TestHealerIndependentA09(t *testing.T) {
	var base int32
	var cd domain.Tick
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A09", domain.Learned{10312: 1}, on)
		healerSoloUndead(s, m)
		nativeRepairCast(t, s, p, 10312, m.ID)
		loss := 100000 - m.HP
		r := p.CapturePlayerRuntime(s.tick)
		if !on {
			base = loss
			cd = r.SkillReadyIn[10312]
		} else {
			healerSoloNear(t, loss, base, 140)
			if r.SkillReadyIn[10312] != cd {
				t.Fatal("cooldown changed")
			}
		}
	}
	s, p, m := healerSoloFixture(t, "YS-A09", domain.Learned{10312: 1}, true)
	side := healerSoloMonster(s, 901, 350, 100)
	out := healerSoloMonster(s, 902, 351, 100)
	for i := 0; i < 5; i++ {
		d := domain.StatusDef{ID: domain.StatusID(190600 + i), DurationSec: 30, TypeKnown: true, Type: domain.StatusGood}
		if i == 3 {
			d.Permanent = true
		}
		if i == 4 {
			d.Type = domain.StatusBad
		}
		m.Status.Apply(d, s.tick, m.ID)
	}
	nativeRepairCast(t, s, p, 10312, m.ID)
	healerSoloNear(t, 100000-m.HP, base, 140)
	healerSoloNear(t, 100000-side.HP, base, 140)
	if out.HP != 100000 {
		t.Fatal("220 radius leaked")
	}
	n := 0
	for i := 0; i < 3; i++ {
		if m.Status.HasEffective(domain.StatusID(190600 + i)) {
			n++
		}
	}
	if n != 1 || !m.Status.HasEffective(190603) || !m.Status.HasEffective(190604) {
		t.Fatal("two-benefit dispel bounds", n)
	}
}
func healerSoloDebuffMagnitude(t *testing.T, id string, skill domain.SkillID, attr int32, pct int32) {
	t.Helper()
	var base int32
	var duration domain.Tick
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, id, domain.Learned{skill: 1}, on)
		side := healerSoloMonster(s, 901, 220, 100)
		outside := healerSoloMonster(s, 902, 231, 100)
		pos := m.Pos
		nativeRepairCast(t, s, p, skill, m.ID)
		a := healerSoloStatus(t, m, skill)
		value := int32(0)
		for _, f := range a.Def.Affixes {
			if f.Attr == attr {
				value = f.Value
			}
		}
		if !on {
			base = value
			duration = a.ExpireAt - s.tick
		} else {
			healerSoloNear(t, value, base, pct)
			if a.ExpireAt-s.tick != duration {
				t.Fatal("native duration changed")
			}
			if id == "YS-A10" && !legendaryCasterHasSkill(side, skill) {
				t.Fatal("expanded weakness missed neighbor")
			}
		}
		if m.HP != 100000 || side.HP != 100000 || m.Pos != pos || legendaryCasterHasSkill(outside, skill) {
			t.Fatal("status-only power invented damage/movement")
		}
	}
}
func TestHealerIndependentA10(t *testing.T) {
	healerSoloDebuffMagnitude(t, "YS-A10", 10324, domain.AttrAtk, 160)
}
func TestHealerIndependentA11(t *testing.T) {
	var base int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A11", domain.Learned{10312: 1}, on)
		healerSoloUndead(s, m)
		side := healerSoloMonster(s, 901, 390, 100)
		out := healerSoloMonster(s, 902, 401, 100)
		nativeRepairCast(t, s, p, 10312, m.ID)
		loss := 100000 - m.HP
		if !on {
			base = loss
			if side.HP != 100000 {
				t.Fatal("control radius")
			}
		} else {
			healerSoloNear(t, loss, base, 145)
			healerSoloNear(t, 100000-side.HP, base, 145)
			_, n := healerSoloCount(s, m.ID)
			if n != 1 || out.HP != 100000 {
				t.Fatal("damage repeated or 270 radius leaked", n)
			}
		}
	}
}
func TestHealerIndependentA12(t *testing.T) {
	var base int32
	for _, mode := range []string{"off", "interruptible", "immune"} {
		s, p, m := healerSoloFixture(t, "YS-A12", domain.Learned{10312: 1}, mode != "off")
		md := domain.MonsterDef{ID: m.Monster.TypeID, Kind: domain.MonsterNormal, Family: domain.MonsterFamilyUndead}
		if mode == "immune" {
			md.ImmuneSkills = map[domain.SkillID]struct{}{10312: {}}
		}
		s.defs = spawn.MapDefs{m.Monster.TypeID: md}
		m.Monster.SkillReady = map[domain.SkillID]domain.Tick{}
		m.Monster.SkillCasts = map[domain.SkillID]int32{}
		rule := domain.MonsterSkillRule{Skill: domain.SkillDef{ID: 900999, Level: 1, Kind: domain.SkillSingle, PrepareMS: 2000, Dist: 500, TargetEnemy: true, HurtType: domain.HurtHoly, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}}
		s.startMonsterSkill(m, rule, []domain.EntityID{p.ID})
		serial, deadline := m.Monster.CastSerial, m.Monster.CastingUntil
		nativeRepairCast(t, s, p, 10312, m.ID)
		loss := 100000 - m.HP
		if mode == "off" {
			base = loss
		} else {
			healerSoloNear(t, loss, base, 150)
		}
		if mode == "interruptible" {
			if m.Monster.CastingSkill != 0 || m.Monster.CastSerial == serial {
				t.Fatal("AI windup survived")
			}
			s.tick = deadline
			s.timer.advance(s.tick)
			if m.Monster.SkillCasts[900999] != 0 {
				t.Fatal("cancelled AI timer fired")
			}
		} else if m.Monster.CastingSkill == 0 || m.Monster.CastSerial != serial {
			t.Fatal("baseline or immune AI cast cancelled")
		}
	}
}
func TestHealerIndependentA13(t *testing.T) {
	healerSoloOwnedDebuff(t, "YS-A13", 10326, []domain.SkillID{10325}, 180)
}
func TestHealerIndependentA14(t *testing.T) {
	var baseCD domain.Tick
	var paid int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A14", domain.Learned{10326: 1}, on)
		nativeRepairCast(t, s, p, 10326, m.ID)
		r := p.CapturePlayerRuntime(s.tick)
		cd := r.SkillReadyIn[10326]
		if !on {
			baseCD = cd
			paid = 10000 - p.MP
		} else {
			if cd != baseCD*55/100 || 10000-p.MP != paid || !m.Alive() {
				t.Fatalf("unconditional cooldown/payment: %d vs %d MP%d", cd, baseCD, 10000-p.MP)
			}
			s.tick += cd - 1
			if p.Player.SkillReadyAt(10326, s.tick) {
				t.Fatal("cooldown ended early")
			}
			s.tick++
			if !p.Player.SkillReadyAt(10326, s.tick) {
				t.Fatal("cooldown did not end")
			}
		}
	}
}
func TestHealerIndependentA15(t *testing.T) {
	for _, kind := range []string{"ordinary", "cap", "actual_loss", "dead_primary"} {
		var baseline []int32
		for _, on := range []bool{false, true} {
			s, p, m := healerSoloFixture(t, "YS-A15", domain.Learned{10326: 1}, on)
			targets := []*entity.Entity{m}
			n := 2
			if kind == "cap" {
				n = 5
			}
			for i := 0; i < n; i++ {
				targets = append(targets, healerSoloMonster(s, domain.EntityID(901+i), float64(140+i*5), 100))
			}
			if kind == "actual_loss" {
				targets[1].HP = 1
				targets[2].HP = 2
			}
			if kind == "dead_primary" {
				m.HP = 1
			}
			before := make([]int32, len(targets))
			for i, v := range targets {
				before[i] = v.HP
			}
			nativeRepairCast(t, s, p, 10326, m.ID)
			loss := make([]int32, len(targets))
			for i, v := range targets {
				loss[i] = before[i] - v.HP
			}
			if !on {
				baseline = loss
				continue
			}
			pool := int32(0)
			for i := 1; i < len(targets); i++ {
				pool += baseline[i]
				if loss[i] != baseline[i] {
					t.Fatal("secondary direct damage altered", kind, i)
				}
			}
			want := baseline[0] + min(baseline[0], pool*35/100)
			if kind == "dead_primary" {
				want = baseline[0]
			}
			if loss[0] != want {
				t.Fatal("pool actual-loss/cap/dead-primary", kind, loss[0], want)
			}
		}
	}
}
func TestHealerIndependentA16(t *testing.T) {
	var base int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A16", domain.Learned{10326: 1}, on)
		nativeRepairCast(t, s, p, 10326, m.ID)
		if !on {
			base = 100000 - m.HP
		} else {
			healerSoloNear(t, 100000-m.HP, base, 135)
		}
	}
	s, p, m := healerSoloFixture(t, "YS-A16", domain.Learned{10326: 1}, true)
	d, _ := s.skills.Get(10326, 1)
	s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: p.Pos.X + float64(d.Dist+199), Y: 100})
	center := p.Pos
	if d.Area.Center == domain.SkillAreaCenterTarget {
		center = m.Pos
	}
	r := float64(max(d.Radius, d.Area.Radius) + 100)
	inside := healerSoloMonster(s, 901, center.X, center.Y+r-1)
	out := healerSoloMonster(s, 902, center.X, center.Y+r+1)
	nativeRepairCast(t, s, p, 10326, m.ID)
	healerSoloNear(t, 100000-inside.HP, base, 135)
	if out.HP != 100000 {
		t.Fatal("radius/center changed")
	}
	s, p, m = healerSoloFixture(t, "YS-A16", domain.Learned{10326: 1}, false)
	s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: p.Pos.X + float64(d.Dist+199), Y: 100})
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10326, Target: m.ID})
	if len(s.casts) != 0 {
		t.Fatal("baseline admitted extended range")
	}
}
func TestHealerIndependentA18(t *testing.T) {
	healerSoloDebuffMagnitude(t, "YS-A18", 10325, domain.AttrAtkSpeed, 150)
}

func TestHealerIndependentA07(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A07", domain.Learned{10317: 1}, false)
	nativeRepairCast(t, s, p, 10317, m.ID)
	base := 100000 - m.HP
	s, p, m = healerSoloFixture(t, "YS-A07", domain.Learned{10317: 1}, true)
	edge := healerSoloMonster(s, 901, 309, 100)
	outside := healerSoloMonster(s, 902, 311, 100)
	nativeRepairCast(t, s, p, 10317, m.ID)
	healerSoloNear(t, 100000-m.HP, base, 110)
	if !m.Status.HasEffective(1008) {
		t.Fatal("direct blind removed")
	}
	start := m.HP
	s.stepLegendaryPowers()
	if m.HP != start {
		t.Fatal("field ticks before one second")
	}
	for i := 1; i <= 4; i++ {
		s.tick += domain.Ticks(1000)
		s.stepLegendaryPowers()
		healerSoloNear(t, start-m.HP, base, int32(i*30))
		if edge.Status.Count() != 0 {
			t.Fatal("field applied blind")
		}
	}
	hp := m.HP
	s.tick += domain.Ticks(1000)
	s.stepLegendaryPowers()
	if m.HP != hp || outside.HP != 100000 {
		t.Fatal("field duration/radius")
	}
	healerSoloNear(t, 100000-edge.HP, base, 120)
	// Create three accepted native casts without advancing the zone clock. The
	// normal skill cooldown is cleared only to exercise the two-slot field pool.
	s, p, m = healerSoloFixture(t, "YS-A07", domain.Learned{10317: 1}, true)
	for i := 0; i < 3; i++ {
		p.Player.ReduceRemainingSkillCooldown(10317, s.tick, domain.Ticks(5000))
		nativeRepairCast(t, s, p, 10317, m.ID)
	}
	hp = m.HP
	s.tick += domain.Ticks(1000)
	s.stepLegendaryPowers()
	healerSoloNear(t, hp-m.HP, base, 30)
	again := m.HP
	s.stepLegendaryPowers()
	if m.HP != again {
		t.Fatal("overlap/repeated scheduler tick stacked")
	}
	zones := 0
	for k, v := range s.legendaryState().states {
		if k.Phase == "light-field-0" || k.Phase == "light-field-1" {
			if v.Stacks > 0 {
				zones++
			}
		}
	}
	if zones != 2 {
		t.Fatal("field pool did not retain exactly two", zones)
	}
}
func TestHealerIndependentA17(t *testing.T) {
	for _, immune := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A17", domain.Learned{10324: 1, 10312: 1}, true)
		healerSoloUndead(s, m)
		side := healerSoloMonster(s, 901, 379, 100)
		out := healerSoloMonster(s, 902, 381, 100)
		if immune {
			m.Status.Apply(domain.StatusDef{ID: 199991, DurationSec: 30, BlockHarmful: true}, s.tick, m.ID)
		}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10324, Target: m.ID})
		if len(s.casts) != 1 || !s.casts[0].releaseAtWall.IsZero() {
			t.Fatal("weakness still has real windup")
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		if 100000-m.HP != 180 || 100000-side.HP != 180 || out.HP != 100000 {
			t.Fatal("180% weakness/radius")
		}
		if legendaryCasterHasSkill(m, 10324) == immune {
			t.Fatal("native immunity/status contract")
		}
	}
	var base int32
	for _, source := range []domain.EntityID{0, 1, 777} {
		s, p, m := healerSoloFixture(t, "YS-A17", domain.Learned{10312: 1, 10324: 1}, true)
		healerSoloUndead(s, m)
		var old domain.Active
		if source != 0 {
			nativeRepairCast(t, s, p, 10324, m.ID)
			old = healerSoloStatus(t, m, 10324)
			m.Status.Remove(old.Def.ID)
			m.Status.Apply(old.Def, s.tick, source)
			old = healerSoloStatus(t, m, 10324)
		}
		hp := m.HP
		nativeRepairCast(t, s, p, 10312, m.ID)
		if source == 0 {
			base = hp - m.HP
		} else {
			healerSoloNear(t, hp-m.HP, base, 160)
			a := healerSoloStatus(t, m, 10324)
			if a.Source != old.Source || a.ExpireAt != old.ExpireAt {
				t.Fatal("weakness consumed/refreshed")
			}
		}
	}
}
func TestHealerIndependentA19(t *testing.T) {
	for _, unequipped := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A19", domain.Learned{10325: 1}, unequipped)
		side := healerSoloMonster(s, 901, 330, 100)
		if unequipped {
			p.Player.Worn = &domain.EquipSet{}
			s.refreshStats(p)
		}
		pos := side.Pos
		nativeRepairCast(t, s, p, 10325, m.ID)
		if m.HP != 100000 || side.HP != 100000 || side.Pos != pos || legendaryCasterHasSkill(side, 10325) {
			t.Fatalf("absent/removed power granted shackle damage, splash or pull: unequipped=%v", unequipped)
		}
	}

	for _, kind := range []string{"normal", "boss", "immune"} {
		s, p, m := healerSoloFixture(t, "YS-A19", domain.Learned{10325: 1}, true)
		side := healerSoloMonster(s, 901, 330, 100)
		out := healerSoloMonster(s, 902, 381, 100)
		if kind == "boss" {
			side.Monster.Kind = domain.MonsterBoss
		}
		if kind == "immune" {
			side.Status.Apply(domain.StatusDef{ID: 199992, DurationSec: 30, BlockHarmful: true, SuppressHarmful: true}, s.tick, side.ID)
		}
		pos := side.Pos
		nativeRepairCast(t, s, p, 10325, m.ID)
		if 100000-m.HP != 200 || 100000-side.HP != 200 || out.HP != 100000 {
			t.Fatal("shackle damage/radius")
		}
		if kind == "normal" {
			if sqDist(m.Pos, side.Pos) != 80*80 || !legendaryCasterHasSkill(side, 10325) {
				t.Fatal("normal pull/native slow", side.Pos)
			}
		} else if side.Pos != pos {
			t.Fatal("boss/immune displaced", kind)
		}
		if kind == "immune" && legendaryCasterHasSkill(side, 10325) {
			t.Fatal("immune slow applied")
		}
	}
}
func healerSoloStatusDamage(t *testing.T, id string, buff domain.SkillID, pct int32) {
	t.Helper()
	for _, skill := range []domain.SkillID{10319, 10317, 10312} {
		var base int32
		for _, mode := range []string{"off", "missing", "active"} {
			s, p, m := healerSoloFixture(t, id, domain.Learned{skill: 1, buff: 1}, mode != "off")
			healerSoloUndead(s, m)
			if mode != "missing" {
				nativeRepairCast(t, s, p, buff, p.ID)
			}
			p.Stats = domain.Stats{MAtk: 100, MinAtk: 100, MaxAtk: 100, Hit: 100000}
			var old domain.Active
			if mode != "missing" {
				old = healerSoloStatus(t, p, buff)
			}
			hp := m.HP
			nativeRepairCast(t, s, p, skill, m.ID)
			loss := hp - m.HP
			if mode == "off" {
				base = loss
			} else if mode == "active" {
				healerSoloNear(t, loss, base, pct)
			} else if loss != base {
				t.Fatal("bonus without native buff", skill, loss, base)
			}
			if mode != "missing" {
				a := healerSoloStatus(t, p, buff)
				if a.ExpireAt != old.ExpireAt {
					t.Fatal("buff consumed/refreshed")
				}
			}
		}
	}
}
func TestHealerIndependentA20(t *testing.T) {
	healerSoloStatusDamage(t, "YS-A20", 10330, 140)
	s, p, m := healerSoloFixture(t, "YS-A20", domain.Learned{10330: 1}, true)
	nativeRepairCast(t, s, p, 10330, p.ID)
	a := healerSoloStatus(t, p, 10330)
	before := m.HP
	loss := healerSoloIncoming(s, p, m, false, true)
	if loss != 1000 || before-m.HP != 1000*a.Def.PhysicalReflectPct/100 {
		t.Fatal("native thorns reflection altered or extra needle", loss, before-m.HP)
	}
}
func TestHealerIndependentA21(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A21", domain.Learned{10320: 1}, on)
		edge := healerSoloMonster(s, 901, 320, 100)
		outside := healerSoloMonster(s, 902, 321, 100)
		nativeRepairCast(t, s, p, 10320, p.ID)
		shield := universalShieldValue(p)
		if shield <= 0 {
			t.Fatal("no native shield")
		}
		want := int32(0)
		if on {
			want = shield * 80 / 100
		}
		if 100000-m.HP != want || 100000-edge.HP != want || outside.HP != 100000 || universalShieldValue(p) != shield {
			t.Fatal("shield burst radius/amount/consumption")
		}
		if on {
			_, n := healerSoloCount(s, m.ID)
			if n != 1 {
				t.Fatal("burst repeated", n)
			}
		}
	}
}
func healerSoloWave(t *testing.T, id string, skill domain.SkillID, radius float64, pct int32) {
	t.Helper()
	for _, on := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			s, p, m := healerSoloFixture(t, id, domain.Learned{skill: 1}, on)
			a := p
			if skill != 10304 {
				a = healerSoloAlly(t, s, p, 2, 400, 100, 100, 1000)
				s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 400, Y: 100})
			}
			if full {
				a.HP = a.MaxHP
			} else {
				a.HP = 1
			}
			edge := healerSoloMonster(s, 901, a.Pos.X+radius, 100)
			out := healerSoloMonster(s, 902, a.Pos.X+radius+1, 100)
			hp := a.HP
			nativeRepairCast(t, s, p, skill, a.ID)
			want := int32(0)
			if on {
				want = pct
			}
			for _, v := range []*entity.Entity{m, edge} {
				if 100000-v.HP != want {
					t.Fatalf("%s full=%v damage %d want %d", id, full, 100000-v.HP, want)
				}
				if on {
					_, n := healerSoloCount(s, v.ID)
					if n != 1 {
						t.Fatal("wave repeated", n)
					}
				}
			}
			if out.HP != 100000 {
				t.Fatal("wave radius leaked")
			}
			if skill == 10304 && !full && a.HP <= hp {
				t.Fatal("native heal removed")
			}
		}
	}
}
func TestHealerIndependentA22(t *testing.T) { healerSoloWave(t, "YS-A22", 10306, 220, 220) }
func TestHealerIndependentA24(t *testing.T) { healerSoloWave(t, "YS-A24", 10304, 250, 250) }
func TestHealerIndependentA23(t *testing.T) {
	var raw int32
	for _, on := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			s, p, m := healerSoloFixture(t, "YS-A23", domain.Learned{10301: 1}, on)
			other := healerSoloMonster(s, 901, 150, 100)
			out := healerSoloMonster(s, 902, 351, 100)
			if !full {
				p.HP = 1
			}
			before := p.HP
			nativeRepairCast(t, s, p, 10301, p.ID)
			heal := p.HP - before
			if !on && !full {
				raw = heal
			}
			if !full && heal != raw {
				t.Fatal("main heal changed", heal, raw)
			}
			want := int32(0)
			if on {
				want = raw
			}
			if 100000-m.HP != want || 100000-other.HP != want || out.HP != 100000 {
				t.Fatal("all in-range granted bolts/full HP trigger", on, full, 100000-m.HP, want)
			}
			if p.Player.Char.Skills[10319] != 0 {
				t.Fatal("granted bolt learned an active skill")
			}
		}
	}
}
func TestHealerIndependentA25(t *testing.T) {
	var base domain.Active
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-A25", domain.Learned{10307: 1}, on)
		a := healerSoloAlly(t, s, p, 2, 340, 100, 1, 1000)
		out := healerSoloAlly(t, s, p, 3, 351, 100, 1, 1000)
		nativeRepairCast(t, s, p, 10307, p.ID)
		st := healerSoloStatus(t, p, 10307)
		if !on {
			base = st
			if legendaryCasterHasSkill(a, 10307) {
				t.Fatal("baseline range")
			}
		} else {
			for i, f := range st.Def.Affixes {
				healerSoloNear(t, f.Value, base.Def.Affixes[i].Value, 160)
			}
			if st.ExpireAt != base.ExpireAt || !legendaryCasterHasSkill(a, 10307) || legendaryCasterHasSkill(out, 10307) {
				t.Fatal("group blessing radius/duration")
			}
		}
	}
}
func TestHealerIndependentA26(t *testing.T) {
	var baseDamage int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A26", domain.Learned{10321: 1, 10319: 1}, on)
		a := healerSoloAlly(t, s, p, 2, 330, 100, 50, 1000)
		b := healerSoloAlly(t, s, p, 3, 340, 100, 200, 1000)
		excluded := healerSoloAlly(t, s, p, 4, 345, 100, 900, 1000)
		out := healerSoloAlly(t, s, p, 5, 351, 100, 1, 1000)
		nativeRepairCast(t, s, p, 10321, p.ID)
		st := healerSoloStatus(t, p, 10321)
		loss := healerSoloIncoming(s, p, m, true, false)
		want := int32(900)
		if on {
			want = 830
		}
		if loss != want || universalShieldValue(p) != 0 || st.Def.DurationSec != 300 {
			t.Fatal("defense strength/duration/shield", loss, st.Def)
		}
		p.Stats = domain.Stats{MAtk: 100, Hit: 100000}
		hp := m.HP
		nativeRepairCast(t, s, p, 10319, m.ID)
		if !on {
			baseDamage = hp - m.HP
		} else {
			healerSoloNear(t, hp-m.HP, baseDamage, 140)
			if !legendaryCasterHasSkill(a, 10321) || !legendaryCasterHasSkill(b, 10321) || !legendaryCasterHasSkill(excluded, 10321) || legendaryCasterHasSkill(out, 10321) {
				t.Fatal("all in-range additional recipients")
			}
		}
	}
}
func TestHealerIndependentA27(t *testing.T) { healerSoloStatusDamage(t, "YS-A27", 10311, 150) }
func TestHealerIndependentA28(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A28", domain.Learned{10302: 1}, on)
		a := healerSoloAlly(t, s, p, 2, 400, 100, 1000, 1000)
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 400, Y: 100})
		edge := healerSoloMonster(s, 901, 580, 100)
		out := healerSoloMonster(s, 902, 581, 100)
		nativeRepairCast(t, s, p, 10302, a.ID)
		mana, damage := int32(100), int32(0)
		if on {
			mana, damage = 170, 220
		}
		if a.MP != mana || 100000-m.HP != damage || 100000-edge.HP != damage || out.HP != 100000 {
			t.Fatal("mana basis/wave radius", a.MP, 100000-m.HP)
		}
		if on {
			_, n := healerSoloCount(s, m.ID)
			if n != 1 {
				t.Fatal("wave repeated")
			}
		}
	}
}
func TestHealerIndependentA29(t *testing.T) {
	for _, skill := range []domain.SkillID{10319, 10317, 10312, 10326} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			var base int32
			for _, mode := range []string{"none", "native", "power"} {
				s, p, m := healerSoloFixture(t, "YS-A29", domain.Learned{skill: 1, 10328: 1}, mode == "power")
				healerSoloUndead(s, m)
				r := float64(220)
				if skill == 10317 {
					r = 100
				}
				if skill == 10312 || skill == 10326 {
					r = 250
				}
				side := healerSoloMonster(s, 901, m.Pos.X+r-1, 100)
				out := healerSoloMonster(s, 902, m.Pos.X+r+1, 100)
				if mode != "none" {
					nativeRepairCast(t, s, p, 10328, p.ID)
				}
				s.refreshStats(p)
				nativeRepairCast(t, s, p, skill, m.ID)
				loss := 100000 - m.HP
				if mode == "none" {
					base = loss
				} else if mode == "native" {
					healerSoloNear(t, loss, base, 140)
				} else {
					healerSoloNear(t, loss, base, 190)
					if 100000-side.HP != loss || out.HP != 100000 {
						t.Fatal("release full damage/final circle geometry", skill, loss, 100000-side.HP)
					}
				}
				if legendaryCasterHasSkill(p, 10328) != (mode == "power") {
					t.Fatal("only learned equipped release must remain permanent")
				}
			}
		})
	}
}
func TestHealerIndependentA30(t *testing.T) {
	// Group heal exercises a real native next-windup healing effect. The
	// separate Heavenly Judgment test covers damage without rewriting PrepareMS.
	var base int32
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-A30", domain.Learned{10329: 1, 10304: 1}, on)
		nativeRepairCast(t, s, p, 10329, p.ID)
		p.HP = 1
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
		if len(s.casts) != 1 {
			t.Fatal("group heal rejected")
		}
		if on {
			if !s.casts[0].releaseAtWall.IsZero() {
				t.Fatal("native instant did not remove full windup")
			}
		} else if s.casts[0].releaseAtWall.IsZero() {
			t.Fatal("baseline native instant unexpectedly fully instant")
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		if !on {
			base = p.HP - 1
		} else {
			healerSoloNear(t, p.HP-1, base, 140)
		}
		if legendaryCasterHasSkill(p, 10329) != on {
			t.Fatal("instant persistence must match equipped learned power")
		}
	}
	s, p, _ := healerSoloFixture(t, "YS-A30", domain.Learned{10329: 1, 10305: 1}, true)
	a := healerSoloAlly(t, s, p, 2, 150, 100, 0, 1000)
	nativeRepairCast(t, s, p, 10329, p.ID)
	nativeRepairCast(t, s, p, 10305, a.ID)
	before := p.MP
	a.HP = 0
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10305, Target: a.ID})
	if len(s.casts) != 0 || p.MP != before {
		t.Fatal("instant bypassed native resurrection cooldown")
	}
}

func TestHealerIndependentD01(t *testing.T) {
	var base int32
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-D01", domain.Learned{10301: 1}, on)
		p.HP = 1
		nativeRepairCast(t, s, p, 10301, p.ID)
		if !on {
			base = p.HP - 1
		} else {
			healerSoloNear(t, p.HP-1, base, 160)
		}
		if universalShieldValue(p) != 0 {
			t.Fatal("direct healing generated shield")
		}
	}
}
func healerSoloCleanse(t *testing.T, id string, skill domain.SkillID, guaranteed bool) {
	t.Helper()
	successes, failures := 0, 0
	for seed := int64(1); seed <= 12; seed++ {
		s, p, _ := healerSoloFixture(t, id, domain.Learned{skill: 1}, true)
		a := healerSoloAlly(t, s, p, 2, 340, 100, 1000, 1000)
		out := healerSoloAlly(t, s, p, 3, 351, 100, 1000, 1000)
		stranger := healerSoloAlly(t, s, p, 4, 200, 100, 1000, 1000)
		stranger.Player.Party = 2
		d, _ := s.skills.Get(skill, 1)
		var removable domain.StatusID
		for _, e := range d.DirectEffects() {
			if len(e.RemoveStatusIDs) > 0 {
				removable = e.RemoveStatusIDs[0]
			}
		}
		if removable == 0 {
			t.Fatal("missing native removal list")
		}
		for _, e := range []*entity.Entity{p, a, out, stranger} {
			e.Status.Apply(domain.StatusDef{ID: removable, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, 777)
			e.Status.Apply(domain.StatusDef{ID: 199990, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, 777)
		}
		s.rng = rand.New(rand.NewSource(seed))
		nativeRepairCast(t, s, p, skill, p.ID)
		for _, e := range []*entity.Entity{p, a} {
			if e.Status.HasEffective(removable) {
				failures++
				if guaranteed {
					t.Fatal("guaranteed cleanse left native removable status", e.ID)
				}
			} else {
				successes++
			}
			if !e.Status.HasEffective(199990) {
				t.Fatal("cleanse broadened native list")
			}
		}
		if !out.Status.HasEffective(removable) || !stranger.Status.HasEffective(removable) {
			t.Fatal("cleanse hit outside radius/party")
		}
	}
	if !guaranteed && (successes == 0 || failures == 0) {
		t.Fatal("native per-target chance became guaranteed or never succeeds", successes, failures)
	}
	s, p, _ := healerSoloFixture(t, id, domain.Learned{skill: 1}, false)
	a := healerSoloAlly(t, s, p, 2, 200, 100, 1000, 1000)
	d, _ := s.skills.Get(skill, 1)
	removable := d.DirectEffects()[0].RemoveStatusIDs[0]
	a.Status.Apply(domain.StatusDef{ID: removable, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, 777)
	nativeRepairCast(t, s, p, skill, p.ID)
	if !a.Status.HasEffective(removable) {
		t.Fatal("baseline unexpectedly group cleansed")
	}
}
func TestHealerIndependentD02(t *testing.T) { healerSoloCleanse(t, "YS-D02", 10306, true) }
func TestHealerIndependentD07(t *testing.T) { healerSoloCleanse(t, "YS-D07", 10316, false) }
func TestHealerIndependentU10(t *testing.T) { healerSoloCleanse(t, "YS-U10", 10318, true) }
func TestHealerIndependentD03(t *testing.T) {
	var base int32
	var duration domain.Tick
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-D03", domain.Learned{10320: 1}, on)
		a := healerSoloAlly(t, s, p, 2, 200, 100, 100, 1000)
		b := healerSoloAlly(t, s, p, 3, 300, 100, 200, 1000)
		excluded := healerSoloAlly(t, s, p, 4, 200, 150, 900, 1000)
		out := healerSoloAlly(t, s, p, 5, 351, 100, 1, 1000)
		nativeRepairCast(t, s, p, 10320, p.ID)
		st := healerSoloStatus(t, p, 10320)
		shield := universalShieldValue(p)
		if !st.Def.InstantDeathImmune {
			t.Fatal("native death immunity lost")
		}
		if !on {
			base = shield
			duration = st.ExpireAt - s.tick
		} else {
			healerSoloNear(t, shield, base, 160)
			if st.ExpireAt-s.tick != duration || universalShieldValue(a) != shield || universalShieldValue(b) != shield || universalShieldValue(excluded) != shield || universalShieldValue(out) != 0 {
				t.Fatal("all-in-range shield/duration/radius")
			}
		}
		before := p.HP
		loss := healerSoloIncoming(s, p, m, false, true)
		if loss != max(int32(0), 1000-shield) || before-p.HP != loss || universalShieldValue(p) != max(int32(0), shield-1000) {
			t.Fatal("real shield absorption", loss, shield)
		}
	}
}
func TestHealerIndependentD04(t *testing.T) {
	for _, magic := range []bool{false, true} {
		for _, on := range []bool{false, true} {
			s, p, m := healerSoloFixture(t, "YS-D04", domain.Learned{10321: 1}, on)
			nativeRepairCast(t, s, p, 10321, p.ID)
			st := healerSoloStatus(t, p, 10321)
			want := int32(900)
			if on {
				want = 550
			}
			if got := healerSoloIncoming(s, p, m, magic, false); got != want {
				t.Fatal("physical/magic reduction consumer", magic, on, got, want)
			}
			if st.Def.DurationSec != 300 {
				t.Fatal("duration changed")
			}
			s.tick = st.ExpireAt
			s.stepStatus()
			p.HP = 10000
			if got := healerSoloIncoming(s, p, m, magic, false); got != 1000 {
				t.Fatal("defense persists after native expiry", got)
			}
		}
	}
}
func healerSoloWindup(t *testing.T, id string, skill domain.SkillID, pct int32, hot, steady bool) {
	t.Helper()
	for _, mode := range []string{"off", "active", "missing_condition", "cancelled"} {
		s, p, m := healerSoloFixture(t, id, domain.Learned{skill: 1, 10303: 1}, mode != "off")
		if hot && mode != "missing_condition" {
			nativeRepairCast(t, s, p, 10303, p.ID)
		}
		p.HP, p.MaxHP = 10000, 10000
		target := p
		if skill == 10305 {
			target = healerSoloAlly(t, s, p, 2, 150, 100, 0, 1000)
		}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: target.ID})
		if len(s.casts) != 1 || s.casts[0].releaseAtWall.IsZero() {
			t.Fatal("expected actual native windup")
		}
		cast := s.casts[0]
		if mode == "cancelled" {
			s.clearCastWindup(p.ID)
		}
		s.combatTiming.CastInterruptBaseBP = 10000
		s.combatTiming.CastInterruptMinBP = 10000
		got := healerSoloIncoming(s, p, m, false, true)
		want := int32(1000)
		protected := mode == "active" || mode == "missing_condition" && !hot
		if id == "YS-D05" && mode == "cancelled" {
			protected = true
		}
		if protected {
			want = 1000 * pct / 100
		}
		if got != want {
			t.Fatal("windup HP reduction", id, mode, got, want)
		}
		if protected && steady && mode != "cancelled" && cast.windup.finished {
			t.Fatal("ordinary damage interrupted protected windup")
		}
		if !protected && mode != "cancelled" && !cast.windup.finished {
			t.Fatal("baseline ordinary hit failed to interrupt")
		}
		if protected && steady && mode != "cancelled" {
			// Hard native control still cancels the actual accepted cast at settlement.
			s.applyStatusDef(p, domain.StatusDef{ID: 1002, Level: 1, DurationSec: 5, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun}, m.ID)
			before := target.HP
			s.stepCastsAt(s.now().Add(time.Minute), false)
			s.stepCastsAt(s.now().Add(2*time.Minute), false)
			if target.HP > before {
				t.Fatal("hard stun failed to cancel protected heal/revive")
			}
		}
		s.clearCastWindup(p.ID)
		p.HP = 10000
		cancelWant := int32(1000)
		if id == "YS-D05" && mode != "off" && mode != "missing_condition" {
			cancelWant = 750
		}
		if got = healerSoloIncoming(s, p, m, false, false); got != cancelWant {
			t.Fatal("wrong protection after cancellation", got, cancelWant)
		}
	}
}
func TestHealerIndependentD05(t *testing.T) {
	healerSoloWindup(t, "YS-D05", 10304, 75, true, true)
	healerSoloWindup(t, "YS-D05", 10305, 75, true, true)
}
func TestHealerIndependentD06(t *testing.T) { healerSoloWindup(t, "YS-D06", 10304, 60, false, true) }
func TestHealerIndependentD08(t *testing.T) { healerSoloWindup(t, "YS-D08", 10305, 40, false, false) }
func TestHealerIndependentD09(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, active := range []bool{false, true} {
			s, p, m := healerSoloFixture(t, "YS-D09", domain.Learned{10330: 1}, on)
			if active {
				nativeRepairCast(t, s, p, 10330, p.ID)
			}
			p.HP, p.MaxHP = 10000, 10000
			before := m.HP
			got := healerSoloIncoming(s, p, m, false, true)
			want := int32(1000)
			if on && active {
				want = 650
			}
			if got != want {
				t.Fatal("actual native basic mitigation", on, active, got)
			}
			if active {
				a := healerSoloStatus(t, p, 10330)
				pct := int32(10)
				if on {
					pct = 17
				}
				if a.Def.PhysicalReflectPct != pct || a.Def.DurationSec != 20 || before-m.HP != got*pct/100 {
					t.Fatal("original reflection 175%/duration", a.Def.PhysicalReflectPct, before-m.HP)
				}
			}
			p.HP = 10000
			if got = healerSoloIncoming(s, p, m, false, false); got != 1000 {
				t.Fatal("nonbasic skill improperly mitigated", got)
			}
		}
	}
	TestReviewYS09ActualRiftHazard(t)
}
func TestHealerIndependentD10(t *testing.T) {
	for _, owner := range []domain.EntityID{0, 1, 777} {
		s, p, m := healerSoloFixture(t, "YS-D10", domain.Learned{10325: 1}, true)
		var old domain.Active
		if owner != 0 {
			nativeRepairCast(t, s, p, 10325, m.ID)
			old = healerSoloStatus(t, m, 10325)
			m.Status.Remove(old.Def.ID)
			m.Status.Apply(old.Def, s.tick, owner)
			old = healerSoloStatus(t, m, 10325)
		}
		want := int32(1000)
		if owner == p.ID {
			want = 600
		}
		if got := healerSoloIncoming(s, p, m, false, false); got != want {
			t.Fatal("shackle ownership damage", owner, got, want)
		}
		if owner != 0 {
			now := healerSoloStatus(t, m, 10325)
			if now.Source != old.Source || now.ExpireAt != old.ExpireAt {
				t.Fatal("incoming mitigation consumed/refreshed shackle")
			}
		}
		other := healerSoloMonster(s, 901, 150, 150)
		if got := healerSoloIncoming(s, p, other, true, false); got != 1000 {
			t.Fatal("unshackled source gained reduction")
		}
	}
}

func healerSoloResourceStart(s *Scene, p *entity.Entity) int32 {
	s.refreshStats(p)
	p.MP = p.MaxMP
	return p.MP
}
func TestHealerIndependentU01(t *testing.T) {
	var baseTick, baseCost int32
	var duration domain.Tick
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U01", domain.Learned{10303: 1}, on)
		startMP := healerSoloResourceStart(s, p)
		p.HP = 1
		nativeRepairCast(t, s, p, 10303, p.ID)
		cost := startMP - p.MP
		a := healerSoloStatus(t, p, 10303)
		if !on {
			baseCost = cost
			duration = a.ExpireAt - s.tick
		} else {
			healerSoloNear(t, cost, baseCost, 80)
			if a.ExpireAt-s.tick != duration {
				t.Fatal("HOT duration changed")
			}
			before := p.MP
			nativeRepairCast(t, s, p, 10303, p.ID)
			if delta := before - p.MP; delta < cost-1 || delta > cost+1 {
				t.Fatal("zero cooldown second cast rejected/cost changed")
			}
			count := 0
			p.Status.EachEffective(func(x domain.Active) {
				if x.Def.SkillSnapshotID == 10303 {
					count++
				}
			})
			if count != 1 {
				t.Fatal("same-caster HOT stacked", count)
			}
		}
		before := p.HP
		s.tick += domain.Ticks(1000)
		s.stepStatus()
		tick := p.HP - before
		if !on {
			baseTick = tick
		} else {
			healerSoloNear(t, tick, baseTick, 150)
		}
		if tick <= 0 {
			t.Fatal("HOT did not heal")
		}
		before = p.HP
		s.stepStatus()
		if p.HP != before {
			t.Fatal("same tick healed twice")
		}
		s.tick = a.ExpireAt + domain.Ticks(1000)
		s.stepStatus()
		if legendaryCasterHasSkill(p, 10303) {
			t.Fatal("HOT outlived native duration")
		}
	}
}
func TestHealerIndependentU02(t *testing.T) {
	for _, rank := range []int32{1, 2} {
		var baseShield, baseCost int32
		var duration domain.Tick
		for _, on := range []bool{false, true} {
			s, p, _ := healerSoloFixture(t, "YS-U02", domain.Learned{10322: rank, 10320: 1}, on)
			startMP := healerSoloResourceStart(s, p)
			nativeRepairCast(t, s, p, 10322, p.ID)
			cost := startMP - p.MP
			nativeRepairCast(t, s, p, 10320, p.ID)
			a := healerSoloStatus(t, p, 10320)
			shield := universalShieldValue(p)
			if !on {
				baseShield = shield
				baseCost = cost
				duration = a.ExpireAt - s.tick
			} else {
				healerSoloNear(t, shield, baseShield, 150)
				healerSoloNear(t, cost, baseCost, 70)
				if a.ExpireAt-s.tick != duration {
					t.Fatal("native focus duration changed", rank, a.ExpireAt-s.tick, duration, a.Def.DurationSec)
				}
				if rank == 1 && a.Def.DurationSec != 90 {
					t.Fatal("rank one 50% native shield duration", a.Def.DurationSec)
				}
			}
			if legendaryCasterHasSkill(p, 10322) != on {
				t.Fatal("focus persistence must match equipped learned power")
			}
		}
	}
	for _, skill := range []domain.SkillID{10301, 10319} {
		var baseline int32
		for _, on := range []bool{false, true} {
			s, p, m := healerSoloFixture(t, "YS-U02", domain.Learned{10322: 1, skill: 1}, on)
			nativeRepairCast(t, s, p, 10322, p.ID)
			p.Stats = domain.Stats{MAtk: 100, Hit: 100000}
			target := m
			if skill == 10301 {
				target = p
				p.HP = 1
			}
			before := target.HP
			nativeRepairCast(t, s, p, skill, target.ID)
			delta := target.HP - before
			if !on {
				baseline = delta
			} else if delta != baseline {
				t.Fatal("focus boosted direct damage/heal", skill, delta, baseline)
			}
		}
	}
}
func TestHealerIndependentU03(t *testing.T) {
	for _, mode := range []string{"overflow", "no_overflow", "deficit_cap", "off"} {
		s, p, _ := healerSoloFixture(t, "YS-U03", domain.Learned{10302: 1}, mode != "off")
		a := healerSoloAlly(t, s, p, 2, 400, 100, 1000, 1000)
		a.MP = 1000
		first := healerSoloAlly(t, s, p, 3, 500, 100, 1000, 1000)
		second := healerSoloAlly(t, s, p, 4, 550, 100, 1000, 1000)
		third := healerSoloAlly(t, s, p, 5, 600, 100, 1000, 1000)
		outside := healerSoloAlly(t, s, p, 6, 751, 100, 1000, 1000)
		first.MaxMP, first.MP = 200, 0
		second.MaxMP, second.MP = 400, 0
		third.MaxMP, third.MP = 1000, 100
		outside.MP = 0
		if mode == "no_overflow" {
			a.MP = 800
		}
		if mode == "deficit_cap" {
			first.MP = 190
			second.MP = 390
			third.MP = 999
		}
		nativeRepairCast(t, s, p, 10302, a.ID)
		want1, want2 := int32(0), int32(0)
		if mode == "overflow" {
			want1, want2 = 40, 60
		}
		if mode == "deficit_cap" {
			want1, want2 = 200, 400
		}
		if mode == "no_overflow" && a.MP != 900 || mode != "no_overflow" && a.MP != 1000 {
			t.Fatal("main restoration altered")
		}
		if first.MP != want1 || second.MP != want2 {
			t.Fatal("ordered transfer cap/overflow budget", mode, first.MP, second.MP, want1, want2)
		}
		wantThird := int32(100)
		if mode == "deficit_cap" {
			wantThird = 999
		}
		if third.MP != wantThird || outside.MP != 0 {
			t.Fatal("third recipient/range spill")
		}
	}
}
func TestHealerIndependentU04(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U04", domain.Learned{10301: 1}, on)
		p.HP = 1
		a := healerSoloAlly(t, s, p, 2, 340, 100, 10, 1000)
		b := healerSoloAlly(t, s, p, 3, 300, 100, 20, 2000)
		excluded := healerSoloAlly(t, s, p, 4, 200, 100, 900, 1000)
		out := healerSoloAlly(t, s, p, 5, 351, 100, 1, 1000)
		nativeRepairCast(t, s, p, 10301, p.ID)
		if p.HP != 1101 {
			t.Fatal("main heal changed", p.HP)
		}
		wantA, wantB := int32(10), int32(20)
		if on {
			wantA, wantB = 210, 320
		}
		wantExcluded := int32(900)
		if on {
			wantExcluded = 1000
		}
		if a.HP != wantA || b.HP != wantB || excluded.HP != wantExcluded || out.HP != 1 {
			t.Fatal("all-in-range own-maxHP healing", on, a.HP, b.HP)
		}
		for _, e := range []*entity.Entity{p, a, b} {
			if universalShieldValue(e) != 0 {
				t.Fatal("secondary heal generated shield")
			}
		}
	}
	for _, full := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U04", domain.Learned{10304: 1}, true)
		a := healerSoloAlly(t, s, p, 2, 200, 100, 1, 1000)
		b := healerSoloAlly(t, s, p, 3, 250, 100, 300, 1000)
		if full {
			a.HP = a.MaxHP
			b.HP = b.MaxHP
		}
		s.outbox = nil
		nativeRepairCast(t, s, p, 10304, p.ID)
		want := int32(105)
		if !full {
			want -= 105 * 35 / 100
		}
		if 10000-p.MP != want {
			t.Fatal("group-heal effective-only refund", full, 10000-p.MP, want)
		}
		if !full {
			first := domain.EntityID(0)
			for _, x := range s.outbox {
				if h, ok := x.ev.(event.HealDone); ok && h.Amount > 0 {
					first = h.Dst
					break
				}
			}
			if first != a.ID {
				t.Fatal("low health priority", first)
			}
		}
	}
}
func TestHealerIndependentU05(t *testing.T) {
	for _, remote := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U05", domain.Learned{10311: 1}, true)
		a := healerSoloAlly(t, s, p, 2, 650, 100, 1000, 1000)
		center := p
		if remote {
			center = a
		}
		b := healerSoloAlly(t, s, p, 3, center.Pos.X+220, 100, 1000, 1000)
		out := healerSoloAlly(t, s, p, 4, center.Pos.X+221, 100, 1000, 1000)
		startMP := healerSoloResourceStart(s, p)
		nativeRepairCast(t, s, p, 10311, center.ID)
		for _, e := range []*entity.Entity{p, center, b} {
			st := healerSoloStatus(t, e, 10311)
			if st.Def.DurationSec != 30 || len(st.Def.Affixes) != 1 || st.Def.Affixes[0].Value != 26 {
				t.Fatal("haste strength/duration", st.Def.Affixes)
			}
			count := 0
			e.Status.EachEffective(func(a domain.Active) {
				if a.Def.SkillSnapshotID == 10311 {
					count++
				}
			})
			if count != 1 {
				t.Fatal("same recipient doubled")
			}
		}
		if legendaryCasterHasSkill(out, 10311) || startMP-p.MP != 105 {
			t.Fatal("range or cost changed")
		}
	}
	s, p, _ := healerSoloFixture(t, "YS-U05", domain.Learned{10311: 1}, true)
	a := healerSoloAlly(t, s, p, 2, 701, 100, 1000, 1000)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10311, Target: a.ID})
	if len(s.casts) != 0 || p.MP != 10000 {
		t.Fatal("out-of-range haste accepted")
	}
}
func TestHealerIndependentU06(t *testing.T) {
	for _, learned := range []bool{false, true} {
		for _, on := range []bool{false, true} {
			skills := domain.Learned{10307: 1}
			if learned {
				skills[10321] = 1
				skills[10311] = 1
			}
			s, p, _ := healerSoloFixture(t, "YS-U06", skills, on)
			a := healerSoloAlly(t, s, p, 2, 290, 100, 1000, 1000)
			out := healerSoloAlly(t, s, p, 3, 301, 100, 1000, 1000)
			startMP := healerSoloResourceStart(s, p)
			nativeRepairCast(t, s, p, 10307, p.ID)
			if startMP-p.MP != 35 {
				t.Fatal("granted buffs charged extra mana")
			}
			for _, e := range []*entity.Entity{p, a} {
				healerSoloStatus(t, e, 10307)
				for _, skill := range []domain.SkillID{10321, 10311} {
					want := learned && on
					if legendaryCasterHasSkill(e, skill) != want {
						t.Fatal("learned same-recipient buff grant", on, learned, e.ID, skill)
					}
					if want {
						st := healerSoloStatus(t, e, skill)
						duration := int32(300)
						if skill == 10311 {
							duration = 30
						}
						if st.Def.DurationSec != duration {
							t.Fatal("granted duration changed")
						}
					}
				}
			}
			if legendaryCasterHasSkill(out, 10307) || legendaryCasterHasSkill(out, 10321) || legendaryCasterHasSkill(out, 10311) {
				t.Fatal("grant independently expanded native recipients")
			}
			for _, o := range s.outbox {
				if e, ok := o.ev.(event.SkillCastChanged); ok && (e.Skill == 10321 || e.Skill == 10311) {
					t.Fatal("grant became another active cast")
				}
			}
		}
	}
}
func TestHealerIndependentU07(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, has := range []bool{false, true} {
			s, p, _ := healerSoloFixture(t, "YS-U07", domain.Learned{10306: 1}, on)
			startMP := healerSoloResourceStart(s, p)
			if has {
				p.Status.Apply(domain.StatusDef{ID: 1001, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, 777)
			}
			nativeRepairCast(t, s, p, 10306, p.ID)
			if startMP-p.MP != 48 {
				t.Fatal("cleanse no longer refunds empty casts", startMP-p.MP)
			}
			if p.Status.HasEffective(legendaryHealerEscortStatus) != on {
				t.Fatal("successful release escort eligibility", on, has)
			}
			if got := p.CapturePlayerRuntime(s.tick).SkillReadyIn[10306]; got != domain.Ticks(1000) {
				t.Fatal("escort changed native cooldown", got)
			}
		}
	}
}

func TestHealerIndependentU08(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U08", domain.Learned{10301: 1, 10327: 1}, on)
		nativeRepairCast(t, s, p, 10327, p.ID)
		startMP := healerSoloResourceStart(s, p)
		a := healerSoloAlly(t, s, p, 2, 949, 100, 1, 1000)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10301, Target: a.ID})
		if !on {
			if len(s.casts) != 0 || p.MP != startMP {
				t.Fatal("baseline accepted extended range")
			}
			continue
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		if a.HP != 201 || startMP-p.MP != (23*55+99)/100 {
			t.Fatal("single friendly window range/mana/heal", a.HP, startMP-p.MP)
		}
	}

	for _, point := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U08", domain.Learned{10327: 1}, true)
		center := p.Pos
		if point {
			center.X = 700
		}
		a := healerSoloAlly(t, s, p, 2, center.X+299, 100, 1000, 1000)
		out := healerSoloAlly(t, s, p, 3, center.X+301, 100, 1000, 1000)
		a.Player.Party = 2
		out.Player.Party = 2
		for _, e := range []*entity.Entity{a, out} {
			e.Status.Apply(domain.StatusDef{ID: 1029, DurationSec: 60, Invisible: true}, s.tick, e.ID)
		}
		startMP := healerSoloResourceStart(s, p)
		cmd := UseSkill{ID: p.ID, Skill: 10327, Target: p.ID}
		if point {
			cmd.Target = 0
			cmd.At = &center
		}
		s.onUseSkill(cmd)
		if len(s.casts) != 1 {
			t.Fatal("native reveal not admitted")
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		if entityInvisible(a) || !entityInvisible(out) {
			t.Fatal("real remote/native reveal radius")
		}
		st := healerSoloStatus(t, a, 10327)
		if st.Def.DurationSec != 10 {
			t.Fatal("native reveal duration changed", st.Def.DurationSec)
		}
		if startMP-p.MP != 63 {
			t.Fatal("AoE reveal incorrectly received single-target discount")
		}
	}
}
func TestHealerIndependentU09(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U09", domain.Learned{10305: 1}, on)
		a := healerSoloAlly(t, s, p, 2, 150, 100, 0, 1000)
		a.Player.DeathLoss = domain.DeathLoss{Exp: 100, Money: 100}
		beforeExp := a.Player.Char.Exp
		nativeRepairCast(t, s, p, 10305, a.ID)
		wantHP, wantCost := int32(50), int32(81)
		if on {
			wantHP = 700
			wantCost -= 81 * 50 / 100
		}
		if a.HP != wantHP || 10000-p.MP != wantCost {
			t.Fatal("successful native resurrection HP/refund", on, a.HP, 10000-p.MP)
		}
		money, _ := inventoryMoney(a.Player.Char.Money)
		if a.Player.Char.Exp-beforeExp != 60 || money != 60 || !a.Player.DeathLoss.Empty() {
			t.Fatal("native death-loss refund changed")
		}
	}
	s, p, _ := healerSoloFixture(t, "YS-U09", domain.Learned{10305: 1}, true)
	a := healerSoloAlly(t, s, p, 2, 150, 100, 0, 1000)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10305, Target: a.ID})
	paid := 10000 - p.MP
	a.HP = 500
	s.stepCastsAt(s.now().Add(time.Minute), false)
	s.stepCastsAt(s.now().Add(2*time.Minute), false)
	if 10000-p.MP != paid || a.HP != 500 {
		t.Fatal("unsuccessful resurrection received refund/HP floor")
	}
}

func TestHealerIndependentNativeWindupControl(t *testing.T) {
	for _, id := range []string{"none", "YS-D05", "YS-D06"} {
		for _, status := range []domain.StatusID{domain.StatusStun, domain.StatusSilence} {
			t.Run(fmt.Sprintf("%s/%d", id, status), func(t *testing.T) {
				s, p, m := healerSoloFixture(t, "YS-D05", domain.Learned{10304: 1, 10303: 1}, false)
				if id != "none" {
					nativeRepairEquip(t, s, p, id)
				}
				wall := time.Unix(100000, 0)
				s.now = func() time.Time { return wall }
				if id == "YS-D05" {
					nativeRepairCast(t, s, p, 10303, p.ID)
				}
				healerSoloResourceStart(s, p)
				p.HP = 1
				s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
				paidMP := p.MP
				if len(s.casts) != 1 {
					t.Fatal("native heal windup missing")
				}
				cast := s.casts[0]
				wall = wall.Add(2 * time.Second)
				s.tick += domain.Ticks(2000)
				d, ok := s.statuses.Get(status, 1)
				if !ok {
					t.Fatal("native control absent")
				}
				if !s.applyStatusDef(p, d, m.ID) || canCast(p) {
					t.Fatal("native control not effective before release", d)
				}
				var active domain.Active
				p.Status.EachEffective(func(a domain.Active) {
					if a.Def.ID == status {
						active = a
					}
				})
				releaseTick := s.tick + domain.Ticks(int(cast.releaseAtWall.Sub(wall).Milliseconds()))
				if active.ExpireAt <= releaseTick {
					t.Fatal("test control expires before release", active.ExpireAt, releaseTick)
				}
				if !cast.windup.interrupted {
					t.Fatal("effective native hard control did not immediately cancel pending windup", id, status)
				}
				before := p.HP
				wall = cast.releaseAtWall
				s.tick = releaseTick
				s.stepCastsAt(wall, false)
				if p.HP != before {
					t.Fatal("effective native hard control did not cancel unreleased windup", id, status, before, p.HP)
				}
				if len(s.casts) != 0 || p.MP != paidMP {
					t.Fatal("blocked windup retained or charged again")
				}
				for _, out := range s.outbox {
					if e, ok := out.ev.(event.SkillCastChanged); ok && e.Skill == 10304 && e.Phase == event.SkillCastRelease {
						t.Fatal("cancelled windup emitted release")
					}
				}
			})
		}
	}
}

func TestHealerIndependentNativeControlBoundaries(t *testing.T) {
	t.Run("released_projectile_survives", func(t *testing.T) {
		s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10319: 1}, true)
		wall := time.Unix(200000, 0)
		s.now = func() time.Time { return wall }
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10319, Target: m.ID})
		s.stepCastsAt(wall, false)
		if len(s.casts) != 1 || !s.casts[0].released {
			t.Fatal("expected actual released projectile before impact")
		}
		d, _ := s.statuses.Get(domain.StatusStun, 1)
		if !s.applyStatusDef(p, d, m.ID) {
			t.Fatal("control rejected")
		}
		wall = wall.Add(time.Second)
		s.stepCastsAt(wall, false)
		if m.HP >= 100000 || len(s.casts) != 0 {
			t.Fatal("hard control revoked released projectile")
		}
	})
	t.Run("rejected_control_preserves_windup", func(t *testing.T) {
		s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10304: 1}, true)
		wall := time.Unix(200000, 0)
		s.now = func() time.Time { return wall }
		p.HP = 1
		p.Status.Apply(domain.StatusDef{ID: 199989, DurationSec: 30, BlockHarmful: true}, s.tick, p.ID)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
		if len(s.casts) != 1 {
			t.Fatal("missing windup")
		}
		cast := s.casts[0]
		d, _ := s.statuses.Get(domain.StatusStun, 1)
		if s.applyStatusDef(p, d, m.ID) || cast.windup.interrupted {
			t.Fatal("immune/rejected control cancelled cast")
		}
		wall = cast.releaseAtWall.Add(time.Second)
		s.stepCastsAt(wall, false)
		s.stepCastsAt(wall.Add(time.Second), false)
		if p.HP <= 1 {
			t.Fatal("rejected control prevented real heal")
		}
	})
	t.Run("suppressed_control_does_not_cancel", func(t *testing.T) {
		s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10304: 1}, true)
		p.HP = 1
		p.Status.Apply(domain.StatusDef{ID: 199988, DurationSec: 30, SuppressHarmful: true}, s.tick, p.ID)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
		cast := s.casts[0]
		d, _ := s.statuses.Get(domain.StatusStun, 1)
		if !s.applyStatusDef(p, d, m.ID) {
			t.Fatal("stored suppressed control was not admitted")
		}
		if !canCast(p) || cast.windup.interrupted {
			t.Fatal("suppressed layer incorrectly cancelled windup")
		}
		s.stepCastsAt(cast.releaseAtWall.Add(time.Second), false)
		s.stepCastsAt(cast.releaseAtWall.Add(2*time.Second), false)
		if p.HP <= 1 {
			t.Fatal("effective-state control check blocked heal")
		}
	})

	t.Run("short_control_cannot_resurrect_cancelled_cast", func(t *testing.T) {
		s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10304: 1}, true)
		wall := time.Unix(200000, 0)
		s.now = func() time.Time { return wall }
		p.HP = 1
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
		cast := s.casts[0]
		d, _ := s.statuses.Get(domain.StatusStun, 27)
		if d.DurationSec != 3 {
			t.Fatal("native short stun fixture", d.DurationSec)
		}
		if !s.applyStatusDef(p, d, m.ID) || !cast.windup.interrupted {
			t.Fatal("short native stun did not cancel")
		}
		s.tick += domain.Ticks(3000)
		s.stepStatus()
		if !canCast(p) {
			t.Fatal("native short control failed to expire")
		}
		before := p.HP
		wall = cast.releaseAtWall.Add(time.Second)
		s.stepCastsAt(wall, false)
		s.stepCastsAt(wall.Add(time.Second), false)
		if p.HP != before || len(s.casts) != 0 {
			t.Fatal("expired short control resurrected cancelled heal")
		}
	})
}

func TestHealerIndependentCombinations(t *testing.T) {
	t.Run("prism_echo_each_distinct_victim", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10319: 1}, "YS-A01", "YS-A02")
		side := healerSoloMonster(s, 901, 140, 100)
		nativeRepairCast(t, s, p, 10319, m.ID)
		if 100000-m.HP != 388 || 100000-side.HP != 126+126*85/100 {
			t.Fatal("secondary lost distinct echo or primary lost full hit", 100000-m.HP, 100000-side.HP)
		}
		_, primaryCount := healerSoloCount(s, m.ID)
		_, sideCount := healerSoloCount(s, side.ID)
		if primaryCount != 2 || sideCount != 2 {
			t.Fatal("echo generation scope", primaryCount, sideCount)
		}
	})
	t.Run("healing_prism_amplifier", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-A23", "YS-A01", "YS-A03")
		p.HP = 1
		s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 330, Y: 100})
		side := healerSoloMonster(s, 901, 400, 100)
		extra := healerSoloMonster(s, 902, 410, 100)
		third := healerSoloMonster(s, 903, 420, 100)
		excluded := healerSoloMonster(s, 904, 430, 100)
		nativeRepairCast(t, s, p, 10301, p.ID)
		if p.HP != 1101 || 100000-m.HP != 1760 {
			t.Fatal("heal-derived light lost direct strength or main heal")
		}
		for _, e := range []*entity.Entity{side, extra, third, excluded} {
			if 100000-e.HP != 1056 {
				t.Fatal("heal-derived split amplification", e.ID, 100000-e.HP)
			}
			_, n := healerSoloCount(s, e.ID)
			if n != 1 {
				t.Fatal("healing child recurred")
			}
		}
	})
	t.Run("shackle_damage_and_slow", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10325: 1}, "YS-A18", "YS-A19")
		side := healerSoloMonster(s, 901, 330, 100)
		nativeRepairCast(t, s, p, 10325, m.ID)
		for _, v := range []*entity.Entity{m, side} {
			if 100000-v.HP != 320 {
				t.Fatal("A18 did not amplify granted shackle", 100000-v.HP)
			}
			a := healerSoloStatus(t, v, 10325)
			if a.Def.Affixes[0].Value != 45 {
				t.Fatal("native slow amplification")
			}
		}
		if sqDist(side.Pos, m.Pos) != 80*80 {
			t.Fatal("composition lost pull")
		}
	})
	t.Run("focused_group_shield_strongest_burst", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10322: 1, 10320: 1}, "YS-U02", "YS-D03", "YS-A21")
		a := healerSoloAlly(t, s, p, 2, 120, 100, 1, 1000)
		b := healerSoloAlly(t, s, p, 3, 150, 100, 1, 1000)
		nativeRepairCast(t, s, p, 10322, p.ID)
		nativeRepairCast(t, s, p, 10320, p.ID)
		shield := universalShieldValue(p)
		if shield <= 700 || universalShieldValue(a) != shield || universalShieldValue(b) != shield || 100000-m.HP != shield*80/100 {
			t.Fatal("final amplified shields not consumed once by burst", shield, 100000-m.HP)
		}
		_, n := healerSoloCount(s, m.ID)
		if n != 1 {
			t.Fatal("overlapping group-shield burst stacked", n)
		}
	})
	t.Run("full_strength_extra_heals", func(t *testing.T) {
		s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-U04", "YS-D01")
		p.HP = 1
		a := healerSoloAlly(t, s, p, 2, 140, 100, 1, 1000)
		b := healerSoloAlly(t, s, p, 3, 150, 100, 1, 2000)
		nativeRepairCast(t, s, p, 10301, p.ID)
		if p.HP != 1761 || a.HP != 321 || b.HP != 481 {
			t.Fatal("full D01 modifier missing on extra recipients", p.HP, a.HP, b.HP)
		}
		if universalShieldValue(p) != 0 || universalShieldValue(a) != 0 {
			t.Fatal("extra healing generated shield")
		}
	})
	t.Run("triage_does_not_repeat_granted_bolt", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10301: 1}, "YS-U04", "YS-A23")
		p.HP = 1
		a := healerSoloAlly(t, s, p, 2, 140, 100, 1, 1000)
		b := healerSoloAlly(t, s, p, 3, 150, 100, 1, 1000)
		nativeRepairCast(t, s, p, 10301, p.ID)
		if a.HP != 201 || b.HP != 201 || 100000-m.HP != 1100 {
			t.Fatal("secondary heals generated extra bolts", a.HP, b.HP, 100000-m.HP)
		}
		_, n := healerSoloCount(s, m.ID)
		if n != 1 {
			t.Fatal("recursive granted bolt")
		}
	})
	t.Run("mana_overflow_wave_once", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10302: 1}, "YS-U03", "YS-A28")
		a := healerSoloAlly(t, s, p, 2, 400, 100, 1000, 1000)
		a.MP = 1000
		b := healerSoloAlly(t, s, p, 3, 430, 100, 1000, 1000)
		c := healerSoloAlly(t, s, p, 4, 450, 100, 1000, 1000)
		s.aoi.Move(m, a.Pos)
		nativeRepairCast(t, s, p, 10302, a.ID)
		if a.MP != 1000 || b.MP != 170 || c.MP != 0 || 100000-m.HP != 220 {
			t.Fatal("amplified mana overflow/wave", a.MP, b.MP, c.MP, 100000-m.HP)
		}
		_, n := healerSoloCount(s, m.ID)
		if n != 1 {
			t.Fatal("overflow transfer repeated support wave")
		}
	})
	t.Run("release_final_rectangle", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10317: 1, 10328: 1}, "YS-A29", "YS-A08")
		long := healerSoloMonster(s, 901, 799, 100)
		wide := healerSoloMonster(s, 902, 300, 229)
		outLong := healerSoloMonster(s, 903, 801, 100)
		outWide := healerSoloMonster(s, 904, 300, 231)
		nativeRepairCast(t, s, p, 10328, p.ID)
		nativeRepairCast(t, s, p, 10317, m.ID)
		damage := 100000 - m.HP
		for _, e := range []*entity.Entity{m, long, wide} {
			if 100000-e.HP != damage || damage <= 0 {
				t.Fatal("final rectangle full damage")
			}
			_, n := healerSoloCount(s, e.ID)
			if n != 1 {
				t.Fatal("shape composition duplicated hit")
			}
		}
		if outLong.HP != 100000 || outWide.HP != 100000 {
			t.Fatal("final rectangle extent leaked")
		}
	})
	t.Run("release_final_circle", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10317: 1, 10328: 1}, "YS-A29", "YS-A06")
		side := healerSoloMonster(s, 901, 429, 100)
		out := healerSoloMonster(s, 902, 431, 100)
		nativeRepairCast(t, s, p, 10328, p.ID)
		nativeRepairCast(t, s, p, 10317, m.ID)
		if side.HP == 100000 || out.HP != 100000 {
			t.Fatal("final native circle +100 not consumed")
		}
		_, n := healerSoloCount(s, m.ID)
		if n != 1 {
			t.Fatal("two shape powers duplicated hit")
		}
	})
}

func TestHealerIndependentA30NativeDamage(t *testing.T) {
	var baseline int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A30", domain.Learned{10329: 1, 10313: 1}, on)
		nativeRepairCast(t, s, p, 10329, p.ID)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10313, Target: m.ID})
		if len(s.casts) != 1 {
			t.Fatal("native Heavenly Judgment rejected")
		}
		if got := s.casts[0].releaseAtWall.IsZero(); got != on {
			t.Fatal("native damage windup not fully removed", on, got)
		}
		s.stepCastsAt(s.now().Add(time.Minute), false)
		s.stepCastsAt(s.now().Add(2*time.Minute), false)
		loss := 100000 - m.HP
		if !on {
			baseline = loss
		} else {
			healerSoloNear(t, loss, baseline, 140)
		}
		if baseline <= 0 {
			t.Fatal("native damaging windup did not hit")
		}
	}
}
func TestHealerIndependentA07OverlapUsesStrongerField(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A07", domain.Learned{10317: 5}, false)
	nativeRepairCast(t, s, p, 10317, m.ID)
	strongBase := 100000 - m.HP
	s, p, m = healerSoloFixture(t, "YS-A07", domain.Learned{10317: 1}, true)
	nativeRepairCast(t, s, p, 10317, m.ID)
	p.Player.Char.Skills[10317] = 5
	p.Player.ReduceRemainingSkillCooldown(10317, s.tick, domain.Ticks(5000))
	nativeRepairCast(t, s, p, 10317, m.ID)
	before := m.HP
	s.tick += domain.Ticks(1000)
	s.stepLegendaryPowers()
	healerSoloNear(t, before-m.HP, strongBase, 30)
}
func TestHealerIndependentU02ReductionStrength(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-U02", domain.Learned{10322: 1, 10321: 1}, on)
		nativeRepairCast(t, s, p, 10322, p.ID)
		nativeRepairCast(t, s, p, 10321, p.ID)
		want := int32(900)
		if on {
			want = 850
		}
		if loss := healerSoloIncoming(s, p, m, true, false); loss != want {
			t.Fatal("50% native buff strength must be 10% to 15%, not +50 points", loss, want)
		}
	}
}
func healerSoloTrain(p *entity.Entity, learned domain.Learned) {
	p.Player.Char.Race = domain.Healer
	p.Player.Char.Level = 100
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.Player.Char.Skills = learned
	p.MP, p.MaxMP = 10000, 10000
	p.Stats = domain.Stats{MAtk: 100, Hit: 100000}
}
func TestHealerIndependentA20DoesNotModifyOtherCaster(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A20", domain.Learned{10330: 1}, true)
	a := healerSoloAlly(t, s, p, 2, 140, 100, 1000, 1000)
	healerSoloTrain(a, domain.Learned{10330: 1, 10319: 1})
	nativeRepairCast(t, s, a, 10330, a.ID)
	a.Stats = domain.Stats{MAtk: 100, Hit: 100000}
	nativeRepairCast(t, s, a, 10319, m.ID)
	if 100000-m.HP != 210 {
		t.Fatal("another caster inherited holder's A20", 100000-m.HP)
	}
}
func TestHealerIndependentA26AllyHolyDamage(t *testing.T) {
	var baseline int32
	for _, on := range []bool{false, true} {
		s, p, m := healerSoloFixture(t, "YS-A26", domain.Learned{10321: 1}, on)
		a := healerSoloAlly(t, s, p, 2, 140, 100, 1000, 1000)
		healerSoloTrain(a, domain.Learned{10319: 1})
		nativeRepairCast(t, s, p, 10321, p.ID)
		a.Stats = domain.Stats{MAtk: 100, Hit: 100000}
		nativeRepairCast(t, s, a, 10319, m.ID)
		if !on {
			baseline = 100000 - m.HP
		} else {
			healerSoloNear(t, 100000-m.HP, baseline, 140)
		}
	}
}
func TestHealerIndependentA20SelfCenterCombination(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10330: 1, 10317: 1}, "YS-A20", "YS-A08")
	s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 500, Y: 100})
	side := healerSoloMonster(s, 901, 100, 319)
	out := healerSoloMonster(s, 902, 100, 321)
	nativeRepairCast(t, s, p, 10330, p.ID)
	nativeRepairCast(t, s, p, 10317, m.ID)
	loss := 100000 - m.HP
	if loss <= 0 || 100000-side.HP != loss || out.HP != 100000 {
		t.Fatal("legal caster-centered shape must use radius220 and retain distant primary", loss, 100000-side.HP)
	}
	for _, v := range []*entity.Entity{m, side} {
		_, n := healerSoloCount(s, v.ID)
		if n != 1 {
			t.Fatal("caster-centered shape repeated hit", n)
		}
	}
}

func TestHealerIndependentShieldStrongestRecipientCombination(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10322: 1, 10320: 1}, "YS-U02", "YS-D03", "YS-A21")
	a := healerSoloAlly(t, s, p, 2, 120, 100, 1, 1000)
	b := healerSoloAlly(t, s, p, 3, 150, 100, 1, 1000)
	healerSoloTrain(b, domain.Learned{})
	incomingEquipPower(t, s, b, "TY-D13", domain.SlotBody)
	if len(domain.ActiveLegendaryPowers(s.functionalWorn(b), s.itemDef, b.Player.Char)) != 1 {
		t.Fatal("recipient shield power inactive")
	}
	nativeRepairCast(t, s, p, 10322, p.ID)
	nativeRepairCast(t, s, p, 10320, p.ID)
	weak, strong := universalShieldValue(a), universalShieldValue(b)
	if strong <= weak || 100000-m.HP != strong*80/100 || universalShieldValue(p) != weak {
		t.Fatal("overlap did not select strongest newly committed recipient shield", weak, strong, 100000-m.HP)
	}
	_, count := healerSoloCount(s, m.ID)
	if count != 1 {
		t.Fatal("unequal overlapping bursts stacked", count)
	}
}
func TestHealerIndependentA06PerTargetBlindImmunity(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A06", domain.Learned{10317: 1}, true)
	side := healerSoloMonster(s, 901, 140, 100)
	side.Status.Apply(domain.StatusDef{ID: 199987, DurationSec: 30, BlockHarmful: true}, s.tick, side.ID)
	nativeRepairCast(t, s, p, 10317, m.ID)
	if !m.Status.HasEffective(1008) || side.Status.HasEffective(1008) || 100000-m.HP != 100000-side.HP || m.HP >= 100000 {
		t.Fatal("blind immunity affected another victim or blocked direct damage")
	}
}

func TestHealerIndependentNativeReactivatedControl(t *testing.T) {
	for _, mode := range []string{"removed", "expired"} {
		t.Run(mode, func(t *testing.T) {
			s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10304: 1}, true)
			wall := time.Unix(300000, 0)
			s.now = func() time.Time { return wall }
			healerSoloResourceStart(s, p)
			p.HP = 1
			s.applyStatusDef(p, domain.StatusDef{ID: 199986, DurationSec: 1, SuppressHarmful: true}, p.ID)
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
			if len(s.casts) != 1 {
				t.Fatal("missing actual windup")
			}
			cast := s.casts[0]
			d, _ := s.statuses.Get(domain.StatusStun, 1)
			if !s.applyStatusDef(p, d, m.ID) || !canCast(p) || cast.windup.interrupted {
				t.Fatal("suppressed native control must initially permit windup")
			}
			if mode == "removed" {
				s.statusesRemoved(p, p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == 199986 }))
			} else {
				s.tick += domain.Ticks(1000)
				wall = wall.Add(time.Second)
				s.stepStatus()
			}
			if canCast(p) || !p.Status.HasEffective(domain.StatusStun) {
				t.Fatal("native control did not reactivate")
			}
			before := p.HP
			wall = cast.releaseAtWall.Add(time.Second)
			s.stepCastsAt(wall, false)
			s.stepCastsAt(wall.Add(time.Second), false)
			if p.HP != before {
				t.Fatal("reactivated effective native control allowed queued heal", mode, before, p.HP)
			}
		})
	}
}

func TestHealerIndependentU01NativeTickCount(t *testing.T) {
	var baseTotal int32
	for _, on := range []bool{false, true} {
		s, p, _ := healerSoloFixture(t, "YS-U01", domain.Learned{10303: 1}, on)
		healerSoloResourceStart(s, p)
		p.HP = 1
		nativeRepairCast(t, s, p, 10303, p.ID)
		if on {
			nativeRepairCast(t, s, p, 10303, p.ID)
		}
		s.outbox = nil
		for i := 0; i < 121; i++ {
			s.tick += domain.Ticks(1000)
			s.stepStatus()
		}
		count := 0
		total := int32(0)
		for _, out := range s.outbox {
			if h, ok := out.ev.(event.HealDone); ok && h.Dst == p.ID && h.SkillID == 10303 {
				count++
				total += h.Amount
			}
		}
		if count != 120 {
			t.Fatal("native HOT tick count changed or recast stacked ticks", on, count)
		}
		if !on {
			baseTotal = total
		} else {
			healerSoloNear(t, total, baseTotal, 150)
		}
	}
}
func TestHealerIndependentU08RangeLimit(t *testing.T) {
	s, p, _ := healerSoloFixture(t, "YS-U08", domain.Learned{10301: 1}, true)
	a := healerSoloAlly(t, s, p, 2, 751, 100, 1, 1000)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10301, Target: a.ID})
	if len(s.casts) != 0 || p.MP != 10000 || a.HP != 1 {
		t.Fatal("friendly single spell exceeded native range plus200")
	}
}

func TestHealerIndependentNativeReactivatedControlExpiresBeforeRelease(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10304: 1}, true)
	wall := time.Unix(400000, 0)
	s.now = func() time.Time { return wall }
	healerSoloResourceStart(s, p)
	p.HP = 1
	s.applyStatusDef(p, domain.StatusDef{ID: 199985, DurationSec: 1, SuppressHarmful: true}, p.ID)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
	cast := s.casts[0]
	d, _ := s.statuses.Get(domain.StatusStun, 27)
	if !s.applyStatusDef(p, d, m.ID) || !canCast(p) {
		t.Fatal("short native stun should initially be suppressed")
	}
	s.tick += domain.Ticks(1000)
	s.stepStatus()
	if canCast(p) || !cast.windup.interrupted {
		t.Fatal("reactivated short control did not cancel immediately")
	}
	s.tick += domain.Ticks(2000)
	s.stepStatus()
	if !canCast(p) {
		t.Fatal("native short stun did not expire before original release")
	}
	before := p.HP
	wall = cast.releaseAtWall.Add(time.Second)
	s.stepCastsAt(wall, false)
	s.stepCastsAt(wall.Add(time.Second), false)
	if p.HP != before || len(s.casts) != 0 {
		t.Fatal("control no longer effective but old windup resurrected")
	}
}

func TestHealerIndependentD05InstantHealRetainsRejuvenationGuard(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-D05", domain.Learned{10303: 1, 10301: 1}, true)
	nativeRepairCast(t, s, p, 10303, p.ID)
	p.HP, p.MaxHP = 10000, 10000
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10301, Target: p.ID})
	if len(s.casts) != 1 || !s.casts[0].releaseAtWall.IsZero() || s.casts[0].windup != nil {
		t.Fatal("native instant Heal unexpectedly acquired a windup")
	}
	if got := healerSoloIncoming(s, p, m, false, true); got != 750 {
		t.Fatal("rejuvenation guard missing during instant Heal", got)
	}
	s.stepCastsAt(s.now().Add(time.Second), false)
	s.stepCastsAt(s.now().Add(2*time.Second), false)
	p.HP = 10000
	if got := healerSoloIncoming(s, p, m, false, true); got != 750 {
		t.Fatal("rejuvenation guard missing after instant Heal", got)
	}
}

func TestHealerIndependentNativeControlIgnoresMonsterAndNoWindup(t *testing.T) {
	t.Run("monster_AI_uses_its_own_timer", func(t *testing.T) {
		s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10304: 1}, true)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10304, Target: p.ID})
		cast := s.casts[0]
		m.Monster.SkillReady = map[domain.SkillID]domain.Tick{}
		m.Monster.SkillCasts = map[domain.SkillID]int32{}
		rule := domain.MonsterSkillRule{Skill: domain.SkillDef{ID: 900999, Level: 1, Kind: domain.SkillSingle, PrepareMS: 2000, Dist: 500, TargetEnemy: true, HurtType: domain.HurtHoly, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}}
		s.startMonsterSkill(m, rule, []domain.EntityID{p.ID})
		serial, skill := m.Monster.CastSerial, m.Monster.CastingSkill
		d, _ := s.statuses.Get(domain.StatusStun, 1)
		if !s.applyStatusDef(m, d, p.ID) {
			t.Fatal("monster control did not commit")
		}
		if cast.windup.interrupted || m.Monster.CastSerial != serial || m.Monster.CastingSkill != skill {
			t.Fatal("player windup helper mutated independent monster timer or another player's cast")
		}
	})
	t.Run("accepted_instant_payload_has_no_windup", func(t *testing.T) {
		s, p, m := healerSoloFixture(t, "YS-D06", domain.Learned{10319: 1}, true)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10319, Target: m.ID})
		if len(s.casts) != 1 || s.casts[0].windup != nil {
			t.Fatal("expected accepted native instant payload")
		}
		d, _ := s.statuses.Get(domain.StatusStun, 1)
		if !s.applyStatusDef(p, d, m.ID) {
			t.Fatal("native control rejected")
		}
		s.stepCastsAt(s.now().Add(time.Second), false)
		s.stepCastsAt(s.now().Add(2*time.Second), false)
		if m.HP >= 100000 || canCast(p) {
			t.Fatal("windup-only cancellation revoked accepted instant payload or removed control")
		}
	})
}

// A23 is the named 治愈术 producer, not a generic heal-to-damage passive.
// 回春术 keeps its periodic healing identity; assigning every tick a synthetic
// primary would incorrectly turn that different skill into repeated light bolts.
func TestHealerA23NativePeriodicIdentityAndDelayedDirectHeal(t *testing.T) {
	s, p, m := healerSoloFixture(t, "YS-A23", domain.Learned{10301: 1, 10303: 1}, true)
	for key, def := range s.skills {
		if key.ID != 10301 {
			continue
		}
		if len(def.Statuses) != 0 {
			t.Fatalf("治愈术 rank %d unexpectedly gained a status producer", key.Level)
		}
		for _, effect := range def.DirectEffects() {
			if effect.Kind != domain.EffectHeal {
				t.Fatalf("治愈术 rank %d is no longer a direct-only heal: %+v", key.Level, effect)
			}
		}
	}
	p.HP = 1
	nativeRepairCast(t, s, p, 10303, p.ID)
	a := healerSoloStatus(t, p, 10303)
	before := p.HP
	s.tickStatus(p, a)
	if p.HP <= before || m.HP != 100000 {
		t.Fatal("回春术 must heal without impersonating 治愈术 or generating its light bolts", p.HP, before, m.HP)
	}
	nativeRepairCast(t, s, p, 10301, p.ID)
	if m.HP >= 100000 {
		t.Fatal("the actual delayed 治愈术 primary heal lost A23")
	}
}
