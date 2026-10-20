package scene

import (
	"encoding/json"
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
)

func (s *Scene) endgameCommit(p *entity.Entity, req domain.EndgameRequest, snap domain.Snapshot, response domain.EndgameResponse, run *domain.EndgameRunRecord, runRev int64, after func()) bool {
	return s.endgameCommitPayload(p, req, snap, response, run, runRev, after, s.endgamePlayer(p).state.Payload)
}
func (s *Scene) endgameCommitPayload(p *entity.Entity, req domain.EndgameRequest, snap domain.Snapshot, response domain.EndgameResponse, run *domain.EndgameRunRecord, runRev int64, after func(), payload string) bool {
	x := s.endgamePlayer(p)
	// Async run/quote loads may span another mutation: revalidate immediately
	// before taking the inventory lease, not only at initial dispatch.
	if req.Mutates() || req.ExpectedRevision != "" {
		rev, err := strconv.ParseInt(req.ExpectedRevision, 10, 64)
		if err != nil || rev != x.state.Revision || strconv.FormatInt(rev, 10) != req.ExpectedRevision {
			s.endgameError(p, req, "stale_revision", "物品状态已变化，请刷新")
			return false
		}
	}
	if s.endgameBackend == nil || x.commit != nil || endgameEconomicBusy(p) {
		s.endgameError(p, req, "busy", "其他物品事务尚未结束")
		return false
	}
	response.OK = true
	response.RequestID = req.RequestID
	if response.Kind == "" {
		response.Kind = req.Op
	}
	response.Revision = strconv.FormatInt(x.state.Revision+1, 10)
	b, err := json.Marshal(response)
	if err != nil {
		s.endgameError(p, req, "internal", "无法编码结算")
		return false
	}
	if payload == "" {
		payload = "{}"
	}
	state := domain.EndgameState{Revision: x.state.Revision + 1, Payload: payload}
	before := s.snapshotOf(p)
	x.pending = req
	x.candidate = snap.Clone()
	x.after = after
	x.onFailure = nil
	p.Player.TradeBusy = true
	x.commit = s.endgameBackend.Commit(domain.EndgameCommitPlan{Before: before, Snapshot: x.candidate.Clone(), ExpectedRevision: x.state.Revision, State: state, Receipt: domain.EndgameReceipt{CharID: p.Player.Char.ID, RequestID: req.RequestID, Operation: req.Op, Digest: endgameDigest(req), Revision: state.Revision, Response: string(b)}, Run: run, ExpectedRunRevision: runRev})
	return true
}
func (s *Scene) applyEndgameSnapshot(p *entity.Entity, snap domain.Snapshot) {
	if snap.Char == nil {
		return
	}
	leveledUp := snap.Char.Level > p.Player.Char.Level
	*p.Player.Char = *snap.Char
	p.Player.Bag = snap.Bag
	p.Player.Worn = snap.Worn
	p.Player.ChangeSet = snap.ChangeSet
	p.Player.Warehouse = snap.Warehouse
	p.Player.Wardrobe = snap.Wardrobe
	p.Player.Stall = snap.Stall
	s.syncEndgamePetProjection(p)
	p.Level = snap.Char.Level
	// 试炼/洞天经验在事务成功后才应用；恢复权威快照也只在等级真正增长时回满。
	s.refreshStatsWithLevelUp(p, leveledUp)
	s.refreshAppearance(p)
}
func (s *Scene) finishEndgameCommit(p *entity.Entity, x *endgamePlayerState, r domain.EndgameCommitResult) {
	x.commit = nil
	after, failed := x.after, x.onFailure
	x.after = nil
	x.onFailure = nil
	// Authoritative recovery always wins, even if a backend supplied inconsistent
	// auxiliary flags. Candidate publication and callbacks are strictly exclusive.
	recovered := r.Authoritative != nil
	if recovered {
		s.applyEndgameSnapshot(p, r.Authoritative.Clone())
		x.state = r.State
	}
	publish := r.Err == nil && !r.Replayed && !r.RequiresReload && !recovered
	if publish {
		s.applyEndgameSnapshot(p, x.candidate)
		x.state = r.State
	}
	if r.Err == nil && r.State.Revision >= x.state.Revision {
		x.state = r.State
	}
	if publish {
		if after != nil {
			after()
		}
	} else if failed != nil {
		failed()
	}
	p.Player.TradeBusy = false
	switch {
	case r.Err != nil:
		code, message := "storage", "结算未完成，请使用原请求编号重试"
		if errors.Is(r.Err, domain.ErrEndgameRevision) {
			code, message = "stale_revision", "已同步最新存档，请刷新后重试"
		}
		if errors.Is(r.Err, domain.ErrEndgameReceiptConflict) {
			code, message = "request_id_conflict", "请求编号已用于另一操作"
		}
		s.endgameError(p, x.pending, code, message)
	case r.Replayed:
		// Lost-ACK and reconnect replay can also require aggregate recovery, but
		// their response remains the exact original successful receipt.
		var response domain.EndgameResponse
		if json.Unmarshal([]byte(r.Receipt.Response), &response) == nil {
			s.endgameReply(p, response)
		}
	case r.RequiresReload || recovered:
		s.endgameError(p, x.pending, "stale_revision", "已同步最新存档，请刷新后重试")
	default:
		var response domain.EndgameResponse
		if json.Unmarshal([]byte(r.Receipt.Response), &response) == nil {
			s.endgameReply(p, response)
		}
	}
	p.Player.MarkDirty()
	s.pushPetSnapshot(p)
	_ = s.pushInventory(p) // also publishes the extension snapshot exactly once
	s.runDeferredTradeMutations(p.ID)
	s.finishTradeLifecycle(p.ID)
}
