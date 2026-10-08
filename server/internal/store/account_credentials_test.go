package store

import (
	"context"
	"sync"
	"testing"
)

func TestMemoryCredentialCompareAndSwap(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	acc, err := m.CreateRegisteredAccount(ctx, "owner", "old-pass", "old-sec")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, password := range []string{"new-pass-a", "new-pass-b"} {
		wg.Add(1)
		go func(password string) {
			defer wg.Done()
			updated, err := m.CompareAndSwapAccountCredentials(ctx, acc.ID, "old-pass", "old-sec", password, "old-sec")
			if err != nil {
				t.Error(err)
			}
			results <- updated
		}(password)
	}
	wg.Wait()
	if a, b := <-results, <-results; a == b {
		t.Fatal("exactly one concurrent credential update must succeed")
	}
	current, _ := m.AccountByName(ctx, "owner")
	if updated, err := m.CompareAndSwapAccountCredentials(ctx, acc.ID, current.PassHash, "stale-sec", "hijacked", "new-sec"); err != nil || updated {
		t.Fatal("CAS ignored the verified security hash", updated, err)
	}
	if err := m.WithTx(ctx, func(tx Store) error {
		updated, err := tx.CompareAndSwapAccountCredentials(ctx, acc.ID, current.PassHash, "old-sec", current.PassHash, "new-sec")
		if !updated {
			t.Error("valid transaction CAS failed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
