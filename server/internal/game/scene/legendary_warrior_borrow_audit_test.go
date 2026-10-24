package scene

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Actual native casts, including the nested A27 -> A08 -> A04 donor chain.
// No native skill/status/range/cost rows are replaced to make the cases pass.
func TestLegendaryWarriorAuditBorrowedGeometryWithoutLearningGate(t *testing.T) {
	for _, skill := range []domain.SkillID{10009, 10027} {
		for _, rank := range []int32{0, 5} {
			for pieces := 0; pieces <= 3; pieces++ {
				for _, set := range []bool{false, true} {
					t.Run(fmt.Sprintf("skill%d/donor%d/powers%d/set%v", skill, rank, pieces, set), func(t *testing.T) {
						learned := domain.Learned{skill: 1}
						powers := []string{}
						if skill == 10009 {
							if rank > 0 {
								learned[10005] = rank
							}
							if pieces > 0 {
								powers = append(powers, "ZS-A08")
							}
							if pieces > 1 {
								powers = append(powers, "ZS-A04")
							}
							if pieces > 2 {
								powers = append(powers, "ZS-D02")
							}
						} else {
							if rank > 0 {
								learned[10009], learned[10005] = rank, rank
							}
							if pieces > 0 {
								powers = append(powers, "ZS-A27")
							}
							if pieces > 1 {
								powers = append(powers, "ZS-A08")
							}
							if pieces > 2 {
								powers = append(powers, "ZS-A04")
							}
						}
						s, p, m := meleeAuditNativeScene(t, domain.Warrior, learned, powers...)
						if set {
							meleeAuditEquipSet(t, s, p, 5)
						}
						near := putMonster(s, 901, 10000, domain.Stats{}, 140, 120)
						wide := putMonster(s, 902, 10000, domain.Stats{}, 140, 145)
						outside := putMonster(s, 903, 10000, domain.Stats{}, 140, 148)
						mBefore, nearBefore, wideBefore, outBefore := m.HP, near.HP, wide.HP, outside.HP
						mp := p.MP
						d := meleeAuditCast(t, s, p, skill, m.ID)
						rectangle := pieces > 0 && (skill == 10009 || pieces > 1)
						expanded := rectangle && (skill == 10009 && pieces > 1 || skill == 10027 && pieces > 2)
						if rectangle {
							length, width := int32(150), int32(50)
							if expanded {
								length, width = 180, 95
							}
							if d.Kind != domain.SkillArea || d.Area.Shape != domain.SkillAreaRectangle || d.Area.Length != length || d.Area.Width != width {
								t.Fatalf("borrowed geometry %+v want rectangle %dx%d", d.Area, length, width)
							}
						} else if d.Kind != domain.SkillSingle || d.Area.Valid() {
							t.Fatalf("native single shape changed: %+v", d.Area)
						}
						if m.HP >= mBefore || (near.HP < nearBefore) != rectangle || (wide.HP < wideBefore) != expanded || outside.HP != outBefore {
							t.Fatalf("target admission main%d near%d wide%d outside%d rectangle%v expanded%v", mBefore-m.HP, nearBefore-near.HP, wideBefore-wide.HP, outBefore-outside.HP, rectangle, expanded)
						}
						status := domain.StatusID(10009)
						if skill == 10027 {
							status = 1002
						}
						if !m.Status.Has(status) || near.Status.Has(status) != rectangle || wide.Status.Has(status) != expanded || outside.Status.Has(status) {
							t.Fatal("native status did not follow borrowed geometry", status)
						}
						if skill == 10027 && (m.Status.Has(10009) || near.Status.Has(10009)) {
							t.Fatal("geometry donor leaked armor payload")
						}
						if set && (d.MPCost != 0 || p.MP != mp || p.HP >= 10000) {
							t.Fatal("dragon set cost/lifesteal path lost", d.MPCost, p.MP, p.HP)
						}
						if !set && (d.MPCost <= 0 || p.MP != mp-d.MPCost) {
							t.Fatal("native mana changed", d.MPCost, mp-p.MP)
						}
						if rank == 0 {
							if learned[10005] != 0 || skill == 10027 && learned[10009] != 0 {
								t.Fatal("borrowing learned the donor")
							}
							donors := []domain.SkillID{10005}
							if skill == 10027 {
								donors = append(donors, 10009)
							}
							for _, donor := range donors {
								s.onUseSkill(UseSkill{ID: p.ID, Skill: donor, Target: m.ID})
								if len(s.casts) != 0 {
									t.Fatal("borrowed donor became actively castable", donor)
								}
							}
						}
					})
				}
			}
		}
	}
}

func TestLegendaryWarriorAuditBorrowedReachWithoutLearningGate(t *testing.T) {
	for _, rank := range []int32{0, 5} {
		for pieces := 0; pieces <= 3; pieces++ {
			for _, set := range []bool{false, true} {
				for _, distance := range []float64{300, 301} {
					t.Run(fmt.Sprintf("donor%d/powers%d/set%v/distance%.0f", rank, pieces, set, distance), func(t *testing.T) {
						learned := domain.Learned{10002: 1}
						if rank > 0 {
							learned[10017] = rank
						}
						powers := []string{}
						if pieces > 0 {
							powers = append(powers, "ZS-A30")
						}
						if pieces > 1 {
							powers = append(powers, "ZS-A01")
						}
						if pieces > 2 {
							powers = append(powers, "ZS-U01")
						}
						s, p, m := meleeAuditNativeScene(t, domain.Warrior, learned, powers...)
						if set {
							meleeAuditEquipSet(t, s, p, 5)
						}
						m.Pos.X = p.Pos.X + distance
						hp, mp := m.HP, p.MP
						s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID})
						allowed := pieces > 0 && distance == 300
						if (len(s.casts) > 0) != allowed {
							t.Fatalf("borrowed range allowed=%v casts=%d outbox=%+v", allowed, len(s.casts), s.outbox)
						}
						if !allowed {
							if m.HP != hp || p.MP != mp {
								t.Fatal("rejected range spent resources or damaged")
							}
							return
						}
						d := s.casts[0].def
						if d.Dist != 300 || d.Kind != domain.SkillSingle || d.Area.Valid() || d.ProjectileSpeedPXPerSec != 0 {
							t.Fatal("range borrowing changed native hit behavior", d.Dist, d.Kind, d.Area, d.ProjectileSpeedPXPerSec)
						}
						now := s.now()
						s.stepCastsAt(now.Add(time.Minute), false)
						s.stepCastsAt(now.Add(2*time.Minute), false)
						if len(s.casts) != 0 || m.HP >= hp {
							t.Fatal("borrowed range did not settle native hit", len(s.casts), hp-m.HP)
						}
						if set && (d.MPCost != 0 || p.MP != mp || p.HP >= 10000) {
							t.Fatal("dragon set cost/lifesteal path lost")
						}
						if !set && (d.MPCost <= 0 || p.MP != mp-d.MPCost) {
							t.Fatal("native mana changed")
						}
						if rank == 0 {
							if learned[10017] != 0 {
								t.Fatal("range donor was learned")
							}
							s.onUseSkill(UseSkill{ID: p.ID, Skill: 10017, Target: m.ID})
							if len(s.casts) != 0 {
								t.Fatal("range donor became actively castable")
							}
						}
					})
				}
			}
		}
	}
}

func TestLegendaryWarriorAuditBorrowedRankAndNativeRows(t *testing.T) {
	for _, id := range []domain.SkillID{10005, 10009, 10017} {
		for _, rank := range []int32{0, 5, 20} {
			t.Run(fmt.Sprintf("skill%d/rank%d", id, rank), func(t *testing.T) {
				learned := domain.Learned{10002: 1}
				if rank > 0 {
					learned[id] = rank
				}
				s, p, m := meleeAuditNativeScene(t, domain.Warrior, learned, "ZS-A08", "ZS-A04", "ZS-A30")
				raw, _ := s.skills.Get(id, max(rank, 1))
				c := legendaryContext{Scene: s, Owner: p, Target: m, Primary: m.ID}
				d, ok := warriorBorrowedSkill(&c, id, true)
				if !ok || d.Level != max(rank, 1) {
					t.Fatal("donor did not use native minimum/current rank", d.Level, rank)
				}
				after, _ := s.skills.Get(id, max(rank, 1))
				if !reflect.DeepEqual(raw, after) {
					t.Fatal("borrowed composition mutated native row")
				}
				if rank == 0 {
					if _, ok := warriorLearned(&c, id, true); ok {
						t.Fatal("explicit learned-skill prerequisite was relaxed")
					}
				}
			})
		}
	}
}
