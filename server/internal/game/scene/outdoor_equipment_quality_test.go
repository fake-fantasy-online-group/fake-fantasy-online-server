package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
	"reflect"
	"testing"
)

func outdoorQualityFixture(t *testing.T) (*Scene, domain.ItemDef) {
	t.Helper()
	s, _, _, _ := cosmeticHatFixture(t)
	s.id = domain.SceneID{MapID: 14}
	s.rift = nil
	s.dungeon = nil
	s.trial = nil
	s.trialRun = nil
	old := *lootRulesFixture()
	fresh := old
	fresh.Version = 3
	fresh.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{20, 25, 20, 15, 10, 5}, LegendaryBasePPB: 50_000_000}
	fresh.OutdoorNormalEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{46, 20, 15, 10, 5, 2}, LegendaryBasePPB: 20_000_000}
	var err error
	s.lootRules, err = domain.NewLootRuleTable(3, []domain.LootRuleSet{old, fresh}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	def := s.items[2105]
	if !def.LegendaryEnabled || len(def.Equip.Affixes) != 0 || !s.equipmentRoll.LegendaryReady(def) || len(domain.LegendaryCandidates(def)) == 0 {
		t.Fatal("shoe not fully eligible")
	}
	return s, def
}

func outdoorTestRules(s *Scene) (*domain.LootRuleSet, domain.EquipmentRollTable) {
	return outdoorTestRulesForKind(s, domain.MonsterBoss)
}
func outdoorTestRulesForKind(s *Scene, kind domain.MonsterKind) (*domain.LootRuleSet, domain.EquipmentRollTable) {
	rule, table := s.lootRules.Active(), s.equipmentRoll
	outdoor := rule.OutdoorEquipment
	if kind == domain.MonsterNormal {
		outdoor = rule.OutdoorNormalEquipment
	}
	table.Counts = map[domain.ColorProfile][]domain.WeightedAffixCount{1: outdoor.Counts()}
	rule.UniqueBasePPB = outdoor.LegendaryBasePPB
	return rule, table
}

func TestOutdoorEquipmentExactFinalColorPartition(t *testing.T) {
	s, def := outdoorQualityFixture(t)
	for _, kind := range []domain.MonsterKind{domain.MonsterBoss, domain.MonsterNormal} {
		rule, table := outdoorTestRulesForKind(s, kind)
		var got [7]int
		total := int64(95)
		want := [7]int{1900, 2375, 1900, 1425, 950, 475, 475}
		if kind == domain.MonsterNormal {
			total = 98
			want = [7]int{4508, 1960, 1470, 980, 490, 196, 196}
		}
		for ordinary := int64(0); ordinary < total; ordinary++ {
			for promotion := int64(0); promotion < 100; promotion++ {
				st := domain.NewStack(def, 1)
				s.rollEquipmentQualityV5(def, &st, def.Equip.LevelReq, table, 1, true, 0, rule, func(n int64) int64 {
					if n != total {
						t.Fatal(n)
					}
					return ordinary
				}, func(int64) int64 { return 0 }, func(n int64) int64 {
					if n != domain.LootProbabilityScaleV5 {
						t.Fatal(n)
					}
					return promotion * (n / 100)
				}, func(int) int { return 0 })
				bucket := int(st.RolledAffixCount)
				if st.Legendary.PowerID != "" {
					bucket = 6
					if st.Legendary.Validate() != nil || len(domain.LegendaryItemBonuses(def, st)) != 6 {
						t.Fatal("invalid legendary")
					}
				}
				if st.SocketCount != 0 || st.Endgame.UniqueID != 0 {
					t.Fatal("socket or old Unique")
				}
				got[bucket]++
			}
		}
		if got != want {
			t.Fatalf("exact partition kind=%v got=%v want=%v", kind, got, want)
		}
	}
}

func TestOutdoorEquipmentFailedPromotionRetainsOrdinaryRoll(t *testing.T) {
	s, original := outdoorQualityFixture(t)
	for name, mutate := range map[string]func(*domain.ItemDef, *domain.EquipmentRollTable){
		"no legal power": func(d *domain.ItemDef, _ *domain.EquipmentRollTable) { d.Equip.Slot = -1 },
		"sparse pool": func(d *domain.ItemDef, r *domain.EquipmentRollTable) {
			d.Equip.ResourceLevel = 0
			d.Equip.Type = 0
			r.Options = magicFindRollTableV5().Options
		},
	} {
		t.Run(name, func(t *testing.T) {
			def := original
			equip := *original.Equip
			def.Equip = &equip
			rule, table := outdoorTestRules(s)
			mutate(&def, &table)
			for draw := int64(0); draw < 95; draw++ {
				st := domain.NewStack(def, 1)
				want := st
				want.Endgame.SourceLevel = def.Equip.LevelReq
				_, rolled := table.RollColorFirstEquipment(def, 1, 0, rule, func(int64) int64 { return draw }, nil, func(int64) int64 { return 0 })
				want.RolledAffixCount = uint8(len(rolled))
				copy(want.RolledAffixes[:], rolled)
				s.rollEquipmentQualityV5(def, &st, def.Equip.LevelReq, table, 1, true, 0, rule, func(int64) int64 { return draw }, func(int64) int64 { return 0 }, func(int64) int64 { return 0 }, func(int) int { return 0 })
				if st != want {
					t.Fatal("failed promotion changed ordinary roll", draw)
				}
			}
		})
	}
}

func TestOutdoorEquipmentSourceIsolationAndFrozenRules(t *testing.T) {
	s, original := outdoorQualityFixture(t)
	before := append([]domain.WeightedAffixCount(nil), s.equipmentRoll.CountsFor(1)...)
	legacy, _ := s.lootRules.Version(1)
	for _, tc := range []struct {
		name    string
		mutate  func(*Scene, *entity.Entity, *domain.ItemDef, *bool)
		outdoor bool
	}{
		{"boss", func(*Scene, *entity.Entity, *domain.ItemDef, *bool) {}, true},
		{"elite", func(_ *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) { e.Monster.Kind = domain.MonsterElite }, true},
		{"normal", func(_ *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) { e.Monster.Kind = domain.MonsterNormal }, true},
		{"prop", func(_ *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) { e.Monster.Kind = domain.MonsterProp }, false},
		{"gather", func(_ *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) { e.Monster.Kind = domain.MonsterGather }, false},
		{"normal dungeon", func(s *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) {
			e.Monster.Kind = domain.MonsterNormal
			s.dungeon = &domain.DungeonDef{}
		}, false},
		{"normal rift", func(s *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) {
			e.Monster.Kind = domain.MonsterNormal
			s.rift = &endgameRiftState{}
		}, false},
		{"normal summoned", func(_ *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) {
			e.Monster.Kind = domain.MonsterNormal
			e.Monster.Summoner = 1
		}, false},
		{"summoned", func(_ *Scene, e *entity.Entity, _ *domain.ItemDef, _ *bool) { e.Monster.Summoner = 1 }, false},
		{"dungeon", func(s *Scene, _ *entity.Entity, _ *domain.ItemDef, _ *bool) { s.dungeon = &domain.DungeonDef{} }, false},
		{"rift", func(s *Scene, _ *entity.Entity, _ *domain.ItemDef, _ *bool) { s.rift = &endgameRiftState{} }, false},
		{"instance", func(s *Scene, _ *entity.Entity, _ *domain.ItemDef, _ *bool) { s.id.Instance = 2 }, false},
		{"trial map", func(s *Scene, _ *entity.Entity, _ *domain.ItemDef, _ *bool) {
			s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{14: {{}}}}
		}, false},
		{"trial run", func(s *Scene, _ *entity.Entity, _ *domain.ItemDef, _ *bool) { s.trialRun = &endgameTrialState{} }, false},
		{"fixed row", func(_ *Scene, _ *entity.Entity, _ *domain.ItemDef, b *bool) { *b = false }, false},
		{"no type", func(_ *Scene, _ *entity.Entity, d *domain.ItemDef, _ *bool) { d.Equip.NoTypeDrop = true }, false},
		{"hard dungeon", func(_ *Scene, _ *entity.Entity, d *domain.ItemDef, _ *bool) { d.HardDungeonOnly = true }, false},
		{"not native", func(_ *Scene, _ *entity.Entity, d *domain.ItemDef, _ *bool) { d.Equip.NativeDropEligible = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scene := *s
			def := original
			equip := *original.Equip
			def.Equip = &equip
			roll := true
			dead := &entity.Entity{ID: 800, Level: original.Equip.LevelReq, Monster: &entity.Monster{Kind: domain.MonsterBoss}}
			tc.mutate(&scene, dead, &def, &roll)
			if got := scene.outdoorEquipmentQualitySource(dead, def, roll); got != tc.outdoor {
				t.Fatal("wrong source", got)
			}
			for _, mf := range []int64{0, 100, math.MaxInt64} {
				for ordinal := -1; ordinal < 64; ordinal++ {
					st := domain.NewStack(def, 1)
					want := st
					scene.rollNaturalEquipmentV5(def, &st, dead, ordinal, 1, roll, domain.LootContextV5{Owner: 23, MagicFind: mf, Version: 5})
					q, v, u, x := scene.lootRandomV5(dead.ID, ordinal, 0x56350001), scene.lootRandomV5(dead.ID, ordinal, 0x56350002), scene.lootRandomV5(dead.ID, ordinal, 0x56350003), scene.lootRandomV5(dead.ID, ordinal, 0x56350004)
					rule, table := legacy, s.equipmentRoll
					if tc.outdoor {
						rule, table = outdoorTestRulesForKind(&scene, dead.Monster.Kind)
					}
					scene.rollEquipmentQualityV5(def, &want, dead.Level, table, 1, roll, mf, rule, q.Int63n, v.Int63n, u.Int63n, x.Intn)
					if st != want {
						t.Fatal("actual source generation mismatch", mf, ordinal)
					}
				}
			}
		})
	}
	if !reflect.DeepEqual(s.equipmentRoll.CountsFor(1), before) || s.lootRules.Active().UniqueBasePPB != legacy.UniqueBasePPB || s.lootRules.Active().OutdoorEquipment.LegendaryBasePPB != 50_000_000 {
		t.Fatal("shared rules mutated")
	}
	for seed := uint64(1); seed <= 64; seed++ {
		st := domain.NewStack(original, 1)
		want := st
		q, v, u, x := lootRandomV5{seed}, lootRandomV5{seed + 1}, lootRandomV5{seed + 2}, lootRandomV5{seed + 3}
		q2, v2, u2, x2 := q, v, u, x
		s.rollNaturalEquipmentQualityV5(original, &st, original.Equip.LevelReq, 1, true, 100, legacy, &q, &v, &u, &x)
		s.rollNaturalEquipmentQualityV5(original, &want, original.Equip.LevelReq, 1, true, 100, s.lootRules.Active(), &q2, &v2, &u2, &x2)
		if st != want {
			t.Fatal("outdoor override leaked to Rift helper")
		}
	}
}

func TestOutdoorEquipmentFixedAttributeFloor(t *testing.T) {
	s, original := outdoorQualityFixture(t)
	rule, table := outdoorTestRules(s)
	for fixed := 0; fixed <= 5; fixed++ {
		def := original
		equip := *def.Equip
		def.Equip = &equip
		def.Equip.Affixes = make([]domain.Affix, fixed)
		for draw := int64(0); draw < 95; draw++ {
			st := domain.NewStack(def, 1)
			s.rollEquipmentQualityV5(def, &st, def.Equip.LevelReq, table, 1, true, 0, rule, func(int64) int64 { return draw }, func(int64) int64 { return 0 }, func(n int64) int64 { return n - 1 }, func(int) int { return 0 })
			count := domain.EquipmentAttributeCount(def, st)
			if count < fixed || count > 5 || st.SocketCount != 0 {
				t.Fatal("fixed floor/budget broken", fixed, draw, count)
			}
		}
	}
}

func TestOutdoorEquipmentRealFixedAndOrdinaryShoeDrops(t *testing.T) {
	fixture, def := outdoorQualityFixture(t)
	for _, fixed := range []bool{false, true} {
		s, mid := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
		s.items[def.ID] = def
		s.equipmentRoll = fixture.equipmentRoll
		s.lootRules = fixture.lootRules
		dead := s.entities[mid]
		dead.Monster.Kind = domain.MonsterBoss
		// This quality-distribution test excludes the separately tested kill-only pair.
		dead.Monster.OutdoorBossLegendariesGranted = true
		dead.Level = 35
		if fixed {
			s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: dead.Monster.TypeID}: {Minimum: 1, Pool: []domain.BossFixedDropChoice{{Item: def.ID, Weight: 1, RollAffixes: true}}}}
		}
		var seen [7]bool
		for i := 0; i < 512; i++ {
			s.tick = domain.Tick(i)
			s.rollLootWithContextV5(dead, domain.LootContextV5{Owner: 23, Version: 5})
			if len(s.ground) != 1 {
				t.Fatal("drop count changed", fixed, len(s.ground))
			}
			for id, e := range s.ground {
				st := e.Drop.Stack
				if st.Item != 2105 || e.Drop.Owner != 23 || st.SocketCount != 0 || st.Endgame.UniqueID != 0 {
					t.Fatal("identity/owner changed", st, e.Drop.Owner)
				}
				bucket := int(st.RolledAffixCount)
				if st.Legendary.PowerID != "" {
					bucket = 6
					p, ok := domain.LegendaryPower(st.Legendary.PowerID)
					if !ok || p.Class != "全职业" || !domain.LegendaryPowerEligibleForItem(p, def, st) || st.Legendary.Validate() != nil {
						t.Fatal("illegal shoe legendary")
					}
				}
				seen[bucket] = true
				s.removeLoot(id, 0)
			}
		}
		for bucket, ok := range seen {
			if !ok {
				t.Fatal("real shoe drop route missed outcome", fixed, bucket)
			}
		}
	}
}

func TestOutdoorEquipmentDoesNotReviveUnique(t *testing.T) {
	s, def := outdoorQualityFixture(t)
	// Choose an actual legacy-catalog base, then emulate an ineligible six-line
	// foundation. A successful legacy promotion below proves this is not vacuous.
	found := false
	for _, u := range domain.UniqueCatalog(s.endgameCatalog) {
		if !u.NativeDropEligible {
			continue
		}
		def.ID = u.BaseID
		def.EndgameCatalog = s.endgameCatalog
		def.LegendaryEnabled = false
		found = true
		break
	}
	if !found {
		t.Fatal("no native legacy unique fixture")
	}
	dead := &entity.Entity{ID: 800, Level: def.Equip.LevelReq, Monster: &entity.Monster{Kind: domain.MonsterBoss}}
	legacy, _ := s.lootRules.Version(1)
	sawLegacy := false
	for ordinal := 0; ordinal < 512; ordinal++ {
		st := domain.NewStack(def, 1)
		s.rollNaturalEquipmentV5(def, &st, dead, ordinal, 1, true, domain.LootContextV5{MagicFind: 100, Version: 5})
		if st.Endgame.UniqueID != 0 || st.Legendary.PowerID != "" {
			t.Fatal("outdoor override revived legacy Unique")
		}
		// Theft shares the same outdoor policy, including the legacy-Unique guard.
		s.tick = domain.Tick(ordinal)
		stolen := domain.NewStack(def, 1)
		want := stolen
		s.rollNaturalEquipmentV5(def, &stolen, dead, -1, 1, true, domain.LootContextV5{MagicFind: 100, Version: 5})
		q, v, u, x := s.lootRandomV5(dead.ID, -1, 0x56350001), s.lootRandomV5(dead.ID, -1, 0x56350002), s.lootRandomV5(dead.ID, -1, 0x56350003), s.lootRandomV5(dead.ID, -1, 0x56350004)
		s.rollNaturalEquipmentQualityV5(def, &want, dead.Level, 1, true, 100, legacy, &q, &v, &u, &x)
		if stolen.Endgame.UniqueID != 0 || stolen.Legendary.PowerID != "" {
			t.Fatal("theft revived old Unique")
		}
		sawLegacy = sawLegacy || want.Endgame.UniqueID != 0
	}
	if !sawLegacy {
		t.Fatal("fixture never exercised the excluded old Unique path")
	}
}

func TestOutdoorEquipmentActualTheftSharesTargetQuality(t *testing.T) {
	fixture, def := outdoorQualityFixture(t)
	for _, kind := range []domain.MonsterKind{domain.MonsterBoss, domain.MonsterElite, domain.MonsterNormal} {
		s, src, dst, _ := theftTestScene(t, 10000)
		s.items[def.ID] = def
		s.equipmentRoll = fixture.equipmentRoll
		s.lootRules = fixture.lootRules
		s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}}
		dst.Monster.Kind = kind
		skill := s.skills[domain.SkillKey{ID: 10212, Level: 1}]
		for i := 0; i < 128; i++ {
			s.tick = domain.Tick(i)
			dst.Monster.TheftSuccesses = 0
			if !s.stealMonsterItem(src, dst, skill, skill.Effects[0]) {
				t.Fatal("theft failed", kind, i)
			}
			drop := onlyTheftDrop(t, s)
			actual := drop.Drop.Stack
			want := domain.NewStack(def, 1)
			want.UID = actual.UID
			want.Endgame.FormationRollBP = actual.Endgame.FormationRollBP
			q, v, u, x := s.lootRandomV5(dst.ID, -1, 0x56350001), s.lootRandomV5(dst.ID, -1, 0x56350002), s.lootRandomV5(dst.ID, -1, 0x56350003), s.lootRandomV5(dst.ID, -1, 0x56350004)
			rule, table := s.lootRules.Active(), s.equipmentRoll
			rule, table = outdoorTestRulesForKind(s, kind)
			s.rollEquipmentQualityV5(def, &want, dst.Level, table, 1, true, 0, rule, q.Int63n, v.Int63n, u.Int63n, x.Intn)
			if actual != want || drop.Drop.Owner != src.ID {
				t.Fatal("theft did not use target source quality", kind, i)
			}
			s.removeLoot(drop.ID, 0)
		}
	}
}
