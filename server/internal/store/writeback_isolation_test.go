package store

import (
	"context"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// A callback intentionally ignores its expired context while reconciling an
// unknown commit. Only that character's lane may be held.
func TestWriteBackUnknownCommitIsolatesCharacter(t *testing.T) {
	st := newSlowStore()
	w := NewWriteBack(st, time.Millisecond, nil)
	w.attemptTimeout = 10 * time.Millisecond
	go w.Run(context.Background())
	started, release := make(chan struct{}), make(chan struct{})
	result := w.CommitMutation(ch(1, 1), func(ctx context.Context, _ Store, _ domain.Snapshot) error {
		close(started)
		<-ctx.Done()
		<-release
		return nil
	})
	<-started
	same := w.SaveAck(ch(1, 2))
	other := w.SaveAck(ch(2, 3))
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("unrelated character blocked by unknown commit")
	}
	select {
	case <-same:
		t.Fatal("same character bypassed unresolved mutation")
	default:
	}
	if err := w.Close(20 * time.Millisecond); err == nil {
		t.Fatal("unresolved mutation reported drained")
	}
	select {
	case <-result:
		t.Fatal("timeout released unknown economic ownership")
	default:
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := w.Close(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-same:
	default:
		t.Fatal("successor was not committed")
	}
}

func TestWriteBackPairWaitsForBothPriorLanes(t *testing.T) {
	st := gatedStore()
	w := NewWriteBack(st, time.Millisecond, nil)
	go w.Run(context.Background())
	first := w.SaveAck(ch(1, 1))
	<-st.blocked
	called := make(chan struct{})
	result := w.CommitPairMutation(ch(1, 2), ch(2, 2), func(_ context.Context, _ Store, _, _ domain.Snapshot) error {
		close(called)
		return nil
	})
	select {
	case <-called:
		t.Fatal("pair overtook older single write")
	case <-time.After(20 * time.Millisecond):
	}
	close(st.gate)
	<-first
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := w.Close(time.Second); err != nil {
		t.Fatal(err)
	}
}

type deadlineStore struct {
	Store
	expired chan struct{}
	recover chan struct{}
}

func (s deadlineStore) SaveSnapshot(ctx context.Context, _ domain.Snapshot) error {
	select {
	case <-s.recover:
		return nil
	case <-ctx.Done():
	}
	select {
	case s.expired <- struct{}{}:
	default:
	}
	return ctx.Err()
}
func TestWriteBackSaveAttemptsHaveDeadline(t *testing.T) {
	st := deadlineStore{expired: make(chan struct{}, 1), recover: make(chan struct{})}
	w := NewWriteBack(st, time.Millisecond, nil)
	w.attemptTimeout = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	ack := w.SaveAck(ch(3, 1))
	select {
	case <-st.expired:
	case <-time.After(time.Second):
		t.Fatal("save context had no deadline")
	}
	select {
	case <-ack:
		t.Fatal("failed save acknowledged")
	default:
	}
	if w.Pending() == 0 {
		t.Fatal("failed save discarded")
	}
	close(st.recover)
	if err := w.Close(time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWriteBackPositionCoalescingKeepsFullSnapshot(t *testing.T) {
	st := newSlowStore()
	w := NewWriteBack(st, time.Millisecond, nil)
	w.Save(ch(4, 42))
	w.Save(domain.Snapshot{Parts: domain.SnapshotPosition, Char: &domain.Character{ID: 4, Pos: domain.Pos{X: 20}}})
	go w.Run(context.Background())
	if err := w.Close(time.Second); err != nil {
		t.Fatal(err)
	}
	got := st.written(4)
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("position update discarded full character: %v", got)
	}
}

func TestWriteBackPositionRetryKeepsFailedFullSnapshot(t *testing.T) {
	st := gatedStore()
	st.failN = 1
	w := NewWriteBack(st, time.Millisecond, nil)
	go w.Run(context.Background())
	ack := w.SaveAck(ch(4, 42))
	<-st.blocked
	w.Save(domain.Snapshot{Parts: domain.SnapshotPosition, Char: &domain.Character{ID: 4, Pos: domain.Pos{X: 20}}})
	close(st.gate)
	if err := w.Close(time.Second); err != nil {
		t.Fatal(err)
	}
	got := st.written(4)
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("position retry discarded full character: %v", got)
	}
	select {
	case <-ack:
	default:
		t.Fatal("failed full save waiter lost during merge")
	}
}
