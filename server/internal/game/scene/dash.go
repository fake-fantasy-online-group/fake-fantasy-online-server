package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Same-map skill displacement is a dash, not a map reload: the player keeps
// the loaded scene, observers interpolate over 150-250ms, and the dasher is
// immune to control and displacement for that window.
const (
	dashMinMS = 150
	dashMaxMS = 250
	// dashFullDistance is the distance at which a dash takes dashMaxMS.
	dashFullDistance = 400.0
)

func dashDurationMS(from, to domain.Pos) int32 {
	d := math.Hypot(to.X-from.X, to.Y-from.Y)
	ms := dashMinMS + int32(d*(dashMaxMS-dashMinMS)/dashFullDistance)
	return min(max(ms, dashMinMS), dashMaxMS)
}

// skillDash moves e to a walkable same-map point. It returns false when the
// move was not performed; the caller must not apply arrival effects then.
func (s *Scene) skillDash(e *entity.Entity, to domain.Pos, dir uint8) bool {
	if e == nil || !e.Alive() || !s.spawnPositionWalkable(to) {
		return false
	}
	if s.mapLoads[e.ID] != nil {
		// A client still loading keeps the authoritative queued-teleport path.
		return s.onTeleport(Teleport{ID: e.ID, To: s.id, At: to, Dir: dir})
	}
	s.endSharedRide(e)
	s.interruptWork(e)
	// Same discontinuity rule as a server teleport: queued actions admitted at
	// the old position must not resolve from the landing point.
	combatants := []domain.EntityID{e.ID}
	if e.Player != nil && e.Player.Pet != 0 {
		combatants = append(combatants, e.Player.Pet)
	}
	s.cancelPendingCombat(combatants...)
	from := e.Pos
	duration := dashDurationMS(from, to)
	s.blockPortalLanding(e.ID, to)
	s.move(MoveTo{ID: e.ID, To: to, Dir: dir, Stop: true}, false, true)
	if e.Pos == from {
		return false
	}
	if s.dashImmuneUntil == nil {
		s.dashImmuneUntil = map[domain.EntityID]domain.Tick{}
	}
	s.dashImmuneUntil[e.ID] = s.tick + domain.Ticks(int(duration))
	// Everyone, including the dasher, receives the authoritative dash; the
	// dasher's client did not predict the server-chosen landing point.
	s.emit(event.EntityMoved{ID: e.ID, To: to, Dir: dir, Stop: true, Snap: true, Dash: true, DurationMS: duration})
	return true
}

// dashControlImmune reports whether e is inside its dash window.
func (s *Scene) dashControlImmune(e *entity.Entity) bool {
	if e == nil || s.dashImmuneUntil == nil {
		return false
	}
	until, ok := s.dashImmuneUntil[e.ID]
	if !ok {
		return false
	}
	if s.tick >= until {
		delete(s.dashImmuneUntil, e.ID)
		return false
	}
	return true
}

// dashBlocksStatus rejects harmful control (stun/freeze/silence/root and the
// like) while dashing. Damage and non-control debuffs still apply.
func (s *Scene) dashBlocksStatus(e *entity.Entity, def domain.StatusDef) bool {
	if !isHarmful(def) || def.Control == 0 && !def.BlockMove {
		return false
	}
	return s.dashControlImmune(e)
}
