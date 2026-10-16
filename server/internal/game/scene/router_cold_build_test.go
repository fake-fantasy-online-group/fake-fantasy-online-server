package scene

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func coldBuildResult(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation blocked behind cold builder")
	}
}

func TestColdBuildDoesNotBlockExistingRoute(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	r := NewRouter(func(id domain.SceneID) (*Scene, error) {
		if id.MapID == 2 {
			close(entered)
			<-release
		}
		return New(Config{ID: id}), nil
	}, nil)
	r.Start(context.Background())
	defer r.Drain(time.Second)
	existing := domain.SceneID{MapID: 1}
	if _, err := r.Ensure(existing); err != nil {
		t.Fatal(err)
	}
	built := make(chan error, 1)
	go func() { _, err := r.Ensure(domain.SceneID{MapID: 2}); built <- err }()
	<-entered
	routed := make(chan error, 1)
	go func() {
		done := make(chan struct{})
		if !r.Post(existing, Inspect{Fn: func(*Scene) {}, Done: done}) {
			routed <- fmt.Errorf("existing route rejected")
			return
		}
		<-done
		routed <- nil
	}()
	coldBuildResult(t, routed)
	unblock()
	coldBuildResult(t, built)
}

func TestColdBuildSingleFlight(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := NewRouter(func(id domain.SceneID) (*Scene, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return New(Config{ID: id}), nil
	}, nil)
	r.Start(context.Background())
	defer r.Drain(time.Second)
	const count = 32
	results := make(chan *Scene, count)
	errs := make(chan error, count)
	var callers sync.WaitGroup
	for i := 0; i < count; i++ {
		callers.Add(1)
		go func() { defer callers.Done(); s, err := r.Ensure(domain.SceneID{MapID: 1}); results <- s; errs <- err }()
	}
	<-entered
	close(release)
	callers.Wait()
	var first *Scene
	for i := 0; i < count; i++ {
		coldBuildResult(t, errs)
		s := <-results
		if first == nil {
			first = s
		}
		if s != first {
			t.Fatal("duplicate scene publication")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("builder called %d times", calls.Load())
	}
}

func TestColdBuildCannotPublishAfterDrain(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var built *Scene
	r := NewRouter(func(id domain.SceneID) (*Scene, error) {
		close(entered)
		<-release
		built = New(Config{ID: id})
		return built, nil
	}, nil)
	r.Start(context.Background())
	result := make(chan error, 1)
	go func() { _, err := r.Ensure(domain.SceneID{MapID: 1}); result <- err }()
	<-entered
	drained := make(chan error, 1)
	go func() { drained <- r.Drain(time.Second) }()
	coldBuildResult(t, drained)
	unblock()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("late build accepted after drain")
		}
	case <-time.After(time.Second):
		t.Fatal("builder did not complete")
	}
	if r.Count() != 0 {
		t.Fatal("late scene published")
	}
	select {
	case <-built.quit:
	default:
		t.Fatal("discarded scene not stopped")
	}
}

func TestColdBuildFailureAllowsRetry(t *testing.T) {
	var calls int
	r := NewRouter(func(id domain.SceneID) (*Scene, error) {
		calls++
		if calls == 1 {
			panic("cold build failure")
		}
		return New(Config{ID: id}), nil
	}, nil)
	r.Start(context.Background())
	defer r.Drain(time.Second)
	if _, err := r.Ensure(domain.SceneID{MapID: 1}); err == nil {
		t.Fatal("panic should fail the build")
	}
	if _, err := r.Ensure(domain.SceneID{MapID: 1}); err != nil {
		t.Fatal(err)
	}
}
