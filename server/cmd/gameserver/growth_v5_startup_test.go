package main

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	"io"
	"log/slog"
	"os"
	"testing"
)

// Explicit read-only integration against an already migrated disposable database.
func TestV5ActualWorldStartup(t *testing.T) {
	dsn := os.Getenv("FANTASY_V5_WORLD_TEST_DSN")
	if dsn == "" {
		t.Skip("explicit migrated test database required")
	}
	st, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	w := loadWorldData(context.Background(), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if w.progressionErr != nil || w.skillErr != nil || w.colorProfileErr != nil {
		t.Fatalf("startup rejects world: progression=%v skills=%v quality=%v", w.progressionErr, w.skillErr, w.colorProfileErr)
	}
	if w.levels == nil || w.levels.MaxLevel() != 100 {
		t.Fatal("missing 100-level progression")
	}
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		if d, ok := w.skills.Get(id, 21); !ok || d.FormulaVersion != 5 {
			t.Fatalf("skill %d missing", id)
		}
	}
	if len(w.statusOverlay) == 0 || len(w.statuses) == 0 || len(w.items) == 0 {
		t.Fatal("empty world dependencies")
	}
	t.Logf("loaded skills=%d statuses=%d items=%d levelCap=%d", len(w.skills), len(w.statuses), len(w.items), w.levels.MaxLevel())
}
