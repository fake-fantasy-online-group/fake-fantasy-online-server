package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
	"strings"
)

// All successful socket mutations pass through the shared durable receipt gate.
// The original furnace delegates 星篆 here; legacy cards keep their random rules.
func (s *Scene) onEndgameSocket(p *entity.Entity, req domain.EndgameRequest) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || !p.Alive() {
		s.endgameError(p, req, "requirements", "角色当前不能调整星篆")
		return
	}
	target, err := strconv.ParseInt(req.TargetUID, 10, 64)
	if err != nil || target <= 0 || req.Socket < 0 || req.Socket >= 5 {
		s.endgameError(p, req, "invalid_request", "请选择有效装备和孔位")
		return
	}
	if req.Op != "socket.insert" {
		s.endgameError(p, req, "unsupported_operation", "星篆请使用镶嵌炉；不提供独立取出功能")
		return
	}
	if strings.HasPrefix(req.RequestID, "native-inlay-") {
		q := s.endgamePlayer(p).nativeSocket
		if q == nil || q.request.RequestID != req.RequestID || endgameDigest(q.request) != endgameDigest(req) ||
			!s.nativeSocketSelectionCurrent(p, q, q.command) {
			s.endgameError(p, req, "stale_revision", "装备或星篆已变化，请重新将装备放入镶嵌炉。")
			return
		}
	}
	runeUID, e := strconv.ParseInt(req.RuneUID, 10, 64)
	if e != nil || runeUID <= 0 {
		s.endgameError(p, req, "invalid_rune", "请选择背包中的星篆")
		return
	}
	plan, err := domain.PlanAstralInsert(p.Player.Bag, p.Player.Worn, s.itemDef, target, runeUID, uint8(req.Socket), p.Player.Char)
	if err != nil {
		s.endgameError(p, req, err.Error(), astralErrorText(err.Error()))
		return
	}
	snap := s.snapshotOf(p)
	snap.Bag = plan.Bag
	snap.Worn = plan.Worn
	message := "星篆已镶入，单枚属性随装备生效；填满孔位后按底材与宿序判阵。"
	response := domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Message: message}
	s.endgameCommit(p, req, snap, response, nil, 0, func() { s.refreshStats(p) })
}
func astralErrorText(code string) string {
	switch code {
	case "rune_level":
		return "角色等级未达到该星篆的使用要求"
	case "formation_level":
		return "角色等级未达到该宿阵的使用要求"
	case "not_owner", "not_found":
		return "物品已移动，请刷新后重试"
	case "locked":
		return "请先解除装备或星篆锁定"
	case "bag_full":
		return "背包空间不足，星篆未取出"
	case "invalid_socket":
		return "孔位或装备状态不符合要求"
	case "invalid_rune":
		return "星篆不存在、已镶入或状态不正确"
	}
	return "无法调整星篆"
}
