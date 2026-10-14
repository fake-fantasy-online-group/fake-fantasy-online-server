package scene

import (
	"math"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	// Actors are points in the legacy movement protocol. Both prediction and
	// authority expand each wall's local axes by this body clearance.
	temporaryWallActorClearance = 12.0
	temporaryWallSpawnClearance = 40.0
	temporaryWallMinimumGap     = 80.0
	maxTemporaryWalls           = 96
)

// temporaryWall is a flat-ended oriented rectangle, not a capsule. X/Y and
// X2/Y2 are the endpoints of its centre line; Width is the full physical width.
// All coordinates retain their precision in JSON. ID and Owner identify a
// specific effect generation, never a reusable entity ID. Expiry is exclusive.
// Collision expands BOTH rectangle axes by temporaryWallActorClearance. This
// contract must also be used by client prediction; it does not affect LOS.
type temporaryWall struct {
	ID          string  `json:"id"`
	Owner       string  `json:"owner"`
	MapID       int32   `json:"mapId"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	X2          float64 `json:"x2"`
	Y2          float64 `json:"y2"`
	Width       float64 `json:"width"`
	ExpiresAtMS int64   `json:"expiresAtMs"`
}

func finiteTemporaryCoordinate(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (w temporaryWall) valid(mapID int32, now int64) bool {
	if w.ID == "" || w.Owner == "" || w.MapID != mapID || w.ExpiresAtMS <= now {
		return false
	}
	for _, v := range []float64{w.X, w.Y, w.X2, w.Y2, w.Width} {
		if !finiteTemporaryCoordinate(v) || math.Abs(v) > 1e7 {
			return false
		}
	}
	length := math.Hypot(w.X2-w.X, w.Y2-w.Y)
	return w.Width > 0 && w.Width <= 1024 && length > 0 && length <= 2048
}

func (w temporaryWall) local(p domain.Pos) (float64, float64) {
	dx, dy := w.X2-w.X, w.Y2-w.Y
	length := math.Hypot(dx, dy)
	px, py := p.X-(w.X+w.X2)/2, p.Y-(w.Y+w.Y2)/2
	return (px*dx + py*dy) / length, (-px*dy + py*dx) / length
}

func (w temporaryWall) pointBlocked(p domain.Pos, padding float64) bool {
	if p.MapID != w.MapID {
		return false
	}
	x, y := w.local(p)
	return math.Abs(x) <= math.Hypot(w.X2-w.X, w.Y2-w.Y)/2+padding && math.Abs(y) <= w.Width/2+padding
}

// Slab clipping is continuous: a fast move or diagonal grazing a corner
// cannot tunnel between samples. Contact with the boundary is blocked.
func (w temporaryWall) pathBlocked(from, to domain.Pos, padding float64) bool {
	if from.MapID != w.MapID || to.MapID != w.MapID {
		return false
	}
	x, y := w.local(from)
	x2, y2 := w.local(to)
	lo, hi := 0.0, 1.0
	for _, axis := range [][3]float64{{x, x2 - x, math.Hypot(w.X2-w.X, w.Y2-w.Y)/2 + padding}, {y, y2 - y, w.Width/2 + padding}} {
		if math.Abs(axis[1]) < 1e-12 {
			if math.Abs(axis[0]) > axis[2] {
				return false
			}
			continue
		}
		a, b := (-axis[2]-axis[0])/axis[1], (axis[2]-axis[0])/axis[1]
		if a > b {
			a, b = b, a
		}
		lo, hi = math.Max(lo, a), math.Min(hi, b)
		if lo > hi {
			return false
		}
	}
	return true
}

func (w temporaryWall) corners(padding float64) [4]domain.Pos {
	dx, dy := w.X2-w.X, w.Y2-w.Y
	length := math.Hypot(dx, dy)
	ux, uy := dx/length, dy/length
	cx, cy := (w.X+w.X2)/2, (w.Y+w.Y2)/2
	lx, ly := length/2+padding, w.Width/2+padding
	var out [4]domain.Pos
	for i, sign := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		out[i] = domain.Pos{MapID: w.MapID, X: cx + sign[0]*lx*ux - sign[1]*ly*uy, Y: cy + sign[0]*lx*uy + sign[1]*ly*ux}
	}
	return out
}

func temporaryWallsTooClose(a, b temporaryWall) bool {
	corners := a.corners(0)
	for i := range corners {
		if b.pathBlocked(corners[i], corners[(i+1)%4], temporaryWallMinimumGap-1e-6) {
			return true
		}
	}
	return a.pointBlocked(domain.Pos{MapID: b.MapID, X: b.X, Y: b.Y}, 0)
}

func (s *Scene) temporaryWallNow() int64 {
	if s.now == nil {
		return 0
	}
	return s.now().UnixMilli()
}

func (s *Scene) temporaryPositionWalkable(pos domain.Pos) bool {
	if len(s.temporaryWalls) == 0 {
		return true
	}
	if !finiteTemporaryCoordinate(pos.X) || !finiteTemporaryCoordinate(pos.Y) {
		return false
	}
	now := s.temporaryWallNow()
	for _, w := range s.temporaryWalls {
		if w.ExpiresAtMS > now && w.pointBlocked(pos, temporaryWallActorClearance) {
			return false
		}
	}
	return true
}

func (s *Scene) temporaryPathWalkable(from, to domain.Pos) bool {
	if len(s.temporaryWalls) == 0 {
		return true
	}
	if from.MapID != to.MapID || !finiteTemporaryCoordinate(from.X) || !finiteTemporaryCoordinate(from.Y) || !finiteTemporaryCoordinate(to.X) || !finiteTemporaryCoordinate(to.Y) {
		return false
	}
	now := s.temporaryWallNow()
	for _, w := range s.temporaryWalls {
		if w.ExpiresAtMS > now && w.pathBlocked(from, to, temporaryWallActorClearance) {
			return false
		}
	}
	return true
}

func (s *Scene) spawnPositionWalkable(pos domain.Pos) bool {
	return (s.walkable == nil || s.walkable(pos)) && s.temporaryPositionWalkable(pos)
}

// Each accepted rectangle has its own unobstructed surrounding band. Along
// with the inter-wall gap, this conservative certificate prevents a wall from
// joining terrain/another wall and sealing a corridor or enclosing a player.
// A narrow/uncertain placement simply fails; permanent MASK is never changed.
func (s *Scene) temporaryWallPlacementSafe(w temporaryWall, spawning bool) bool {
	padding := temporaryWallSpawnClearance + temporaryWallActorClearance
	entityPadding := temporaryWallActorClearance
	if spawning {
		entityPadding = padding
	}
	for _, e := range s.entities {
		if e != nil && (e.Player != nil || e.Monster != nil || e.NPC != nil) && w.pointBlocked(e.Pos, entityPadding) {
			return false
		}
	}
	// Revive/entry landings remain usable even before a floor plan is populated.
	if s.revive.Enabled() && s.revive.Scene == s.id && w.pointBlocked(s.revive.Pos, padding) {
		return false
	}
	if s.rift != nil && w.pointBlocked(s.dungeons[s.id.MapID].Enter, padding) {
		return false
	}
	if s.rift != nil && s.rift.data.Floor != nil {
		f := s.rift.data.Floor
		if w.pointBlocked(f.Entry, padding) || w.pointBlocked(f.Exit, padding) || w.pointBlocked(f.Guardian.Point.Pos, padding) {
			return false
		}
		for _, spawn := range f.Spawns {
			if w.pointBlocked(spawn.Point.Pos, temporaryWallActorClearance) {
				return false
			}
		}
	}
	areas := append([]domain.Polygon(nil), s.riftPortalAreas...)
	for _, portal := range s.portals {
		areas = append(areas, portal.Area)
	}
	corners := w.corners(padding)
	for _, area := range areas {
		for _, p := range corners {
			if area.Contains(p) {
				return false
			}
		}
		for i, p := range area {
			to := area[(i+1)%len(area)]
			p.MapID, to.MapID = w.MapID, w.MapID // Portal polygons may omit their map ID.
			if w.pathBlocked(p, to, padding) {
				return false
			}
		}
	}
	if s.walkable != nil {
		// Existing MASK resolution is 10 units; sample no more than 5 units.
		length := math.Hypot(w.X2-w.X, w.Y2-w.Y) + 2*padding
		width := w.Width + 2*padding
		nx, ny := int(math.Ceil(length/roamPathSample)), int(math.Ceil(width/roamPathSample))
		for ix := 0; ix <= nx; ix++ {
			for iy := 0; iy <= ny; iy++ {
				x, y := float64(ix)/float64(nx), float64(iy)/float64(ny)
				p := domain.Pos{MapID: w.MapID, X: corners[0].X + x*(corners[1].X-corners[0].X) + y*(corners[3].X-corners[0].X), Y: corners[0].Y + x*(corners[1].Y-corners[0].Y) + y*(corners[3].Y-corners[0].Y)}
				if !s.walkable(p) {
					return false
				}
			}
		}
	}
	return true
}

func (s *Scene) validateTemporaryWalls(walls []temporaryWall, existing map[string]temporaryWall, spawning bool) bool {
	now := s.temporaryWallNow()
	if len(walls) == 0 || len(walls)+len(existing) > maxTemporaryWalls {
		return false
	}
	seen := make(map[string]bool, len(walls))
	for i, w := range walls {
		if !w.valid(s.id.MapID, now) || seen[w.ID] || !s.temporaryWallPlacementSafe(w, spawning) {
			return false
		}
		if _, ok := existing[w.ID]; ok {
			return false
		}
		seen[w.ID] = true
		for _, other := range existing {
			if other.ExpiresAtMS > now && temporaryWallsTooClose(w, other) {
				return false
			}
		}
		for j := 0; j < i; j++ {
			if temporaryWallsTooClose(w, walls[j]) {
				return false
			}
		}
	}
	return true
}

// addTemporaryWalls is atomic. The caller must pass a complete cast group;
// rejected groups consume neither an ID slot nor any collision cells.
func (s *Scene) addTemporaryWalls(walls []temporaryWall) bool {
	s.expireTemporaryWalls(s.temporaryWallNow())
	if !s.validateTemporaryWalls(walls, s.temporaryWalls, true) {
		return false
	}
	if s.temporaryWalls == nil {
		s.temporaryWalls = make(map[string]temporaryWall)
	}
	for _, w := range walls {
		s.temporaryWalls[w.ID] = w
	}
	s.invalidateTemporaryWallPaths()
	return true
}

func (s *Scene) removeTemporaryWalls(owner string) {
	changed := false
	for id, w := range s.temporaryWalls {
		if w.Owner == owner {
			delete(s.temporaryWalls, id)
			changed = true
		}
	}
	if changed {
		s.invalidateTemporaryWallPaths()
	}
}

func (s *Scene) expireTemporaryWalls(nowMS int64) {
	changed := false
	for id, w := range s.temporaryWalls {
		if w.ExpiresAtMS <= nowMS {
			delete(s.temporaryWalls, id)
			changed = true
		}
	}
	if changed {
		s.invalidateTemporaryWallPaths()
	}
}

func (s *Scene) clearTemporaryWalls() {
	if len(s.temporaryWalls) == 0 {
		return
	}
	s.temporaryWalls = nil
	s.invalidateTemporaryWallPaths()
}

func (s *Scene) temporaryWallSnapshot() []temporaryWall {
	out := make([]temporaryWall, 0, len(s.temporaryWalls))
	now := s.temporaryWallNow()
	for _, w := range s.temporaryWalls {
		if w.ExpiresAtMS > now {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Restoring never resets lifetime. Validate as one complete replacement before
// mutation; callers cancel the matching persisted hazard if recovery is unsafe.
func (s *Scene) restoreTemporaryWalls(walls []temporaryWall) bool {
	live := make([]temporaryWall, 0, len(walls))
	for _, w := range walls {
		if w.ExpiresAtMS > s.temporaryWallNow() {
			live = append(live, w)
		}
	}
	if len(live) > 0 && !s.validateTemporaryWalls(live, nil, false) {
		return false
	}
	next := make(map[string]temporaryWall, len(live))
	for _, w := range live {
		next[w.ID] = w
	}
	s.temporaryWalls = next
	s.invalidateTemporaryWallPaths()
	return true
}

func (s *Scene) invalidateTemporaryWallPaths() {
	for _, m := range s.monsters {
		if m == nil || m.Monster == nil {
			continue
		}
		d := m.Monster
		s.clearMonsterChasePath(d)
		d.ChasePathRetryAt = 0
		d.ChasePathRetryTarget = domain.Pos{}
		if d.Roaming || d.CombatPathActive {
			s.emit(event.EntityMoved{ID: m.ID, To: m.Pos, Dir: m.Dir, Snap: true})
		}
		d.Roaming = false
		d.CombatPathActive = false
	}
}
