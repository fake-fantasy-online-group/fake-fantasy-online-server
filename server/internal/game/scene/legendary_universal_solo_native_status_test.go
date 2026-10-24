package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math/rand"
	"testing"
)

func TestLegendaryUniversalSoloNativePeriodicProducer(t *testing.T) {
	for _, tc := range []struct {
		id                     string
		race                   domain.Race
		skill                  domain.SkillID
		status                 domain.StatusID
		damagePct, durationPct int32
	}{
		{"TY-A18", domain.Assassin, 10214, domain.StatusPoison, 200, 100},
		{"TY-A22", domain.Assassin, 10214, domain.StatusPoison, 100, 150},
		{"TY-A22", domain.Warlock, 10421, 1036, 100, 150},
		{"TY-A27", domain.Warlock, 10421, 1036, 200, 100},
		{"TY-A28", domain.Assassin, 10214, domain.StatusPoison, 200, 100},
		{"TY-A29", domain.Warlock, 10421, 1036, 200, 100},
		{"TY-U05", domain.Healer, 10303, 10303, 160, 100},
	} {
		t.Run(fmt.Sprintf("%s/skill=%d", tc.id, tc.skill), func(t *testing.T) {
			var baseDamage int32
			var baseDuration domain.Tick
			var baseInterval int32
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprint(enabled), func(t *testing.T) {
					powers := []string{}
					if enabled {
						powers = append(powers, tc.id)
					}
					s, p, m := nativeRepairFixture(t, tc.race, domain.Learned{tc.skill: 1}, powers...)
					s.rng = rand.New(rand.NewSource(1))
					s.refreshStats(p)
					p.MP = p.MaxMP
					p.Stats.CritRate = 10000
					p.Stats.MCritRate = 10000
					target := m
					cmd := UseSkill{Skill: tc.skill, Target: m.ID}
					if tc.race == domain.Assassin {
						cmd.Target = 0
						cmd.At = &m.Pos
					}
					if tc.race == domain.Healer {
						target = p
						cmd.Target = p.ID
					}
					assassinRegressionCast(t, s, p, cmd)
					if tc.race == domain.Assassin {
						for _, tr := range s.traps {
							s.triggerAssassinTrap(tr, false)
						}
					}
					a := universalSoloActive(t, target, tc.status)
					target.HP = target.MaxHP / 2
					before := target.HP
					s.tickStatus(target, a)
					damage := before - target.HP
					if damage < 0 {
						damage = -damage
					}
					if !enabled {
						baseDamage, baseDuration, baseInterval = damage, a.ExpireAt-s.tick, a.Def.IntervalSec
						if baseDamage <= 0 {
							t.Fatal("native periodic source did not act")
						}
						return
					}
					if damage != baseDamage*tc.damagePct/100 || a.ExpireAt-s.tick != baseDuration*domain.Tick(tc.durationPct)/100 || a.Def.IntervalSec != baseInterval {
						t.Fatalf("native periodic result amount%d duration%d interval%d want%d/%d/%d", damage, a.ExpireAt-s.tick, a.Def.IntervalSec, baseDamage*tc.damagePct/100, baseDuration*domain.Tick(tc.durationPct)/100, baseInterval)
					}
					if a.Source != p.ID || a.Def.LegendaryPowerID != "" {
						t.Fatal("native source identity changed")
					}
				})
			}
		})
	}
}

func TestLegendaryUniversalSoloNativeStatusProducer(t *testing.T) {
	for _, tc := range []struct {
		id                     string
		race                   domain.Race
		skill                  domain.SkillID
		status                 domain.StatusID
		durationPct, shieldPct int32
	}{
		{"TY-A13", domain.Warlock, 10428, domain.StatusFreeze, 140, 100},
		{"TY-D16", domain.Healer, 10320, 1021, 100, 160},
		{"TY-D18", domain.Healer, 10320, 1021, 140, 100},
		{"TY-U11", domain.Healer, 10321, 10321, 150, 100},
	} {
		t.Run(tc.id, func(t *testing.T) {
			var duration domain.Tick
			var shield int32
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprint(enabled), func(t *testing.T) {
					powers := []string{}
					if enabled {
						powers = append(powers, tc.id)
					}
					s, p, m := nativeRepairFixture(t, tc.race, domain.Learned{tc.skill: 1}, powers...)
					s.refreshStats(p)
					p.MP = p.MaxMP
					p.Stats.EffectHitBP = 1000000
					target := p
					if tc.id == "TY-A13" {
						target = m
					}
					assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: target.ID})
					a := universalSoloActive(t, target, tc.status)
					if !enabled {
						duration, shield = a.ExpireAt-s.tick, a.ShieldLeft
						return
					}
					if a.ExpireAt-s.tick != duration*domain.Tick(tc.durationPct)/100 || a.ShieldLeft != shield*tc.shieldPct/100 {
						t.Fatal("native status committed magnitude/lifetime", a)
					}
					if tc.id == "TY-A13" && canMove(target) {
						t.Fatal("native freeze did not block movement")
					}
					if a.ShieldLeft > 0 {
						m.Stats.MinAtk = 100
						m.Stats.MaxAtk = 100
						before := target.HP
						attackNativeShieldConsumer(s, m, target)
						if target.HP != before || universalShieldValue(target) != a.ShieldLeft-100 {
							t.Fatal("native shield not consumed")
						}
					}
				})
			}
		})
	}
}

func attackNativeShieldConsumer(s *Scene, source, target *entity.Entity) {
	s.resolveStrikeStatuses(source, target, &combat.HitTracker{}, combat.Blow{SkillID: 10002, GuaranteedHit: true})
}

func TestLegendaryUniversalSoloNativeManaNova(t *testing.T) {
	for _, mana := range []int32{0, 1000} {
		t.Run(fmt.Sprintf("TY-A19/mp=%d", mana), func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-A19", domain.Assassin, domain.Learned{10217: 1})
			m.MaxMP = 1000
			m.MP = mana
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10217, At: &m.Pos})
			if len(s.traps) != 1 {
				t.Fatal("nova trap count")
			}
			for _, tr := range s.traps {
				s.triggerAssassinTrap(tr, false)
			}
			if m.MP != max(int32(0), mana-540) {
				t.Fatal("native maximum-mana damage", m.MP, mana)
			}
			if len(s.traps) != 0 {
				t.Fatal("extra mana trap")
			}
		})
	}
}
