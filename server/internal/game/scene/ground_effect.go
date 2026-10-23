package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

type groundEffectKey struct {
	owner domain.EntityID
	key   string
}
type activeGroundEffect struct {
	event   event.GroundEffect
	started domain.Tick
}

// Presentation deadlines reuse the scene wheel. They never create gameplay ticks.
func (s *Scene) trackGroundEffect(e event.GroundEffect) {
	k := groundEffectKey{e.Owner, e.Key}
	delete(s.groundEffects, k)
	if e.Mode != 1 || e.MapID != s.id.MapID || e.Owner == 0 || e.Key == "" || e.Effect == "" || e.DurationMS < 0 {
		return
	}
	if s.groundEffects == nil {
		s.groundEffects = map[groundEffectKey]*activeGroundEffect{}
	}
	a := &activeGroundEffect{event: e, started: s.tick}
	s.groundEffects[k] = a
	if e.DurationMS > 0 {
		s.timer.after(s.tick, domain.Ticks(int(e.DurationMS)), func() {
			if s.groundEffects[k] != a {
				return
			}
			delete(s.groundEffects, k)
			stop := a.event
			stop.Mode = 2
			stop.Effect = ""
			stop.DurationMS = 0
			s.emit(stop)
		})
	}
}
func (s *Scene) replayGroundEffects(to domain.EntityID) {
	p := s.players[to]
	if p == nil || p.Player == nil || !p.Player.EndgameGroundFX || s.mapLoads[to] != nil {
		return
	}
	// This server's visibility boundary is the whole map, matching sendAllEntities.
	keys := make([]groundEffectKey, 0, len(s.groundEffects))
	for k := range s.groundEffects {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].owner != keys[j].owner {
			return keys[i].owner < keys[j].owner
		}
		return keys[i].key < keys[j].key
	})
	for _, k := range keys {
		a := s.groundEffects[k]
		e := a.event
		if e.DurationMS > 0 {
			remaining := int64(e.DurationMS) - (s.tick - a.started).Millis()
			if remaining <= 0 {
				delete(s.groundEffects, k)
				continue
			}
			e.DurationMS = int32(remaining)
		}
		s.emitTo(to, e) // Never emit(): replay must not reset the original deadline.
	}
}
func (s *Scene) removeOwnerGroundEffects(owner domain.EntityID) {
	for k, a := range s.groundEffects {
		if k.owner == owner {
			delete(s.groundEffects, k)
			e := a.event
			e.Mode = 2
			e.Effect = ""
			e.DurationMS = 0
			s.emit(e)
		}
	}
}
