package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type endgameRiftAdmission struct {
	Data    endgameRiftPersisted
	Retired bool
	Paid    bool
	Allowed map[domain.CharID]bool
}

func (r *Router) reserveEndgameRift(mapID int32, data endgameRiftPersisted) (domain.SceneID, error) {
	r.instanceMu.Lock()
	defer r.instanceMu.Unlock()
	r.mu.RLock()
	n := 0
	for id := range r.riftAdmissions {
		if id.MapID == mapID {
			n++
		}
	}
	r.mu.RUnlock()
	if n >= 100 {
		return domain.SceneID{}, errors.New("洞天已满")
	}
	id := r.NewInstance(mapID)
	r.mu.Lock()
	if r.riftAdmissions == nil {
		r.riftAdmissions = map[domain.SceneID]*endgameRiftAdmission{}
	}
	a := &endgameRiftAdmission{Data: data.clone(), Allowed: map[domain.CharID]bool{}}
	for char := range data.Run.Members {
		a.Allowed[char] = true
	}
	r.riftAdmissions[id] = a
	r.mu.Unlock()
	if _, err := r.Ensure(id); err != nil {
		r.mu.Lock()
		delete(r.riftAdmissions, id)
		r.mu.Unlock()
		return domain.SceneID{}, err
	}
	return id, nil
}
func (r *Router) payEndgameRift(id domain.SceneID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.riftAdmissions[id]; a != nil {
		a.Paid = true
	}
}
func (r *Router) discardEndgameRift(id domain.SceneID) {
	r.mu.Lock()
	delete(r.riftAdmissions, id)
	s := r.scenes[id]
	r.mu.Unlock()
	if s != nil {
		s.Stop()
	}
}
func (r *Router) riftAdmission(id domain.SceneID) (endgameRiftPersisted, bool) {
	if r == nil {
		return endgameRiftPersisted{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.riftAdmissions[id]
	if a == nil || !a.Paid {
		return endgameRiftPersisted{}, false
	}
	return a.Data.clone(), true
}
func (r *Router) riftScene(runID string) (domain.SceneID, bool) {
	if r == nil {
		return domain.SceneID{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, a := range r.riftAdmissions {
		if a.Paid && !a.Retired && a.Data.Run.RunID == runID {
			if s := r.scenes[id]; s != nil {
				select {
				case <-s.quit:
					continue
				default:
					return id, true
				}
			}
		}
	}
	return domain.SceneID{}, false
}
func (r *Router) withdrawRift(id domain.SceneID, char domain.CharID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.riftAdmissions[id]; a != nil {
		delete(a.Allowed, char)
	}
}
func (r *Router) ActiveRiftForCharacter(id domain.SceneID, char domain.CharID) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.riftAdmissions[id]
	s := r.scenes[id]
	if a == nil || !a.Paid || a.Retired || !a.Allowed[char] || s == nil {
		return false
	}
	select {
	case <-s.quit:
		return false
	default:
		return true
	}
}
func (r *Router) PostExistingRift(id domain.SceneID, char domain.CharID, c Command) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.riftAdmissions[id]
	s := r.scenes[id]
	return a != nil && a.Paid && !a.Retired && a.Allowed[char] && s != nil && s.Post(c)
}
func (r *Router) isRiftScene(id domain.SceneID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.riftAdmissions[id] != nil
}

// publishEndgameRiftFloor atomically changes the canonical live floor only after
// the complete run snapshot is durable. Target data is immutable across actors.
func (r *Router) publishEndgameRiftFloor(from, to domain.SceneID, d endgameRiftPersisted) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, b := r.riftAdmissions[from], r.riftAdmissions[to]
	if a == nil || b == nil || r.scenes[to] == nil {
		return false
	}
	select {
	case <-r.scenes[to].quit:
		return false
	default:
	}
	a.Retired = true
	b.Data = d.clone()
	b.Paid = true
	for char := range b.Allowed {
		if !d.Run.Admitted(char) {
			delete(b.Allowed, char)
		}
	}
	return true
}

// ActiveRiftDestination lets reconnects follow the current floor even if their
// last character save still names an earlier floor. It never creates a scene.
func (r *Router) ActiveRiftDestination(char domain.CharID) (domain.SceneID, domain.Pos, bool) {
	if r == nil {
		return domain.SceneID{}, domain.Pos{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *endgameRiftAdmission
	var id domain.SceneID
	for candidate, a := range r.riftAdmissions {
		if !a.Paid || a.Retired || !a.Allowed[char] || r.scenes[candidate] == nil {
			continue
		}
		if a.Data.Run != nil && !a.Data.Run.ResumeAllowed() {
			continue
		}
		select {
		case <-r.scenes[candidate].quit:
			continue
		default:
		}
		if best == nil || a.Data.Run.CreatedMS > best.Data.Run.CreatedMS {
			best = a
			id = candidate
		}
	}
	if best == nil {
		return domain.SceneID{}, domain.Pos{}, false
	}
	if best.Data.Floor != nil {
		return id, best.Data.Floor.Entry, true
	}
	return id, domain.Pos{}, true
}
func (r *Router) riftEntry(id domain.SceneID, fallback domain.Pos) domain.Pos {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a := r.riftAdmissions[id]; a != nil && a.Data.Floor != nil {
		return a.Data.Floor.Entry
	}
	return fallback
}

func (r *Router) postExistingRiftHandoff(id domain.SceneID, runID string, c Enter) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.riftAdmissions[id]
	s := r.scenes[id]
	if a != nil && a.Data.Run != nil && riftNeedsPages(a.Data.Run.Rules) && (c.Runtime == nil || !c.Runtime.EndgameRiftPages) {
		return false
	}
	return a != nil && a.Paid && !a.Retired && a.Data.Run.RunID == runID && a.Allowed[domain.CharID(c.Char.ID)] && s != nil && s.PostHandoff(c)
}

// CancelledRiftNeedsCityReturn reports that a saved location still belongs to a
// rift that is no longer live. Login must then use 龙城 rather than the pre-rift
// map or a public counterpart of the expired instance. Live admissions, including
// paged-reconnect holds, are not matched.
func (r *Router) CancelledRiftNeedsCityReturn(char domain.CharID, saved domain.SceneID, pos domain.Pos) bool {
	if r == nil || char == 0 {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	live := func(id domain.SceneID, a *endgameRiftAdmission) bool {
		if a == nil || !a.Paid || a.Retired || !a.Allowed[char] {
			return false
		}
		s := r.scenes[id]
		if s == nil {
			return false
		}
		select {
		case <-s.quit:
			return false
		default:
			return a.Data.Run == nil || a.Data.Run.ResumeAllowed()
		}
	}
	if saved.Instance != 0 {
		a := r.riftAdmissions[saved]
		if a == nil {
			return false
		}
		return !live(saved, a)
	}
	for id, a := range r.riftAdmissions {
		if a == nil || a.Data.Run == nil {
			continue
		}
		m := a.Data.Run.Members[char]
		if m == nil && !a.Allowed[char] {
			continue
		}
		if live(id, a) {
			continue
		}
		riftMap := a.Data.MapID
		if a.Data.Floor != nil && a.Data.Floor.MapID != 0 {
			riftMap = a.Data.Floor.MapID
		}
		if pos.MapID == riftMap || pos.MapID == id.MapID {
			return true
		}
	}
	return false
}
