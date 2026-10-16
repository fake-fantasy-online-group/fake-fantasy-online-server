package session

import (
	"context"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type closeFriendStore struct {
	store.Store
	read func(context.Context)
}

func (s closeFriendStore) LoadFriends(ctx context.Context, _ int64) ([]domain.Friend, error) {
	s.read(ctx)
	return nil, nil
}

func TestCloseRemovesPresenceBeforeOptionalDatabaseRead(t *testing.T) {
	r, router, stop := closeRig(t)
	defer stop()
	s := r.login("closing", 11, 1)
	id := domain.CharID(s.char.ID)
	s.deps.Router = router
	checked := false
	s.deps.Store = closeFriendStore{Store: r.store, read: func(ctx context.Context) {
		checked = true
		if r.idx.Online(id) {
			t.Error("offline notification read preceded presence removal")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Error("optional close read lacks bounded deadline")
		}
		s.mu.Lock()
		stage := s.stage
		s.mu.Unlock()
		if stage != StageClosed {
			t.Error("session remains active during optional read")
		}
	}}
	s.OnClose("test")
	if !checked {
		t.Fatal("notification was not attempted")
	}
}
