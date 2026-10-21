package data

import (
	"context"
	fixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/monsterbehavior"
	"github.com/jackc/pgx/v5"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const nativeMigrationDir = "../../migrations"

func TestNativeAggressionRestorationExactScope(t *testing.T) {
	rows := fixture.Read(t, nativeMigrationDir)
	approved := map[int32]bool{}
	for _, r := range readApprovedMonsterBehaviors(t) {
		approved[r.ID] = r.Aggressive
	}
	expected := map[int32]fixture.Row{}
	for _, r := range rows {
		if fixture.Scoped(r) && r.Aggressive && !approved[r.ID] {
			expected[r.ID] = r
		}
	}
	if len(expected) != 471 {
		t.Fatalf("native restoration scope changed: %d", len(expected))
	}
	seed := fixture.SQL(t, nativeMigrationDir, "000000_init-97.sql")
	upgrade := fixture.SQL(t, nativeMigrationDir, "upgrades/000097_20261021_restore_native_aggression.sql")
	if !strings.Contains(upgrade, "BEGIN;\n"+seed+"COMMIT;\n") {
		t.Fatal("upgrade differs from seed")
	}
	seen := map[int32]bool{}
	for _, m := range regexp.MustCompile(`\((\d+),'([^']*)',(\d+),'([^']*)'\)`).FindAllStringSubmatch(seed, -1) {
		id, _ := strconv.Atoi(m[1])
		lv, _ := strconv.Atoi(m[3])
		r, ok := expected[int32(id)]
		if !ok || seen[r.ID] || r.Name != m[2] || r.Level != int32(lv) || r.Kind != m[4] {
			t.Fatalf("not an exact native restoration: %v", m)
		}
		seen[r.ID] = true
	}
	if len(seen) != len(expected) {
		t.Fatal("incomplete restoration")
	}
	for _, bad := range []string{"SET call_help", "SET flee", "SET pickup", "SET can_attack", "DELETE ", "TRUNCATE ", "schema_migrations"} {
		if strings.Contains(seed, bad) {
			t.Fatal(bad)
		}
	}
}
func TestNativeAggressionRestorationActualPostgres(t *testing.T) {
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
	rows := fixture.Read(t, nativeMigrationDir)
	fixture.Seed(t, ctx, tx, rows)
	exec := func(s string) {
		t.Helper()
		if _, e := tx.Exec(ctx, s); e != nil {
			t.Fatal(e)
		}
	}
	exec(fixture.SQL(t, nativeMigrationDir, "000000_init-88.sql"))
	snapshot := func(withAggression bool) string {
		t.Helper()
		var v string
		expr := "to_jsonb(m)-'aggressive'"
		if withAggression {
			expr = "to_jsonb(m)"
		}
		if e := tx.QueryRow(ctx, "SELECT jsonb_agg("+expr+" ORDER BY id)::text FROM game_monsters m").Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	unchanged := snapshot(false)
	seed := fixture.SQL(t, nativeMigrationDir, "000000_init-97.sql")
	exec(seed)
	if snapshot(false) != unchanged {
		t.Fatal("changed fields other than aggressive")
	}
	approved := map[int32]bool{}
	for _, r := range readApprovedMonsterBehaviors(t) {
		approved[r.ID] = r.Aggressive
	}
	for _, r := range rows {
		var active bool
		if e := tx.QueryRow(ctx, "SELECT aggressive FROM game_monsters WHERE id=$1", r.ID).Scan(&active); e != nil {
			t.Fatal(e)
		}
		want := r.Aggressive
		if fixture.Scoped(r) {
			want = want || approved[r.ID]
		}
		if active != want {
			t.Fatalf("id %d: got %v want %v", r.ID, active, want)
		}
	}
	once := snapshot(true)
	exec(seed)
	if snapshot(true) != once {
		t.Fatal("not idempotent")
	}
	// A late identity conflict must roll back earlier updates in the same DO.
	for _, change := range []string{"name='changed'", "level=61", "kind='场景物件'", "hp=0"} {
		exec("SAVEPOINT probe")
		exec("UPDATE game_monsters SET aggressive=FALSE WHERE id=3049")
		exec("UPDATE game_monsters SET " + change + " WHERE id=(SELECT max(id) FROM game_monsters WHERE aggressive AND level BETWEEN 1 AND 60 AND kind IN ('普通','精英','BOSS'))")
		// Explicitly corrupt the final restored identity, independent of approved additions.
		last := regexp.MustCompile(`\((\d+),'[^']*',\d+,'[^']*'\)`).FindAllStringSubmatch(seed, -1)
		id := last[len(last)-1][1]
		exec("UPDATE game_monsters SET " + change + " WHERE id=" + id)
		before := snapshot(true)
		nested, e := tx.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = nested.Exec(ctx, seed); e == nil {
			t.Fatal("identity conflict accepted")
		}
		if e = nested.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
		if snapshot(true) != before {
			t.Fatal("identity failure partially applied")
		}
		exec("ROLLBACK TO SAVEPOINT probe")
	}
}
