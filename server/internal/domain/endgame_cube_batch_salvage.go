package domain

import (
	"fmt"
	"sort"
	"strconv"
)

// Batch salvage is the existing equipment-only recipe, not an inventory sweep
// or a fallback for malformed crafting ingredients. Progression wings never
// enter salvage, including the removed wing + darkgold extraction pattern.
func matchBatchSalvage(ctx CubeContext, input EndgameRequest) (EndgameRequest, CubePlan, error) {
	requests, err := batchSalvageRequests(ctx, input)
	if err != nil {
		return input, CubePlan{}, err
	}
	req := input
	req.RecipeID = requests[0].RecipeID
	req.TargetUID = requests[0].TargetUID
	for _, r := range requests {
		def, _ := ctx.Rules.Recipe(r.RecipeID)
		if def.Operation == "life.salvage.unique" {
			req.RecipeID = r.RecipeID
			break
		}
	}
	p, err := planBatchSalvage(ctx, req, false)
	return req, p, err
}

func batchSalvageRequests(ctx CubeContext, req EndgameRequest) ([]EndgameRequest, error) {
	if ctx.Bag == nil || ctx.Rules == nil || len(req.CubeInputs) < 2 || len(req.CubeInputs) > CubeInputCapacity || len(req.ItemUIDs) != 0 || req.RuneUID != "" || req.Socket != 0 || req.Tier != 0 || req.RunID != "" {
		return nil, cubeError("invalid_request", "请仅投入待分解的装备")
	}
	seen := map[int64]bool{}
	requests := make([]EndgameRequest, 0, len(req.CubeInputs))
	for _, in := range req.CubeInputs {
		uid, err := strconv.ParseInt(in.UID, 10, 64)
		if err != nil || uid <= 0 || strconv.FormatInt(uid, 10) != in.UID || in.ItemID != 0 || seen[uid] {
			return nil, cubeError("invalid_request", "投料实例无效或重复")
		}
		seen[uid] = true
		var st Stack
		found := 0
		ctx.Bag.Each(func(_ int, s Stack) {
			if !s.Empty() && s.UID == uid {
				st = s
				found++
			}
		})
		if found != 1 {
			return nil, cubeError("not_owner", "投料物品已不在自己的背包")
		}
		d, ok := ctx.Items[st.Item]
		if !ok || d.Equip == nil || d.InstanceKind != ItemInstanceEquipment {
			return nil, cubeError("requirements", "请仅投入待分解的装备")
		}
		// Never reinterpret a wing upgrade or removed extraction as salvage.
		if ctx.WingRules != nil {
			if _, wing := ctx.WingRules.Item(st.Item); wing {
				return nil, cubeError("partial_match", "部分材料相合，尚需调配。")
			}
		}
		_, unique := UniqueOf(st, ctx.Catalog)
		op := "life.salvage.normal"
		if unique || st.Legendary.PowerID != "" {
			op = "life.salvage.unique"
		}
		id := int32(0)
		for _, r := range ctx.Rules.Recipes() {
			if r.Operation != op {
				continue
			}
			if id != 0 || !r.TargetRequired || len(r.Materials) != 0 {
				return nil, cubeError("configuration", "炼方相争，暂不可炼。")
			}
			id = r.ID
		}
		if id == 0 {
			return nil, cubeError("no_match", "鼎中诸物，未合炼方。")
		}
		r := req
		r.RecipeID, r.TargetUID, r.CubeInputs = id, in.UID, nil
		// The original immutable bag validates every socket UID and target,
		// including duplicates across different selected items, before any roll.
		if _, err := planCubeMode(ctx, r, false, true); err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}
	return requests, nil
}

func planBatchSalvage(ctx CubeContext, req EndgameRequest, execute bool) (CubePlan, error) {
	p := CubePlan{Money: ctx.Money, Fingerprint: CubeFingerprint(ctx.Bag, ctx.Money, req)}
	requests, err := batchSalvageRequests(ctx, req)
	if err != nil {
		return p, err
	}
	selectedRecipe := requests[0].RecipeID
	for _, r := range requests {
		d, _ := ctx.Rules.Recipe(r.RecipeID)
		if d.Operation == "life.salvage.unique" {
			selectedRecipe = r.RecipeID
			break
		}
	}
	if req.RecipeID != selectedRecipe || req.TargetUID != requests[0].TargetUID {
		return p, cubeError("invalid_request", "分解参数与投料不符")
	}
	d, _ := ctx.Rules.Recipe(selectedRecipe)
	p.Recipe = cubeRecipeView(d, ctx.Items)
	p.Recipe.Name = "批量分解"
	p.Description = fmt.Sprintf("分解所投%d件装备，按各自等级、品质与部位获得原矿，并返还镶嵌卡片。矿种随等级随机，数量依品质与部位而定。", len(requests))
	p.Recipe.Description = p.Description
	p.Bag = ctx.Bag.Clone()
	slots := make([]int, len(requests))
	targets := make([]Stack, len(requests))
	for i, r := range requests {
		uid := parseCubeUID(r.TargetUID)
		p.Bag.Each(func(slot int, st Stack) {
			if !st.Empty() && st.UID == uid {
				slots[i], targets[i] = slot, st
			}
		})
		p.Bag.Set(slots[i], Stack{})
		targetDef := ctx.Items[targets[i].Item]
		p.Materials = append(p.Materials, EndgameMaterialView{ItemID: int32(targets[i].Item), Count: 1, Owned: 1, Name: targetDef.Name})
	}
	// All targets release their slots before returned cards/output capacity is
	// checked. PlanCube performs this entire conservative pass before RNG runs.
	ores := map[ItemID]int32{}
	for i, r := range requests {
		p.Bag.Set(slots[i], targets[i])
		partCtx := ctx
		partCtx.Bag, partCtx.Money = p.Bag, p.Money
		part, err := planCube(partCtx, r, execute)
		if err != nil {
			return CubePlan{}, err
		}
		p.Bag.Set(slots[i], Stack{})
		p.Money = part.Money
		p.ConsumedUIDs = append(p.ConsumedUIDs, part.ConsumedUIDs...)
		for _, out := range part.Outputs {
			if out.UID == 0 {
				ores[out.Item] += out.Count
			} else {
				p.Outputs = append(p.Outputs, out)
			}
		}
	}
	ids := make([]int, 0, len(ores))
	for id := range ores {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		p.Outputs = append(p.Outputs, NormalizeItemBinding(ctx.Items[ItemID(id)], Stack{Item: ItemID(id), Count: ores[ItemID(id)], Bound: true}))
	}
	for _, out := range p.Outputs {
		def, exists := ctx.Items[out.Item]
		if !exists || !def.InventoryTabKnown {
			return CubePlan{}, cubeError("requirements", "产物模板缺失")
		}
		if p.Bag.AddStack(def, out) != 0 {
			return CubePlan{}, cubeError("bag_full", "背包空间不足，未消耗任何物品")
		}
	}
	p.Recipe.Money = strconv.FormatInt(ctx.Money-p.Money, 10)
	return p, nil
}
