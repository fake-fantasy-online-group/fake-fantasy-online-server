package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestEquipmentSourceLevelMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/000000_init-108.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000108_20261024_equipment_source_level.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(upgrade) != "-- Existing databases: apply atomically; do not rerun fresh init.\nBEGIN;\n"+string(body)+"COMMIT;\n" {
		t.Fatal("fresh and upgrade SQL diverge")
	}
	joined, _, parts, err := loadInit("../../migrations")
	if err != nil || parts < 109 || !bytes.Contains(joined, body) {
		t.Fatal("source-level migration missing from contiguous initializer", parts, err)
	}
	for _, want := range []string{"source_level INTEGER NOT NULL DEFAULT 0", "CHECK (source_level >= 0)", "convalidated", "a.atttypid='integer'::regtype"} {
		if !strings.Contains(string(body), want) {
			t.Fatal("missing safety guard", want)
		}
	}
	if strings.Contains(string(body), "UPDATE equipment_endgame_state") {
		t.Fatal("migration must not backfill/infer old provenance")
	}
}
