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

func TestRiftCosmeticHatMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/000000_init-105.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000105_20261024_rift_cosmetic_hats.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(upgrade, body) || !bytes.Contains(upgrade, []byte("BEGIN;")) || !bytes.HasSuffix(bytes.TrimSpace(upgrade), []byte("COMMIT;")) {
		t.Fatal("fresh and atomic upgrade migrations diverge")
	}
	if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(body) {
		t.Fatal("init must not own transaction")
	}
	parts := strings.Split(string(body), "$excluded$")
	if len(parts) != 3 {
		t.Fatal("expected one reviewed exclusion manifest")
	}
	var exclusions []struct {
		ID   int
		Name string
		Sex  int
	}
	if err := json.Unmarshal([]byte(parts[1]), &exclusions); err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, x := range exclusions {
		ids = append(ids, x.ID)
		if x.Name == "" || x.Sex < 0 || x.Sex > 2 {
			t.Fatal("invalid identity", x)
		}
	}
	if !reflect.DeepEqual(ids, []int{2037, 2038, 2039, 2040, 2041, 2042, 2043, 2044, 2060, 2061, 2062, 2063}) {
		t.Fatal("exclusion scope changed", ids)
	}
	sql := string(body)
	for _, want := range []string{
		"ALTER TABLE game_rift_rule_versions ADD COLUMN IF NOT EXISTS equipment_slot_fallbacks JSONB;",
		"SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE",
		"game_equipment,gamedata.ov_arm,game_equip_extra IN SHARE MODE",
		"WHERE version=10 AND published", "tgenabled IN ('O','A')",
		"tgfoid='guard_published_endgame_rules()'::regprocedure",
		"ROW(x.name,2,1,1,5,x.sex,9,0)", "rift10 is missing a reviewed cosmetic hat",
		"jsonb_agg(i ORDER BY item_ordinal)", "ORDER BY pool_ordinal",
		"rift cosmetic hat exclusion would empty an equipment tier",
		"'version',11,'published',false,'equipment_pools',pools",
		"'equipment_slot_fallbacks',fallbacks",
		"WITH fallback_candidates AS (",
		"SELECT e.id,e.slot FROM game_equipment e JOIN gamedata.ov_arm a ON a.index=e.id",
		"e.slot=1 AND a.query_type=15 AND a.position_=101",
		"e.slot=10 AND a.query_type=13 AND a.position_=110",
		"a.no_type_drop=0 AND a.level>=0",
		"e.level_req BETWEEN 0 AND 60 AND a.level_need BETWEEN 0 AND 60",
		"NOT EXISTS(SELECT FROM game_equip_extra x WHERE x.arm_id=e.id)",
		"SELECT jsonb_agg(id ORDER BY id),count(DISTINCT slot) INTO fallbacks,fallback_slots FROM fallback_candidates",
		"IF fallbacks IS NULL OR fallback_slots<>2 THEN",
		"rift missing-slot fallback requires normal face and backpack bases",
		"rift11 is occupied by different configuration", "EXCEPT ALL",
		"UPDATE game_rift_rule_versions SET published=true WHERE version=11",
		"UPDATE game_rift_rule_active SET version=11 WHERE id=1 AND version=10",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing migration guard: %s", want)
		}
	}
	for _, gate := range []string{"newbie", "warrior", "swordman", "stabber", "druggist", "magician"} {
		if !strings.Contains(sql, "COALESCE(a."+gate+",0)=0") {
			t.Fatalf("missing normal-class fallback eligibility gate: %s", gate)
		}
	}
	for _, gate := range []string{"sr_warrior", "sr_swordman", "sr_stabber", "sr_druggist", "sr_magician"} {
		if !strings.Contains(sql, "COALESCE(a."+gate+",0)<>0") {
			t.Fatalf("missing ascension-only fallback eligibility gate: %s", gate)
		}
	}
	// The new column must remain NULL on old published versions, and be frozen
	// only in the newly inserted v11. A default/backfill would change old runs.
	if strings.Count(sql, "ALTER TABLE") != 1 || strings.Contains(sql, "SET equipment_slot_fallbacks") {
		t.Fatal("fallback schema change must not backfill old versions")
	}
	if strings.Count(sql, "UPDATE game_rift_rule_versions SET") != 1 {
		t.Fatal("only publication of the new rule version may mutate a rule row")
	}
	if strings.Count(sql, "UPDATE game_rift_rule_active SET") != 1 {
		t.Fatal("unexpected selector mutation")
	}
	// The only mutable row from previous versions is the active selector. No
	// catalog/player instance/native drop table, old rule or saved run is written.
	allowed := map[string]bool{"game_rift_rule_versions": true, "game_rift_class_rules": true, "game_rift_ability_rules": true, "game_rift_rule_active": true}
	for _, m := range regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_]+)`).FindAllStringSubmatch(sql, -1) {
		if !allowed[strings.ToLower(m[1])] {
			t.Fatalf("unexpected write: %s", m[0])
		}
	}
	if strings.Index(sql, "INSERT INTO game_rift_ability_rules") > strings.Index(sql, "UPDATE game_rift_rule_versions SET published=true") {
		t.Fatal("published before copying dependencies")
	}
}
