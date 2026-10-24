package scene

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// This matrix uses the authored poison-trap and burn producers, native cast
// admission and the actual periodic consumer. It is not a PostgreSQL-loader
// assertion. All three powers occupy distinct eligible level-100 bases.
func TestLegendaryUniversalRereviewNativePeriodicTriple(t *testing.T) {
	for _, tc := range []struct {
		name   string
		race   domain.Race
		skill  domain.SkillID
		status domain.StatusID
		powers []string
	}{
		{"poison", domain.Assassin, 10214, domain.StatusPoison, []string{"TY-A18", "TY-A22", "TY-A28"}},
		{"burn", domain.Warlock, 10421, 1036, []string{"TY-A27", "TY-A22", "TY-A29"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var baseDamage, baseDuration int32
			for _, enhanced := range []bool{false, true} {
				t.Run(fmt.Sprintf("enhanced=%v", enhanced), func(t *testing.T) {
					var powers []string
					if enhanced {
						powers = tc.powers
					}
					s, p, m := nativeRepairFixture(t, tc.race, domain.Learned{tc.skill: 1}, powers...)
					s.rng = rand.New(rand.NewSource(1))
					p.Stats.CritRate, p.Stats.MCritRate = 10000, 10000
					cmd := UseSkill{Skill: tc.skill, Target: m.ID}
					if tc.race == domain.Assassin {
						cmd.Target, cmd.At = 0, &m.Pos
					}
					assassinRegressionCast(t, s, p, cmd)
					if tc.race == domain.Assassin {
						for _, tr := range s.traps {
							s.triggerAssassinTrap(tr, false)
						}
					}
					var active domain.Active
					m.Status.EachEffective(func(a domain.Active) {
						if a.Def.ID == tc.status {
							active = a
						}
					})
					if active.Source != p.ID || active.Def.IntervalSec <= 0 || active.Def.LegendaryPowerID != "" {
						t.Fatalf("native periodic producer did not commit its own status: %+v", active)
					}
					if enhanced && (!active.Def.LegendaryModifiers.PeriodicCanCrit || active.Def.LegendaryModifiers.PeriodicCritChanceBP != 10000) {
						t.Fatalf("source critical snapshot not preserved: %+v", active.Def.LegendaryModifiers)
					}
					tick := func() int32 {
						s.outbox = nil
						before := m.HP
						s.tickStatus(m, active)
						n := 0
						for _, out := range s.outbox {
							if e, ok := out.ev.(event.DamageDealt); ok && e.Dst == m.ID {
								n++
								if e.Origin != domain.DamageDOTOrigin || e.Flag.Has(event.DamageCrit) != enhanced {
									t.Fatalf("wrong periodic provenance: %+v", e)
								}
							}
						}
						if n != 1 {
							t.Fatalf("periodic consumer emitted %d damage events", n)
						}
						return before - m.HP
					}
					damage := tick()
					if !enhanced {
						baseDamage, baseDuration = damage, active.Def.DurationSec
						if baseDamage <= 0 {
							t.Fatal("native periodic damage missing")
						}
						return
					}
					if damage != baseDamage*4 || active.Def.LegendaryDurationMS != baseDuration*1500 {
						t.Fatalf("native triple damage=%d duration=%dms want=%d/%dms", damage, active.Def.LegendaryDurationMS, baseDamage*4, baseDuration*1500)
					}
					// Removing equipment and changing live crit cannot rewrite an existing DOT.
					p.Player.Worn = domain.NewEquipSet()
					p.Stats.CritRate, p.Stats.MCritRate = 0, 0
					s.invalidateCombatAttributes(p)
					if got := tick(); got != damage {
						t.Fatalf("existing DOT borrowed live equipment: %d want %d", got, damage)
					}
				})
			}
		})
	}
}

func TestLegendaryUniversalRereviewNativeShieldTripleAndRecipient(t *testing.T) {
	var baseShield int32
	for _, enhanced := range []bool{false, true} {
		t.Run(fmt.Sprintf("enhanced=%v", enhanced), func(t *testing.T) {
			var powers []string
			if enhanced {
				powers = []string{"TY-D16", "TY-U13", "TY-U03"}
			}
			s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10320: 1}, powers...)
			ally := nativeRepairAlly(t, s, p, 2, 140)
			ally.Player.Char.Level = 100
			if enhanced {
				warriorEquip(t, s, ally, "TY-D13", domain.SlotBody)
			}
			if enhanced && len(domain.ActiveLegendaryPowers(s.functionalWorn(ally), s.itemDef, ally.Player.Char)) != 1 {
				t.Fatal("recipient power inactive")
			}
			native, _ := s.skills.Get(10320, 1)
			before := p.MP
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10320, Target: ally.ID})
			shield := universalShieldValue(ally)
			if shield <= 0 {
				t.Fatal("native ally light shield not committed")
			}
			if !enhanced {
				baseShield = shield
				return
			}
			if shield != baseShield*2 {
				t.Fatalf("source 60%% and receiver 40%% must add once: shield=%d want%d", shield, baseShield*2)
			}
			wantCost := (native.MPCost + 1) / 2
			if before-p.MP != wantCost || d.MPCost != wantCost {
				t.Fatalf("native shield 20%%+30%% reduction paid=%d prepared=%d want%d", before-p.MP, d.MPCost, wantCost)
			}
			// A recipient bonus does not retroactively increase existing absorption.
			ally.Player.Worn = domain.NewEquipSet()
			s.invalidateCombatAttributes(ally)
			if got := universalShieldValue(ally); got != shield {
				t.Fatalf("unrelated receiver unequip rewrote shield: %d want%d", got, shield)
			}
		})
	}
}

func TestLegendaryUniversalRereviewNativeBasicReachTriple(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{}, "TY-A01", "TY-A03", "TY-A35")
	weapon := p.Player.Worn.At(domain.SlotWeapon)
	item := s.items[weapon.Item]
	item.Equip.Appearance = domain.EquipAppearance{Part: domain.AppearanceWeaponR, Known: true, AttackKnown: true, AtkDist: 100}
	s.items[item.ID] = item
	s.refreshAppearance(p)
	if p.Look.AtkDist != 150 || p.UsesRangedWeapon() {
		t.Fatal("reach must not change native melee identity", p.Look.AtkDist)
	}
	inside := putMonster(s, 901, 100000, domain.Stats{}, 300, 100)
	outside := putMonster(s, 902, 100000, domain.Stats{}, 326, 100)
	s.outbox = nil
	s.onAttack(Attack{ID: p.ID, Target: m.ID, Blow: combat.Blow{GuaranteedHit: true}})
	if len(s.attacks) != 1 {
		t.Fatal("native attack admission failed")
	}
	s.stepCombatAt(s.now().Add(time.Second), false)
	if m.HP != 99900 || inside.HP != 99930 || outside.HP != 100000 {
		t.Fatalf("reach150 then fan+50%% must hit side200 but not226: hp=%d/%d/%d", m.HP, inside.HP, outside.HP)
	}
}
