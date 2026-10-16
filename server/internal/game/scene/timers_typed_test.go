package scene

import (
	"container/heap"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type referenceTimerHeap []timer

func (h referenceTimerHeap) Len() int           { return len(h) }
func (h referenceTimerHeap) Less(i, j int) bool { return h[i].at < h[j].at }
func (h referenceTimerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *referenceTimerHeap) Push(x any)        { *h = append(*h, x.(timer)) }
func (h *referenceTimerHeap) Pop() any {
	old := *h
	n := len(old) - 1
	v := old[n]
	old[n] = timer{}
	*h = old[:n]
	return v
}

func TestTypedTimerMatchesReferenceOrder(t *testing.T) {
	for seed := int64(0); seed < 32; seed++ {
		r := rand.New(rand.NewSource(seed))
		var got timers
		var want referenceTimerHeap
		var gotCalls, wantCalls []int
		next := 0
		for now := domain.Tick(0); now < 128; now++ {
			for k := 0; k < r.Intn(64); k++ {
				id := next
				next++
				d := domain.Tick(r.Intn(12))
				got.after(now, d, func() { gotCalls = append(gotCalls, id) })
				if d == 0 {
					d = 1
				}
				heap.Push(&want, timer{at: now + d, fn: func() { wantCalls = append(wantCalls, id) }})
			}
			got.advance(now)
			for len(want) > 0 && want[0].at <= now {
				heap.Pop(&want).(timer).fn()
			}
			if !reflect.DeepEqual(gotCalls, wantCalls) {
				t.Fatalf("seed=%d tick=%d callback order mismatch", seed, now)
			}
		}
		got.advance(1000)
		for len(want) > 0 {
			heap.Pop(&want).(timer).fn()
		}
		if !reflect.DeepEqual(gotCalls, wantCalls) {
			t.Fatalf("seed=%d final order mismatch", seed)
		}
	}
}

func TestTypedTimerCallbackCanSchedule(t *testing.T) {
	var q timers
	var got []int
	q.after(0, 1, func() { got = append(got, 1); q.after(1, 0, func() { got = append(got, 3) }) })
	q.after(0, 1, func() { got = append(got, 2) })
	q.advance(1)
	if !reflect.DeepEqual(got, []int{1, 2}) || q.pending() != 1 {
		t.Fatalf("got=%v pending=%d", got, q.pending())
	}
	q.advance(2)
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatal(got)
	}
}

func BenchmarkTypedTimerBurst(b *testing.B) {
	for _, n := range []int{1, 64, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			q := timers{h: make(timerHeap, 0, n)}
			fn := func() {}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < n; j++ {
					q.after(0, domain.Tick((j*17)%31+1), fn)
				}
				q.advance(32)
			}
		})
	}
}

func BenchmarkTypedTimerFreshBurst(b *testing.B) {
	for _, n := range []int{64, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			fn := func() {}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var q timers
				for j := 0; j < n; j++ {
					q.after(0, domain.Tick((j*17)%31+1), fn)
				}
				q.advance(32)
			}
		})
	}
}

func TestTypedTimerHeapMatchesReferenceOperations(t *testing.T) {
	for seed := int64(0); seed < 32; seed++ {
		r := rand.New(rand.NewSource(seed))
		var got timerHeap
		var want referenceTimerHeap
		gotID, wantID := -1, -1
		pop := func() {
			g, w := got.pop(), heap.Pop(&want).(timer)
			g.fn()
			w.fn()
			if g.at != w.at || gotID != wantID {
				t.Fatalf("seed=%d popped (%d,%d), want (%d,%d)", seed, g.at, gotID, w.at, wantID)
			}
			for i, entry := range got[len(got):cap(got)] {
				if entry.fn != nil || entry.at != 0 {
					t.Fatalf("seed=%d unused slot %d retains timer", seed, len(got)+i)
				}
			}
		}
		for id := 0; id < 4096; id++ {
			id := id
			at := domain.Tick(r.Intn(16)) // Many ties, plus growth and interleaved removal.
			got.push(timer{at: at, fn: func() { gotID = id }})
			heap.Push(&want, timer{at: at, fn: func() { wantID = id }})
			if r.Intn(3) == 0 {
				pop()
			}
			if len(got) != len(want) {
				t.Fatalf("seed=%d length mismatch", seed)
			}
			for i := range got {
				if got[i].at != want[i].at {
					t.Fatalf("seed=%d heap layout mismatch at %d", seed, i)
				}
			}
		}
		for len(got) != 0 {
			pop()
		}
	}
}

func TestTypedTimerNestedDeadlinesMatchReference(t *testing.T) {
	var got timers
	var want referenceTimerHeap
	var gotCalls, wantCalls []int
	var gotSchedule, wantSchedule func(domain.Tick, domain.Tick, int, int)
	gotSchedule = func(now, delay domain.Tick, id, depth int) {
		got.after(now, delay, func() {
			gotCalls = append(gotCalls, id)
			if depth == 0 {
				return
			}
			// Some nested timers are already overdue when advance skips ticks.
			// The large burst also forces backing-array growth inside callbacks.
			for j := 0; j < 80; j++ {
				gotSchedule(now+1, domain.Tick(j%4), id*100+j, depth-1)
			}
		})
	}
	wantSchedule = func(now, delay domain.Tick, id, depth int) {
		d := delay
		if d == 0 {
			d = 1
		}
		heap.Push(&want, timer{at: now + d, fn: func() {
			wantCalls = append(wantCalls, id)
			if depth == 0 {
				return
			}
			for j := 0; j < 80; j++ {
				wantSchedule(now+1, domain.Tick(j%4), id*100+j, depth-1)
			}
		}})
	}
	for id := 1; id <= 4; id++ {
		gotSchedule(0, domain.Tick(id-1), id, 2)
		wantSchedule(0, domain.Tick(id-1), id, 2)
	}
	for _, now := range []domain.Tick{0, 1, 1, 3, 3, 5, 100} {
		got.advance(now)
		for len(want) > 0 && want[0].at <= now {
			heap.Pop(&want).(timer).fn()
		}
		if !reflect.DeepEqual(gotCalls, wantCalls) || got.pending() != len(want) {
			t.Fatalf("tick=%d calls=%d/%d pending=%d/%d", now, len(gotCalls), len(wantCalls), got.pending(), len(want))
		}
	}
	if got.pending() != 0 || len(gotCalls) != 4*(1+80+80*80) {
		t.Fatalf("incomplete drain: calls=%d pending=%d", len(gotCalls), got.pending())
	}
	for i, entry := range got.h[:cap(got.h)] {
		if entry.fn != nil || entry.at != 0 {
			t.Fatalf("drained slot %d retains timer", i)
		}
	}
}
