package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Fast structural coverage in the normal Go suite. Actual PostgreSQL execution
// and fault injection live in tests/outdoor_equipment_postgres.mjs.
func TestOutdoorEquipmentMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/000000_init-106.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000106_20261024_outdoor_equipment_colours.sql")
	if err != nil {
		t.Fatal(err)
	}
	expectedUpgrade := "-- Existing databases: apply atomically; do not rerun fresh init.\nBEGIN;\n" + string(body) + "COMMIT;\n"
	if string(upgrade) != expectedUpgrade {
		t.Fatal("fresh and atomic upgrade migrations diverge")
	}
	if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(body) {
		t.Fatal("init must not own transaction")
	}
	joined, _, parts, err := loadInit("../../migrations")
	if err != nil || parts < 107 || !bytes.Contains(joined, body) {
		t.Fatalf("migration106 missing from contiguous initializer: parts=%d err=%v", parts, err)
	}
	sql := string(body)
	config := regexp.MustCompile(`'({"AffixWeights":\[[^']+})'::jsonb`).FindStringSubmatch(sql)
	if len(config) != 2 {
		t.Fatal("missing one exact outdoor equipment configuration")
	}
	var got struct {
		AffixWeights     []int64
		LegendaryBasePPB int64
	}
	if err := json.Unmarshal([]byte(config[1]), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.AffixWeights, []int64{20, 25, 20, 15, 10, 5}) || got.LegendaryBasePPB != 50000000 {
		t.Fatalf("outdoor baseline changed: %+v", got)
	}
	for _, want := range []string{
		"ALTER TABLE game_loot_rule_versions ADD COLUMN IF NOT EXISTS outdoor_equipment JSONB;",
		"SELECT version INTO active_version FROM game_loot_rule_active WHERE id=1 FOR UPDATE",
		"game_loot_socket_levels,game_loot_grades IN SHARE MODE",
		"a.atttypid='jsonb'::regtype AND NOT a.attnotnull AND NOT a.atthasdef",
		"tgenabled IN ('O','A') AND tgtype=31",
		"tgfoid='guard_published_endgame_rules()'::regprocedure",
		"WHERE version=2 AND published AND outdoor_equipment IS NULL AND outdoor_normal_equipment IS NULL",
		"ALTER TABLE game_loot_rule_versions ADD COLUMN IF NOT EXISTS outdoor_normal_equipment JSONB;",
		`'{"AffixWeights":[46,20,15,10,5,2],"LegendaryBasePPB":20000000}'::jsonb`,
		"requires complete loot2 dependencies",
		"'version',3,'published',false,'outdoor_equipment'",
		"loot3 is occupied by different configuration",
		"must preserve every loot2 rune, socket, and grade dependency",
		"UPDATE game_loot_rule_versions SET published=true WHERE version=3",
		"UPDATE game_loot_rule_active SET version=3 WHERE id=1 AND version=2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing migration guard: %s", want)
		}
	}
	children := []string{"game_loot_rune_pool", "game_loot_socket_levels", "game_loot_grades"}
	for _, table := range children {
		if !strings.Contains(sql, "jsonb_populate_record(NULL::"+table) ||
			strings.Count(sql, "to_jsonb(r)-'version' FROM "+table) != 4 {
			t.Fatalf("%s must copy and compare every dependency field", table)
		}
		if strings.Index(sql, "INSERT INTO "+table) > strings.Index(sql, "UPDATE game_loot_rule_versions SET published=true") {
			t.Fatal("published before copying dependencies")
		}
	}
	if strings.Count(sql, "ALTER TABLE") != 2 || strings.Count(sql, "UPDATE game_loot_rule_versions SET") != 1 ||
		strings.Count(sql, "UPDATE game_loot_rule_active SET") != 1 || strings.Contains(sql, "SET outdoor_equipment") ||
		strings.Contains(sql, "DISABLE TRIGGER") || strings.Contains(sql, "DROP TRIGGER") {
		t.Fatal("must not rewrite historical versions or remove immutable guards")
	}
	allowed := map[string]bool{"game_loot_rule_versions": true, "game_loot_rule_active": true,
		"game_loot_rune_pool": true, "game_loot_socket_levels": true, "game_loot_grades": true}
	for _, m := range regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_]+)`).FindAllStringSubmatch(sql, -1) {
		if !allowed[strings.ToLower(m[1])] {
			t.Fatalf("unexpected write outside loot rules: %s", m[0])
		}
	}
}
