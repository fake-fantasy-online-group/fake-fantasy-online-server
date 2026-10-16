package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCommitErrorClassification(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, errors.New("connection lost"), &pgconn.PgError{Code: "40003"}} {
		got := classifyCommitError(err)
		if !errors.Is(got, ErrCommitUnknown) || !errors.Is(got, err) {
			t.Fatalf("lost uncertainty/cause: %v", got)
		}
	}
	for _, err := range []error{nil, pgx.ErrTxCommitRollback, pgx.ErrTxClosed, &pgconn.PgError{Code: "23505"}} {
		if errors.Is(classifyCommitError(err), ErrCommitUnknown) {
			t.Fatalf("definitive rejection marked unknown: %v", err)
		}
	}
}

type ambiguousSnapshotStore struct {
	*slowStore
	muTx         sync.Mutex
	calls        int
	retryStarted chan struct{}
	release      chan struct{}
}

func (s *ambiguousSnapshotStore) WithTx(ctx context.Context, fn func(Store) error) error {
	s.muTx.Lock()
	s.calls++
	n := s.calls
	s.muTx.Unlock()
	if n == 2 {
		close(s.retryStarted)
		<-s.release
		return errors.New("temporary failure after earlier unknown commit")
	}
	if err := fn(s); err != nil {
		return err
	}
	if n == 1 {
		return classifyCommitError(errors.New("connection lost after COMMIT"))
	}
	return nil
}
func TestSnapshotSettlementRetainsBothOwnersAcrossUnknownCommit(t *testing.T) {
	st := &ambiguousSnapshotStore{slowStore: newSlowStore(), retryStarted: make(chan struct{}), release: make(chan struct{})}
	w := NewWriteBack(st, time.Millisecond, nil)
	go w.Run(context.Background())
	b := ch(2, 8)
	result := w.CommitSnapshots(ch(1, 7), &b)
	<-st.retryStarted
	aNext := w.SaveAck(ch(1, 9))
	bNext := w.SaveAck(ch(2, 10))
	other := w.SaveAck(ch(3, 11))
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("unrelated owner blocked")
	}
	for _, done := range []<-chan struct{}{aNext, bNext} {
		select {
		case <-done:
			t.Fatal("owner escaped ambiguous transaction")
		default:
		}
	}
	select {
	case <-result:
		t.Fatal("unknown commit exposed as rollback")
	default:
	}
	close(st.release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idempotent recovery did not finish")
	}
	if err := w.Close(time.Second); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int32{1: 9, 2: 10} {
		got := st.written(id)
		if len(got) < 3 || got[len(got)-1] != want {
			t.Fatalf("owner %d order: %v", id, got)
		}
	}
}
