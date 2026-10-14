package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestEliteActivationMigrationReviewExactConfigAndMonotonicSelector(t *testing.T) {
	b, err := os.ReadFile("../../migrations/000000_init-81.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(b)
	parts := strings.Split(sql, "$config$")
	if len(parts) != 3 {
		t.Fatal("expected exactly one activation config literal")
	}
	var got, want map[string]any
	if err := json.Unmarshal([]byte(parts[1]), &got); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile("../../docs/endgame/elite-skills-v6.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("activation and authored runtime configurations differ")
	}
	var lines []string
	for _, line := range strings.Split(sql, "\n") {
		lines = append(lines, strings.SplitN(line, "--", 2)[0])
	}
	body := strings.Join(lines, "\n")
	for _, required := range []string{
		"SELECT version INTO active_version FROM game_rift_rule_active WHERE id=1 FOR UPDATE",
		"IF active_version IS NULL OR active_version < 5 THEN",
		"IF active_version > 6 THEN",
		"RETURN;",
		"IF active_version=5 THEN",
		"UPDATE game_rift_rule_active SET version=6 WHERE id=1 AND version=5",
		"tgfoid='guard_published_endgame_rules()'::regprocedure",
		"tgenabled IN ('O','A')",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing activation guard: %s", required)
		}
	}
	if strings.Index(body, "IF active_version > 6 THEN") > strings.Index(body, "LOCK TABLE") {
		t.Fatal("later-version replay must skip obsolete dependencies")
	}
	if strings.Count(body, "UPDATE game_rift_rule_active SET") != 1 {
		t.Fatal("must have one selector write")
	}
	for _, forbidden := range []string{"INSERT INTO", "DELETE FROM", "ALTER TABLE", "UPDATE game_loot", "UPDATE game_monster", "UPDATE game_rift_rule_versions", "UPDATE game_rift_class", "UPDATE game_rift_ability"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unexpected persistent content change: %s", forbidden)
		}
	}
}
