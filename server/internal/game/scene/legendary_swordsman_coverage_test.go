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
	"reflect"
	"testing"
)

func swordsmanCoverageFixture(t *testing.T, id string, equipped bool) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, p, m := legendaryMeleeFixture(t, id)
	if !equipped {
		p.Player.Worn = &domain.EquipSet{}
	}
	p.MP, p.MaxMP = 10000, 10000
	return s, p, m
}
func swordsmanCoverageDef(skill domain.SkillID) domain.SkillDef {
	d := warriorCoverageDef(skill)
	d.Prof = 2
	return d
}
func swordsmanCoverageNativeCast(t *testing.T, s *Scene, p *entity.Entity, d domain.SkillDef, target domain.EntityID) domain.SkillDef {
	t.Helper()
	s.skills = skillTable(d)
	p.Player.Char.Skills[d.ID] = 1
	return meleeAuditCast(t, s, p, d.ID, target)
}
func swordsmanCoverageRootStatus() domain.StatusDef {
	return domain.StatusDef{ID: 800111, Level: 1, DurationSec: 6, BlockMove: true, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
}

func TestLegendarySwordsmanSoloDamageCoverage(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skills []domain.SkillID
		want   int32
	}{
		{"JK-A01", []domain.SkillID{10102}, 200}, {"JK-A02", []domain.SkillID{10117}, 115}, {"JK-A03", []domain.SkillID{10102}, 260},
		{"JK-A05", []domain.SkillID{10111}, 140}, {"JK-A07", []domain.SkillID{10103}, 200}, {"JK-A08", []domain.SkillID{10103}, 190},
		{"JK-A09", []domain.SkillID{10104, 10119}, 115}, {"JK-A13", []domain.SkillID{10113}, 170}, {"JK-A14", []domain.SkillID{10113}, 150},
		{"JK-A15", []domain.SkillID{10121}, 216}, {"JK-A16", []domain.SkillID{10121}, 130}, {"JK-A19", []domain.SkillID{10102, 10122}, 125},
		{"JK-A21", []domain.SkillID{10116}, 420}, {"JK-A22", []domain.SkillID{10123}, 190}, {"JK-A24", []domain.SkillID{10126}, 180},
		{"JK-A26", []domain.SkillID{10102, 10117, 10104, 10119}, 200}, {"JK-A27", []domain.SkillID{10102, 10103, 10121}, 420},
		{"JK-A28", []domain.SkillID{10104, 10119}, 360}, {"JK-A30", []domain.SkillID{10116, 10123}, 210},
	} {
		t.Run(tc.id, func(t *testing.T) {
			for _, skill := range tc.skills {
				t.Run(fmt.Sprint(skill), func(t *testing.T) {
					for _, equipped := range []bool{false, true} {
						s, p, m := swordsmanCoverageFixture(t, tc.id, equipped)
						before := m.HP
						swordCast(t, s, p, m, swordsmanCoverageDef(skill))
						want := int32(100)
						if equipped {
							want = tc.want
						}
						if before-m.HP != want {
							t.Fatalf("actual damage=%d want=%d", before-m.HP, want)
						}
					}
				})
			}
		})
	}
}

func TestLegendarySwordsmanSoloIncomingCoverage(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"JK-D02", 100}, {"JK-D04", 50}, {"JK-D05", 60}, {"JK-D06", 65}, {"JK-D07", 100}, {"JK-D09", 550}, {"JK-D10", 65}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, mode := range []string{"equipped", "unequipped", "dot", "condition-false", "magic"} {
				t.Run(mode, func(t *testing.T) {
					s, p, m := swordsmanCoverageFixture(t, tc.id, mode != "unequipped")
					damage := int32(100)
					if tc.id == "JK-D09" {
						damage = 1000
					}
					m.Stats = domain.Stats{MinAtk: damage, MaxAtk: damage, MAtk: damage}
					m.Status = domain.NewStatusSet()
					if tc.id == "JK-D04" {
						m.Monster.Target = p.ID
						m.Monster.TauntPower = 1
						if mode == "condition-false" {
							m.Monster.Target = 999
						}
					}
					if tc.id == "JK-D05" && mode == "condition-false" {
						m.Pos.X = p.Pos.X + 76
					}
					if tc.id == "JK-D10" {
						m.Pos.X = p.Pos.X + 76
						if mode == "condition-false" {
							m.Pos.X = p.Pos.X + 75
						}
					}
					if (tc.id == "JK-D06" || tc.id == "JK-D07") && mode != "condition-false" {
						skill := domain.SkillID(10111)
						if tc.id == "JK-D07" {
							skill = 10113
						}
						s.legendaryCastReleased(p, domain.SkillDef{ID: skill}, 0, nil)
					}
					if tc.id == "JK-D09" && mode == "condition-false" {
						damage = 799
						m.Stats.MinAtk, m.Stats.MaxAtk = damage, damage
					}
					before := p.HP
					if mode == "dot" {
						s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 190911, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -damage}}}})
					} else {
						s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, Magic: mode == "magic", GuaranteedHit: true})
					}
					want := tc.want
					if tc.id == "JK-D07" && mode == "magic" {
						want = 50
					}
					if mode == "unequipped" || mode == "condition-false" || mode == "dot" {
						want = damage
					}
					if tc.id == "JK-D02" && mode == "dot" {
						want = 45
					}
					if tc.id == "JK-D05" && mode == "magic" {
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

func TestLegendarySwordsmanSoloTargetCaps(t *testing.T) {
	for _, tc := range []struct {
		id            string
		skill         domain.SkillID
		count         int
		primary, side int32
		remote        bool
	}{
		{"JK-A02", 10117, 11, 115, 115, true}, {"JK-A04", 10117, 11, 175, 75, true},
		{"JK-A08", 10103, 11, 100, 130, false}, {"JK-A09", 10104, 11, 115, 115, false},
		{"JK-A19", 10102, 11, 125, 125, false}, {"JK-A20", 10116, 11, 100, 160, false},
		{"JK-A22", 10123, 11, 190, 170, false}, {"JK-A23", 10123, 11, 100, 110, false},
		{"JK-A27", 10103, 11, 420, 320, false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, tc.id, true)
			if tc.remote {
				m.Pos.X = 190
				s.aoi.Move(m, m.Pos)
			}
			if tc.id == "JK-A02" {
				m.Pos.X = 400
				s.aoi.Move(m, m.Pos)
			}
			if tc.id == "JK-A23" {
				m.HP = 50000
			}
			before := m.HP
			for i := 0; i < 10; i++ {
				putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
			}
			swordCast(t, s, p, m, swordsmanCoverageDef(tc.skill))
			if before-m.HP != tc.primary {
				t.Fatalf("primary=%d want=%d", before-m.HP, tc.primary)
			}
			n := 0
			for i := 0; i < 10; i++ {
				q := s.entities[domain.EntityID(901+i)]
				loss := q.MaxHP - q.HP
				if loss > 0 {
					n++
					if loss != tc.side {
						t.Fatalf("side %d loss=%d want=%d", i, loss, tc.side)
					}
				}
			}
			if n != tc.count-1 {
				t.Fatalf("side cap=%d want=%d", n, tc.count-1)
			}
		})
	}
}

func TestLegendarySwordsmanSoloControlCoverage(t *testing.T) {
	for _, tc := range []struct {
		id               string
		skill            domain.SkillID
		limit            int
		damage, cooldown int32
	}{{"JK-A05", 10111, 9, 140, 24000}, {"JK-A13", 10113, 9, 170, 24000}, {"JK-A24", 10126, 9, 180, 18000}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, tc.id, true)
			st := swordsmanCoverageRootStatus()
			if tc.id == "JK-A13" {
				st = domain.StatusDef{ID: domain.StatusSilence, Level: 1, DurationSec: 6, Control: domain.ControlSilence, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
			}
			if tc.id == "JK-A24" {
				st = domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 6, Control: domain.ControlStun, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
				m.Status = domain.NewStatusSet()
				m.Status.Apply(swordsmanCoverageRootStatus(), s.tick, 999)
			}
			for i := 0; i < 8; i++ {
				q := putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
				if tc.id == "JK-A24" {
					q.Status = domain.NewStatusSet()
					q.Status.Apply(swordsmanCoverageRootStatus(), s.tick, 999)
				}
			}
			s.statuses = statusTable(st)
			d := swordsmanCoverageDef(tc.skill)
			d.CooldownMS = 120000
			d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			swordsmanCoverageNativeCast(t, s, p, d, m.ID)
			if !p.Player.SkillReadyAt(tc.skill, s.tick+domain.Ticks(int(tc.cooldown))) || p.Player.SkillReadyAt(tc.skill, s.tick+domain.Ticks(int(tc.cooldown))-1) {
				t.Fatal("actual cooldown", tc.cooldown)
			}
			if m.MaxHP-m.HP != tc.damage || !m.Status.Has(st.ID) {
				t.Fatal("primary damage/control", m.HP)
			}
			n := 1
			for i := 0; i < 8; i++ {
				q := s.entities[domain.EntityID(901+i)]
				if q.HP == q.MaxHP {
					continue
				}
				n++
				if q.MaxHP-q.HP != tc.damage {
					t.Fatal("side damage", q.HP)
				}
				if tc.id == "JK-A24" {
					if q.Status.Has(st.ID) {
						t.Fatal("side copied primary stun")
					}
				} else if !q.Status.Has(st.ID) {
					t.Fatal("side lost original control")
				}
			}
			if n != tc.limit {
				t.Fatalf("actual selected=%d want=%d", n, tc.limit)
			}
		})
	}
	t.Run("JK-A06", func(t *testing.T) {
		for _, control := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A06", true)
			d := swordsmanCoverageDef(10111)
			if control {
				st := swordsmanCoverageRootStatus()
				s.statuses = statusTable(st)
				d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			}
			before := m.HP
			swordCast(t, s, p, m, d)
			want := int32(100)
			if control {
				want = 300
			}
			if before-m.HP != want {
				t.Fatal("root success gating", before-m.HP)
			}
		}
	})
	t.Run("JK-A18", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-A18", true)
		m.Stats.Def = 100
		s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
		m.Status = domain.NewStatusSet()
		for i := 0; i < 2; i++ {
			legendaryMeleeStrike(s, p, m, 10122)
		}
		count := 0
		m.Status.EachEffective(func(a domain.Active) {
			if a.Def.LegendaryPowerID == "JK-A18" {
				count++
				if a.Def.Affixes[0].Value != -30 || a.ExpireAt != s.tick+domain.Ticks(8000) {
					t.Fatal("armor magnitude/lifetime", a)
				}
			}
		})
		if count != 1 {
			t.Fatal("armor stacked", count)
		}
		if m.Stats.Def != 70 {
			t.Fatal("actual defense did not update", m.Stats.Def)
		}
	})
}

func TestLegendarySwordsmanSoloRestoreAndShieldCoverage(t *testing.T) {
	t.Run("JK-A10", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A10", equipped)
			m.HP = 90000
			legendaryMeleeStrike(s, p, m, 10104)
			legendaryMeleeStrike(s, p, m, 10104)
			want := int32(100)
			if equipped {
				want = 35
			}
			for i := 0; i < 2; i++ {
				hp, maxHP, mp := m.HP, m.MaxHP, m.MP
				ev := combat.Restore(m, m, 100, 0)
				if ev.Amount != want || m.HP-hp != want || m.MaxHP != maxHP || m.MP != mp {
					t.Fatal("actual heal suppression", ev)
				}
			}
			m.Status.Expire(s.tick + domain.Ticks(8000))
			if combat.Restore(m, m, 100, 0).Amount != 100 {
				t.Fatal("suppression outlived 8 seconds")
			}
		}
	})
	t.Run("JK-A11", func(t *testing.T) {
		for _, skill := range []domain.SkillID{10104, 10119} {
			for _, targets := range []int{1, 2, 3, 4, 6} {
				s, p, m := swordsmanCoverageFixture(t, "JK-A11", true)
				p.HP, p.MP = 1000, 1000
				d := swordDualDef(skill)
				finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
				root := s.equipmentState().root
				root.def = d
				for i := 0; i < targets; i++ {
					q := m
					if i > 0 {
						q = putMonster(s, domain.EntityID(900+i), 10000, domain.Stats{}, 140+float64(i), 100)
					}
					s.applySkill(p, q, d, 100, false)
				}
				s.flushLegendarySwordRestore()
				finish()
				want := int32(20 * (min(targets, 4) + 2) / 2)
				got := p.HP - 1000
				if skill == 10119 {
					got = p.MP - 1000
				}
				if got != want {
					t.Fatalf("skill=%d distinct=%d restore=%d want=%d", skill, targets, got, want)
				}
			}
		}
		TestLegendarySwordsmanRestoreActualHitsAndNonRecursive(t)
	})
	t.Run("JK-A12", func(t *testing.T) {
		for _, mp := range []int32{0, 990, 1000} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A12", true)
			p.MaxMP, p.MP = 1000, mp
			before := m.HP
			swordCast(t, s, p, m, swordDualDef(10119))
			actual := min(int32(20), 1000-mp)
			if p.MP != mp+actual || before-m.HP != 150+actual*280/100 {
				t.Fatalf("real restore conversion mp=%d damage=%d", p.MP, before-m.HP)
			}
		}
	})
	for _, id := range []string{"JK-D01", "JK-D03", "JK-D08"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, id, true)
			s.refreshStats(p)
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			p.HP = p.MaxHP
			maxHP := p.MaxHP
			skill := domain.SkillID(10102)
			d := swordsmanCoverageDef(skill)
			want := maxHP * 12 / 100
			if id == "JK-D03" {
				d = swordDualDef(10104)
				want = 20
			}
			if id == "JK-D08" {
				d = swordsmanCoverageDef(10116)
				want = maxHP * 14 / 100
			}
			swordCast(t, s, p, m, d)
			if universalShieldValue(p) != want {
				t.Fatalf("native shield=%d want=%d", universalShieldValue(p), want)
			}
			p.Status.AbsorbDamage(want / 2)
			swordCast(t, s, p, m, d)
			if universalShieldValue(p) != want {
				t.Fatal("refresh stacked", universalShieldValue(p))
			}
			p.Status.Expire(s.tick + domain.Ticks(999999))
			if universalShieldValue(p) != want {
				t.Fatal("permanent shield expired")
			}
			p.Player.Worn = &domain.EquipSet{}
			s.pruneLegendaryOwner(p)
			if universalShieldValue(p) != 0 {
				t.Fatal("source removal retained shield")
			}
		})
	}
	t.Run("JK-D03-cap", TestLegendarySwordsmanShieldCapRefreshNoExpiry)
	t.Run("JK-D08-allies", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-D08", true)
		s.refreshStats(p)
		p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
		p.Player.Party = 1
		for i := 2; i <= 5; i++ {
			join(t, s, domain.EntityID(i), int64(i*100), fmt.Sprint(i), 120+float64(i), 100)
			q := s.players[domain.EntityID(i)]
			q.Player.Party = 1
			q.MaxHP = 1000
			q.HP = int32(i * 100)
		}
		far := s.players[5]
		far.Pos.X = 400
		s.aoi.Move(far, far.Pos)
		swordCast(t, s, p, m, swordsmanCoverageDef(10116))
		want := p.MaxHP * 14 / 100
		if universalShieldValue(s.players[2]) != want || universalShieldValue(s.players[3]) != want || universalShieldValue(s.players[4]) != want || universalShieldValue(far) != 0 {
			t.Fatal("ally ratio/radius/cap filtering")
		}
		p.Player.Worn = &domain.EquipSet{}
		s.pruneLegendaryOwner(p)
		for i := 2; i <= 5; i++ {
			if universalShieldValue(s.players[domain.EntityID(i)]) != 0 {
				t.Fatal("ally shield survived source removal")
			}
		}
	})
}

func TestLegendarySwordsmanSoloUtilityCoverage(t *testing.T) {
	t.Run("JK-U01", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, _ := swordsmanCoverageFixture(t, "JK-U01", equipped)
			s.legendaryCastReleased(p, domain.SkillDef{ID: 10117}, 0, nil)
			st := domain.Stats{MoveSpeed: 100}
			s.legendaryPassiveStats(p, &st)
			want := int32(100)
			if equipped {
				want = 180
			}
			if st.MoveSpeed != want {
				t.Fatal("movement stat consumer", st.MoveSpeed)
			}
		}
	})
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"JK-U02", 10104}, {"JK-U05", 10103}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, tc.id, true)
			p.HP = 1000
			p.Status = domain.NewStatusSet()
			statuses := []domain.StatusDef{{ID: domain.StatusPoison, DurationSec: 10}, {ID: domain.StatusSlow, DurationSec: 10}, {ID: 800111, BlockMove: true, DurationSec: 10}, {ID: domain.StatusStun, Control: domain.ControlStun, DurationSec: 10}, {ID: domain.StatusFreeze, Control: domain.ControlStun, DurationSec: 10}, {ID: domain.StatusPetrify, Control: domain.ControlStun, DurationSec: 10}, {ID: 190922, Control: domain.ControlStun, DurationSec: 10, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -20}}}}
			for _, st := range statuses {
				p.Status.Apply(st, s.tick, m.ID)
			}
			// Release is the production post-admission consumer; controls already active
			// at acceptance are simulated here so they cannot prevent this accepted cast.
			d := swordsmanCoverageDef(tc.skill)
			if tc.id == "JK-U02" {
				d = swordDualDef(10104)
			}
			s.legendaryCastReleased(p, d, m.ID, []domain.EntityID{m.ID})
			if p.Status.Has(domain.StatusSlow) || tc.id == "JK-U02" && p.Status.Has(domain.StatusPoison) {
				t.Fatal("cleanse failed")
			}
			for _, st := range statuses[2:] {
				if !p.Status.Has(st.ID) {
					t.Fatal("cleanse removed protected control", st.ID)
				}
			}
			if tc.id == "JK-U05" && !p.Status.Has(domain.StatusPoison) {
				t.Fatal("slow cleanse removed poison")
			}
			if !s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusSlow, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}, m.ID) {
				t.Fatal("cleanse invented future immunity")
			}
		})
	}
	for _, tc := range []struct {
		id     string
		skill  domain.SkillID
		refund int32
	}{{"JK-U03", 10119, 60}, {"JK-U09", 10103, 80}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, mode := range []string{"equipped", "unequipped", "miss", "control-success"} {
				s, p, m := swordsmanCoverageFixture(t, tc.id, mode != "unequipped")
				p.MP = 500
				d := swordsmanCoverageDef(tc.skill)
				d.MPCost = 100
				if mode == "miss" {
					d.Effect.GuaranteedHit = false
					p.Stats.Hit = 0
					m.Stats.Def = 1000000
				}
				if mode == "control-success" && tc.id == "JK-U06" {
					st := swordsmanCoverageRootStatus()
					s.statuses = statusTable(st)
					d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
				}
				swordsmanCoverageNativeCast(t, s, p, d, m.ID)
				want := int32(400) + tc.refund
				if mode == "unequipped" || mode == "miss" || mode == "control-success" && tc.id == "JK-U06" {
					want = 400
				}
				if p.MP != want {
					t.Fatalf("%s real paid/refund MP=%d want=%d", mode, p.MP, want)
				}
			}
		})
	}
	t.Run("JK-U07", func(t *testing.T) {
		for _, skill := range []domain.SkillID{10102, 10117, 10103} {
			s, p, m := swordsmanCoverageFixture(t, "JK-U07", true)
			p.MaxMP, p.MP = 1000, 500
			d := swordsmanCoverageDef(skill)
			d.MPCost = 100
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			swordsmanCoverageNativeCast(t, s, p, d, m.ID)
			if p.MP != 520 {
				t.Fatalf("native 12%% once per cast mp=%d", p.MP)
			}
		}
	})
	t.Run("JK-U08", swordsmanCoverageSoloMovementImmunity)

	t.Run("JK-U04", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-U04", equipped)
			m.Pos.X = 230
			s.aoi.Move(m, m.Pos)
			d := swordsmanCoverageDef(10127)
			d.Kind = domain.SkillArea
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
			d.Effect = domain.SkillEffect{Kind: domain.EffectTaunt, TauntPower: 100}
			swordsmanCoverageNativeCast(t, s, p, d, 0)
			if (m.Monster.Target == p.ID) != equipped {
				t.Fatal("expanded native taunt radius", m.Monster.Target)
			}
		}
	})
	t.Run("JK-U10", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-U10", equipped)
			m.Pos.X = 240
			s.aoi.Move(m, m.Pos)
			d := swordsmanCoverageDef(10113)
			d.Dist = 100
			st := domain.StatusDef{ID: domain.StatusSilence, Level: 1, DurationSec: 6, Control: domain.ControlSilence, EffectHitBP: 100000000}
			s.statuses = statusTable(st)
			d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			s.skills = skillTable(d)
			s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
			if (len(s.casts) > 0) != equipped {
				t.Fatal("native reach admission")
			}
			if equipped {
				s.stepCasts()
				if m.HP != 99900 || !m.Status.Has(st.ID) {
					t.Fatal("range bonus changed native damage/silence")
				}
			}
		}
	})
}

func TestLegendarySwordsmanSoloSpecialCoverage(t *testing.T) {
	t.Run("JK-A17", func(t *testing.T) {
		for _, radius := range []int32{100, 200} {
			for _, equipped := range []bool{false, true} {
				s, p, m := swordsmanCoverageFixture(t, "JK-A17", equipped)
				m.Pos.X = 240
				s.aoi.Move(m, m.Pos)
				far := putMonster(s, 901, 10000, domain.Stats{}, 280, 100)
				d := swordsmanCoverageDef(10121)
				d.Kind = domain.SkillArea
				d.Radius = radius
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: radius}
				swordsmanCoverageNativeCast(t, s, p, d, 0)
				if (m.HP < m.MaxHP) != (equipped || radius == 200) || (far.HP < far.MaxHP) != (radius == 200) {
					t.Fatal("minimum radius/original larger radius not consumed", m.HP, far.HP)
				}
			}
		}
	})
	t.Run("JK-A25", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A25", equipped)
			d := swordsmanCoverageDef(10127)
			d.Effect = domain.SkillEffect{Kind: domain.EffectTaunt, TauntPower: 100}
			before := m.HP
			swordCast(t, s, p, m, d)
			want := int32(0)
			if equipped {
				want = 160
			}
			if before-m.HP != want || m.Monster.Target != p.ID {
				t.Fatal("successful native taunt/counter", m.HP, m.Monster.Target)
			}
		}
	})
	t.Run("JK-A29", func(t *testing.T) {
		for _, kind := range []string{"direct", "shielded", "dot", "proc", "dead-owner", "unequipped"} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A29", kind != "unequipped")
			m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
			if kind == "shielded" {
				p.Status = domain.NewStatusSet()
				p.Status.Apply(domain.StatusDef{ID: 190929, DamageShield: 1000, DurationSec: 10}, s.tick, p.ID)
			}
			if kind == "dead-owner" {
				p.HP = 1
			}
			before := m.HP
			switch kind {
			case "dot":
				s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 190930, HPChanges: []domain.ResourceChange{{Value: -100}}}})
			case "proc":
				s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true, Origin: domain.DamageProcOrigin, ProcDepth: 1})
			default:
				s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true})
			}
			want := int32(0)
			if kind == "direct" || kind == "shielded" {
				want = 180
			}
			if before-m.HP != want {
				t.Fatalf("%s counter=%d want=%d", kind, before-m.HP, want)
			}
		}
	})
	t.Run("JK-A04", func(t *testing.T) {
		for _, dist := range []float64{74, 75} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A04", true)
			m.Pos.X = 100 + dist
			s.aoi.Move(m, m.Pos)
			before := m.HP
			swordCast(t, s, p, m, swordsmanCoverageDef(10117))
			want := int32(100)
			if dist >= 75 {
				want = 175
			}
			if before-m.HP != want {
				t.Fatal("range threshold", before-m.HP)
			}
			before = m.HP
			s.tick += domain.Ticks(5000)
			s.stepLegendaryPowers()
			if m.HP != before {
				t.Fatal("instant scar left damage zone")
			}
		}
	})
	t.Run("JK-A07", func(t *testing.T) {
		for _, dist := range []float64{150, 151} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A07", true)
			m.Pos.X = 100 + dist
			s.aoi.Move(m, m.Pos)
			d := swordsmanCoverageDef(10103)
			st := domain.StatusDef{ID: 190931, Level: 1, DurationSec: 8, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrHit, Mode: domain.ModePercent, Value: -20}}}
			s.statuses = statusTable(st)
			d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			before := m.HP
			swordCast(t, s, p, m, d)
			want := int32(110)
			if dist <= 150 {
				want = 200
			}
			if before-m.HP != want || !m.Status.Has(st.ID) {
				t.Fatal("shadow range/native hit debuff", before-m.HP)
			}
		}
	})
	t.Run("JK-A23", func(t *testing.T) {
		for _, hp := range []int32{50001, 50000, 50} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A23", true)
			m.HP = hp
			q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			swordCast(t, s, p, m, swordsmanCoverageDef(10123))
			want := int32(0)
			if hp <= 50000 {
				want = 110
			}
			if q.MaxHP-q.HP != want {
				t.Fatal("prehit threshold/kill", hp, q.HP)
			}
		}
	})
	t.Run("JK-A19", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-A19", true)
		before := m.HP
		swordCast(t, s, p, m, swordsmanCoverageDef(10117))
		if before-m.HP != 200 {
			t.Fatal("vacuum damage", before-m.HP)
		}
	})
}

func TestLegendarySwordsmanSoloNativePreservation(t *testing.T) {
	for _, id := range []string{"JK-A01", "JK-A19", "JK-A26"} {
		t.Run(id, func(t *testing.T) {
			var distance [2]float64
			var loss [2]int32
			for i, equipped := range []bool{false, true} {
				powers := []string{}
				if equipped {
					powers = []string{id}
				}
				s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10102: 1}, powers...)
				before, hp := m.Pos, m.HP
				meleeAuditCast(t, s, p, 10102, m.ID)
				distance[i] = m.Pos.X - before.X
				loss[i] = hp - m.HP
			}
			if distance[0] != 100 || distance[1] != 100 {
				t.Fatal("native knockback changed", distance)
			}
			pct := int32(200)
			if id == "JK-A19" {
				pct = 125
			}
			if loss[1] != loss[0]*pct/100 {
				t.Fatal("native direct damage coefficient", loss)
			}
		})
	}
	for _, id := range []string{"JK-A22", "JK-A30"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10123: 1}, id)
			s.rng = rand.New(meleeAuditZeroSource{})
			side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			meleeAuditCast(t, s, p, 10123, m.ID)
			if m.Alive() {
				t.Fatal("native instant death lost")
			}
			if !side.Alive() {
				t.Fatal("child copied primary instant death")
			}
		})
	}
	t.Run("JK-A30-lion", func(t *testing.T) {
		s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10116: 1}, "JK-A30")
		p.Stats.Hit = 0
		m.Stats.Def = 1000000
		before := m.HP
		meleeAuditCast(t, s, p, 10116, m.ID)
		if before == m.HP {
			t.Fatal("native lion guaranteed hit lost")
		}
	})
	t.Run("JK-A18", func(t *testing.T) {
		s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10122: 1}, "JK-A18")
		meleeAuditCast(t, s, p, 10122, m.ID)
		if s.applyStatusDef(m, domain.StatusDef{ID: 190940, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusGood, Affixes: []domain.Affix{{Attr: domain.AttrAtk, Value: 10}}}, p.ID) {
			t.Fatal("native beneficial-status block lost")
		}
	})
	for _, id := range []string{"JK-A09", "JK-A26", "JK-A28", "JK-U02"} {
		t.Run(id+"-restore", func(t *testing.T) {
			for _, skill := range []domain.SkillID{10104, 10119} {
				if id == "JK-U02" && skill != 10104 {
					continue
				}
				s, p, m := swordsmanCoverageFixture(t, id, true)
				p.HP, p.MP = 1000, 1000
				for i := 0; i < 5; i++ {
					putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
				}
				swordCast(t, s, p, m, swordDualDef(skill))
				wantHP, wantMP := int32(1000), int32(1000)
				if skill == 10104 {
					wantHP += 20
				} else {
					wantMP += 20
				}
				if p.HP != wantHP || p.MP != wantMP {
					t.Fatal("native restore duplicated/lost", id, skill, p.HP, p.MP)
				}
			}
		})
	}
	t.Run("JK-D06", func(t *testing.T) {
		for _, status := range []domain.StatusID{domain.StatusStun, domain.StatusFreeze, domain.StatusPetrify} {
			s, p, m := swordsmanCoverageFixture(t, "JK-D06", true)
			s.legendaryCastReleased(p, domain.SkillDef{ID: 10111}, 0, nil)
			m.Status = domain.NewStatusSet()
			m.Status.Apply(domain.StatusDef{ID: status, Control: domain.ControlStun, DurationSec: 10}, s.tick, p.ID)
			m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
			before := p.HP
			s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100})
			if before-p.HP != 65 {
				t.Fatal("own hard-control source ignored", status)
			}
		}
	})
	t.Run("JK-D09", func(t *testing.T) {
		for _, damage := range []int32{998, 1000} {
			s, p, m := swordsmanCoverageFixture(t, "JK-D09", true)
			p.Status = domain.NewStatusSet()
			p.Status.Apply(domain.StatusDef{ID: 190941, DurationSec: 10, PhysicalDamageReductionPct: 20, DamageShield: 2000}, s.tick, p.ID)
			m.Stats = domain.Stats{MinAtk: damage, MaxAtk: damage}
			before := p.HP
			s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, SkillPct: 100})
			want := int32(799)
			if damage == 1000 {
				want = 440
			}
			if p.HP != before || universalShieldValue(p) != 2000-want {
				t.Fatalf("post-mitigation/pre-shield threshold absorbed=%d want=%d", 2000-universalShieldValue(p), want)
			}
		}
	})
	t.Run("JK-D08-departure", func(t *testing.T) {
		for _, ownerLeaves := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-D08", true)
			join(t, s, 2, 200, "ally", 120, 100)
			q := s.players[2]
			p.Player.Party, q.Player.Party = 1, 1
			swordCast(t, s, p, m, swordsmanCoverageDef(10116))
			if universalShieldValue(q) == 0 {
				t.Fatal("ally shield missing")
			}
			id := q.ID
			if ownerLeaves {
				id = p.ID
			}
			s.onLeave(Leave{ID: id})
			if universalShieldValue(q) != 0 {
				t.Fatal("ally shield survived scene departure")
			}
		}
	})
}

func TestLegendarySwordsmanSoloSettledAndPullBoundaries(t *testing.T) {
	t.Run("JK-A14", func(t *testing.T) {
		for _, damage := range []int32{100, 1000} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A14", true)
			m.Status = domain.NewStatusSet()
			m.Status.Apply(domain.StatusDef{ID: 190942, DurationSec: 10, DamageShield: 10000}, s.tick, m.ID)
			d := swordsmanCoverageDef(10113)
			d.Effect.DamagePct = damage
			before := m.HP
			swordCast(t, s, p, m, d)
			want := damage + min(damage*120/100, int32(400))
			if universalShieldValue(m) != 10000-want || m.HP != before {
				t.Fatal("extra shield cap/no spill", universalShieldValue(m), want)
			}
		}
	})
	t.Run("JK-A15", func(t *testing.T) {
		TestLegendarySwordsmanConcentrateExcludesOverkill(t)
		s, p, m := swordsmanCoverageFixture(t, "JK-A15", true)
		m.Stats.PhysResist = 20
		q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
		q.Status = domain.NewStatusSet()
		q.Status.Apply(domain.StatusDef{ID: 190943, DurationSec: 10, DamageShield: 1000}, s.tick, q.ID)
		finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
		s.equipmentState().root.def = swordsmanCoverageDef(10121)
		for _, e := range []*entity.Entity{m, q} {
			s.resolveStrikeStatuses(p, e, &combat.HitTracker{}, combat.Blow{SkillID: 10121, SkillPct: 100, GuaranteedHit: true})
		}
		finish()
		if m.MaxHP-m.HP != 128+44 || q.HP != 10000 {
			t.Fatal("settled actual-health sum/resistance reapplication", m.HP, q.HP)
		}
	})
	t.Run("JK-A16", func(t *testing.T) {
		TestLegendarySwordsmanGatherImmunityAndRadius(t)
		s, p, m := swordsmanCoverageFixture(t, "JK-A16", true)
		m.Pos.X = 240
		s.aoi.Move(m, m.Pos)
		s.walkable = func(pos domain.Pos) bool { return pos.X < 180 || pos.X > 190 }
		d := swordsmanCoverageDef(10121)
		d.Kind = domain.SkillArea
		d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}
		swordCast(t, s, p, m, d)
		if m.Pos.X < 190 || m.Pos.X >= 240 {
			t.Fatal("pull crossed/ignored wall", m.Pos)
		}
	})
}

func TestLegendarySwordsmanSoloMovementOutput(t *testing.T) {
	t.Run("JK-U01", func(t *testing.T) { meleeCoverageMovementOutput(t, "JK-U01", 180) })
}
func swordsmanCoverageSoloMovementImmunity(t *testing.T) {
	for _, key := range []domain.StatusKey{{ID: 1009, Level: 1}, {ID: 10111, Level: 1}, {ID: 1034, Level: 41}} {
		t.Run(fmt.Sprintf("lifecycle-%d-%d", key.ID, key.Level), func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{}, "JK-U08")
			weapon := p.Player.Worn.At(domain.SlotWeapon)
			p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
			s.refreshStats(p)
			d, ok := s.statuses.Get(key.ID, key.Level)
			if !ok {
				t.Fatal("native fixture absent")
			}
			d.EffectHitBP = 100000000
			if !s.applyStatusDef(p, d, m.ID) || !p.Status.Has(key.ID) {
				t.Fatal("unequipped baseline absent")
			}
			p.Player.Worn.Set(domain.SlotWeapon, weapon)
			s.refreshStats(p)
			if p.Status.Has(key.ID) || !canMove(p) {
				t.Fatal("equip did not cleanse movement impairment")
			}
			if s.applyStatusDef(p, d, m.ID) {
				t.Fatal("new native movement impairment accepted")
			}
			p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
			s.refreshStats(p)
			if !s.applyStatusDef(p, d, m.ID) {
				t.Fatal("immunity survived unequip")
			}
		})
	}
	for _, id := range []domain.StatusID{domain.StatusStun, domain.StatusFreeze, domain.StatusPetrify, 81020} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("mixed-%d-existing-%v", id, existing), func(t *testing.T) {
				s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{}, "JK-U08")
				weapon := p.Player.Worn.At(domain.SlotWeapon)
				if existing {
					p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
				}
				s.refreshStats(p)
				d := domain.StatusDef{ID: id, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000, Control: domain.ControlStun, Immobile: 1, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}, {Attr: domain.AttrDef, Value: -20, Mode: domain.ModePercent}}, PhysicalDamageTakenPct: 30, IntervalSec: 2, HPChanges: []domain.ResourceChange{{Value: -7}}, DamageShield: 200}
				if id == 81020 {
					d.Control, d.BlockMove = domain.ControlNone, true
				}
				if !s.applyStatusDef(p, d, m.ID) {
					t.Fatal("mixed state discarded")
				}
				before := verifiedActive(t, p, id)
				if existing {
					s.tick += domain.Ticks(1000)
					p.Player.Worn.Set(domain.SlotWeapon, weapon)
					s.refreshStats(p)
				}
				got := verifiedActive(t, p, id)
				if got.Source != m.ID || got.ExpireAt != before.ExpireAt || got.NextTickAt != before.NextTickAt || got.ShieldLeft != before.ShieldLeft {
					t.Fatal("mixed lifecycle changed")
				}
				if got.Def.Control != d.Control || len(got.Def.Affixes) != 1 || got.Def.Affixes[0].Attr != domain.AttrDef || got.Def.PhysicalDamageTakenPct != 30 || len(got.Def.HPChanges) != 1 || got.Def.HPChanges[0].Value != -7 {
					t.Fatal("unrelated mixed payload changed")
				}
				if id == 81020 {
					if !canMove(p) {
						t.Fatal("root survived immunity")
					}
				} else if canMove(p) {
					t.Fatal("hard control incorrectly removed")
				}
			})
		}
	}
	TestLegendarySwordsmanMovementImmunityPreservesNativeAttackSlow(t)
}

func TestLegendarySwordsmanCoverageCombinations(t *testing.T) {
	t.Run("child-does-not-restore-mana", func(t *testing.T) {
		for _, id := range []string{"JK-A03", "JK-A04", "JK-A07", "JK-A27"} {
			skill := domain.SkillID(10102)
			if id == "JK-A04" {
				skill = 10117
			}
			if id == "JK-A07" || id == "JK-A27" {
				skill = 10103
			}
			s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{skill: 1}, "JK-U07", id)
			p.MaxMP, p.MP = 1000, 500
			if id == "JK-A04" {
				m.Pos.X = 190
				s.aoi.Move(m, m.Pos)
			}
			before := p.MP
			d := meleeAuditCast(t, s, p, skill, m.ID)
			if p.MP != before-d.MPCost+120 {
				t.Fatalf("%s child repeated native restoration MP=%d", id, p.MP)
			}
		}
	})
	t.Run("double-sword-three-powers", TestLegendarySwordsmanBloodSoulStandaloneAndCombinations)
}

func TestLegendarySwordsmanCoverageNoRecursiveLifesteal(t *testing.T) {
	for _, tc := range []struct {
		id     string
		skill  domain.SkillID
		direct int32
	}{
		{"JK-A03", 10102, 100}, {"JK-A04", 10117, 100}, {"JK-A06", 10111, 100}, {"JK-A07", 10103, 110}, {"JK-A08", 10103, 100}, {"JK-A12", 10119, 100}, {"JK-A15", 10121, 160}, {"JK-A20", 10116, 100}, {"JK-A21", 10116, 100}, {"JK-A22", 10123, 120}, {"JK-A23", 10123, 100}, {"JK-A27", 10103, 100}, {"JK-A28", 10104, 100},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{tc.skill: 1}, tc.id, "TY-A17")
			p.Status = domain.NewStatusSet()
			// Controlled skill-origin lifesteal status tests the shared consumer; it is not a claim that a native sword grants this buff.
			s.applyStatusDef(p, domain.StatusDef{ID: 190945, SkillApplied: true, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusGood, LifeStealPct: 100, LifeStealChance: 100}, p.ID)
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			p.HP = 1000
			p.MP = 1000
			d := swordsmanCoverageDef(tc.skill)
			if tc.id == "JK-A04" {
				m.Pos.X = 190
				s.aoi.Move(m, m.Pos)
			}
			if tc.id == "JK-A08" || tc.id == "JK-A20" || tc.id == "JK-A22" || tc.id == "JK-A23" {
				putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			}
			if tc.id == "JK-A23" {
				m.HP = 50000
			}
			if tc.id == "JK-A06" {
				st := swordsmanCoverageRootStatus()
				s.statuses = statusTable(st)
				d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
			}
			if tc.id == "JK-A12" {
				d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectRestoreMP, Target: domain.EffectTargetCaster, SourceAttackPct: 20, RequiresPreviousLanded: true}}
			}
			swordCast(t, s, p, m, d)
			want := int32(1000) + tc.direct*160/100
			if p.HP != want {
				t.Fatalf("equipment child repeated native lifesteal: HP=%d want=%d", p.HP, want)
			}
		})
	}
}

func TestLegendarySwordsmanSoloShieldRefreshEdges(t *testing.T) {
	t.Run("JK-D03", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-D03", true)
		s.refreshStats(p)
		p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
		p.MP = p.MaxMP
		maximum := p.MaxMP
		swordCast(t, s, p, m, swordDualDef(10119))
		if universalShieldValue(p) != 20 || p.MP != maximum {
			t.Fatal("actual mana overflow not converted")
		}
	})
	for _, id := range []string{"JK-D01", "JK-D08"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, id, true)
			s.refreshStats(p)
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			skill := domain.SkillID(10102)
			if id == "JK-D08" {
				skill = 10116
			}
			swordCast(t, s, p, m, swordsmanCoverageDef(skill))
			old := universalShieldValue(p)
			p.MaxHP = 1000
			swordCast(t, s, p, m, swordsmanCoverageDef(skill))
			if universalShieldValue(p) != old {
				t.Fatal("smaller new shield replaced higher remainder")
			}
		})
	}
}

func TestLegendarySwordsmanSoloPullCap(t *testing.T) {
	t.Run("JK-A16", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-A16", true)
		m.Pos.X = 240
		s.aoi.Move(m, m.Pos)
		targets := []*entity.Entity{m}
		for i := 1; i < 10; i++ {
			angle := float64(i) * 2 * math.Pi / 10
			targets = append(targets, putMonster(s, domain.EntityID(900+i), 10000, domain.Stats{}, 100+140*math.Cos(angle), 100+140*math.Sin(angle)))
		}
		before := map[domain.EntityID]domain.Pos{}
		for _, e := range targets {
			before[e.ID] = e.Pos
		}
		d := swordsmanCoverageDef(10121)
		d.Radius = 100
		swordCast(t, s, p, m, d)
		moved := 0
		for _, e := range targets {
			if e.Pos != before[e.ID] {
				moved++
				if math.Sqrt(sqDist(p.Pos, e.Pos)) > 51 {
					t.Fatal("pull stopped away from50 without obstacle", e.Pos)
				}
			}
		}
		if moved != len(targets) {
			t.Fatalf("successful pulls=%d want%d", moved, len(targets))
		}
	})
}

func TestLegendarySwordsmanSoloExactPayloadClauses(t *testing.T) {
	t.Run("JK-A07", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-A07", true)
		s.outbox = nil
		swordCast(t, s, p, m, swordsmanCoverageDef(10103))
		got := meleeCoverageDamageAmounts(s, p.ID, m.ID)
		if !reflect.DeepEqual(got, []int32{110, 45, 45}) {
			t.Fatal("two distinct45% shadows", got)
		}
	})
	t.Run("JK-A22", func(t *testing.T) {
		s, p, m := swordsmanCoverageFixture(t, "JK-A22", true)
		side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
		s.outbox = nil
		swordCast(t, s, p, m, swordsmanCoverageDef(10123))
		if got := meleeCoverageDamageAmounts(s, p.ID, m.ID); !reflect.DeepEqual(got, []int32{120, 70}) {
			t.Fatal("primary native then70%", got)
		}
		if got := meleeCoverageDamageAmounts(s, p.ID, side.ID); !reflect.DeepEqual(got, []int32{100, 70}) {
			t.Fatal("separate100% and70% side waves", got)
		}
	})
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		extra int32
	}{{"JK-A03", 10102, 160}, {"JK-A21", 10116, 320}, {"JK-A28", 10104, 260}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, tc.id, true)
			s.outbox = nil
			d := swordsmanCoverageDef(tc.skill)
			finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
			s.equipmentState().root.def = d
			s.equipmentState().root.targets = []domain.EntityID{m.ID}
			s.applySkill(p, m, d, 0, false)
			s.applySkill(p, m, d, 0, false)
			finish()
			got := meleeCoverageDamageAmounts(s, p.ID, m.ID)
			if !reflect.DeepEqual(got, []int32{100, 100, tc.extra}) {
				t.Fatal("same root repeated primary bonus", got)
			}
		})
	}
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		stun  bool
	}{{"JK-A05", 10111, false}, {"JK-A13", 10113, false}, {"JK-A24", 10126, true}} {
		t.Run(tc.id, func(t *testing.T) {
			for _, immune := range []bool{false, true} {
				s, p, m := swordsmanCoverageFixture(t, tc.id, true)
				st := swordsmanCoverageRootStatus()
				if tc.id == "JK-A13" {
					st.ID, st.BlockMove, st.Control = domain.StatusSilence, false, domain.ControlSilence
				}
				if tc.stun {
					st.ID, st.BlockMove, st.Control = domain.StatusStun, false, domain.ControlStun
				}
				s.statuses = statusTable(st)
				if immune {
					m.Status = domain.NewStatusSet()
					m.Status.Apply(domain.StatusDef{ID: 190946, DurationSec: 10, BlockHarmful: true}, s.tick, m.ID)
				}
				d := swordsmanCoverageDef(tc.skill)
				d.Statuses = []domain.StatusApplication{{ID: st.ID, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
				before := m.HP
				swordCast(t, s, p, m, d)
				if before == m.HP {
					t.Fatal("control immunity cancelled direct damage")
				}
				if immune {
					if m.Status.Has(st.ID) {
						t.Fatal("original control immunity bypassed")
					}
				} else {
					a := verifiedActive(t, m, st.ID)
					baseline, owner, target := swordsmanCoverageFixture(t, tc.id, false)
					baseline.statuses = statusTable(st)
					swordCast(t, baseline, owner, target, d)
					original := verifiedActive(t, target, st.ID)
					if a.ExpireAt-s.tick != original.ExpireAt-baseline.tick {
						t.Fatal("native control duration changed", a.ExpireAt, original.ExpireAt)
					}
				}
			}
		})
	}
}
func meleeCoverageDamageAmounts(s *Scene, src, dst domain.EntityID) []int32 {
	var out []int32
	for _, o := range s.outbox {
		if ev, ok := o.ev.(event.DamageDealt); ok && ev.Src == src && ev.Dst == dst {
			out = append(out, ev.Amount)
		}
	}
	return out
}
