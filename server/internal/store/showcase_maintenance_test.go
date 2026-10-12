package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type showcaseRow func(...any) error

func (r showcaseRow) Scan(dest ...any) error { return r(dest...) }

type showcaseTestTx struct {
	pgx.Tx                             // Unexpected methods panic: these tests never touch a database.
	lock                               bool
	sessions                           []int64
	prepared                           int64
	checks, clears, commits, rollbacks int
	lockErr, commitErr, rollbackErr    error
	events                             []string
}

func (t *showcaseTestTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	return showcaseRow(func(dest ...any) error {
		if strings.Contains(sql, "pg_try_advisory_xact_lock") {
			t.events = append(t.events, "advisory")
			*dest[0].(*bool) = t.lock
			return nil
		}
		t.events = append(t.events, "offline")
		if t.checks < len(t.sessions) {
			*dest[0].(*int64) = t.sessions[t.checks]
		}
		*dest[1].(*int64) = t.prepared
		t.checks++
		return nil
	})
}
func (t *showcaseTestTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if sql == showcaseTableLocks {
		t.events = append(t.events, "tables")
		return pgconn.CommandTag{}, t.lockErr
	}
	if sql != "SELECT pg_stat_clear_snapshot()" {
		panic("unexpected SQL: " + sql)
	}
	t.clears++
	t.events = append(t.events, "refresh")
	return pgconn.CommandTag{}, nil
}
func (t *showcaseTestTx) Commit(context.Context) error {
	t.commits++
	t.events = append(t.events, "commit")
	return t.commitErr
}
func (t *showcaseTestTx) Rollback(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	t.rollbacks++
	return t.rollbackErr
}

func TestShowcaseMaintenanceBoundary(t *testing.T) {
	failure := errors.New("injected failure")
	for _, tc := range []struct {
		name                  string
		dryRun                bool
		tx                    showcaseTestTx
		callbackErr           error
		wantCalled, wantError bool
		wantCommits           int
	}{
		{name: "commit", tx: showcaseTestTx{lock: true}, wantCalled: true, wantCommits: 1},
		{name: "dry-run", dryRun: true, tx: showcaseTestTx{lock: true}, wantCalled: true},
		{name: "callback failure", tx: showcaseTestTx{lock: true}, callbackErr: failure, wantCalled: true, wantError: true},
		{name: "maintenance busy", tx: showcaseTestTx{lock: false}, wantError: true},
		{name: "existing session", tx: showcaseTestTx{lock: true, sessions: []int64{1}}, wantError: true},
		{name: "new session", tx: showcaseTestTx{lock: true, sessions: []int64{0, 1}}, wantCalled: true, wantError: true},
		{name: "prepared transaction", tx: showcaseTestTx{lock: true, prepared: 1}, wantError: true},
		{name: "table lock failure", tx: showcaseTestTx{lock: true, lockErr: failure}, wantError: true},
		{name: "commit failure", tx: showcaseTestTx{lock: true, commitErr: failure}, wantCalled: true, wantError: true, wantCommits: 1},
		{name: "dry-run rollback failure", dryRun: true, tx: showcaseTestTx{lock: true, rollbackErr: failure}, wantCalled: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			err := runShowcaseMaintenance(context.Background(), &tc.tx, tc.dryRun, func(s Store, q ShowcaseQuerier) error {
				called = true
				if s.(*pgTx).tx != &tc.tx || q != &tc.tx {
					t.Fatal("callback did not receive the same production transaction")
				}
				tc.tx.events = append(tc.tx.events, "callback")
				return tc.callbackErr
			})
			if (err != nil) != tc.wantError || called != tc.wantCalled || tc.tx.commits != tc.wantCommits || tc.tx.rollbacks == 0 {
				t.Fatalf("err=%v called=%v commits=%d rollbacks=%d", err, called, tc.tx.commits, tc.tx.rollbacks)
			}
			if tc.tx.clears != tc.tx.checks {
				t.Fatalf("statistics not refreshed for every check: %v", tc.tx.events)
			}
			if tc.name == "commit" {
				want := "advisory,refresh,offline,tables,callback,refresh,offline,commit"
				if got := strings.Join(tc.tx.events, ","); got != want {
					t.Fatalf("unsafe order: %s", got)
				}
			}
		})
	}
}

func TestShowcaseMaintenancePanicRollback(t *testing.T) {
	tx := &showcaseTestTx{lock: true}
	defer func() {
		if recover() != "callback panic" {
			t.Error("panic not preserved")
		}
		if tx.rollbacks != 1 || tx.commits != 0 {
			t.Errorf("panic did not roll back: %+v", tx)
		}
	}()
	_ = runShowcaseMaintenance(context.Background(), tx, false, func(Store, ShowcaseQuerier) error { panic("callback panic") })
}

func TestShowcaseMaintenanceCancelledCallbackRollback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx := &showcaseTestTx{lock: true}
	err := runShowcaseMaintenance(ctx, tx, false, func(Store, ShowcaseQuerier) error { cancel(); return ctx.Err() })
	if !errors.Is(err, context.Canceled) || tx.rollbacks != 1 || tx.commits != 0 {
		t.Fatalf("err=%v rollbacks=%d commits=%d", err, tx.rollbacks, tx.commits)
	}
}

func TestShowcaseMaintenanceRejectNilBeforeConnect(t *testing.T) {
	if err := WithShowcaseMaintenance(context.Background(), "invalid DSN", false, nil); err == nil || !strings.Contains(err.Error(), "callback") {
		t.Fatalf("got %v", err)
	}
}

type showcaseUIDTx struct {
	pgx.Tx
	existing int64
	err      error
	calls    int
}

func (t *showcaseUIDTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	t.calls++
	if sql != `SELECT uid FROM item_instances WHERE uid=ANY($1) LIMIT 1` || len(args) != 1 {
		panic("unexpected UID query")
	}
	return showcaseRow(func(dest ...any) error { *dest[0].(*int64) = t.existing; return t.err })
}
func TestShowcaseUIDCheck(t *testing.T) {
	failure := errors.New("query failed")
	for _, tc := range []struct {
		name      string
		ids       []int64
		existing  int64
		err       error
		wantError bool
		calls     int
	}{
		{name: "empty"},
		{name: "unused", ids: []int64{11, 12}, err: pgx.ErrNoRows, calls: 1},
		{name: "collision", ids: []int64{11, 12}, existing: 12, wantError: true, calls: 1},
		{name: "query failure", ids: []int64{11}, err: failure, wantError: true, calls: 1},
		{name: "invalid", ids: []int64{0}, wantError: true},
		{name: "duplicate", ids: []int64{11, 11}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &showcaseUIDTx{existing: tc.existing, err: tc.err}
			var checker ShowcaseUIDChecker = &pgTx{tx: tx}
			err := checker.CheckShowcaseUIDsUnused(context.Background(), tc.ids)
			if (err != nil) != tc.wantError || tx.calls != tc.calls {
				t.Fatalf("err=%v calls=%d", err, tx.calls)
			}
			if tc.err == failure && !errors.Is(err, failure) {
				t.Fatalf("lost query error: %v", err)
			}
		})
	}
}
