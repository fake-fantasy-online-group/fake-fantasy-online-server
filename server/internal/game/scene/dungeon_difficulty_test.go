package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func TestDungeonDifficultyStatsFromOriginalTemplate(t *testing.T) {
	for _, kind := range []domain.MonsterKind{domain.MonsterNormal, domain.MonsterElite, domain.MonsterBoss} {
		d := domain.MonsterDef{ID: 99, Kind: kind, HP: 1000, Stats: domain.Stats{Hit: 37, Def: 11, MDef: 17, MinAtk: 90, MaxAtk: 110, MAtk: 55}}
		s := &Scene{id: domain.SceneID{MapID: 20502, Instance: 1}, dungeon: &塔狱, dungeons: domain.DungeonTable{20502: 塔狱}, defs: spawn.MapDefs{99: d}}
		m := spawn.Encounter(90001, domain.Pos{MapID: 20502}, 0, d)
		for _, difficulty := range []domain.DungeonDifficulty{domain.DungeonNormal, domain.DungeonHard, domain.DungeonNormal, domain.DungeonHard} {
			s.dungeonDifficulty = difficulty
			s.scaleDungeonMonster(m)
			hp, hit := int32(500), int32(37)
			if difficulty == domain.DungeonHard {
				hp = 1500
				if kind == domain.MonsterBoss {
					hp = 1000
					hit = 74
				}
			}
			if m.HP != hp || m.MaxHP != hp || m.Stats.Hit != hit {
				t.Fatalf("kind=%v difficulty=%v HP=%v max=%v hit=%v", kind, difficulty, m.HP, m.MaxHP, m.Stats.Hit)
			}
			if m.Stats.Def != 11 || m.Stats.MDef != 17 || m.Stats.MinAtk != 90 || m.Stats.MaxAtk != 110 || m.Stats.MAtk != 55 {
				t.Fatal("unrequested stats changed")
			}
		}
		if d.HP != 1000 || d.Stats.Hit != 37 {
			t.Fatal("template mutated")
		}
		s.rift = &endgameRiftState{}
		m.HP = 123
		m.MaxHP = 456
		m.Stats.Hit = 789
		s.scaleDungeonMonster(m)
		if m.HP != 123 || m.MaxHP != 456 || m.Stats.Hit != 789 {
			t.Fatal("rift changed")
		}
	}
}

func TestDungeonDifficultyPartyReuseAndFloorInheritance(t *testing.T) {
	rig := dungeonSetup(t)
	owner := domain.DungeonOwner{Party: 1}
	hard, err := rig.r.openDungeonAccessDifficulty(塔狱, owner, true, nil, 0, domain.DungeonHard, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rig.r.openDungeonAccessDifficulty(塔狱, owner, true, nil, 0, domain.DungeonNormal, false)
	if err != nil || got != hard {
		t.Fatalf("ordinary party join failed: %v %v", got, err)
	}
	if _, err = rig.r.openDungeonAccessDifficulty(塔狱, owner, true, nil, 0, domain.DungeonNormal, true); err == nil {
		t.Fatal("opposite selection replaced active run")
	}
	floor := domain.SceneID{MapID: 20503, Instance: hard.Instance}
	if err = rig.r.EnsureDungeonFloor(floor, owner); err != nil {
		t.Fatal(err)
	}
	rig.r.mu.RLock()
	for _, id := range []domain.SceneID{hard, floor} {
		if rig.r.dungeonDifficulties[id] != domain.DungeonHard || rig.r.scenes[id].dungeonDifficulty != domain.DungeonHard {
			t.Error("floor lost difficulty", id)
		}
	}
	rig.r.mu.RUnlock()
	if err = rig.r.EnsureDungeonFloor(domain.SceneID{MapID: 20504, Instance: hard.Instance}, domain.DungeonOwner{Party: 2}); err == nil {
		t.Fatal("wrong owner admitted")
	}
	other, err := rig.r.openDungeonAccessDifficulty(塔狱, domain.DungeonOwner{Party: 2}, true, nil, 0, domain.DungeonNormal, true)
	if err != nil || other == hard {
		t.Fatal("separate party failed", err)
	}
}

func TestTowerPortalWaitsForNativeDifficultySelection(t *testing.T) {
	rig := dungeonSetup(t)
	town, err := rig.r.Ensure(domain.SceneID{MapID: 507})
	if err != nil {
		t.Fatal(err)
	}
	town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: &fakeSink{}, Party: 1, Char: &domain.Character{ID: 100, Name: "selector", Pos: domain.Pos{MapID: 507, X: 100, Y: 100}}})
	rig.step(t, town, func(s *Scene) {
		s.portals = []domain.Portal{domain.NewPortal(domain.Teleport{Dungeon: true, To: 塔狱.Enter, Area: domain.Polygon{{X: 50, Y: 50}, {X: 150, Y: 50}, {X: 150, Y: 150}, {X: 50, Y: 150}}})}
		p := s.players[1]
		if s.checkPortals(&p.Pos, p.ID) {
			t.Fatal("portal admitted before native choice")
		}
		if len(rig.instances(20502)) != 0 {
			t.Fatal("portal created normal instance")
		}
		if !s.onEnterDungeon(EnterDungeon{ID: 1, MapID: 20502, Difficulty: domain.DungeonHard, ExplicitDifficulty: true, NativeMenu: true}) {
			t.Fatal("hard menu rejected in native portal")
		}
	})
	instances := rig.instances(20502)
	if len(instances) != 1 {
		t.Fatal(instances)
	}
	dungeon, _ := rig.r.Ensure(instances[0])
	rig.step(t, dungeon, func(s *Scene) {
		if s.players[1] == nil || s.dungeonDifficulty != domain.DungeonHard {
			t.Fatal("native selection did not admit hard")
		}
	})
}

func TestTowerNativeCommandRequiresEntranceAndParty(t *testing.T) {
	rig := dungeonSetup(t)
	town, _ := rig.r.Ensure(domain.SceneID{MapID: 507})
	town.Post(Enter{ID: 1, Bag: domain.NewBag(10), Sink: &fakeSink{}, Party: 1, Char: &domain.Character{ID: 100, Name: "selector", Pos: domain.Pos{MapID: 507, X: 100, Y: 100}}})
	rig.step(t, town, func(s *Scene) {
		if s.onEnterDungeon(EnterDungeon{ID: 1, MapID: 20502, Difficulty: domain.DungeonHard, ExplicitDifficulty: true, NativeMenu: true}) {
			t.Fatal("remote tower entry accepted")
		}
		s.portals = []domain.Portal{domain.NewPortal(domain.Teleport{Dungeon: true, To: 塔狱.Enter, Area: domain.Polygon{{X: 50, Y: 50}, {X: 150, Y: 50}, {X: 150, Y: 150}, {X: 50, Y: 150}}})}
		s.players[1].Player.Party = 0
		if s.onEnterDungeon(EnterDungeon{ID: 1, MapID: 20502, Difficulty: domain.DungeonHard, ExplicitDifficulty: true, NativeMenu: true}) {
			t.Fatal("unpartied tower entry accepted")
		}
	})
	if len(rig.instances(20502)) != 0 {
		t.Fatal("invalid selection allocated run")
	}
}

func TestDungeonDifficultyExcludesPublicAndUnrelatedInstances(t *testing.T) {
	d := domain.MonsterDef{ID: 99, Kind: domain.MonsterBoss, HP: 1000, Stats: domain.Stats{Hit: 37}}
	for _, id := range []domain.SceneID{{MapID: 20502}, {MapID: 123, Instance: 1}} {
		def := 塔狱
		if id.MapID == 123 {
			def.ID = 123
		}
		s := &Scene{id: id, dungeon: &def, dungeonDifficulty: domain.DungeonHard, dungeons: domain.DungeonTable{20502: 塔狱}, defs: spawn.MapDefs{99: d}}
		m := spawn.Encounter(90001, domain.Pos{MapID: id.MapID}, 0, d)
		s.scaleDungeonMonster(m)
		if m.HP != 1000 || m.Stats.Hit != 37 {
			t.Fatal("unrelated scene scaled", id)
		}
	}
}

func TestDungeonDifficultyFailedCreationDoesNotLeakMetadata(t *testing.T) {
	r := NewRouter(func(domain.SceneID) (*Scene, error) { t.Fatal("unstarted router must not build"); return nil, nil }, nil)
	if _, err := r.openDungeonAccessDifficulty(塔狱, domain.DungeonOwner{Party: 1}, true, nil, 0, domain.DungeonHard, true); err == nil {
		t.Fatal("unstarted router admitted")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.dungeonDifficulties) != 0 {
		t.Fatal("failed creation retained metadata")
	}
}
