package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type blockedCloseStore struct {
	store.Store
	release chan struct{}
}

func (s blockedCloseStore) Close() error { <-s.release; return nil }
func TestStoreCloseSharesShutdownBudget(t *testing.T) {
	st := blockedCloseStore{release: make(chan struct{})}
	defer close(st.release)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := closeStoreWithin(ctx, st); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close=%v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("pool close escaped shutdown deadline")
	}
}
