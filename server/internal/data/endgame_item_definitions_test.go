package data

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"github.com/jackc/pgx/v5"
)

func endgameDefinitionFixture(t *testing.T) ItemDefs {
	t.Helper()
	var defs []domain.ItemDef
	if err := testfixture.LoadEndgameItems(&defs); err != nil {
		t.Fatal(err)
	}
	out := ItemDefs{}
	for _, d := range defs {
		out[d.ID] = d
	}
	return out
}

func TestEndgameDatabaseDefinitionsRejectMissingOrInvalidBase(t *testing.T) {
	for _, id := range domain.EndgameBaseItemIDs() {
		items := endgameDefinitionFixture(t)
		delete(items, id)
		if err := domain.ValidateEndgameItemDefinitions(items); err == nil || !strings.Contains(err.Error(), "000000_init-59.sql") {
			t.Fatalf("missing %d: %v", id, err)
		}
	}
	for _, change := range []func(*domain.ItemDef){
		func(d *domain.ItemDef) { d.Stackable = true },
		func(d *domain.ItemDef) { d.InstanceKind = domain.ItemInstanceNone },
		func(d *domain.ItemDef) { d.UseLevel = 0 },
		func(d *domain.ItemDef) { d.CardDefaults.Count = 0 },
		func(d *domain.ItemDef) { d.Icon = 0 },
	} {
		items := endgameDefinitionFixture(t)
		d := items[domain.FirstAstralRune]
		change(&d)
		items[d.ID] = d
		if err := domain.ValidateEndgameItemDefinitions(items); err == nil {
			t.Fatal("invalid rune base accepted")
		}
	}
}

// End-to-end original LoadItems queries against an explicitly selected disposable
// cluster. No running application or live database is used or mutated.
func TestEndgameDatabaseDefinitionsActualReadOnlyLoader(t *testing.T) {
	path := os.Getenv("FANTASY_ENDGAME_ITEMS_PG_DATA")
	if path == "" {
		t.Skip("set FANTASY_ENDGAME_ITEMS_PG_DATA to the isolated migration-59 validation cluster")
	}
	bin := os.Getenv("PG_BIN")
	if bin == "" {
		t.Fatal("PG_BIN is required")
	}
	if _, err := os.Stat(filepath.Join(path, "PG_VERSION")); err != nil {
		t.Fatal(err)
	}
	q := &v5SingleUserQuerier{bin: bin, path: path, database: "postgres"}
	items, err := LoadItems(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	expected := endgameDefinitionFixture(t)
	for id, want := range expected {
		got, exists := items[id]
		if !exists {
			t.Fatalf("normal loader missed %d", id)
		}
		if got.Name != want.Name || got.Description != want.Description || got.Icon != want.Icon || got.Level != want.Level || got.UseLevel != want.UseLevel || got.Weight != want.Weight || got.Price != want.Price || got.SellPrice != want.SellPrice || got.Stackable != want.Stackable || got.CanMail != want.CanMail || got.CanTrade != want.CanTrade || got.InventoryTab != want.InventoryTab || !got.InventoryTabKnown || got.InstanceKind != want.InstanceKind || got.CardDefaults != want.CardDefaults || !reflect.DeepEqual(got.SocketAffixes, want.SocketAffixes) {
			t.Fatalf("database item %d differs from committed configuration: got=%+v want=%+v", id, got, want)
		}
		if !got.Stackable {
			st := domain.NewStack(got, 1)
			if st.UID <= 0 || st.Item != id || st.InstanceKind != domain.ItemInstanceSocketCard || st.Card != want.CardDefaults {
				t.Fatalf("physical rune instance semantics lost: %+v", st)
			}
		}
	}
	if len(items) < 20000 || items[3001].Name == "" || items[1001].Equip == nil {
		t.Fatal("original item/equipment loader regressed")
	}
	if _, err = LoadItems(context.Background(), omitEndgameKeyQuerier{q}); err == nil || !strings.Contains(err.Error(), "000000_init-59.sql") {
		t.Fatalf("missing persisted row silently synthesized: %v", err)
	}
	t.Logf("32 persisted base definitions matched through %d actual read-only PostgreSQL queries; existing templates=%d", q.queries, len(items)-32)
}

type omitEndgameKeyQuerier struct{ Querier }

func (q omitEndgameKeyQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	sql = strings.Replace(sql, "FROM game_items i", "FROM (SELECT * FROM game_items WHERE id <> 1900028101) i", 1)
	return q.Querier.Query(ctx, sql, args...)
}
