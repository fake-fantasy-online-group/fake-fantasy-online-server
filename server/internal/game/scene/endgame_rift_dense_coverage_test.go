package scene

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func denseCoverageSceneFixture(t *testing.T, width, height int, tier, floor int32) (*Scene, domain.RiftFloorPlan) {
	t.Helper()
	s, _ := seededRiftLayoutFixture(t, floor)
	legacy := riftEliteRulesFixture(t)
	legacy.Version = 8
	legacy.FloorLayoutVersion = domain.RiftFloorCoverageVersion
	legacy.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []domain.MonsterID{1011}, Items: []domain.ItemID{1234}}}
	fresh := *legacy
	fresh.Version = 9
	fresh.FloorLayoutVersion = domain.RiftFloorDenseCoverageVersion
	items := balanceItemsFixture()
	items[1234] = domain.ItemDef{ID: 1234, Equip: &domain.EquipDef{LevelReq: 40}}
	var err error
	s.riftRules, err = domain.NewRiftRuleTable(fresh.Version, []domain.RiftRules{*riftRulesFixture(), *legacy, fresh}, lootRulesTableFixture(), items)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Run.Rules, s.rift.data.Run.RulesVersion = &fresh, fresh.Version
	s.rift.data.Run.Tier = tier
	defs := s.defs.(spawn.MapDefs)
	for _, g := range fresh.Elite.Guardians {
		defs[g.MonsterID] = domain.MonsterDef{ID: g.MonsterID, Name: "原生守关首领", Kind: domain.MonsterBoss, Sprite: 211, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}}}}
	}
	region := domain.RiftCombatRegion{CombatArea: float64(width * height * 64 * 64)}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			region.Population = append(region.Population, domain.SpawnPoint{ID: -int32(len(region.Population) + 1), Pos: domain.Pos{MapID: 7, X: float64(32 + x*64), Y: float64(32 + y*64)}})
		}
	}
	s.riftLayouts[0].CombatRegions = []domain.RiftCombatRegion{region}
	s.walkable = func(p domain.Pos) bool {
		return p.MapID == 7 && p.X >= 0 && p.Y >= 0 && p.X < float64(width*64) && p.Y < float64(height*64)
	}
	base, err := domain.PlanRiftCoverageFloor(s.rift.data.Run.RunID, floor, s.riftLayouts, 0)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, floor, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Floor = &plan
	s.rift.floorLoading = false
	return s, base
}

func denseCoverageClassCounts(plan domain.RiftFloorPlan) [3]int {
	var counts [3]int
	for _, spawn := range plan.Spawns {
		counts[spawn.Class]++
	}
	return counts
}

func TestRiftDenseCoverageScenePopulationKeepsLegacyClientBudget(t *testing.T) {
	for _, size := range [][2]int{{22, 22}, {40, 40}, {72, 72}, {80, 98}} {
		for _, tier := range []int32{1, domain.RiftMaxTier} {
			t.Run(fmt.Sprintf("%dx%d-tier%d", size[0], size[1], tier), func(t *testing.T) {
				s, base := denseCoverageSceneFixture(t, size[0], size[1], tier, 1)
				plan := *s.rift.data.Floor
				want := min(320, (len(base.Spawns)*3+1)/2)
				if plan.GeneratorVersion != domain.RiftFloorDenseCoverageVersion || len(plan.Spawns) != want {
					t.Fatalf("v5 plan: version=%d count=%d, want=%d", plan.GeneratorVersion, len(plan.Spawns), want)
				}
				before, after := denseCoverageClassCounts(base), denseCoverageClassCounts(plan)
				if before[1] != after[1] || before[2] != after[2] || after[0]-before[0] != want-len(base.Spawns) {
					t.Fatalf("density must add only ordinary monsters: before=%v after=%v", before, after)
				}
				if plan.Entry != base.Entry || plan.Exit != base.Exit || plan.MapID != base.MapID || plan.Seed != base.Seed {
					t.Fatal("density changed frozen geometry or seed")
				}
				s.populateEndgameRift(false)
				if s.rift.data.Run.Phase != domain.RiftActive || len(s.monsters) != want || len(s.rift.data.Run.Monsters) != want {
					t.Fatalf("dense floor not fully populated: phase=%s entities=%d registered=%d want=%d", s.rift.data.Run.Phase, len(s.monsters), len(s.rift.data.Run.Monsters), want)
				}
				seen := map[domain.Pos]bool{}
				for id, m := range s.monsters {
					class := s.rift.data.Run.Monsters[id].Class
					if seen[m.Pos] || !s.walkable(m.Pos) || riftDistance2(m.Pos, plan.Entry) < domain.RiftFloorSafeRadius*domain.RiftFloorSafeRadius || riftDistance2(m.Pos, plan.Exit) < 80*80 {
						t.Fatal("density violated unique, walkable, safe encounter positions")
					}
					seen[m.Pos] = true
					if m.MaxHP != domain.RiftScaledStat(100, tier, 1, class, true, s.rift.data.Run.Rules) || m.Stats.MinAtk != domain.RiftScaledStat(30, tier, 1, class, false, s.rift.data.Run.Rules) {
						t.Fatal("density changed per-monster combat scaling")
					}
					if class > 0 && (s.rift.abilities[id] == nil || (len(s.rift.abilities[id].pattern) < int(class)-1 || len(s.rift.abilities[id].pattern) > int(class))) {
						t.Fatal("elite lost more than the removed golden affix")
					}
					for _, a := range s.riftMonsterAbilityIDs(id) {
						if a == 0 {
							t.Fatal("golden affix generated")
						}
					}
				}
				view := s.riftView(s.players[1], s.rift.data, true, true)
				if len(view.Auras) != want || len(view.Auras) > 321 || after[1] > 24 || after[2] > 8 || int32(after[1]+after[2])*s.rift.data.Run.Rules.Abilities[1].Count > 96 {
					t.Fatal("existing client aura/elite/clone limits changed")
				}
				response := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: view}
				raw, err := json.Marshal(response)
				if err != nil || len(raw) > domain.EndgameMaxJSONBytes || len(protocol.EncodeEndgame(response)) == 0 {
					t.Fatalf("real dense scene snapshot exceeds existing transport: bytes=%d err=%v", len(raw), err)
				}
				t.Logf("v4=%d v5=%d classes=%v actual snapshot=%d bytes", len(base.Spawns), want, after, len(raw))
				s.clearRiftMonsters()
				s.rift.data.Run.Phase = domain.RiftBoss
				s.populateEndgameRift(true)
				s.populateEndgameRift(true)
				if len(s.monsters) != 1 {
					t.Fatal("density multiplied guardian or repeated admission")
				}
				for id, m := range s.monsters {
					if m.Monster.Kind != domain.MonsterBoss || s.rift.data.Run.Monsters[id].Class != 3 {
						t.Fatal("guardian class changed")
					}
				}
			})
		}
	}
}

func TestRiftDenseCoverageSavedAndLegacyPlansRemainFrozen(t *testing.T) {
	for _, version := range []int32{8, 9} {
		t.Run(fmt.Sprintf("rules-v%d", version), func(t *testing.T) {
			s, _ := denseCoverageSceneFixture(t, 40, 40, 150, 37)
			rules, ok := s.riftRules.Version(version)
			if !ok {
				t.Fatal("fixture missing rules")
			}
			s.rift.data.Run.Rules, s.rift.data.Run.RulesVersion = rules, version
			plan, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, 37, 0)
			if err != nil {
				t.Fatal(err)
			}
			s.rift.data.Floor = &plan
			s.populateEndgameRift(false)
			s.rift.data.Run.Progress = 123
			s.rift.data.Run.PenaltyMS = 5000
			s.rift.data.Run.Members[100].PresentMS = 9000
			snapshot := s.rift.data.clone()
			record := s.rift.data.record(17)
			restored, ok := decodeEndgameRift(&record, s.riftRules)
			if !ok || restored.StorageRevision != 17 || !reflect.DeepEqual(restored.Floor, snapshot.Floor) || !reflect.DeepEqual(restored.Run, snapshot.Run) {
				t.Fatal("save/reload changed frozen plan, rules, monsters, progress or timing")
			}
			if restored.Run.RulesVersion != version || restored.Floor.GeneratorVersion != version-4 {
				t.Fatal("active v9 rules replaced saved v8 population")
			}
			// A changed map catalog may affect future floors, never this saved plan.
			s.riftLayouts[0].CombatRegions = nil
			s.rift.data = restored
			if !reflect.DeepEqual(*s.rift.data.Floor, plan) || s.validateEndgameRiftFloor(*s.rift.data.Floor) != nil {
				t.Fatal("saved plan was regenerated or rejected after catalog change")
			}
		})
	}
}
