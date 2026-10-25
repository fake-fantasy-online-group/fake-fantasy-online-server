package scene

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func cumulativeRiftFixture(t *testing.T) (*Scene, *domain.RiftRules, *domain.RiftRules) {
	t.Helper()
	s, _, old, _ := cosmeticHatFixture(t)
	old.LootVersion = 2
	old.Loot = weightedSceneLootFixture(t).Active()
	raw, _ := json.Marshal(old)
	var fresh domain.RiftRules
	if err := json.Unmarshal(raw, &fresh); err != nil {
		t.Fatal(err)
	}
	fresh.Version, fresh.LootVersion, fresh.CumulativeLegendaryPerTier = 12, 3, 10
	fresh.Equipment, fresh.TimelyEquipment = 0, 0
	fresh.UniqueBaseChance, fresh.UniquePerTier, fresh.UniqueCap = 0, 0, 0
	l1 := *lootRulesFixture()
	l2 := *weightedSceneLootFixture(t).Active()
	l3 := l2
	l3.Version = 3
	l3.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{20, 25, 20, 15, 10, 5}, LegendaryBasePPB: 50_000_000}
	loot, err := domain.NewLootRuleTable(3, []domain.LootRuleSet{l1, l2, l3}, s.items)
	if err != nil {
		t.Fatal(err)
	}
	s.lootRules = loot
	s.riftRules, err = domain.NewRiftRuleTable(12, []domain.RiftRules{*riftRulesFixture(), *old, fresh}, loot, s.items, riftRulesTableFixture().Entrances()...)
	if err != nil {
		t.Fatal(err)
	}
	return s, s.riftRules.Active(), func() *domain.RiftRules { r, _ := s.riftRules.Version(11); return r }()
}

func prepareCumulativeRun(s *Scene, rules *domain.RiftRules, id string, tier int32, timely bool, levels ...int32) {
	run := &domain.RiftRun{RunID: id, Tier: tier, Timely: timely, RulesVersion: rules.Version, Rules: rules, Members: map[domain.CharID]*domain.RiftMember{}}
	profiles := map[domain.CharID]endgameRiftIdentity{}
	for i, level := range levels {
		char := domain.CharID((i + 1) * 100)
		run.Members[char] = &domain.RiftMember{CharID: char, Level: level, Eligible: true}
		profiles[char] = endgameRiftIdentity{Race: domain.Race(i % 5), Gender: uint8(i % 2), Employed: true, RewardLevel: level}
	}
	s.rift = &endgameRiftState{data: endgameRiftPersisted{Version: 2, RewardSourceLevel: 65, Run: run, Profiles: profiles, Grants: map[domain.CharID]endgameRiftGrant{}}}
}

func TestRiftCumulativeActualCountsMFReplayAndFrozenPolicy(t *testing.T) {
	s, rules, old := cumulativeRiftFixture(t)
	oldJSON, _ := json.Marshal(old)
	originalItems := map[domain.ItemID][2]int32{}
	for id, d := range s.items {
		if d.Equip != nil {
			originalItems[id] = [2]int32{d.Equip.LevelReq, d.Equip.ResourceLevel}
		}
	}
	observedMFChange := false
	for _, tier := range []int32{1, 9, 10, 11, 15, 99, 100, 149, 150} {
		for seed := 0; seed < 8; seed++ {
			id := fmt.Sprintf("cumulative-t%d-s%d", tier, seed)
			var firstSpecial, firstOrdinary []domain.Stack
			for _, timely := range []bool{false, true} {
				for _, mf := range []int64{0, 1000000} {
					prepareCumulativeRun(s, rules, id, tier, timely, 43)
					countRNG := s.riftCumulativeRandom(100, -1, "count")
					n := int(domain.PlanRiftRewards(tier, timely, rules).CumulativeLegendaryCount(countRNG.Intn))
					s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 65, MagicFind: mf}})
					if !s.rift.data.GrantsReady {
						t.Fatal("missing guarantee", tier)
					}
					grant := s.rift.data.Grants[100]
					var equipment []domain.Stack
					for _, st := range grant.Items {
						if s.items[st.Item].Equip != nil {
							equipment = append(equipment, st)
						}
					}
					if len(equipment) != n+10 || (tier == 150 && n != 15) {
						t.Fatalf("tier%d timely%v count%d special%d", tier, timely, len(equipment), n)
					}
					for _, st := range equipment[:n] {
						d := s.items[st.Item]
						if st.Legendary.PowerID == "" || st.Endgame.UniqueID != 0 || st.Legendary.Validate() != nil || !st.Bound || st.SocketCount != 0 || d.Equip.LevelReq > 43 || d.Equip.AscensionOnly || d.HardDungeonOnly {
							t.Fatal("invalid special", st.Item)
						}
					}
					special, ordinary := normalizeRiftStacks(equipment[:n]), normalizeRiftStacks(equipment[n:])
					if !timely && mf == 0 {
						firstSpecial, firstOrdinary = special, ordinary
					} else {
						if !reflect.DeepEqual(special, firstSpecial) {
							t.Fatal("MF/timing changed cumulative entitlement", tier)
						}
						if mf == 0 && !reflect.DeepEqual(ordinary, firstOrdinary) {
							t.Fatal("timing changed ordinary10")
						}
						if mf != 0 && !reflect.DeepEqual(ordinary, firstOrdinary) {
							observedMFChange = true
						}
					}
					saved, _ := json.Marshal(s.rift.data)
					s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 1, MagicFind: 999}})
					after, _ := json.Marshal(s.rift.data)
					if string(saved) != string(after) {
						t.Fatal("ready grants rerolled")
					}
					var restored endgameRiftPersisted
					if err := json.Unmarshal(saved, &restored); err != nil {
						t.Fatal(err)
					}
					if err := restored.Run.BindRules(s.riftRules); err != nil {
						t.Fatal(err)
					}
					s.rift.data = restored
					s.generateEndgameRiftGrants()
					after, _ = json.Marshal(s.rift.data)
					if string(saved) != string(after) {
						t.Fatal("saved grants changed on replay")
					}
				}
			}
		}
	}
	if !observedMFChange {
		t.Fatal("ordinary10 ignored MF")
	}
	afterOld, _ := json.Marshal(old)
	if string(oldJSON) != string(afterOld) {
		t.Fatal("old rule mutated")
	}
	for id, levels := range originalItems {
		d := s.items[id]
		if levels != [2]int32{d.Equip.LevelReq, d.Equip.ResourceLevel} {
			t.Fatal("native level changed", id)
		}
	}
}

func TestRiftCumulativeLegalPoolsAndMixedParty(t *testing.T) {
	s, rules, _ := cumulativeRiftFixture(t)
	for level := int32(12); level <= 65; level++ {
		for race := domain.Race(0); race < 5; race++ {
			for _, employed := range []bool{false, true} {
				for _, gender := range []uint8{0, 1} {
					identity := endgameRiftIdentity{Race: race, Gender: gender, Employed: employed}
					for _, poolLevel := range []int32{level, 65} {
						pool := s.riftCumulativeLegendaryPool(rules, poolLevel, level, 65, identity)
						if len(pool) == 0 {
							t.Fatalf("empty guaranteed pool lv%d race%d employed%v gender%d", level, race, employed, gender)
						}
						ch := &domain.Character{Race: race, Employed: employed, EmploymentKnown: true}
						sex := uint8(2)
						if gender == 1 {
							sex = 1
						}
						for _, id := range pool {
							d := s.items[id]
							if !d.LegendaryEnabled || !s.riftLegendaryBaseEligibleAtLevel(d, 65) || d.Equip.LevelReq > level || !d.Equip.Need.AllowsCharacter(ch) || d.Equip.Need.Sex != 0 && d.Equip.Need.Sex != sex {
								t.Fatal("illegal guaranteed candidate", id)
							}
						}
					}
				}
			}
		}
	}
	prepareCumulativeRun(s, rules, "mixed-cumulative", 150, true, 12, 20, 43, 60, 65)
	s.rift.data.Run.Members[500].Eligible = false
	s.generateEndgameRiftGrants()
	if !s.rift.data.GrantsReady || len(s.rift.data.Grants) != 4 {
		t.Fatal("mixed party grants")
	}
	for char, grant := range s.rift.data.Grants {
		gear := 0
		for _, st := range grant.Items {
			if d := s.items[st.Item]; d.Equip != nil {
				gear++
				if d.Equip.LevelReq > s.rift.data.Run.Members[char].Level {
					t.Fatal("party level leaked")
				}
			}
		}
		if gear != 25 {
			t.Fatal("mixed member lost guarantee", char, gear)
		}
	}
}

func TestRiftCumulativeInvalidPoolFailsClosedAndOldRulesStayLegacy(t *testing.T) {
	s, rules, old := cumulativeRiftFixture(t)
	prepareCumulativeRun(s, old, "old-paid-run", 150, true, 43)
	before := s.rift.data.clone()
	s.generateEndgameRiftGrants()
	want := normalizeRiftStacks(s.rift.data.Grants[100].Items)
	if p := domain.PlanRiftRewards(150, true, old); p.Equipment != 2 || p.UniqueChance != 60 || p.GuaranteedLegendaries != 0 {
		t.Fatal("old policy changed", p)
	}
	// Live loot config is irrelevant to both pinned old and new runs.
	s.lootRules = nil
	s.rift.data = before
	s.generateEndgameRiftGrants()
	if !reflect.DeepEqual(want, normalizeRiftStacks(s.rift.data.Grants[100].Items)) {
		t.Fatal("old paid run changed")
	}
	prepareCumulativeRun(s, rules, "missing-foundation", 150, true, 43)
	for id, d := range s.items {
		d.LegendaryEnabled = false
		s.items[id] = d
	}
	s.generateEndgameRiftGrants()
	if s.rift.data.GrantsReady || len(s.rift.data.Grants) != 0 {
		t.Fatal("silently consumed guaranteed slots")
	}
}

func TestRiftCumulativeMissingFoundationRejectsBeforeKey(t *testing.T) {
	s, _, _ := cumulativeRiftFixture(t)
	p := s.players[1]
	p.Player.EndgameRiftPages = true
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "八仙使者", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"}}
	p.Player.QuestNPC = "八仙使者"
	p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 1, Bound: true})
	for id, d := range s.items {
		d.LegendaryEnabled = false
		s.items[id] = d
	}
	s.startEndgameRift(p, domain.EndgameRequest{Op: "rift.start", NPCEntityID: 77, Tier: 1})
	b := s.endgameBackend.(*riftTestBackend)
	if b.commits != 0 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 1 || len(s.router.riftAdmissions) != 0 {
		t.Fatal("missing foundation charged key")
	}
	s.flush()
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if m, ok := ev.(event.EndgameMessage); ok && m.Message.Error == "reward_pool" {
			return
		}
	}
	t.Fatal("missing precise rejection")
}

func TestRiftCumulativeRulesRejectDuplicateOrUnpinnedPolicy(t *testing.T) {
	_, rules, _ := cumulativeRiftFixture(t)
	for _, change := range []func(*domain.RiftRules){
		func(r *domain.RiftRules) { r.Version = 11 },
		func(r *domain.RiftRules) { r.CumulativeLegendaryPerTier = 0 },
		func(r *domain.RiftRules) { r.CumulativeLegendaryPerTier = 20 },
		func(r *domain.RiftRules) { r.Equipment = 1 },
		func(r *domain.RiftRules) { r.TimelyEquipment = 1 },
		func(r *domain.RiftRules) { r.UniqueBaseChance = 20 },
		func(r *domain.RiftRules) { r.UniquePerTier = 2 },
		func(r *domain.RiftRules) { r.UniqueCap = 60 },
		func(r *domain.RiftRules) { r.NormalBossEquipment = 9 },
		func(r *domain.RiftRules) { r.Loot = lootRulesFixture(); r.LootVersion = r.Loot.Version },
	} {
		bad := *rules
		change(&bad)
		if bad.Validate() == nil {
			t.Fatalf("accepted invalid policy %+v", bad)
		}
	}
}

func TestRiftCumulativeLegacyPreVariantOracle(t *testing.T) {
	s, _, old := cumulativeRiftFixture(t)
	old.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 43, SourceMonsters: []domain.MonsterID{1}, Items: []domain.ItemID{2105}}}
	old.EquipmentSlotFallbacks = nil
	prepareCumulativeRun(s, old, "legacy-prevariant-oracle", 150, true, 43)
	// Source100 already admitted all universal powers before their level gate was removed.
	// This golden was independently derived from baseline96d8adb at source100.
	s.rift.data.RewardSourceLevel = 100
	s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 43, MagicFind: 100}})
	items := normalizeRiftStacks(s.rift.data.Grants[100].Items)
	for i := range items {
		if items[i].Legendary.Tier != domain.LegendaryTierNormal {
			t.Fatal("old paid run gained variant")
		}
		items[i].Endgame.SourceLevel = 0 // The sole new provenance metadata is excluded from this pre-variant oracle.
	}
	// Wings add independent durable metadata; absent powers must not change
	// this pre-wing gameplay oracle or be mistaken for a changed old drop.
	type legacyStack domain.Stack
	type legacyProjection struct {
		legacyStack
		WingPowers *domain.WingPowers `json:",omitempty"`
	}
	projected := make([]legacyProjection, len(items))
	for i, st := range items {
		if st.WingPowers != (domain.WingPowers{}) {
			t.Fatal("old paid run gained wing powers")
		}
		projected[i] = legacyProjection{legacyStack: legacyStack(st)}
	}
	raw, _ := json.Marshal(projected)
	got := fmt.Sprintf("%x", sha256.Sum256(raw))
	if got != "b648c14518deb94802ff36c15dde9b30e0027ed8d7e7ad2eae855f0cf7990a82" {
		t.Fatalf("pre-variant normalized item oracle changed: %s", got)
	}
}

func TestRiftCumulativePowerEligibilityUsesCapturedMonsterNotWearer(t *testing.T) {
	s, rules, _ := cumulativeRiftFixture(t)
	prepareCumulativeRun(s, rules, "source-not-wearer", 150, true, 12)
	s.generateEndgameRiftGrants()
	if !s.rift.data.GrantsReady {
		t.Fatal("high source level unavailable")
	}
	foundHigh := false
	for _, st := range s.rift.data.Grants[100].Items {
		if d := s.items[st.Item]; d.Equip != nil {
			if st.Endgame.SourceLevel != 65 || d.Equip.LevelReq > 12 {
				t.Fatal("source/wearer level mixed", st.Item)
			}
			if power, ok := domain.LegendaryPower(st.Legendary.PowerID); ok {
				if !domain.LegendaryPowerEligibleAtLevel(power, d, 65) {
					t.Fatal("invalid source gated power")
				}
				if power.MinLevel > d.Equip.LevelReq {
					foundHigh = true
				}
			}
		}
	}
	if !foundHigh {
		t.Fatal("low wearable bases still barred from high-source powers")
	}
	prepareCumulativeRun(s, rules, "unknown-source", 150, true, 12)
	s.rift.data.RewardSourceLevel = 0
	s.trial = nil
	s.generateEndgameRiftGrants()
	if s.rift.data.GrantsReady {
		t.Fatal("unknown monster level substituted recipient level")
	}
}

func TestRiftRewardSourceLevelRecoveryMatchesEncounterDefinition(t *testing.T) {
	s, rules, _ := cumulativeRiftFixture(t)
	prepareCumulativeRun(s, rules, "source-recovery", 150, true, 43)
	s.rift.data.RewardSourceLevel = 0
	s.trial.BossOffset = 5
	s.trial.Monsters[48] = map[domain.MonsterID]domain.MonsterDef{
		101: {ID: 101, Level: 68, Kind: domain.MonsterBoss, HP: 100},
		102: {ID: 102, Level: 71, Kind: domain.MonsterBoss, HP: 100},
	}
	s.rift.data.Floor = &domain.RiftFloorPlan{Seed: 1}
	if got := s.riftRewardSourceLevel(); got != 71 {
		t.Fatal("did not recover selected actual Boss base", got)
	}
	s.rift.data.Floor = nil
	if got := s.riftRewardSourceLevel(); got != 0 {
		t.Fatal("ambiguous floorless source invented", got)
	}
	s.rift.data.Run.Monsters = map[domain.EntityID]*domain.RiftMonster{777: {Class: 3}}
	s.monsters[777] = &entity.Entity{ID: 777, Level: 73}
	if got := s.riftRewardSourceLevel(); got != 73 {
		t.Fatal("live actual boss not preferred", got)
	}
	s.rift.data.RewardSourceLevel = 75
	if got := s.riftRewardSourceLevel(); got != 75 {
		t.Fatal("frozen source not preserved", got)
	}
}

func TestRiftRewardSourceLevelFreezesBeforeDeferredCompletion(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		s, b, now := riftFixture(t)
		activateTestRift(t, s, b, *now)
		r := s.rift.data.Run
		r.Phase = domain.RiftBoss
		id := s.alloc.Monster()
		r.RegisterMonster(id, 3)
		s.rift.economicPending = deferred
		*now += 1000
		boss := &entity.Entity{ID: id, Level: 77, Pos: s.players[1].Pos}
		s.onEndgameRiftMonsterDeath(boss, s.players[1])
		if deferred {
			if len(s.rift.delayedDeaths) != 1 || s.rift.delayedDeaths[0].SourceLevel != 77 {
				t.Fatal("source missing from delayed death")
			}
			boss.Level = 1
			death := s.rift.delayedDeaths[0]
			s.rift.economicPending = false
			s.rift.delayedDeaths = nil
			s.applyEndgameRiftDeath(death)
		}
		if s.rift.data.RewardSourceLevel != 77 || !s.rift.data.GrantsReady {
			t.Fatal("source lost on completion")
		}
		for _, grant := range s.rift.data.Grants {
			for _, st := range grant.Items {
				if s.items[st.Item].Equip != nil && st.Endgame.SourceLevel != 77 {
					t.Fatal("equipment lost source")
				}
			}
		}
		record := s.rift.data.record(1)
		restored, ok := decodeEndgameRift(&record, s.riftRules)
		if !ok || restored.RewardSourceLevel != 77 {
			t.Fatal("source lost after snapshot replay")
		}
	}
}

func TestRiftLegacyUnknownSourceDoesNotConsumePaidGuarantee(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		s, _, old := cumulativeRiftFixture(t)
		prepareCumulativeRun(s, old, "legacy-unknown-source", 150, true, 43)
		s.rift.data.RewardSourceLevel = 0
		if ambiguous {
			s.trial.Monsters[43] = map[domain.MonsterID]domain.MonsterDef{
				1: {ID: 1, Level: 60, HP: 100, Kind: domain.MonsterBoss},
				2: {ID: 2, Level: 65, HP: 100, Kind: domain.MonsterBoss},
			}
		} else {
			s.trial = nil
		}
		s.generateEndgameRiftGrants()
		if s.rift.data.GrantsReady || len(s.rift.data.Grants) != 0 {
			t.Fatal("unknown legacy source consumed guaranteed reward")
		}
	}
}
