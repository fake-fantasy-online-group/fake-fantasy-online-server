package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
	"time"
)

// Every row equips one power, or its unequipped control, and observes an actual
// consumer. This is contract-case coverage, not Go statement coverage.
func warriorCoverageFixture(t *testing.T, id string, equipped bool) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, p, m := legendaryMeleeFixture(t, id)
	if !equipped {
		p.Player.Worn = &domain.EquipSet{}
	}
	p.MP, p.MaxMP = 10000, 10000
	return s, p, m
}
func warriorCoverageDef(skill domain.SkillID) domain.SkillDef {
	return domain.SkillDef{ID: skill, Level: 1, Prof: 1, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 1000, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
}
func warriorCoverageNativeCast(t *testing.T, s *Scene, p *entity.Entity, d domain.SkillDef, target domain.EntityID) domain.SkillDef {
	t.Helper()
	s.skills = skillTable(d)
	p.Player.Char.Skills[d.ID] = 1
	return meleeAuditCast(t, s, p, d.ID, target)
}
func warriorCoverageLoss(t *testing.T, s *Scene, p, m *entity.Entity, d domain.SkillDef, want int32) {
	t.Helper()
	before := m.HP
	warriorTestCast(t, s, p, m, d)
	if got := before - m.HP; got != want {
		t.Fatalf("actual HP loss=%d want=%d", got, want)
	}
}

func TestLegendaryWarriorSoloDamageCoverage(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skills []domain.SkillID
		want   int32
	}{
		{"ZS-A01", []domain.SkillID{10002}, 180}, {"ZS-A02", []domain.SkillID{10002}, 115},
		{"ZS-A04", []domain.SkillID{10005}, 115}, {"ZS-A05", []domain.SkillID{10005}, 160},
		{"ZS-A06", []domain.SkillID{10005}, 210}, {"ZS-A07", []domain.SkillID{10005, 10009}, 150},
		{"ZS-A09", []domain.SkillID{10013}, 125}, {"ZS-A11", []domain.SkillID{10017}, 160},
		{"ZS-A12", []domain.SkillID{10017}, 110}, {"ZS-A13", []domain.SkillID{10006}, 145},
		{"ZS-A15", []domain.SkillID{10006}, 160}, {"ZS-A16", []domain.SkillID{10003}, 115},
		{"ZS-A17", []domain.SkillID{10003}, 130}, {"ZS-A18", []domain.SkillID{10021, 10015}, 190},
		{"ZS-A19", []domain.SkillID{10021}, 170}, {"ZS-A20", []domain.SkillID{10015}, 340},
		{"ZS-A21", []domain.SkillID{10015}, 200}, {"ZS-A22", []domain.SkillID{10022}, 120},
		{"ZS-A23", []domain.SkillID{10022}, 340}, {"ZS-A24", []domain.SkillID{10023}, 185},
		{"ZS-A25", []domain.SkillID{10023, 10022}, 135}, {"ZS-A27", []domain.SkillID{10027, 10009}, 180},
		{"ZS-A28", []domain.SkillID{10002, 10005, 10013, 10017, 10006, 10003, 10021, 10015, 10022, 10023, 10027}, 140},
		{"ZS-A29", []domain.SkillID{10002, 10005, 10013, 10017, 10006, 10003, 10021, 10015, 10022, 10023, 10027}, 280},
		{"ZS-A30", []domain.SkillID{10017, 10002}, 165},
	} {
		t.Run(tc.id, func(t *testing.T) {
			for _, skill := range tc.skills {
				t.Run(fmt.Sprint(skill), func(t *testing.T) {
					for _, equipped := range []bool{false, true} {
						s, p, m := warriorCoverageFixture(t, tc.id, equipped)
						p.Player.Char.Skills = domain.Learned{skill: 1}
						want := int32(100)
						if equipped {
							want = tc.want
						}
						warriorCoverageLoss(t, s, p, m, warriorCoverageDef(skill), want)
						if m.Status != nil && m.Status.Has(domain.StatusSlow) {
							t.Fatal("plain bonus invented slow")
						}
					}
				})
			}
		})
	}
}
func TestLegendaryWarriorSoloIncomingCoverage(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"ZS-D02", 84}, {"ZS-D03", 50}, {"ZS-D04", 40}, {"ZS-D05", 60}, {"ZS-D06", 65}, {"ZS-D07", 55}, {"ZS-D08", 70}, {"ZS-D10", 75}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, mode := range []string{"equipped", "unequipped", "dot", "condition-false"} {
				t.Run(mode, func(t *testing.T) {
					s, p, m := warriorCoverageFixture(t, tc.id, mode != "unequipped")
					p.Look.AtkDist = 100
					m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100}
					magic := tc.id == "ZS-D04"
					if tc.id == "ZS-D03" {
						m.Status = domain.NewStatusSet()
						source := p.ID
						if mode == "condition-false" {
							source = 999
						}
						m.Status.Apply(domain.StatusDef{ID: 190909, SourceSkillID: 10009, DurationSec: 10, Affixes: []domain.Affix{{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: -30}}}, s.tick, source)
					}
					if mode == "condition-false" && tc.id == "ZS-D06" {
						m.Pos.X = p.Pos.X + 151
					}
					if mode == "condition-false" && tc.id == "ZS-D04" {
						magic = false
					}
					before := p.HP
					if mode == "dot" {
						s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 190910, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -100}}}})
					} else {
						s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, Magic: magic, GuaranteedHit: true})
					}
					want := tc.want
					if mode == "unequipped" || mode == "dot" || mode == "condition-false" && (tc.id == "ZS-D03" || tc.id == "ZS-D04" || tc.id == "ZS-D06") {
						want = 100
					}
					if before-p.HP != want {
						t.Fatalf("actual received=%d want=%d", before-p.HP, want)
					}
				})
			}
		})
	}
}
func TestLegendaryWarriorSoloResourceConsumers(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		mana  int32
	}{{"ZS-U01", 10002, 60}, {"ZS-U02", 10013, 40}, {"ZS-U04", 10028, 70}, {"ZS-U05", 10006, 55}, {"ZS-U06", 10002, 40}, {"ZS-U06", 10005, 40}, {"ZS-U06", 10023, 40}, {"ZS-U08", 10005, 70}, {"ZS-U08", 10028, 70}, {"ZS-U10", 10028, 0}, {"ZS-U10", 10023, 40}} {
		t.Run(tc.id+"/"+fmt.Sprint(tc.skill), func(t *testing.T) {
			for _, equipped := range []bool{false, true} {
				s, p, m := warriorCoverageFixture(t, tc.id, equipped)
				d := warriorCoverageDef(tc.skill)
				d.MPCost = 100

				mp, hp, enemy := p.MP, p.HP, m.HP
				warriorCoverageNativeCast(t, s, p, d, m.ID)
				want := int32(100)
				if equipped {
					want = tc.mana
				}
				if mp-p.MP != want || hp-p.HP != 0 || enemy-m.HP != 100 {
					t.Fatalf("actual costs/hit MP=%d HP=%d hit=%d", mp-p.MP, hp-p.HP, enemy-m.HP)
				}
			}
		})
	}
}
func TestLegendaryWarriorSoloDrainConsumers(t *testing.T) {
	for _, tc := range []struct {
		id            string
		drain, damage int32
	}{{"ZS-A10", 60, 220}, {"ZS-U02", 96, 100}, {"ZS-D04", 75, 100}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, equipped := range []bool{false, true} {
				for _, mp := range []int32{0, 20, 1000} {
					s, p, m := warriorCoverageFixture(t, tc.id, equipped)
					m.MP, m.MaxMP = mp, 1000
					d := warriorCoverageDef(10013)
					d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectDamageMP, SourceAttackPct: 60}}
					hp := m.HP
					warriorTestCast(t, s, p, m, d)
					drain, damage := int32(60), int32(100)
					if equipped {
						drain, damage = tc.drain, tc.damage
					}
					if m.MP != max(int32(0), mp-drain) || hp-m.HP != damage {
						t.Fatalf("mp=%d HP loss=%d want drain=%d damage=%d", m.MP, hp-m.HP, drain, damage)
					}
				}
			}
		})
	}
}
func TestLegendaryWarriorSoloGeometryConsumers(t *testing.T) {
	for _, tc := range []struct {
		id                    string
		skill                 domain.SkillID
		shape                 domain.SkillAreaShape
		length, width, radius int32
		x, y                  float64
		want                  int32
	}{
		{"ZS-A04", 10005, domain.SkillAreaRectangle, 100, 100, 0, 210, 180, 115}, {"ZS-A13", 10006, domain.SkillAreaCircle, 0, 0, 100, 215, 100, 145},
		{"ZS-A16", 10003, domain.SkillAreaRectangle, 0, 0, 0, 260, 150, 115}, {"ZS-A20", 10015, domain.SkillAreaRectangle, 0, 0, 0, 260, 150, 340},
		{"ZS-D08", 10022, domain.SkillAreaCircle, 0, 0, 100, 215, 100, 100}, {"ZS-U04", 10028, domain.SkillAreaCircle, 0, 0, 100, 260, 100, 100},
		{"ZS-U05", 10006, domain.SkillAreaCircle, 0, 0, 100, 240, 100, 100}, {"ZS-U08", 10005, domain.SkillAreaRectangle, 100, 100, 0, 260, 180, 100},
		{"ZS-U08", 10028, domain.SkillAreaCircle, 0, 0, 100, 260, 100, 100}, {"ZS-U10", 10028, domain.SkillAreaCircle, 0, 0, 100, 220, 100, 100},
	} {
		t.Run(tc.id+"/"+fmt.Sprint(tc.skill), func(t *testing.T) {
			for _, equipped := range []bool{false, true} {
				s, p, m := warriorCoverageFixture(t, tc.id, equipped)
				side := putMonster(s, 901, 10000, domain.Stats{}, tc.x, tc.y)
				d := warriorCoverageDef(tc.skill)
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: tc.shape, Center: domain.SkillAreaCenterCaster, Length: tc.length, Width: tc.width, Radius: tc.radius}
				before := side.HP
				warriorTestCast(t, s, p, m, d)
				want := int32(0)
				if equipped {
					want = tc.want
				}
				if before-side.HP != want {
					t.Fatalf("expanded target HP loss=%d want=%d pos=%+v", before-side.HP, want, side.Pos)
				}
			}
		})
	}
}
func TestLegendaryWarriorSoloRangeAndTimingConsumers(t *testing.T) {
	t.Run("ZS-U03", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := warriorCoverageFixture(t, "ZS-U03", equipped)
			m.Pos.X = 240
			s.aoi.Move(m, m.Pos)
			d := warriorCoverageDef(10017)
			d.Dist = 100
			s.skills = skillTable(d)
			s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
			if (len(s.casts) > 0) != equipped {
				t.Fatal("extended distance admission")
			}
			if equipped {
				s.stepCasts()
				if m.HP != 99900 {
					t.Fatal("ranged hit did not settle")
				}
			}
			st := domain.Stats{MoveSpeed: 100}
			s.legendaryPassiveStats(p, &st)
			want := int32(100)
			if equipped {
				want = 130
			}
			if st.MoveSpeed != want {
				t.Fatal("movement consumer", st.MoveSpeed)
			}
		}
	})
	t.Run("ZS-U09", func(t *testing.T) {
		for _, skill := range []domain.SkillID{10017, 10027} {
			for _, equipped := range []bool{false, true} {
				s, p, m := warriorCoverageFixture(t, "ZS-U09", equipped)
				d := warriorCoverageDef(skill)
				d.PrepareMS = 1000
				d.CooldownMS = 10000
				s.skills = skillTable(d)
				now := s.now()
				s.now = func() time.Time { return now }
				s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: m.ID})
				if len(s.casts) != 1 {
					t.Fatal("cast rejected")
				}
				cooldown := 10000
				if equipped {
					cooldown = 5000
				}
				if p.Player.SkillReadyAt(skill, s.tick+domain.Ticks(cooldown)-1) || !p.Player.SkillReadyAt(skill, s.tick+domain.Ticks(cooldown)) {
					t.Fatal("actual cooldown deadline incorrect")
				}
				prepare := time.Second
				if equipped && skill == 10017 {
					prepare /= 2
				}
				if got := s.casts[0].releaseAtWall.Sub(now); got != prepare {
					t.Fatalf("release delay %s want %s", got, prepare)
				}
				s.stepCastsAt(now.Add(prepare), false)
				if len(s.casts) > 0 {
					s.stepCastsAt(now.Add(2*time.Second), false)
				}
				s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: m.ID})
				if len(s.casts) != 0 {
					t.Fatal("active cooldown bypassed")
				}
			}
		}
	})
	t.Run("ZS-U07", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := warriorCoverageFixture(t, "ZS-U07", equipped)
			p.MP = 0
			p.MaxMP = 1000
			s.onAttack(Attack{ID: p.ID, Target: m.ID})
			s.stepCombat()
			want := int32(0)
			if equipped {
				want = 50
			}
			if p.MP != want {
				t.Fatal("actual basic mana", p.MP)
			}
			before := p.MP
			warriorTestCast(t, s, p, m, warriorCoverageDef(10002))
			if p.MP != before {
				t.Fatal("active restored basic-only mana")
			}
		}
	})
}

func TestLegendaryWarriorSoloSpecialCoverage(t *testing.T) {
	t.Run("ZS-A03", TestLegendaryWarriorChainAndBasicCoefficient)
	t.Run("ZS-A14", func(t *testing.T) {
		t.Run("same-roll-defense", TestLegendaryWarriorQuakeExactReleaseSnapshotAndDefenses)
		t.Run("empty-cast", TestLegendaryWarriorEmptyQuakeSnapshotsOnce)
		t.Run("accepted-source", TestLegendaryWarriorQuakeUsesAcceptedAttackSnapshot)
		t.Run("overlap-lifetime", TestLegendaryWarriorQuakeOverlapAndLifetime)
	})
	t.Run("ZS-A08", func(t *testing.T) {
		for _, learned := range []bool{false, true} {
			skills := domain.Learned{10009: 1}
			if learned {
				skills[10005] = 5
			}
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, skills, "ZS-A08")
			m.Stats.Def = 100
			s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
			side := putMonster(s, 901, 10000, domain.Stats{Def: 100}, 160, 110)
			s.defs.(spawn.MapDefs)[side.Monster.TypeID] = domain.MonsterDef{ID: side.Monster.TypeID, Stats: side.Stats}
			before := side.HP
			native, _ := s.skills.Get(10009, 1)
			nativeStatus, _ := s.resolveSkillStatusDef(native, native.Statuses[0], 0)
			meleeAuditCast(t, s, p, 10009, m.ID)
			actualStatus := verifiedActive(t, m, 10009)
			if actualStatus.Def.DurationSec != nativeStatus.DurationSec {
				t.Fatal("armor extension changed original duration")
			}
			if before == side.HP || !side.Status.Has(10009) || side.Stats.Def != 55 || m.Stats.Def != 55 {
				t.Fatalf("borrowed rectangle/native armor not consumed: hp=%d def=%d/%d", side.HP, m.Stats.Def, side.Stats.Def)
			}
			if !learned && p.Player.Char.Skills[10005] != 0 {
				t.Fatal("borrow learned the donor")
			}
		}
	})
	t.Run("ZS-A26", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			for _, immune := range []bool{false, true} {
				powers := []string{}
				if equipped {
					powers = []string{"ZS-A26"}
				}
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10016: 1}, powers...)
				if immune {
					s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{10016: {}}}}
				}
				before := m.HP
				raw, _ := s.skills.Get(10016, 1)
				meleeAuditCast(t, s, p, 10016, m.ID)
				want := int32(0)
				if equipped {
					want = 240
				}
				if before-m.HP != want {
					t.Fatalf("petrify damage=%d want=%d", before-m.HP, want)
				}
				if !immune && !m.Status.Has(domain.StatusPetrify) {
					t.Fatal("new petrification broken")
				}
				if immune && m.Status.Has(domain.StatusPetrify) {
					t.Fatal("control immunity bypassed")
				}
				cd := raw.CooldownMS
				if equipped {
					cd /= 2
				}
				if !p.Player.SkillReadyAt(10016, s.tick+domain.Ticks(int(cd))) {
					t.Fatal("cooldown did not halve")
				}
			}
		}
	})
	for _, id := range []string{"ZS-D01", "ZS-D09"} {
		t.Run(id, func(t *testing.T) {
			for _, equipped := range []bool{false, true} {
				s, p, m := warriorCoverageFixture(t, id, equipped)
				skill := domain.SkillID(10028)
				if id == "ZS-D09" {
					skill = 10016
				}
				d := warriorCoverageDef(skill)
				d.CooldownMS = 1000
				maxHP := p.MaxHP
				warriorCoverageNativeCast(t, s, p, d, m.ID)
				want := int32(0)
				if equipped {
					want = maxHP / 4
				}
				if universalShieldValue(p) != want {
					t.Fatalf("release shield=%d want=%d", universalShieldValue(p), want)
				}
				if !equipped {
					continue
				}
				hp := p.HP
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true})
				if p.HP != hp || universalShieldValue(p) != want-1000 {
					t.Fatal("shield did not absorb actual damage")
				}
				want = p.MaxHP / 4
				s.legendaryCastReleased(p, d, m.ID, []domain.EntityID{m.ID})
				if universalShieldValue(p) != want {
					t.Fatal("shield refresh stacked rather than refilled")
				}
				p.Status.Expire(s.tick + domain.Ticks(999999))
				if universalShieldValue(p) != want {
					t.Fatal("shield acquired expiration")
				}
				p.Player.Worn = &domain.EquipSet{}
				s.pruneLegendaryOwner(p)
				if universalShieldValue(p) != 0 {
					t.Fatal("unequipped shield retained")
				}
			}
		})
	}
	t.Run("ZS-D10", func(t *testing.T) {
		TestLegendaryWarriorEquippedSlowCleansing(t)
		s, p, m := warriorCoverageFixture(t, "ZS-D10", true)
		slow := domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}}}
		if s.applyStatusDef(p, slow, m.ID) {
			t.Fatal("new slow applied")
		}
		p.Player.Worn = &domain.EquipSet{}
		if !s.applyStatusDef(p, slow, m.ID) {
			t.Fatal("unequipped immunity retained")
		}
	})
	t.Run("ZS-U04", func(t *testing.T) {
		s, p, _ := warriorCoverageFixture(t, "ZS-U04", true)
		st := domain.Stats{MoveSpeed: 100}
		s.legendaryPassiveStats(p, &st)
		if st.MoveSpeed != 130 {
			t.Fatal(st.MoveSpeed)
		}
		p.Player.Worn = &domain.EquipSet{}
		st.MoveSpeed = 100
		s.legendaryPassiveStats(p, &st)
		if st.MoveSpeed != 100 {
			t.Fatal(st.MoveSpeed)
		}
	})
}

func TestLegendaryWarriorSoloTargetAndStatusCoverage(t *testing.T) {
	t.Run("ZS-A02", TestLegendaryWarriorChainCircleTargetCapAndNativeControls)
	t.Run("ZS-A12", func(t *testing.T) {
		TestLegendaryWarriorFlyMissDoesNotHitSides(t)
		s, p, m := warriorCoverageFixture(t, "ZS-A12", true)
		p.Look.AtkDist = 100
		m.Pos.X = 300
		s.aoi.Move(m, m.Pos)
		for i := 0; i < 8; i++ {
			putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 310+float64(i), 100)
		}
		far := putMonster(s, 990, 10000, domain.Stats{}, 451, 100)
		ids := warriorTestCast(t, s, p, m, warriorCoverageDef(10017))
		if len(ids) != 6 || m.MaxHP-m.HP != 110 || far.HP != 10000 {
			t.Fatal("landing circle cap/radius/primary", ids, m.HP, far.HP)
		}
		for _, id := range ids[1:] {
			if s.entities[id].MaxHP-s.entities[id].HP != 110 {
				t.Fatal("side hit duplicated", id)
			}
		}
	})
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		want  int32
	}{{"ZS-A05", 10005, 160}, {"ZS-A15", 10006, 160}, {"ZS-A19", 10021, 170}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := warriorCoverageFixture(t, tc.id, true)
			q := putMonster(s, 901, 10000, domain.Stats{}, 150, 100)
			far := putMonster(s, 902, 10000, domain.Stats{}, 201, 100)
			d := warriorCoverageDef(tc.skill)
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			st := domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 5, Control: domain.ControlStun, EffectHitBP: 1000000}
			s.statuses = statusTable(st)
			d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			ids := warriorTestCast(t, s, p, m, d)
			if len(ids) != 2 || m.MaxHP-m.HP != tc.want || q.MaxHP-q.HP != tc.want || far.HP != 10000 || !q.Status.Has(st.ID) {
				t.Fatal("all-target damage/native geometry/control", ids, m.HP, q.HP, far.HP)
			}
		})
	}
	for _, id := range []string{"ZS-A13", "ZS-U05"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := warriorCoverageFixture(t, id, true)
			d := warriorCoverageDef(10006)
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 90}
			d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectKnockback, KnockbackDistance: 40, ChanceBP: 10000, ChanceExplicit: true, RequiresPreviousLanded: true}}
			before := m.Pos
			warriorTestCast(t, s, p, m, d)
			if m.Pos.X-before.X != 40 {
				t.Fatalf("native knockback changed %+v -> %+v", before, m.Pos)
			}
		})
	}
	t.Run("ZS-A09", func(t *testing.T) {
		for _, shield := range []int32{0, 10, 1000} {
			s, p, m := warriorCoverageFixture(t, "ZS-A09", true)
			m.Status = domain.NewStatusSet()
			if shield > 0 {
				m.Status.Apply(domain.StatusDef{ID: 190099, DamageShield: shield, DurationSec: 10}, s.tick, m.ID)
			}
			before := m.HP
			warriorTestCast(t, s, p, m, warriorCoverageDef(10013))
			want := int32(125)
			if shield > 0 {
				want = max(int32(0), 220-shield)
			}
			if before-m.HP != want {
				t.Fatal("shield tier or break-through loss", before-m.HP, want)
			}
		}
	})
}

func TestLegendaryWarriorSoloRemainingClauses(t *testing.T) {
	t.Run("ZS-A02", func(t *testing.T) {
		s, p, m := warriorCoverageFixture(t, "ZS-A02", true)
		p.Look.AtkDist = 100
		m.Pos.X = 300
		s.aoi.Move(m, m.Pos)
		near := putMonster(s, 901, 10000, domain.Stats{}, 229, 100)
		outside := putMonster(s, 902, 10000, domain.Stats{}, 231, 100)
		d := warriorCoverageDef(10002)
		ids := warriorTestCast(t, s, p, m, d)
		if len(ids) != 2 || ids[0] != m.ID || m.MaxHP-m.HP != 115 || near.MaxHP-near.HP != 115 || outside.HP != 10000 {
			t.Fatal("selected distant primary/circle boundary", ids)
		}
	})
	for _, id := range []string{"ZS-A06", "ZS-A18"} {
		t.Run(id, func(t *testing.T) {
			for _, primary := range []bool{false, true} {
				s, p, m := warriorCoverageFixture(t, id, true)
				m.Pos.X = 200
				s.aoi.Move(m, m.Pos)
				m.Stats.PhysResist = 20
				q := putMonster(s, 901, 10000, domain.Stats{PhysResist: 20}, 140, 100)
				selected := domain.EntityID(0)
				if primary {
					selected = m.ID
				}
				skill := domain.SkillID(10005)
				if id == "ZS-A18" {
					skill = 10021
				}
				finish := s.beginEquipmentRoot(p.ID, selected, domain.EquipmentCombatStats{})
				root := s.equipmentState().root
				root.def = warriorCoverageDef(skill)
				root.targets = []domain.EntityID{m.ID, q.ID}
				for _, e := range []*entity.Entity{m, q} {
					s.applySkill(p, e, root.def, 0, false)
				}
				finish()
				mWant, qWant := int32(80), int32(80)
				echo := int32(88)
				if id == "ZS-A18" {
					echo = 72
				}
				if primary || id == "ZS-A18" {
					mWant += echo
				} else {
					qWant += echo
				}
				if m.MaxHP-m.HP != mWant || q.MaxHP-q.HP != qWant {
					t.Fatalf("selected/farthest/nearest settled echo %d/%d want%d/%d", m.MaxHP-m.HP, q.MaxHP-q.HP, mWant, qWant)
				}
			}
		})
	}
	t.Run("ZS-A07", TestLegendaryWarriorAuditArmorImmunityAndUnlearned)
	for _, id := range []string{"ZS-A07", "ZS-A25"} {
		t.Run(id+"-armor", func(t *testing.T) {
			skills := []domain.SkillID{10005, 10009}
			if id == "ZS-A25" {
				skills = []domain.SkillID{10022, 10023}
			}
			for _, skill := range skills {
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{skill: 1, 10009: 1}, id)
				m.Stats.Def = 100
				s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
				before := m.HP
				meleeAuditCast(t, s, p, skill, m.ID)
				if m.HP == before || !m.Status.Has(10009) || m.Stats.Def != 70 {
					t.Fatal("same-strike native armor/damage", skill, m.HP, m.Stats)
				}
			}
		})
	}
	t.Run("ZS-D02", func(t *testing.T) {
		for _, skill := range []domain.SkillID{10005, 10009} {
			for _, nativeArmor := range []bool{false, true} {
				s, p, m := warriorCoverageFixture(t, "ZS-D02", true)
				m.Stats.Def = 100
				s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
				d := warriorCoverageDef(skill)
				if nativeArmor {
					st := domain.StatusDef{ID: 190934, SourceSkillID: int32(skill), Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: -40}}}
					s.statuses = statusTable(st)
					d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
				}
				warriorTestCast(t, s, p, m, d)
				want := int32(100)
				if nativeArmor {
					want = 50
				}
				if m.Stats.Def != want {
					t.Fatal("native armor strength or invented armor", m.Stats.Def)
				}
			}
		}
	})
	t.Run("ZS-A16", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			powers := []string{}
			if equipped {
				powers = []string{"ZS-A16"}
			}
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10003: 1}, powers...)
			q := putMonster(s, 901, 10000, domain.Stats{}, 150, 110)
			before := p.Player.Worn.At(domain.SlotWeapon)
			mp := p.MP
			meleeAuditCast(t, s, p, 10003, m.ID)
			after := p.Player.Worn.At(domain.SlotWeapon)
			if before.Durability-after.Durability != 5 || after.DurabilityWearRaw-before.DurabilityWearRaw != 100 || p.MP >= mp {
				t.Fatal("native whole-cast durability/mana", before, after, mp, p.MP)
			}
			if (q.HP < q.MaxHP) != equipped {
				t.Fatal("native rectangle side admission")
			}
		}
	})
	t.Run("ZS-A17", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			powers := []string{}
			if equipped {
				powers = []string{"ZS-A17"}
			}
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10003: 1}, powers...)
			raw, _ := s.skills.Get(10003, 1)
			before := p.Player.Worn.At(domain.SlotWeapon)
			mp := p.MP
			meleeAuditCast(t, s, p, 10003, m.ID)
			after := p.Player.Worn.At(domain.SlotWeapon)
			want := int32(5)
			if equipped {
				want = 0
			}
			if before.Durability-after.Durability != want || mp-p.MP != raw.MPCost {
				t.Fatalf("durability=%d mana=%d raw=%d", before.Durability-after.Durability, mp-p.MP, raw.MPCost)
			}
		}
	})
	t.Run("ZS-A20", func(t *testing.T) {
		s, p, m := warriorCoverageFixture(t, "ZS-A20", true)
		p.Stats.Hit = 0
		m.Stats.Def = 1000000
		m.Pos = domain.Pos{MapID: 7, X: 100, Y: 350}
		s.aoi.Move(m, m.Pos)
		side := putMonster(s, 901, 10000, domain.Stats{Def: 1000000}, 150, 260)
		ids := warriorTestCast(t, s, p, m, warriorCoverageDef(10015))
		if len(ids) != 2 || m.MaxHP-m.HP != 340 || side.MaxHP-side.HP != 340 {
			t.Fatal("guaranteed hit/preserved primary/rectangle", ids)
		}
	})
	t.Run("ZS-A21", func(t *testing.T) {
		TestLegendaryWarriorHeartEyeStandaloneAndUnlearned(t)
		s, p, m := warriorCoverageFixture(t, "ZS-A21", true)
		s.skills = skillTable(domain.SkillDef{ID: 10015, Level: 1, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 165, GuaranteedHit: true}})
		p.MP = 0
		s.onAttack(Attack{ID: p.ID, Target: m.ID})
		s.stepCombat()
		if p.MP != 0 || m.MaxHP-m.HP != 165 || len(s.casts) != 0 {
			t.Fatal("basic replacement is not a free native swing")
		}
		before := m.HP
		s.onAttack(Attack{ID: p.ID, Target: m.ID})
		s.stepCombat()
		if m.HP != before {
			t.Fatal("basic interval changed")
		}
	})
	t.Run("ZS-A22", func(t *testing.T) {
		for _, selected := range []bool{false, true} {
			s, p, m := warriorCoverageFixture(t, "ZS-A22", true)
			p.Look.AtkDist = 100
			m.Pos.X = 390
			s.aoi.Move(m, m.Pos)
			q := putMonster(s, 901, 10000, domain.Stats{}, 410, 100)
			self := putMonster(s, 902, 10000, domain.Stats{}, 120, 100)
			d := warriorCoverageDef(10022)
			d.Dist = 100
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			target := domain.EntityID(0)
			if selected {
				target = m.ID
			}
			warriorCoverageNativeCast(t, s, p, d, target)
			if selected {
				if m.MaxHP-m.HP != 120 || q.MaxHP-q.HP != 120 || self.HP != 10000 {
					t.Fatal("selected target-centered release")
				}
			} else if self.MaxHP-self.HP != 120 || q.HP != 10000 {
				t.Fatal("no target lost self-centered release")
			}
		}
	})
	t.Run("ZS-A23", func(t *testing.T) {
		s, p, m := warriorCoverageFixture(t, "ZS-A23", true)
		m.HP = 50000
		q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
		d := warriorCoverageDef(10022)
		d.Kind = domain.SkillArea
		d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
		before := m.HP
		warriorCoverageNativeCast(t, s, p, d, 0)
		if before-m.HP != 180 || q.MaxHP-q.HP != 340 {
			t.Fatal("highest remaining ratio fallback/once", m.HP, q.HP)
		}
	})
	t.Run("ZS-A24", func(t *testing.T) {
		for _, learned := range []bool{false, true} {
			s, p, m := warriorCoverageFixture(t, "ZS-A24", true)
			p.Player.Char.Skills = domain.Learned{10023: 1}
			if learned {
				p.Player.Char.Skills[10022] = 1
			}
			donor := warriorCoverageDef(10022)
			donor.Kind = domain.SkillArea
			donor.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 150}
			s.skills = skillTable(donor)
			side := putMonster(s, 901, 10000, domain.Stats{}, 230, 100)
			d := warriorCoverageDef(10023)
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			warriorTestCast(t, s, p, m, d)
			want := int32(0)
			if learned {
				want = 185
			}
			if side.MaxHP-side.HP != want {
				t.Fatal("learned-only geometry borrowing", side.HP)
			}
		}
	})
	t.Run("ZS-A28", func(t *testing.T) {
		TestLegendaryWarriorPullStopsAtWallsAndBodies(t)
		for _, immune := range []bool{false, true} {
			s, p, m := warriorCoverageFixture(t, "ZS-A28", true)
			m.Pos.X = 260
			s.aoi.Move(m, m.Pos)
			if immune {
				m.Status = domain.NewStatusSet()
				m.Status.Apply(domain.StatusDef{ID: 190935, DurationSec: 10, SuppressHarmful: true}, s.tick, m.ID)
			}
			d := warriorCoverageDef(10028)
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			d.Effect = domain.SkillEffect{Kind: domain.EffectTaunt, TauntPower: 100}
			warriorCoverageNativeCast(t, s, p, d, 0)
			if !immune && (m.Pos.X >= 260 || m.Monster.Target != p.ID || m.Monster.TauntPower != 200) {
				t.Fatal("native radius/pull/taunt", m.Pos, m.Monster)
			}
			if immune && m.Pos.X != 260 {
				t.Fatal("immune target moved")
			}
		}
	})
	t.Run("ZS-A29", func(t *testing.T) {
		s, p, m := warriorCoverageFixture(t, "ZS-A29", true)
		before := m.HP
		s.onAttack(Attack{ID: p.ID, Target: m.ID})
		s.stepCombat()
		if before-m.HP != 100 {
			t.Fatal("active-only flat leaked into basic")
		}
		before = m.HP
		s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 10005, SkillPct: 100, GuaranteedHit: true, Origin: domain.DamageProcOrigin, ProcDepth: 1})
		if before-m.HP != 100 {
			t.Fatal("active-only flat leaked into equipment payload")
		}
	})
	t.Run("ZS-A30", func(t *testing.T) {
		for _, learned := range []bool{false, true} {
			skills := domain.Learned{10002: 1}
			if learned {
				skills[10017] = 5
			}
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, skills, "ZS-A30")
			donor, _ := s.skills.Get(10017, 1)
			m.Pos.X = p.Pos.X + float64(donor.Dist) - 1
			s.aoi.Move(m, m.Pos)
			before := m.HP
			meleeAuditCast(t, s, p, 10002, m.ID)
			if m.HP == before {
				t.Fatal("borrowed reach not consumed")
			}
			if !learned && p.Player.Char.Skills[10017] != 0 {
				t.Fatal("borrow learned donor")
			}
		}
	})
}

func TestLegendaryWarriorSoloMovementOutput(t *testing.T) {
	for _, id := range []string{"ZS-U03", "ZS-U04"} {
		t.Run(id, func(t *testing.T) { meleeCoverageMovementOutput(t, id, 130) })
	}
}
func meleeCoverageMovementOutput(t *testing.T, id string, pct int32) {
	t.Helper()
	s, p, _ := legendaryMeleeFixture(t, id)
	worn := p.Player.Worn
	p.Player.Worn = &domain.EquipSet{}
	s.refreshStats(p)
	basePanel := s.attributeSnapshot(p).Current[9]
	basePX := s.playerMoveSpeedPX(p)
	if basePanel <= 0 || basePX <= 0 {
		t.Fatal("invalid movement baseline")
	}
	p.Player.Worn = worn
	s.refreshStats(p)
	panel := s.attributeSnapshot(p)
	if panel.Current[9] != basePanel*pct/100 || s.playerMoveSpeedPX(p) != basePX*pct/100 {
		t.Fatalf("real movement output panel=%d px=%d base=%d/%d", panel.Current[9], s.playerMoveSpeedPX(p), basePanel, basePX)
	}
	p.Player.Worn = &domain.EquipSet{}
	s.refreshStats(p)
	if s.attributeSnapshot(p).Current[9] != basePanel || s.playerMoveSpeedPX(p) != basePX {
		t.Fatal("unequipped speed persisted in movement output")
	}
}

// Combinations have their own registry and are never counted in the fifty
// single-power denominator. nativeRepairEquip allocates only legal slots.
func TestLegendaryWarriorCoverageCombinations(t *testing.T) {
	t.Run("warcry-highest-radius", func(t *testing.T) {
		for _, utility := range []string{"ZS-U04", "ZS-U08", "ZS-U10"} {
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10028: 1}, utility, "ZS-A28")
			m.Pos.X = 269
			s.aoi.Move(m, m.Pos)
			outside := putMonster(s, 901, 10000, domain.Stats{}, 271, 100)
			d := meleeAuditCast(t, s, p, 10028, 0)
			if d.Area.Radius != 170 || m.Monster.Target != p.ID || outside.Monster.Target != 0 {
				t.Fatal("warcry max-only radius consumer", utility, d.Area.Radius)
			}
		}
	})
	t.Run("petrify-shield-cooldown", func(t *testing.T) {
		s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10016: 1}, "ZS-A26", "ZS-D09")
		raw, _ := s.skills.Get(10016, 1)
		before, maxHP := m.HP, p.MaxHP
		s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{10016: {}}}}
		d := meleeAuditCast(t, s, p, 10016, m.ID)
		if before-m.HP != 240 || universalShieldValue(p) != maxHP/4 || d.CooldownMS != raw.CooldownMS/2 || m.Status.Has(domain.StatusPetrify) {
			t.Fatal("immune target damage/shield/one cooldown", m.HP, universalShieldValue(p), d.CooldownMS)
		}
	})
	t.Run("utility-preserves-set-life-cost", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			powers := []string{}
			if equipped {
				powers = []string{"ZS-U06"}
			}
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10003: 1}, powers...)
			meleeAuditEquipSet(t, s, p, 5)
			hp, mp, enemy := p.HP, p.MP, m.HP
			meleeAuditCast(t, s, p, 10003, m.ID)
			if p.MP != mp || p.HP != hp-classSetBloodCost(hp)+(enemy-m.HP)/5 {
				t.Fatal("mana reducer changed set life cost", equipped, p.HP, p.MP)
			}
		}
	})
}

func TestLegendaryWarriorSoloNativeEdgeCoverage(t *testing.T) {
	t.Run("ZS-D09", func(t *testing.T) {
		s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10016: 1}, "ZS-D09")
		s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{10016: {}}}}
		raw, _ := s.skills.Get(10016, 1)
		maxHP := p.MaxHP
		d := meleeAuditCast(t, s, p, 10016, m.ID)
		if universalShieldValue(p) != maxHP/4 || d.CooldownMS != raw.CooldownMS/2 || m.Status.Has(domain.StatusPetrify) {
			t.Fatal("immune native cast lost standalone shield/cooldown")
		}
	})
	t.Run("ZS-A27", func(t *testing.T) {
		for _, learned := range []bool{false, true} {
			s, p, m := warriorCoverageFixture(t, "ZS-A27", true)
			p.Player.Char.Skills = domain.Learned{10027: 1}
			if learned {
				p.Player.Char.Skills[10009] = 1
			}
			donor := warriorCoverageDef(10009)
			donor.Kind = domain.SkillArea
			donor.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 150}
			s.skills = skillTable(donor)
			side := putMonster(s, 901, 10000, domain.Stats{}, 230, 100)
			st := domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 5, Control: domain.ControlStun, EffectHitBP: 10000000}
			s.statuses = statusTable(st)
			d := warriorCoverageDef(10027)
			d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			warriorTestCast(t, s, p, m, d)
			if side.MaxHP-side.HP != 180 || !side.Status.Has(st.ID) || !m.Status.Has(st.ID) {
				t.Fatal("borrowed geometry lost per-target control")
			}
			if !learned && p.Player.Char.Skills[10009] != 0 {
				t.Fatal("donor learned")
			}
		}
	})
	t.Run("ZS-A30", func(t *testing.T) {
		s, p, m := warriorCoverageFixture(t, "ZS-A30", true)
		m.Pos.X = 450
		s.aoi.Move(m, m.Pos)
		chain := warriorCoverageDef(10002)
		chain.Dist = 400
		fly := warriorCoverageDef(10017)
		fly.Dist = 100
		s.skills = skillTable(chain, fly)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID})
		if len(s.casts) != 1 {
			t.Fatal("shorter donor reduced original reach")
		}
		s.stepCasts()
		if m.MaxHP-m.HP != 165 {
			t.Fatal("accepted long chain did not settle")
		}
	})
}

// Native armor status continuation caps percentage defense loss at 100%.
// Legendary magnitude may reach that cap sooner, but cannot create negative
// target defense or change the native status duration.
func TestLegendaryWarriorSoloArmorCapCoverage(t *testing.T) {
	for _, id := range []string{"ZS-A08", "ZS-D02"} {
		t.Run(id, func(t *testing.T) {
			for _, rank := range []int32{1, 5, 20} {
				t.Run(fmt.Sprint(rank), func(t *testing.T) {
					for _, equipped := range []bool{false, true} {
						powers := []string{}
						if equipped {
							powers = []string{id}
						}
						s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10009: rank}, powers...)
						m.Stats.Def = 100
						s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
						meleeAuditCast(t, s, p, 10009, m.ID)
						want := map[int32]int32{1: 70, 5: 30, 20: 0}[rank]
						if equipped {
							want = map[int32]int32{1: 55, 5: 0, 20: 0}[rank]
							if id == "ZS-D02" {
								want = map[int32]int32{1: 63, 5: 13, 20: 0}[rank]
							}
						}
						a := verifiedActive(t, m, 10009)
						if m.Stats.Def != want {
							t.Fatalf("native rank%d actual defense=%d want%d (never below zero)", rank, m.Stats.Def, want)
						}
						if a.Def.DurationSec != 15 {
							t.Fatal("native duration changed", a.Def.DurationSec)
						}
						for _, affix := range a.Def.Affixes {
							if affix.Attr == domain.AttrDef && affix.Mode == domain.ModePercent && affix.Value < -100 {
								t.Fatal("native defense-loss cap escaped", affix)
							}
						}
					}
				})
			}
		})
	}
}
func TestLegendaryWarriorCoverageArmorCapCombinations(t *testing.T) {
	for _, skill := range []domain.SkillID{10005, 10022, 10023} {
		for _, rank := range []int32{5, 20} {
			t.Run(fmt.Sprintf("skill%d/rank%d", skill, rank), func(t *testing.T) {
				power := "ZS-A07"
				if skill != 10005 {
					power = "ZS-A25"
				}
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{skill: 1, 10009: rank}, power, "ZS-A08", "ZS-D02")
				m.Stats.Def = 100
				s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
				before := m.HP
				meleeAuditCast(t, s, p, skill, m.ID)
				if m.Stats.Def != 0 || m.HP >= before {
					t.Fatal("borrowed armor cap and same-hit damage", m.Stats.Def, m.HP)
				}
				if verifiedActive(t, m, 10009).Def.DurationSec != 15 {
					t.Fatal("borrowed native duration changed")
				}
			})
		}
	}
}

// Controlled status inputs exercise the real status/stat consumer, including
// the authored exception that native continuation explicitly preserves.
func TestLegendaryWarriorSoloArmorCapBoundaries(t *testing.T) {
	for _, id := range []string{"ZS-A08", "ZS-D02"} {
		t.Run(id, func(t *testing.T) {
			for _, tc := range []struct {
				name             string
				affix            domain.Affix
				wantA08, wantD02 int32
			}{
				{"authored-below-minus100", domain.Affix{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: -120}, -20, -20},
				{"absolute-unmodified-scaling", domain.Affix{Attr: domain.AttrDef, Mode: domain.ModeAbsolute, Value: -120}, -80, -50},
				{"positive-defense-unchanged", domain.Affix{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: 20}, 120, 120},
				{"other-attribute-unchanged", domain.Affix{Attr: domain.AttrHit, Mode: domain.ModePercent, Value: -120}, -20, -20},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for _, equipped := range []bool{false, true} {
						s, p, m := warriorCoverageFixture(t, id, equipped)
						m.Stats.Def, m.Stats.Hit = 100, 100
						s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
						d := domain.StatusDef{ID: 190947, SourceSkillID: 10009, DurationSec: 15, Affixes: []domain.Affix{tc.affix}}
						if !s.applyStatusDef(m, d, p.ID) {
							t.Fatal("controlled status did not apply")
						}
						want := int32(100) + tc.affix.Value
						if equipped {
							want = tc.wantA08
							if id == "ZS-D02" {
								want = tc.wantD02
							}
						}
						got := m.Stats.Def
						if tc.affix.Attr == domain.AttrHit {
							got = m.Stats.Hit
						}
						if got != want {
							t.Fatalf("equipped=%v actual stat=%d want=%d", equipped, got, want)
						}
						if d.Affixes[0] != tc.affix || verifiedActive(t, m, d.ID).Def.DurationSec != 15 {
							t.Fatal("source definition or duration mutated")
						}
					}
				})
			}
		})
	}
}

// Def is evasion, not physical resistance. Zero and negative Def already have
// identical hit/damage behavior: the fix enforces the authored attribute bound,
// not an additional damage or hit-rate nerf. No GuaranteedHit bypass is used.
func TestLegendaryWarriorArmorDefenseConsumerBoundary(t *testing.T) {
	for _, def := range []int32{1, 0, -50, -120} {
		t.Run(fmt.Sprint(def), func(t *testing.T) {
			s, p, m := warriorCoverageFixture(t, "ZS-A08", false)
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 0}
			m.Stats = domain.Stats{Def: def, PhysResist: 20}
			before := m.HP
			ev := combat.Strike(p, m, &combat.HitTracker{}, combat.Blow{}, s.rng)
			want := int32(80)
			if def > 0 {
				want = 0
			}
			if ev.Amount != want || before-m.HP != want {
				t.Fatalf("Def%d actualHP=%d damage=%d want%d", def, before-m.HP, ev.Amount, want)
			}
		})
	}
}
