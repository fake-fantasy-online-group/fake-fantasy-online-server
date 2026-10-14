package domain

import "testing"

// Independent review regressions use IDs distinct from the implementation fixtures.
func reviewSuppressor(id StatusID, seconds int32) StatusDef {
	return StatusDef{ID: id, Level: 1, TypeKnown: true, Type: StatusGood, DurationSec: seconds,
		SuppressHarmful: true, BlockHarmful: true, PhysicalDamageReductionPct: 95,
		MagicDamageReductionPct: 95, DamageReductionCapPct: 95}
}

func TestReviewSuppressionOverlappingSourcesAndRefresh(t *testing.T) {
	s := NewStatusSet()
	bad := StatusDef{ID: 899001, TypeKnown: true, Type: StatusBad, DurationSec: 20,
		Control: ControlStun, IntervalSec: 1, PeriodicDamageFraction100: 37}
	s.Apply(bad, 0, 1)
	s.Apply(reviewSuppressor(899002, 5), 0, 1)
	s.Apply(reviewSuppressor(899003, 5), Ticks(3000), 1)
	for ms := 1000; ms <= 7000; ms += 1000 {
		s.ExpireHarmfulSuppression(Ticks(ms))
		if due := s.DueTicks(Ticks(ms)); len(due) != 0 {
			t.Fatalf("suppressed tick at %d: %+v", ms, due)
		}
		if s.Controlled() != ControlNone {
			t.Fatalf("overlapping source released control at %d", ms)
		}
	}
	// Refresh the second instance in place; neither DR nor fractions may compound.
	s.Apply(reviewSuppressor(899003, 5), Ticks(7000), 1)
	if damage, _ := s.MitigateDamage(false, 1000); damage != 50 {
		t.Fatalf("refresh compounded damage: %d", damage)
	}
	for ms := 8000; ms < 12000; ms += 1000 {
		s.ExpireHarmfulSuppression(Ticks(ms))
		if due := s.DueTicks(Ticks(ms)); len(due) != 0 {
			t.Fatalf("refreshed suppression emitted tick at %d", ms)
		}
	}
	s.ExpireHarmfulSuppression(Ticks(12000))
	if s.Controlled() != ControlStun || !s.Has(bad.ID) {
		t.Fatal("remaining negative lifetime did not resume")
	}
	due := s.DueTicks(Ticks(12000))
	if len(due) != 1 || due[0].PeriodicDamageRemainder100 != 37 || due[0].PeriodicDamageTickExtra != 0 {
		t.Fatalf("suppression accumulated fractional damage: %+v", due)
	}
	s.Expire(Ticks(20000))
	if s.Has(bad.ID) {
		t.Fatal("refresh extended the original harmful deadline")
	}
}

func TestReviewSuppressionBoundaryNeverRestoresExpiredDebuff(t *testing.T) {
	for _, seconds := range []int32{5, 6} {
		s := NewStatusSet()
		s.Apply(StatusDef{ID: 899010, TypeKnown: true, Type: StatusBad, DurationSec: seconds, Control: ControlStun}, 0, 1)
		s.Apply(reviewSuppressor(899011, 5), 0, 1)
		s.ExpireHarmfulSuppression(Ticks(7000))
		if s.Controlled() != ControlNone {
			t.Errorf("expired %ds debuff resumed after suppression at t=7s", seconds)
		}
	}
}

func TestReviewSuppressedSiblingSurvivesPositiveOverlayReplacement(t *testing.T) {
	s := NewStatusSet()
	bad := StatusDef{ID: 899020, TypeKnown: true, Type: StatusBad, DurationSec: 20, Control: ControlStun}
	good := StatusDef{ID: bad.ID, TypeKnown: true, Type: StatusGood, DurationSec: 20}
	s.Apply(bad, 0, 1)
	s.ApplyWithOverlay(good, 0, 1, StatusOverlay{{Existing: bad.ID, Incoming: good.ID}: OverlayCoexist})
	s.Apply(reviewSuppressor(899021, 5), 0, 1)
	replacement := StatusDef{ID: 899022, TypeKnown: true, Type: StatusGood, DurationSec: 10}
	result := s.ApplyWithOverlay(replacement, Ticks(1000), 1, StatusOverlay{{Existing: bad.ID, Incoming: replacement.ID}: OverlayReplace})
	if !result.Applied {
		t.Fatal("positive replacement was blocked")
	}
	if !s.Has(bad.ID) {
		t.Fatal("replacing the effective positive instance deleted its suppressed harmful sibling")
	}
	s.ExpireHarmfulSuppression(Ticks(5000))
	if s.Controlled() != ControlStun {
		t.Fatal("suppressed harmful lifetime was lost by overlay replacement")
	}
}
