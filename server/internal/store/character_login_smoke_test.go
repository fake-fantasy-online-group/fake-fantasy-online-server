package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is deliberately independent of testStore: it never initializes, truncates,
// saves, repairs or migrates data. All character reads use one READ ONLY transaction.
// Run with the actual application's database role, not the migration administrator.
func TestCharacterLoginReadOnlySmoke(t *testing.T) {
	dsn, name := os.Getenv("FANTASY_CHARACTER_SMOKE_DSN"), os.Getenv("FANTASY_CHARACTER_SMOKE_NAME")
	if dsn == "" && name == "" {
		t.Skip("explicit application DSN and existing character name required")
	}
	if dsn == "" || name == "" {
		t.Fatal("both FANTASY_CHARACTER_SMOKE_DSN and FANTASY_CHARACTER_SMOKE_NAME are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Catalog loaders may issue a nested query while iterating rows. They need
	// separate pool connections, all enforced READ ONLY by the server default.
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := &Postgres{pool: pool}
	defer p.Close()
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var role string
	var superuser bool
	if err := tx.QueryRow(ctx, `SELECT current_user, rolsuper FROM pg_roles WHERE rolname=current_user`).Scan(&role, &superuser); err != nil {
		t.Fatal(err)
	}
	if superuser {
		t.Fatal("smoke must use the application role, not a superuser")
	}
	t.Logf("application role=%s; transaction is READ ONLY", role)
	// Empty spirit tables still require privileges; an empty character must not
	// accidentally skip this release gate. Check each privilege separately because
	// has_table_privilege's comma-separated form means ANY, not ALL.
	for _, table := range []string{"spirit_instances", "equipment_spirit_fusions"} {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT has_table_privilege(current_user,$1,$2)`, table, privilege).Scan(&ok); err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("application role %s lacks %s on %s", role, privilege, table)
			}
		}
	}
	items, err := data.LoadItems(ctx, p.ReadOnly())
	if err != nil {
		t.Fatal("item definitions", err)
	}
	skills, _, err := data.LoadSkills(ctx, p.ReadOnly())
	if err != nil {
		t.Fatal("skill definitions", err)
	}
	catalog, err := data.LoadEndgameCatalog(ctx, p.ReadOnly(), items, skills.ProgressionCatalog())
	if err != nil {
		t.Fatal("endgame catalog", err)
	}
	s := &pgTx{tx: tx}
	if err := SetItemDefinitions(s, items); err != nil {
		t.Fatal(err)
	}
	if err := SetEndgameCatalog(s, catalog); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CharByName(ctx, name)
	if err != nil {
		t.Fatal("character", err)
	}
	if ch == nil {
		t.Fatal("existing character not found")
	}
	if _, err := s.LoadBag(ctx, ch.ID, int(ch.BagSlots)); err != nil {
		t.Fatal("bag", err)
	}
	if _, err := s.LoadStall(ctx, ch.ID); err != nil {
		t.Fatal("stall", err)
	}
	if _, err := s.LoadEquips(ctx, ch.ID); err != nil {
		t.Fatal("equipment", err)
	}
	if _, err := s.LoadChangeSet(ctx, ch.ID); err != nil {
		t.Fatal("change set", err)
	}
	warehouseRule, err := data.LoadWarehouseRule(ctx, p.ReadOnly())
	if err != nil {
		t.Fatal("warehouse rule", err)
	}
	if warehouseRule.Valid() {
		if _, err := s.LoadWarehouse(ctx, ch.ID, warehouseRule); err != nil {
			t.Fatal("warehouse", err)
		}
	}
	wardrobeRule, err := data.LoadWardrobeRule(ctx, p.ReadOnly())
	if err != nil {
		t.Fatal("wardrobe rule", err)
	}
	if wardrobeRule.Valid() {
		if _, err := s.LoadWardrobe(ctx, ch.ID, wardrobeRule); err != nil {
			t.Fatal("wardrobe", err)
		}
	}
	if _, err := s.LoadSkills(ctx, ch.ID); err != nil {
		t.Fatal("skills", err)
	}
	if _, err := s.LoadQuests(ctx, ch.ID); err != nil {
		t.Fatal("quests", err)
	}
	if _, err := s.LoadPets(ctx, ch.ID); err != nil {
		t.Fatal("pets", err)
	}
	if _, err := s.LoadSeenTips(ctx, ch.ID); err != nil {
		t.Fatal("seen tips", err)
	}
	t.Logf("character id=%d bag/equipment/change set/stall/warehouse/wardrobe/skills/quests/pets/tips loaded through real pgx", ch.ID)
}
