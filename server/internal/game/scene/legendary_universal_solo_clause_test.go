package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
	"math/rand"
	"testing"
)

// Controlled supported geometry inputs, not claims of authored PvE cone bases.
func TestLegendaryUniversalSoloConeConsumerBoundary(t *testing.T) {
	for _, tc := range []struct{ base, final int32 }{{43, 60}, {86, 120}, {120, 168}, {129, 180}, {172, 240}, {300, 360}} {
		for _, dir := range []uint8{0, 2, 4, 6} {
			t.Run(fmt.Sprintf("TY-A04/angle=%d/dir=%d", tc.final, dir), func(t *testing.T) {
				s, p, _ := universalSoloFixture(t, "TY-A04", domain.Warrior, domain.Learned{10006: 1})
				p.Dir = dir
				d, _ := s.skills.Get(10006, 1)
				d.Area.Angle = tc.base
				d.FormulaVersion = 0
				d.Progression = nil
				s.skills[domain.SkillKey{ID: 10006, Level: 1}] = d
				type target struct {
					id  domain.EntityID
					hit bool
				}
				var targets []target
				for i, v := range []struct {
					a, r float64
					hit  bool
				}{{0, 0, true}, {0, 149, true}, {0, 150, true}, {0, 151, false}, {float64(tc.final)/2 - 1, 100, true}, {float64(tc.final) / 2, 100, true}, {-float64(tc.final) / 2, 100, true}, {float64(tc.final)/2 + 1, 100, tc.final == 360}, {90, 100, tc.final >= 180}, {180, 100, tc.final == 360}} {
					a := (v.a + float64(dir)*45) * math.Pi / 180
					dx, dy := v.r*math.Cos(a), v.r*math.Sin(a)
					if math.Abs(dx) < 1e-10 {
						dx = 0
					}
					if math.Abs(dy) < 1e-10 {
						dy = 0
					}
					e := putMonster(s, domain.EntityID(910+i), 10000, domain.Stats{}, p.Pos.X+dx, p.Pos.Y+dy)
					targets = append(targets, target{e.ID, v.hit})
				}
				assassinRegressionCast(t, s, p, UseSkill{Skill: 10006})
				for _, v := range targets {
					want := int32(0)
					n := 0
					if v.hit {
						want = 130
						n = 1
					}
					universalSoloDamage(t, s, v.id, 10006, want, n)
					if s.entities[v.id].HP != 10000-want {
						t.Fatal("HP boundary")
					}
				}
			})
		}
	}
	t.Run("TY-A04/zero_aim_no_targets", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A04", domain.Warrior, domain.Learned{10006: 1})
		at := p.Pos
		before := p.MP
		d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10006, At: &at})
		if len(s.casts) != 0 || before-p.MP != d.MPCost || m.HP != m.MaxHP {
			t.Fatal("zero aim hit a target or changed payment")
		}
	})
}
func TestLegendaryUniversalSoloClauseInvariants(t *testing.T) {
	for _, id := range []string{"TY-A09", "TY-A37"} {
		t.Run(id+"/native_crit_and_poison", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Assassin, domain.Learned{})
			p.Stats.CritRate = 10000
			p.Look.AtkDist = 100
			p.Status.Apply(domain.StatusDef{ID: 1018, SourceSkillID: 10211, DurationSec: 120, PoisonOnHitChance: 100, PoisonOnHitLevel: 21, SourceEffectHitKnown: true}, s.tick, p.ID)
			universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
			want := int32(200)
			if id == "TY-A37" {
				want = 350
			}
			universalSoloDamage(t, s, m.ID, 0, want, 1)
			if !m.Status.Has(domain.StatusPoison) {
				t.Fatal("native poison lost")
			}
		})
	}
	for _, tc := range []struct {
		id     string
		status domain.StatusID
	}{{"TY-A30", 1034}, {"TY-A31", 1001}, {"TY-A32", 1036}} {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/state_unchanged=%v", tc.id, active), func(t *testing.T) {
				s, p, m := universalSoloFixture(t, tc.id, domain.Warrior, domain.Learned{})
				p.Look.AtkDist = 100
				if active {
					m.Status.Apply(domain.StatusDef{ID: tc.status, SourceSkillID: 10002, DurationSec: 10, BlockMove: tc.status == 1034, TypeKnown: true, Type: domain.StatusBad}, s.tick, p.ID)
				}
				s.tick += domain.Ticks(1000)
				var before domain.Active
				if active {
					before = universalSoloActive(t, m, tc.status)
				}
				universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
				want := int32(100)
				if active {
					want = 140
					if tc.id == "TY-A30" {
						want = 150
					}
				}
				universalSoloDamage(t, s, m.ID, 0, want, 1)
				n := 0
				m.Status.EachEffective(func(a domain.Active) {
					if a.Def.ID == tc.status {
						n++
						if a.ExpireAt != before.ExpireAt || a.Source != before.Source {
							t.Fatal("state refreshed or copied")
						}
					}
				})
				expected := 0
				if active {
					expected = 1
				}
				if n != expected {
					t.Fatal("state count", n)
				}
			})
		}
	}
	t.Run("TY-A08/nearby_enemy_not_required_empty", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A08", domain.Warrior, domain.Learned{10002: 1})
		side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		universalSoloDamage(t, s, m.ID, 10002, 208, 1)
		if side.HP != 10000 {
			t.Fatal("single became area")
		}
	})
	t.Run("TY-A05/native_ground_trap_unchanged", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A05", domain.Assassin, domain.Learned{10213: 1})
		outside := putMonster(s, 901, 10000, domain.Stats{}, m.Pos.X+110, m.Pos.Y)
		s.monsters[outside.ID] = outside
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
		if len(s.traps) != 1 {
			t.Fatal("ground trap count")
		}
		s.stepTraps()
		universalSoloDamage(t, s, m.ID, 10213, 200, 1)
		if outside.HP != 10000 {
			t.Fatal("ground converted")
		}
	})
	t.Run("TY-A23/poison_resistance", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A23", domain.Assassin, domain.Learned{})
		p.Status.Apply(domain.StatusDef{ID: 1018, SourceSkillID: 10211, DurationSec: 60, PoisonOnHitChance: 100, PoisonOnHitLevel: 21, SourceEffectHitKnown: true}, s.tick, p.ID)
		m.Stats.StatusResist = 100
		if s.applyCombatTriggeredStatus(p, m, domain.StatusPoison) || m.Status.Has(domain.StatusPoison) {
			t.Fatal("resistance bypassed")
		}
	})
	t.Run("TY-D09/minimum_control", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-D09", domain.Warrior, domain.Learned{})
		d := domain.StatusDef{ID: 1034, SourceSkillID: 10326, LegendaryDurationMS: 100, BlockMove: true, TypeKnown: true, Type: domain.StatusBad}
		if !s.applyStatusDef(p, d, m.ID) || canMove(p) {
			t.Fatal("control became immune")
		}
		a := universalSoloActive(t, p, d.ID)
		if a.ExpireAt-s.tick != domain.Ticks(100) {
			t.Fatal("minimum duration", a.ExpireAt-s.tick)
		}
		s.tick = a.ExpireAt
		s.stepStatus()
		if !canMove(p) {
			t.Fatal("minimum expiry")
		}
	})
}

// Controlled existing reduction buckets validate additive payment at the actual
// admission boundary; native carrier costs and statuses remain authored.
func TestLegendaryUniversalSoloClausePaymentAndHealing(t *testing.T) {
	for _, tc := range []struct {
		id    string
		race  domain.Race
		skill domain.SkillID
		self  bool
		pct   int32
	}{{"TY-U03", domain.Warrior, 10002, false, 50}, {"TY-U04", domain.Healer, 10301, true, 50}, {"TY-U12", domain.Warrior, 10016, false, 50}, {"TY-U13", domain.Healer, 10320, true, 60}, {"TY-U14", domain.Warlock, 10421, false, 50}} {
		t.Run(tc.id+"/additive_payment", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, tc.race, domain.Learned{tc.skill: 1})
			s.refreshStats(p)
			p.MP = p.MaxMP
			p.HP = p.MaxHP / 2
			d, _ := s.skills.Get(tc.skill, 1)
			nativeCost := d.MPCost
			d.LegendaryModifiers.ManaReductionBP = 2000
			s.skills[domain.SkillKey{ID: tc.skill, Level: 1}] = d
			target := m.ID
			if tc.self {
				target = p.ID
			}
			before := p.MP
			assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: target})
			if before-p.MP != (nativeCost*tc.pct+99)/100 {
				t.Fatal("payment did not add once", before-p.MP, nativeCost)
			}
		})
	}
	t.Run("TY-U10/controlled_cone_heal", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U10", domain.Healer, domain.Learned{10304: 1})
		d, _ := s.skills.Get(10304, 1)
		d.Area.Shape = domain.SkillAreaCone
		d.Area.Angle = 120
		s.skills[domain.SkillKey{ID: 10304, Level: 1}] = d
		a := nativeRepairAlly(t, s, p, 2, 500)
		b := nativeRepairAlly(t, s, p, 3, 521)
		c := nativeRepairAlly(t, s, p, 4, 100)
		s.aoi.Move(c, domain.Pos{MapID: p.Pos.MapID, X: 100, Y: 500})
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10304})
		if a.HP != 231 || b.HP != 1 || c.HP != 1 {
			t.Fatal("healing cone radius/angle", a.HP, b.HP, c.HP)
		}
	})
	t.Run("TY-U07/native_knockback_unchanged", func(t *testing.T) {
		var base float64
		for _, enhanced := range []bool{false, true} {
			var powers []string
			if enhanced {
				powers = []string{"TY-U07"}
			}
			s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{10102: 1}, powers...)
			before := m.Pos.X
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10102, Target: m.ID})
			delta := m.Pos.X - before
			if !enhanced {
				base = delta
				if base <= 0 {
					t.Fatal("native knockback absent")
				}
			} else if delta != base {
				t.Fatal("teleport bonus widened knockback", delta, base)
			}
		}
	})
}

func TestLegendaryUniversalSoloCriticalCounterfactual(t *testing.T) {
	t.Run("TY-A11", func(t *testing.T) {
		seed := int64(0)
		for ; seed < 10000; seed++ {
			r := rand.New(rand.NewSource(seed))
			r.Intn(100000)
			if r.Intn(10000) >= 8000 {
				break
			}
		}
		for _, enhanced := range []bool{false, true} {
			t.Run(fmt.Sprint(enhanced), func(t *testing.T) {
				var powers []string
				if enhanced {
					powers = []string{"TY-A11"}
				}
				s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, powers...)
				p.Stats.CritRate = 8000
				p.Stats.Hit = 100000
				m.Stats.Def = 0
				s.rng = rand.New(rand.NewSource(seed))
				end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
				ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10002, SkillPct: 100, GuaranteedHit: true})
				end()
				want := int32(100)
				if enhanced {
					want = 200
				}
				if ev.Amount != want || m.MaxHP-m.HP != want {
					t.Fatalf("same-seed counterfactual enhanced=%v damage=%d want=%d seed=%d", enhanced, ev.Amount, want, seed)
				}
			})
		}
	})
}

func TestLegendaryUniversalSoloAreaSingleTargetCount(t *testing.T) {
	for _, id := range []string{"TY-A34", "TY-A36"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{10022: 1})
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10022, Target: m.ID})
			want := int32(155)
			if id == "TY-A34" {
				want = 279
			}
			universalSoloDamage(t, s, m.ID, 10022, want, 1)
		})
	}
}

func TestLegendaryUniversalSoloCriticalExactProbabilityBoundary(t *testing.T) {
	for _, draw := range []int{1999, 2000} {
		t.Run(fmt.Sprintf("TY-A11/draw=%d", draw), func(t *testing.T) {
			seed := int64(0)
			for {
				r := rand.New(rand.NewSource(seed))
				r.Intn(100000)
				if r.Intn(10000) == draw {
					break
				}
				seed++
			}
			for _, enabled := range []bool{false, true} {
				var powers []string
				if enabled {
					powers = []string{"TY-A11"}
				}
				s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, powers...)
				p.Stats.CritRate = 0
				p.Stats.Hit = 100000
				m.Stats.Def = 0
				s.rng = rand.New(rand.NewSource(seed))
				end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
				ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10002, SkillPct: 100, GuaranteedHit: true})
				end()
				want := int32(100)
				if enabled && draw == 1999 {
					want = 200
				}
				if ev.Amount != want || m.MaxHP-m.HP != want || ev.Flag.Has(event.DamageCrit) != (want == 200) {
					t.Fatalf("exact20pp boundary enabled=%v draw=%d seed=%d event=%+v HP=%d want=%d", enabled, draw, seed, ev, m.MaxHP-m.HP, want)
				}
			}
		})
	}
}
