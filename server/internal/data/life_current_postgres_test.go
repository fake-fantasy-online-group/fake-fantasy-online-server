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
func TestLifeCurrentPostgresLoaders(t *testing.T) {
	path := os.Getenv("FANTASY_LIFE_PG_DATA")
	if path == "" {
		t.Skip("explicit idle disposable PostgreSQL 117 copy required")
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
	if loot.Active().Version != 6 || !loot.Active().LegendaryPowerRolls || !loot.Active().RetiredCubeMaterials || rift.Active().Version != 18 || rift.Active().LootVersion != 6 {
		t.Fatalf("unexpected active generation: loot=%+v rift=%+v", loot.Active(), rift.Active())
	}
	for version := int32(1); version <= 4; version++ {
		old, ok := loot.Version(version)
		if !ok || old.LegendaryPowerRolls {
			t.Fatalf("historical loot %d not frozen", version)
		}
	}
	for version := int32(1); version <= 5; version++ {
		old, ok := loot.Version(version)
		if !ok || old.RetiredCubeMaterials {
			t.Fatalf("historical loot material gate changed: %d", version)
		}
	}
	craft, err := LoadCraftRecipes(ctx, q, items)
	if err != nil {
		t.Fatal(err)
	}
	for id := domain.FirstAstralRune; id < domain.FirstAstralRune+28; id++ {
		if len(craft.ProductRecipes(0, id)) != 10 {
			t.Fatalf("missing furnace routes %d", id)
		}
	}
	for _, id := range []domain.ItemID{4501, 4478} {
		if len(craft.ProductRecipes(0, id)) < 10 {
			t.Fatalf("missing ore routes %d", id)
		}
	}
	low := craft.ProductRecipes(0, domain.FirstAstralRune)
	high := craft.ProductRecipes(0, domain.FirstAstralRune+6)
	if low[0].Materials[0].Qty != 50 || high[0].Materials[0].Qty != 350 || high[9].Materials[0].Qty != 88 {
		t.Fatal("final x5 furnace costs mismatch")
	}
	t.Logf("real PostgreSQL 117 current loaders PASS: %d items, %d cube recipes, %d read-only SQL queries; Loot 6/Rift 18 and old Loot 1..4 roll gates / 1..5 material gates validated", len(items), len(cube.Recipes()), q.queries)
}
