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
	// The wire carries integer coordinates and the client reports its landing
	// as integers; land on whole pixels so that echo equals the server position.
	to.X, to.Y = math.Round(to.X), math.Round(to.Y)
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
	if e.Player != nil {
		if s.dashLanding == nil {
			s.dashLanding = map[domain.EntityID]dashLandingGuard{}
		}
		s.dashLanding[e.ID] = dashLandingGuard{from: from, to: e.Pos, until: s.tick + domain.Ticks(int(duration)+dashReportGraceMS)}
	}
	// Everyone, including the dasher, receives the authoritative dash; the
	// dasher's client did not predict the server-chosen landing point.
	s.emit(event.EntityMoved{ID: e.ID, To: to, Dir: dir, Stop: true, Snap: true, Dash: true, DurationMS: duration})
	return true
}

// dashReportGraceMS covers reports the client sent before it received the
// dash (network round trip) on top of the dash duration itself.
const dashReportGraceMS = 1500

// dashLandingGuard remembers a player's last skill dash until the client has
// caught up with the server-authored landing point.
type dashLandingGuard struct {
	from, to domain.Pos
	until    domain.Tick
}

// filterClientReport decides what to do with a real client position report
// (0x1017/0x100a, ReceivedAt set). It returns false when the report must be
// ignored without side effects:
//   - a report equal to the current position (the client echoes the dash
//     landing once it finishes interpolating) is not a move: it must not
//     spend speed credit, re-trigger portals, stop resting, re-broadcast a
//     walk to observers (which would cut their dash interpolation short) or
//     reset the facing chosen by the dash;
//   - inside the grace window after a dash, a report nearer the pre-dash
//     origin than the landing point is a stale pre-dash report and would
//     otherwise drag the player back while their own client stays at the
//     landing point.
func (s *Scene) filterClientReport(e *entity.Entity, cmd MoveTo) bool {
	if e.Player == nil || cmd.ReceivedAt.IsZero() {
		return true
	}
	echo := e.Pos.X == cmd.To.X && e.Pos.Y == cmd.To.Y && e.Pos.MapID == cmd.To.MapID
	if g, ok := s.dashLanding[e.ID]; ok {
		switch {
		case s.tick >= g.until || e.Pos != g.to:
			delete(s.dashLanding, e.ID)
		case echo:
			delete(s.dashLanding, e.ID)
		case math.Hypot(cmd.To.X-g.from.X, cmd.To.Y-g.from.Y) < math.Hypot(cmd.To.X-g.to.X, cmd.To.Y-g.to.Y):
			return false
		default:
			delete(s.dashLanding, e.ID)
		}
	}
	return !echo
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
