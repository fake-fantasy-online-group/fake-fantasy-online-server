package domain

import (
	"math"
	"testing"
)

func TestTheftDropSelectionUsesExistingRelativeWeightsOnce(t *testing.T) {
	entries := []DropEntry{{Item: 1, RatePct: 31.4}, {Item: 2, RatePct: 5.7}, {Item: 3, RatePct: 0.00045}}
	var calls int
	for _, tc := range []struct {
		roll int64
		want ItemID
	}{{0, 1}, {31_399_999, 1}, {31_400_000, 2}, {37_099_999, 2}, {37_100_000, 3}, {37_100_449, 3}} {
		got, ok := PickTheftDrop(entries, func(n int64) int64 {
			calls++
			if n != 37_100_450 {
				t.Fatalf("weight total=%d", n)
			}
			return tc.roll
		})
		if !ok || got.Item != tc.want {
			t.Fatalf("roll%d: got%+v ok%v want%d", tc.roll, got, ok, tc.want)
		}
	}
	if calls != 6 {
		t.Fatalf("selection rerolled probability: %d calls", calls)
	}
}

func TestTheftDropSelectionRejectsMalformedRates(t *testing.T) {
	for _, rate := range []float64{0, -1, 101, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, ok := PickTheftDrop([]DropEntry{{Item: 1, RatePct: rate}}, func(int64) int64 { t.Fatal("empty pool rolled"); return 0 }); ok {
			t.Fatalf("accepted invalid rate %v", rate)
		}
	}
	if _, ok := PickTheftDrop([]DropEntry{{Item: 1, RatePct: 1}}, nil); ok {
		t.Fatal("accepted nil random source")
	}
	for _, roll := range []int64{-1, 1_000_000} {
		if _, ok := PickTheftDrop([]DropEntry{{Item: 1, RatePct: 1}}, func(int64) int64 { return roll }); ok {
			t.Fatal("accepted out-of-range random result")
		}
	}
}
