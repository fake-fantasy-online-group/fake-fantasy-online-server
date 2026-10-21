package scene

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	fixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/monsterbehavior"
	"github.com/jackc/pgx/v5"
	"os"
	"testing"
)

func TestDungeonAggressionIsSceneScoped(t *testing.T) {
	for _, mode := range []string{"world", "ordinary", "trial", "rift"} {
		for _, level := range []int32{1, 29, 60, 120} {
			for _, kind := range []domain.MonsterKind{domain.MonsterNormal, domain.MonsterElite, domain.MonsterBoss, domain.MonsterProp, domain.MonsterGather} {
				for _, noAttack := range []bool{false, true} {
					d := 被动树妖
					d.Level = level
					d.Kind = kind
					d.NoAttack = noAttack
					s, id := aiScene(t, d, 140, 100)
					switch mode {
					case "ordinary":
						s.dungeon = &domain.DungeonDef{ID: 20701}
					case "trial":
						s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{14: {{}}}}
					case "rift":
						s.rift = &endgameRiftState{}
					}
					got := s.monsterAIParams(s.entities[id])
					want := mode != "world" && kind.Hostile() && !noAttack
					if got.Aggressive != want {
						t.Fatalf("%s level%d kind%v noAttack%v: aggressive=%v", mode, level, kind, noAttack, got.Aggressive)
					}
					if s.entities[id].Monster.Aggressive || d.Aggressive {
						t.Fatal("mutated native aggression")
					}
					// The override must not expand view/trace/attack ranges or movement.
					original := aiParams(s.entities[id])
					got.Aggressive = original.Aggressive
					if got != original {
						t.Fatal("changed non-aggression AI parameters")
					}
				}
			}
		}
	}
}
func TestDungeonPassiveNativeActuallyAcquiresAndStationaryAttacks(t *testing.T) {
	for _, mode := range []string{"ordinary", "trial", "rift"} {
		for _, stationary := range []bool{false, true} {
			d := 被动树妖
			d.Level = 120
			if stationary {
				d.Stats.MoveSpeed = 0
			}
			s, id := aiScene(t, d, 140, 100)
			join(t, s, 1, 100, "甲", 100, 100)
			switch mode {
			case "ordinary":
				s.dungeon = &domain.DungeonDef{ID: 20701}
			case "trial":
				s.trial = &domain.TrialRules{Maps: map[int32][]domain.TrialSpawn{14: {{}}}}
			case "rift":
				s.rift = &endgameRiftState{data: endgameRiftPersisted{Run: &domain.RiftRun{Phase: domain.RiftActive}}}
			}
			m := s.entities[id]
			start := m.Pos
			for i := 0; i < aiScanEvery*2; i++ {
				s.tick++
				s.stepAI()
			}
			if m.Monster.Target != 1 || len(s.attacks) == 0 {
				t.Fatal("dungeon monster did not acquire and queue attack")
			}
			if stationary && m.Pos != start {
				t.Fatal("stationary native monster moved")
			}
		}
	}

}

// Exercise the actual migration result through spawning and scene AI. No whole
// database bootstrap, artificial aggression flags, or user database required.
func TestRestoredNativeAggressionSceneActualPostgres(t *testing.T) {
	dsn := os.Getenv("FANTASY_NATIVE_AGGRO_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	conn, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close(ctx)
	tx, e := conn.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	const dir = "../../../migrations"
	native := fixture.Read(t, dir)
	fixture.Seed(t, ctx, tx, native)
	for _, name := range []string{"000000_init-88.sql", "000000_init-97.sql"} {
		if _, e := tx.Exec(ctx, fixture.SQL(t, dir, name)); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct {
		id            int32
		dungeon, want bool
	}{{3049, true, true}, {3050, true, true}, {3051, true, true}, {3080, true, true}, {3081, true, true}, {3082, true, true}, {3060, true, true}, {1003, false, true}, {1001, false, false}, {1001, true, true}, {1002, false, true}} {
		var r fixture.Row
		if e := tx.QueryRow(ctx, "SELECT id,name,level,hp,kind,aggressive,move_speed,atk_speed FROM game_monsters WHERE id=$1", tc.id).Scan(&r.ID, &r.Name, &r.Level, &r.HP, &r.Kind, &r.Aggressive, &r.MoveSpeed, &r.AttackSpeed); e != nil {
			t.Fatal(e)
		}
		d := domain.MonsterDef{ID: domain.MonsterID(r.ID), Name: r.Name, Level: r.Level, HP: r.HP, Kind: domain.ParseMonsterKind(r.Kind), Aggressive: r.Aggressive, ViewDist: 400, TraceDist: 600, Stats: domain.NewMonsterStats(1, 0, 9999, 0, 0, r.AttackSpeed, r.MoveSpeed)}
		s, id := aiScene(t, d, 140, 100)
		if tc.dungeon {
			s.dungeon = &domain.DungeonDef{ID: 20701}
		}
		join(t, s, 1, 100, "甲", 100, 100)
		for i := 0; i < aiScanEvery*2; i++ {
			s.tick++
			s.stepAI()
		}
		if (s.entities[id].Monster.Target == 1) != tc.want {
			t.Fatalf("%d dungeon=%v: target=%d", tc.id, tc.dungeon, s.entities[id].Monster.Target)
		}
		if tc.want && len(s.attacks) == 0 {
			t.Fatalf("%d did not queue a nearby attack", tc.id)
		}
	}
}
