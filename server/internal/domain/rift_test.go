package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func TestTrialKeyMaximumAndMentorThreshold(t *testing.T) {
	roster := []TrialKeyMember{{1, 60, 5}, {2, 50, 0}, {3, 50, 0}, {4, 50, 0}, {5, 50, 0}}
	if n, e := TrialKeyReward(1, roster); e != nil || n != 14 {
		t.Fatalf("max %d %v", n, e)
	}
	roster[1].Level = 51
	if n, _ := TrialKeyReward(1, roster); n != 13 {
		t.Fatal(n)
	}
	if n, _ := TrialKeyReward(2, roster); n != 5 {
		t.Fatal(n)
	}
}
func TestTrialKeyExhaustiveBounds(t *testing.T) {
	for count := 1; count <= 5; count++ {
		for round := int32(0); round <= 5; round++ {
			var roster []TrialKeyMember
			for i := 0; i < count; i++ {
				roster = append(roster, TrialKeyMember{CharID: CharID(i + 1), Level: 60 - int32(i*10), Cycle: round})
			}
			n, e := TrialKeyReward(1, roster)
			if e != nil || n != 1+int32(count-1)*2+round || n > 14 {
				t.Fatalf("%d %d: %d %v", count, round, n, e)
			}
		}
	}
}
func TestTrialKeyInvalidRoster(t *testing.T) {
	for _, roster := range [][]TrialKeyMember{nil, {{1, 60, 6}}, {{1, 60, 0}, {1, 50, 0}}, {{1, 60, 0}, {2, 0, 0}}} {
		if _, err := TrialKeyReward(1, roster); err == nil {
			t.Fatal(roster)
		}
	}
}
func testRift(t *testing.T) *RiftRun {
	r, e := NewRiftRun("run", 1, 1, []TrialKeyMember{{1, 60, 0}}, 1000)
	if e != nil {
		t.Fatal(e)
	}
	r.Activate(1000)
	return r
}
func TestRiftProgressOrbsBossAndDedupe(t *testing.T) {
	r := testRift(t)
	r.RegisterMonster(1, 2)
	n, ok := r.RecordKill(1, 1100)
	if !ok || n != 4 {
		t.Fatal(n, ok)
	}
	if n, ok = r.RecordKill(1, 1100); ok || n != 0 {
		t.Fatal("doubledeath")
	}
	r.Progress = 9800
	r.Globes[3] = RiftGlobe{ID: 3, Value: 200}
	if !r.CollectGlobe(3, 1) || r.Phase != RiftBoss || r.Progress != 10000 {
		t.Fatal(r)
	}
	if r.CollectGlobe(3, 1) {
		t.Fatal("doubleglobe")
	}
	r.RegisterMonster(5, 3)
	r.Members[1].PresentMS = 1000
	r.Members[1].LastSeenMS = 2000
	r.RecordKill(5, 2000)
	if r.Phase != RiftCompleted || !r.Members[1].Eligible || !r.Timely {
		t.Fatal(r)
	}
	if !r.MarkClaimed(1) || r.MarkClaimed(1) {
		t.Fatal("claim dedupe")
	}
}
func TestRiftTimeoutAndPenalty(t *testing.T) {
	r := testRift(t)
	r.RecordDeath(1)
	if r.PenaltyMS != 5000 {
		t.Fatal(r.PenaltyMS)
	}
	r.Phase = RiftBoss
	r.RegisterMonster(1, 3)
	r.RecordKill(1, 1000+RiftHardMS)
	if r.Phase != RiftFailed {
		t.Fatal(r.Phase)
	}
	r = testRift(t)
	r.Phase = RiftBoss
	r.RegisterMonster(1, 3)
	r.RecordKill(1, 1000+RiftTimedMS)
	if !r.Timely {
		t.Fatal("inclusive timed boundary")
	}
}
func TestRiftLoadingLeaveNoRefund(t *testing.T) {
	r, e := NewRiftRun("run", 1, 1, []TrialKeyMember{{1, 60, 0}}, 1000)
	if e != nil {
		t.Fatal(e)
	}
	r.Leave(1)
	if r.Phase != RiftAborted || r.Activate(2000) {
		t.Fatal(r)
	}
}
func TestRiftCloneAndPersistence(t *testing.T) {
	r := testRift(t)
	clone := r.Clone()
	clone.Members[1].Claimed = true
	if r.Members[1].Claimed {
		t.Fatal("shared member")
	}
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var restored RiftRun
	if e = json.Unmarshal(b, &restored); e != nil || !restored.Admitted(1) {
		t.Fatal(e)
	}
}
func TestRiftRewardAndStatBounds(t *testing.T) {
	p := PlanRiftRewards(50, true)
	if p.Dust != 19 || p.Equipment != 2 || p.Rune != 2 || p.UniqueChance != 60 {
		t.Fatal(p)
	}
	if RiftScaledStat(100, 1, 5, 2, true) != 2720 {
		t.Fatal("elite party HP")
	}
	if RiftScaledStat(2147483647, 50, 5, 3, true) != 2147483647 {
		t.Fatal("overflow")
	}
}

func TestRiftDifficultyTierAdmissionBounds(t *testing.T) {
	roster := []TrialKeyMember{{1, 60, 0}}
	for tier := int32(1); tier <= 150; tier++ {
		r, err := NewRiftRun("run", tier, 1, roster, 1000)
		if err != nil || r == nil || r.Tier != tier {
			t.Fatalf("tier %d: run=%+v err=%v", tier, r, err)
		}
	}
	for _, tier := range []int32{math.MinInt32, -1, 0, 151, math.MaxInt32} {
		if r, err := NewRiftRun("run", tier, 1, roster, 1000); err == nil || r != nil {
			t.Fatalf("invalid difficulty tier %d admitted: %+v", tier, r)
		}
	}
}

func TestRiftScaledStatAllDifficultyTiers(t *testing.T) {
	for party := 1; party <= 5; party++ {
		for class := int32(0); class <= 3; class++ {
			for _, hp := range []bool{false, true} {
				for _, base := range []int32{1, 100, 10000, math.MaxInt32} {
					last := int32(0)
					for tier := int32(1); tier <= 150; tier++ {
						got := RiftScaledStat(base, tier, party, class, hp)
						if got < base || got < last {
							t.Fatalf("tier=%d party=%d class=%d hp=%v base=%d: got=%d previous=%d", tier, party, class, hp, base, got, last)
						}
						last = got
					}
				}
			}
		}
	}
	if got := RiftScaledStat(10000, 150, 5, 3, true); got != math.MaxInt32 {
		t.Fatalf("guardian HP must saturate: %d", got)
	}
	want := int32(math.Ceil(100 * math.Pow(1.04, 149)))
	if got := RiftScaledStat(100, 150, 1, 0, false); got != want {
		t.Fatalf("tier 150 attack=%d want=%d", got, want)
	}
	if RiftScaledStat(100, 150, 1, 0, false) <= RiftScaledStat(100, 50, 1, 0, false) {
		t.Fatal("difficulty above tier 50 stopped scaling")
	}
}

func TestRiftScaledStatClampsInvalidInputs(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, hp := range []bool{false, true} {
		for _, tc := range []struct {
			tier, wantTier   int32
			party, wantParty int
			class, wantClass int32
		}{
			{math.MinInt32, 1, -maxInt - 1, 1, math.MinInt32, 0},
			{0, 1, 0, 1, -1, 0},
			{151, 150, 6, 5, 4, 3},
			{math.MaxInt32, 150, maxInt, 5, math.MaxInt32, 3},
		} {
			got := RiftScaledStat(100, tc.tier, tc.party, tc.class, hp)
			want := RiftScaledStat(100, tc.wantTier, tc.wantParty, tc.wantClass, hp)
			if got != want {
				t.Fatalf("%+v hp=%v: got=%d want=%d", tc, hp, got, want)
			}
		}
		for _, base := range []int32{math.MinInt32, -1, 0} {
			if got := RiftScaledStat(base, 150, 5, 3, hp); got != 0 {
				t.Fatalf("nonpositive base %d: %d", base, got)
			}
		}
	}
}

func TestRiftRewardsDifficultyTierBounds(t *testing.T) {
	for tier := int32(1); tier <= 150; tier++ {
		for _, timely := range []bool{false, true} {
			got := PlanRiftRewards(tier, timely)
			wantDust := 6 + (tier-1)/5
			if timely {
				wantDust += 4
			}
			if got.Dust != wantDust || got.UniqueChance != min(int32(60), 20+2*tier) {
				t.Fatalf("tier %d timely=%v: %+v", tier, timely, got)
			}
		}
	}
	for _, timely := range []bool{false, true} {
		for _, tc := range []struct{ tier, wantTier int32 }{{math.MinInt32, 1}, {0, 1}, {151, 150}, {math.MaxInt32, 150}} {
			if got, want := PlanRiftRewards(tc.tier, timely), PlanRiftRewards(tc.wantTier, timely); got != want {
				t.Fatalf("tier %d timely=%v: got=%+v want=%+v", tc.tier, timely, got, want)
			}
		}
	}
}
