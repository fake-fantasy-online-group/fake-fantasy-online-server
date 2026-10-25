package scene

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func guardianHPRulesFixture(t *testing.T) (*domain.RiftRuleTable, *Scene) {
	t.Helper()
	source, old, _ := cumulativeRiftFixture(t)
	old.StatAnchorTier = 30
	fresh := *old
	fresh.Version, fresh.GuardianHPDivisor = 13, 10
	table, err := domain.NewRiftRuleTable(13, []domain.RiftRules{*riftRulesFixture(), *old, fresh}, source.lootRules, source.items)
	if err != nil {
		t.Fatal(err)
	}
	return table, source
}

func guardianHPSpawnFixture(t *testing.T, version, tier int32, party int, baseHP int32) *Scene {
	t.Helper()
	s, _ := seededRiftLayoutFixture(t, 1)
	table, source := guardianHPRulesFixture(t)
	s.riftRules, s.lootRules, s.items = table, source.lootRules, source.items
	r := s.rift.data.Run
	r.Rules, _ = table.Version(version)
	r.RulesVersion, r.Tier = version, tier
	for i := 2; i <= party; i++ {
		id := domain.CharID(i * 100)
		r.Members[id] = &domain.RiftMember{CharID: id, Level: 60}
	}
	def := s.trial.Monsters[60][2]
	def.HP, def.Stats = baseHP, domain.Stats{MaxHP: baseHP, MinAtk: 31, MaxAtk: 47, MAtk: 23, MoveSpeed: 120}
	s.trial.Monsters[60][2] = def
	guardian := r.Rules.Elite.Guardians[0].MonsterID
	s.rift.data.Floor.Guardian.Point.Monster = guardian
	native := domain.MonsterDef{ID: guardian, Name: "guardian-native", Sprite: 601, Kind: domain.MonsterBoss, Level: 150, HP: 0, Exp: 9876, Stats: domain.Stats{Def: 37, MoveSpeed: 98}, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}, Enabled: true}}, ImmuneSkills: map[domain.SkillID]struct{}{12001: {}}}
	s.defs = endgameRiftDefs{base: s.defs, trial: map[domain.MonsterID]domain.MonsterDef{guardian: native}}
	return s
}

func onlyGuardian(t *testing.T, s *Scene) *entity.Entity {
	t.Helper()
	if len(s.monsters) != 1 {
		t.Fatalf("want one guardian, got%d phase%s", len(s.monsters), s.rift.data.Run.Phase)
	}
	for _, m := range s.monsters {
		return m
	}
	return nil
}

func TestRiftGuardianHPActualSpawnOnlyHealthChanges(t *testing.T) {
	for _, tier := range []int32{1, 30, 150} {
		for _, party := range []int{1, 5} {
			for _, base := range []int32{1, 1009, math.MaxInt32} {
				t.Run(fmt.Sprintf("tier%d-party%d-base%d", tier, party, base), func(t *testing.T) {
					before, after := guardianHPSpawnFixture(t, 12, tier, party, base), guardianHPSpawnFixture(t, 13, tier, party, base)
					for _, s := range []*Scene{before, after} {
						s.rift.data.Run.Phase = domain.RiftBoss
						s.populateEndgameRift(true)
					}
					old, got := onlyGuardian(t, before), onlyGuardian(t, after)
					want := *old
					want.HP = max(int32(1), old.HP/10)
					want.MaxHP = want.HP
					want.Stats.MaxHP = want.HP
					if !reflect.DeepEqual(*got, want) {
						t.Fatalf("guardian changed beyond final HP: got%+v want%+v", *got, want)
					}
					if after.rift.baseStats[got.ID].MaxHP != got.MaxHP {
						t.Fatal("cached max HP diverged")
					}
					// The same shared definition still creates unmodified wilderness/dungeon/trial encounters.
					native, _ := after.riftNativeDefinition(got.Monster.TypeID)
					direct := spawn.Encounter(999, got.Pos, got.Dir, native)
					if native.HP != 0 || direct.MaxHP != 0 || native.Stats.Def != 37 || native.Exp != 9876 {
						t.Fatal("native/global template mutated")
					}
					got.HP = max(int32(1), got.HP-1)
					current := got.HP
					for i := 0; i < 3; i++ {
						after.populateEndgameRift(true)
					}
					if onlyGuardian(t, after) != got || got.HP != current {
						t.Fatal("retry duplicated, divided, or healed guardian")
					}
					// Reconstruct the paid run against its frozen rules and native sources.
					raw, err := json.Marshal(after.rift.data)
					if err != nil {
						t.Fatal(err)
					}
					var saved endgameRiftPersisted
					if err = json.Unmarshal(raw, &saved); err != nil {
						t.Fatal(err)
					}
					if !after.initEndgameRift(saved) {
						t.Fatal("restore failed")
					}
					after.rift.floorLoading = false
					after.populateEndgameRift(true)
					restored := onlyGuardian(t, after)
					if restored.HP != want.HP || restored.MaxHP != want.MaxHP || restored.Stats.MaxHP != want.MaxHP {
						t.Fatal("restore divided twice", restored.MaxHP, want.MaxHP)
					}
				})
			}
		}
	}
}

func TestRiftGuardianHPOrdinaryAndEliteUnaffected(t *testing.T) {
	before, after := guardianHPSpawnFixture(t, 12, 150, 5, 1009), guardianHPSpawnFixture(t, 13, 150, 5, 1009)
	before.populateEndgameRift(false)
	after.populateEndgameRift(false)
	if len(before.monsters) == 0 || !reflect.DeepEqual(before.monsters, after.monsters) || !reflect.DeepEqual(before.rift.baseStats, after.rift.baseStats) {
		t.Fatal("normal/blue/gold population changed")
	}
}

func TestRiftGuardianHPVersionValidationAndFrozenRewards(t *testing.T) {
	table, _ := guardianHPRulesFixture(t)
	old, _ := table.Version(12)
	fresh := table.Active()
	unchanged := *fresh
	unchanged.Version = 12
	unchanged.GuardianHPDivisor = 0
	if !reflect.DeepEqual(&unchanged, old) {
		t.Fatal("changed unrelated frozen rules")
	}
	for tier := int32(1); tier <= 150; tier++ {
		for _, timely := range []bool{false, true} {
			if domain.PlanRiftRewards(tier, timely, old) != domain.PlanRiftRewards(tier, timely, fresh) {
				t.Fatal("rewards changed", tier)
			}
		}
	}
	for _, r := range []*domain.RiftRules{old, fresh} {
		raw, _ := json.Marshal(&domain.RiftRun{RulesVersion: r.Version, Rules: r})
		var restored domain.RiftRun
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		if err := restored.BindRules(table); err != nil || !reflect.DeepEqual(restored.Rules, r) {
			t.Fatal("frozen run failed to bind", err)
		}
	}
	for _, change := range []func(*domain.RiftRules){
		func(r *domain.RiftRules) { r.GuardianHPDivisor = 0 }, func(r *domain.RiftRules) { r.GuardianHPDivisor = -1 }, func(r *domain.RiftRules) { r.GuardianHPDivisor = 1 }, func(r *domain.RiftRules) { r.GuardianHPDivisor = 100 }, func(r *domain.RiftRules) { r.Version = 12 },
	} {
		bad := table.Active()
		change(bad)
		if bad.Validate() == nil {
			t.Fatal("invalid guardian HP policy accepted")
		}
	}
}

// Keep the supported pre-elite spawn/recovery paths covered as well as the
// current approved-native guardian path. The HP policy itself is independent
// of layout/arrival presentation, but still belongs to a frozen rule version.
func guardianHPLegacyRules(t *testing.T, arrival int64) (*domain.RiftRuleTable, *Scene) {
	t.Helper()
	table, source := guardianHPRulesFixture(t)
	var rows = []domain.RiftRules{*riftRulesFixture()}
	for _, version := range []int32{12, 13} {
		r, _ := table.Version(version)
		r.Elite = nil
		r.Abilities = riftRulesFixture().Abilities
		r.FloorLayoutVersion = 0
		r.GuardianRecentWindow = 0
		r.GuardianArrivalMS = arrival
		rows = append(rows, *r)
	}
	result, err := domain.NewRiftRuleTable(13, rows, source.lootRules, source.items)
	if err != nil {
		t.Fatal(err)
	}
	return result, source
}

func TestRiftGuardianHPLegacySpawnAndRetry(t *testing.T) {
	for _, version := range []int32{12, 13} {
		s, _, now := riftFixture(t)
		table, source := guardianHPLegacyRules(t, 0)
		s.riftRules, s.lootRules, s.items = table, source.lootRules, source.items
		rule, _ := table.Version(version)
		r, err := domain.NewRiftRun("legacy-guardian-hp", 150, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, *now, rule)
		if err != nil {
			t.Fatal(err)
		}
		if !s.initEndgameRift(endgameRiftPersisted{Version: 1, Run: r, MapID: 7}) {
			t.Fatal("legacy init failed")
		}
		r.Phase = domain.RiftBoss
		original := s.trial.Monsters[60][2]
		want := domain.RiftScaledStat(original.HP, 150, 1, 3, true, rule)
		for i := 0; i < 3; i++ {
			s.populateEndgameRift(true)
			m := onlyGuardian(t, s)
			if m.HP != want || m.MaxHP != want || m.Stats.MaxHP != want {
				t.Fatal("legacy health mismatch", version, i)
			}
			if !reflect.DeepEqual(original, s.trial.Monsters[60][2]) {
				t.Fatal("trial source mutated")
			}
		}
		s.clearRiftMonsters()
		s.populateEndgameRift(true)
		if onlyGuardian(t, s).MaxHP != want {
			t.Fatal("legacy reconstruction divided twice")
		}
	}
}

func TestRiftGuardianHPDelayedArrivalReload(t *testing.T) {
	s, b, now := arrivalRiftFixture(t)
	table, source := guardianHPLegacyRules(t, 5000)
	s.riftRules, s.lootRules, s.items = table, source.lootRules, source.items
	r := s.rift.data.Run
	r.Rules, r.RulesVersion = table.Active(), 13
	b.runs[r.RunID] = s.rift.data.record(s.rift.revision)
	triggerRiftArrival(t, s, false)
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	deadline := s.rift.data.GuardianArrival.DeadlineMS
	record := b.runs[r.RunID]
	restored, ok := decodeEndgameRift(&record, table)
	if !ok || !s.initEndgameRift(restored) {
		t.Fatal("persisted arrival restore failed")
	}
	s.syncEndgameRift(s.players[1])
	s.stepEndgameRift()
	*now = deadline - 1
	s.stepEndgameRift()
	if len(s.monsters) != 0 {
		t.Fatal("guardian spawned early")
	}
	*now = deadline
	s.stepEndgameRift()
	s.stepEndgameRift()
	m := onlyGuardian(t, s)
	old, _ := table.Version(12)
	want := max(int32(1), domain.RiftScaledStat(1000, r.Tier, len(r.Members), 3, true, old)/10)
	if m.HP != want || m.MaxHP != want || m.Stats.MaxHP != want {
		t.Fatal("delayed restored guardian HP", m.HP, want)
	}
	for i := 0; i < 3; i++ {
		s.syncEndgameRift(s.players[1])
		s.populateEndgameRift(true)
		s.stepEndgameRift()
	}
	if onlyGuardian(t, s) != m || m.HP != want {
		t.Fatal("arrival retries changed HP or duplicated guardian")
	}
}

func TestRiftGuardianHPRejectsZeroTrialBase(t *testing.T) {
	s := guardianHPSpawnFixture(t, 13, 150, 5, 0)
	s.rift.data.Run.Phase = domain.RiftBoss
	s.populateEndgameRift(true)
	if len(s.monsters) != 0 || s.rift.data.Run.Phase != domain.RiftFailed || s.rift.data.Run.Reason != "spawn_failed" {
		t.Fatal("invalid zero source became a guardian")
	}
}
