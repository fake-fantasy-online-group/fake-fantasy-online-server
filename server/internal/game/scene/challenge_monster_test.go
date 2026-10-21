package scene

import (
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func TestSeededChallengeKeepsNativeMovementSkillsAndAI(t *testing.T) {
	for _, speed := range []int32{0, 100} {
		t.Run(string(rune('a'+speed)), func(t *testing.T) {
			s, _ := seededRiftLayoutFixture(t, 1)
			native, _ := s.defs.Def(501)
			native.Stats.MoveSpeed = speed
			native.AI = domain.MonsterAIProfile{ID: "caster", ViewDist: 321, TraceDist: 700, BasicAttackDist: 450, HelpRadius: 99}
			native.NoBasicAttack = true
			native.Skills = []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}, InitialDelay: 7, Enabled: true}}
			native.ImmuneSkills = map[domain.SkillID]struct{}{12001: {}}
			s.defs.(spawn.MapDefs)[501] = native
			for pass := 0; pass < 2; pass++ {
				s.populateEndgameRift(false)
				for id, m := range s.monsters {
					d, _ := s.defs.Def(m.Monster.TypeID)
					if m.Stats.MoveSpeed != speed*3/2 || m.Monster.AI.TraceDist != 2100 || m.Monster.AI.ViewDist != 321 || !m.Monster.NoBasicAttack || !reflect.DeepEqual(d.Skills, native.Skills) || !reflect.DeepEqual(d.ImmuneSkills, native.ImmuneSkills) || m.Monster.SkillReady[12013] != s.tick+7 {
						t.Fatal("native template lost", m, d)
					}
					if class := s.rift.data.Run.Monsters[id].Class; class > 0 && s.rift.abilities[id] == nil {
						t.Fatal("lost independent affixes")
					}
					for repeat := 0; repeat < 3; repeat++ {
						s.refreshEntityStats(m)
						if m.Stats.MoveSpeed != speed*3/2 {
							t.Fatal("refresh compounded", m.Stats)
						}
					}
					// Even additive move buffs cannot make a native stationary monster walk.
					if speed == 0 {
						s.applyStatusDef(m, domain.StatusDef{ID: 199998, Level: 1, Permanent: true, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModeAbsolute, Value: 100}}}, m.ID)
						if m.Stats.MoveSpeed != 0 {
							t.Fatal("buff removed native immobility")
						}
					}
				}
				s.clearRiftMonsters()
			}
		})
	}
}

func TestChallengePathFailureWaitIsBoundedAndDoesNotMove(t *testing.T) {
	for _, challenge := range []bool{false, true} {
		d := 主动树妖
		if challenge {
			d = domain.ChallengeMonsterBehavior(d)
		}
		s, id := aiScene(t, d, 100, 100)
		if challenge {
			s.trialLevel = 60
			s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{14: {{}}}}
		}
		join(t, s, 1, 100, "target", 500, 100)
		s.entities[1].Pos.MapID = 14
		m := s.monsters[id]
		m.Monster.Target = 1
		s.walkable = func(p domain.Pos) bool { return p.X < 200 || p.X > 300 }
		start := m.Pos
		s.stepAI()
		if !challenge {
			if m.Monster.Target != 0 {
				t.Fatal("world behavior changed")
			}
			continue
		}
		if m.Monster.Target != 1 || m.Pos != start {
			t.Fatal("challenge did not hold target in place")
		}
		deadline := m.Monster.ChallengePathWaitUntil
		for s.tick < deadline {
			s.tick++
			s.stepAI()
			if m.Pos != start {
				t.Fatal("walked through wall")
			}
		}
		if m.Monster.Target != 0 {
			t.Fatal("unreachable target held forever")
		}
	}
}

func TestChallengeTraceBoundaryAndRootedMovement(t *testing.T) {
	d := domain.ChallengeMonsterBehavior(主动树妖)
	s, id := aiScene(t, d, 100, 100)
	s.trialLevel = 60
	s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{14: {{}}}}
	join(t, s, 1, 100, "target", 1900, 100)
	s.entities[1].Pos.MapID = 14
	m := s.monsters[id]
	m.Monster.Target = 1
	s.applyStatusDef(m, domain.StatusDef{ID: 199997, Level: 1, Permanent: true, BlockMove: true}, m.ID)
	start := m.Pos
	s.stepAI()
	if m.Monster.Target != 1 || m.Pos != start {
		t.Fatal("boundary/root semantics")
	}
	s.entities[1].Pos.X = 1901
	s.stepAI()
	if m.Monster.Target != 0 {
		t.Fatal("new trace distance not enforced")
	}
}

func TestTrialNativeSummonsUseFallbackCatalogAndScaleOnce(t *testing.T) {
	s, id := aiScene(t, 主动树妖, 100, 100)
	caster := s.monsters[id]
	s.alloc = domain.NewEntityAlloc()
	s.alloc.Monster() // The fixture already allocated its original monster.
	native := 食人花
	native.Stats.MoveSpeed = 80
	s.defs = spawn.MapDefs{主动树妖.ID: 主动树妖, native.ID: native}
	s.router = &Router{trialLevels: map[domain.SceneID]int32{s.id: 60}}
	s.trial = &domain.TrialRules{MinLevel: 1, MaxLevel: 100, NormalCount: 1,
		Maps:     map[int32][]domain.TrialSpawn{14: {{Point: domain.SpawnPoint{ID: 3, Pos: domain.Pos{MapID: 14, X: 200, Y: 200}}, Candidates: []domain.MonsterID{主动树妖.ID}}}},
		Monsters: map[int32]map[domain.MonsterID]domain.MonsterDef{60: {主动树妖.ID: domain.ChallengeMonsterBehavior(主动树妖)}}}
	s.prepareTrial(Enter{})
	selected, _ := s.defs.Def(主动树妖.ID)
	if selected.Stats.MoveSpeed != 165 || selected.TraceDist != 1800 {
		t.Fatal("trial doubled challenge behavior", selected)
	}
	if _, ok := s.defs.Def(native.ID); !ok {
		t.Fatal("trial discarded native summon catalog")
	}
	s.summonMonsters(caster, domain.MonsterSkillExtra{SummonMonster: native.ID, Count: 1})
	count := 0
	for _, m := range s.monsters {
		if m.Monster.Summoner == caster.ID {
			count++
			if m.Stats.MoveSpeed != 120 || aiParams(m).TraceDist != 1800 {
				t.Fatal("summon lost challenge behavior", m)
			}
			for i := 0; i < 3; i++ {
				s.refreshEntityStats(m)
				if m.Stats.MoveSpeed != 120 {
					t.Fatal("summon refresh compounded")
				}
			}
		}
	}
	if count != 1 {
		t.Fatal("native summon not created", count)
	}
}

func TestSeededChallengePreservesNativePassiveMovementLock(t *testing.T) {
	s, _ := seededRiftLayoutFixture(t, 1)
	native, _ := s.defs.Def(501)
	native.Stats.MoveSpeed = 100
	native.Skills = []domain.MonsterSkillRule{{Enabled: true, Skill: domain.SkillDef{ID: 12345, Kind: domain.SkillPassive, Statuses: []domain.StatusApplication{{ID: 199996, Level: 1}}}}}
	s.defs.(spawn.MapDefs)[501] = native
	s.statuses = domain.StatusTable{{ID: 199996, Level: 1}: {ID: 199996, Level: 1, Permanent: true, BlockMove: true}}
	s.populateEndgameRift(false)
	for _, m := range s.monsters {
		if canMove(m) || m.Stats.MoveSpeed != 150 {
			t.Fatal("native passive lock not retained", m.Status, m.Stats)
		}
	}
}

func TestLegacyRiftChallengeBehaviorIsNotCompounded(t *testing.T) {
	s, _ := seededRiftLayoutFixture(t, 1)
	for level, defs := range s.trial.Monsters {
		for id, d := range defs {
			d.Stats.MoveSpeed = 80
			d.TraceDist = 600
			d.AI = domain.MonsterAIProfile{}
			s.trial.Monsters[level][id] = domain.ChallengeMonsterBehavior(d)
		}
	}
	s.rift.data.Floor = nil
	s.populateEndgameRift(false)
	if len(s.monsters) == 0 {
		t.Fatal("missing legacy population")
	}
	for _, m := range s.monsters {
		if m.Stats.MoveSpeed != 120 || aiParams(m).TraceDist != 1800 {
			t.Fatal("legacy multiplier compounded", m.Stats, m.Monster)
		}
	}
}

func TestChallengeCloneAndSummonBaselinesRemainSingleScaled(t *testing.T) {
	s, m, _, now := riftEliteFixture(t, 1)
	d, _ := s.defs.Def(m.Monster.TypeID)
	d = domain.ChallengeMonsterBehavior(d)
	s.defs.(spawn.MapDefs)[d.ID] = d
	m.Stats = d.Stats
	m.Monster.AI = d.AI
	m.Monster.TraceDist = d.TraceDist
	s.rift.baseStats[m.ID] = d.Stats
	if !s.spawnRiftClones(m, s.currentRiftRules().Abilities[1], *now) {
		t.Fatal("clone creation failed")
	}
	for id := range s.eliteSnapshot().Clones {
		c := s.monsters[id]
		s.refreshEntityStats(c)
		if c.Stats.MoveSpeed != 150 || aiParams(c).TraceDist != 1800 {
			t.Fatal("clone compounded", c.Stats, c.Monster)
		}
	}
	child := 主动树妖
	child.ID = 123456
	child.Stats.MoveSpeed = 80
	s.defs.(spawn.MapDefs)[child.ID] = child
	s.summonMonsters(m, domain.MonsterSkillExtra{SummonMonster: child.ID, Count: 1})
	count := 0
	for _, c := range s.monsters {
		if c.Monster.TypeID != child.ID {
			continue
		}
		count++
		s.refreshEntityStats(c)
		if c.Stats.MoveSpeed != 120 || aiParams(c).TraceDist != 1800 {
			t.Fatal("summon wrong baseline", c)
		}
		s.despawnSummonedMonster(c)
		if _, ok := s.rift.baseStats[c.ID]; ok {
			t.Fatal("despawn leaked baseline")
		}
	}
	if count != 1 {
		t.Fatal("summon missing")
	}
}

func TestChallengePathGraceResetsOnDisengageAndRespawn(t *testing.T) {
	d := domain.ChallengeMonsterBehavior(主动树妖)
	s, id := aiScene(t, d, 100, 100)
	s.trialLevel = 60
	s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{14: {{}}}}
	join(t, s, 1, 100, "target", 500, 100)
	s.entities[1].Pos.MapID = 14
	m := s.monsters[id]
	s.walkable = func(p domain.Pos) bool { return p.X < 200 || p.X > 300 }
	m.Monster.Target = 1
	s.stepAI()
	first := m.Monster.ChallengePathWaitUntil
	if first == 0 {
		t.Fatal("missing initial grace")
	}
	s.dropTarget(m)
	if m.Monster.ChallengePathWaitUntil != 0 {
		t.Fatal("stale deadline survived disengage")
	}
	s.tick = first + 10
	m.Monster.Target = 1
	s.stepAI()
	if m.Monster.Target != 1 || m.Monster.ChallengePathWaitUntil <= s.tick {
		t.Fatal("new target did not get fresh grace")
	}
	s.initializeMonster(m)
	if m.Monster.ChallengePathWaitUntil != 0 {
		t.Fatal("stale deadline survived initialization")
	}
}

func TestChallengeNativeResolutionSkipsOnlyGeneratedOverrides(t *testing.T) {
	native := 主动树妖
	native.Stats.MoveSpeed = 80
	native.AI = domain.MonsterAIProfile{ID: "native", TraceDist: 600}
	catalog := endgameRiftDefs{trial: map[domain.MonsterID]domain.MonsterDef{native.ID: native}}
	generated := domain.ChallengeMonsterBehavior(native)
	generated.Stats.MinAtk = 9999
	// A native extension can sit above already-generated encounter definitions.
	// Resolving an unrelated original must not stop at that extension's fallback.
	extension := native
	extension.ID = 987654
	extension.Name = "native guardian extension"
	s := &Scene{defs: endgameRiftDefs{base: endgameRiftDefs{base: catalog, trial: map[domain.MonsterID]domain.MonsterDef{native.ID: generated}, scaled: true}, trial: map[domain.MonsterID]domain.MonsterDef{extension.ID: extension}}}
	got, ok := s.riftNativeDefinition(native.ID)
	if !ok || !reflect.DeepEqual(got, native) {
		t.Fatal("native catalog lost behind generated layer", got)
	}
	got, ok = s.riftNativeDefinition(extension.ID)
	if !ok || !reflect.DeepEqual(got, extension) {
		t.Fatal("native extension lost", got)
	}
	if _, ok = s.riftNativeDefinition(999999); ok {
		t.Fatal("missing original was fabricated")
	}
	s.defs = endgameRiftDefs{trial: map[domain.MonsterID]domain.MonsterDef{native.ID: generated}, scaled: true}
	if _, ok = s.riftNativeDefinition(native.ID); ok {
		t.Fatal("scaled override mistaken for original")
	}
}

func TestChallengeSeededLifecycleCatalogRepopulationKeepsNativeSpeed(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	// The lifecycle fixture supplies a native catalog in a wrapper without a
	// separate base; repeat materialization must preserve that source underneath
	// generated overrides, not lose it or compound challenge multipliers.
	raw, ok := s.riftNativeDefinition(1)
	if !ok {
		t.Fatal("missing wrapped native catalog")
	}
	for pass := 0; pass < 3; pass++ {
		s.clearRiftMonsters()
		s.populateEndgameRift(false)
		if s.rift.data.Run.Phase != domain.RiftActive || len(s.monsters) == 0 {
			t.Fatal("population failed", pass)
		}
		for _, m := range s.monsters {
			if m.Stats.MoveSpeed != raw.Stats.MoveSpeed*3/2 || aiParams(m).TraceDist != 1800 {
				t.Fatal("repopulation compounded behavior", m.Stats, m.Monster)
			}
		}
	}
}
