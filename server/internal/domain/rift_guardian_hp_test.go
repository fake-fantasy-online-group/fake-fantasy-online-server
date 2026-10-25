package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func TestRiftGuardianHPFinalReductionAllTiers(t *testing.T) {
	old := riftRulesFixture()
	old.StatAnchorTier = 30
	fresh := cloneRiftRules(*old)
	fresh.Version, fresh.GuardianHPDivisor = 13, 10
	for _, base := range []int32{1, 2, 9, 10, 11, 1009, 29748, math.MaxInt32} {
		for party := 1; party <= 5; party++ {
			for tier := int32(1); tier <= 150; tier++ {
				for class := int32(0); class <= 3; class++ {
					for _, hp := range []bool{false, true} {
						want := RiftScaledStat(base, tier, party, class, hp, old)
						if hp && class == 3 {
							want = max(int32(1), want/10)
						}
						if got := RiftScaledStat(base, tier, party, class, hp, &fresh); got != want {
							t.Fatalf("base%d tier%d party%d class%d hp%v: got%d want%d", base, tier, party, class, hp, got, want)
						}
					}
				}
			}
		}
	}
	// Reduce the already wire-saturated HP, not its unbounded float intermediate.
	if got := RiftScaledStat(math.MaxInt32, 150, 5, 3, true, &fresh); got != math.MaxInt32/10 {
		t.Fatal("saturation order", got)
	}
	if got := RiftScaledStat(1, 1, 1, 3, true, &fresh); got != 1 {
		t.Fatal("minimum HP", got)
	}
	// Invalid source stats remain invalid, rather than being promoted into a boss.
	for _, base := range []int32{math.MinInt32, -1, 0} {
		if got := RiftScaledStat(base, 150, 5, 3, true, &fresh); got != 0 {
			t.Fatal("invalid base changed", base, got)
		}
	}
	raw, _ := json.Marshal(old)
	var saved RiftRules
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.GuardianHPDivisor != 0 || RiftScaledStat(1009, 30, 5, 3, true, &saved) != RiftScaledStat(1009, 30, 5, 3, true, old) {
		t.Fatal("legacy JSON changed scaling")
	}
}
