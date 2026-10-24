package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"testing"
	"time"
)

func TestLegendaryUniversalSoloManaCost(t *testing.T) {
	for _, tc := range []struct {
		id    string
		race  domain.Race
		skill domain.SkillID
		pct   int32
		self  bool
	}{
		{"TY-U03", domain.Warrior, 10002, 70, false}, {"TY-U04", domain.Healer, 10301, 70, true}, {"TY-U04", domain.Healer, 10303, 70, true},
		{"TY-U12", domain.Warrior, 10016, 70, false}, {"TY-U13", domain.Healer, 10320, 80, true}, {"TY-U14", domain.Warlock, 10421, 70, false}, {"TY-U14", domain.Assassin, 10211, 70, true},
	} {
		t.Run(fmt.Sprintf("%s/skill=%d", tc.id, tc.skill), func(t *testing.T) {
			s, p, m := universalSoloFixture(t, tc.id, tc.race, domain.Learned{tc.skill: 1})
			s.refreshStats(p)
			p.MP = p.MaxMP
			p.HP = p.MaxHP / 2
			native, _ := s.skills.Get(tc.skill, 1)
			target := m.ID
			if tc.self {
				target = p.ID
			}
			before := p.MP
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: target})
			want := (native.MPCost*tc.pct + 99) / 100
			if before-p.MP != want || d.MPCost != want {
				t.Fatalf("paid %d want%d", before-p.MP, want)
			}
		})
	}
	for _, id := range []string{"TY-U04", "TY-U12", "TY-U13", "TY-U14"} {
		t.Run(id+"/unrelated", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warrior, domain.Learned{10002: 1})
			before := p.MP
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
			if before-p.MP != 8 || d.MPCost != 8 {
				t.Fatal("unrelated skill discounted")
			}
		})
	}
}

func TestLegendaryUniversalSoloHealing(t *testing.T) {
	for _, id := range []string{"TY-U06", "TY-U16", "TY-U17", "TY-D12"} {
		t.Run(id, func(t *testing.T) {
			skill := domain.SkillID(10301)
			if id == "TY-U06" {
				skill = 10302
			}
			if id == "TY-U17" {
				skill = 10305
			}
			s, p, _ := universalSoloFixture(t, id, domain.Healer, domain.Learned{skill: 1})
			a := nativeRepairAlly(t, s, p, 2, 140)
			target := a
			if id == "TY-D12" {
				target = p
				target.MaxHP = 1000
				target.HP = 1
			}
			target.MP = 0
			target.MaxMP = 1000
			if id == "TY-D12" {
				target.MP = 1000
			}
			if id == "TY-U17" {
				target.HP = 0
			}
			assassinRegressionCast(t, s, p, UseSkill{Skill: skill, Target: target.ID})
			switch id {
			case "TY-U06":
				if target.MP != 150 {
					t.Fatal("mana restore", target.MP)
				}
			case "TY-U16":
				if target.HP != 321 {
					t.Fatal("direct flat+maxHP healing", target.HP)
				}
			case "TY-D12":
				if target.HP != 281 {
					t.Fatal("incoming direct healing", target.HP)
				}
			case "TY-U17":
				if target.HP != 70 {
					t.Fatal("resurrection percentage", target.HP)
				}
				n := 0
				for _, o := range s.outbox {
					if _, ok := o.ev.(event.PlayerRevived); ok {
						n++
					}
				}
				if n != 1 {
					t.Fatal("revive event count", n)
				}
			}
		})
	}
	t.Run("TY-U10", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U10", domain.Healer, domain.Learned{10304: 1})
		near := nativeRepairAlly(t, s, p, 2, 500)
		far := nativeRepairAlly(t, s, p, 3, 521)
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10304})
		if near.HP != 231 || far.HP != 1 {
			t.Fatalf("healing radius hp%d/%d", near.HP, far.HP)
		}
	})
	for _, id := range []string{"TY-A17", "TY-U16"} {
		t.Run(id+"/drinking_sword", func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Swordsman, domain.Learned{10104: 1})
			p.HP = 1000
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10104, Target: m.ID})
			if p.HP != 1032 {
				t.Fatalf("native sword restore %d", p.HP)
			}
			universalSoloDamage(t, s, m.ID, 10104, 150, 1)
		})
	}
	t.Run("TY-A17/no_leech", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A17", domain.Warrior, domain.Learned{10002: 1})
		p.HP = 1000
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		if p.HP != 1000 {
			t.Fatal("invented lifesteal")
		}
	})
}

func TestLegendaryUniversalSoloTiming(t *testing.T) {
	for _, id := range []string{"TY-A07", "TY-U01"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Warlock, domain.Learned{10401: 1})
			s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 500, Y: 100})
			started := time.Unix(100, 0)
			s.now = func() time.Time { return started }
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10401, Target: m.ID})
			if len(s.casts) != 1 {
				t.Fatal("cast rejected")
			}
			c := s.casts[0]
			native, _ := s.skills.Get(10401, 1)
			wantPrepare := native.PrepareMS
			if id == "TY-U01" {
				wantPrepare = wantPrepare * 70 / 100
			}
			if c.releaseAtWall.Sub(started) != time.Duration(wantPrepare)*time.Millisecond {
				t.Fatal("actual release deadline", c.releaseAtWall, started, wantPrepare)
			}
			s.stepCastsAt(c.releaseAtWall.Add(-time.Nanosecond), false)
			if m.HP != m.MaxHP {
				t.Fatal("damage before windup")
			}
			s.stepCastsAt(c.releaseAtWall, false)
			if len(s.casts) != 1 {
				t.Fatal("projectile disappeared")
			}
			impact := s.casts[0].resolveAtWall
			speed := native.ProjectileSpeedPXPerSec
			if id == "TY-A07" {
				speed *= 2
			}
			wantTravel := projectileTravelDuration(p.Pos, m.Pos, speed)
			if impact.Sub(c.releaseAtWall) != wantTravel {
				t.Fatalf("actual travel%v want%v", impact.Sub(c.releaseAtWall), wantTravel)
			}
			s.stepCastsAt(impact.Add(-time.Nanosecond), false)
			if m.HP != m.MaxHP {
				t.Fatal("early projectile")
			}
			s.stepCastsAt(impact, false)
			if m.HP == m.MaxHP || len(s.casts) != 0 {
				t.Fatal("impact did not settle")
			}
		})
	}
	t.Run("TY-U15", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U15", domain.Warrior, domain.Learned{10002: 1})
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
		ready := p.CapturePlayerRuntime(s.tick).SkillReadyIn[10002]
		if ready != domain.Ticks(800) {
			t.Fatal("actual cooldown", ready)
		}
		s.tick += ready - 1
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID})
		if len(s.casts) != 0 {
			t.Fatal("cooldown accepted early")
		}
		s.tick++
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
	})
}

func TestLegendaryUniversalSoloMovement(t *testing.T) {
	t.Run("TY-U07", func(t *testing.T) {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprint(blocked), func(t *testing.T) {
				s, p, _ := universalSoloFixture(t, "TY-U07", domain.Warlock, domain.Learned{10432: 1})
				s.refreshStats(p)
				p.MP = p.MaxMP
				s.rng = rand.New(rand.NewSource(universalSoloSuccessSeed(6000)))
				from := p.Pos
				to := domain.Pos{MapID: p.Pos.MapID, X: 700, Y: 100}
				before := p.MP
				if blocked {
					s.walkable = func(domain.Pos) bool { return false }
					s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
					if len(s.casts) != 0 || p.Pos != from || p.MP != before {
						t.Fatal("blocked teleport executed")
					}
					return
				}
				d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10432, At: &to})
				if p.Pos != to || before-p.MP != d.MPCost {
					t.Fatal("extended teleport failed")
				}
			})
		}
	})
	t.Run("TY-A20", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-A20", domain.Warlock, domain.Learned{10432: 1})
		s.refreshStats(p)
		p.MP = p.MaxMP
		s.rng = rand.New(rand.NewSource(universalSoloSuccessSeed(6000)))
		s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: 1000, Y: 100})
		positions := map[domain.EntityID]domain.Pos{}
		for i := 0; i < 8; i++ {
			e := putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 410+float64(i*10), 100)
			positions[e.ID] = e.Pos
		}
		immune := putMonster(s, 909, 10000, domain.Stats{}, 405, 100)
		immune.Status = domain.NewStatusSet()
		immune.Status.Apply(domain.StatusDef{ID: 190983, DurationSec: 60, SuppressHarmful: true}, s.tick, immune.ID)
		to := domain.Pos{MapID: p.Pos.MapID, X: 300, Y: 100}
		before := p.MP
		d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10432, At: &to})
		if p.Pos != to || before-p.MP != d.MPCost {
			t.Fatal("root movement/cost")
		}
		for id, pos := range positions {
			e := s.entities[id]
			moved := id < 907
			if (e.Pos != pos) != moved {
				t.Fatal("nearest six pull cap", id, e.Pos, pos)
			}
			if moved && mathDistance(pos, e.Pos) != 100 {
				t.Fatal("pull distance", pos, e.Pos)
			}
			if e.HP != 10000 {
				t.Fatal("pull caused damage")
			}
		}
		if immune.Pos.X != 405 || len(s.casts) != 0 || len(s.attacks) != 0 {
			t.Fatal("immunity/recursion")
		}
	})
}
func mathDistance(a, b domain.Pos) float64 {
	x := a.X - b.X
	if x < 0 {
		x = -x
	}
	return x
}

func TestLegendaryUniversalSoloDetectionAndDispel(t *testing.T) {
	t.Run("TY-U19/detection", func(t *testing.T) {
		s, p, m := universalSoloFixture(t, "TY-U19", domain.Assassin, domain.Learned{10228: 1})
		for _, x := range []float64{450, 501} {
			s.placeTrap(m, domain.SkillDef{ID: 10213}, domain.Pos{MapID: p.Pos.MapID, X: x, Y: 100}, 50, 100)
		}
		// A deterministic successful native45% roll, not a modified skill chance.
		seed := int64(0)
		for ; seed < 1000; seed++ {
			if rand.New(rand.NewSource(seed)).Intn(10000) < 4500 {
				break
			}
		}
		s.rng = rand.New(rand.NewSource(seed))
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10228})
		for _, tr := range s.traps {
			if tr.Trap.VisibleToPlayer(p.ID) != (tr.Pos.X == 450) {
				t.Fatal("native detection boundary", tr.Pos)
			}
		}
	})
	t.Run("TY-U19/reveal", func(t *testing.T) {
		s, p, _ := universalSoloFixture(t, "TY-U19", domain.Healer, domain.Learned{10327: 1})
		join(t, s, 2, 102, "enemy", 650, 100)
		join(t, s, 3, 103, "far", 701, 100)
		for _, e := range []domain.EntityID{2, 3} {
			s.players[e].Status.Apply(domain.StatusDef{ID: 1017, DurationSec: 60, Invisible: true}, s.tick, e)
		}
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10327})
		if entityInvisible(s.players[2]) || !entityInvisible(s.players[3]) {
			t.Fatal("reveal range")
		}
	})
	t.Run("TY-U18", func(t *testing.T) {
		for _, success := range []bool{false, true} {
			t.Run(fmt.Sprint(success), func(t *testing.T) {
				s, p, m := universalSoloFixture(t, "TY-U18", domain.Warlock, domain.Learned{10433: 1})
				native, _ := s.skills.Get(10433, 1)
				fx := native.DirectEffects()[0]
				if len(fx.RemoveStatusIDs) == 0 {
					t.Fatal("native dispel list missing")
				}
				m.Status.Apply(domain.StatusDef{ID: fx.RemoveStatusIDs[0], DurationSec: 60, TypeKnown: true, Type: domain.StatusGood}, s.tick, m.ID)
				m.Status.Apply(domain.StatusDef{ID: 190984, DurationSec: 60}, s.tick, m.ID)
				other := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
				other.Status = domain.NewStatusSet()
				other.Status.Apply(domain.StatusDef{ID: fx.RemoveStatusIDs[0], DurationSec: 60}, s.tick, other.ID)
				seed := int64(0)
				for ; seed < 10000; seed++ {
					r := rand.New(rand.NewSource(seed)).Intn(10000)
					if success && r >= 2000 && r < 6000 || !success && r >= 6000 {
						break
					}
				}
				s.rng = rand.New(rand.NewSource(seed))
				assassinRegressionCast(t, s, p, UseSkill{Skill: 10433, Target: m.ID})
				if m.Status.Has(fx.RemoveStatusIDs[0]) == success || !m.Status.Has(190984) || !other.Status.Has(fx.RemoveStatusIDs[0]) {
					t.Fatal("dispel chance/kind boundary", success)
				}
			})
		}
	})
}

func universalSoloSuccessSeed(chance int) int64 {
	for seed := int64(0); ; seed++ {
		if rand.New(rand.NewSource(seed)).Intn(10000) < chance {
			return seed
		}
	}
}
