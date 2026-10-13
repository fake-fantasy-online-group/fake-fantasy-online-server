package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ShowcaseQuerier permits loading the production item catalog on the maintenance
// transaction's connection, without opening a second database session.
type ShowcaseQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// WithShowcaseMaintenance runs an offline showcase grant in one production store
// transaction. The caller MUST stop every game service and prevent restarts for
// the entire invocation. These database guards cannot invalidate in-memory game
// state, and are not a replacement for that external maintenance boundary.
//
// It uses one dedicated connection, rejects other database sessions (even idle
// ones), serializes maintenance commands, and locks every snapshot table before
// invoking fn. All target resolution, catalog reads, validation and SaveSnapshot
// calls must happen through the callback arguments. Do not retain them or open
// another connection. A nil callback is rejected before connecting. dryRun runs
// the same work but rolls back; errors and panics also roll back. As with any
// database transaction, nontransactional sequence increments are not undone.
func WithShowcaseMaintenance(ctx context.Context, dsn string, dryRun bool, fn func(Store, ShowcaseQuerier) error) error {
	if fn == nil {
		return fmt.Errorf("store: showcase maintenance callback is required")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("store: connect showcase maintenance: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(cleanup)
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("store: begin showcase maintenance: %w", err)
	}
	return runShowcaseMaintenance(ctx, tx, dryRun, fn)
}

// Fixed lock order and NOWAIT fail closed rather than waiting on a live server.
// Keep this list in sync with pgTx.SaveSnapshot and item instance persistence.
const showcaseTableLocks = `LOCK TABLE accounts, characters,
 character_items, character_equips, character_change_set_items,
 character_warehouses, character_warehouse_items,
 character_wardrobes, character_wardrobe_items,
 character_stalls, character_stall_items, character_stall_sales,
 character_skills, character_quests, character_quest_kills, character_titles,
 char_pets, char_pet_skills, char_pet_items,
 item_instances, equipment_instances, card_instances, equipment_endgame_state,
 character_endgame_state, endgame_receipts, endgame_runs
 IN ACCESS EXCLUSIVE MODE NOWAIT`

func runShowcaseMaintenance(ctx context.Context, tx pgx.Tx, dryRun bool, fn func(Store, ShowcaseQuerier) error) error {
	defer func() {
		// Cancellation must not strand the connection's transaction or its locks.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(1936224119, 1)`).Scan(&locked); err != nil {
		return fmt.Errorf("store: acquire showcase maintenance lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("store: another showcase maintenance command is running")
	}
	if err := assertShowcaseOffline(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, showcaseTableLocks); err != nil {
		return fmt.Errorf("store: lock showcase snapshot tables: %w", err)
	}
	if err := fn(&pgTx{tx: tx}, tx); err != nil {
		return err
	}
	if err := assertShowcaseOffline(ctx, tx); err != nil {
		return err
	}
	if dryRun {
		if err := tx.Rollback(ctx); err != nil {
			return fmt.Errorf("store: rollback showcase dry-run: %w", err)
		}
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit showcase maintenance: %w", err)
	}
	return nil
}

func assertShowcaseOffline(ctx context.Context, tx pgx.Tx) error {
	// PostgreSQL may cache statistics for a transaction. Refresh before EACH
	// check, especially the final check for clients that connected during fn.
	if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		return fmt.Errorf("store: refresh showcase connection check: %w", err)
	}
	var sessions, prepared int64
	// Count all backends rather than filtering on potentially redacted activity
	// fields. This deliberately fails closed, including for background workers.
	if err := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM pg_stat_activity WHERE datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND pid<>pg_backend_pid()),
 (SELECT count(*) FROM pg_prepared_xacts WHERE database=current_database())`).Scan(&sessions, &prepared); err != nil {
		return fmt.Errorf("store: verify showcase offline database: %w", err)
	}
	if sessions != 0 || prepared != 0 {
		return fmt.Errorf("store: showcase requires offline database: %d other sessions and %d prepared transactions; stop all services and prevent restarts", sessions, prepared)
	}
	return nil
}

// ShowcaseUIDChecker checks the global instance namespace before granting new
// items. Use only inside WithShowcaseMaintenance, whose table locks close the
// gap between this check and SaveSnapshot's upserts.
type ShowcaseUIDChecker interface {
	CheckShowcaseUIDsUnused(context.Context, []int64) error
}

func (t *pgTx) CheckShowcaseUIDsUnused(ctx context.Context, uids []int64) error {
	if len(uids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(uids))
	for _, uid := range uids {
		if uid <= 0 {
			return fmt.Errorf("store: invalid showcase UID %d", uid)
		}
		if _, exists := seen[uid]; exists {
			return fmt.Errorf("store: duplicate showcase UID %d", uid)
		}
		seen[uid] = struct{}{}
	}
	var existing int64
	err := t.tx.QueryRow(ctx, `SELECT uid FROM item_instances WHERE uid=ANY($1) LIMIT 1`, uids).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: check showcase UID namespace: %w", err)
	}
	return fmt.Errorf("store: showcase UID %d already exists", existing)
}
