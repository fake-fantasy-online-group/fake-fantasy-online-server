// Package monsterbehavior reads the original native import, not a hand-written
// behavioral expectation, for focused migration and scene regressions.
package monsterbehavior

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
)

type Row struct {
	ID, Level, HP, MoveSpeed, AttackSpeed int32
	Name, Kind                            string
	Aggressive, CallHelp                  bool
}

func Read(t *testing.T, migrationDir string) []Row {
	t.Helper()
	read := func(name string) string {
		b, e := os.ReadFile(filepath.Join(migrationDir, name))
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	names := map[int32]string{}
	number := func(s string) int32 {
		n, e := strconv.ParseInt(s, 10, 32)
		if e != nil {
			t.Fatal(e)
		}
		return int32(n)
	}
	for _, m := range regexp.MustCompile(`(?m)^  \((\d+), \d+, '([^']*)', decode`).FindAllStringSubmatch(read("000000_init-2.sql"), -1) {
		names[number(m[1])] = m[2]
	}
	re := regexp.MustCompile(`(?m)^  \((\d+),(\d+),(\d+),(\d+),(\d+),(\d+),(\d+),'([^']*)','([^']*)',(\d+),(\d+),(\d+),(\d+),(\d+),(TRUE|FALSE),(TRUE|FALSE)\)`)
	var rows []Row
	for _, m := range re.FindAllStringSubmatch(read("000000_init-32.sql"), -1) {
		r := Row{ID: number(m[1]), Level: number(m[2]), HP: number(m[3]), MoveSpeed: number(m[5]), AttackSpeed: number(m[6]), Name: names[number(m[1])], Kind: m[8], Aggressive: m[15] == "TRUE", CallHelp: m[16] == "TRUE"}
		if r.Name == "" {
			t.Fatalf("native name missing: %d", r.ID)
		}
		rows = append(rows, r)
	}
	if len(rows) < 2000 {
		t.Fatalf("incomplete native import: %d", len(rows))
	}
	return rows
}
func Scoped(r Row) bool {
	return r.Level >= 1 && r.Level <= 60 && r.HP > 0 && (r.Kind == "普通" || r.Kind == "精英" || r.Kind == "BOSS")
}
func SQL(t *testing.T, dir, name string) string {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(dir, name))
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

// Seed creates only temporary tables from exact native identity/behavior rows.
func Seed(t *testing.T, ctx context.Context, tx pgx.Tx, rows []Row) {
	t.Helper()
	if _, e := tx.Exec(ctx, `CREATE TEMP TABLE game_monsters(id INTEGER PRIMARY KEY,name TEXT,level INTEGER,hp INTEGER,kind TEXT,aggressive BOOLEAN,call_help BOOLEAN,can_attack BOOLEAN DEFAULT TRUE,move_speed INTEGER,atk_speed INTEGER);
 CREATE TEMP TABLE game_monster_ai_templates(id TEXT PRIMARY KEY);
 INSERT INTO game_monster_ai_templates VALUES('native');`); e != nil {
		t.Fatal(e)
	}
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`INSERT INTO game_monsters(id,name,level,hp,kind,aggressive,call_help,move_speed,atk_speed) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.ID, r.Name, r.Level, r.HP, r.Kind, r.Aggressive, r.CallHelp, r.MoveSpeed, r.AttackSpeed)
	}
	if e := tx.SendBatch(ctx, batch).Close(); e != nil {
		t.Fatal(e)
	}
}
