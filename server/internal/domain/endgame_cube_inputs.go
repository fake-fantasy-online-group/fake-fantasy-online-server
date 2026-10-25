package domain

import (
	"strconv"
)

// CubeInputCapacity is the number of reservations in the native 5-by-7 grid.
// It bounds selected identities, not stack quantities or recipe requirements.
const CubeInputCapacity = 35

// CubeInput is a reservation in the player's bag, never a second inventory.
// An instance uses UID/count=1; a stackable material uses ItemID/count. Both
// forms together, duplicate identities and quantities not actually owned fail.
type CubeInput struct {
	UID    string `json:"uid,omitempty"`
	ItemID ItemID `json:"itemId,omitempty"`
	Count  int32  `json:"count"`
}

// MatchCubeInputs derives every eligible recipe from the immutable DB rules.
// It performs no RNG, UID allocation, physical movement or bag mutation.
// Identical-input variants of a single operation may form an equal-weight pool;
// ambiguity across operations or fees fails closed rather than using row order.
func MatchCubeInputs(ctx CubeContext, input EndgameRequest) ([]EndgameRequest, []CubePlan, error) {
	fail := func(code, message string) ([]EndgameRequest, []CubePlan, error) {
		return nil, nil, cubeError(code, message)
	}
	if ctx.Rules == nil || ctx.Bag == nil {
		return fail("configuration", "周天鼎配置尚未载入")
	}
	if input.Socket != 0 || input.Tier != 0 || input.RunID != "" || len(input.ItemUIDs) > 5 || input.RecipeID != 0 || input.TargetUID != "" || input.RuneUID != "" || len(input.CubeInputs) == 0 || len(input.CubeInputs) > CubeInputCapacity {
		return fail("invalid_request", "请将背包中的物品放入鼎中")
	}
	materials := map[ItemID]int32{}
	seen := map[int64]bool{}
	var gear string
	var gears []string
	var runes []string
	for _, selected := range input.CubeInputs {
		if selected.Count <= 0 {
			return fail("invalid_request", "投料数量无效")
		}
		if selected.UID == "" {
			def, ok := ctx.Items[selected.ItemID]
			if !ok || selected.ItemID <= 0 || !def.Stackable || def.InstanceKind != ItemInstanceNone || !def.InventoryTabKnown || materials[selected.ItemID] != 0 {
				return fail("invalid_request", "投料材料无效或重复")
			}
			if ctx.Bag.UsableCountOf(selected.ItemID) < selected.Count {
				return fail("materials", "投料材料不足或已锁定")
			}
			materials[selected.ItemID] = selected.Count
			continue
		}
		uid, err := strconv.ParseInt(selected.UID, 10, 64)
		if err != nil || uid <= 0 || strconv.FormatInt(uid, 10) != selected.UID || selected.ItemID != 0 || selected.Count != 1 || seen[uid] {
			return fail("invalid_request", "投料实例无效或重复")
		}
		seen[uid] = true
		var found Stack
		ctx.Bag.Each(func(slot int, st Stack) {
			if st.UID == uid && !st.Empty() {
				found = st
			}
		})
		if found.Empty() {
			return fail("not_owner", "投料物品已不在自己的背包")
		}
		def, ok := ctx.Items[found.Item]
		if !ok || found.Locked || found.Card.Locked {
			return fail("locked", "投料物品不可用或已锁定")
		}
		if def.Equip != nil && def.InstanceKind == ItemInstanceEquipment {
			gears = append(gears, selected.UID)
			gear = selected.UID
		} else if _, ok := AstralRune(found.Item, ctx.Catalog); ok && def.InstanceKind == ItemInstanceSocketCard {
			runes = append(runes, selected.UID)
		} else {
			return fail("requirements", "这些物品无法炼化")
		}
	}
	if requests, plans, handled, err := matchWingCubeInputs(ctx, input, materials, gears, runes); handled {
		return requests, plans, err
	}
	if len(gears) > 1 {
		return fail("partial_match", "部分材料相合，尚需调配。")
	}
	if len(runes) > 5 {
		return fail("requirements", "投料星篆数量过多")
	}
	var requests []EndgameRequest
	var plans []CubePlan
	var eligibleError error
	for _, recipe := range ctx.Rules.Recipes() {
		if recipe.TargetRequired != (gear != "") || len(recipe.Materials) != len(materials) {
			continue
		}
		match := true
		for _, material := range recipe.Materials {
			if materials[material.ItemID] != material.Count {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		req := input
		req.CubeInputs = nil
		req.RecipeID = recipe.ID
		req.TargetUID = gear
		req.ItemUIDs = nil
		switch recipe.Operation {
		case "salvage.rune":
			if len(runes) != 1 || len(input.ItemUIDs) != 0 {
				continue
			}
			req.RuneUID = runes[0]
		case "rune.convert", "socket.array":
			if len(input.ItemUIDs) != 0 {
				continue
			}
			req.ItemUIDs = append([]string(nil), runes...)
		case "socket.reorder":
			if len(runes) != 0 {
				continue
			}
			req.ItemUIDs = append([]string(nil), input.ItemUIDs...)
		default:
			if len(runes) != 0 || len(input.ItemUIDs) != 0 {
				continue
			}
		}
		plan, err := PlanCube(ctx, req, false)
		if err != nil {
			eligibleError = err
			continue
		}
		if len(plans) > 0 && (plan.Recipe.Operation != plans[0].Recipe.Operation || plan.Money != plans[0].Money) {
			return fail("configuration", "这些投料匹配了不同炼化方式，暂不能炼化")
		}
		requests = append(requests, req)
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		if eligibleError != nil {
			mismatch := cubeInputMismatch(ctx, input, materials, gear, runes)
			if CubeErrorCode(eligibleError) == "requirements" && CubeErrorCode(mismatch) == "insufficient_count" {
				return nil, nil, mismatch
			}
			return nil, nil, eligibleError
		}
		return nil, nil, cubeInputMismatch(ctx, input, materials, gear, runes)
	}
	return requests, plans, nil
}

// Diagnose only after reservation validation and unsuccessful exact matching.
// Rank is independent of recipe order: a fully compatible, underfilled recipe
// outranks partial overlap, which outranks no overlap. No names, IDs, quantities,
// hidden outputs or candidate counts are disclosed. Extra items or overfilling
// are partial mismatch, never misleadingly reported as a shortage.
func cubeInputMismatch(ctx CubeContext, input EndgameRequest, materials map[ItemID]int32, gear string, runes []string) error {
	partial := false
	for _, recipe := range ctx.Rules.Recipes() {
		sameTypes, short, over := len(materials) == len(recipe.Materials), false, false
		for _, m := range recipe.Materials {
			n := materials[m.ItemID]
			if n > 0 {
				partial = true
			} else {
				sameTypes = false
			}
			short = short || n < m.Count
			over = over || n > m.Count
		}
		if recipe.Operation == "rune.convert" || recipe.Operation == "salvage.rune" {
			ctx.Bag.Each(func(_ int, st Stack) {
				if !seenCubeRune(runes, st.UID) {
					return
				}
				r, ok := AstralRune(st.Item, ctx.Catalog)
				if !ok || !st.Card.Initialized || st.Card.Count > 8 || st.Count != 1 || st.InstanceKind != ItemInstanceSocketCard {
					return
				}
				if recipe.Operation == "salvage.rune" {
					partial = true
					return
				}
				out, ok := AstralRune(recipe.Outputs[0].ItemID, ctx.Catalog)
				if ok && r.Quadrant == out.Quadrant && st.Item != recipe.Outputs[0].ItemID {
					partial = true
				}
			})
		}
		if recipe.TargetRequired != (gear != "") {
			continue
		}
		req := input
		req.CubeInputs = nil
		req.RecipeID, req.TargetUID = recipe.ID, gear
		req.ItemUIDs = nil
		sourceShort := false
		switch recipe.Operation {
		case "salvage.rune":
			if len(runes) != 1 || len(input.ItemUIDs) != 0 {
				continue
			}
			req.RuneUID = runes[0]
		case "rune.convert":
			if len(input.ItemUIDs) != 0 || len(runes) == 0 || len(runes) > int(recipe.SourceCount) {
				continue
			}
			req.ItemUIDs = append([]string(nil), runes...)
			sourceShort = len(runes) < int(recipe.SourceCount)
		case "socket.array":
			if len(input.ItemUIDs) != 0 {
				continue
			}
			req.ItemUIDs = append([]string(nil), runes...)
		case "socket.reorder":
			if len(runes) != 0 {
				continue
			}
			req.ItemUIDs = append([]string(nil), input.ItemUIDs...)
		default:
			if len(runes) != 0 || len(input.ItemUIDs) != 0 {
				continue
			}
		}
		if gear == "" && len(runes) == 0 && (!sameTypes || over || !short) {
			continue // no instance overlap or possible count-only match to validate
		}
		// Reuse the actual planner's equipment, socket, rune and level checks;
		// diagnostic mode ignores only costs/capacity and conversion source count.
		plan, err := planCubeMode(ctx, req, false, true)
		if !plan.Target.Empty() {
			partial = true
		}
		if err != nil {
			continue
		}
		if gear != "" || len(runes) > 0 {
			partial = true
		}
		if sameTypes && !over && (short || sourceShort) {
			return cubeError("insufficient_count", "材料虽合，数目尚缺。")
		}
	}
	if partial {
		return cubeError("partial_match", "部分材料相合，尚需调配。")
	}
	return cubeError("no_match", "鼎中诸物，未合炼方。")
}

func seenCubeRune(runes []string, uid int64) bool {
	for _, raw := range runes {
		if raw == strconv.FormatInt(uid, 10) {
			return true
		}
	}
	return false
}
