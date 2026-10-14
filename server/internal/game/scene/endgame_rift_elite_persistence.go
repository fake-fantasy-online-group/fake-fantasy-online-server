package scene

import (
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func validRiftEliteSnapshot(d endgameRiftPersisted) bool {
	x := d.Elite
	if x == nil {
		return true
	}
	if d.Run.Rules.Elite == nil || len(x.Effects) > int(d.Run.Rules.Elite.HazardMaxConcurrent)+32 || len(x.Clones) > maxSummonsPerScene || len(x.PoisonNext) > 5 {
		return false
	}
	if len(x.Casters) > 33 || len(x.Deaths) > 33 {
		return false
	}
	for key, c := range x.Casters {
		if key == "" || c == nil || c.Ready == nil || len(c.Ready) > 8 || c.RecoverUntil < 0 {
			return false
		}
		for id, at := range c.Ready {
			if id < 0 || id > 7 || at < 0 {
				return false
			}
		}
	}
	for id, c := range x.Clones {
		if id == 0 || c.Owner == 0 || c.ExpiresAtMS < d.Run.StartedMS {
			return false
		}
	}
	wallIDs := map[string]bool{}
	for id, h := range x.Effects {
		if h == nil || id != h.ID || !strings.HasPrefix(id, d.Run.RunID+"/") || h.Source == 0 || h.Pos.MapID != d.MapID || math.IsNaN(h.Pos.X) || math.IsNaN(h.Pos.Y) || math.IsInf(h.Pos.X, 0) || math.IsInf(h.Pos.Y, 0) || math.Abs(h.Pos.X) > math.MaxInt32 || math.Abs(h.Pos.Y) > math.MaxInt32 {
			return false
		}
		a := h.Ability
		if a.ID < 0 || int(a.ID) >= len(d.Run.Rules.Abilities) || !reflect.DeepEqual(a, d.Run.Rules.Abilities[a.ID]) || h.ExpiresMS-h.BornMS != a.DurationMS || h.ExpiresMS < h.BornMS || h.BornMS < d.Run.StartedMS || h.LastStepMS < h.BornMS || len(h.ViewerGeneration) > 5 || len(h.Viewers) > 5 || len(h.Occupants) > 5 || len(h.PulseHits) > 5 {
			return false
		}
		if a.Mechanism == "walls" {
			if len(h.Walls) != int(a.Count) {
				return false
			}
			for _, w := range h.Walls {
				if wallIDs[w.ID] || !strings.HasPrefix(w.ID, id+"/w") || math.Abs(math.Hypot(w.X2-w.X, w.Y2-w.Y)-float64(a.Length)) > 1e-6 || w.Owner != id || w.MapID != d.MapID || w.ExpiresAtMS != h.ExpiresMS || w.Width != float64(a.Width) || !w.valid(d.MapID, h.BornMS) {
					return false
				}
				wallIDs[w.ID] = true
			}
		} else if len(h.Walls) > 0 {
			return false
		}
	}
	return true
}

// Reconstruction invalidates transport identities, never extends deadlines.
// Ordinary reconnects keep the actor and use armRiftHazards after a full snapshot.
func (s *Scene) restoreRiftElite() {
	if !s.eliteRift() || s.rift.data.Elite == nil {
		return
	}
	x := s.eliteSnapshot()
	x.PoisonNext = map[domain.EntityID]int64{}
	now := s.riftMS()
	for id, h := range x.Effects {
		h.Viewers = map[domain.EntityID]int64{}
		h.ViewerGeneration = map[domain.EntityID]uint64{}
		h.Occupants = map[domain.EntityID]endgameRiftOccupant{}
		h.PulseHits = map[domain.EntityID]bool{}
		h.LastStepMS = max(now, h.BornMS)
		if now >= h.ExpiresMS {
			delete(x.Effects, id)
		}
	}
	// Rebuild collision from authoritative effect geometry, not a second,
	// potentially stale duplicate array. Unsafe recovery cancels the whole group.
	s.clearTemporaryWalls()
	ids := make([]string, 0, len(x.Effects))
	for id := range x.Effects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		h := x.Effects[id]
		if h.Ability.Mechanism == "walls" {
			candidate := append(s.temporaryWallSnapshot(), h.Walls...)
			if !s.restoreTemporaryWalls(candidate) {
				delete(x.Effects, id)
			}
		}
	}
	x.Walls = s.temporaryWallSnapshot()
}
