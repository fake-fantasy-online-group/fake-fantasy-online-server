package publiccatalog

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The caller must create a disposable database with testdata/synthetic.sql.
// This test only reads it. Never set this to an existing user/production DB.
func TestPostgresSyntheticFixture(t *testing.T) {
	dsn := os.Getenv("FANTASY_PUBLIC_CATALOG_TEST_DSN")
	if dsn == "" {
		t.Skip("set FANTASY_PUBLIC_CATALOG_TEST_DSN to a disposable synthetic-fixture database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	config.RuntimeParams["search_path"] = "pg_catalog"
	config.RuntimeParams["default_transaction_read_only"] = "on"
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	artifact, err := Export(ctx, conn, Options{SourceRevision: "synthetic-fixture-only"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Manifest.Counts != (Counts{Item: 2, Equipment: 1, Total: 3}) || time.Since(artifact.Manifest.GeneratedAt) > time.Minute {
		t.Fatalf("unexpected real fixture metadata: %+v", artifact.Manifest)
	}
	if strings.Contains(string(artifact.JSON), "PRIVATE_") || strings.Contains(string(artifact.JSON), "Duplicate") {
		t.Fatal("private sentinel or duplicate row escaped explicit public projection")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(artifact.JSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	item, equipment, card := snapshot.Items[0], snapshot.Items[1], snapshot.Items[2]
	if item.Name != "Synthetic potion" || item.Price != 20 || item.SellPrice != 10 || item.IconID != 11 || !item.Stackable {
		t.Fatalf("item source parity failed: %+v", item)
	}
	if equipment.Name != "Synthetic sword" || equipment.Price != 100 || equipment.SellPrice != 50 || equipment.Durability != 60 || equipment.Stats.STR != 12 || equipment.Stats.LUK != -2 || equipment.Sex != 1 || strings.Join(equipment.Professions, "、") != "战士、剑客" {
		t.Fatalf("equipment normalized/source parity failed: %+v", equipment)
	}
	if card.Name != "Synthetic card" || card.Stackable {
		t.Fatal("card runtime stackability parity failed")
	}
	if err := Write(t.TempDir(), artifact); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM public.accounts").Scan(&count); err == nil {
		t.Fatal("fixture reader unexpectedly has access to private account table")
	}
	if artifact, err := Export(ctx, conn, Options{MaxRows: 1}); err == nil || len(artifact.JSON) != 0 {
		t.Fatal("large fixture export was silently truncated")
	}
}
