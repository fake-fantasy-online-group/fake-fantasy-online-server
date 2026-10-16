package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// equipmentRequirementResolution belongs to one synchronous evaluation of one
// actor. Every recursive preview only removes slots from that evaluation's worn
// set; a slot therefore identifies exactly one immutable stack in this context.
// Never retain this context on Scene/Entity or reuse it after any input changes.
// Memoization preserves the original admission order and prerequisite checks,
// including revalidation of each smaller set for passive soft-rank sources.
// With n worn slots each table has at most 2^n entries (usually only n prefixes),
// replacing repeated recursive expansion with one evaluation per subset.
type equipmentRequirementResolution struct {
	plainRanks                  domain.SkillResolutionSnapshot
	plainRanksReady             bool
	bases                       map[uint32]domain.Base
	worn                        map[uint32]*domain.EquipSet
	raw                         map[uint32]domain.Learned
	baseHits, wornHits, rawHits int
}

func newEquipmentRequirementResolution() *equipmentRequirementResolution {
	return &equipmentRequirementResolution{
		bases: make(map[uint32]domain.Base),
		worn:  make(map[uint32]*domain.EquipSet),
		raw:   make(map[uint32]domain.Learned),
	}
}

func equipmentRequirementMask(worn *domain.EquipSet) (mask uint32) {
	worn.Each(func(slot domain.EquipSlot, _ domain.Stack) {
		mask |= uint32(1) << uint32(slot)
	})
	return mask
}
