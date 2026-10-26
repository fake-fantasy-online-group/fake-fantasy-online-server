package domain

import (
	"fmt"
	"sort"
	"strconv"
)

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
	var candidates []WingCubeRecipeDef
	professionNeeded := false
	for _, recipe := range ctx.WingRules.Recipes() {
		if recipe.Source != wingItem {
			continue
		}
		_, enough, short := chooseWingMaterials(ctx.WingRules, recipe, materials)
		if !enough && !short {
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
		candidates = append(candidates, recipe)
	}
	if len(candidates) > 1 {
		declared := map[ItemID]int32{}
		for _, in := range input.CubeInputs {
			if in.UID == "" {
				declared[in.ItemID] = in.Count
			}
		}
		var exact []WingCubeRecipeDef
		for _, r := range candidates {
			if ok, _ := wingMaterialsMatch(ctx.WingRules, r, declared); ok {
				exact = append(exact, r)
			}
		}
		if len(exact) != 1 {
			return nil, nil, true, cubeError("configuration", "这些投料匹配了不同炼化方式，请调整数量以明炼意。")
		}
		candidates = exact
	}
	if len(candidates) == 1 {
		recipe := candidates[0]
		chosen, enough, _ := chooseWingMaterials(ctx.WingRules, recipe, materials)
		if !enough {
			return nil, nil, true, cubeError("insufficient_count", "材料虽合，数目尚缺。")
		}
		req := input
		req.CubeInputs = nil
		req.RecipeID = recipe.ID
		req.TargetUID = wing
		if recipe.Operation != "wing.feather" {
			for _, m := range sortedWingMaterials(chosen) {
				req.CubeInputs = append(req.CubeInputs, CubeInput{ItemID: m.ItemID, Count: m.Count})
			}
		}
		plan, err := planWingCube(ctx, req)
		return []EndgameRequest{req}, []CubePlan{plan}, true, err
	}
	if professionNeeded {
		return nil, nil, true, cubeError("requirements", "请先确定职业，再以周天鼎炼成适合你的羽翼。")
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
	if req.Socket != 0 || req.Tier != 0 || req.RunID != "" || req.RuneUID != "" || ctx.Money < 0 {
		return p, cubeError("invalid_request", "周天鼎参数不适用于该配方")
	}
	if recipe, ok := ctx.WingRules.Recipe(req.RecipeID); ok && recipe.Operation == "wing.feather" {
		if len(req.CubeInputs) != 0 {
			return p, cubeError("invalid_request", "额外投料参数无效")
		}
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
		if len(req.CubeInputs) != 0 {
			return p, cubeError("invalid_request", "额外投料参数无效")
		}
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
		consumed := recipe.Materials
		if len(req.CubeInputs) > 0 {
			selected := map[ItemID]int32{}
			if len(req.CubeInputs) > 4 {
				return p, cubeError("invalid_request", "投料材料无效或重复")
			}
			for _, in := range req.CubeInputs {
				d, ok := ctx.Items[in.ItemID]
				if !ok || !d.Stackable || d.InstanceKind != ItemInstanceNone || !d.InventoryTabKnown || in.UID != "" || in.Count <= 0 || selected[in.ItemID] != 0 {
					return p, cubeError("invalid_request", "投料材料无效或重复")
				}
				if ctx.Bag.UsableCountOf(in.ItemID) < in.Count {
					return p, cubeError("materials", "投料材料不足或已锁定")
				}
				selected[in.ItemID] = in.Count
			}
			payment, exact, short := chooseWingMaterials(ctx.WingRules, recipe, selected)
			exact = exact && sameWingPayment(payment, selected)
			if !exact {
				if short {
					return p, cubeError("insufficient_count", "材料虽合，数目尚缺。")
				}
				return p, cubeError("partial_match", "部分材料相合，尚需调配。")
			}
			consumed = sortedWingMaterials(selected)
		}
		refundBound := false
		for _, m := range consumed {
			remaining := m.Count
			p.Bag.Each(func(_ int, st Stack) {
				if remaining > 0 && st.Item == m.ItemID && !st.Locked {
					take := min(remaining, st.Count)
					if take > 0 && st.Bound {
						refundBound = true
					}
					remaining -= take
				}
			})
			v := EndgameMaterialView{ItemID: int32(m.ItemID), Name: ctx.Items[m.ItemID].Name, Count: m.Count, Owned: ctx.Bag.UsableCountOf(m.ItemID)}
			p.Materials = append(p.Materials, v)
			p.Recipe.Materials = append(p.Recipe.Materials, v)
			if !p.Bag.Remove(m.ItemID, m.Count) {
				return p, cubeError("insufficient_count", "材料虽合，数目尚缺。")
			}
		}
		change := wingPaymentChange(recipe, consumed)
		if change > 0 {
			refund := Stack{Item: 25027, Count: change, Bound: refundBound}
			if p.Bag.AddStack(ctx.Items[25027], refund) != 0 {
				return p, cubeError("bag_full", "背包空间不足，未消耗任何物品")
			}
			p.Outputs = append(p.Outputs, refund)
			p.Recipe.Description += fmt.Sprintf(" 返还%s×%d。", ctx.Items[25027].Name, change)
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

// Common white feathers are denominations, not alternative authored recipes.
// A change-of-series marker remains one physical target-series feather even when
// it has the same ID as the lowest common denomination (dark -> light).
func wingMaterialsMatch(rules *WingProgressionRules, recipe WingCubeRecipeDef, selected map[ItemID]int32) (exact, short bool) {
	if recipe.Operation == "wing.feather" {
		exact, same, under, over := len(selected) == len(recipe.Materials), len(selected) == len(recipe.Materials), false, false
		for _, m := range recipe.Materials {
			n := selected[m.ItemID]
			exact = exact && n == m.Count
			same = same && n > 0
			under, over = under || n < m.Count, over || n > m.Count
		}
		return exact, same && under && !over
	}
	var marker ItemID
	if recipe.Operation == "wing.change" {
		target, ok := rules.Item(recipe.Target)
		if !ok {
			return false, false
		}
		switch target.Series {
		case "light":
			marker = 25027
		case "dark":
			marker = 11682
		default:
			return false, false
		}
	}
	value := func(mats map[ItemID]int32) (int64, bool) {
		if marker != 0 && mats[marker] < 1 {
			return 0, false
		}
		var total int64
		for id, n := range mats {
			if n <= 0 {
				return 0, false
			}
			if id == marker {
				n--
			}
			if n == 0 {
				continue
			}
			var unit int64
			switch id {
			case 25027:
				unit = 1
			case 25028:
				unit = 5
			case 25029:
				unit = 25
			default:
				return 0, false
			}
			total += int64(n) * unit
		}
		return total, true
	}
	want := map[ItemID]int32{}
	for _, m := range recipe.Materials {
		want[m.ItemID] = m.Count
	}
	need, valid := value(want)
	got, allowed := value(selected)
	if !valid || !allowed || need <= 0 {
		return false, false
	}
	return got == need, got < need
}

func sortedWingMaterials(selected map[ItemID]int32) []CubeMaterialDef {
	out := make([]CubeMaterialDef, 0, len(selected))
	for id, n := range selected {
		out = append(out, CubeMaterialDef{ItemID: id, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID < out[j].ItemID })
	return out
}

// Select an exact payment from the selected material types only. The public
// input count is never used as an inventory reservation or an operation switch.
func chooseWingMaterials(rules *WingProgressionRules, recipe WingCubeRecipeDef, available map[ItemID]int32) (map[ItemID]int32, bool, bool) {
	if recipe.Operation == "wing.feather" {
		if len(available) != len(recipe.Materials) {
			return nil, false, false
		}
		out := map[ItemID]int32{}
		short := false
		for _, m := range recipe.Materials {
			if available[m.ItemID] <= 0 {
				return nil, false, false
			}
			short = short || available[m.ItemID] < m.Count
			out[m.ItemID] = m.Count
		}
		return out, !short, short
	}
	var marker ItemID
	if recipe.Operation == "wing.change" {
		target, ok := rules.Item(recipe.Target)
		if !ok {
			return nil, false, false
		}
		switch target.Series {
		case "light":
			marker = 25027
		case "dark":
			marker = 11682
		default:
			return nil, false, false
		}
		if available[marker] < 1 {
			return nil, false, false
		}
	}
	amounts := map[ItemID]int32{}
	for id, n := range available {
		if id != 25027 && id != 25028 && id != 25029 && id != marker {
			return nil, false, false
		}
		amounts[id] = n
	}
	if marker != 0 {
		amounts[marker]--
	}
	var need int64
	for _, m := range recipe.Materials {
		n := int64(m.Count)
		if m.ItemID == marker {
			n--
		}
		switch m.ItemID {
		case 25027:
			need += n
		case 25028:
			need += n * 5
		case 25029:
			need += n * 25
		}
	}
	total := int64(amounts[25027]) + int64(amounts[25028])*5 + int64(amounts[25029])*25
	if total < int64(need) {
		return nil, false, true
	}
	// Search at most 25 values. For a fixed value, maximal 25s then 5s
	// minimizes both item count and required 1s; reducing either cannot improve
	// feasibility in this divisible denomination system.
	for value := need; value < need+25; value++ {
		h := min(int64(amounts[25029]), value/25)
		left := value - h*25
		m := min(int64(amounts[25028]), left/5)
		l := left - m*5
		if l > int64(amounts[25027]) {
			continue
		}
		out := map[ItemID]int32{}
		if h > 0 {
			out[25029] = int32(h)
		}
		if m > 0 {
			out[25028] = int32(m)
		}
		if l > 0 {
			out[25027] = int32(l)
		}
		if marker != 0 {
			out[marker]++
		}
		return out, true, false
	}

	return nil, false, false // unavailable selected denomination combination
}

func sameWingPayment(a, b map[ItemID]int32) bool {
	if len(a) != len(b) {
		return false
	}
	for id, n := range a {
		if b[id] != n {
			return false
		}
	}
	return true
}
func wingPaymentChange(recipe WingCubeRecipeDef, payment []CubeMaterialDef) int32 {
	value := func(mats []CubeMaterialDef) int64 {
		var n int64
		for _, m := range mats {
			switch m.ItemID {
			case 25027:
				n += int64(m.Count)
			case 25028:
				n += int64(m.Count) * 5
			case 25029:
				n += int64(m.Count) * 25
			}
		}
		return n
	}
	return int32(value(payment) - value(recipe.Materials))
}
