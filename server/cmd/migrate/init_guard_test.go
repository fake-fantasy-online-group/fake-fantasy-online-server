package main

import (
	"os"
	"path/filepath"
	"testing"
)

const tempCreate = "CREATE TEMP TABLE _migration60_existing_endgame_definitions ON COMMIT DROP AS SELECT owner_kind,content_version,definition_id FROM game_endgame_definitions;\n"
const tempDrop = "DROP TABLE pg_temp._migration60_existing_endgame_definitions;\n"

func TestInitTemporaryCleanupGuard(t *testing.T) {
	tests := []struct {
		name, sql string
		ok        bool
	}{
		{"documented temporary cleanup", tempCreate + tempDrop, true},
		{"temporary cleanup without creation", tempDrop, false},
		{"permanent creation", "CREATE TABLE _migration60_existing_endgame_definitions AS SELECT 1;\n" + tempDrop, false},
		{"unqualified drop", tempCreate + "DROP TABLE _migration60_existing_endgame_definitions;\n", false},
		{"public drop", tempCreate + "DROP TABLE public._migration60_existing_endgame_definitions;\n", false},
		{"other temporary table", tempCreate + "DROP TABLE pg_temp.other;\n", false},
		{"cascade", tempCreate + "DROP TABLE pg_temp._migration60_existing_endgame_definitions CASCADE;\n", false},
		{"multiple targets", tempCreate + "DROP TABLE pg_temp._migration60_existing_endgame_definitions, accounts;\n", false},
		{"same line second statement", tempCreate + "DROP TABLE pg_temp._migration60_existing_endgame_definitions; DROP TABLE accounts;\n", false},
		{"next line destructive statement", tempCreate + tempDrop + "DROP TABLE accounts;\n", false},
		{"truncate", tempCreate + tempDrop + "TRUNCATE accounts;\n", false},
		{"drop schema", tempCreate + tempDrop + "DROP SCHEMA public;\n", false},
		{"drop database", tempCreate + tempDrop + "DROP DATABASE fantasy;\n", false},
		{"unguarded delete", tempCreate + tempDrop + "DELETE FROM accounts;\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "000000_init-0.sql"), []byte(tt.sql), 0600); err != nil {
				t.Fatal(err)
			}
			body, _, _, err := loadInit(dir)
			if (err == nil) != tt.ok {
				t.Fatalf("loadInit error=%v, want allowed=%v", err, tt.ok)
			}
			if tt.ok && string(body) != tt.sql {
				t.Fatal("guard changed executable SQL")
			}
		})
	}
}
