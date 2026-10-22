package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func riftNeedsPages(rules *domain.RiftRules) bool {
	return rules != nil && rules.FloorLayoutVersion == domain.RiftFloorFullCoverageVersion
}

func (s *Scene) requireRiftPages(p *entity.Entity, req domain.EndgameRequest, rules *domain.RiftRules) bool {
	if !riftNeedsPages(rules) || p != nil && p.Player != nil && p.Player.EndgameRiftPages {
		return true
	}
	s.endgameError(p, req, "update_required", "请更新客户端后进入洞天；未扣除符钥，已有入场资格与进度保留。")
	return false
}

// A fresh login cannot prove support until its endgame negotiation completes.
// Hold v6 reconnects at their original public return point, not inside a private
// encounter that the old client cannot render. Admission remains unchanged.
func (r *Router) PagedRiftReconnect(char domain.CharID) (domain.SceneID, domain.Pos, domain.Pos, string, bool) {
	id, entry, ok := r.ActiveRiftDestination(char)
	if !ok {
		return domain.SceneID{}, domain.Pos{}, domain.Pos{}, "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.riftAdmissions[id]
	if a == nil || !a.Paid || a.Retired || !a.Allowed[char] || a.Data.Run == nil || !riftNeedsPages(a.Data.Run.Rules) || a.Data.Run.Terminal() {
		return domain.SceneID{}, domain.Pos{}, domain.Pos{}, "", false
	}
	return id, entry, a.Data.Return, a.Data.Run.RunID, true
}

func (s *Scene) resumePagedRift(p *entity.Entity) {
	if s.router == nil || !s.id.Persistent() || p == nil || p.Player == nil || !p.Player.EndgameRiftPages {
		return
	}
	charID := domain.CharID(p.Player.Char.ID)
	_, _, _, _, ok := s.router.PagedRiftReconnect(charID)
	if !ok {
		return
	}
	entityID := p.ID
	// The capability echo is emitted first. Reuse the paid-run transfer path;
	// this is neither a new admission nor an economic mutation.
	s.timer.after(s.tick, 1, func() {
		if peer := s.players[entityID]; peer != nil && peer.Player.EndgameRiftPages {
			if id, entry, _, runID, stillLive := s.router.PagedRiftReconnect(charID); stillLive {
				s.onTeleport(Teleport{ID: entityID, RiftRunID: runID, To: id, At: entry})
			}
		}
	})
}
