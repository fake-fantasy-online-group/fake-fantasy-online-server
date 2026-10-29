package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is deliberately independent of testStore: it never initializes, truncates,
// saves, repairs or migrates data. All character reads use one READ ONLY transaction.
// Run with the actual application's database role, not the migration administrator.
func TestSortCurrentInventoryReadOnlyProbe(t *testing.T) {
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
	bag, err := s.LoadBag(ctx, ch.ID, int(ch.BagSlots))
	if err != nil {
		t.Fatal("bag", err)
	}
	report := func(place string, entries []sortProbeEntry) {
		for i, a := range entries {
			for _, b := range entries[i+1:] {
				if a.st.Item != b.st.Item || a.st.Count >= domain.MaxStack && b.st.Count >= domain.MaxStack {
					continue
				}
				d := items[a.st.Item]
				ac, bc := a.st, b.st
				if domain.IsAstralRuneStack(ac) && ac.Bound {
					ac.Card.Bound = true
				}
				if domain.IsAstralRuneStack(bc) && bc.Bound {
					bc.Card.Bound = true
				}
				same := domain.SameStackInstance(ac, bc)
				ac.Count, bc.Count = 1, 1
				if domain.IsAstralRuneStack(ac) && domain.IsAstralRuneStack(bc) {
					ac.UID, bc.UID = 1, 1
				}
				t.Logf("sortprobe %s slots=%d/%d item=%d count=%d/%d def_stackable=%t tab=%d kind=%d/%d root_bound=%t/%t card_bound=%t/%t locked=%t/%t card_locked=%t/%t rune=%t/%t after_bag_fix_same=%t diff=%v", place, a.slot, b.slot, a.st.Item, a.st.Count, b.st.Count, d.Stackable, d.InventoryTab, a.st.InstanceKind, b.st.InstanceKind, a.st.Bound, b.st.Bound, a.st.Card.Bound, b.st.Card.Bound, a.st.Locked, b.st.Locked, a.st.Card.Locked, b.st.Card.Locked, domain.IsAstralRuneStack(a.st), domain.IsAstralRuneStack(b.st), same, sortProbeDiff(reflect.ValueOf(ac), reflect.ValueOf(bc), ""))
			}
		}
	}
	var entries []sortProbeEntry
	bag.Each(func(slot int, st domain.Stack) { entries = append(entries, sortProbeEntry{slot, st}) })
	report("bag", entries)
	rule, err := data.LoadWarehouseRule(ctx, p.ReadOnly())
	if err != nil {
		t.Fatal(err)
	}
	if rule.Valid() {
		w, err := s.LoadWarehouse(ctx, ch.ID, rule)
		if err != nil {
			t.Fatal(err)
		}
		entries = nil
		w.Each(func(page, slot int, st domain.Stack) {
			entries = append(entries, sortProbeEntry{page*w.SlotsPerPage() + slot, st})
		})
		report("warehouse", entries)
	}
}

type sortProbeEntry struct {
	slot int
	st   domain.Stack
}

func sortProbeDiff(a, b reflect.Value, path string) []string {
	if reflect.DeepEqual(a.Interface(), b.Interface()) {
		return nil
	}
	if a.Kind() != reflect.Struct {
		return []string{path}
	}
	var out []string
	for i := 0; i < a.NumField(); i++ {
		p := a.Type().Field(i).Name
		if path != "" {
			p = path + "." + p
		}
		out = append(out, sortProbeDiff(a.Field(i), b.Field(i), p)...)
	}
	return out
}
