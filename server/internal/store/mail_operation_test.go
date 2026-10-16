package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type mailOutcomeFake struct {
	mu                        sync.Mutex
	committed                 bool
	commitCalls, resolveCalls int
	unavailable               chan struct{}
}

func (f *mailOutcomeFake) CommitMailOperation(context.Context, MailOperation, domain.Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitCalls++
	return errors.New("connection lost after COMMIT")
}
func (f *mailOutcomeFake) ResolveMailOperation(ctx context.Context, _ MailOperation, _ domain.Snapshot) (bool, error) {
	f.mu.Lock()
	f.resolveCalls++
	f.mu.Unlock()
	if f.unavailable != nil {
		select {
		case <-f.unavailable:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	return f.committed, nil
}
func TestMailLostCommitAcknowledgementReconcilesWithoutRetryingTransfer(t *testing.T) {
	f := &mailOutcomeFake{committed: true}
	if err := SettleMailOperation(context.Background(), f, MailOperation{}, domain.Snapshot{}, domain.Snapshot{}); err != nil {
		t.Fatal(err)
	}
	if f.commitCalls != 1 || f.resolveCalls != 1 {
		t.Fatalf("transfer replayed: commits=%d resolves=%d", f.commitCalls, f.resolveCalls)
	}
}
func TestMailVerifiedRollbackReturnsFailure(t *testing.T) {
	f := &mailOutcomeFake{}
	if err := SettleMailOperation(context.Background(), f, MailOperation{}, domain.Snapshot{}, domain.Snapshot{}); err == nil {
		t.Fatal("rollback reported success")
	}
}
func TestMailUnknownOutcomeRetainsOwnershipPastAttemptCancellation(t *testing.T) {
	f := &mailOutcomeFake{committed: true, unavailable: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- SettleMailOperation(ctx, f, MailOperation{}, domain.Snapshot{}, domain.Snapshot{}) }()
	select {
	case err := <-done:
		t.Fatalf("unknown outcome released ownership: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(f.unavailable)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("did not reconcile")
	}
}

// The receipt lookup must be behind the same PostgreSQL transaction lock as
// submission: reading an absent row without that barrier is not rollback proof.
type mailReceiptTx struct {
	pgx.Tx
	locked  bool
	lockErr error
	row     pgx.Row
}

func (f *mailReceiptTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if !strings.Contains(sql, "pg_advisory_xact_lock") {
		return pgconn.CommandTag{}, errors.New("unexpected SQL")
	}
	if f.lockErr != nil {
		return pgconn.CommandTag{}, f.lockErr
	}
	f.locked = true
	return pgconn.NewCommandTag("SELECT 1"), nil
}
func (f *mailReceiptTx) QueryRow(context.Context, string, ...any) pgx.Row {
	if !f.locked {
		panic("receipt read before transaction outcome barrier")
	}
	return f.row
}

type mailReceiptRow struct {
	charID    int64
	digest    string
	committed bool
	err       error
}

func (r mailReceiptRow) Scan(dst ...any) error {
	if r.err != nil {
		return r.err
	}
	*dst[0].(*int64) = r.charID
	*dst[1].(*string) = r.digest
	*dst[2].(*bool) = r.committed
	return nil
}
func TestMailReceiptLookupRequiresOutcomeBarrier(t *testing.T) {
	for _, committed := range []bool{false, true} {
		fake := &mailReceiptTx{row: mailReceiptRow{charID: 7, digest: "digest", committed: committed}}
		exists, result, err := mailOperationReceipt(context.Background(), &pgTx{tx: fake}, MailOperation{ID: "id"}, 7, "digest")
		if err != nil || !exists || result != committed || !fake.locked {
			t.Fatalf("receipt result=%v %v %v", exists, result, err)
		}
	}
	fake := &mailReceiptTx{row: mailReceiptRow{err: pgx.ErrNoRows}}
	exists, _, err := mailOperationReceipt(context.Background(), &pgTx{tx: fake}, MailOperation{ID: "id"}, 7, "digest")
	if exists || err != nil || !fake.locked {
		t.Fatal("absent receipt not protected by lock")
	}
	fake = &mailReceiptTx{lockErr: context.DeadlineExceeded}
	_, _, err = mailOperationReceipt(context.Background(), &pgTx{tx: fake}, MailOperation{ID: "id"}, 7, "digest")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lock timeout treated as rollback")
	}
}
func TestMailReceiptRejectsIdentityMismatch(t *testing.T) {
	for _, row := range []mailReceiptRow{{charID: 8, digest: "digest"}, {charID: 7, digest: "other"}} {
		fake := &mailReceiptTx{row: row}
		_, _, err := mailOperationReceipt(context.Background(), &pgTx{tx: fake}, MailOperation{ID: "id"}, 7, "digest")
		if err == nil {
			t.Fatal("mismatched operation accepted")
		}
	}
}

func (f *mailOutcomeFake) CheckMailOperations(context.Context) error { return nil }
