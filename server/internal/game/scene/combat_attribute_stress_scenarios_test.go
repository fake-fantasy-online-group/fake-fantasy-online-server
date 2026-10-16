package scene

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Opt-in, finite synthetic scene stress. Times exclude fixture construction and
// admission, include command validation, target selection, resolution and fanout.
// Real skill catalog and admission; no sockets, persistence or client rendering.
func TestCombatAttributeStressScenarios(t *testing.T) {
	if os.Getenv("FFO_COMBAT_STRESS") == "" {
		t.Skip("set FFO_COMBAT_STRESS=1 for bounded stress")
	}
	rs, template := requirementResolutionFixture(t)
	for _, slot := range []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10} {
		requirementResolutionItem(rs, template, slot, 0, 1)
	}
	rs.items[掉落树枝.ID] = 掉落树枝
	for id, d := range rs.items {
		d.InventoryTab, d.InventoryTabKnown = 2, true
		rs.items[id] = d
	}
	const repetitions = 20
	t.Logf("runtime=%s CPUs=%d GOMAXPROCS=%d repetitions=%d", runtime.Version(), runtime.NumCPU(), runtime.GOMAXPROCS(0), repetitions)
	for _, players := range []int{1, 10, 50} {
		for _, targets := range []int{1, 10, 50, 100, 300} {
			for _, lethal := range []bool{false, true} {
				// Bound total delivered events: large lethal fanout is covered separately.
				if players > 1 && (lethal || targets > 10) {
					continue
				}
				for _, cold := range []bool{false, true} {
					name := fmt.Sprintf("players%d_targets%d_lethal%t_cold%t", players, targets, lethal, cold)
					t.Run(name, func(t *testing.T) {
						durations := make([]time.Duration, 0, repetitions)
						var bytes, allocs uint64
						var deliveries, hits, kills, drops int
						for repeat := 0; repeat < repetitions; repeat++ {
							positions := make([]domain.Pos, targets)
							for i := range positions {
								positions[i] = domain.Pos{MapID: 14, X: 200 + float64(i%10), Y: 100 + float64(i/10)}
							}
							s, ids := skillScene(t, positions...)
							s.skills, s.items, s.levels = rs.skills, rs.items, rewardLevels()
							s.alloc = domain.NewEntityAlloc()
							s.lootTable = map[domain.MonsterID][]domain.DropEntry{1001: {}}
							for i := 0; i < 8; i++ {
								s.lootTable[1001] = append(s.lootTable[1001], domain.DropEntry{Item: 掉落树枝.ID, RatePct: 100})
							}
							ps := make([]*entity.Entity, players)
							sinks := make([]*fakeSink, players)
							skill := domain.SkillID(10402)
							if targets == 1 {
								skill = 10401
							}
							clock := time.Unix(1700000000, 0)
							s.now = func() time.Time { return clock }
							for i := range ps {
								ch := *template.Player.Char
								ch.ID = int64(100 + i)
								ch.Name = fmt.Sprintf("stress%d", i)
								ch.Pos = domain.Pos{MapID: 14, X: 100, Y: 100}
								ch.Skills = domain.Learned{10401: 1, 10402: 1}
								ch.SkillAllocationVersion = domain.SkillFormulaVersion
								sink := &fakeSink{}
								id := domain.EntityID(i + 1)
								s.exec(Enter{ID: id, Char: &ch, Worn: template.Player.Worn, Sink: sink})
								ps[i] = s.players[id]
								sinks[i] = sink
								if ps[i] == nil || ps[i].Player.CombatAttributes == nil || !ps[i].Player.CombatAttributes.Ready {
									t.Fatal("admission failed")
								}
								ps[i].MP = 1000000
								ps[i].MaxMP = 1000000
								if cold {
									s.invalidateCombatAttributes(ps[i])
								}
							}
							for _, id := range ids {
								m := s.entities[id]
								if lethal {
									m.HP = 1
								} else {
									m.HP = 100000000
									m.MaxHP = m.HP
								}
							}
							s.flush()
							for _, sink := range sinks {
								sink.take()
							}
							var before, after runtime.MemStats
							runtime.ReadMemStats(&before)
							start := time.Now()
							for _, p := range ps {
								s.exec(UseSkill{ID: p.ID, Skill: skill, Target: ids[0]})
							}
							clock = clock.Add(5 * time.Second)
							s.stepCastsAt(clock, false)
							clock = clock.Add(5 * time.Second)
							s.stepCastsAt(clock, false)
							s.flush()
							durations = append(durations, time.Since(start))
							runtime.ReadMemStats(&after)
							bytes += after.TotalAlloc - before.TotalAlloc
							allocs += after.Mallocs - before.Mallocs
							if len(s.casts) != 0 {
								t.Fatalf("unresolved casts %d", len(s.casts))
							}
							events := sinks[0].take()
							hits = countOf[event.DamageDealt](events)
							kills = countOf[event.ExpGained](events)
							drops = len(s.ground)
							deliveries = len(events)
							for _, sink := range sinks[1:] {
								other := sink.take()
								deliveries += len(other)
								hits += countOf[event.DamageDealt](other)
							}
							if hits != players*targets {
								t.Fatalf("damage events=%d want%d; events=%v", hits, players*targets, events)
							}
							for _, id := range ids {
								m := s.entities[id]
								if lethal == m.Alive() {
									t.Fatalf("wrong alive state %d lethal%t", id, lethal)
								}
								if !lethal && m.HP >= m.MaxHP {
									t.Fatal("no damage")
								}
							}
							if lethal && (kills != targets || drops != 8*targets) {
								t.Fatalf("kills/drops %d/%d", kills, drops)
							}
							if !lethal && (kills != 0 || drops != 0) {
								t.Fatal("unexpected rewards")
							}
						}
						sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
						var total time.Duration
						for _, d := range durations {
							total += d
						}
						t.Logf("avg=%s p95=%s max=%s B/op=%d allocs/op=%d hits=%d XP=%d drops=%d delivered=%d", total/repetitions, durations[18], durations[19], bytes/repetitions, allocs/repetitions, hits, kills, drops, deliveries)
					})
				}
			}
		}
	}
}
