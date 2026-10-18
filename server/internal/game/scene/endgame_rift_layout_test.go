package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"reflect"
	"testing"
)

func seededRiftLayoutFixture(t *testing.T, floor int32) (*Scene, domain.RiftFloorPlan) {
	t.Helper()
	s, _, now := riftFixture(t)
	layout := domain.RiftFloorTemplate{MapID: 7, MapName: "原生森林", Monsters: []domain.MonsterID{501}, Guardians: []domain.MonsterID{502}}
	for i := 0; i < 100; i++ {
		layout.Anchors = append(layout.Anchors, domain.SpawnPoint{ID: int32(i + 1), Monster: 501, Pos: domain.Pos{MapID: 7, X: 100 + float64(i%10)*160, Y: 100 + float64(i/10)*160}, Dir: 67})
	}
	s.riftLayouts = []domain.RiftFloorTemplate{layout}
	s.walkable = func(p domain.Pos) bool { return p.MapID == 7 && p.X >= 0 && p.X < 2000 && p.Y >= 0 && p.Y < 2000 }
	s.defs = spawn.MapDefs{501: {ID: 501, Sprite: 600, Level: 1, HP: 99999, Stats: domain.Stats{MinAtk: 9999, MaxAtk: 9999}, Kind: domain.MonsterNormal}, 502: {ID: 502, Sprite: 601, Level: 150, HP: 9999999, Kind: domain.MonsterBoss}}
	normal := s.trial.Monsters[60][1]
	normal.Stats = domain.Stats{MaxHP: 100, MinAtk: 30, MaxAtk: 40, MAtk: 20, MoveSpeed: 120}
	s.trial.Monsters[60][1] = normal
	plan, err := s.planEndgameRiftFloor("seeded-scene", floor, 0)
	if err != nil {
		t.Fatal(err)
	}
	run, err := domain.NewRiftRun("seeded-scene", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, *now, riftRulesFixture())
	if err != nil {
		t.Fatal(err)
	}
	s.initEndgameRift(endgameRiftPersisted{Version: 1, Run: run, MapID: 7, Floor: &plan})
	run.Activate(*now)
	return s, plan
}
func TestSeededRiftPopulationUsesFrozenPlanAndScaledBases(t *testing.T) {
	s, plan := seededRiftLayoutFixture(t, 1)
	original, _ := s.defs.Def(501)
	s.populateEndgameRift(false)
	if len(s.monsters) != len(plan.Spawns) || s.rift.data.Run.Phase != domain.RiftActive {
		t.Fatalf("spawned %d, want %d", len(s.monsters), len(plan.Spawns))
	}
	classes := map[int32]int{}
	for id, m := range s.monsters {
		class := s.rift.data.Run.Monsters[id].Class
		classes[class]++
		if m.Look.ModelID != 600 || m.Level != 60 {
			t.Fatal("lost existing visual or frozen level")
		}
		if m.MaxHP != domain.RiftScaledStat(100, 1, 1, class, true, riftRulesFixture()) || m.Stats.MinAtk != domain.RiftScaledStat(30, 1, 1, class, false, riftRulesFixture()) {
			t.Fatal("natural map stats changed rift difficulty", m.MaxHP, m.Stats)
		}
		if class > 0 && (m.Monster.Kind != domain.MonsterElite || s.rift.abilities[id] == nil) {
			t.Fatal("missing native elite or deterministic affix")
		}
		if m.Pos == plan.Entry || m.Pos == plan.Exit {
			t.Fatal("unsafe spawn")
		}
	}
	if classes[0] == 0 || classes[1] == 0 || classes[2] == 0 {
		t.Fatal("missing population classes")
	}
	base := s.defs.(endgameRiftDefs).base
	unchanged, _ := base.Def(501)
	if !reflect.DeepEqual(original, unchanged) {
		t.Fatal("mutated shared native definition")
	}
	s.clearRiftMonsters()
	s.rift.data.Run.Phase = domain.RiftBoss
	s.populateEndgameRift(true)
	if len(s.monsters) != 1 {
		t.Fatal("missing guardian")
	}
	for id, m := range s.monsters {
		if m.Monster.Kind != domain.MonsterBoss || m.Look.ModelID != 601 || m.Pos != plan.Exit || s.rift.data.Run.Monsters[id].Class != 3 || s.rift.abilities[id] == nil {
			t.Fatal("invalid guardian")
		}
	}
}
func TestSeededRiftFloorIndexDoesNotScaleCombat(t *testing.T) {
	for _, floor := range []int32{1, 2, 1000} {
		s, _ := seededRiftLayoutFixture(t, floor)
		s.populateEndgameRift(false)
		for id, m := range s.monsters {
			class := s.rift.data.Run.Monsters[id].Class
			if m.MaxHP != domain.RiftScaledStat(100, 1, 1, class, true, riftRulesFixture()) {
				t.Fatal("floor changed tier scaling")
			}
		}
	}
}
func TestSeededRiftInvalidPopulationFailsAtomically(t *testing.T) {
	for _, bad := range []string{"mask", "blocked", "asset", "version"} {
		t.Run(bad, func(t *testing.T) {
			s, _ := seededRiftLayoutFixture(t, 1)
			switch bad {
			case "mask":
				s.walkable = nil
			case "blocked":
				s.walkable = func(domain.Pos) bool { return false }
			case "asset":
				s.rift.data.Floor.Spawns[len(s.rift.data.Floor.Spawns)-1].Point.Monster = 999
			case "version":
				s.rift.data.Floor.GeneratorVersion = domain.RiftFloorGeneratorVersion + 1
			}
			s.populateEndgameRift(false)
			if len(s.monsters) != 0 || s.rift.data.Run.Phase != domain.RiftFailed || s.rift.data.Run.Reason != "spawn_failed" {
				t.Fatal("invalid floor partially populated")
			}
		})
	}
}

func TestSeededRiftDensePopulationUsesFullPlanAtLowAndMaxTier(t *testing.T) {
	for _, tier := range []int32{1, domain.RiftMaxTier} {
		s, _ := seededRiftLayoutFixture(t, 1)
		layout := &s.riftLayouts[0]
		layout.WalkableArea = 1000 * domain.RiftFloorAreaPerMonster
		for i := 0; i < 400; i++ {
			layout.Population = append(layout.Population, domain.SpawnPoint{ID: -int32(i + 1), Pos: domain.Pos{MapID: 7, X: float64(i%20)*80 + 32, Y: float64(i/20)*80 + 32}, Dir: 67})
		}
		plan, err := s.planEndgameRiftFloor("dense-scene", 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		s.rift.data.Floor, s.rift.data.Run.Tier = &plan, tier
		s.populateEndgameRift(false)
		if len(s.monsters) != len(plan.Spawns) || len(s.monsters) < 144 || s.rift.data.Run.Phase != domain.RiftActive {
			t.Fatalf("dense plan not fully spawned at tier %d: %d", tier, len(s.monsters))
		}
		seen := map[domain.Pos]bool{}
		for id, m := range s.monsters {
			class := s.rift.data.Run.Monsters[id].Class
			if seen[m.Pos] || !s.walkable(m.Pos) || m.MaxHP != domain.RiftScaledStat(100, tier, 1, class, true, riftRulesFixture()) {
				t.Fatal("density changed geometry or difficulty stats")
			}
			seen[m.Pos] = true
		}
	}
}

func TestSeededRiftSavedVersionOnePopulationNotExpanded(t *testing.T) {
	s, plan := seededRiftLayoutFixture(t, 1)
	plan.GeneratorVersion, plan.Seed, plan.Spawns = 1, 123456, plan.Spawns[:48]
	s.rift.data.Floor = &plan
	s.populateEndgameRift(false)
	if len(s.monsters) != 48 || s.rift.data.Run.Phase != domain.RiftActive || s.rift.data.Floor.Seed != 123456 {
		t.Fatal("saved version one plan was rejected or regenerated")
	}
}

// The map pool is geometry/content based. Character level and challenge tier
// may scale combat, but must not select a different set of native maps.
func TestRiftMapSelectionUsesFullPoolIndependentOfLevelAndTier(t *testing.T) {
	s, _ := seededRiftLayoutFixture(t, 1)
	s.riftLayouts = riftFloorTemplatesForTest()
	s.riftLayouts = append(s.riftLayouts, domain.RiftFloorTemplate{MapID: 999}) // Invalid maps stay excluded.
	seen := map[int32]bool{}
	for floor := int32(1); floor <= 100; floor++ {
		var expected domain.RiftFloorPlan
		for _, level := range []int32{12, 60, 100} {
			for _, tier := range []int32{1, 30, 150} {
				s.rift.data.Run.Members[100].Level = level
				s.rift.data.Run.Tier = tier
				plan, err := s.planEndgameRiftFloor("all-levels-full-pool", floor, 0)
				if err != nil || !plan.Valid() || (plan.MapID != 7 && plan.MapID != 8) {
					t.Fatalf("level %d tier %d: invalid plan or map: %+v %v", level, tier, plan, err)
				}
				if expected.MapID == 0 {
					expected = plan
				} else if !reflect.DeepEqual(plan, expected) {
					t.Fatal("character level or tier changed seeded map, portals, or population")
				}
				seen[plan.MapID] = true
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("not every viable map was reachable: %v", seen)
	}
}
