package scene

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

type endgameCubeState struct {
	quoteID     string
	request     domain.EndgameRequest
	revision    int64
	fingerprint string
	expires     time.Time
}

func (s *Scene) catalogCubeRecipeViews() []domain.EndgameRecipeView {
	return domain.CubeRecipes(s.items)
}
func (s *Scene) planEndgameCube(p *entity.Entity, req domain.EndgameRequest, execute bool) (domain.CubePlan, error) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || !p.Alive() {
		return domain.CubePlan{}, &domain.CubeError{Code: "requirements", Message: "角色当前不能使用周天鼎"}
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil {
		return domain.CubePlan{}, &domain.CubeError{Code: "requirements", Message: "铜币状态无效"}
	}
	return domain.PlanCube(domain.CubeContext{Bag: p.Player.Bag, Money: money, Items: s.items, Intn: s.rng.Intn, NewUID: domain.NewItemInstanceID}, req, execute)
}
func (s *Scene) onEndgameCube(p *entity.Entity, req domain.EndgameRequest) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	state := s.endgamePlayer(p)
	if !state.loaded || s.endgameBackend == nil {
		s.endgameError(p, req, "storage", "修行记录尚未载入")
		return
	}
	if endgameEconomicBusy(p) || state.commit != nil {
		s.endgameError(p, req, "busy", "上一项物品事务尚未结束")
		return
	}
	switch req.Op {
	case "cube.quote":
		state.cube = nil
		expected, err := strconv.ParseInt(req.ExpectedRevision, 10, 64)
		if err != nil || expected != state.state.Revision {
			s.endgameError(p, req, "stale_revision", "物品状态已改变，请刷新后再试")
			return
		}
		plan, err := s.planEndgameCube(p, req, false)
		if err != nil {
			s.endgameError(p, req, domain.CubeErrorCode(err), err.Error())
			return
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			s.endgameError(p, req, "storage", "暂时无法签发炼化预览")
			return
		}
		id := hex.EncodeToString(nonce[:])
		expires := s.now().Add(60 * time.Second)
		req.ItemUIDs = append([]string(nil), req.ItemUIDs...)
		state.cube = &endgameCubeState{quoteID: id, request: req, revision: state.state.Revision, fingerprint: plan.Fingerprint, expires: expires}
		quote := domain.EndgameQuoteView{QuoteID: id, RecipeID: req.RecipeID, TargetUID: req.TargetUID, Revision: strconv.FormatInt(state.state.Revision, 10), Money: plan.Recipe.Money, Materials: plan.Materials, ExpiresAt: expires.UTC().Format(time.RFC3339), Description: plan.Description}
		s.endgameReply(p, domain.EndgameResponse{Kind: "cube.quote", RequestID: req.RequestID, OK: true, Quote: &quote})
	case "cube.execute":
		q := state.cube
		if q == nil || q.quoteID != req.QuoteID || !s.now().Before(q.expires) {
			s.endgameError(p, req, "quote_expired", "炼化预览已过期，请重新问鼎")
			return
		}
		if req.ExpectedRevision != strconv.FormatInt(q.revision, 10) || state.state.Revision != q.revision {
			s.endgameError(p, req, "stale_revision", "修行记录已变化，请重新问鼎")
			return
		}
		if req.RecipeID != 0 && req.RecipeID != q.request.RecipeID || req.TargetUID != "" && req.TargetUID != q.request.TargetUID || req.RuneUID != "" || len(req.ItemUIDs) > 0 || req.Socket != 0 || req.Tier != 0 || req.RunID != "" {
			s.endgameError(p, req, "invalid_request", "执行参数与炼化预览不一致")
			return
		}
		money, err := inventoryMoney(p.Player.Char.Money)
		if err != nil || domain.CubeFingerprint(p.Player.Bag, money, q.request) != q.fingerprint {
			s.endgameError(p, req, "stale_revision", "装备、材料或铜币已变化，请重新问鼎")
			return
		}
		plan, err := s.planEndgameCube(p, q.request, true)
		if err != nil {
			s.endgameError(p, req, domain.CubeErrorCode(err), err.Error())
			return
		}
		candidate := s.snapshotOf(p).Clone()
		candidate.Bag = plan.Bag
		candidate.Char.Money = moneyFromInventory(plan.Money)
		result := map[string]string{"recipeId": strconv.FormatInt(int64(plan.Recipe.ID), 10)}
		if !plan.Target.Empty() {
			result["itemUid"] = strconv.FormatInt(plan.Target.UID, 10)
		}
		s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "cube.result", Message: plan.Recipe.Name + "完成。", Result: result}, nil, 0, func() { state.cube = nil })
	default:
		s.endgameError(p, req, "unsupported_operation", "不支持的周天鼎操作")
	}
}
