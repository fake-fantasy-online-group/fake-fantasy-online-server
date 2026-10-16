package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"runtime"
	"sort"
	"testing"
	"time"
)

// This bounded in-memory stress harness does not use PostgreSQL or network sinks.
// Status-heavy variants duplicate a known poison fixture under distinct IDs to
// exercise independent status bookkeeping; they are saturation tests, not builds.
func stressStatusAuraMeasure(t *testing.T, label string, samples int, f func(int)) {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	ds := make([]time.Duration, samples)
	for i := range ds {
		start := time.Now()
		f(i)
		ds[i] = time.Since(start)
	}
	runtime.ReadMemStats(&after)
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	t.Logf("MATRIX %s samples=%d mean_us=%.3f p95_us=%.3f max_us=%.3f allocs/op=%.2f bytes/op=%.2f", label, samples, float64(sum.Nanoseconds())/float64(samples)/1000, float64(ds[(samples*95-1)/100].Nanoseconds())/1000, float64(ds[len(ds)-1].Nanoseconds())/1000, float64(after.Mallocs-before.Mallocs)/float64(samples), float64(after.TotalAlloc-before.TotalAlloc)/float64(samples))
}

func TestStressStatusAuraDOTMatrix(t *testing.T) {
	for _, n := range []int{10, 50, 100, 300} {
		for _, k := range []int{1, 5, 32} {
			t.Run(fmt.Sprintf("targets_%d_statuses_%d", n, k), func(t *testing.T) {
				s := newCombatScene(t)
				dueCount := k
				if k == 5 {
					dueCount = 1
				}
				targets := make([]*entity.Entity, n)
				for i := range targets {
					e := putMonster(s, domain.EntityID(1000+i), 1000000, domain.Stats{}, 100+float64(i%10), 100)
					e.Status = domain.NewStatusSet()
					targets[i] = e
					for j := 0; j < k; j++ {
						d := 中毒
						if k == 32 {
							d.ID = domain.StatusID(900000 + j)
						}
						if k == 5 {
							d = []domain.StatusDef{中毒, 昏迷, 封印, 衰弱, 狂暴}[j]
						}
						d.DurationSec = 1000
						e.Status.Apply(d, s.tick, e.ID)
					}
				}
				s.outbox = nil
				stressStatusAuraMeasure(t, fmt.Sprintf("dot targets=%d statuses_each=%d due_each=%d", n, k, dueCount), 64, func(i int) { s.tick += domain.Ticks(2000); s.outbox = s.outbox[:0]; s.stepStatus() })
				for _, e := range targets {
					if want := int32(1000000 - 64*dueCount*10); e.HP != want {
						t.Fatalf("target %d hp=%d want=%d", e.ID, e.HP, want)
					}
				}
				damage := 0
				for _, p := range s.outbox {
					if _, ok := p.ev.(event.DamageDealt); ok {
						damage++
					}
				}
				if damage != n*dueCount {
					t.Fatalf("damage receipts %d want %d", damage, n*dueCount)
				}
				s.tick = domain.Ticks(1000001)
				s.outbox = s.outbox[:0]
				s.stepStatus()
				for _, e := range targets {
					if e.Status.Count() != 0 {
						t.Fatal("expired status retained")
					}
				}
				removed := 0
				for _, p := range s.outbox {
					if _, ok := p.ev.(event.StatusRemoved); ok {
						removed++
					}
				}
				if removed != n*k {
					t.Fatalf("removal receipts %d want %d", removed, n*k)
				}
			})
		}
	}
}

func TestStressStatusAuraOverlapMatrix(t *testing.T) { stressStatusAuraOverlapMatrix(t, 0) }

func TestStressStatusAuraPartySixMatrix(t *testing.T) { stressStatusAuraOverlapMatrix(t, 6) }

func stressStatusAuraOverlapMatrix(t *testing.T, partySize int) {
	for _, n := range []int{10, 50, 100, 300} {
		t.Run(fmt.Sprintf("players_%d", n), func(t *testing.T) {
			s := newCombatScene(t)
			players := make([]*entity.Entity, n)
			for i := range players {
				id := domain.EntityID(i + 1)
				join(t, s, id, int64(100+i), "aura stress", 100+float64(i%10), 100)
				p := s.players[id]
				p.Player.Party = 1
				if partySize > 0 {
					p.Player.Party = domain.PartyID(1 + i/partySize)
				}
				players[i] = p
				wearFormationAuraTest(t, s, p, 16, int64(9000+i))
			}
			aura := formationAuraTest(t, 16)
			s.stepEquipmentAuras()
			for _, p := range players {
				if got := s.equipmentDefense(p, false).PhysicalMitigationBP; got != aura.Value {
					t.Fatalf("overlap stacked or absent: %d want %d", got, aura.Value)
				}
			}
			s.outbox = nil
			stressStatusAuraMeasure(t, fmt.Sprintf("aura_steady players_sources=%d party_size=%d", n, partySize), 64, func(i int) { s.outbox = s.outbox[:0]; s.stepEquipmentAuras() })
			stressStatusAuraMeasure(t, fmt.Sprintf("aura_refresh players_sources=%d party_size=%d", n, partySize), 32, func(i int) {
				s.tick += domain.Ticks(int(aura.RefreshMS))
				s.outbox = s.outbox[:0]
				s.stepEquipmentAuras()
			})
			// Moving a recipient away must immediately invalidate grants from other sources.
			q := players[n-1]
			s.aoi.Move(q, domain.Pos{MapID: 7, X: 10000, Y: 10000})
			s.stepEquipmentAuras()
			for _, g := range s.equipmentState().auras[q.ID] {
				if g.owner != q.ID {
					t.Fatal("out-of-range external aura retained")
				}
			}
			// Source death plus lease expiry must leave no defense or visual status behind.
			for _, p := range players {
				p.HP = 0
			}
			s.tick += domain.Ticks(int(aura.LeaseMS)) + 1
			s.outbox = s.outbox[:0]
			s.stepEquipmentAuras()
			if len(s.equipmentState().auras) != 0 || len(s.equipmentState().auraSchedules) != 0 {
				t.Fatal("dead sources retained aura leases")
			}
			for _, p := range players {
				if s.equipmentDefense(p, false).PhysicalMitigationBP != 0 || len(s.equipmentAuraStatuses(p)) != 0 {
					t.Fatal("stale defense or visual aura")
				}
			}
			removed := 0
			for _, p := range s.outbox {
				if ev, ok := p.ev.(event.EntityStatusEffect); ok && ev.DurationMS == 0 && ev.Effect == aura.VisualKey {
					removed++
				}
			}
			if removed < n {
				t.Fatalf("missing aura removal events: %d want >=%d", removed, n)
			}
		})
	}
}

func TestStressStatusAuraDenseAIMatrix(t *testing.T) {
	for _, n := range []int{10, 50, 100, 300} {
		t.Run(fmt.Sprintf("monsters_%d", n), func(t *testing.T) {
			s := newCombatScene(t)
			for i := 0; i < 10; i++ {
				join(t, s, domain.EntityID(i+1), int64(100+i), "AI target", 100+float64(i), 100)
				p := s.players[domain.EntityID(i+1)]
				p.HP = 1000000
				p.MaxHP = 1000000
			}
			for i := 0; i < n; i++ {
				m := putMonster(s, domain.EntityID(1000+i), 246, 主动树妖.Stats, 300+float64(i%10), 100)
				m.Monster.TypeID = 主动树妖.ID
				m.Monster.Aggressive = true
				m.Monster.ViewDist = 300
				m.Monster.TraceDist = 600
				s.monsters[m.ID] = m
			}
			acquired, moved := 0, 0
			stressStatusAuraMeasure(t, fmt.Sprintf("ai_acquire_chase monsters=%d players=10", n), 64, func(i int) {
				// Reset to identical dense acquisition geometry, preserving staggered scan IDs.
				s.tick += aiScanEvery
				s.outbox = s.outbox[:0]
				s.attacks = s.attacks[:0]
				for _, m := range s.monsters {
					m.Monster.Target = 0
					m.Monster.Roaming = false
					s.aoi.Move(m, domain.Pos{MapID: 7, X: 300, Y: 100})
				}
				s.stepAI()
				for _, m := range s.monsters {
					if m.Monster.Target != 0 {
						acquired++
					}
					if m.Pos.X < 300 {
						moved++
					}
				}
			})
			if acquired == 0 || moved == 0 {
				t.Fatalf("AI did no real acquisition/chase acquired=%d moved=%d", acquired, moved)
			}
			t.Logf("AI correctness acquired=%d moved=%d (staggered subset scans each sample)", acquired, moved)
		})
	}
}
