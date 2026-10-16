package entity

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// CombatAttributes is a player's derived runtime attribute entity. Scene
// admission initializes it before exposing the player; equipment, progression,
// status and pet changes rebuild it through the same attribute pipeline.
// It is owned by Player, never saved or transferred: destination admission
// constructs its own entity after restoring authoritative inputs.
//
// Stats/Growth are deterministic pre-limit values. Entity.Stats retains the
// effective combat values after live status floors/caps. HP/MP, XP, targets and
// random/proc decisions are deliberately not part of this derived state.
// Consumers use scene accessors so dependency changes cannot expose stale data.
type CombatAttributes struct {
	Ready          bool
	Revision       uint64
	FunctionalWorn *domain.EquipSet
	RawSkills      domain.Learned
	Base           domain.Base
	Stats, Growth  domain.Stats
}
