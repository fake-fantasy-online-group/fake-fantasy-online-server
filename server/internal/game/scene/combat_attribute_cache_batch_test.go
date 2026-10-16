package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// End-to-end reproducer: one lethal AoE, ten XP grants and eighty drops. Keep
// timing informational rather than imposing a machine-dependent CI threshold.
func TestCombatAttributeCacheBatchDeathsDelivery(t *testing.T) {
	s, p, ids, sink := newCombatAttributeBatch(t)
	start := time.Now()
	p.Stats.MAtk = 99999
	s.casts = append(s.casts, pendingCast{src: p.ID, targets: ids, def: 火球术})
	s.stepCasts()
	s.flush()
	elapsed := time.Since(start)
	for _, id := range ids {
		if s.entities[id].Alive() {
			t.Fatal("target survived", id)
		}
	}
	if len(s.ground) != 80 {
		t.Fatalf("ground drops=%d want80", len(s.ground))
	}
	var xp, panels int
	var previous event.Event
	for _, e := range sink.take() {
		switch e.(type) {
		case event.StatsChanged:
			panels++
		case event.ExpGained:
			xp++
			if _, ok := previous.(event.StatsChanged); !ok {
				t.Fatalf("XP delivery order: panels%d XP%d", panels, xp)
			}
		}
		previous = e
	}
	if xp != 10 || panels < 10 {
		t.Fatalf("XP/snapshot counts: %d/%d", xp, panels)
	}
	t.Logf("ten lethal targets, nine equipment slots, eighty drops: %s", elapsed)
}

func newCombatAttributeBatchWorld(t testing.TB) (*Scene, []domain.EntityID, Enter, *fakeSink) {
	t.Helper()
	positions := make([]domain.Pos, 10)
	for i := range positions {
		positions[i] = domain.Pos{MapID: 14, X: 200 + float64(i), Y: 100}
	}
	s, ids := skillScene(t, positions...)
	rs, rp := requirementResolutionFixture(t)
	s.skills, s.items, s.levels = rs.skills, rs.items, rewardLevels()
	s.alloc = domain.NewEntityAlloc()
	s.items[掉落树枝.ID] = 掉落树枝
	s.lootTable = map[domain.MonsterID][]domain.DropEntry{}
	for i := 0; i < 8; i++ {
		s.lootTable[1001] = append(s.lootTable[1001], domain.DropEntry{Item: 掉落树枝.ID, RatePct: 100})
	}
	for _, slot := range []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10} {
		requirementResolutionItem(s, rp, slot, 0, 1)
	}
	for id, d := range s.items {
		d.InventoryTab, d.InventoryTabKnown = 2, true
		s.items[id] = d
	}
	rp.Player.Char.ID, rp.Player.Char.Name = 100, "admitted batch"
	rp.Player.Char.Pos = domain.Pos{MapID: 14, X: 100, Y: 100}
	sink := &fakeSink{}
	return s, ids, Enter{ID: 1, Char: rp.Player.Char, Worn: rp.Player.Worn, Sink: sink}, sink
}

func newCombatAttributeBatch(t testing.TB) (*Scene, *entity.Entity, []domain.EntityID, *fakeSink) {
	t.Helper()
	s, ids, cmd, sink := newCombatAttributeBatchWorld(t)
	s.exec(cmd)
	s.flush()
	sink.take()
	p := s.players[cmd.ID]
	if p == nil || p.Player.CombatAttributes == nil || !p.Player.CombatAttributes.Ready {
		t.Fatal("admission did not initialize attributes")
	}
	return s, p, ids, sink
}

// Full admission is reported separately; fixture/data construction is untimed.
func BenchmarkCombatAttributeAdmission(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		s, _, cmd, _ := newCombatAttributeBatchWorld(b)
		b.StartTimer()
		s.exec(cmd)
		s.flush()
		b.StopTimer()
		p := s.players[cmd.ID]
		if p == nil || p.Player.CombatAttributes == nil || !p.Player.CombatAttributes.Ready {
			b.Fatal("admission did not initialize attributes")
		}
	}
}

// Both cases use real admission. The ordinary case performs no extra warming;
// forced_rebuild deliberately invalidates the admitted attributes to report the
// separate rebuild cost. Both resolve ten kills and emit all eighty drops.
func BenchmarkCombatAttributeBatchDeaths(b *testing.B) {
	for _, warm := range []bool{false, true} {
		name := "forced_rebuild"
		if warm {
			name = "admitted_player"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				s, p, ids, sink := newCombatAttributeBatch(b)
				if !warm {
					s.invalidateCombatAttributes(p)
				}
				owned, revision := p.Player.CombatAttributes, p.Player.CombatAttributes.Revision
				p.Stats.MAtk = 99999
				b.StartTimer()
				s.casts = append(s.casts, pendingCast{src: p.ID, targets: ids, def: 火球术})
				s.stepCasts()
				s.flush()
				b.StopTimer()
				for _, id := range ids {
					if s.entities[id].Alive() {
						b.Fatal("target survived", id)
					}
				}
				if warm && (p.Player.CombatAttributes != owned || owned.Revision != revision) {
					b.Fatal("first combat rebuilt admitted attributes")
				}
				if len(s.ground) != 80 {
					b.Fatal("incorrect ground drops", len(s.ground))
				}
				if got := countOf[event.ExpGained](sink.take()); got != 10 {
					b.Fatal("incorrect XP count", got)
				}
			}
		})
	}
}

func BenchmarkCombatAttributeCacheNineSlots(b *testing.B) {
	s, p := requirementResolutionFixture(b)
	for _, slot := range []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10} {
		requirementResolutionItem(s, p, slot, 0, 1)
	}
	b.Run("warm_snapshot", func(b *testing.B) {
		s.combatGrowthStats(p)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.attributeSnapshot(p)
		}
	})
	b.Run("cold_snapshot", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.invalidateCombatAttributes(p)
			s.attributeSnapshot(p)
		}
	})
}
