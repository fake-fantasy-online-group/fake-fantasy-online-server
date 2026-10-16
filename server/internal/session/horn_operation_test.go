package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type hornOutcomeStore struct {
	saved []domain.Snapshot
	store.Store
	unknown bool
	calls   int
	retry   chan struct{}
	release chan struct{}
}

func (s *hornOutcomeStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	s.calls++
	if !s.unknown {
		return errors.New("known rejected transaction")
	}
	if s.calls == 1 {
		if err := fn(s); err != nil {
			return err
		}
		return store.ErrCommitUnknown
	}
	if s.calls == 2 {
		close(s.retry)
		<-s.release
		return errors.New("later failure cannot disprove first commit")
	}
	return fn(s)
}
func (s *hornOutcomeStore) SaveSnapshot(_ context.Context, snap domain.Snapshot) error {
	s.saved = append(s.saved, snap.Clone())
	return nil
}
func TestHornWritebackKnownFailureAndUnknownOutcome(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "known failure", true: "unknown commit"}[unknown], func(t *testing.T) {
			backend := &hornOutcomeStore{Store: store.NewMemory(), unknown: unknown, retry: make(chan struct{}), release: make(chan struct{})}
			wb := store.NewWriteBack(backend, time.Millisecond, nil)
			go wb.Run(context.Background())
			s := &Session{deps: Deps{WriteBack: wb}}
			bag := domain.NewBag(10)
			bag.Set(0, domain.Stack{Item: 777001, Count: 1})
			candidate := domain.Snapshot{Char: &domain.Character{ID: 1, AccountID: 1}, Bag: bag}
			done := make(chan error, 1)
			go func() { done <- s.commitReservedHorn(scene.MailReserveResult{Snapshot: candidate}) }()
			if unknown {
				select {
				case <-backend.retry:
				case <-time.After(time.Second):
					t.Fatal("unknown outcome not retried")
				}
				select {
				case <-done:
					t.Fatal("unknown horn outcome exposed as failure")
				default:
				}
				close(backend.release)
			}
			select {
			case err := <-done:
				if (err == nil) != unknown {
					t.Fatalf("wrong horn outcome: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("horn completion missing")
			}
			if unknown {
				if len(backend.saved) != 2 {
					t.Fatal("candidate was not replayed")
				}
				for _, snap := range backend.saved {
					if snap.Bag.CountOf(777001) != 1 {
						t.Fatal("retry consumed the horn twice")
					}
				}
			}
			if err := wb.Close(time.Second); err != nil {
				t.Fatal(err)
			}
		})
	}
}
