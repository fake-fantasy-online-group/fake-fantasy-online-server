package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestWingScroll119InitUpgradeParityAndNoInventoryWrites(t *testing.T) {
	init, err := os.ReadFile("../../migrations/000000_init-119.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000119_20261027_wing_power_scroll.sql")
	if err != nil {
		t.Fatal(err)
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(string(upgrade), "BEGIN;\n", ""), "COMMIT;\n", "")
	if normalized != string(init) {
		t.Fatal("init and upgrade diverged")
	}
	if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(init) {
		t.Fatal("nested transaction")
	}
	if !bytes.Contains(init, []byte("requires power codex migration 118")) || !bytes.Contains(init, []byte("use_action='power_scroll'")) {
		t.Fatal("missing codex dependency guard")
	}
	if regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+(?:characters|character_items|item_instances|equipment_wing_power_state|equipment_endgame_state|game_wing_progression_rules)\b`).Match(init) {
		t.Fatal("must not rewrite owned items/progression")
	}
	body, _, parts, err := loadInit("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	if parts < 120 || !bytes.Contains(body, init) {
		t.Fatal("119 omitted from initializer")
	}
}
