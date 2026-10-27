package data

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Explicitly supplied idle disposable copy only: no sockets, no production DSN,
// and every loader query runs inside a READ ONLY transaction.
func TestPowerBookCurrentPostgresLoaders(t *testing.T) {
	path := os.Getenv("FANTASY_POWER_BOOK_PG_DATA")
	if path == "" {
		t.Skip("explicit idle disposable PostgreSQL 115 copy required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); !os.IsNotExist(err) {
		t.Fatal("cluster is not demonstrably idle", err)
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "fresh_test"}
	ctx := context.Background()
	items, err := LoadItems(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	skills, _, err := LoadSkills(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadEndgameCatalog(ctx, q, items, skills.ProgressionCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if err = domain.ValidateEquipmentRuntimeV5(skills, catalog); err != nil {
		t.Fatal(err)
	}
	cube, err := LoadCubeRules(ctx, q, items)
	if err != nil {
		t.Fatal(err)
	}
	loot, err := LoadLootRules(ctx, q, items)
	if err != nil {
		t.Fatal(err)
	}
	rift, err := LoadRiftRules(ctx, q, loot, items)
	if err != nil {
		t.Fatal(err)
	}
	if loot.Active().Version != 5 || !loot.Active().LegendaryPowerRolls || rift.Active().Version != 17 || rift.Active().LootVersion != 5 {
		t.Fatalf("unexpected active generation: loot=%+v rift=%+v", loot.Active(), rift.Active())
	}
	for version := int32(1); version <= 4; version++ {
		old, ok := loot.Version(version)
		if !ok || old.LegendaryPowerRolls {
			t.Fatalf("historical loot %d not frozen", version)
		}
	}
	t.Logf("real PostgreSQL 115 current loaders PASS: %d items, %d cube recipes, %d read-only SQL queries; Loot 5/Rift 17 and old Loot 1..4 roll gates validated", len(items), len(cube.Recipes()), q.queries)
}
