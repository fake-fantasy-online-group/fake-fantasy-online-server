package scene

import (
	"encoding/json"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

const powerBookPageSize = 50

func (s *Scene) onEndgamePowerBook(p *entity.Entity, req domain.EndgameRequest) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	if !p.Player.EndgamePowerCodex && req.Op != "powerbook.imprint" {
		s.endgameError(p, req, "update_required", "请更新客户端后翻阅秘典或使用威能卷轴")
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || s.endgameBackend == nil {
		s.endgameError(p, req, "storage", "秘典尚未载入")
		return
	}
	book, err := domain.ReadLegendaryPowerBook(x.state.Payload)
	if err != nil {
		s.endgameError(p, req, "storage", "秘典暂不可读，器物尚未改动")
		return
	}
	switch req.Op {
	case "powerbook.list":
		if !s.hasPowerCodex(p) {
			s.endgameError(p, req, "tool_required", "请将秘典带在背包中再翻阅")
			return
		}
		offset := 0
		if req.Cursor != "" {
			n, e := strconv.Atoi(req.Cursor)
			if e != nil || n < 0 || n > len(domain.LegendaryPowers()) || strconv.Itoa(n) != req.Cursor {
				s.endgameError(p, req, "cursor", "秘典页码无效，请重新翻阅")
				return
			}
			offset = n
		}
		ids := make([]string, 0, len(domain.LegendaryPowers()))
		for _, power := range domain.LegendaryPowers() {
			ids = append(ids, power.ID)
		}
		end := offset + powerBookPageSize
		if end > len(ids) {
			end = len(ids)
		}
		views := make([]domain.EndgamePowerBookView, 0, end-offset)
		for _, id := range ids[offset:end] {
			power, _ := domain.LegendaryPowerForRoll(id, func() int32 {
				if domain.LegendaryPowerHasRoll(id) {
					return domain.EffectiveLegendaryPowerRollBP(id, book[id])
				}
				return 0
			}())
			_, unlocked := book[id]
			v := domain.EndgamePowerBookView{Unlocked: unlocked, Slots: powerCodexSlots(power), PowerID: id, Name: power.Name, Category: power.Category, Class: power.Class, Description: power.Name + "：" + power.Effect, MinLevel: power.MinLevel}
			if domain.LegendaryPowerHasRoll(id) {
				if unlocked {
					v.RollBP = domain.EffectiveLegendaryPowerRollBP(id, book[id])
				}
				v.MinRollBP = domain.LegendaryPowerRollMinBP
				v.MaxRollBP = domain.LegendaryPowerRollMaxBP
			}
			views = append(views, v)
			raw, encodeErr := json.Marshal(views)
			if encodeErr != nil || len(raw) > domain.EndgameMaxJSONBytes-2048 {
				views = views[:len(views)-1]
				break
			}
		}
		end = offset + len(views)
		if end == offset && offset < len(ids) {
			s.endgameError(p, req, "snapshot_too_large", "此页威能说明超出传输范围")
			return
		}
		next := ""
		if end < len(ids) {
			next = strconv.Itoa(end)
		}
		s.endgameReply(p, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Powers: views, Cursor: req.Cursor, NextCursor: next})
	case "powerbook.extract":
		s.extractPowerScroll(p, req, book)
	case "powerbook.apply":
		s.applyPowerScroll(p, req)
	case "powerbook.recover":
		s.recoverPowerCodex(p, req)
	case "powerbook.imprint":
		s.endgameError(p, req, "scroll_required", "请提取卷轴后，右击卷轴附着威能")

	default:
		s.endgameError(p, req, "unsupported_operation", "不支持的秘典操作")
	}
}

// The book update shares the cube inventory transaction and receipt. No live
// book or item is changed during quotation, failed storage, or replay recovery.
func powerBookAfterCube(payload string, before *domain.Bag, plan domain.CubePlan) (string, error) {
	if plan.Recipe.Operation != "salvage.unique" && plan.Recipe.Operation != "life.salvage.unique" {
		return payload, nil
	}
	book, err := domain.ReadLegendaryPowerBook(payload)
	if err != nil {
		return "", err
	}
	consumed := map[int64]bool{}
	for _, uid := range plan.ConsumedUIDs {
		consumed[uid] = true
	}
	changed := false
	for i := 0; before != nil && i < before.Cap(); i++ {
		st := before.At(i)
		if !consumed[st.UID] {
			continue
		}
		improved, e := book.RecordSalvagedPower(st)
		if e != nil {
			return "", e
		}
		changed = changed || improved
	}
	if !changed {
		return payload, nil
	}
	return domain.WriteLegendaryPowerBook(payload, book)
}
