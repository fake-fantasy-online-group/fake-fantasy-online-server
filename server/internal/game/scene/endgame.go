package scene

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strconv"
	"strings"
)

type endgamePlayerState struct {
	cube             *endgameCubeState
	state            domain.EndgameState
	loaded           bool
	load             <-chan domain.EndgameLoadResult
	receipt          <-chan domain.EndgameReceiptLoadResult
	commit           <-chan domain.EndgameCommitResult
	pending          domain.EndgameRequest
	candidate        domain.Snapshot
	after, onFailure func()
	snapshotSequence uint64
	snapshotID       string
	snapshotItems    []domain.EndgameItemView
}

func (s *Scene) endgamePlayer(p *entity.Entity) *endgamePlayerState {
	if s.endgamePlayers == nil {
		s.endgamePlayers = map[domain.EntityID]*endgamePlayerState{}
	}
	x := s.endgamePlayers[p.ID]
	if x == nil {
		x = &endgamePlayerState{}
		s.endgamePlayers[p.ID] = x
	}
	return x
}
func (s *Scene) endgameReply(p *entity.Entity, r domain.EndgameResponse) {
	if p == nil || p.Player == nil {
		return
	}
	if r.Revision == "" {
		r.Revision = strconv.FormatInt(s.endgamePlayer(p).state.Revision, 10)
	}
	s.emitTo(p.ID, event.EndgameMessage{Who: p.ID, Message: r})
}
func (s *Scene) endgameError(p *entity.Entity, r domain.EndgameRequest, code, message string) {
	s.endgameReply(p, domain.EndgameResponse{Kind: r.Op, RequestID: r.RequestID, Error: code, Message: message})
}
func endgameDigest(req domain.EndgameRequest) string {
	b, _ := json.Marshal(req)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func endgameEconomicBusy(p *entity.Entity) bool {
	return p == nil || p.Player == nil || p.Player.TradeBusy || p.Player.StallBusy || p.Player.MailBusy || p.Player.FamilyBusy || p.Player.RackBusy || p.Player.Stall != nil
}
func (s *Scene) onEndgame(cmd EndgameRequest) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	r := cmd.Request
	x := s.endgamePlayer(p)
	if s.endgameBackend == nil {
		s.endgameError(p, r, "unavailable", "演劫系统尚未启用")
		return
	}
	if x.load != nil || x.receipt != nil || x.commit != nil {
		s.endgameError(p, r, "pending", "请求处理中，请稍候")
		return
	}
	if !x.loaded {
		x.pending = r
		x.load = s.endgameBackend.Load(p.Player.Char.ID)
		return
	}
	if r.Mutates() {
		if !validEndgameRequestID(r.RequestID) {
			s.endgameError(p, r, "request_id", "请求编号无效")
			return
		}
		// Replay lookup precedes revision, ownership and affordability checks.
		x.pending = r
		x.receipt = s.endgameBackend.LoadReceipt(p.Player.Char.ID, r.RequestID)
		return
	}
	s.dispatchEndgame(p, r)
}
func (s *Scene) dispatchEndgame(p *entity.Entity, r domain.EndgameRequest) {
	x := s.endgamePlayer(p)
	if r.Op == "skill.upgrade" && !p.Player.EndgameGrowthV5 {
		s.endgameError(p, r, "update_required", "请更新客户端后分配技能点。")
		return
	}
	if r.Mutates() {
		rev, err := strconv.ParseInt(r.ExpectedRevision, 10, 64)
		if err != nil || rev != x.state.Revision || strconv.FormatInt(rev, 10) != r.ExpectedRevision {
			s.endgameError(p, r, "stale_revision", "物品状态已变化，请刷新")
			s.sendEndgameSnapshot(p)
			return
		}
	}
	if r.Op != "hello" && r.Op != "snapshot" && endgameEconomicBusy(p) {
		s.endgameError(p, r, "busy", "当前正在处理其他物品事务")
		return
	}
	switch {
	case r.Op == "hello":
		caps := []string{domain.EndgameV1}
		growth := false
		for _, cap := range r.Capabilities {
			if cap == domain.EndgameGrowthV5 {
				growth = true
			}
		}
		if growth {
			caps = append(caps, domain.EndgameGrowthV5)
		}
		priorNativeStats := p.Player.EndgameNativeStats
		p.Player.EndgameNativeStats = false
		for _, cap := range r.Capabilities {
			if cap == domain.EndgameNativeStatsV1 {
				p.Player.EndgameNativeStats = true
				break
			}
		}
		if p.Player.EndgameNativeStats {
			caps = append(caps, domain.EndgameNativeStatsV1)
		}
		// Native descriptions must arrive before capability echo makes the
		// new client stop reconstructing them from catalog rows.
		if p.Player.EndgameNativeStats || priorNativeStats {
			nativeSnapshot, err := s.inventorySnapshot(p)
			if err != nil {
				p.Player.EndgameNativeStats = priorNativeStats
				s.endgameError(p, r, "busy", "物品信息暂时无法刷新，请重试")
				return
			}
			s.emitTo(p.ID, nativeSnapshot)
		}
		p.Player.EndgameGrowthV5 = growth
		p.Player.EndgameSkillBehavior = false
		baseNegotiated := false
		for _, cap := range r.Capabilities {
			if cap == domain.EndgameV1 {
				baseNegotiated = true
				break
			}
		}
		if growth && baseNegotiated {
			for _, cap := range r.Capabilities {
				if cap == domain.EndgameSkillBehaviorV1 {
					p.Player.EndgameSkillBehavior = true
					caps = append(caps, domain.EndgameSkillBehaviorV1)
					break
				}
			}
		}
		s.endgameReply(p, domain.EndgameResponse{Kind: "hello", RequestID: r.RequestID, OK: true, Capabilities: caps, CatalogVersion: domain.UniqueCatalogVersion})
		s.sendEndgameSnapshot(p)
		if growth {
			s.sendGrowthV5Skills(p, domain.EndgameRequest{Op: "skill.ranks"})
		}
	case r.Op == "skill.ranks":
		s.sendGrowthV5Skills(p, r)
	case r.Op == "skill.upgrade":
		s.onEndgameSkillUpgrade(p, r)
	case r.Op == "snapshot":
		s.sendEndgameSnapshot(p)
	case strings.HasPrefix(r.Op, "catalog"):
		s.sendEndgameCatalog(p, r)
	case r.Op == "socket.insert" || r.Op == "socket.remove":
		s.onEndgameSocket(p, r)
	case strings.HasPrefix(r.Op, "cube."):
		s.onEndgameCube(p, r)
	case strings.HasPrefix(r.Op, "rift."):
		s.onEndgameRift(p, r)
	default:
		s.endgameError(p, r, "operation", "不支持的演劫操作")
	}
}
func (s *Scene) finishEndgame(wait bool) {
	for id, x := range s.endgamePlayers {
		p := s.players[id]
		if p == nil {
			// Read-only requests need no lifecycle lease; their buffered result
			// can be discarded after the owning entity leaves. Do not retain a
			// disconnected lazy-load/receipt state forever.
			if x.commit == nil {
				delete(s.endgamePlayers, id)
			}
			continue
		}
		if x.load != nil {
			var r domain.EndgameLoadResult
			if wait {
				r = <-x.load
			} else {
				select {
				case r = <-x.load:
				default:
					continue
				}
			}
			x.load = nil
			if r.Err != nil {
				s.endgameError(p, x.pending, "storage", "暂时无法读取存档，请重试")
				continue
			}
			x.state = r.State
			x.loaded = true
			s.onEndgame(EndgameRequest{ID: id, Request: x.pending})
		}
		if x.receipt != nil {
			var r domain.EndgameReceiptLoadResult
			if wait {
				r = <-x.receipt
			} else {
				select {
				case r = <-x.receipt:
				default:
					continue
				}
			}
			x.receipt = nil
			if r.Err != nil {
				s.endgameError(p, x.pending, "storage", "暂时无法确认请求状态")
				continue
			}
			if r.Receipt != nil {
				if r.Receipt.CharID != p.Player.Char.ID || r.Receipt.RequestID != x.pending.RequestID {
					s.endgameError(p, x.pending, "storage", "结算回执归属无效")
				} else if r.Receipt.Digest != endgameDigest(x.pending) || r.Receipt.Operation != x.pending.Op {
					s.endgameError(p, x.pending, "request_id_conflict", "请求编号不能用于不同操作")
				} else {
					var response domain.EndgameResponse
					if json.Unmarshal([]byte(r.Receipt.Response), &response) != nil {
						s.endgameError(p, x.pending, "storage", "结算回执无法读取")
					} else {
						s.endgameReply(p, response)
					}
				}
				continue
			}
			s.dispatchEndgame(p, x.pending)
		}
		if x.commit != nil {
			var r domain.EndgameCommitResult
			if wait {
				r = <-x.commit
			} else {
				select {
				case r = <-x.commit:
				default:
					continue
				}
			}
			s.finishEndgameCommit(p, x, r)
		}
	}
}

func validEndgameRequestID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
