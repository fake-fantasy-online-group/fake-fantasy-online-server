package main

import (
	"bytes"
	"os"
	"regexp"
	"testing"
)

func TestUIReview124IncludedParityAndNarrowWrites(t *testing.T) {
	init, err := os.ReadFile("../../migrations/000000_init-124.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000124_20261028_ui_review_metadata.sql")
	if err != nil || string(upgrade) != "BEGIN;\n"+string(init)+"COMMIT;\n" {
		t.Fatal("migration divergence", err)
	}
	all, _, parts, err := loadInit("../../migrations")
	if err != nil || parts < 125 || !bytes.Contains(all, init) {
		t.Fatal("124 not included", parts, err)
	}
	if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(init) {
		t.Fatal("nested transaction")
	}
	writes := regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_.]+)`).FindAllSubmatch(init, -1)
	if len(writes) != 3 {
		t.Fatalf("unexpected write count %d", len(writes))
	}
	for _, w := range writes {
		if string(w[1]) != "gamedata.ov_desc" && string(w[1]) != "game_shops" {
			t.Fatalf("unexpected write %s", w[1])
		}
	}
}
