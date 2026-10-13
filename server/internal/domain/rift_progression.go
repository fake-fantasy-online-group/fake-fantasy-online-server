package domain

// RiftTierJump advances difficulty tiers, not the map-floor index within a run.
// Every successful clear advances at least one tier, including an overtime
// clear; only the extra tiers depend on the frozen remaining timed seconds.
func RiftTierJump(remainingSeconds int32, rule *RiftRules) int32 {
	if rule == nil {
		return 0
	}
	return max(int32(1), remainingSeconds/rule.TierJumpSeconds)
}

func RiftUnlockedTier(previous, completedTier, remainingSeconds int32, rule *RiftRules) int32 {
	if rule == nil {
		return 0
	}
	previous = clampRiftTier(previous, rule)
	completedTier = clampRiftTier(completedTier, rule)
	return max(previous, int32(min(int64(rule.MaxTier), int64(completedTier)+int64(RiftTierJump(remainingSeconds, rule)))))
}

// RemainingSeconds freezes successful completion at the authoritative guardian
// death timestamp, so looting, claiming, reconnecting and closing cannot shrink
// the earned tier jump. Older incomplete records cannot invent saved time.
func (r *RiftRun) RemainingSeconds(now int64) int32 {
	if r == nil || r.Rules == nil || r.StartedMS <= 0 || r.Phase == RiftReserved {
		return 0
	}
	if r.Phase == RiftCompleted || r.Phase == RiftClosed {
		if r.CompletedMS <= 0 {
			return 0
		}
		now = r.CompletedMS
	} else if r.Terminal() {
		return 0
	}
	elapsed := max(int64(0), now-r.StartedMS)
	remaining := r.Rules.TimedMS - elapsed - max(int64(0), r.PenaltyMS)
	return int32(max(int64(0), remaining) / 1000)
}

// UnlockedTierFor reuses the run's existing completion eligibility. It neither
// requires a timely clear nor adds a personal-tier/leader/loot-claim gate.
func (r *RiftRun) UnlockedTierFor(char CharID, previous int32) int32 {
	if r == nil || r.Rules == nil {
		return previous
	}
	rule := r.Rules
	previous = clampRiftTier(previous, rule)
	if r == nil || (r.Phase != RiftCompleted && r.Phase != RiftClosed) {
		return previous
	}
	m := r.Members[char]
	if m == nil || !m.Eligible || m.Departed {
		return previous
	}
	return RiftUnlockedTier(previous, r.Tier, r.RemainingSeconds(r.CompletedMS), rule)
}
