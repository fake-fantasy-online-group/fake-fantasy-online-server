package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestEliteMigrationReviewConfigPublicationAndInactivePointer(t *testing.T) {
	b, err := os.ReadFile("../../migrations/000000_init-80.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(b)
	parts := strings.Split(sql, "$config$")
	if len(parts) != 3 {
		t.Fatal("expected exactly one migration config literal")
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
		t.Fatal("migration and authored runtime configs differ")
	}
	var executable []string
	for _, line := range strings.Split(sql, "\n") {
		executable = append(executable, strings.SplitN(line, "--", 2)[0])
	}
	body := strings.Join(executable, "\n")
	if strings.Contains(strings.ToLower(body), "game_rift_rule_active") {
		t.Fatal("migration touches live active pointer")
	}
	parent := strings.Index(body, "INSERT INTO game_rift_rule_versions")
	classes := strings.Index(body, "INSERT INTO game_rift_class_rules")
	abilities := strings.Index(body, "INSERT INTO game_rift_ability_rules")
	publish := strings.Index(body, "UPDATE game_rift_rule_versions SET published=true WHERE version=6")
	if parent < 0 || classes <= parent || abilities <= classes || publish <= abilities {
		t.Fatal("published-rule trigger ordering violated")
	}
	if !strings.Contains(body, "'version',6,'published',false") {
		t.Fatal("new parent must be unpublished while children are copied")
	}
	if !strings.Contains(body, "IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=6)") || !strings.Contains(body, "IS DISTINCT FROM") {
		t.Fatal("missing duplicate-version mismatch guard")
	}
}
