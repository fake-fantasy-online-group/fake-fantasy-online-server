package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

// statusVisual uses optional database presentation metadata. An unconfigured
// status keeps its original name and anchor; the runtime identity and mechanics
// are never changed. Legacy self clients share the HUD title and aura name.
func (s *Scene) statusVisual(def domain.StatusDef) (effect, buffName string, position uint8) {
	effect, buffName, position = def.Name, def.Name, statusPositionMode(def)
	if rule, ok := s.statusVisuals.Get(def.ID, def.Name); ok {
		effect = rule.EffectName
		if rule.UseEffectNameForSelf {
			buffName = rule.EffectName
		}
		if rule.PositionMode != domain.StatusVisualPositionInherit {
			position = uint8(rule.PositionMode)
		}
	}
	return
}

// statusWorldEffects coalesces only the name-keyed visual channel, never the
// independent Buff/icon/status rows. Its deadline is the last contributor's
// deadline, not the last row's deadline. Equipment keys stay separate.
func statusWorldEffects(statuses []statusPresentation) []event.EntityStatusEffect {
	var out []event.EntityStatusEffect
	index := make(map[string]int, len(statuses))
	for _, status := range statuses {
		effect := status.effect
		if effect.Effect == "" {
			continue
		}
		if i, ok := index[effect.Effect]; ok {
			if effect.DurationMS > out[i].DurationMS {
				out[i].DurationMS = effect.DurationMS
			}
			continue
		}
		index[effect.Effect] = len(out)
		out = append(out, effect)
	}
	return out
}

// hasStatusVisual does not make name matching less strict: it identifies which
// identities require actual publication history rather than a lowest-rank guess.
func (s *Scene) hasStatusVisual(id domain.StatusID) bool {
	for key, rule := range s.statusVisuals {
		if key.ID == id && rule.EffectName != "" {
			return true
		}
	}
	return false
}

func statusEffectsByID(statuses []statusPresentation) map[domain.StatusID][]event.EntityStatusEffect {
	out := make(map[domain.StatusID][]event.EntityStatusEffect)
	for _, st := range statuses {
		id := domain.StatusID(st.icon.Status)
		out[id] = append(out[id], st.effect)
	}
	return out
}

// Only clear a key that was really published and has no remaining contributor.
// Zero-duration packets are not harmless probes: old clients briefly instantiate
// unknown keys. The full snapshot below refreshes all surviving shared keys.
func (s *Scene) reconcileStatusVisuals(e *entity.Entity, statuses []statusPresentation) {
	if e == nil || (e.Kind != domain.KindMonster && e.Kind != domain.KindPet && (e.Kind != domain.KindPlayer || e.Player == nil)) {
		return
	}
	remaining := make(map[string]bool)
	for _, st := range statuses {
		remaining[st.effect.Effect] = true
	}
	removed := make(map[string]event.EntityStatusEffect)
	for id, effects := range e.PublishedStatusEffects {
		if !s.hasStatusVisual(id) {
			continue
		}
		for _, effect := range effects {
			if effect.Effect != "" && !remaining[effect.Effect] {
				effect.DurationMS = 0
				removed[effect.Effect] = effect
			}
		}
	}
	names := make([]string, 0, len(removed))
	for name := range removed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s.emitExcept(removed[name], e.ID)
	}
	e.PublishedStatusEffects = statusEffectsByID(statuses)
}

func (s *Scene) configuredStatusKeyWasPublished(e *entity.Entity, name string) bool {
	for id, effects := range e.PublishedStatusEffects {
		if !s.hasStatusVisual(id) {
			continue
		}
		for _, effect := range effects {
			if effect.Effect == name {
				return true
			}
		}
	}
	return false
}
