package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"io"
	"log/slog"
	"runtime"
	"sort"
	"testing"
	"time"
)

type lifecycleMeasure struct {
	times         []time.Duration
	bytes, allocs uint64
}

func (m *lifecycleMeasure) run(fn func()) {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	start := time.Now()
	fn()
	m.times = append(m.times, time.Since(start))
	runtime.ReadMemStats(&b)
	m.bytes += b.TotalAlloc - a.TotalAlloc
	m.allocs += b.Mallocs - a.Mallocs
}
func (m *lifecycleMeasure) report(t *testing.T, label string) {
	sort.Slice(m.times, func(i, j int) bool { return m.times[i] < m.times[j] })
	var sum time.Duration
	for _, d := range m.times {
		sum += d
	}
	n := len(m.times)
	t.Logf("%s samples=%d mean=%s p95=%s max=%s bytes/batch=%d allocs/batch=%d", label, n, sum/time.Duration(n), m.times[(95*n+99)/100-1], m.times[n-1], m.bytes/uint64(n), m.allocs/uint64(n))
}
func lifecycleScene(id int32) *Scene {
	return New(Config{ID: domain.SceneID{MapID: id}, EndgameCatalog: catalogfixture.EndgameCatalog(), Items: map[domain.ItemID]domain.ItemDef{场景钢剑.ID: 场景钢剑, 掉落树枝.ID: 掉落树枝}, Alloc: domain.NewEntityAlloc(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}
func lifecycleEnter(id int, mapID int32, sink *fakeSink) Enter {
	w := domain.NewEquipSet()
	w.Set(domain.SlotWeapon, domain.NewStack(场景钢剑, 1))
	return Enter{ID: domain.EntityID(id), Char: &domain.Character{ID: int64(id), Name: fmt.Sprintf("stress-%d", id), Level: 10, Base: domain.Base{STR: 20, AGI: 20, VIT: 20, INT: 20, SPI: 20, DEX: 20}, Pos: domain.Pos{MapID: mapID, X: 100, Y: 100}}, Worn: w, Sink: sink}
}
func lifecycleEmpty(t *testing.T, s *Scene) {
	t.Helper()
	if len(s.players) != 0 || len(s.combatAttributes) != 0 || len(s.skillRawCache) != 0 || len(s.mapLoads) != 0 || len(s.hits) != 0 {
		t.Fatalf("retained player state: players=%d attributes=%d skills=%d loads=%d hits=%d", len(s.players), len(s.combatAttributes), len(s.skillRawCache), len(s.mapLoads), len(s.hits))
	}
}

// Admission runs real Enter processing and event delivery, excluding fixture setup.
// Transfer uses the production router handoff queue with deterministic manual drain.
// These are scene tests, not TCP/authentication/database throughput measurements.
func TestStressLifecycleAdmissionTransferChurn(t *testing.T) {
	for _, n := range []int{10, 50, 100} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var admit, churn, transfer, leave lifecycleMeasure
			for sample := 0; sample < 20; sample++ {
				a, b := lifecycleScene(7), lifecycleScene(14)
				r := NewRouter(nil, nil)
				r.scenes[a.id] = a
				r.scenes[b.id] = b
				a.router = r
				b.router = r
				cmds := make([]Enter, n)
				sinks := make([]*fakeSink, n)
				for i := range cmds {
					sinks[i] = &fakeSink{}
					cmds[i] = lifecycleEnter(i+1, 7, sinks[i])
				}
				admit.run(func() {
					for _, cmd := range cmds {
						a.exec(cmd)
					}
					a.flush()
				})
				for i := 1; i <= n; i++ {
					p := a.players[domain.EntityID(i)]
					if p == nil || p.Player.CombatAttributes == nil || !p.Player.CombatAttributes.Ready || p.HP <= 0 {
						t.Fatalf("admission %d invalid", i)
					}
					evs := sinks[i-1].take()
					if countOf[event.EntitySpawned](evs) != n-1 {
						t.Fatalf("admission observer %d got %d want %d peer spawns", i, countOf[event.EntitySpawned](evs), n-1)
					}
				}
				churn.run(func() {
					for round := 0; round < 5; round++ {
						for i := 1; i <= n; i++ {
							id := domain.EntityID(i)
							p := a.players[id]
							a.exec(Unequip{ID: id, Slot: domain.SlotWeapon})
							bagSlot := -1
							p.Player.Bag.Each(func(slot int, st domain.Stack) {
								if st.Item == 场景钢剑.ID {
									bagSlot = slot
								}
							})
							a.exec(Equip{ID: id, BagSlot: bagSlot})
							p.Status.Apply(domain.StatusDef{ID: 990001, Level: 1, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 3}}}, a.tick, id)
							a.refreshStats(p)
							p.Status.Remove(990001)
							a.refreshStats(p)
						}
						a.flush()
						for _, s := range sinks {
							s.take()
						}
					}
				})
				for i := 1; i <= n; i++ {
					p := a.players[domain.EntityID(i)]
					if p.Player.Worn.At(domain.SlotWeapon).Item != 场景钢剑.ID || p.Player.Bag.CountOf(场景钢剑.ID) != 0 {
						t.Fatal("equipment churn duplicated/lost item")
					}
					assertCombatAttributeCacheFresh(t, a, p)
				}
				transfer.run(func() {
					for round := 0; round < 5; round++ {
						for i := 1; i <= n; i++ {
							id := domain.EntityID(i)
							old := a.combatAttributes[id]
							if !a.onTeleport(Teleport{ID: id, To: b.id, At: domain.Pos{MapID: b.id.MapID, X: 100, Y: 100}}) {
								t.Fatal("transfer rejected")
							}
							if a.players[id] != nil || a.combatAttributes[id] != nil {
								t.Fatal("source retained ownership")
							}
							b.drain()
							p := b.players[id]
							state := b.combatAttributes[id]
							if p == nil || state == nil || state == old || state.owner != p || state.values != p.Player.CombatAttributes || !state.values.Ready {
								t.Fatal("transfer owner/cache invalid")
							}
						}
						b.flush()
						for _, s := range sinks {
							s.take()
						}
						a, b = b, a
					}
				})
				leave.run(func() {
					for i := 1; i <= n; i++ {
						a.exec(Leave{ID: domain.EntityID(i), Reason: "stress complete"})
					}
					a.flush()
				})
				lifecycleEmpty(t, a)
				lifecycleEmpty(t, b)
				if len(a.entities) != 0 || len(b.entities) != 0 {
					t.Fatal("entities remain after leave")
				}
			}
			admit.report(t, "admission")
			churn.report(t, "five equip/status churn rounds")
			transfer.report(t, "five transfers/player")
			leave.report(t, "leave")
			t.Log("retained players/entities/attribute dependencies/raw skill ranks/map loads/hits after leave: zero")
		})
	}
}

func TestStressGroundLootBurstPickup(t *testing.T) {
	for _, n := range []int{100, 500, 1000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var spawn, pickup lifecycleMeasure
			for sample := 0; sample < 20; sample++ {
				s := lifecycleScene(7)
				sink := &fakeSink{}
				s.exec(lifecycleEnter(1, 7, sink))
				s.flush()
				sink.take()
				p := s.players[1]
				spawn.run(func() {
					for i := 0; i < n; i++ {
						s.spawnLoot(掉落树枝, p.Pos, p.ID)
					}
					s.flush()
				})
				evs := sink.take()
				if len(s.ground) != n || countOf[event.EntitySpawned](evs) != n {
					t.Fatal("spawn/event count mismatch")
				}
				ids := make([]domain.EntityID, 0, n)
				for id := range s.ground {
					ids = append(ids, id)
				}
				sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
				pickup.run(func() {
					for _, id := range ids {
						s.exec(PickUp{ID: 1, Target: id})
					}
					s.flush()
				})
				if len(s.ground) != 0 || countOf[event.EntityDespawned](sink.take()) != n {
					t.Fatalf("pickup mismatch ground=%d", len(s.ground))
				}
				total := p.Player.Bag.CountOf(掉落树枝.ID)
				if total != int32(n) {
					t.Fatalf("item conservation=%d want%d", total, n)
				}
				s.exec(PickUp{ID: 1, Target: ids[0]})
				s.flush()
				if countOf[event.Rejected](sink.take()) != 1 {
					t.Fatal("duplicate pickup accepted")
				}
				s.exec(Leave{ID: 1})
				lifecycleEmpty(t, s)
				if len(s.entities) != 0 {
					t.Fatal("entities remain")
				}
				if s.timer.pending() != n {
					t.Fatalf("TTL timer count=%d want%d", s.timer.pending(), n)
				}
				s.timer.advance(s.tick + lootTTLTicks)
				if s.timer.pending() != 0 {
					t.Fatal("TTL callbacks not drained")
				}
			}
			spawn.report(t, "loot spawn+flush")
			pickup.report(t, "pickup+delivery")
			t.Logf("post-pickup TTL callbacks=%d; after TTL zero; post-leave entities/player caches zero", n)
		})
	}
}
