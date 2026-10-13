package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func TestRiftTierJumpRemainingTimeBoundaries(t *testing.T) {
	for _, tc := range []struct{ remaining, jump int32 }{
		{-1, 1}, {0, 1}, {179, 1}, {180, 1}, {359, 1}, {360, 2}, {540, 3}, {899, 4}, {900, 5},
	} {
		if got := RiftTierJump(tc.remaining); got != tc.jump {
			t.Fatalf("remaining=%d: jump=%d want=%d", tc.remaining, got, tc.jump)
		}
		if got := RiftUnlockedTier(1, 20, tc.remaining); got != 20+tc.jump {
			t.Fatalf("remaining=%d: unlocked=%d want=%d", tc.remaining, got, 20+tc.jump)
		}
	}
	for _, tc := range []struct{ old, tier, remaining, want int32 }{
		{1, 149, 360, 150}, {1, 150, 0, 150}, {150, 1, 900, 150}, {90, 10, 540, 90},
		{0, 1, 0, 2}, {math.MaxInt32, 1, 0, 150}, {1, math.MaxInt32, math.MaxInt32, 150},
	} {
		if got := RiftUnlockedTier(tc.old, tc.tier, tc.remaining); got != tc.want {
			t.Fatalf("%+v: got=%d", tc, got)
		}
	}
}

func TestRiftCompletionFreezesTimeAndPenaltyAcrossPersistence(t *testing.T) {
	for _, remaining := range []int64{0, 179, 180, 359, 360, 540} {
		r := testRift(t)
		r.Tier = 20
		r.Phase = RiftBoss
		r.RegisterMonster(10, 3)
		r.RecordDeath(1)
		completed := r.StartedMS + RiftTimedMS - remaining*1000 - RiftDeathPenaltyMS
		r.ObserveClock(completed, map[CharID]bool{1: true})
		if _, ok := r.RecordKill(10, completed); !ok || !r.Timely {
			t.Fatalf("remaining=%d: completion failed", remaining)
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var restored RiftRun
		if err := json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		for _, phase := range []RiftPhase{RiftCompleted, RiftClosed} {
			restored.Phase = phase
			if got := restored.RemainingSeconds(completed + 24*60*60*1000); got != int32(remaining) {
				t.Fatalf("phase=%s remaining=%d: got=%d", phase, remaining, got)
			}
			want := int32(20) + RiftTierJump(int32(remaining))
			if got := restored.UnlockedTierFor(1, 1); got != want {
				t.Fatalf("remaining=%d: unlocked=%d want=%d", remaining, got, want)
			}
		}
		if restored.RecordDeath(1) || restored.PenaltyMS != RiftDeathPenaltyMS {
			t.Fatal("post-clear death changed frozen penalty")
		}
	}
}

func TestRiftCompletedTeamUnlocksWithoutLeaderClaimOrPersonalTierGate(t *testing.T) {
	r, err := NewRiftRun("party", 20, 1, []TrialKeyMember{{1, 60, 0}, {2, 60, 0}, {3, 60, 0}, {4, 60, 0}}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	r.Activate(1000)
	r.Phase = RiftBoss
	r.RegisterMonster(10, 3)
	r.Leave(3)
	completed := r.StartedMS + RiftTimedMS - 540000
	r.ObserveClock(completed, map[CharID]bool{1: true, 2: true})
	if _, ok := r.RecordKill(10, completed); !ok {
		t.Fatal("completion failed")
	}
	for _, id := range []CharID{1, 2} {
		if got := r.UnlockedTierFor(id, 1); got != 23 {
			t.Fatalf("member %d unlocked=%d", id, got)
		}
		if !r.MarkClaimed(id) || r.UnlockedTierFor(id, 23) != 23 || r.MarkClaimed(id) {
			t.Fatalf("member %d claim replay affected unlock", id)
		}
	}
	for _, id := range []CharID{3, 4, 999} {
		if got := r.UnlockedTierFor(id, 1); got != 1 {
			t.Fatalf("noneligible member %d unlocked=%d", id, got)
		}
	}
}

func TestRiftOvertimeCompletionStillAdvancesButFailureNeverDoes(t *testing.T) {
	r := testRift(t)
	r.Tier = 20
	r.Phase = RiftBoss
	r.RegisterMonster(10, 3)
	completed := r.StartedMS + RiftTimedMS + 1000
	r.ObserveClock(completed, map[CharID]bool{1: true})
	r.RecordKill(10, completed)
	if r.Timely || r.Phase != RiftCompleted || r.RemainingSeconds(completed) != 0 || r.UnlockedTierFor(1, 1) != 21 {
		t.Fatalf("overtime minimum missing: %+v", r)
	}
	for _, phase := range []RiftPhase{RiftReserved, RiftActive, RiftBoss, RiftFailed, RiftAborted} {
		r.Phase = phase
		if got := r.UnlockedTierFor(1, 1); got != 1 {
			t.Fatalf("phase=%s granted %d", phase, got)
		}
	}
	r.Phase = RiftCompleted
	r.CompletedMS = 0 // Older records without a timestamp cannot invent bonus time.
	if r.RemainingSeconds(completed) != 0 || r.UnlockedTierFor(1, 1) != 21 {
		t.Fatal("legacy timestamp fallback must award only the clear minimum")
	}
}
