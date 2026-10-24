package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"math"
	"math/rand"
	"testing"
	"time"
)

// Every fixture below equips either the ONE named power or no power. Native
// skills retain their authored costs, cast time, geometry, effects and sources.
// Multi-power clauses are deliberately exercised in the separate combination suite.
func casterIndependentFixture(t *testing.T, id string, learned domain.Learned, enabled bool) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	powers := []string{}
	if enabled {
		powers = append(powers, id)
	}
	race := domain.Warlock
	if id[:2] == "YS" {
		race = domain.Healer
	}
	s, p, m := nativeRepairFixture(t, race, learned, powers...)
	s.rng = rand.New(rand.NewSource(1))
	return s, p, m
}
func casterIndependentNear(t *testing.T, got, base, pct int32) {
	t.Helper()
	want := base * pct / 100
	if got < want-2 || got > want+2 {
		t.Fatalf("actual HP damage %d; baseline %d × %d%% = %d", got, base, pct, want)
	}
}
func casterIndependentBurn(s *Scene, p, m *entity.Entity, source domain.EntityID) domain.Active {
	if m.Status == nil {
		m.Status = domain.NewStatusSet()
	}
	d := domain.StatusDef{ID: 1036, Level: 1, DurationSec: 10, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, SourceSkillID: 10421, SkillSnapshotID: 10421, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}
	m.Status.Apply(d, s.tick, source)
	var a domain.Active
	m.Status.EachEffective(func(x domain.Active) {
		if x.Def.ID == 1036 {
			a = x
		}
	})
	return a
}
func casterIndependentDamageCount(s *Scene, id domain.EntityID) (int32, int) {
	var n int32
	count := 0
	for _, out := range s.outbox {
		if e, ok := out.ev.(event.DamageDealt); ok && e.Dst == id {
			n += e.Amount
			count++
		}
	}
	return n, count
}

func TestCasterIndependentDirectDamage(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		pct   int32
	}{
		{"SS-A01", 10402, 170}, {"SS-A06", 10402, 120}, {"SS-A08", 10409, 120}, {"SS-A12", 10410, 120}, {"SS-A13", 10410, 185}, {"SS-A14", 10410, 120}, {"SS-A18", 10405, 140}, {"SS-A20", 10412, 155}, {"SS-A21", 10412, 180},
	} {
		t.Run(tc.id, func(t *testing.T) {
			run := func(on bool) int32 {
				s, p, m := casterIndependentFixture(t, tc.id, domain.Learned{tc.skill: 1}, on)
				nativeRepairCast(t, s, p, tc.skill, m.ID)
				damage, count := casterIndependentDamageCount(s, m.ID)
				if count != 1 {
					t.Fatalf("direct cast settled %d attacks", count)
				}
				return damage
			}
			casterIndependentNear(t, run(true), run(false), tc.pct)
		})
	}
}
func TestCasterIndependentA01BurnOwnership(t *testing.T) {
	for _, source := range []domain.EntityID{0, 1, 99} {
		t.Run(fmt.Sprint(source), func(t *testing.T) {
			run := func(on bool) int32 {
				s, p, m := casterIndependentFixture(t, "SS-A01", domain.Learned{10402: 1}, on)
				if source > 0 {
					casterIndependentBurn(s, p, m, source)
				}
				nativeRepairCast(t, s, p, 10402, m.ID)
				return 100000 - m.HP
			}
			pct := int32(170)
			if source == 1 {
				pct = 210
			}
			casterIndependentNear(t, run(true), run(false), pct)
		})
	}
}
func TestCasterIndependentBurnSettlement(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		pct   int32
	}{{"SS-A02", 10401, 80}, {"SS-A11", 10409, 100}, {"SS-A16", 10405, 160}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, source := range []domain.EntityID{0, 1, 99} {
				t.Run(fmt.Sprint(source), func(t *testing.T) {
					run := func(on bool) (int32, int) {
						s, p, m := casterIndependentFixture(t, tc.id, domain.Learned{tc.skill: 1}, on)
						var before domain.Active
						if source > 0 {
							before = casterIndependentBurn(s, p, m, source)
						}
						nativeRepairCast(t, s, p, tc.skill, m.ID)
						if source > 0 {
							a, ok := legendaryCasterSourceStatus(m, 1036, source)
							if !ok || a.ExpireAt != before.ExpireAt || a.Def.HPChanges[0].Value != -10 {
								t.Fatal("settlement consumed/shortened/refreshed burn")
							}
						}
						return casterIndependentDamageCount(s, m.ID)
					}
					base, _ := run(false)
					got, count := run(true)
					want := base
					if source == 1 {
						want += tc.pct
						if count != 2 {
							t.Fatal("settlement recurred or absent", count)
						}
					} else if count != 1 {
						t.Fatal("foreign/missing burn settled", count)
					}
					if got != want {
						t.Fatal("remaining burn damage", got, want)
					}
				})
			}
		})
	}
}
func TestCasterIndependentA03SplashBatch(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(fmt.Sprint(on), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A03", domain.Learned{10401: 1}, on)
			var extra []*entity.Entity
			for i := 0; i < 6; i++ {
				extra = append(extra, putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, m.Pos.X+float64(10+i*25), 100))
			}
			outside := putMonster(s, 910, 100000, domain.Stats{}, m.Pos.X+170, 100)
			nativeRepairCast(t, s, p, 10401, m.ID)
			d, n := casterIndependentDamageCount(s, m.ID)
			if d <= 0 || n != 1 {
				t.Fatal("primary full hit missing")
			}
			count := 0
			for _, e := range extra {
				v, hits := casterIndependentDamageCount(s, e.ID)
				if v > 0 {
					count++
					if v != d || hits != 1 {
						t.Fatal("side not full single hit", v, d, hits)
					}
				}
			}
			want := 0
			if on {
				want = 4
			}
			if count != want || outside.HP != 100000 {
				t.Fatal("batch cap/radius/recursion", count, want, outside.HP)
			}
		})
	}
}
func TestCasterIndependentA04Echo(t *testing.T) {
	run := func(on bool) (int32, int32, int, int) {
		s, p, m := casterIndependentFixture(t, "SS-A04", domain.Learned{10402: 1}, on)
		side := putMonster(s, 901, 100000, domain.Stats{}, m.Pos.X+90, 100)
		outside := putMonster(s, 902, 100000, domain.Stats{}, m.Pos.X+101, 100)
		nativeRepairCast(t, s, p, 10402, m.ID)
		if outside.HP != 100000 {
			t.Fatal("echo radius escaped")
		}
		a, ac := casterIndependentDamageCount(s, m.ID)
		b, bc := casterIndependentDamageCount(s, side.ID)
		if m.Status.HasEffective(1036) || side.Status.HasEffective(1036) {
			t.Fatal("status invented")
		}
		return a, b, ac, bc
	}
	a, b, _, _ := run(false)
	x, y, xc, yc := run(true)
	casterIndependentNear(t, x-a, a, 70)
	casterIndependentNear(t, y-b, b, 70)
	if xc != 2 || yc != 2 {
		t.Fatal("echo recursively repeated", xc, yc)
	}
}
func TestCasterIndependentA05HitCounts(t *testing.T) {
	for _, count := range []int{1, 2, 3, 5} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			run := func(on bool) (int32, []int32) {
				s, p, m := casterIndependentFixture(t, "SS-A05", domain.Learned{10402: 1}, on)
				var side []*entity.Entity
				for i := 1; i < count; i++ {
					side = append(side, putMonster(s, domain.EntityID(900+i), 100000, domain.Stats{}, 130+float64(i*10), 100))
				}
				nativeRepairCast(t, s, p, 10402, m.ID)
				ds := []int32{}
				for _, e := range side {
					ds = append(ds, 100000-e.HP)
				}
				return 100000 - m.HP, ds
			}
			base, bs := run(false)
			got, gs := run(true)
			pct := int32(175)
			if count >= 3 {
				pct = 135
			}
			casterIndependentNear(t, got, base, pct)
			for i := range bs {
				pct = 175
				if count >= 3 {
					pct = 100
				}
				casterIndependentNear(t, gs[i], bs[i], pct)
			}
		})
	}
}
func TestCasterIndependentAreaConsumers(t *testing.T) {
	for _, tc := range []struct {
		id        string
		skill     domain.SkillID
		radius    float64
		near, far int32
	}{{"SS-A06", 10402, 165, 120, 145}, {"SS-A13", 10410, 100, 185, 165}, {"SS-A18", 10405, 270, 140, 140}, {"SS-A20", 10412, 210, 155, 155}, {"SS-A21", 10412, 150, 180, 120}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, tc.id, domain.Learned{tc.skill: 1}, true)
			center := m.Pos
			if tc.skill == 10410 {
				center = p.Pos
				s.aoi.Move(m, domain.Pos{MapID: center.MapID, X: center.X + 20, Y: center.Y})
			}
			edge := putMonster(s, 901, 100000, domain.Stats{}, center.X+tc.radius-1, center.Y)
			outside := putMonster(s, 902, 100000, domain.Stats{}, center.X+tc.radius+1, center.Y)
			nativeRepairCast(t, s, p, tc.skill, m.ID)
			baseS, baseP, baseM := casterIndependentFixture(t, tc.id, domain.Learned{tc.skill: 1}, false)
			nativeRepairCast(t, baseS, baseP, tc.skill, baseM.ID)
			base := 100000 - baseM.HP
			casterIndependentNear(t, 100000-m.HP, base, tc.near)
			casterIndependentNear(t, 100000-edge.HP, base, tc.far)
			if outside.HP != 100000 {
				t.Fatal("outside radius hit")
			}
			if tc.id == "SS-A20" {
				d, _ := s.skills.Get(tc.skill, 1)
				if p.MP != p.MaxMP-d.MPCost || !p.Player.SkillReadyAt(tc.skill, s.tick+domain.Ticks(int(d.CooldownMS*80/100))) || p.Player.SkillReadyAt(tc.skill, s.tick+domain.Ticks(int(d.CooldownMS*80/100))-1) {
					t.Fatal("actual cost/cooldown did not follow A20", p.MP, d.MPCost)
				}
			}
		})
	}
}
func TestCasterIndependentA08RangeThreshold(t *testing.T) {
	for _, distance := range []float64{224, 225, 300} {
		t.Run(fmt.Sprint(distance), func(t *testing.T) {
			run := func(on bool) int32 {
				s, p, m := casterIndependentFixture(t, "SS-A08", domain.Learned{10409: 1}, on)
				s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + distance, Y: p.Pos.Y})
				nativeRepairCast(t, s, p, 10409, m.ID)
				return 100000 - m.HP
			}
			pct := int32(120)
			if distance >= 225 {
				pct = 155
			}
			casterIndependentNear(t, run(true), run(false), pct)
		})
	}
}
func TestCasterIndependentControlledDamage(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		pct   int32
	}{{"SS-A09", 10409, 200}, {"SS-A15", 10410, 160}, {"SS-A22", 10412, 190}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, controlled := range []bool{false, true} {
				t.Run(fmt.Sprint(controlled), func(t *testing.T) {
					run := func(on bool) int32 {
						learned := domain.Learned{tc.skill: 1}
						if tc.id == "SS-A09" {
							learned[10427] = 1
						}
						s, p, m := casterIndependentFixture(t, tc.id, learned, on)
						if controlled {
							m.Status.Apply(domain.StatusDef{ID: 199990, DurationSec: 60, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -20, Mode: domain.ModePercent}}}, s.tick, 99)
						}
						nativeRepairCast(t, s, p, tc.skill, m.ID)
						if controlled && !m.Status.HasEffective(199990) {
							t.Fatal("old control consumed")
						}
						return 100000 - m.HP
					}
					pct := int32(100)
					if controlled {
						pct = tc.pct
					}
					casterIndependentNear(t, run(true), run(false), pct)
				})
			}
		})
	}
}
func TestCasterIndependentA12Centers(t *testing.T) {
	for _, center := range []string{"self", "remote", "out_of_range"} {
		t.Run(center, func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A12", domain.Learned{10410: 1}, true)
			x := float64(400)
			if center == "self" {
				x = 130
			}
			if center == "out_of_range" {
				x = 551
			}
			s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: x, Y: 100})
			side := putMonster(s, 901, 100000, domain.Stats{}, x+99, 100)
			outside := putMonster(s, 902, 100000, domain.Stats{}, x+101, 100)
			target := m.ID
			if center == "self" {
				target = 0
			}
			if center == "out_of_range" {
				before := p.MP
				s.onUseSkill(UseSkill{ID: p.ID, Skill: 10410, Target: m.ID})
				if len(s.casts) != 0 || p.MP != before || m.HP != 100000 {
					t.Fatal("invalid center accepted")
				}
				return
			}
			nativeRepairCast(t, s, p, 10410, target)
			if m.HP == 100000 {
				t.Fatal("center missed without prior freeze")
			}
			if center == "remote" && (side.HP == 100000 || outside.HP != 100000) {
				t.Fatal("remote center/radius lost")
			}
		})
	}
}

func TestCasterIndependentA07HighestBurnSpread(t *testing.T) {
	for _, own := range []bool{false, true} {
		t.Run(fmt.Sprint(own), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A07", domain.Learned{10402: 1}, true)
			source := domain.EntityID(99)
			if own {
				source = p.ID
			}
			casterIndependentBurn(s, p, m, source)
			var sides []*entity.Entity
			for i := 0; i < 6; i++ {
				sides = append(sides, putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, 140+float64(i*8), 100))
			}
			strong := sides[0]
			strong.Status = domain.NewStatusSet()
			burn := casterIndependentBurn(s, p, strong, source)
			strong.Status.Remove(1036)
			burn.Def.HPChanges[0].Value = -20
			strong.Status.Apply(burn.Def, s.tick, source)
			outside := putMonster(s, 920, 100000, domain.Stats{}, 231, 100)
			nativeRepairCast(t, s, p, 10402, m.ID)
			base, baseCount := casterIndependentDamageCount(s, strong.ID)
			if baseCount != 1 {
				t.Fatal("highest-burn source received its own spread", baseCount)
			}
			spread := 0
			for _, e := range append([]*entity.Entity{m}, sides[1:]...) {
				d, c := casterIndependentDamageCount(s, e.ID)
				if c == 2 {
					spread++
					if d != base+180 {
						t.Fatal("90% highest remainder must hit each, not divide", d, base)
					}
				} else if d != base || c != 1 {
					t.Fatal("unexpected spread recursion", d, c)
				}
			}
			want := 0
			if own {
				want = 4
			}
			if spread != want || outside.HP != 100000 || !m.Status.HasEffective(1036) || !strong.Status.HasEffective(1036) {
				t.Fatal("spread source/count/radius/preservation", spread, want)
			}
		})
	}
}
func TestCasterIndependentGuaranteedStatusLearning(t *testing.T) {
	for _, tc := range []struct {
		id             string
		skill, passive domain.SkillID
		status         domain.StatusID
	}{{"SS-A09", 10409, 10427, 10427}, {"SS-A17", 10405, 10421, 1036}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, learned := range []bool{false, true} {
				t.Run(fmt.Sprint(learned), func(t *testing.T) {
					skills := domain.Learned{tc.skill: 1}
					if learned {
						skills[tc.passive] = 1
					}
					s, p, m := casterIndependentFixture(t, tc.id, skills, true)
					m.Stats.StatusResist = 1000000
					nativeRepairCast(t, s, p, tc.skill, m.ID)
					if m.Status.HasEffective(tc.status) != learned {
						t.Fatal("native learning gate or guaranteed status failed", learned)
					}
					if p.Player.Char.Skills[tc.passive] != skills[tc.passive] {
						t.Fatal("borrow taught skill")
					}
				})
			}
		})
	}
}
func TestCasterIndependentA17BurnTickAndDuration(t *testing.T) {
	var baseTick, boostTick int32
	var baseDuration, boostDuration domain.Tick
	for _, on := range []bool{false, true} {
		s, p, m := casterIndependentFixture(t, "SS-A17", domain.Learned{10405: 1, 10421: 1}, on)
		skill := domain.SkillID(10421)
		if on {
			skill = 10405
		}
		nativeRepairCast(t, s, p, skill, m.ID)
		a, ok := legendaryCasterSourceStatus(m, 1036, p.ID)
		if !ok {
			t.Fatal("native burn missing")
		}
		before := m.HP
		s.tickStatus(m, a)
		d := before - m.HP
		if on {
			boostTick = d
			boostDuration = a.ExpireAt - s.tick
		} else {
			baseTick = d
			baseDuration = a.ExpireAt - s.tick
		}
	}
	casterIndependentNear(t, boostTick, baseTick, 182)
	if boostDuration != baseDuration {
		t.Fatal("burn duration changed", baseDuration, boostDuration)
	}
}
func TestCasterIndependentA19Opening(t *testing.T) {
	for _, skill := range []domain.SkillID{10421, 10405} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			run := func(on bool) (int32, int64, domain.Tick) {
				s, p, m := casterIndependentFixture(t, "SS-A19", domain.Learned{skill: 1, 10421: 1, 10423: 5}, on)
				if skill == 10405 {
					s.rng = rand.New(casterIndependentZeroSource{})
				}
				nativeRepairCast(t, s, p, skill, m.ID)
				c := &legendaryContext{Scene: s, Owner: p, Target: m}
				total := legendaryCasterBurn(c, m)
				if skill == 10405 && total == 0 {
					return 0, 0, 0
				}
				a, ok := legendaryCasterSourceStatus(m, 1036, p.ID)
				if !ok {
					t.Fatal("burn not applied")
				}
				return 100000 - m.HP, total, a.ExpireAt
			}
			base, total, expiry := run(false)
			got, remaining, after := run(true)
			if total == 0 {
				t.Fatal("chosen deterministic seed did not apply native flame burn")
			}
			if remaining != total || after != expiry || got-base != int32(total*65/100) {
				t.Fatal("new burn opening/preservation", base, got, total, remaining, expiry, after)
			}
		})
	}
}
func TestCasterIndependentFreezeAreas(t *testing.T) {
	for _, tc := range []struct {
		id     string
		radius float64
	}{{"SS-A23", 150}, {"SS-D06", 100}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, tc.id, domain.Learned{10428: 1}, true)
			var sides []*entity.Entity
			for i := 0; i < 6; i++ {
				sides = append(sides, putMonster(s, domain.EntityID(901+i), 100000, domain.Stats{}, m.Pos.X+float64(5+i*10), 100))
			}
			outside := putMonster(s, 920, 100000, domain.Stats{}, m.Pos.X+tc.radius+1, 100)
			nativeRepairCast(t, s, p, 10428, m.ID)
			if !m.Status.HasEffective(domain.StatusFreeze) {
				t.Fatal("primary not frozen")
			}
			count := 0
			for _, x := range sides {
				if x.Status != nil && x.Status.HasEffective(domain.StatusFreeze) {
					count++
				}
			}
			if count != 4 || (outside.Status != nil && outside.Status.HasEffective(domain.StatusFreeze)) {
				t.Fatal("freeze area cap/center", count)
			}
			if m.HP != 100000 {
				t.Fatal("freeze invented damage")
			}
		})
	}
}
func TestCasterIndependentA24FreezeDamage(t *testing.T) {
	for _, learned := range []bool{false, true} {
		t.Run(fmt.Sprint(learned), func(t *testing.T) {
			skills := domain.Learned{10428: 1}
			if learned {
				skills[10409] = 1
			}
			s, p, m := casterIndependentFixture(t, "SS-A24", skills, true)
			nativeRepairCast(t, s, p, 10428, m.ID)
			if !m.Status.HasEffective(domain.StatusFreeze) {
				t.Fatal("added damage broke freeze")
			}
			want := int32(0)
			if learned {
				bs, bp, bm := casterIndependentFixture(t, "SS-A24", domain.Learned{10409: 1}, false)
				nativeRepairCast(t, bs, bp, 10409, bm.ID)
				want = (100000 - bm.HP) * 150 / 100
			}
			if got := 100000 - m.HP; got != want {
				t.Fatal("native donor multiplier/learning", got, want)
			}
			if p.Player.Char.Skills[10409] != skills[10409] {
				t.Fatal("donor learned implicitly")
			}
		})
	}
}
func TestCasterIndependentA25CrossElement(t *testing.T) {
	for _, tc := range []struct{ skill, passive domain.SkillID }{{10401, 10431}, {10409, 10424}} {
		t.Run(fmt.Sprint(tc.skill), func(t *testing.T) {
			for _, learned := range []bool{false, true} {
				skills := domain.Learned{tc.skill: 1}
				if learned {
					skills[tc.passive] = 1
				}
				run := func(on bool) (int32, int) {
					s, p, m := casterIndependentFixture(t, "SS-A25", skills, on)
					nativeRepairCast(t, s, p, tc.skill, m.ID)
					return casterIndependentDamageCount(s, m.ID)
				}
				base, _ := run(false)
				got, n := run(true)
				pct := int32(100)
				if learned {
					pct = 104
				}
				casterIndependentNear(t, got, base, pct)
				if n != 1 {
					t.Fatal("cross element launched extra attack")
				}
			}
		})
	}
}
func TestCasterIndependentManaAndCooldown(t *testing.T) {
	for _, id := range []string{"SS-D08", "SS-U07", "SS-U09"} {
		t.Run(id, func(t *testing.T) {
			for _, skill := range []domain.SkillID{10401, 10402, 10410, 10405, 10412, 10421} {
				t.Run(fmt.Sprint(skill), func(t *testing.T) {
					s, p, m := casterIndependentFixture(t, id, domain.Learned{skill: 1}, true)
					d, _ := s.skills.Get(skill, 1)
					pct, cd := int32(80), int32(100)
					if id == "SS-U07" {
						pct = 78
					}
					if id == "SS-U09" {
						if skill == 10401 || skill == 10421 {
							pct = 100
						} else {
							cd = 70
						}
					}
					before, max := p.MP, p.MaxMP
					nativeRepairCast(t, s, p, skill, m.ID)
					if p.MaxMP != max || before-p.MP != (d.MPCost*pct+99)/100 {
						t.Fatal("actual mana payment/max changed", before-p.MP, d.MPCost, pct)
					}
					ready := s.tick + domain.Ticks(int(d.CooldownMS*cd/100))
					if !p.Player.SkillReadyAt(skill, ready) || p.Player.SkillReadyAt(skill, ready-1) {
						t.Fatal("actual cooldown wrong", skill, d.CooldownMS, cd)
					}
					s.stepCasts()
					if p.MP != before-(d.MPCost*pct+99)/100 {
						t.Fatal("unexpected deferred refund")
					}
				})
			}
		})
	}
}
func TestCasterIndependentU10Admission(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(fmt.Sprint(on), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-U10", domain.Learned{10401: 1, 10410: 1}, on)
			s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 719, Y: 100})
			before := p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10401, Target: m.ID})
			if on {
				if len(s.casts) != 1 {
					t.Fatal("extended range rejected")
				}
				sharedTargetFinishCasts(t, s)
				if m.HP == 100000 {
					t.Fatal("extended attack missed")
				}
			} else if len(s.casts) != 0 || p.MP != before {
				t.Fatal("baseline out-of-range accepted")
			}
			m.HP = 100000
			s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 101, Y: 100})
			nativeRepairCast(t, s, p, 10410, 0)
			if m.HP != 100000 {
				t.Fatal("range extended self radius")
			}
		})
	}
}
func TestCasterIndependentU06EmptyRefund(t *testing.T) {
	for _, skill := range []domain.SkillID{10402, 10410} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			for _, hit := range []bool{false, true} {
				t.Run(fmt.Sprint(hit), func(t *testing.T) {
					s, p, m := casterIndependentFixture(t, "SS-U06", domain.Learned{skill: 1}, true)
					d, _ := s.skills.Get(skill, 1)
					before := p.MP
					target := m.ID
					if skill == 10410 {
						target = 0
					}
					cmd := UseSkill{ID: p.ID, Skill: skill, Target: target}
					if !hit {
						s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 900, Y: 900})
						cmd.Target = 0
						if skill == 10402 {
							pos := domain.Pos{MapID: p.Pos.MapID, X: 200, Y: 100}
							cmd.At = &pos
						}
					}
					s.onUseSkill(cmd)
					if len(s.casts) == 0 {
						t.Fatal("parent not admitted")
					}
					if !hit {
						s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 900, Y: 900})
					}
					sharedTargetFinishCasts(t, s)
					want := before
					if hit {
						want -= d.MPCost
					}
					if p.MP != want {
						t.Fatal("actual refund", p.MP, want)
					}
					pct := int32(20)
					if hit {
						pct = 100
					}
					ready := s.tick + domain.Ticks(int(d.CooldownMS*pct/100))
					if !p.Player.SkillReadyAt(skill, ready) || p.Player.SkillReadyAt(skill, ready-1) {
						t.Fatal("remaining cooldown not scaled", skill, hit)
					}
					s.stepCasts()
					if p.MP != want {
						t.Fatal("duplicate refund")
					}
				})
			}
		})
	}
}

func TestCasterIndependentA10IceMitigation(t *testing.T) {
	for _, kind := range []string{"ice", "fire", "shield", "generic", "immune"} {
		t.Run(kind, func(t *testing.T) {
			run := func(on bool) int32 {
				s, p, m := casterIndependentFixture(t, "SS-A10", domain.Learned{10409: 1, 10401: 1}, on)
				// Controlled strike-boundary fixture only: player elemental equipment is
				// not currently present on ordinary monsters; this is not PvP admission evidence.
				join(t, s, 2, 200, "elemental defense boundary", 130, 100)
				m = s.players[2]
				m.Player.Char.Base.VIT = 100000
				s.refreshStats(m)
				m.HP = 100000
				m.Stats = domain.Stats{}
				var defense domain.EquipmentCombatStats
				defense.Elements[domain.HurtElementIndex(domain.HurtIce)] = domain.ElementCombatStats{ResistBP: 6000}
				cacheElementV5(s, m, defense)
				skill := domain.SkillID(10409)
				if kind == "fire" {
					skill = 10401
				}
				if kind == "shield" {
					m.Status.Apply(domain.StatusDef{ID: 199987, DurationSec: 60, DamageShield: 20}, s.tick, m.ID)
				}
				if kind == "generic" {
					m.Status.Apply(domain.StatusDef{ID: 199987, DurationSec: 60, MagicDamageReductionPct: 50}, s.tick, m.ID)
				}
				if kind == "immune" {
					m.Status.Apply(domain.StatusDef{ID: 199987, DurationSec: 60, MagicDamageReductionPct: 100, DamageReductionCapPct: 100}, s.tick, m.ID)
				}
				d, _ := s.skills.Get(skill, 1)
				s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
				s.legendaryPrepareSkill(p, &d, m.ID)
				s.applySkill(p, m, d, 0, false)
				return 100000 - m.HP
			}
			base, got := run(false), run(true)
			switch kind {
			case "ice":
				if base != 40 || got != 70 {
					t.Fatal("ice 60% reduction only half ignored", base, got)
				}
			case "shield":
				if base != 20 || got != 50 {
					t.Fatal("shield bypassed", base, got)
				}
			case "generic":
				if base != 25 || got != 35 {
					t.Fatal("generic reduction bypassed", base, got)
				}
			case "immune":
				if got != 0 || base != 0 {
					t.Fatal("immunity bypassed", base, got)
				}
			case "fire":
				if got != base {
					t.Fatal("fire gained ice pierce", base, got)
				}
			}
		})
	}
}
func TestCasterIndependentA26Reflection(t *testing.T) {
	for _, amount := range []int32{100, 3000} {
		t.Run(fmt.Sprint(amount), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A26", domain.Learned{}, true)
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MAtk: amount}
			p.Status.Apply(domain.StatusDef{ID: 1011, SourceSkillID: 10417, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, p.ID)
			m.Status.Apply(domain.StatusDef{ID: 1011, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, m.ID)
			before := p.HP
			ev, raw, reflected := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10401, Magic: true, HurtType: domain.HurtFire, GuaranteedHit: true})
			s.afterStrikeStatuses(m, p, ev, raw, reflected)
			want := min(raw*120/100, p.MaxHP*18/100)
			if 100000-m.HP != want || p.HP != before {
				t.Fatal("reflection percent/cap/loop", 100000-m.HP, want, before-p.HP)
			}
			_, n := casterIndependentDamageCount(s, m.ID)
			if n != 1 {
				t.Fatal("reflection repeated", n)
			}
		})
	}
}
func TestCasterIndependentGrantedFireBasis(t *testing.T) {
	for _, id := range []string{"SS-A27", "SS-A28"} {
		t.Run(id, func(t *testing.T) {
			for _, rank := range []int32{0, 1, 3} {
				t.Run(fmt.Sprint(rank), func(t *testing.T) {
					skill := domain.SkillID(10432)
					if id == "SS-A28" {
						skill = 10433
					}
					learned := domain.Learned{10418: 1, 10432: 5, skill: 5}
					if rank > 0 {
						learned[10401] = rank
					}
					s, p, m := casterIndependentFixture(t, id, learned, true)
					side := putMonster(s, 901, 100000, domain.Stats{}, p.Pos.X+179, 100)
					outside := putMonster(s, 902, 100000, domain.Stats{}, p.Pos.X+181, 100)
					if id == "SS-A27" {
						to := domain.Pos{MapID: p.Pos.MapID, X: 400, Y: 100}
						s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, At: &to})
						if len(s.casts) == 0 {
							t.Fatal("teleport rejected")
						}
						sharedTargetFinishCasts(t, s)
						if p.Pos != to {
							t.Fatal("native rank5 teleport failed", p.Pos)
						}
						if side.HP == 100000 || outside.HP != 100000 {
							t.Fatal("departure center/radius", side.HP, outside.HP)
						}
					} else {
						nativeRepairCast(t, s, p, skill, m.ID)
						if side.HP != 100000 {
							t.Fatal("dispel invented firebolt AoE")
						}
					}
					if p.Player.Char.Skills[10401] != rank {
						t.Fatal("borrowed damage taught firebolt")
					}
					bs, bp, bm := casterIndependentFixture(t, id, domain.Learned{10401: max(int32(1), rank)}, false)
					nativeRepairCast(t, bs, bp, 10401, bm.ID)
					pct := int32(180)
					if id == "SS-A28" {
						pct = 220
					}
					casterIndependentNear(t, 100000-m.HP, 100000-bm.HP, pct)
					_, n := casterIndependentDamageCount(s, m.ID)
					if n != 1 {
						t.Fatal("grant recursively repeated", n)
					}
				})
			}
		})
	}
}
func TestCasterIndependentA28DefenseKindsAndAdmission(t *testing.T) {
	for _, n := range []int{0, 1, 3, 4} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A28", domain.Learned{10418: 1, 10432: 1, 10433: 5}, true)
			for i, id := range []domain.StatusID{1010, 1011, 1021, 1037} {
				if i < n {
					p.Status.Apply(domain.StatusDef{ID: id, DurationSec: 30}, s.tick, p.ID)
				}
			}
			nativeRepairCast(t, s, p, 10433, m.ID)
			bs, bp, bm := casterIndependentFixture(t, "SS-A28", domain.Learned{10401: 1}, false)
			nativeRepairCast(t, bs, bp, 10401, bm.ID)
			casterIndependentNear(t, 100000-m.HP, 100000-bm.HP, int32(220+50*min(n, 3)))
			s.tick += domain.Ticks(3000)
			s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 551, Y: 100})
			before, mp := m.HP, p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10433, Target: m.ID})
			if len(s.casts) != 0 || m.HP != before || p.MP != mp {
				t.Fatal("dispel range bypass")
			}
		})
	}
}
func TestCasterIndependentReadyDamage(t *testing.T) {
	for _, id := range []string{"SS-A29", "SS-D10"} {
		t.Run(id, func(t *testing.T) {
			for _, ready := range []bool{false, true} {
				t.Run(fmt.Sprint(ready), func(t *testing.T) {
					run := func(on bool) int32 {
						s, p, m := casterIndependentFixture(t, id, domain.Learned{10402: 1}, on)
						if ready {
							p.Status.Apply(domain.StatusDef{ID: 1027, DurationSec: 30, NextSkillDamagePct: 40}, s.tick, p.ID)
						}
						nativeRepairCast(t, s, p, 10402, m.ID)
						if ready && p.Status.HasEffective(1027) {
							t.Fatal("native charge not consumed")
						}
						return 100000 - m.HP
					}
					base, got := run(false), run(true)
					if !ready {
						if got != base {
							t.Fatal("inactive readiness boosted damage", base, got)
						}
					} else {
						pct := int32(166)
						if id == "SS-D10" {
							pct = 135
						}
						casterIndependentNear(t, got, base, pct)
					}
				})
			}
		})
	}
}
func TestCasterIndependentA30EchoConsumedOnce(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(fmt.Sprint(ready), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A30", domain.Learned{10402: 1}, true)
			side := putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
			if ready {
				p.Status.Apply(domain.StatusDef{ID: 1028, SourceSkillID: 10420, DurationSec: 30, NextCastTimeReductionPct: 100}, s.tick, p.ID)
			}
			nativeRepairCast(t, s, p, 10402, m.ID)
			base := 100000 - side.HP
			want := base
			if ready {
				want += base * 80 / 100
			}
			if got := 100000 - m.HP; got != want {
				t.Fatal("primary-only 80% echo", got, want)
			}
			if p.Status.HasEffective(1028) {
				t.Fatal("instant survived")
			}
			s.tick += domain.Ticks(2000)
			before := m.HP
			nativeRepairCast(t, s, p, 10402, m.ID)
			if before-m.HP != base {
				t.Fatal("instant echo repeated on later cast")
			}
		})
	}
}

func TestCasterIndependentPassiveConsumers(t *testing.T) {
	for _, tc := range []struct {
		id  string
		pct int32
	}{{"SS-D04", 130}, {"SS-D07", 128}, {"SS-D08", 130}, {"SS-U04", 140}} {
		t.Run(tc.id, func(t *testing.T) {
			run := func(on bool) (int32, int32, *Scene, *entity.Entity) {
				s, p, _ := casterIndependentFixture(t, tc.id, domain.Learned{}, on)
				s.refreshStats(p)
				p.HP, p.MP = p.MaxHP, p.MaxMP
				return p.MaxHP, s.playerMoveSpeedPX(p), s, p
			}
			hp, speed, _, _ := run(false)
			got, movement, s, p := run(true)
			if tc.id == "SS-U04" {
				casterIndependentNear(t, movement, speed, tc.pct)
				if got != hp {
					t.Fatal("speed changed max HP")
				}
				distance := (float64(speed)*.6*1.6 + 40 + float64(movement)*.6*1.6 + 40) / 2
				from := p.Pos
				s.onMove(MoveTo{ID: p.ID, To: domain.Pos{MapID: from.MapID, X: from.X + distance, Y: from.Y}, ReceivedAt: time.Unix(100, 0)})
				if p.Pos == from {
					t.Fatal("movement speed not consumed by movement budget")
				}
			} else {
				casterIndependentNear(t, got, hp, tc.pct)
				if got <= hp {
					t.Fatal("runtime max HP not raised")
				}
			}
			p.Player.Worn = &domain.EquipSet{}
			s.refreshStats(p)
			if p.MaxHP != hp || s.playerMoveSpeedPX(p) != speed {
				t.Fatal("passive persists after removal")
			}
		})
	}
}
func TestCasterIndependentDefenseConditions(t *testing.T) {
	for _, id := range []string{"SS-D01", "SS-D02", "SS-D05", "SS-D06", "SS-D09", "SS-D10"} {
		t.Run(id, func(t *testing.T) {
			run := func(state bool, amount int32, distance float64, source domain.EntityID) (int32, bool) {
				s, p, m := casterIndependentFixture(t, id, domain.Learned{}, true)
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MAtk: amount}
				s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + distance, Y: 100})
				if state {
					switch id {
					case "SS-D01":
						p.Status.Apply(domain.StatusDef{ID: 1037, SourceSkillID: 10425, DurationSec: 30}, s.tick, p.ID)
					case "SS-D02":
						p.Status.Apply(domain.StatusDef{ID: 1038, SourceSkillID: 10435, DurationSec: 30}, s.tick, p.ID)
					case "SS-D05":
						p.Status.Apply(domain.StatusDef{ID: 1026, NextStatusDurationPct: 50, DurationSec: 30}, s.tick, p.ID)
					case "SS-D09":
						casterIndependentBurn(s, p, m, source)
					}
				}
				before := p.HP
				s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10401, Magic: true, GuaranteedHit: true})
				retained := id != "SS-D05" || !state || p.Status.HasEffective(1026)
				if id == "SS-D09" && state {
					retained = m.Status.HasEffective(1036)
				}
				return before - p.HP, retained
			}
			for _, state := range []bool{false, true} {
				got, retained := run(state, 1000, 100, 1)
				want := int32(1000)
				if id == "SS-D06" || id == "SS-D10" {
					want = 650
				} else if state {
					switch id {
					case "SS-D01":
						want = 720
					case "SS-D02":
						want = 450
					case "SS-D05":
						want = 500
					case "SS-D09":
						want = 600
					}
				}
				if got != want || !retained {
					t.Fatal("conditional real incoming consumer", state, got, want, retained)
				}
			}
			switch id {
			case "SS-D01":
				if got, _ := run(true, 1000, 181, 1); got != 1000 {
					t.Fatal("far damage reduced")
				}
			case "SS-D02":
				if got, _ := run(true, 800, 100, 1); got != 800 {
					t.Fatal("threshold must be strict")
				}
			case "SS-D05":
				if got, _ := run(true, 8000, 100, 1); got != 5800 {
					t.Fatal("22% cap", got)
				}
			case "SS-D09":
				if got, _ := run(true, 1000, 100, 99); got != 1000 {
					t.Fatal("foreign burn reduced damage")
				}
			}
		})
	}
}
func TestCasterIndependentD07NativeShields(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(fmt.Sprint(on), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-D07", domain.Learned{10425: 1, 10435: 1}, on)
			s.statusOverlay = domain.StatusOverlay{{Existing: 1037, Incoming: 1038}: domain.OverlayMustReplace, {Existing: 1038, Incoming: 1037}: domain.OverlayMustReplace}
			nativeRepairCast(t, s, p, 10425, p.ID)
			nativeRepairCast(t, s, p, 10435, p.ID)
			if p.Status.HasEffective(1037) != on || !p.Status.HasEffective(1038) {
				t.Fatal("native shield coexist", on)
			}
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
			s.rng = rand.New(casterIndependentZeroSource{})
			before, enemy := p.HP, m.HP
			ev, raw, reflected := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
			s.afterStrikeStatuses(m, p, ev, raw, reflected)
			want := int32(700)
			if on {
				want = 400
			}
			if before-p.HP != want {
				t.Fatal("native resist consumers", before-p.HP, want)
			}
			froze := false
			for _, o := range s.outbox {
				if e, ok := o.ev.(event.StatusApplied); ok && e.Who == m.ID && e.Status == int32(domain.StatusFreeze) {
					froze = true
				}
			}
			if !froze {
				t.Fatal("native ice counter-freeze never applied")
			}
			if on && m.HP >= enemy {
				t.Fatal("native fire reflection lost")
			}
			if s.statusOverlay[domain.OverlayKey{Existing: 1037, Incoming: 1038}] != domain.OverlayMustReplace {
				t.Fatal("global overlay mutated")
			}
		})
	}
}
func TestCasterIndependentU01Teleport(t *testing.T) {
	for _, restricted := range []string{"none", "distance", "blocked_landing", "stun"} {
		t.Run(restricted, func(t *testing.T) {
			s, p, _ := casterIndependentFixture(t, "SS-U01", domain.Learned{10418: 1, 10432: 1}, true)
			from := p.Pos
			to := domain.Pos{MapID: from.MapID, X: 400, Y: 100}
			if restricted == "distance" {
				to.X = 551
			}
			if restricted == "blocked_landing" {
				s.walkable = func(pos domain.Pos) bool { return pos.X < 350 }
			}
			if restricted == "stun" {
				p.Status.Apply(domain.StatusDef{ID: 199986, DurationSec: 30, Control: domain.ControlStun}, s.tick, p.ID)
			}
			mp := p.MP
			d, _ := s.skills.Get(10432, 1)
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
			if restricted != "none" {
				if len(s.casts) > 0 {
					sharedTargetFinishCasts(t, s)
				}
				if p.Pos != from {
					t.Fatal("illegal teleport moved")
				}
				return
			}
			sharedTargetFinishCasts(t, s)
			if p.Pos != to || mp-p.MP != (d.MPCost*85+99)/100 || !p.Player.SkillReadyAt(10432, s.tick) {
				t.Fatal("guaranteed teleport/cost/cooldown", p.Pos, mp-p.MP)
			}
			for i := 0; i < 5; i++ {
				next := from
				if i%2 == 1 {
					next = to
				}
				s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &next})
				if len(s.casts) == 0 {
					t.Fatal("zero cooldown teleport rejected")
				}
				sharedTargetFinishCasts(t, s)
				if p.Pos != next {
					t.Fatal("guaranteed teleport failed")
				}
			}
		})
	}
}
func TestCasterIndependentU02NativeSteal(t *testing.T) {
	seed := int64(0)
	for n := int64(1); n < 100; n++ {
		v := rand.New(rand.NewSource(n)).Intn(10000)
		if v >= 2000 && v < 5000 {
			seed = n
			break
		}
	}
	if seed == 0 {
		t.Fatal("seed search")
	}
	for _, on := range []bool{false, true} {
		t.Run(fmt.Sprint(on), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-U02", domain.Learned{10418: 1, 10432: 1, 10433: 1}, on)
			s.rng = rand.New(rand.NewSource(seed))
			m.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, m.ID)
			m.Status.AbsorbDamage(70)
			nativeRepairCast(t, s, p, 10433, m.ID)
			if !on {
				if !m.Status.HasEffective(1021) || p.Status.HasEffective(1021) {
					t.Fatal("baseline probability unexpectedly dispelled")
				}
				return
			}
			if m.Status.HasEffective(1021) {
				t.Fatal("+30 percentage points not consumed")
			}
			found := false
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == 1021 {
					found = true
					if a.ShieldLeft != 30 || a.ExpireAt != s.tick+domain.Ticks(30000) {
						t.Fatal("stolen shield refilled/lifetime", a.ShieldLeft, a.ExpireAt)
					}
				}
			})
			if !found {
				t.Fatal("successful native dispel not stolen")
			}
		})
	}
}
func TestCasterIndependentU03Concentration(t *testing.T) {
	for _, skill := range []domain.SkillID{10421, 10428, 10425} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			run := func(on bool) (int32, domain.Tick, bool) {
				s, p, m := casterIndependentFixture(t, "SS-U03", domain.Learned{10418: 1, skill: 1}, on)
				nativeRepairCast(t, s, p, 10418, p.ID)
				before := p.MP
				target := m.ID
				if skill == 10425 {
					target = p.ID
				}
				nativeRepairCast(t, s, p, skill, target)
				recipient := s.entities[target]
				var duration domain.Tick
				recipient.Status.EachEffective(func(a domain.Active) {
					if a.Def.SourceSkillID == int32(skill) || a.Def.SkillSnapshotID == skill {
						duration = a.ExpireAt - s.tick
					}
				})
				return before - p.MP, duration, p.Status.HasEffective(1026)
			}
			base, duration, _ := run(false)
			got, longer, ready := run(true)
			if got != (base*45+99)/100 || longer != duration || duration <= 0 || ready {
				t.Fatal("paid focus discount/native duration/consume", base, got, duration, longer, ready)
			}
		})
	}
}
func TestCasterIndependentU05NativeBurnDeath(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-U05", domain.Learned{10421: 1}, true)
	nativeRepairCast(t, s, p, 10421, m.ID)
	a, ok := legendaryCasterSourceStatus(m, 1036, p.ID)
	if !ok {
		t.Fatal("native burn absent")
	}
	p.MP = 1000
	m.HP = 1
	s.tickStatus(m, a)
	if m.HP != 0 || p.MP != 1600 {
		t.Fatal("burn kill mana reward", m.HP, p.MP)
	}
	s.tickStatus(m, a)
	if p.MP != 1600 {
		t.Fatal("same death rewarded twice")
	}
}
func TestCasterIndependentU08MirrorLifetime(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-U08", domain.Learned{10417: 1}, true)
	slow := domain.StatusDef{ID: 199985, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -20}}}
	s.applyStatusDef(p, slow, m.ID)
	nativeRepairCast(t, s, p, 10417, p.ID)
	if p.Status.HasEffective(slow.ID) {
		t.Fatal("native mirror did not cleanse existing slow")
	}
	if s.applyStatusDef(p, slow, m.ID) || p.Status.HasEffective(slow.ID) {
		t.Fatal("new slow bypassed native mirror protection")
	}
	p.Status.Remove(1011)
	if !s.applyStatusDef(p, slow, m.ID) || !p.Status.HasEffective(slow.ID) {
		t.Fatal("slow immunity outlived mirror")
	}
}

type casterIndependentZeroSource struct{}

func (casterIndependentZeroSource) Int63() int64 { return 0 }
func (casterIndependentZeroSource) Seed(int64)   {}

func TestCasterIndependentA14PullLimitAndBarrier(t *testing.T) {
	t.Run("farthest_five", func(t *testing.T) {
		s, p, m := casterIndependentFixture(t, "SS-A14", domain.Learned{10410: 1}, true)
		s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 120, Y: 100})
		near := putMonster(s, 901, 100000, domain.Stats{}, 100, 130)
		var far []*entity.Entity
		for i := 0; i < 5; i++ {
			angle := float64(i) * 2 * math.Pi / 5
			far = append(far, putMonster(s, domain.EntityID(902+i), 100000, domain.Stats{}, 100+95*math.Cos(angle), 100+95*math.Sin(angle)))
		}
		nativeRepairCast(t, s, p, 10410, 0)
		if m.Pos.X != 120 || near.Pos.Y != 130 {
			t.Fatal("near target displaced instead of farthest")
		}
		for _, e := range far {
			if sqDist(e.Pos, p.Pos) >= 95*95-.01 || 100000-e.HP <= 0 {
				t.Fatal("farthest eligible target not pulled/damaged", e.ID, e.Pos)
			}
		}
	})
	t.Run("barrier", func(t *testing.T) {
		s, p, m := casterIndependentFixture(t, "SS-A14", domain.Learned{10410: 1}, true)
		s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: 195, Y: 100})
		s.walkable = func(pos domain.Pos) bool { return pos.X < 155 || pos.X > 160 }
		nativeRepairCast(t, s, p, 10410, 0)
		if m.Pos.X <= 160 || m.Pos.X >= 195 || m.HP == 100000 {
			t.Fatal("pull crossed wall or blocked damage", m.Pos, m.HP)
		}
	})
}
func TestCasterIndependentA29ReadyKinds(t *testing.T) {
	for _, kind := range []string{"focus", "instant", "all"} {
		t.Run(kind, func(t *testing.T) {
			run := func(on bool) int32 {
				s, p, m := casterIndependentFixture(t, "SS-A29", domain.Learned{10402: 1, 10410: 1}, on)
				skill := domain.SkillID(10402)
				if kind == "focus" {
					skill = 10410
				}
				if kind == "focus" || kind == "all" {
					p.Status.Apply(domain.StatusDef{ID: 1026, DurationSec: 30, NextStatusDurationPct: 50}, s.tick, p.ID)
				}
				if kind == "instant" || kind == "all" {
					p.Status.Apply(domain.StatusDef{ID: 1028, DurationSec: 30, NextCastTimeReductionPct: 100}, s.tick, p.ID)
				}
				if kind == "all" {
					p.Status.Apply(domain.StatusDef{ID: 1027, DurationSec: 30, NextSkillDamagePct: 40}, s.tick, p.ID)
				}
				nativeRepairCast(t, s, p, skill, m.ID)
				return 100000 - m.HP
			}
			casterIndependentNear(t, run(true), run(false), 166)
		})
	}
}
func TestCasterIndependentA18NativeBurnProbability(t *testing.T) {
	for _, seed := range []int64{0, 1, 3, 7, 11} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			run := func(on bool) (bool, domain.Tick) {
				s, p, m := casterIndependentFixture(t, "SS-A18", domain.Learned{10405: 1, 10423: 5}, on)
				s.rng = rand.New(rand.NewSource(seed))
				if seed == 0 {
					s.rng = rand.New(casterIndependentZeroSource{})
				}
				nativeRepairCast(t, s, p, 10405, m.ID)
				a, ok := legendaryCasterSourceStatus(m, 1036, p.ID)
				return ok, a.ExpireAt
			}
			a, d := run(false)
			b, e := run(true)
			if seed == 0 && !a {
				t.Fatal("positive native burn fixture absent")
			}
			if a != b || d != e {
				t.Fatal("area damage altered native burn probability/duration", a, b, d, e)
			}
		})
	}
}
func TestCasterCombinationPhantomDamage(t *testing.T) {
	run := func(boost bool) int32 {
		powers := []string{"SS-A27"}
		if boost {
			powers = append(powers, "SS-D04")
		}
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10418: 1, 10432: 5}, powers...)
		to := domain.Pos{MapID: p.Pos.MapID, X: 400, Y: 100}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
		sharedTargetFinishCasts(t, s)
		return 100000 - m.HP
	}
	casterIndependentNear(t, run(true), run(false), 130)
}
func TestCasterCombinationFreezeRadius250(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10428: 1}, "SS-A23", "SS-D06")
	edge := putMonster(s, 901, 100000, domain.Stats{}, m.Pos.X+249, 100)
	outside := putMonster(s, 902, 100000, domain.Stats{}, m.Pos.X+251, 100)
	nativeRepairCast(t, s, p, 10428, m.ID)
	if !m.Status.HasEffective(domain.StatusFreeze) || !edge.Status.HasEffective(domain.StatusFreeze) || outside.Status.HasEffective(domain.StatusFreeze) {
		t.Fatal("combined freeze250 geometry lost")
	}
}
func TestCasterCombinationMirror180Cap(t *testing.T) {
	for _, amount := range []int32{100, 3000} {
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{}, "SS-A26", "SS-D03")
		p.Stats = domain.Stats{}
		m.Stats = domain.Stats{MAtk: amount}
		p.Status.Apply(domain.StatusDef{ID: 1011, SourceSkillID: 10417, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, p.ID)
		ev, raw, reflected := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 10401, Magic: true, GuaranteedHit: true})
		s.afterStrikeStatuses(m, p, ev, raw, reflected)
		if got, want := 100000-m.HP, min(raw*180/100, p.MaxHP*18/100); got != want {
			t.Fatal("mirror combination ratio/cap", got, want)
		}
	}
}

// Native Phantom is a force teleport, not ordinary walking. U01 retains this
// original rule: movement-only root does not become a newly invented cast lock.
func TestCasterIndependentU01RetainsNativeMovementRule(t *testing.T) {
	for _, on := range []bool{false, true} {
		s, p, _ := casterIndependentFixture(t, "SS-U01", domain.Learned{10418: 1, 10432: 5}, on)
		p.Status.Apply(domain.StatusDef{ID: 199986, DurationSec: 30, BlockMove: true, Immobile: 1}, s.tick, p.ID)
		to := domain.Pos{MapID: p.Pos.MapID, X: 400, Y: 100}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
		sharedTargetFinishCasts(t, s)
		if p.Pos != to || !p.Status.HasEffective(199986) {
			t.Fatal("native movement-only root semantics changed", on, p.Pos)
		}
	}
}
func TestCasterIndependentA10NativeNoElementDefense(t *testing.T) {
	run := func(on bool) int32 {
		s, p, m := casterIndependentFixture(t, "SS-A10", domain.Learned{10409: 1}, on)
		nativeRepairCast(t, s, p, 10409, m.ID)
		return 100000 - m.HP
	}
	if a, b := run(false), run(true); a != b || a <= 0 {
		t.Fatal("ice penetration fabricated bonus without elemental defense", a, b)
	}
}

func TestCasterCombinationFreezeArrowMastery(t *testing.T) {
	run := func(master bool) int32 {
		powers := []string{"SS-A24"}
		if master {
			powers = append(powers, "SS-A09")
		}
		s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10428: 1, 10409: 1, 10427: 1}, powers...)
		nativeRepairCast(t, s, p, 10428, m.ID)
		if !m.Status.HasEffective(domain.StatusFreeze) {
			t.Fatal("child broke parent freeze")
		}
		return 100000 - m.HP
	}
	casterIndependentNear(t, run(true), run(false), 200)
}
func TestCasterCombinationBurnKillRewards(t *testing.T) {
	for _, id := range []string{"SS-A02", "SS-A19"} {
		t.Run(id, func(t *testing.T) {
			skill := domain.SkillID(10401)
			if id == "SS-A19" {
				skill = 10421
			}
			s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{skill: 1}, "SS-U05", id)
			p.MP = 1000
			if id == "SS-A02" {
				casterIndependentBurn(s, p, m, p.ID)
				m.HP = 250
			} else {
				m.HP = 1
			}
			d, _ := s.skills.Get(skill, 1)
			nativeRepairCast(t, s, p, skill, m.ID)
			want := int32(1000) - d.MPCost + p.MaxMP*6/100
			if m.HP != 0 || p.MP != want {
				t.Fatal("derived burn death reward", m.HP, p.MP, want)
			}
			s.stepCasts()
			if p.MP != want {
				t.Fatal("duplicate death reward")
			}
		})
	}
}
func TestCasterIndependentA24DamageImmunity(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-A24", domain.Learned{10428: 1, 10409: 1}, true)
	m.Status.Apply(domain.StatusDef{ID: 199984, DurationSec: 30, MagicDamageReductionPct: 100, DamageReductionCapPct: 100}, s.tick, m.ID)
	nativeRepairCast(t, s, p, 10428, m.ID)
	if m.HP != 100000 || !m.Status.HasEffective(domain.StatusFreeze) {
		t.Fatal("damage immunity/freeze independence", m.HP)
	}
}
func TestCasterIndependentA30DeadPrimaryNoTransfer(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-A30", domain.Learned{10402: 1}, true)
	side := putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
	m.HP = 1
	p.Status.Apply(domain.StatusDef{ID: 1028, DurationSec: 30, NextCastTimeReductionPct: 100}, s.tick, p.ID)
	nativeRepairCast(t, s, p, 10402, m.ID)
	_, n := casterIndependentDamageCount(s, side.ID)
	if m.HP != 0 || n != 1 {
		t.Fatal("dead primary echo transferred", m.HP, n)
	}
}

type casterIndependentFixedSource struct{ value int64 }

func (s casterIndependentFixedSource) Int63() int64 { return s.value << 32 }
func (casterIndependentFixedSource) Seed(int64)     {}
func TestCasterIndependentD03NativeDefenseConsumers(t *testing.T) {
	for _, skill := range []domain.SkillID{10417, 10425, 10435} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			run := func(on bool) (int32, int32, bool, domain.Active) {
				s, p, m := casterIndependentFixture(t, "SS-D03", domain.Learned{skill: 1}, on)
				nativeRepairCast(t, s, p, skill, p.ID)
				var active domain.Active
				p.Status.EachEffective(func(a domain.Active) {
					if a.Def.SourceSkillID == int32(skill) {
						active = a
					}
				})
				if active.Def.ID == 0 {
					t.Fatal("native defensive state absent")
				}
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
				value := int64(1<<31) * 45 / 100
				magic := skill == 10417
				if magic {
					value = 45
				}
				s.rng = rand.New(casterIndependentFixedSource{value})
				before, enemy := p.HP, m.HP
				ev, raw, reflection := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: magic, GuaranteedHit: true})
				s.afterStrikeStatuses(m, p, ev, raw, reflection)
				return before - p.HP, enemy - m.HP, m.Status.HasEffective(domain.StatusFreeze), active
			}
			base, baseReflect, baseFreeze, a := run(false)
			got, reflect, freeze, b := run(true)
			if a.ExpireAt != b.ExpireAt {
				t.Fatal("defensive duration changed")
			}
			switch skill {
			case 10417:
				if b.Def.MagicFullReflectChancePct != min(int32(100), a.Def.MagicFullReflectChancePct+30) || got != 0 || base <= 0 || reflect <= 0 {
					t.Fatal("mirror +30 points not consumed", base, got, reflect, a.Def.MagicFullReflectChancePct, b.Def.MagicFullReflectChancePct)
				}
			case 10425:
				if got != 550 || base != 700 || reflect <= baseReflect || b.Def.PhysicalReflectPct != a.Def.PhysicalReflectPct*150/100 {
					t.Fatal("fire defense/reflection consumers", base, got, baseReflect, reflect)
				}
			case 10435:
				if got != 550 || base != 700 || baseFreeze || !freeze || b.Def.FreezeAttackerChance != min(int32(100), a.Def.FreezeAttackerChance+30) {
					t.Fatal("ice freeze probability consumer", base, got, baseFreeze, freeze)
				}
			}
		})
	}
}
func TestCasterIndependentIceBurstNativeControl(t *testing.T) {
	for _, id := range []string{"SS-A12", "SS-A13", "SS-A14", "SS-A15"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, id, domain.Learned{10410: 1}, true)
			s.rng = rand.New(casterIndependentZeroSource{})
			nativeRepairCast(t, s, p, 10410, m.ID)
			if m.HP >= 100000 || !m.Status.HasEffective(domain.StatusFreeze) {
				t.Fatal("damage modifier lost original freeze", m.HP)
			}
		})
	}
}

func TestCasterCombinationFireBoltSplashSettlement(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10401: 1, 10431: 1}, "SS-A03", "SS-A02", "SS-A25")
	side := putMonster(s, 901, 100000, domain.Stats{}, 150, 100)
	casterIndependentBurn(s, p, m, p.ID)
	casterIndependentBurn(s, p, side, p.ID)
	nativeRepairCast(t, s, p, 10401, m.ID)
	a, ac := casterIndependentDamageCount(s, m.ID)
	b, bc := casterIndependentDamageCount(s, side.ID)
	if a != b || ac != 2 || bc != 2 || a <= 288 {
		t.Fatal("splash lost native fire/passive/settlement or recurred", a, b, ac, bc)
	}
}
func TestCasterCombinationStolenShieldKeepsBalance(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10418: 1, 10432: 1, 10433: 5}, "SS-U02", "SS-D03")
	m.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, m.ID)
	m.Status.AbsorbDamage(70)
	nativeRepairCast(t, s, p, 10433, m.ID)
	found := false
	p.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 1021 {
			found = true
			if a.ShieldLeft != 30 || a.ExpireAt != s.tick+domain.Ticks(30000) {
				t.Fatal("strengthened transferred shield refilled or time changed", a.ShieldLeft, a.ExpireAt)
			}
		}
	})
	if !found {
		t.Fatal("shield not stolen")
	}
}

func TestCasterIndependentA19NewBurnOnly(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-A19", domain.Learned{10421: 1}, true)
	s.statusOverlay = domain.StatusOverlay{{Existing: 1036, Incoming: 1036}: domain.OverlayCoexist}
	nativeRepairCast(t, s, p, 10421, m.ID)
	first := 100000 - m.HP
	s.tick += domain.Ticks(6000)
	before := m.HP
	nativeRepairCast(t, s, p, 10421, m.ID)
	if second := before - m.HP; second != first || first <= 0 {
		t.Fatal("new burn re-settled old coexist instance", first, second)
	}
}

// The consumer boundary is intentional: ordinary monsters have no elemental
// equipment stats. No test here asserts that a player-versus-player command is admitted.
func TestCasterIndependentA10DerivedStrikeBoundary(t *testing.T) {
	for _, kind := range []string{"proc", "child", "mitigation"} {
		t.Run(kind, func(t *testing.T) {
			s, p, _ := casterIndependentFixture(t, "SS-A10", domain.Learned{}, true)
			join(t, s, 2, 200, "elemental boundary", 130, 100)
			target := s.players[2]
			target.HP, target.MaxHP = 10000, 10000
			target.Stats = domain.Stats{}
			var defense domain.EquipmentCombatStats
			stat := domain.ElementCombatStats{ResistBP: 6000}
			if kind == "mitigation" {
				stat = domain.ElementCombatStats{MitigationBP: 6000}
			}
			defense.Elements[domain.HurtElementIndex(domain.HurtIce)] = stat
			cacheElementV5(s, target, defense)
			blow := combat.Blow{SkillID: 10409, Magic: true, HurtType: domain.HurtIce, GuaranteedHit: true, Origin: domain.DamageProcOrigin, ProcDepth: 1, LegendaryPayload: true}
			if kind == "child" {
				blow.LegendaryChild = true
			}
			s.resolveStrikeStatuses(p, target, &combat.HitTracker{}, blow)
			if got := 10000 - target.HP; got != 70 {
				t.Fatal("derived ice payload lost half element-only penetration", got)
			}
		})
	}
}
func TestCasterIndependentA25BurnTick(t *testing.T) {
	run := func(on bool) int32 {
		s, p, m := casterIndependentFixture(t, "SS-A25", domain.Learned{10421: 1, 10431: 1}, on)
		nativeRepairCast(t, s, p, 10421, m.ID)
		a, ok := legendaryCasterSourceStatus(m, 1036, p.ID)
		if !ok {
			t.Fatal("native burn missing")
		}
		before := m.HP
		s.tickStatus(m, a)
		return before - m.HP
	}
	casterIndependentNear(t, run(true), run(false), 104)
}
func TestCasterIndependentU03StatuslessDoesNotSpendFocus(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-U03", domain.Learned{10418: 1, 10402: 1}, true)
	nativeRepairCast(t, s, p, 10418, p.ID)
	before := p.MP
	d, _ := s.skills.Get(10402, 1)
	nativeRepairCast(t, s, p, 10402, m.ID)
	if before-p.MP != d.MPCost || !p.Status.HasEffective(1026) {
		t.Fatal("statusless spell borrowed focus discount/consumed focus")
	}
}

func TestCasterIndependentA15AcceptedControlSnapshot(t *testing.T) {
	run := func(before bool) int32 {
		s, p, m := casterIndependentFixture(t, "SS-A15", domain.Learned{10410: 1}, true)
		slow := domain.StatusDef{ID: 199983, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -20, Mode: domain.ModePercent}}}
		if before {
			m.Status.Apply(slow, s.tick, p.ID)
		}
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10410, Target: m.ID})
		if len(s.casts) == 0 {
			t.Fatal("cast not admitted")
		}
		if before {
			m.Status.Remove(slow.ID)
		} else {
			m.Status.Apply(slow, s.tick, p.ID)
		}
		sharedTargetFinishCasts(t, s)
		return 100000 - m.HP
	}
	casterIndependentNear(t, run(true), run(false), 160)
}
func TestCasterIndependentA24ControlImmunity(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-A24", domain.Learned{10428: 1, 10409: 1}, true)
	s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Kind: domain.MonsterNormal, ImmuneSkills: map[domain.SkillID]struct{}{10428: {}}}}
	nativeRepairCast(t, s, p, 10428, m.ID)
	if m.HP != 99850 || m.Status.HasEffective(domain.StatusFreeze) {
		t.Fatal("freeze immunity suppressed independent native child damage or control landed", m.HP)
	}
}
func TestCasterIndependentD03ChanceCap(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-D03", domain.Learned{10417: 15}, true)
	nativeRepairCast(t, s, p, 10417, p.ID)
	if p.Status.MagicFullReflectChance() != 100 {
		t.Fatal("mirror chance exceeds100 or not capped")
	}
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MAtk: 1000}
	s.rng = rand.New(casterIndependentFixedSource{99})
	before := p.HP
	ev, raw, refl := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, GuaranteedHit: true})
	s.afterStrikeStatuses(m, p, ev, raw, refl)
	if p.HP != before || refl <= 0 {
		t.Fatal("capped guaranteed resist not consumed", before-p.HP, refl)
	}
}

func TestCasterIndependentA27OnlySuccessfulMoveAndCost(t *testing.T) {
	for _, moving := range []bool{false, true} {
		t.Run(fmt.Sprint(moving), func(t *testing.T) {
			s, p, m := casterIndependentFixture(t, "SS-A27", domain.Learned{10418: 1, 10432: 5}, true)
			d, _ := s.skills.Get(10432, 5)
			to := p.Pos
			if moving {
				to.X += 300
			}
			before := p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
			sharedTargetFinishCasts(t, s)
			if (m.HP < 100000) != moving {
				t.Fatal("failed/no movement generated explosion", moving, m.HP)
			}
			if before-p.MP != d.MPCost {
				t.Fatal("derived explosion charged additional mana", before-p.MP, d.MPCost)
			}
			at := s.tick + domain.Ticks(int(d.CooldownMS))
			if !p.Player.SkillReadyAt(10432, at) || p.Player.SkillReadyAt(10432, at-1) {
				t.Fatal("power changed native teleport cooldown")
			}
		})
	}
}
func TestCasterCombinationDispelSeesSameCastStolenDefense(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10418: 1, 10432: 1, 10433: 5}, "SS-A28", "SS-U02")
	m.Status.Apply(domain.StatusDef{ID: 1037, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, PhysicalReflectPct: 5}, s.tick, m.ID)
	nativeRepairCast(t, s, p, 10433, m.ID)
	if !p.Status.HasEffective(1037) || m.Status.HasEffective(1037) {
		t.Fatal("native steal did not precede damage")
	}
	bs, bp, bm := casterIndependentFixture(t, "SS-A28", domain.Learned{10401: 1}, false)
	nativeRepairCast(t, bs, bp, 10401, bm.ID)
	casterIndependentNear(t, 100000-m.HP, 100000-bm.HP, 270)
}

func TestCasterIndependentA05UnselectedCenter(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-A05", domain.Learned{10402: 1}, true)
	a := putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
	b := putMonster(s, 902, 100000, domain.Stats{}, 150, 100)
	at := m.Pos
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10402, At: &at})
	sharedTargetFinishCasts(t, s)
	bs, bp, bm := casterIndependentFixture(t, "SS-A05", domain.Learned{10402: 1}, false)
	nativeRepairCast(t, bs, bp, 10402, bm.ID)
	base := 100000 - bm.HP
	for _, e := range []*entity.Entity{m, a, b} {
		if got := 100000 - e.HP; got != base {
			t.Fatal("unselected explosion invented a primary bonus", e.ID, got, base)
		}
	}
}
func TestCasterIndependentU02AgedTransferredShield(t *testing.T) {
	s, p, m := casterIndependentFixture(t, "SS-U02", domain.Learned{10418: 1, 10432: 1, 10433: 5}, true)
	m.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, m.ID)
	m.Status.AbsorbDamage(70)
	s.tick += domain.Ticks(5000)
	nativeRepairCast(t, s, p, 10433, m.ID)
	found := false
	p.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == 1021 {
			found = true
			if a.ShieldLeft != 30 || a.ExpireAt != s.tick+domain.Ticks(22500) {
				t.Fatal("remaining lifetime not scaled or shield refilled", a.ShieldLeft, a.ExpireAt)
			}
		}
	})
	if !found {
		t.Fatal("aged native shield not transferred")
	}
}
