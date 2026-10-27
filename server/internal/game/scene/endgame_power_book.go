package scene

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

const powerBookPageSize = 50

func (s *Scene) onEndgamePowerBook(p *entity.Entity, req domain.EndgameRequest) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || s.endgameBackend == nil {
		s.endgameError(p, req, "storage", "威能录尚未载入")
		return
	}
	book, err := domain.ReadLegendaryPowerBook(x.state.Payload)
	if err != nil {
		s.endgameError(p, req, "storage", "威能录暂不可读，器物尚未改动")
		return
	}
	switch req.Op {
	case "powerbook.list":
		offset := 0
		if req.Cursor != "" {
			n, e := strconv.Atoi(req.Cursor)
			if e != nil || n < 0 || n > len(book) || strconv.Itoa(n) != req.Cursor {
				s.endgameError(p, req, "cursor", "威能录页码无效，请重新翻阅")
				return
			}
			offset = n
		}
		ids := make([]string, 0, len(book))
		for id := range book {
			ids = append(ids, id)
		}
		sort.Strings(ids)
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
			v := domain.EndgamePowerBookView{PowerID: id, Name: power.Name, Category: power.Category, Class: power.Class, Description: power.Name + "：" + power.Effect, MinLevel: power.MinLevel}
			if domain.LegendaryPowerHasRoll(id) {
				v.RollBP = domain.EffectiveLegendaryPowerRollBP(id, book[id])
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
	case "powerbook.imprint":
		if !p.Alive() {
			s.endgameError(p, req, "requirements", "角色当前不能刻印威能")
			return
		}
		if req.PowerID == "" || req.RuneUID != "" || req.RecipeID != 0 || req.QuoteID != "" || len(req.ItemUIDs) != 0 || len(req.CubeInputs) != 0 || req.NPCEntityID != 0 || req.Socket != 0 || req.Tier != 0 || req.RunID != "" {
			s.endgameError(p, req, "invalid_request", "请选择一件装备和一项已收录威能")
			return
		}
		uid, e := strconv.ParseInt(req.TargetUID, 10, 64)
		if e != nil || uid <= 0 || strconv.FormatInt(uid, 10) != req.TargetUID {
			s.endgameError(p, req, "invalid_request", "装备实例号无效")
			return
		}
		slot := -1
		for i := 0; i < p.Player.Bag.Cap(); i++ {
			st := p.Player.Bag.At(i)
			if !st.Empty() && st.UID == uid {
				if slot >= 0 {
					s.endgameError(p, req, "requirements", "装备实例状态异常")
					return
				}
				slot = i
			}
		}
		if slot < 0 {
			s.endgameError(p, req, "not_owner", "请将待刻印装备放入自己的背包")
			return
		}
		st := p.Player.Bag.At(slot)
		def, ok := s.itemDef(st.Item)
		if !ok || !def.InventoryTabKnown || !p.Player.Bag.AllowsSlot(def, slot) {
			s.endgameError(p, req, "requirements", "装备位置或模板无效")
			return
		}
		changed, e := book.ImprintPower(st, def, p.Player.Char, req.PowerID)
		if e != nil {
			s.endgameError(p, req, domain.CubeErrorCode(e), e.Error())
			return
		}
		changed.Endgame.Revision++
		candidate := s.snapshotOf(p).Clone()
		candidate.Bag.Set(slot, changed)
		s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "powerbook.result", Message: "威能已刻入器物。", Result: map[string]string{"itemUid": req.TargetUID, "powerId": req.PowerID}}, nil, 0, nil)
	default:
		s.endgameError(p, req, "unsupported_operation", "不支持的威能录操作")
	}
}

// The book update shares the cube inventory transaction and receipt. No live
// book or item is changed during quotation, failed storage, or replay recovery.
func powerBookAfterCube(payload string, before *domain.Bag, plan domain.CubePlan) (string, error) {
	if plan.Recipe.Operation != "salvage.unique" {
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
