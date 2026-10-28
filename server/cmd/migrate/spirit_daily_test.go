package main

import (
	"bytes"
	"os"
	"testing"
)

func TestSpiritDaily123MigrationParity(t *testing.T) {
	init, err := os.ReadFile("../../migrations/000000_init-123.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000123_20261028_spirit_daily.sql")
	if err != nil || string(upgrade) != "BEGIN;\n"+string(init)+"COMMIT;\n" {
		t.Fatal("migration divergence", err)
	}
	all, _, _, err := loadInit("../../migrations")
	if err != nil || !bytes.Contains(all, init) {
		t.Fatal("not included", err)
	}
}
