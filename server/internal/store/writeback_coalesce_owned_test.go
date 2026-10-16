package store

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func ownedFull(id int64) domain.Snapshot {
	s := domain.Snapshot{Char: &domain.Character{ID: id, Level: 20, Attrs: make([]int32, 100), EquipmentProcCooldowns: map[string]int64{"a": 12}}, Bag: domain.NewBag(100)}
	for i := 0; i < 100; i++ {
		s.Bag.Set(i, domain.Stack{Item: 1, Count: 2})
	}
	return s
}
func ownedPosition(id int64, x int32) domain.Snapshot {
	return domain.Snapshot{Parts: domain.SnapshotPosition, Char: &domain.Character{ID: id, Pos: domain.Pos{X: float64(x)}, SceneInstance: uint32(x)}}
}

func TestPendingOwnedMergeEquivalentAndIsolated(t *testing.T) {
	full := ownedFull(1)
	initial := full.Clone()
	got, want := full, full
	owned := false
	updates := []domain.Snapshot{ownedPosition(1, 1), ownedPosition(1, 2), ownedFull(1), ownedPosition(1, 3), ownedPosition(2, 4), ownedPosition(2, 5), {}, ownedFull(1), ownedPosition(1, 6)}
	for i, next := range updates {
		before := next.Clone()
		got = mergePendingSnapshot(got, next, &owned)
		want = domain.MergeSnapshots(want, next)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("update %d differs", i)
		}
		if !reflect.DeepEqual(next, before) {
			t.Fatalf("update %d mutated input", i)
		}
	}
	if !reflect.DeepEqual(full, initial) {
		t.Fatal("mutated original aggregate")
	}
	got.Char.EquipmentProcCooldowns["a"] = 99
	got.Char.Attrs[0] = 99
	got.Bag.Set(0, domain.Stack{})
	if full.Char.EquipmentProcCooldowns["a"] != 12 || full.Char.Attrs[0] != 0 || full.Bag.At(0).Count != 2 {
		t.Fatal("private merge aliases caller")
	}
}

func TestPendingOwnedMergeReusesOnlyPrivateAggregate(t *testing.T) {
	w := NewWriteBack(nil, time.Second, nil)
	full := ownedFull(1)
	w.Save(full)
	w.Save(ownedPosition(1, 1))
	first := w.pending[1].snap
	w.Save(ownedPosition(1, 2))
	if w.pending[1].snap.Char != first.Char || w.pending[1].snap.Bag != first.Bag {
		t.Fatal("repeated movement cloned private snapshot")
	}
	if full.Char.Pos.X != 0 {
		t.Fatal("caller changed")
	}
	replacement := ownedFull(1)
	w.Save(replacement)
	w.Save(ownedPosition(1, 3))
	if w.pending[1].snap.Char == replacement.Char || replacement.Char.Pos.X != 0 {
		t.Fatal("replacement incorrectly treated as private")
	}
}

type ownedCaptureStore struct {
	Store
	entered chan domain.Snapshot
	release chan error
}

func (s *ownedCaptureStore) SaveSnapshot(_ context.Context, snap domain.Snapshot) error {
	s.entered <- snap
	return <-s.release
}
func ownedReceive(t *testing.T, ch <-chan domain.Snapshot) domain.Snapshot {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("writer did not start")
		return domain.Snapshot{}
	}
}
func ownedAck(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("ack did not complete")
	}
}

func TestPendingOwnedMergeFailureRetainedSnapshotAndAck(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "Requeued"
		if pending {
			name = "NewerPending"
		}
		t.Run(name, func(t *testing.T) {
			st := &ownedCaptureStore{entered: make(chan domain.Snapshot, 4), release: make(chan error, 4)}
			w := NewWriteBack(st, time.Millisecond, nil)
			w.Save(ownedFull(1))
			ack := w.SaveAck(ownedPosition(1, 1))
			w.drain(context.Background())
			retained := ownedReceive(t, st.entered)
			// Concurrent readers may retain callback inputs, including after failure.
			stop := make(chan struct{})
			var readers sync.WaitGroup
			readers.Add(1)
			go func() {
				defer readers.Done()
				for {
					select {
					case <-stop:
						return
					default:
						_ = retained.Char.Pos.X
						_ = retained.Bag.At(0)
					}
				}
			}()
			var pendingAck <-chan struct{}
			if pending {
				pendingAck = w.SaveAck(ownedPosition(1, 2))
			}
			st.release <- errors.New("retry")
			waitFor(t, "failed snapshot requeued", func() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.pending[1] != nil && w.inflight[1] == nil })
			w.Save(ownedPosition(1, 2))
			latest := w.SaveAck(ownedPosition(1, 3))
			if retained.Char.Pos.X != 1 {
				t.Fatal("failed writer snapshot mutated")
			}
			select {
			case <-ack:
				t.Fatal("ack closed before success")
			default:
			}
			w.mu.Lock()
			w.pending[1].retryAt = time.Time{}
			w.mu.Unlock()
			w.drain(context.Background())
			retry := ownedReceive(t, st.entered)
			if retry.Char.Pos.X != 3 || retry.Bag.At(0).Count != 2 {
				t.Fatal("retry lost state")
			}
			st.release <- nil
			ownedAck(t, ack)
			ownedAck(t, latest)
			if pending {
				ownedAck(t, pendingAck)
			}
			close(stop)
			readers.Wait()
		})
	}
}

func TestPendingOwnedPairAndMutationCallbacks(t *testing.T) {
	w := NewWriteBack(nil, time.Second, nil)
	a, b := ownedFull(1), ownedFull(2)
	called := make(chan domain.Snapshot, 2)
	result := w.CommitPairMutation(a, b, func(_ context.Context, _ Store, a, b domain.Snapshot) error { called <- a; called <- b; return nil })
	ack := w.SaveAck(ownedPosition(1, 1))
	w.Save(ownedPosition(1, 2))
	w.Save(ownedPosition(2, 3))
	w.Save(ownedPosition(2, 4))
	w.drain(context.Background())
	aa, bb := ownedReceive(t, called), ownedReceive(t, called)
	if aa.Char.Pos.X != 2 || bb.Char.Pos.X != 4 || a.Char.Pos.X != 0 || b.Char.Pos.X != 0 {
		t.Fatal("pair coalescing changed inputs or lost positions")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	ownedAck(t, ack)
	w.Save(a)
	w.Save(ownedPosition(1, 5))
	result = w.CommitMutation(ownedPosition(1, 6), func(_ context.Context, _ Store, s domain.Snapshot) error { called <- s; return nil })
	w.Save(ownedPosition(1, 7))
	w.drain(context.Background())
	if s := ownedReceive(t, called); s.Char.Pos.X != 7 || s.Bag.At(0).Count != 2 {
		t.Fatal("mutation lost coalesced state")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func BenchmarkPendingOwnedMerge(b *testing.B) {
	for _, legacy := range []bool{true, false} {
		label := "Owned"
		if legacy {
			label = "Legacy"
		}
		b.Run(label, func(b *testing.B) {
			for _, first := range []bool{true, false} {
				name := "Repeated"
				if first {
					name = "First"
				}
				b.Run(name, func(b *testing.B) {
					full, pos := ownedFull(1), ownedPosition(1, 2)
					owned := false
					s := full
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if first {
							owned = false
							s = full
						}
						if legacy {
							s = domain.MergeSnapshots(s, pos)
						} else {
							s = mergePendingSnapshot(s, pos, &owned)
						}
					}
					_ = s
				})
			}
		})
	}
}

func BenchmarkWriteBackDelayedParallelPosition(b *testing.B) {
	for _, legacy := range []bool{true, false} {
		label := "Owned"
		if legacy {
			label = "Legacy"
		}
		b.Run(label, func(b *testing.B) { benchmarkDelayedPosition(b, legacy) })
	}
}

func benchmarkDelayedPosition(b *testing.B, legacy bool) {
	st := &ownedCaptureStore{entered: make(chan domain.Snapshot, 64), release: make(chan error, 64)}
	w := NewWriteBack(st, time.Second, nil)
	for id := int64(1); id <= 64; id++ {
		w.Save(ownedFull(id))
	}
	w.drain(context.Background())
	for i := 0; i < 64; i++ {
		<-st.entered
	}
	for id := int64(1); id <= 64; id++ {
		w.Save(ownedFull(id))
	}
	var next atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := next.Add(1)%64 + 1
		pos := ownedPosition(id, 2)
		for pb.Next() {
			if legacy {
				// Exact pre-optimization enqueue path for this fixture: every
				// lane has an inflight writer and a pending full snapshot.
				w.mu.Lock()
				queued := w.pending[id]
				queued.snap = domain.MergeSnapshots(queued.snap, pos)
				w.mu.Unlock()
				select {
				case w.wake <- struct{}{}:
				default:
				}
			} else {
				w.Save(pos)
			}
		}
	})
	b.StopTimer()
	for i := 0; i < 64; i++ {
		st.release <- nil
	}
	// Wait for every callback to finish before the fixture is discarded.
	for {
		w.mu.Lock()
		n := len(w.inflight)
		w.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
}
