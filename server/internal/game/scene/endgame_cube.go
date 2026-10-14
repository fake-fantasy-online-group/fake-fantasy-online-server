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
	// Never expose a random candidate until its atomic commit succeeds. Retain it
	// across definite storage failures so retrying this quote cannot reroll.
	candidate *domain.CubePlan
	choices   []domain.EndgameRequest
	rules     *domain.CubeRuleTable
	catalog   *domain.EndgameCatalog
}

func (s *Scene) catalogCubeRecipeViews() []domain.EndgameRecipeView {
	return domain.CubeRecipes(s.cubeRules, s.items)
}
func (s *Scene) endgameCubeContext(p *entity.Entity) (domain.CubeContext, error) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || !p.Alive() {
		return domain.CubeContext{}, &domain.CubeError{Code: "requirements", Message: "角色当前不能使用周天鼎"}
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil {
		return domain.CubeContext{}, &domain.CubeError{Code: "requirements", Message: "铜币状态无效"}
	}
	return domain.CubeContext{Rules: s.cubeRules, Catalog: s.endgameCatalog, Level: p.Player.Char.Level, Bag: p.Player.Bag, Money: money, Items: s.items, Intn: s.rng.Intn, NewUID: domain.NewItemInstanceID}, nil
}
func (s *Scene) planEndgameCube(p *entity.Entity, req domain.EndgameRequest, execute bool) (domain.CubePlan, error) {
	ctx, err := s.endgameCubeContext(p)
	if err != nil {
		return domain.CubePlan{}, err
	}
	return domain.PlanCube(ctx, req, execute)
}

// Portable authority is the database-configured bound tool in the live bag.
// Non-zero NPC context is obsolete and must never authorize this operation.
func (s *Scene) canUseCubeWorkbench(p *entity.Entity, id domain.EntityID) bool {
	return id == 0 && s.hasCubeTool(p)
}
func (s *Scene) onEndgameCube(p *entity.Entity, req domain.EndgameRequest) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	if !s.canUseCubeWorkbench(p, req.NPCEntityID) {
		s.endgameError(p, req, "tool_required", "请将绑定的周天鼎带在背包中再问鼎")
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
		var plan domain.CubePlan
		var choices []domain.EndgameRequest
		if len(req.CubeInputs) > 0 {
			if !s.canUseCubeWorkbench(p, req.NPCEntityID) {
				s.endgameError(p, req, "tool_required", "请将周天鼎带在背包中再问鼎")
				return
			}
			ctx, contextErr := s.endgameCubeContext(p)
			if contextErr != nil {
				s.endgameError(p, req, domain.CubeErrorCode(contextErr), contextErr.Error())
				return
			}
			var plans []domain.CubePlan
			choices, plans, err = domain.MatchCubeInputs(ctx, req)
			if err == nil {
				plan = plans[0]
				plan.Fingerprint = domain.CubeFingerprint(ctx.Bag, ctx.Money, req)
				if len(choices) > 1 {
					plan.Description = "鼎中投料将随机炼成一枚星篆。"
				}
			}
		} else {
			plan, err = s.planEndgameCube(p, req, false)
		}
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
		req.CubeInputs = append([]domain.CubeInput(nil), req.CubeInputs...)
		state.cube = &endgameCubeState{quoteID: id, request: req, revision: state.state.Revision, fingerprint: plan.Fingerprint, expires: expires, choices: choices, rules: s.cubeRules, catalog: s.endgameCatalog}
		quote := domain.EndgameQuoteView{QuoteID: id, RecipeID: req.RecipeID, TargetUID: req.TargetUID, Revision: strconv.FormatInt(state.state.Revision, 10), Money: plan.Recipe.Money, Materials: plan.Materials, ExpiresAt: expires.UTC().Format(time.RFC3339), Description: plan.Description}
		s.endgameReply(p, domain.EndgameResponse{Kind: "cube.quote", RequestID: req.RequestID, OK: true, Quote: &quote})
	case "cube.execute":
		q := state.cube
		if q == nil || q.quoteID != req.QuoteID || !s.now().Before(q.expires) {
			s.endgameError(p, req, "quote_expired", "炼化预览已过期，请重新问鼎")
			return
		}
		// Rules and catalog are immutable startup snapshots whose accessors
		// return deep copies. A replacement must invalidate quotes, including
		// an already-rolled cached candidate; fees/results cannot change under it.
		if q.rules != s.cubeRules || q.catalog != s.endgameCatalog {
			state.cube = nil
			s.endgameError(p, req, "quote_expired", "炼化配置已变化，请重新问鼎")
			return
		}
		if len(q.choices) > 0 && (req.NPCEntityID != q.request.NPCEntityID || !s.canUseCubeWorkbench(p, req.NPCEntityID)) {
			s.endgameError(p, req, "tool_required", "请将周天鼎带在背包中再问鼎")
			return
		}
		if req.ExpectedRevision != strconv.FormatInt(q.revision, 10) || state.state.Revision != q.revision {
			s.endgameError(p, req, "stale_revision", "修行记录已变化，请重新问鼎")
			return
		}
		if req.RecipeID != 0 && req.RecipeID != q.request.RecipeID || req.TargetUID != "" && req.TargetUID != q.request.TargetUID || req.RuneUID != "" || len(req.ItemUIDs) > 0 || len(req.CubeInputs) > 0 || req.Socket != 0 || req.Tier != 0 || req.RunID != "" {
			s.endgameError(p, req, "invalid_request", "执行参数与炼化预览不一致")
			return
		}
		money, err := inventoryMoney(p.Player.Char.Money)
		if err != nil || domain.CubeFingerprint(p.Player.Bag, money, q.request) != q.fingerprint {
			s.endgameError(p, req, "stale_revision", "装备、材料或铜币已变化，请重新问鼎")
			return
		}
		// A cached candidate does not waive live character/recipe preconditions.
		execution := q.request
		if len(q.choices) > 0 {
			ctx, contextErr := s.endgameCubeContext(p)
			if contextErr != nil {
				s.endgameError(p, req, domain.CubeErrorCode(contextErr), contextErr.Error())
				return
			}
			choices, _, matchErr := domain.MatchCubeInputs(ctx, q.request)
			if matchErr != nil {
				s.endgameError(p, req, domain.CubeErrorCode(matchErr), matchErr.Error())
				return
			}
			if len(choices) != len(q.choices) {
				s.endgameError(p, req, "stale_revision", "投料状态已变化，请重新问鼎")
				return
			}
			for i, choice := range choices {
				if choice.RecipeID != q.choices[i].RecipeID {
					s.endgameError(p, req, "stale_revision", "投料状态已变化，请重新问鼎")
					return
				}
			}
			if q.candidate == nil {
				execution = choices[0]
				if len(choices) > 1 {
					execution = choices[s.rng.Intn(len(choices))]
				}
			}
		} else if _, err := s.planEndgameCube(p, q.request, false); err != nil {
			s.endgameError(p, req, domain.CubeErrorCode(err), err.Error())
			return
		}
		if q.candidate == nil {
			plan, err := s.planEndgameCube(p, execution, true)
			if err != nil {
				s.endgameError(p, req, domain.CubeErrorCode(err), err.Error())
				return
			}
			q.candidate = &plan
		}
		plan := *q.candidate
		candidate := s.snapshotOf(p).Clone()
		candidate.Bag = plan.Bag
		candidate.Char.Money = moneyFromInventory(plan.Money)
		result := map[string]string{"recipeId": strconv.FormatInt(int64(plan.Recipe.ID), 10)}
		if !plan.Target.Empty() {
			result["itemUid"] = strconv.FormatInt(plan.Target.UID, 10)
			if plan.Recipe.Operation == "socket.open" {
				result["socketCount"] = strconv.Itoa(int(plan.Target.SocketCount))
			}
		}
		message := plan.Recipe.Name + "完成。"
		if plan.Recipe.Operation == "socket.open" {
			message = plan.Recipe.Name + "完成：开出" + strconv.Itoa(int(plan.Target.SocketCount)) + "孔。"
		}
		s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "cube.result", Message: message, Result: result}, nil, 0, func() { state.cube = nil })
	default:
		s.endgameError(p, req, "unsupported_operation", "不支持的周天鼎操作")
	}
}
