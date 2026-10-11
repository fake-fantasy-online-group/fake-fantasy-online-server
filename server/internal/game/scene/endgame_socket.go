package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
)

// All successful socket mutations pass through the shared durable receipt gate.
// Legacy random-success card insertion is deliberately unavailable for 星篆.
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
	var plan domain.AstralSocketPlan
	var removalMoney int64
	switch req.Op {
	case "socket.insert":
		if p.Player.Char.Level < 10 {
			s.endgameError(p, req, "requirements", "10级后可以镶嵌星篆")
			return
		}
		runeUID, e := strconv.ParseInt(req.RuneUID, 10, 64)
		if e != nil || runeUID <= 0 {
			s.endgameError(p, req, "invalid_rune", "请选择背包中的星篆")
			return
		}
		plan, err = domain.PlanAstralInsert(p.Player.Bag, p.Player.Worn, s.itemDef, target, runeUID, uint8(req.Socket))
	case "socket.remove":
		money, e := inventoryMoney(p.Player.Char.Money)
		if e != nil || money < 2500 {
			s.endgameError(p, req, "funds", "取下一枚星篆需要2500铜币、1星髓和1净煞尘")
			return
		}
		bag := p.Player.Bag.Clone()
		if !bag.Remove(domain.CubeStarMarrow, 1) || !bag.Remove(domain.CubePurifiedDust, 1) {
			s.endgameError(p, req, "materials", "取下一枚星篆需要1星髓和1净煞尘")
			return
		}
		removalMoney = money - 2500
		// Material slots are freed only on the candidate and may hold the returned card.
		plan, err = domain.PlanAstralRemove(bag, p.Player.Worn, s.itemDef, target, uint8(req.Socket))
	default:
		s.endgameError(p, req, "unsupported_operation", "不支持的星篆操作")
		return
	}
	if err != nil {
		s.endgameError(p, req, err.Error(), astralErrorText(err.Error()))
		return
	}
	snap := s.snapshotOf(p)
	snap.Bag = plan.Bag
	snap.Worn = plan.Worn
	message := "星篆已镶入，宿阵按孔位顺序重新判定。"
	if req.Op == "socket.remove" {
		snap.Char.Money = moneyFromInventory(removalMoney)
		message = "星篆已原样取回，孔位保留，宿阵已重新判定。"
	}
	response := domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Message: message}
	s.endgameCommit(p, req, snap, response, nil, 0, func() { s.refreshStats(p) })
}
func astralErrorText(code string) string {
	switch code {
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
