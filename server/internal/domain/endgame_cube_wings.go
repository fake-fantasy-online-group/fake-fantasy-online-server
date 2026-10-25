package domain

import "strconv"

// Wings reserve the same owned bag identities as every other cube recipe. They
// are matched before the legacy one-gear matcher, never by auto-selecting a bag item.
func matchWingCubeInputs(ctx CubeContext, input EndgameRequest, materials map[ItemID]int32, gears, runes []string) ([]EndgameRequest, []CubePlan, bool, error) {
	if ctx.WingRules == nil {
		return nil, nil, false, nil
	}
	var wing string
	var source string
	var wingItem ItemID
	for _, raw := range gears {
		_, st, err := wingCubeTarget(ctx, raw)
		if err != nil {
			return nil, nil, true, err
		}
		if _, ok := ctx.WingRules.Item(st.Item); ok {
			if wing != "" {
				return nil, nil, true, cubeError("partial_match", "部分材料相合，尚需调配。")
			}
			wing, wingItem = raw, st.Item
		} else {
			source = raw
		}
	}

	feather := false
	for _, id := range ctx.WingRules.config.FeatherIDs {
		feather = feather || materials[id] > 0
	}
	if len(gears) == 2 && len(materials) == 0 && len(runes) == 0 && len(input.ItemUIDs) == 0 {
		req := input
		req.CubeInputs = nil
		req.RecipeID = WingPowerRecipeID
		req.TargetUID = wing
		req.ItemUIDs = []string{source}
		plan, err := planWingCube(ctx, req)
		return []EndgameRequest{req}, []CubePlan{plan}, true, err
	}
	if wing == "" && (!feather || len(gears) > 0 || len(runes) > 0) {
		return nil, nil, false, nil
	}
	if !feather && len(gears) == 1 {
		return nil, nil, false, nil
	}
	if len(gears) > 1 || len(runes) != 0 || len(input.ItemUIDs) != 0 {
		return nil, nil, true, cubeError("partial_match", "部分材料相合，尚需调配。")
	}
	short := false
	professionNeeded := false
	for _, recipe := range ctx.WingRules.Recipes() {
		if recipe.Source != wingItem {
			continue
		}
		exact, sameTypes, under, over := len(materials) == len(recipe.Materials), len(materials) == len(recipe.Materials), false, false
		for _, m := range recipe.Materials {
			n := materials[m.ItemID]
			exact = exact && n == m.Count
			sameTypes = sameTypes && n > 0
			under = under || n < m.Count
			over = over || n > m.Count
		}
		short = short || (sameTypes && under && !over)
		if !exact {
			continue
		}
		if recipe.ProfessionMask != 0 {
			if ctx.Character == nil || !ctx.Character.HasProfession() || !ctx.Character.Race.Valid() {
				professionNeeded = true
				continue
			}
			if recipe.ProfessionMask&(1<<uint8(ctx.Character.Race)) == 0 {
				continue
			}
		}
		req := input
		req.CubeInputs = nil
		req.RecipeID = recipe.ID
		req.TargetUID = wing
		plan, err := planWingCube(ctx, req)
		return []EndgameRequest{req}, []CubePlan{plan}, true, err
	}
	if professionNeeded {
		return nil, nil, true, cubeError("requirements", "请先确定职业，再以周天鼎炼成适合你的羽翼。")
	}
	if short {
		return nil, nil, true, cubeError("insufficient_count", "材料虽合，数目尚缺。")
	}
	return nil, nil, true, cubeError("partial_match", "部分材料相合，尚需调配。")
}

func wingCubeTarget(ctx CubeContext, raw string) (int, Stack, error) {
	uid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || uid <= 0 || strconv.FormatInt(uid, 10) != raw || ctx.Bag == nil {
		return -1, Stack{}, cubeError("invalid_request", "投料实例无效或重复")
	}
	slot := -1
	var found Stack
	duplicate := false
	seen := map[int64]bool{}
	ctx.Bag.Each(func(i int, st Stack) {
		if st.Empty() {
			return
		}
		for _, id := range append([]int64{st.UID}, st.SocketUIDs[:]...) {
			if id > 0 {
				duplicate = duplicate || seen[id]
				seen[id] = true
			}
		}
		if st.UID == uid {
			slot, found = i, st
		}
	})
	if duplicate {
		return -1, Stack{}, cubeError("invalid_request", "背包中存在重复物品实例")
	}
	if slot < 0 {
		return -1, Stack{}, cubeError("not_owner", "投料物品已不在自己的背包")
	}
	def, ok := ctx.Items[found.Item]
	if !ok || def.Equip == nil || !def.InventoryTabKnown || def.InstanceKind != ItemInstanceEquipment || (found.InstanceKind != ItemInstanceNone && found.InstanceKind != ItemInstanceEquipment) || found.Count != 1 || !ctx.Bag.AllowsSlot(def, slot) {
		return -1, Stack{}, cubeError("requirements", "请将未锁定装备放在装备背包中")
	}
	if found.Locked {
		return -1, Stack{}, cubeError("locked", "投料物品不可用或已锁定")
	}
	if err := cubeValidateSockets(found, def, ctx.Items); err != nil {
		return -1, Stack{}, err
	}
	found.InstanceKind = ItemInstanceEquipment // legacy location rows retain kind zero; template and UID prove equipment
	return slot, found, nil
}

func planWingCube(ctx CubeContext, req EndgameRequest) (CubePlan, error) {
	p := CubePlan{Money: ctx.Money, Fingerprint: CubeFingerprint(ctx.Bag, ctx.Money, req)}
	if ctx.WingRules == nil || ctx.Bag == nil {
		return p, cubeError("configuration", "羽翼炼方尚未载入")
	}
	if req.Socket != 0 || req.Tier != 0 || req.RunID != "" || req.RuneUID != "" || len(req.CubeInputs) > 0 || ctx.Money < 0 {
		return p, cubeError("invalid_request", "周天鼎参数不适用于该配方")
	}
	if recipe, ok := ctx.WingRules.Recipe(req.RecipeID); ok && recipe.Operation == "wing.feather" {
		return planWingFeather(ctx, req, recipe, p)
	}
	slot, target, err := wingCubeTarget(ctx, req.TargetUID)
	if err != nil {
		return p, err
	}
	if _, ok := ctx.WingRules.Item(target.Item); !ok {
		return p, cubeError("requirements", "此物并非成长羽翼")
	}
	p.Bag = ctx.Bag.Clone()
	if req.RecipeID == WingPowerRecipeID {
		if len(req.ItemUIDs) != 1 || req.ItemUIDs[0] == req.TargetUID {
			return p, cubeError("partial_match", "部分材料相合，尚需调配。")
		}
		sourceSlot, source, err := wingCubeTarget(ctx, req.ItemUIDs[0])
		if err != nil {
			return p, err
		}
		if _, ok := ctx.WingRules.Item(source.Item); ok {
			return p, cubeError("partial_match", "部分材料相合，尚需调配。")
		}
		target, err = ExtractWingPower(target, source, ctx.Items[source.Item])
		if err != nil {
			return p, cubeError("requirements", err.Error())
		}
		p.Bag.Set(sourceSlot, Stack{})
		p.ConsumedUIDs = []int64{source.UID}
		p.Recipe = EndgameRecipeView{ID: WingPowerRecipeID, Name: "羽翼融威", Operation: "wing.power", TargetRequired: true, Money: "0", Description: "消耗暗金器物，将其威能融入羽翼；仅覆盖同类威能，其他两类保留。"}
	} else {
		recipe, ok := ctx.WingRules.Recipe(req.RecipeID)
		if !ok || recipe.Source != target.Item {
			return p, cubeError("requirements", "这副羽翼尚不合此炼方")
		}
		if recipe.ProfessionMask != 0 && (ctx.Character == nil || !ctx.Character.HasProfession() || !ctx.Character.Race.Valid() || recipe.ProfessionMask&(1<<uint8(ctx.Character.Race)) == 0) {
			return p, cubeError("requirements", "请先确定职业，再以周天鼎炼成适合你的羽翼。")
		}
		if len(req.ItemUIDs) != 0 {
			return p, cubeError("invalid_request", "额外投料参数无效")
		}
		def, ok := ctx.Items[recipe.Target]
		if !ok || def.Equip == nil {
			return p, cubeError("configuration", "羽翼模板尚未载入")
		}
		p.Recipe = EndgameRecipeView{ID: recipe.ID, Name: "羽翼升阶", Operation: recipe.Operation, TargetRequired: true, Money: "0", OutputItemID: int32(recipe.Target), Description: "保留羽翼实例、强化、镶嵌与三类威能；目标原有穿戴条件不变。"}
		if recipe.Operation == "wing.change" {
			p.Recipe.Name = "羽翼换系"
		}
		for _, m := range recipe.Materials {
			v := EndgameMaterialView{ItemID: int32(m.ItemID), Name: ctx.Items[m.ItemID].Name, Count: m.Count, Owned: ctx.Bag.UsableCountOf(m.ItemID)}
			p.Materials = append(p.Materials, v)
			p.Recipe.Materials = append(p.Recipe.Materials, v)
			if !p.Bag.Remove(m.ItemID, m.Count) {
				return p, cubeError("insufficient_count", "材料虽合，数目尚缺。")
			}
		}
		oldMax := target.MaxDurability
		if oldMax <= 0 {
			oldMax = ctx.Items[target.Item].Equip.Durable
		}
		newMax := def.Equip.Durable
		if oldMax > 0 && newMax > 0 && oldMax != newMax {
			target.Durability = int32(int64(target.Durability) * int64(newMax) / int64(oldMax))
			target.MaxDurability = newMax
		}
		target.Item = recipe.Target
	}
	target.Bound = true
	target.Endgame.Revision++
	if !p.Bag.Set(slot, target) {
		return p, cubeError("stale_revision", "羽翼背包位置已变化")
	}
	p.Target = target
	p.Description = p.Recipe.Description
	return p, nil
}

func planWingFeather(ctx CubeContext, req EndgameRequest, recipe WingCubeRecipeDef, p CubePlan) (CubePlan, error) {
	if req.TargetUID != "" || len(req.ItemUIDs) != 0 {
		return p, cubeError("invalid_request", "额外投料参数无效")
	}
	p.Bag = ctx.Bag.Clone()
	p.Recipe = EndgameRecipeView{ID: recipe.ID, Name: "灵羽互兑", Operation: "wing.feather", Money: "0", Description: "按既定面额互兑同系灵羽，不改变总数额。"}
	for _, m := range recipe.Materials {
		v := EndgameMaterialView{ItemID: int32(m.ItemID), Name: ctx.Items[m.ItemID].Name, Count: m.Count, Owned: ctx.Bag.UsableCountOf(m.ItemID)}
		p.Materials = append(p.Materials, v)
		p.Recipe.Materials = append(p.Recipe.Materials, v)
		if !p.Bag.Remove(m.ItemID, m.Count) {
			return p, cubeError("insufficient_count", "材料虽合，数目尚缺。")
		}
	}
	for _, o := range recipe.Outputs {
		st := Stack{Item: o.ItemID, Count: o.Count, Bound: true}
		if p.Bag.AddStack(ctx.Items[o.ItemID], st) != 0 {
			return p, cubeError("bag_full", "背包空间不足，未消耗任何物品")
		}
		p.Outputs = append(p.Outputs, st)
	}
	p.Description = p.Recipe.Description
	return p, nil
}
