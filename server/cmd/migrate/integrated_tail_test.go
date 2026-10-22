package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestIntegratedInitSequenceAndUpgradeParity(t *testing.T) {
	dir := "../../migrations"
	body, _, parts, err := loadInit(dir)
	if err != nil {
		t.Fatal(err)
	}
	if parts < 102 {
		t.Fatalf("need all contiguous fragments 0..101, got %d", parts)
	}
	tails := []struct {
		index int
		name  string
	}{
		{99, "rift_full_coverage"}, {100, "quest_special_weapons"}, {101, "pet_merchant_supplies"},
	}
	last := -1
	for _, tail := range tails {
		init, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("000000_init-%d.sql", tail.index)))
		if err != nil {
			t.Fatal(err)
		}
		upgrade, err := os.ReadFile(filepath.Join(dir, "upgrades", fmt.Sprintf("%06d_20261022_%s.sql", tail.index, tail.name)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(upgrade, init) {
			t.Fatalf("%d fresh init and upgrade diverge", tail.index)
		}
		if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(init) {
			t.Fatalf("%d breaks initializer transaction", tail.index)
		}
		if !bytes.Contains(upgrade, []byte("BEGIN;")) || !bytes.HasSuffix(bytes.TrimSpace(upgrade), []byte("COMMIT;")) {
			t.Fatalf("%d upgrade must own transaction", tail.index)
		}
		at := bytes.Index(body, init)
		if at <= last {
			t.Fatalf("tail %d not in dependency order", tail.index)
		}
		last = at
		if regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+(?:characters|character_items|player_items|inventories)\b`).Match(init) {
			t.Fatalf("%d changes existing player data", tail.index)
		}
	}
	density, err := os.ReadFile(filepath.Join(dir, "000000_init-99.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+game_rift_rule_active\b`).Match(density) {
		t.Fatal("density migration must not activate rule 10")
	}
	if !bytes.Contains(density, []byte(`"floor_layout_version":6`)) {
		t.Fatal("missing full-coverage rule definition")
	}
}
