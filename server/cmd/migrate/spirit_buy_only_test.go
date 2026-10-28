package main

import (
	"bytes"
	"os"
	"regexp"
	"testing"
)

func TestSpiritBuyOnly125IncludedParityAndNarrowWrites(t *testing.T) {
	init, err := os.ReadFile("../../migrations/000000_init-125.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000125_20261028_spirit_buy_only.sql")
	if err != nil || string(upgrade) != "BEGIN;\n"+string(init)+"COMMIT;\n" {
		t.Fatal("migration divergence", err)
	}
	all, _, parts, err := loadInit("../../migrations")
	if err != nil || parts < 126 || !bytes.Contains(all, init) {
		t.Fatal("125 not included", parts, err)
	}
	if regexp.MustCompile(`(?m)^\s*(BEGIN|COMMIT|ROLLBACK);`).Match(init) {
		t.Fatal("nested transaction")
	}
	writes := regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_.]+)`).FindAllSubmatch(init, -1)
	if len(writes) != 2 {
		t.Fatalf("unexpected write count %d", len(writes))
	}
	for _, w := range writes {
		if string(w[1]) != "spirit_buy_only_stage" && string(w[1]) != "game_shops" {
			t.Fatalf("unexpected write %s", w[1])
		}
	}
}
