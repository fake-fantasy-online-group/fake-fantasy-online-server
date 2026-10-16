package session

import (
	"context"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type friendDeadlineStore struct {
	store.Store
	load func(context.Context, int64) ([]domain.Friend, error)
}

func (s friendDeadlineStore) LoadFriends(ctx context.Context, id int64) ([]domain.Friend, error) {
	return s.load(ctx, id)
}

func TestFriendNotificationPropagatesDeadlineToRecipient(t *testing.T) {
	r := newFriendRig(t)
	source, target := r.login("source", 11, 1), r.login("target", 22, 1)
	target.sink = &capSink{}
	if !r.idx.AttachControl(domain.CharID(target.char.ID), target.entity, gmSessionControl{sess: target}) {
		t.Fatal("attach target control")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	deadline, _ := ctx.Deadline()
	source.deps.Store = friendDeadlineStore{Store: r.store, load: func(context.Context, int64) ([]domain.Friend, error) {
		return []domain.Friend{{Char: domain.CharID(target.char.ID)}}, nil
	}}
	read := false
	target.deps.Store = friendDeadlineStore{Store: r.store, load: func(got context.Context, _ int64) ([]domain.Friend, error) {
		read = true
		actual, ok := got.Deadline()
		if !ok || !actual.Equal(deadline) {
			t.Error("recipient lost caller deadline")
			return nil, context.DeadlineExceeded
		}
		<-got.Done()
		return nil, got.Err()
	}}
	source.notifyFriends(ctx, source.char, false)
	if !read {
		t.Fatal("recipient was not refreshed")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatal("recipient query did not respect cancellation")
	}
}

func TestStandaloneFriendRefreshHasDeadline(t *testing.T) {
	r := newFriendRig(t)
	s := r.login("reader", 11, 1)
	s.sink = &capSink{}
	read := false
	s.deps.Store = friendDeadlineStore{Store: r.store, load: func(ctx context.Context, _ int64) ([]domain.Friend, error) {
		read = true
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Error("standalone friend refresh is unbounded")
		}
		return nil, nil
	}}
	s.refreshFriendList()
	if !read {
		t.Fatal("refresh did not read friends")
	}
}
