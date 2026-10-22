package scene

import (
	"context"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestRiftGuardianHistoryAtomicPaidAdmission(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "paid", false: "rollback"}[success], func(t *testing.T) {
			s, b, _ := riftFixture(t)
			rules := riftEliteRulesFixture(t)
			rules.GuardianRecentWindow = 3
			rules.Version = 8
			rules.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []domain.MonsterID{1011}, Items: []domain.ItemID{1234}}}
			rules.FloorLayoutVersion = domain.RiftFloorCoverageVersion
			items := balanceItemsFixture()
			items[1234] = domain.ItemDef{ID: 1234, Equip: &domain.EquipDef{LevelReq: 40}}
			var err error
			s.riftRules, err = domain.NewRiftRuleTable(rules.Version, []domain.RiftRules{*riftRulesFixture(), *rules}, lootRulesTableFixture(), items, s.riftRules.Entrances()...)
			if err != nil {
				t.Fatal(err)
			}
			defs := spawn.MapDefs{}
			for _, g := range rules.Elite.Guardians {
				defs[g.MonsterID] = domain.MonsterDef{ID: g.MonsterID, Kind: domain.MonsterBoss, Sprite: 211, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}}}}
			}
			s.defs = defs
			frames := make(chan time.Time)
			router := NewRouter(func(id domain.SceneID) (*Scene, error) {
				return New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frames, Trial: s.trial, Dungeons: s.dungeons}), nil
			}, nil)
			router.Start(context.Background())
			defer router.Drain(time.Second)
			s.router = router
			var anchors []domain.SpawnPoint
			for i := 0; i < 40; i++ {
				anchors = append(anchors, domain.SpawnPoint{ID: int32(i + 1), Monster: 1, Pos: domain.Pos{MapID: 7, X: float64(100 + i*100), Y: 100}})
			}
			s.riftLayouts = []domain.RiftFloorTemplate{{MapID: 7, MapName: "测试洞天", Anchors: anchors, Monsters: []domain.MonsterID{1}, Guardians: []domain.MonsterID{2}}}
			region := domain.RiftCombatRegion{CombatArea: 32 * 32 * 64 * 64}
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					region.Population = append(region.Population, domain.SpawnPoint{ID: -int32(len(region.Population) + 1), Pos: domain.Pos{MapID: 7, X: float64(32 + x*64), Y: float64(32 + y*64)}})
				}
			}
			s.riftLayouts[0].CombatRegions = []domain.RiftCombatRegion{region}
			// The configured rift entrance is independent from native trial guards.
			s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "八仙使者", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}, NPC: &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"}}
			p := s.players[1]
			p.Player.QuestNPC = "八仙使者"
			p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 3, Bound: true})
			req := domain.EndgameRequest{Op: "rift.start", NPCEntityID: 77, RequestID: "start-one-key-0001", ExpectedRevision: "0", Tier: 1}
			history := []domain.MonsterID{rules.Elite.Guardians[0].MonsterID, rules.Elite.Guardians[1].MonsterID, rules.Elite.Guardians[2].MonsterID}
			original := withRiftProfile(`{"other":{"keep":true}}`, endgameRiftProfile{UnlockedTier: 1, RecentGuardians: history})
			s.endgamePlayer(p).state.Payload = original
			p.Player.Bag.Remove(domain.RiftKeyItemID, 3)
			s.startEndgameRift(p, req)
			if b.commits != 0 || s.endgamePlayer(p).state.Payload != original {
				t.Fatal("keyless admission changed history")
			}
			p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 3, Bound: true})
			s.startEndgameRift(p, req)
			if b.commits != 1 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 || b.plan.Snapshot.Bag.CountOf(domain.RiftKeyItemID) != 2 {
				t.Fatal("key changed outside atomic candidate")
			}
			if s.endgamePlayer(p).state.Payload != original {
				t.Fatal("live history changed before commit")
			}
			var persisted endgameRiftPersisted
			if err := json.Unmarshal([]byte(b.plan.Run.Payload), &persisted); err != nil {
				t.Fatal(err)
			}
			if !validRiftGuardianSelection(persisted) || persisted.Floor.GeneratorVersion != domain.RiftFloorCoverageVersion {
				t.Fatal("guardian not persisted")
			}
			for _, id := range history {
				if id == persisted.GuardianID {
					t.Fatal("recent guardian repeated")
				}
			}
			wantHistory := appendRiftGuardianHistory(history, persisted.GuardianID)
			if !reflect.DeepEqual(riftProfile(b.plan.State.Payload).RecentGuardians, wantHistory) {
				t.Fatal("candidate history differs from run")
			}
			b.finish(success)
			s.finishEndgame(false)
			if success {
				if !reflect.DeepEqual(riftProfile(s.endgamePlayer(p).state.Payload).RecentGuardians, wantHistory) {
					t.Fatal("paid history missing")
				}
				restored, valid := decodeEndgameRift(b.plan.Run, s.riftRules)
				if !valid || restored.GuardianID != persisted.GuardianID {
					t.Fatal("reconnect rerolled guardian")
				}
				if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 2 {
					t.Fatal("wrong sponsor debit")
				}
				if _, ok := router.riftScene(b.plan.Run.RunID); !ok {
					t.Fatal("paid admission missing")
				}
			} else {
				if s.endgamePlayer(p).state.Payload != original {
					t.Fatal("failed admission changed history")
				}
				if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 {
					t.Fatal("rollback lost key")
				}
				router.mu.RLock()
				n := len(router.riftAdmissions)
				router.mu.RUnlock()
				if n != 0 {
					t.Fatal("unpaid reservation leaked")
				}
			}
		})
	}
}

func TestRiftGuardianRecentThreeRotationAndBounds(t *testing.T) {
	rules := riftEliteRulesFixture(t)
	history := []domain.MonsterID(nil)
	seen := map[domain.MonsterID]bool{}
	rng := rand.New(rand.NewSource(42))
	for n := 0; n < 256; n++ {
		seed := rng.Uint64()
		selected := selectRiftGuardian(rules, seed, history)
		if selected != selectRiftGuardian(rules, seed, history) {
			t.Fatal("unstable selection")
		}
		for _, recent := range history {
			if selected == recent {
				t.Fatal("recent guardian repeated")
			}
		}
		seen[selected] = true
		history = appendRiftGuardianHistory(history, selected)
		if len(history) > 3 {
			t.Fatal("unbounded history")
		}
		history = riftProfile(withRiftProfile("{}", endgameRiftProfile{RecentGuardians: history})).RecentGuardians
	}
	if len(seen) != 8 {
		t.Fatal("not all existing guardians reachable", seen)
	}
	original := []domain.MonsterID{1, 2, 3, 4, 5, 6, 7, 8}
	if got := boundedRiftGuardianHistory(original); !reflect.DeepEqual(got, []domain.MonsterID{6, 7, 8}) {
		t.Fatal(got)
	}
	got := appendRiftGuardianHistory(original[:2], 9)
	if original[2] != 3 || !reflect.DeepEqual(got, []domain.MonsterID{1, 2, 9}) {
		t.Fatal("candidate aliases history")
	}
}

func TestRiftGuardianFrozenAcrossFloorsAndLegacyUnchanged(t *testing.T) {
	s, plan := seededRiftLayoutFixture(t, 1)
	rules := riftEliteRulesFixture(t)
	rules.GuardianRecentWindow = 3
	rules.Version = 8
	rules.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []domain.MonsterID{1011}, Items: []domain.ItemID{1234}}}
	s.rift.data.Run.Rules = rules
	s.rift.data.Run.RulesVersion = rules.Version
	s.rift.data.GuardianID = rules.Elite.Guardians[4].MonsterID
	defs := s.defs.(spawn.MapDefs)
	for _, g := range rules.Elite.Guardians {
		defs[g.MonsterID] = domain.MonsterDef{ID: g.MonsterID, Kind: domain.MonsterBoss, Sprite: 211, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}}}}
	}
	var anchors []domain.SpawnPoint
	for i := 0; i < 40; i++ {
		anchors = append(anchors, domain.SpawnPoint{ID: int32(i + 1), Monster: 501, Pos: domain.Pos{MapID: 7, X: float64(100 + i*100), Y: 100}})
	}
	s.riftLayouts = []domain.RiftFloorTemplate{{MapID: 7, MapName: "测试洞天", Anchors: anchors, Monsters: []domain.MonsterID{501}, Guardians: []domain.MonsterID{502}}}
	for floor := int32(1); floor <= 5; floor++ {
		next, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, floor, 0)
		if err != nil {
			t.Fatal(err)
		}
		if next.Guardian.Point.Monster != s.rift.data.GuardianID {
			t.Fatal("floor rerolled guardian")
		}
	}
	plan.Guardian.Point.Monster = s.rift.data.GuardianID
	s.rift.data.Floor = &plan
	if !validRiftGuardianSelection(s.rift.data) {
		t.Fatal("valid guardian rejected")
	}
	s.rift.data.GuardianID = 0
	if validRiftGuardianSelection(s.rift.data) {
		t.Fatal("missing frozen guardian accepted")
	}
	rules.GuardianRecentWindow = 0
	if !validRiftGuardianSelection(s.rift.data) {
		t.Fatal("historical snapshot rejected")
	}
	next, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if next.Guardian.Point.Monster != rules.Elite.Guardians[next.Seed%8].MonsterID {
		t.Fatal("legacy selection changed")
	}
}

func TestRiftGuardianAllEightNativeAffinitiesPreserved(t *testing.T) {
	for _, guardian := range riftEliteRulesFixture(t).Elite.Guardians {
		s, plan := seededRiftLayoutFixture(t, 1)
		r := riftEliteRulesFixture(t)
		r.GuardianRecentWindow = 3
		r.Version = 8
		r.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 40, SourceMonsters: []domain.MonsterID{1011}, Items: []domain.ItemID{1234}}}
		s.rift.data.GuardianID = guardian.MonsterID
		s.rift.data.Run.Rules = r
		s.rift.data.Run.RulesVersion = r.Version
		native := domain.MonsterDef{ID: guardian.MonsterID, Name: "刑天", Kind: domain.MonsterBoss, Sprite: 211, Level: 90, HP: 9999, Stats: domain.Stats{MaxHP: 9999, MinAtk: 700, MaxAtk: 800, MAtk: 900, MoveSpeed: 77, Def: 333}, AI: domain.MonsterAIProfile{ID: "native_xingtian"}, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}, InitialDelay: 7, Enabled: true}}, ImmuneSkills: map[domain.SkillID]struct{}{12001: {}}}
		base := s.defs.(spawn.MapDefs)
		base[guardian.MonsterID] = native
		plan.Guardian.Point.Monster = guardian.MonsterID
		s.rift.data.Floor = &plan
		s.rift.data.Run.Phase = domain.RiftBoss
		s.populateEndgameRift(true)
		if len(s.monsters) != 1 {
			t.Fatal("missing approved guardian")
		}
		for id, m := range s.monsters {
			d, _ := s.defs.Def(m.Monster.TypeID)
			if m.Name != native.Name || m.Look.ModelID != native.Sprite || m.Monster.AI.ID != native.AI.ID || !reflect.DeepEqual(d.Skills, native.Skills) || !reflect.DeepEqual(d.ImmuneSkills, native.ImmuneSkills) || m.Stats.Def != 333 || d.Stats.MoveSpeed != 115 {
				t.Fatal("native guardian changed", m, d)
			}
			if !reflect.DeepEqual(s.riftMonsterAbilityIDs(id), guardian.Abilities) {
				t.Fatal("guardian affixes changed")
			}
		}
	}
}
