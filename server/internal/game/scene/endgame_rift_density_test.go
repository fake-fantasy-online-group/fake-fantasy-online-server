package scene

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func doubledRiftLayoutFixture(t *testing.T, floor, tier int32) (*Scene, domain.RiftFloorPlan, domain.RiftFloorPlan) {
	t.Helper()
	s, _ := seededRiftLayoutFixture(t, floor)
	layout := &s.riftLayouts[0]
	layout.WalkableArea = 1000 * domain.RiftFloorAreaPerMonster
	for i := 0; i < 400; i++ {
		layout.Population = append(layout.Population, domain.SpawnPoint{ID: -int32(i + 1), Pos: domain.Pos{MapID: 7, X: float64(i%20)*80 + 32, Y: float64(i/20)*80 + 32}, Dir: 67})
	}
	base, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, floor, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Run.Rules.FloorPopulationMultiplier = 2
	s.rift.data.Run.Tier = tier
	dense, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, floor, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Floor = &dense
	return s, base, dense
}

func TestSeededRiftDoubleDensityAtLowMaxTierAndFloor(t *testing.T) {
	for _, tier := range []int32{1, 150} {
		for _, floor := range []int32{1, 150} {
			t.Run(fmt.Sprintf("tier%d-floor%d", tier, floor), func(t *testing.T) {
				s, base, dense := doubledRiftLayoutFixture(t, floor, tier)
				s.populateEndgameRift(false)
				if len(s.monsters) != 2*len(base.Spawns) || len(s.rift.data.Run.Monsters) != len(s.monsters) || len(dense.Spawns) != len(s.monsters) {
					t.Fatal("entity allocation or population count lost actors")
				}
				before, after := [3]int{}, [3]int{}
				for _, a := range base.Spawns {
					before[a.Class]++
				}
				seen := map[domain.Pos]bool{}
				for id, m := range s.monsters {
					class := s.rift.data.Run.Monsters[id].Class
					after[class]++
					if seen[m.Pos] || !s.walkable(m.Pos) || m.MaxHP != domain.RiftScaledStat(100, tier, 1, class, true, riftRulesFixture()) {
						t.Fatal("position or per-actor stats changed")
					}
					seen[m.Pos] = true
					if class > 0 && (s.rift.abilities[id] == nil || len(s.rift.abilities[id].pattern) != int(class)) {
						t.Fatal("blue/yellow affix count changed")
					}
				}
				for i := range before {
					if after[i] != 2*before[i] {
						t.Fatal("composition not exactly doubled", before, after)
					}
				}
				snapshot := s.rift.data.clone()
				a, _ := json.Marshal(s.rift.data)
				b, _ := json.Marshal(snapshot)
				if string(a) != string(b) {
					t.Fatal("reload changed run, keys, rules or plan")
				}
				// The first half retains exact v2 spawn indexes, so its per-actor affix seeds
				// and assignment remain unchanged. All new actors have independent IDs.
				original := map[domain.Pos]domain.RiftFloorSpawn{}
				for _, a := range base.Spawns {
					original[a.Point.Pos] = a
				}
				for id, m := range s.monsters {
					if a, ok := original[m.Pos]; ok && a.Class > 0 {
						index := -1
						for i, p := range base.Spawns {
							if p.Point.Pos == m.Pos {
								index = i
								break
							}
						}
						previous := *s.rift.abilities[id]
						s.registerEndgameRiftAbilities(m, a.Class, index, base.Seed, floor)
						if !reflect.DeepEqual(previous, *s.rift.abilities[id]) {
							t.Fatal("original affix assignment changed")
						}
					}
				}
				s.clearRiftMonsters()
				s.rift.data.Run.Phase = domain.RiftBoss
				s.populateEndgameRift(true)
				if len(s.monsters) != 1 {
					t.Fatal("guardian was multiplied")
				}
			})
		}
	}
}

func TestRiftDensityIndependentRuntimeTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("timing diagnostic")
	}
	for _, density := range []int32{1, 2} {
		s, base, dense := doubledRiftLayoutFixture(t, 1, 1)
		if density == 1 {
			s.rift.data.Floor = &base
			s.rift.data.Run.Rules.FloorPopulationMultiplier = 0
		} else {
			s.rift.data.Floor = &dense
		}
		defs := s.defs
		start := time.Now()
		const rounds = 100
		for i := 0; i < rounds; i++ {
			s.populateEndgameRift(false)
			s.clearRiftMonsters()
			s.rift.data.Run.Monsters = map[domain.EntityID]*domain.RiftMonster{}
			s.defs = defs
			s.outbox = s.outbox[:0]
		}
		elapsed := time.Since(start)
		t.Logf("density x%d population %d: spawn+clear %d samples mean %s", density, len(s.rift.data.Floor.Spawns), rounds, elapsed/rounds)
	}
}

func TestRiftDensitySceneUsesFrozenRunRules(t *testing.T) {
	s, base, _ := doubledRiftLayoutFixture(t, 1, 1)
	old := *riftRulesFixture()
	fresh := old
	fresh.Version = 2
	fresh.FloorPopulationMultiplier = 2
	table, err := domain.NewRiftRuleTable(2, []domain.RiftRules{old, fresh}, lootRulesTableFixture(), balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	s.riftRules = table
	s.rift.data.Run.Rules = &old
	oldNext, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, 1, 0)
	if err != nil || !reflect.DeepEqual(base, oldNext) {
		t.Fatal("active density leaked into existing run")
	}
	saved := s.rift.data.record(1)
	restored, ok := decodeEndgameRift(&saved, table)
	if !ok || restored.Run.Rules.FloorPopulationMultiplier != 0 {
		t.Fatal("old run reload rebound to new density")
	}
	s.rift = nil
	newPlan, err := s.planEndgameRiftFloor("seeded-scene", 1, 0)
	if err != nil || newPlan.GeneratorVersion != 3 {
		t.Fatal("new run did not use published density", err)
	}
}

func TestRiftDoubleDensityDeathsRemainExactlyOnce(t *testing.T) {
	s, _, _ := doubledRiftLayoutFixture(t, 1, 1)
	s.populateEndgameRift(false)
	expected := 0
	var normal domain.EntityID
	for id, m := range s.monsters {
		class := s.rift.data.Run.Monsters[id].Class
		if class == 0 {
			normal = id
			continue
		}
		if class == 1 {
			expected += int(s.currentRiftRules().BlueOrbs)
		} else {
			expected += int(s.currentRiftRules().GoldOrbs)
		}
		death := endgameRiftDeath{ID: id, Pos: m.Pos, MS: s.riftMS()}
		s.applyEndgameRiftDeath(death)
		s.applyEndgameRiftDeath(death)
	}
	if len(s.rift.data.Run.Globes) != expected {
		t.Fatal("duplicate deaths added or lost progress orbs", len(s.rift.data.Run.Globes), expected)
	}
	for id, g := range s.rift.data.Run.Globes {
		if id == 0 || g.ID != id || s.rift.data.Run.Monsters[id] != nil {
			t.Fatal("orb and monster identities collided")
		}
	}
	death := endgameRiftDeath{ID: normal, Pos: s.monsters[normal].Pos, MS: s.riftMS()}
	s.applyEndgameRiftDeath(death)
	s.applyEndgameRiftDeath(death)
	if s.rift.data.Run.Progress != s.currentRiftRules().NormalProgress || s.rift.data.GrantsReady {
		t.Fatal("normal progress or final rewards duplicated")
	}
}
