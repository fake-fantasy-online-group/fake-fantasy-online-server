package scene

import (
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func outdoorBossFixture(t *testing.T) (*Scene, *entity.Entity, domain.ItemDef) {
	t.Helper()
	fixture, def := outdoorQualityFixture(t)
	s, mid := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
	s.items[def.ID] = def
	s.equipmentRoll = fixture.equipmentRoll
	s.lootRules = fixture.lootRules
	dead := s.entities[mid]
	dead.Monster.Kind = domain.MonsterBoss
	dead.Level = 43
	return s, dead, def
}

func requireBossPair(t *testing.T, s *Scene, dead *entity.Entity, owner domain.EntityID) []domain.Stack {
	t.Helper()
	var pair []domain.Stack
	uids := map[int64]bool{}
	for _, e := range s.ground {
		st := e.Drop.Stack
		if e.Drop.Owner != owner || st.Legendary.PowerID == "" || st.Legendary.Validate() != nil || st.Endgame.UniqueID != 0 || st.Endgame.SourceLevel != dead.Level || st.Endgame.Revision != 1 || st.SocketCount != 0 || uids[st.UID] {
			t.Fatal("invalid or multiply-generated boss legendary", st, e.Drop.Owner)
		}
		uids[st.UID] = true
		pair = append(pair, st)
	}
	if len(pair) != 2 {
		t.Fatal("not exactly two extra legendary instances", len(pair))
	}
	return pair
}

func TestOutdoorBossGuaranteesTwoIndependentInstancesAndDeduplicates(t *testing.T) {
	s, dead, def := outdoorBossFixture(t)
	s.lootTable[dead.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}, {Choices: []domain.ItemID{def.ID, def.ID}, RatePct: 50, RollAffixes: true}}
	s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: dead.Monster.TypeID}: {Minimum: 1, Pool: []domain.BossFixedDropChoice{{Item: def.ID, Weight: 2}}}}
	if got := s.outdoorBossLegendaryPool(dead); !reflect.DeepEqual(got, []domain.ItemID{def.ID}) {
		t.Fatal("duplicate rows multiplied pool", got)
	}
	if n := s.rollOutdoorBossLegendaries(dead, domain.LootContextV5{Owner: 98765}, 0); n != 2 {
		t.Fatal(n)
	}
	pair := requireBossPair(t, s, dead, 98765)
	if pair[0].Item != def.ID || pair[1].Item != def.ID {
		t.Fatal("invented replacement base")
	}
	if n := s.rollOutdoorBossLegendaries(dead, domain.LootContextV5{Owner: 99}, 0); n != 0 {
		t.Fatal("repeated death granted extras", n)
	}
	current := requireBossPair(t, s, dead, 98765)
	got := map[int64]domain.Stack{}
	for _, st := range current {
		got[st.UID] = st
	}
	for _, st := range pair {
		if got[st.UID] != st {
			t.Fatal("repeat rerolled variant or affixes")
		}
	}
}

func TestOutdoorBossTwoExtrasPreserveOriginalDropsAndOwner(t *testing.T) {
	s, dead, def := outdoorBossFixture(t)
	join(t, s, 1, 100, "owner", 100, 100)
	join(t, s, 2, 200, "party", 100, 100)
	s.entities[70] = &entity.Entity{ID: 70, Pet: &entity.Pet{Owner: 1}}
	ctx := s.freezeLootContextV5(70)
	s.rollLootWithContextV5(dead, ctx)
	if len(s.ground) != 3 {
		t.Fatal("guarantee replaced original equipment", len(s.ground))
	}
	var legendary int
	for _, e := range s.ground {
		if e.Drop.Item != def.ID || e.Drop.Owner != 1 {
			t.Fatal("party borrowed item or owner", e.Drop)
		}
		if e.Drop.Stack.Legendary.PowerID != "" {
			legendary++
		}
	}
	if legendary < 2 {
		t.Fatal("missing guaranteed pair", legendary)
	}
	s.rollLootWithContextV5(dead, ctx)
	if len(s.ground) != 4 {
		t.Fatal("same monster life granted extra pair twice", len(s.ground))
	}
	// A newly spawned lifetime, even of the same template, receives its own pair.
	fresh := *dead
	monster := *dead.Monster
	fresh.Monster = &monster
	fresh.ID++
	fresh.Monster.OutdoorBossLegendariesGranted = false
	if n := s.rollOutdoorBossLegendaries(&fresh, ctx, 0); n != 2 {
		t.Fatal("new life lost its guarantee", n)
	}
}

func TestOutdoorBossSourceAndEligibilityBoundaries(t *testing.T) {
	for name, mutate := range map[string]func(*Scene, *entity.Entity, *domain.ItemDef){
		"normal":   func(_ *Scene, e *entity.Entity, _ *domain.ItemDef) { e.Monster.Kind = domain.MonsterNormal },
		"elite":    func(_ *Scene, e *entity.Entity, _ *domain.ItemDef) { e.Monster.Kind = domain.MonsterElite },
		"summoned": func(_ *Scene, e *entity.Entity, _ *domain.ItemDef) { e.Monster.Summoner = 1 },
		"dungeon":  func(s *Scene, _ *entity.Entity, _ *domain.ItemDef) { s.dungeon = &domain.DungeonDef{} },
		"rift":     func(s *Scene, _ *entity.Entity, _ *domain.ItemDef) { s.rift = &endgameRiftState{} },
		"instance": func(s *Scene, _ *entity.Entity, _ *domain.ItemDef) { s.id.Instance = 7 },
		"trial": func(s *Scene, _ *entity.Entity, _ *domain.ItemDef) {
			s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{s.id.MapID: {{}}}}
		},
		"old rules":   func(s *Scene, _ *entity.Entity, _ *domain.ItemDef) { s.lootRules = lootRulesTableFixture() },
		"low monster": func(_ *Scene, e *entity.Entity, _ *domain.ItemDef) { e.Level = 8 },
		"empty table": func(s *Scene, e *entity.Entity, _ *domain.ItemDef) { s.lootTable[e.Monster.TypeID] = nil },
		"dungeon only row": func(s *Scene, e *entity.Entity, _ *domain.ItemDef) {
			s.lootTable[e.Monster.TypeID][0].DungeonOnly = true
		},
		"disabled legendary": func(_ *Scene, _ *entity.Entity, d *domain.ItemDef) { d.LegendaryEnabled = false },
		"no type":            func(_ *Scene, _ *entity.Entity, d *domain.ItemDef) { d.Equip.NoTypeDrop = true },
		"hard equipment":     func(_ *Scene, _ *entity.Entity, d *domain.ItemDef) { d.HardDungeonOnly = true },
	} {
		t.Run(name, func(t *testing.T) {
			s, dead, original := outdoorBossFixture(t)
			def := original
			equip := *def.Equip
			def.Equip = &equip
			mutate(s, dead, &def)
			s.items[def.ID] = def
			if n := s.rollOutdoorBossLegendaries(dead, domain.LootContextV5{Owner: 1}, 0); n != 0 || len(s.ground) != 0 {
				t.Fatal("excluded source generated extras", n)
			}
		})
	}
}

func TestOutdoorBossUsesMonsterLevelAndOriginalPoolNotWearer(t *testing.T) {
	s, dead, def := outdoorBossFixture(t)
	low := def
	equip := *def.Equip
	low.Equip = &equip
	low.Equip.LevelReq = 1
	s.items[low.ID] = low
	if len(domain.LegendaryCandidatesAtLevel(low, dead.Level)) == 0 {
		t.Fatal("missing source-level fixture")
	}
	if n := s.rollOutdoorBossLegendaries(dead, domain.LootContextV5{Owner: 12345}, 0); n != 2 {
		t.Fatal("base level or missing wearer incorrectly removed legal drops", n)
	}
	requireBossPair(t, s, dead, 12345)
}

func TestOutdoorBossTheftRemainsSingleThenKillAddsTwo(t *testing.T) {
	fixture, def := outdoorQualityFixture(t)
	s, src, dst, _ := theftTestScene(t, 10000)
	s.items[def.ID] = def
	s.equipmentRoll = fixture.equipmentRoll
	s.lootRules = fixture.lootRules
	s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}}
	dst.Monster.Kind = domain.MonsterBoss
	dst.Level = 43
	skill := s.skills[domain.SkillKey{ID: 10212, Level: 1}]
	if !s.stealMonsterItem(src, dst, skill, skill.Effects[0]) || len(s.ground) != 1 || dst.Monster.OutdoorBossLegendariesGranted {
		t.Fatal("theft consumed or duplicated kill guarantee")
	}
	s.rollLootWithContextV5(dst, s.freezeLootContextV5(src.ID))
	if len(s.ground) != 4 || !dst.Monster.OutdoorBossLegendariesGranted {
		t.Fatal("kill did not add original plus pair after theft", len(s.ground))
	}
}

func TestOutdoorBossNativePublicSourceCoverage(t *testing.T) {
	// Verbatim source relation from migration32 monster/drop literals and native
	// map_monster_spawns/ov_arm classifications, same loadDropChoiceLevels mapping.
	// Four original >60 bases are absent from the existing equipment fixture;
	// the test proves every real public Boss has a nonempty legal pool without
	// depending on those missing bases or expanding into unrelated equipment.
	fixture, _ := outdoorQualityFixture(t)
	raw, err := os.ReadFile("testdata/outdoor_boss_native_pools.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID                          domain.MonsterID
		Name                        string
		Level                       int32
		Maps                        []string
		Items                       []domain.ItemID
		MissingFromEquipmentFixture []domain.ItemID
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 11 {
		t.Fatal("native public Boss coverage changed", len(rows))
	}
	for _, row := range rows {
		if row.Level < 28 {
			t.Fatal("unexpected low public Boss", row)
		}
		s := *fixture
		s.lootTable = map[domain.MonsterID][]domain.DropEntry{row.ID: {{Choices: row.Items, RatePct: 100, RollAffixes: true}}}
		dead := &entity.Entity{ID: domain.EntityID(row.ID), Name: row.Name, Level: row.Level, Monster: &entity.Monster{TypeID: row.ID, Kind: domain.MonsterBoss}}
		pool := s.outdoorBossLegendaryPool(dead)
		if len(pool) == 0 {
			t.Fatal("real public Boss has no legal legendary base", row.ID, row.Name)
		}
		for _, id := range pool {
			def := s.items[id]
			st := domain.NewStack(def, 1)
			if !s.equipmentRoll.ApplyLegendaryAtLevel(&st, def, row.Level, func(int) int { return 0 }) || st.Legendary.Validate() != nil || st.Endgame.SourceLevel != row.Level {
				t.Fatal("eligible native base failed promotion", row.ID, id)
			}
		}
		t.Logf("Boss %d %s level%d: at least %d source-legal legendary bases", row.ID, row.Name, row.Level, len(pool))
	}
}

func TestOutdoorBossPrefersDistinctBasesWithoutConsumingNormalRNG(t *testing.T) {
	s, dead, first := outdoorBossFixture(t)
	fixture, _ := outdoorQualityFixture(t)
	second := fixture.items[2106]
	s.items[second.ID] = second
	s.lootTable[dead.Monster.TypeID] = []domain.DropEntry{{Choices: []domain.ItemID{first.ID, first.ID, second.ID}, RatePct: 100, RollAffixes: true}}
	s.rng = rand.New(rand.NewSource(9123))
	wantNext := rand.New(rand.NewSource(9123)).Int63()
	if n := s.rollOutdoorBossLegendaries(dead, domain.LootContextV5{Owner: 777, MagicFind: 900}, 0); n != 2 {
		t.Fatal(n)
	}
	pair := requireBossPair(t, s, dead, 777)
	if pair[0].Item == pair[1].Item {
		t.Fatal("duplicate base despite two legal choices")
	}
	if got := s.rng.Int63(); got != wantNext {
		t.Fatal("bonus consumed normal item/rune RNG", got, wantNext)
	}
}
