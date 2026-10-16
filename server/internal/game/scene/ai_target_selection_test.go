package scene

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// These reference implementations preserve the pre-optimization ordering and side effects.
func (s *Scene) acquireFullSortReference(m *entity.Entity, p ai.Params) {
	type candidate struct {
		entity *entity.Entity
		distSq float64
	}
	candidates := make([]candidate, 0, 4)
	s.aoi.AroundPos(m.Pos, func(o *entity.Entity) {
		if o.Kind != domain.KindPlayer || !o.Alive() || entityInvisible(o) || s.entityMapLoading(o) || !s.equipmentCombatReady(o) {
			return
		}
		if o.Player != nil && o.Player.Protected(s.tick) {
			return // 复活保护中, 别再扑上去
		}
		if !ai.CanAcquire(p, m.Pos, m.Monster.Home, o.Pos) {
			return
		}
		candidates = append(candidates, candidate{entity: o, distSq: sqDist(m.Pos, o.Pos)})
	})
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distSq != candidates[j].distSq {
			return candidates[i].distSq < candidates[j].distSq
		}
		return candidates[i].entity.ID < candidates[j].entity.ID
	})
	for _, cand := range candidates {
		if _, _, ok := s.monsterChaseWaypoint(m, cand.entity.Pos, p.TraceDist); !ok {
			continue
		}
		m.Monster.Target = cand.entity.ID
		m.Monster.Roaming = false
		s.monsterSpeak(m, domain.MonsterSceneEnemy)
		return
	}
}
func (s *Scene) resolveMonsterHealTargetsFullSortReference(caster *entity.Entity, rule domain.MonsterSkillRule) []domain.EntityID {
	castRange := float64(rule.Skill.Dist)
	if castRange <= 0 {
		castRange = float64(caster.Monster.AI.BasicAttackDist)
	}
	if castRange <= 0 {
		castRange = ai.MeleeRange
	}
	radius := float64(rule.Skill.Radius)
	area := rule.Skill.Kind == domain.SkillArea || rule.Skill.TargetTeam || radius > 0
	if area && radius > 0 {
		castRange = radius
	}
	rangeSq := castRange * castRange
	candidates := make([]*entity.Entity, 0, 4)
	s.aoi.AroundPos(caster.Pos, func(candidate *entity.Entity) {
		if candidate == nil || candidate.Monster == nil || !candidate.Alive() ||
			!candidate.Monster.Kind.Hostile() || candidate.HP >= candidate.MaxHP ||
			sqDist(caster.Pos, candidate.Pos) > rangeSq {
			return
		}
		candidates = append(candidates, candidate)
	})
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := int64(candidates[i].HP) * int64(candidates[j].MaxHP)
		right := int64(candidates[j].HP) * int64(candidates[i].MaxHP)
		if left != right {
			return left < right
		}
		leftDist, rightDist := sqDist(caster.Pos, candidates[i].Pos), sqDist(caster.Pos, candidates[j].Pos)
		if leftDist != rightDist {
			return leftDist < rightDist
		}
		return candidates[i].ID < candidates[j].ID
	})
	if !area {
		return []domain.EntityID{candidates[0].ID}
	}
	out := make([]domain.EntityID, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.ID)
	}
	return sortEntityIDs(out)
}
func targetSelectionFixture(n int) (*Scene, *entity.Entity) {
	s := &Scene{aoi: newAOI(300), entities: map[domain.EntityID]*entity.Entity{}, tick: 100}
	m := &entity.Entity{ID: 99999, HP: 100, MaxHP: 100, Kind: domain.KindMonster, Pos: domain.Pos{X: 100, Y: 100}, Monster: &entity.Monster{}}
	for i := 0; i < n; i++ {
		e := &entity.Entity{ID: domain.EntityID(i + 1), HP: 50, MaxHP: 100, Kind: domain.KindPlayer, Pos: domain.Pos{X: 100 + float64(i%20), Y: 100 + float64(i/20)}}
		s.aoi.Enter(e)
		s.entities[e.ID] = e
	}
	return s, m
}
func TestAcquireMatchesFullSort(t *testing.T) {
	for _, n := range []int{0, 1, 8, 16, 17, 64, 256} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("players%d/blocked%t", n, blocked), func(t *testing.T) {
				s, m := targetSelectionFixture(n)
				for _, e := range s.entities {
					e.Pos.X++ // No candidate shares the sole walkable origin in the blocked case.
				}
				checks := 0
				s.walkable = func(p domain.Pos) bool { checks++; return !blocked || p == m.Pos }
				params := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
				for repeat := 0; repeat < 20; repeat++ {
					initial := entity.Monster{Roaming: true}
					*m.Monster = initial
					checks = 0
					s.acquireFullSortReference(m, params)
					want, wantChecks := *m.Monster, checks
					*m.Monster = initial
					checks = 0
					s.acquire(m, params)
					if !reflect.DeepEqual(*m.Monster, want) || checks != wantChecks {
						t.Fatalf("state/checks mismatch: got %+v/%d, want %+v/%d", *m.Monster, checks, want, wantChecks)
					}
				}
			})
		}
	}
}

func TestAcquireBlockedNearestMatchesFullSort(t *testing.T) {
	for _, allBlocked := range []bool{false, true} {
		for _, cachedRetry := range []bool{false, true} {
			for repeat := 0; repeat < 30; repeat++ {
				s, m := targetSelectionFixture(3)
				s.entities[1].Pos = domain.Pos{X: 101, Y: 100}
				s.entities[2].Pos = domain.Pos{X: 100, Y: 103}
				s.entities[3].Pos = domain.Pos{X: 100, Y: 97}
				checks := 0
				s.walkable = func(p domain.Pos) bool {
					checks++
					if allBlocked {
						return p == m.Pos
					}
					return p.X < 100.5
				}
				params := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
				initial := entity.Monster{Roaming: true}
				if cachedRetry {
					initial.ChasePathRetryAt = s.tick + 10
					initial.ChasePathRetryTarget = s.entities[1].Pos
				}
				*m.Monster = initial
				s.acquireFullSortReference(m, params)
				want, wantChecks := *m.Monster, checks
				*m.Monster = initial
				checks = 0
				s.acquire(m, params)
				if !reflect.DeepEqual(*m.Monster, want) || checks != wantChecks {
					t.Fatalf("allBlocked=%t cachedRetry=%t: got %+v/%d, want %+v/%d", allBlocked, cachedRetry, *m.Monster, checks, want, wantChecks)
				}
				if !allBlocked && m.Monster.Target != 2 {
					t.Fatalf("equidistant fallback must choose ID 2, got %d", m.Monster.Target)
				}
				if allBlocked && m.Monster.Target != 0 {
					t.Fatalf("all blocked: selected %d", m.Monster.Target)
				}
			}
		}
	}
}

func TestAcquireDistanceTieAndFilters(t *testing.T) {
	s, m := targetSelectionFixture(6)
	for _, e := range s.entities {
		e.Pos = domain.Pos{X: 101, Y: 100}
	}
	s.entities[1].HP = 0
	s.entities[2].Kind = domain.KindMonster
	s.entities[3].Player = &entity.Player{}
	s.entities[3].Player.Protect(s.tick + 10)
	for repeat := 0; repeat < 30; repeat++ {
		*m.Monster = entity.Monster{}
		s.acquire(m, ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true})
		if m.Monster.Target != 4 {
			t.Fatalf("want lowest eligible ID 4, got %d", m.Monster.Target)
		}
	}
}

func TestMonsterHealMatchesFullSort(t *testing.T) {
	for _, n := range []int{0, 1, 8, 16, 17, 64, 256} {
		for _, mode := range []string{"single", "area", "team", "radius", "basic-range", "melee-range"} {
			t.Run(fmt.Sprintf("monsters%d/%s", n, mode), func(t *testing.T) {
				s, m := targetSelectionFixture(n)
				for _, e := range s.entities {
					e.Kind = domain.KindMonster
					e.Monster = &entity.Monster{Kind: domain.MonsterNormal}
					e.HP = int32(e.ID%7 + 1)
					e.MaxHP = int32(e.ID%3+1) * 100
					switch e.ID % 11 {
					case 1:
						e.HP = 0
					case 2:
						e.HP = e.MaxHP
					case 3:
						e.Monster.Kind = domain.MonsterProp
					case 4:
						e.Monster = nil
					}
				}
				rule := domain.MonsterSkillRule{}
				rule.Skill.Dist = 300
				switch mode {
				case "area":
					rule.Skill.Kind = domain.SkillArea
				case "team":
					rule.Skill.TargetTeam = true
				case "radius":
					rule.Skill.Dist = 1
					rule.Skill.Radius = 10
				case "basic-range":
					rule.Skill.Dist = 0
					m.Monster.AI.BasicAttackDist = 10
				case "melee-range":
					rule.Skill.Dist = 0
				}
				for repeat := 0; repeat < 20; repeat++ {
					want := s.resolveMonsterHealTargetsFullSortReference(m, rule)
					got := s.resolveMonsterHealTargets(m, rule)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("got %v, want %v", got, want)
					}
				}
			})
		}
	}
}

func TestMonsterHealFractionDistanceAndIDTies(t *testing.T) {
	s, m := targetSelectionFixture(4)
	for _, e := range s.entities {
		e.Kind = domain.KindMonster
		e.Monster = &entity.Monster{Kind: domain.MonsterNormal}
		e.Pos = domain.Pos{X: 101, Y: 100}
		e.HP, e.MaxHP = 25, 100
	}
	s.entities[1].HP = 50                           // Lower ID loses on HP fraction.
	s.entities[2].Pos.X = 102                       // Equal fraction loses on distance.
	s.entities[3].HP, s.entities[3].MaxHP = 50, 200 // Equal fraction and distance wins on ID.
	rule := domain.MonsterSkillRule{}
	rule.Skill.Dist = 300
	for repeat := 0; repeat < 30; repeat++ {
		if got := s.resolveMonsterHealTargets(m, rule); !slices.Equal(got, []domain.EntityID{3}) {
			t.Fatalf("single: got %v", got)
		}
	}
	rule.Skill.TargetTeam = true
	if got := s.resolveMonsterHealTargets(m, rule); !slices.Equal(got, []domain.EntityID{1, 2, 3, 4}) {
		t.Fatalf("area order: got %v", got)
	}
}

func BenchmarkAcquireTargetSelection(b *testing.B) {
	for _, n := range []int{0, 1, 8, 64, 256} {
		for _, optimized := range []bool{false, true} {
			b.Run(fmt.Sprintf("players%d/optimized%t", n, optimized), func(b *testing.B) {
				s, m := targetSelectionFixture(n)
				params := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					m.Monster.Target = 0
					if optimized {
						s.acquire(m, params)
					} else {
						s.acquireFullSortReference(m, params)
					}
				}
			})
		}
	}
}

func BenchmarkMonsterHealTargetSelection(b *testing.B) {
	for _, n := range []int{8, 64, 256} {
		for _, area := range []bool{false, true} {
			for _, optimized := range []bool{false, true} {
				b.Run(fmt.Sprintf("monsters%d/area%t/optimized%t", n, area, optimized), func(b *testing.B) {
					s, m := targetSelectionFixture(n)
					for _, e := range s.entities {
						e.Kind = domain.KindMonster
						e.Monster = &entity.Monster{Kind: domain.MonsterNormal}
					}
					rule := domain.MonsterSkillRule{}
					rule.Skill.Dist = 300
					rule.Skill.TargetTeam = area
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if optimized {
							s.resolveMonsterHealTargets(m, rule)
						} else {
							s.resolveMonsterHealTargetsFullSortReference(m, rule)
						}
					}
				})
			}
		}
	}
}

func TestAcquireMultipleBlockedBeforeReachable(t *testing.T) {
	for repeat := 0; repeat < 30; repeat++ {
		s, m := targetSelectionFixture(4)
		s.entities[1].Pos = domain.Pos{X: 101, Y: 100}
		s.entities[2].Pos = domain.Pos{X: 102, Y: 100}
		s.entities[3].Pos = domain.Pos{X: 100, Y: 104}
		s.entities[4].Pos = domain.Pos{X: 100, Y: 96}
		checks := 0
		s.walkable = func(p domain.Pos) bool { checks++; return p.X < 100.5 }
		params := ai.Params{ViewDist: 400, TraceDist: 600, Aggressive: true}
		s.acquireFullSortReference(m, params)
		want, wantChecks := *m.Monster, checks
		*m.Monster = entity.Monster{}
		checks = 0
		s.acquire(m, params)
		if !reflect.DeepEqual(*m.Monster, want) || checks != wantChecks || m.Monster.Target != 3 {
			t.Fatalf("got %+v/%d, want %+v/%d with target 3", *m.Monster, checks, want, wantChecks)
		}
	}
}
