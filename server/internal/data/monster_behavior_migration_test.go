package data

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type approvedMonsterBehavior struct {
	ID         int32  `json:"id"`
	Name       string `json:"name"`
	Aggressive bool   `json:"aggressive"`
	CallHelp   bool   `json:"call_help"`
	Flee       bool   `json:"flee"`
	Pickup     bool   `json:"pickup"`
}

func readApprovedMonsterBehaviors(t *testing.T) []approvedMonsterBehavior {
	t.Helper()
	raw, err := os.ReadFile("testdata/monster-behavior-approved.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []approvedMonsterBehavior
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}
func readBehaviorMigration(t *testing.T, incremental bool) string {
	t.Helper()
	name := "000000_init-88.sql"
	if incremental {
		name = "upgrades/20261019_monster_behavior.sql"
	}
	raw, err := os.ReadFile(filepath.Join("../../migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func TestMonsterBehaviorApprovedMigration(t *testing.T) {
	seed := readBehaviorMigration(t, false)
	upgrade := readBehaviorMigration(t, true)
	if !strings.Contains(upgrade, seed) || !strings.Contains(upgrade, "BEGIN;") || !strings.HasSuffix(upgrade, "COMMIT;\n") {
		t.Fatal("incremental upgrade must run identical seed data inside a transaction")
	}
	for _, guard := range []string{"m.level BETWEEN 1 AND 60 AND m.hp > 0", "m.kind IN ('普通', '精英', 'BOSS')", "a.id = m.id AND a.name = m.name", "COALESCE(a.aggressive, FALSE)", "COALESCE(a.call_help, FALSE)", "ADD COLUMN IF NOT EXISTS", "actual IS DISTINCT FROM ARRAY[68,23,15,27]", "RAISE EXCEPTION"} {
		if !strings.Contains(seed, guard) {
			t.Fatalf("missing scope/identity guard %s", guard)
		}
	}
	if strings.Contains(strings.ToLower(seed), "delete ") || strings.Contains(strings.ToLower(seed), "truncate ") || strings.Contains(seed, "schema_migrations") {
		t.Fatal("must not alter players or manufacture migration history")
	}
	re := regexp.MustCompile(`\((\d+), '([^']+)', (TRUE|FALSE), (TRUE|FALSE), (TRUE|FALSE), (TRUE|FALSE)\)`)
	matches := re.FindAllStringSubmatch(seed, -1)
	approved := readApprovedMonsterBehaviors(t)
	if len(matches) != len(approved) {
		t.Fatalf("migration rows%d approved%d", len(matches), len(approved))
	}
	counts := [4]int{}
	seen := map[int32]bool{}
	for i, m := range matches {
		id, _ := strconv.Atoi(m[1])
		got := approvedMonsterBehavior{int32(id), m[2], m[3] == "TRUE", m[4] == "TRUE", m[5] == "TRUE", m[6] == "TRUE"}
		if got != approved[i] || seen[got.ID] {
			t.Fatalf("incorrect/duplicate decision %+v", got)
		}
		seen[got.ID] = true
		for j, v := range []bool{got.Aggressive, got.CallHelp, got.Flee, got.Pickup} {
			if v {
				counts[j]++
			}
		}
		if got.Pickup && (strings.Contains(got.Name, "兵勇") || strings.Contains(got.Name, "骁将")) {
			t.Fatal("unapproved human pickup", got)
		}
	}
	if counts != [4]int{68, 23, 15, 27} {
		t.Fatalf("decision counts %v", counts)
	}
	for _, id := range []int32{1201, 4141, 5081, 12176} {
		if !seen[id] {
			t.Fatalf("approved pickup missing%d", id)
		}
	}
}

// Opt-in tests use only temporary tables, never persistent game/player data.
func TestMonsterBehaviorIncrementalPostgres(t *testing.T) {
	dsn := os.Getenv("FANTASY_MONSTER_BEHAVIOR_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test DSN required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `CREATE TEMP TABLE game_monsters(id INTEGER PRIMARY KEY,name TEXT,level INTEGER,hp INTEGER,kind TEXT,aggressive BOOLEAN,call_help BOOLEAN);
 CREATE TEMP TABLE game_monster_ai_templates(id TEXT PRIMARY KEY);
 INSERT INTO game_monster_ai_templates VALUES ('melee');`)
	if err != nil {
		t.Fatal(err)
	}
	rows := readApprovedMonsterBehaviors(t)
	for _, r := range rows {
		_, err = conn.Exec(ctx, "INSERT INTO game_monsters(id,name,level,hp,kind,aggressive,call_help) VALUES($1,$2,60,100,'普通',TRUE,TRUE)", r.ID, r.Name)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Start with the old schema to prove the upgrade creates the new columns.
	if _, err = conn.Exec(ctx, readBehaviorMigration(t, true)); err != nil {
		t.Fatal(err)
	}
	var hp, duration, cooldown int
	if err = conn.QueryRow(ctx, "SELECT flee_hp_percent,flee_duration_ms,flee_cooldown_ms FROM game_monster_ai_templates WHERE id='melee'").Scan(&hp, &duration, &cooldown); err != nil {
		t.Fatal(err)
	}
	if hp != 20 || duration != 3000 || cooldown != 10000 {
		t.Fatalf("wrong defaults %d/%d/%d", hp, duration, cooldown)
	}
	_, err = conn.Exec(ctx, `INSERT INTO game_monsters VALUES
 (900001,'unlisted',60,100,'精英',TRUE,TRUE,FALSE,FALSE),
 (900002,'above60',61,100,'普通',TRUE,TRUE,TRUE,TRUE),
 (900003,'prop',60,100,'场景物件',TRUE,TRUE,TRUE,TRUE),
 (900004,'placeholder',0,0,'普通',TRUE,TRUE,TRUE,TRUE);`)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		if _, err = conn.Exec(ctx, readBehaviorMigration(t, true)); err != nil {
			t.Fatal(err)
		}
		for _, want := range rows {
			got := approvedMonsterBehavior{ID: want.ID, Name: want.Name}
			if err = conn.QueryRow(ctx, "SELECT aggressive,call_help,flee,pickup FROM game_monsters WHERE id=$1", want.ID).Scan(&got.Aggressive, &got.CallHelp, &got.Flee, &got.Pickup); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("run%d got%+v want%+v", run, got, want)
			}
		}
		for id := 900001; id <= 900004; id++ {
			var a, h, f, p bool
			if err = conn.QueryRow(ctx, "SELECT aggressive,call_help,flee,pickup FROM game_monsters WHERE id=$1", id).Scan(&a, &h, &f, &p); err != nil {
				t.Fatal(err)
			}
			want := id != 900001
			if a != want || h != want || f != want || p != want {
				t.Fatalf("scope not preserved id%d", id)
			}
		}
	}

	// Identity drift must reject and roll back the flag update atomically.
	for _, mutation := range []string{
		"UPDATE game_monsters SET name='renamed' WHERE id=1002",
		"UPDATE game_monsters SET level=61 WHERE id=1002",
		"DELETE FROM game_monsters WHERE id=1002",
	} {
		if _, err = conn.Exec(ctx, mutation); err != nil {
			t.Fatal(err)
		}
		if _, err = conn.Exec(ctx, "UPDATE game_monsters SET aggressive=FALSE WHERE id=1003"); err != nil {
			t.Fatal(err)
		}
		if _, err = conn.Exec(ctx, readBehaviorMigration(t, true)); err == nil || !strings.Contains(err.Error(), "approved identity/scope mismatch") {
			t.Fatalf("expected explicit identity drift failure, got %v", err)
		}
		if _, err = conn.Exec(ctx, "ROLLBACK"); err != nil {
			t.Fatal(err)
		}
		var active bool
		if err = conn.QueryRow(ctx, "SELECT aggressive FROM game_monsters WHERE id=1003").Scan(&active); err != nil || active {
			t.Fatalf("failed migration did not roll back flag update: active=%v error=%v", active, err)
		}
		if _, err = conn.Exec(ctx, "INSERT INTO game_monsters(id,name,level,hp,kind,aggressive,call_help,flee,pickup) VALUES(1002,'食人花',60,100,'普通',TRUE,FALSE,FALSE,FALSE) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,level=EXCLUDED.level"); err != nil {
			t.Fatal(err)
		}
	}

}

type behaviorLoaderQuery struct{}

func (behaviorLoaderQuery) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	var values [][]any
	switch {
	case strings.Contains(query, "FROM game_monster_color_profiles"):
		values = [][]any{{"", 1}}
	case strings.Contains(query, "FROM game_monsters m"):
		for _, col := range []string{"m.flee", "m.pickup", "a.flee_hp_percent", "a.flee_duration_ms", "a.flee_cooldown_ms"} {
			if !strings.Contains(query, col) {
				return nil, fmt.Errorf("missing column %s", col)
			}
		}
		values = [][]any{{1005, "test", "普通", 0, 20, 100, 1, 1, 10, 20, 1, 1, 1, 1, 0, 0, 2000, 100, true, true, true, true, true, true, "melee", 400, 600, 80, 0, 250, 500, 500, 20, 3000, 10000}}
	}
	out := &v5SingleUserRows{at: -1}
	for _, row := range values {
		var r []json.RawMessage
		for _, v := range row {
			raw, _ := json.Marshal(v)
			r = append(r, raw)
		}
		out.values = append(out.values, r)
	}
	return out, nil
}
func TestMonsterBehaviorLoaderColumns(t *testing.T) {
	defs, err := LoadMonsters(context.Background(), behaviorLoaderQuery{})
	if err != nil {
		t.Fatal(err)
	}
	d := defs[1005]
	if !d.Flee || !d.Pickup || !d.CallHelp || !d.Aggressive || d.AI.FleeHPPercent != 20 || d.AI.FleeDurationTicks != domain.Ticks(3000) || d.AI.FleeCooldownTicks != domain.Ticks(10000) {
		t.Fatalf("behavior/profile not loaded: %+v", d)
	}
}

// This companion to the DSN test executes the production loader's SQL against
// the full, disposable PostgreSQL fixture produced by validate_monster_behavior.py.
// It deliberately makes no claim about pgx transport or concurrent sessions.
func TestMonsterBehaviorPostgresActualLoader(t *testing.T) {
	path := os.Getenv("FANTASY_MONSTER_BEHAVIOR_PG_DATA")
	if path == "" {
		t.Skip("isolated migration88 PostgreSQL cluster required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); !os.IsNotExist(err) {
		t.Fatalf("refusing active, stale-postmaster or inaccessible cluster: %v", err)
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	monsters, err := LoadMonsters(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	approved := map[domain.MonsterID]approvedMonsterBehavior{}
	for _, row := range readApprovedMonsterBehaviors(t) {
		approved[domain.MonsterID(row.ID)] = row
	}
	counts := [4]int{}
	matched := 0
	for id, monster := range monsters {
		if monster.AI.FleeHPPercent != 20 || monster.AI.FleeDurationTicks != domain.Ticks(3000) || monster.AI.FleeCooldownTicks != domain.Ticks(10000) {
			t.Fatalf("incorrect loaded AI defaults for %d: %+v", id, monster.AI)
		}
		if monster.Level > 60 || !monster.Kind.Hostile() {
			continue
		}
		want, listed := approved[id]
		if listed {
			matched++
			if monster.Name != want.Name {
				t.Fatalf("identity mismatch %d: %s", id, monster.Name)
			}
		}
		gotFlags := [4]bool{monster.Aggressive, monster.CallHelp, monster.Flee, monster.Pickup}
		wantFlags := [4]bool{want.Aggressive, want.CallHelp, want.Flee, want.Pickup}
		if gotFlags != wantFlags {
			t.Fatalf("monster %d flags %v, want %v", id, gotFlags, wantFlags)
		}
		for i, value := range gotFlags {
			if value {
				counts[i]++
			}
		}
	}
	if matched != len(approved) || counts != [4]int{68, 23, 15, 27} {
		t.Fatalf("loaded approved=%d/%d counts=%v", matched, len(approved), counts)
	}
	t.Logf("actual LoadMonsters: %d templates, %d approved identities, flags %v, %d read-only PostgreSQL queries", len(monsters), matched, counts, q.queries)
}
