package scene

import "testing"

func TestTimerPopClearsExpiredClosures(t *testing.T) {
	var timers timers
	calls := 0
	for i := 0; i < 64; i++ {
		timers.after(0, 1, func() { calls++ })
	}
	timers.advance(1)
	if calls != 64 || timers.pending() != 0 {
		t.Fatalf("calls=%d pending=%d", calls, timers.pending())
	}
	for i, entry := range timers.h[:cap(timers.h)] {
		if entry.fn != nil {
			t.Fatalf("expired closure retained in slot %d", i)
		}
	}
}
