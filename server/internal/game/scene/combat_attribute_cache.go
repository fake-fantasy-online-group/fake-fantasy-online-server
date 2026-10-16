package scene

import (
	"maps"
	"slices"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// combatAttributeDependencies tracks the inputs of the player's own runtime
// attribute entity. The derived values live on Player.CombatAttributes; this
// scene-local record only detects changes and is discarded on departure.
// Equipment owns its revision; learned ranks and effective affixes are compared
// by value. Catalog tables are immutable for the scene's lifetime.
type combatAttributeDependencies struct {
	owner       *entity.Entity
	player      *entity.Player
	character   *domain.Character
	key         combatAttributeKey
	hard        domain.Learned
	status, pet []domain.Affix
	values      *entity.CombatAttributes
}

type combatAttributeKey struct {
	worn                      *domain.EquipSet
	revision                  uint64
	base                      domain.Base
	level, moveSpeed          int32
	race                      domain.Race
	employed, employmentKnown bool
	gender                    uint8
}

func (s *Scene) combatAttributeState(p *entity.Entity) *combatAttributeDependencies {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return nil
	}
	// A copied player used for a requirement trial must not replace its owner's
	// cache, even though it intentionally carries the same network entity ID.
	if owner := s.players[p.ID]; owner != nil && owner != p {
		return nil
	}
	ch := p.Player.Char
	key := combatAttributeKey{worn: p.Player.Worn, revision: p.Player.Worn.EffectsRevision(),
		base: ch.EffectiveBase(), level: ch.Level, moveSpeed: characterBaseMoveSpeed(ch),
		race: ch.Race, employed: ch.Employed, employmentKnown: ch.EmploymentKnown, gender: ch.Appear.Gender}
	status, pet := p.Status.Affixes(), s.petStatAffixes(p)
	// StatusSet iteration is map-ordered. Compare a canonical multiset without
	// changing the original stat pipeline's arithmetic or status ordering.
	sort.Slice(status, func(i, j int) bool {
		a, b := status[i], status[j]
		if a.Attr != b.Attr {
			return a.Attr < b.Attr
		}
		if a.Mode != b.Mode {
			return a.Mode < b.Mode
		}
		return a.Value < b.Value
	})
	old := s.combatAttributes[p.ID]
	if old != nil && old.owner == p && old.player == p.Player && old.character == ch && old.values == p.Player.CombatAttributes && old.key == key &&
		maps.Equal(old.hard, ch.Skills) && slices.Equal(old.status, status) && slices.Equal(old.pet, pet) {
		return old
	}
	if old != nil {
		// The legacy recursive raw-rank memo also depends on status/pet eligibility.
		delete(s.skillRawCache, p.ID)
	}
	values := p.Player.CombatAttributes
	if values == nil || old != nil && (old.owner != p || old.player != p.Player || old.character != ch) {
		values = &entity.CombatAttributes{}
		p.Player.CombatAttributes = values
	}
	values.Ready = false
	out := &combatAttributeDependencies{owner: p, player: p.Player, character: ch, key: key,
		hard: ch.Skills.Clone(), status: status, pet: pet, values: values}
	if s.combatAttributes == nil {
		s.combatAttributes = make(map[domain.EntityID]*combatAttributeDependencies)
	}
	s.combatAttributes[p.ID] = out
	return out
}

func (s *Scene) invalidateCombatAttributes(p *entity.Entity) {
	if p == nil {
		return
	}
	if p.Player != nil && p.Player.CombatAttributes != nil {
		p.Player.CombatAttributes.Ready = false
	}
	delete(s.combatAttributes, p.ID)
	delete(s.skillRawCache, p.ID)
	if s.equipment != nil {
		delete(s.equipment.cache, p.ID)
	}
}

func (s *Scene) combatGrowthStats(p *entity.Entity) (domain.Base, domain.Stats, domain.Stats) {
	state := s.combatAttributeState(p)
	if state == nil {
		return s.computeSkillGrowthStats(p, s.functionalWorn(p))
	}
	s.prepareCombatAttributes(p, state)
	return state.values.Base, state.values.Stats, state.values.Growth
}

// Populate the immutable actor-local bundle in one synchronous evaluation. The
// subset context is discarded before returning: it is never held on the cache.
// This pays a little extra on a cold eligibility-only read, but avoids rebuilding
// the same prerequisite graph for raw ranks, growth and the first XP snapshot.
func (s *Scene) prepareCombatAttributes(p *entity.Entity, state *combatAttributeDependencies) {
	if state.values.Ready {
		return
	}
	resolution := newEquipmentRequirementResolution()
	state.values.FunctionalWorn = s.functionalWornResolved(p, resolution)
	state.values.RawSkills = s.rawSkillLevelsResolved(p, resolution)
	state.values.Base, state.values.Stats, state.values.Growth = s.computeSkillGrowthStatsResolved(p, state.values.FunctionalWorn, resolution)
	state.values.Revision++
	state.values.Ready = true
}

// Admission is an explicit construction boundary, not the first attack. A new
// player owns a fresh derived entity, rebuilt after runtime/status restoration.
func (s *Scene) initializeCombatAttributes(p *entity.Entity) {
	s.invalidateCombatAttributes(p)
	p.Player.CombatAttributes = &entity.CombatAttributes{}
	s.prepareCombatAttributes(p, s.combatAttributeState(p))
}
