package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// The durable run is the completion entitlement; the character profile is its
// monotone projection. Claim and leave commit this alongside their normal
// receipt, so neither loot capacity nor late settlement can reduce progression.
func riftProfileWithCompletion(payload string, run *domain.RiftRun, char domain.CharID) endgameRiftProfile {
	profile := riftProfile(payload)
	profile.UnlockedTier = run.UnlockedTierFor(char, profile.UnlockedTier)
	return profile
}
