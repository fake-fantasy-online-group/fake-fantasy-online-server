package domain

import (
	"fmt"
	"reflect"
	"testing"
)

func TestStatusSuppressionFastPathNoSourcePreservesFinalTicks(t *testing.T) {
	for _, blockOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("blockOnly=%v", blockOnly), func(t *testing.T) {
			s := NewStatusSet()
			def := StatusDef{ID: 830001, TypeKnown: true, Type: StatusBad, IntervalSec: 1, DurationSec: 1, PeriodicDamageFraction100: 75}
			s.Apply(def, 0, 1)
			if blockOnly {
				s.Apply(StatusDef{ID: 830002, BlockHarmful: true}, 0, 1)
			}
			before := s.act[def.ID][0]
			now := Ticks(1000)
			s.discardSuppressedTicks(now)
			if gone := s.ExpireHarmfulSuppression(now); gone != nil {
				t.Fatalf("unexpected removals: %v", gone)
			}
			if got := s.act[def.ID][0]; !reflect.DeepEqual(got, before) {
				t.Fatalf("no source changed status: got %+v, want %+v", got, before)
			}
			due := s.DueTicks(now)
			if len(due) != 1 || due[0].Def.ID != def.ID || due[0].PeriodicDamageRemainder100 != 75 {
				t.Fatalf("ordinary final tick lost: %+v", due)
			}
			if gone := s.Expire(now); len(gone) != 1 || gone[0] != def.ID {
				t.Fatalf("ordinary final expiry: %v", gone)
			}
		})
	}
	var nilSet *StatusSet
	nilSet.discardSuppressedTicks(1)
	if got := nilSet.ExpireHarmfulSuppression(1); got != nil {
		t.Fatal(got)
	}
	if got := NewStatusSet().ExpireHarmfulSuppression(1); got != nil {
		t.Fatal(got)
	}
}

func TestStatusSuppressionFastPathBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		next, now Tick
		permanent bool
		wantNext  Tick
		wantGone  bool
	}{
		{"before end", Ticks(4000), Ticks(4000), false, Ticks(5000), false},
		{"exclusive end", Ticks(5000), Ticks(5000), false, Ticks(5000), true},
		{"late suppressed receipt", Ticks(4000), Ticks(7000), false, Ticks(8000), true},
		{"permanent", Ticks(5000), Ticks(7000), true, Ticks(8000), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStatusSet()
			bad := Active{Def: StatusDef{ID: 830010, TypeKnown: true, Type: StatusBad, IntervalSec: 1}, NextTickAt: tc.next, ExpireAt: Ticks(20000), PeriodicDamageRemainder100: 75, PeriodicDamageTickExtra: 1}
			positive := Active{Def: StatusDef{ID: 830011, TypeKnown: true, Type: StatusGood, IntervalSec: 1}, NextTickAt: tc.next, ExpireAt: Ticks(20000)}
			end := Ticks(5000)
			if tc.permanent {
				end = 0
			}
			s.act[bad.Def.ID] = []Active{bad}
			s.act[positive.Def.ID] = []Active{positive}
			s.act[830012] = []Active{{Def: StatusDef{ID: 830012, SuppressHarmful: true}, ExpireAt: end}}
			gone := s.ExpireHarmfulSuppression(tc.now)
			if (len(gone) == 1 && gone[0] == 830012) != tc.wantGone || len(gone) > 1 {
				t.Fatalf("source expiry: %v", gone)
			}
			got := s.act[bad.Def.ID][0]
			want := bad
			want.NextTickAt = tc.wantNext
			if tc.wantNext != tc.next {
				want.PeriodicDamageTickExtra = 0
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("harmful state: got %+v, want %+v", got, want)
			}
			if !reflect.DeepEqual(s.act[positive.Def.ID][0], positive) {
				t.Fatal("positive periodic state changed")
			}
		})
	}
}

func TestStatusSuppressionFastPathExpiredHarmfulFinalTick(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprintf("permanent=%v", permanent), func(t *testing.T) {
			s := NewStatusSet()
			s.Apply(StatusDef{ID: 830020, TypeKnown: true, Type: StatusBad, DurationSec: 5, IntervalSec: 1}, 0, 1)
			s.Apply(StatusDef{ID: 830021, TypeKnown: true, Type: StatusGood, DurationSec: 5, IntervalSec: 1}, 0, 1)
			duration := int32(5)
			if permanent {
				duration = 0
			}
			s.Apply(StatusDef{ID: 830022, SuppressHarmful: true, DurationSec: duration}, 0, 1)
			s.ExpireHarmfulSuppression(Ticks(5000))
			if s.Has(830020) {
				t.Fatal("expired harmful state survived special expiry")
			}
			due := s.DueTicks(Ticks(5000))
			if len(due) != 1 || due[0].Def.ID != 830021 {
				t.Fatalf("positive final tick lost or harmful tick replayed: %+v", due)
			}
			s.Expire(Ticks(5000))
			if s.Has(830021) || s.Has(830022) != permanent {
				t.Fatal("incorrect final expiry")
			}
		})
	}
}

var statusFrameBenchmarkSink bool

func BenchmarkStatusFrameFastPath(b *testing.B) {
	for _, n := range []int{1, 5, 32} {
		for _, suppression := range []bool{false, true} {
			b.Run(fmt.Sprintf("n%d/suppress%v", n, suppression), func(b *testing.B) {
				s := NewStatusSet()
				for i := 0; i < n; i++ {
					s.Apply(StatusDef{ID: StatusID(810000 + i), DurationSec: 30000, TypeKnown: true, Type: StatusBad}, 0, 1)
				}
				if suppression {
					s.Apply(StatusDef{ID: 820000, DurationSec: 30000, SuppressHarmful: true}, 0, 1)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					s.ExpireHarmfulSuppression(1)
					s.DueTicks(1)
					statusFrameBenchmarkSink = s.Invisible()
					s.Expire(1)
				}
			})
		}
	}
}

func BenchmarkStatusSuppressionPerReceiptFastPath(b *testing.B) {
	s := NewStatusSet()
	for i := 0; i < 32; i++ {
		s.Apply(StatusDef{ID: StatusID(810000 + i), DurationSec: 30000}, 0, 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.ExpireHarmfulSuppression(1)
	}
}
