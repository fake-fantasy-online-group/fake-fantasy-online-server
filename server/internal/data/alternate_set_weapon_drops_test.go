package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

func alternateSetDropMigration(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../migrations/000000_init-96.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAlternateSetWeaponUpgradeMatchesInit(t *testing.T) {
	seed := alternateSetDropMigration(t)
	b, err := os.ReadFile("../../migrations/upgrades/000096_20261021_alternate_set_weapon_drops.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "BEGIN;\n"+seed+"COMMIT;\n") {
		t.Fatal("incremental upgrade must wrap the identical seed in one transaction")
	}
	for _, bad := range []string{"UPDATE ", "DELETE ", "TRUNCATE ", "INSERT INTO gamedata.", "schema_migrations"} {
		if strings.Contains(seed, bad) {
			t.Fatalf("must only append explicitly scoped drop rows, found %q", bad)
		}
	}
}

// Use only an isolated empty database; the fixture uses exact native seed rows.
// Everything runs in a rollback-only transaction, including rejection probes.
func TestAlternateSetWeaponDropsActualPostgres(t *testing.T) {
	dsn := os.Getenv("FANTASY_ALTERNATE_SET_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated native PostgreSQL DSN required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func(table, where string) string {
		t.Helper()
		var s string
		if err := tx.QueryRow(ctx, "SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM "+table+" t WHERE "+where).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	fixture, err := os.ReadFile("testdata/alternate-set-native-drops.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(fixture))
	unchanged := map[string]string{}
	predicates := map[string]string{
		"game_monster_drops":             "NOT (monster_id=3060 AND item_id=8051 OR monster_id=3062 AND item_id=8451)",
		"game_boss_fixed_equipment_pool": "NOT (map_id=20600 AND (monster_id=3060 AND item_id=8051 OR monster_id=3062 AND item_id=8451))",
		"gamedata.ov_arm":                "index IN (8005,8051,8401,8451)",
	}
	for table, where := range predicates {
		unchanged[table] = snapshot(table, where)
	}
	seed := alternateSetDropMigration(t)
	exec(seed)
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM game_monster_drops WHERE (monster_id,item_id) IN ((3060,8051),(3062,8451))`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("expected exactly four named rows: %d %v", n, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM game_boss_fixed_equipment_pool WHERE map_id=20600 AND (monster_id,item_id) IN ((3060,8051),(3062,8451)) AND minimum=1 AND weight=2 AND NOT roll_affixes`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("expected two unchanged-minimum fixed entries: %d %v", n, err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (VALUES(3060,8005,8051),(3062,8401,8451)) v(mid,src,dst)
 WHERE (SELECT jsonb_agg(jsonb_build_array(rate,item_src,kind_name,lv_min,lv_max) ORDER BY rate) FROM game_monster_drops WHERE monster_id=mid AND item_id=src)
 IS NOT DISTINCT FROM (SELECT jsonb_agg(jsonb_build_array(rate,item_src,kind_name,lv_min,lv_max) ORDER BY rate) FROM game_monster_drops WHERE monster_id=mid AND item_id=dst)
 AND EXISTS(SELECT FROM game_boss_fixed_equipment_pool a JOIN game_boss_fixed_equipment_pool b ON a.map_id=b.map_id AND a.monster_id=b.monster_id
 WHERE a.map_id=20600 AND a.monster_id=mid AND a.item_id=src AND b.item_id=dst AND a.rate_pct=b.rate_pct AND a.weight=b.weight AND a.minimum=b.minimum AND a.roll_affixes=b.roll_affixes)`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("new weapons must inherit source rules: %d %v", n, err)
	}
	for table, where := range predicates {
		if got := snapshot(table, where); got != unchanged[table] {
			t.Fatalf("existing rows changed in %s", table)
		}
	}
	// Build only the item identity/no-type-drop fields consumed by these loaders
	// from the native fixture, without depending on unrelated game systems.
	items := ItemDefs{}
	rows, err := tx.Query(ctx, "SELECT index,name,no_type_drop FROM gamedata.ov_arm ORDER BY index")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id domain.ItemID
		var name string
		var noType int
		if err := rows.Scan(&id, &name, &noType); err != nil {
			t.Fatal(err)
		}
		items[id] = domain.ItemDef{ID: id, Name: name, Equip: &domain.EquipDef{NoTypeDrop: noType != 0}}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	drops, _, err := LoadDrops(ctx, tx, items)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := LoadBossFixedDropTable(ctx, tx, items)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][3]int32{{3060, 8005, 8051}, {3062, 8401, 8451}} {
		rule := fixed[domain.BossFixedDropKey{MapID: 20600, Monster: domain.MonsterID(pair[0])}]
		if rule.Minimum != 1 {
			t.Fatal("loaded guarantee increased")
		}
		for _, id := range pair[1:] {
			named, choices := 0, 0
			for _, d := range drops[domain.MonsterID(pair[0])] {
				if d.Item == domain.ItemID(id) && len(d.Choices) == 0 {
					named++
				}
			}
			for _, d := range rule.Pool {
				if d.Item == domain.ItemID(id) {
					choices++
				}
			}
			if named != 2 || choices != 1 {
				t.Fatalf("runtime loader missing main/alternate %d: %d/%d", id, named, choices)
			}
		}
	}
	afterDrops := snapshot("game_monster_drops", "TRUE")
	afterFixed := snapshot("game_boss_fixed_equipment_pool", "TRUE")
	exec(seed)
	if snapshot("game_monster_drops", "TRUE") != afterDrops || snapshot("game_boss_fixed_equipment_pool", "TRUE") != afterFixed {
		t.Fatal("replay changed drop configuration")
	}
	// A conflict in the SECOND weapon must not leave partial first-weapon writes.
	for _, mutation := range []string{
		"UPDATE gamedata.ov_arm SET stabber=0 WHERE index=8451",
		"UPDATE game_monster_drops SET rate=50 WHERE monster_id=3062 AND item_id=8451",
		"UPDATE game_boss_fixed_equipment_pool SET minimum=2 WHERE map_id=20600 AND monster_id=3062 AND item_id=8451",
		"UPDATE game_boss_fixed_equipment_pool SET weight=1 WHERE map_id=20600 AND monster_id=3062 AND item_id=8401",
	} {
		exec("SAVEPOINT probe")
		exec(mutation)
		exec("DELETE FROM game_monster_drops WHERE monster_id=3060 AND item_id=8051")
		exec("DELETE FROM game_boss_fixed_equipment_pool WHERE map_id=20600 AND monster_id=3060 AND item_id=8051")
		beforeAttemptDrops := snapshot("game_monster_drops", "TRUE")
		beforeAttemptFixed := snapshot("game_boss_fixed_equipment_pool", "TRUE")
		exec("SAVEPOINT attempt")
		if _, err := tx.Exec(ctx, seed); err == nil {
			t.Fatalf("must reject conflicting data: %s", mutation)
		}
		exec("ROLLBACK TO SAVEPOINT attempt")
		if snapshot("game_monster_drops", "TRUE") != beforeAttemptDrops || snapshot("game_boss_fixed_equipment_pool", "TRUE") != beforeAttemptFixed {
			t.Fatal("second-weapon conflict left partial first-weapon writes")
		}
		exec("ROLLBACK TO SAVEPOINT probe")
		exec("RELEASE SAVEPOINT probe")
		if snapshot("game_monster_drops", "TRUE") != afterDrops || snapshot("game_boss_fixed_equipment_pool", "TRUE") != afterFixed {
			t.Fatal("failed migration altered baseline")
		}
	}
	t.Log("real native PostgreSQL: both alternates, exact source parity, original rows, unchanged minima, idempotence, and four fail-closed conflict cases verified")
}
