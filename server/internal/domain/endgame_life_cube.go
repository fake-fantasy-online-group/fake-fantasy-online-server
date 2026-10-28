package domain

import (
	"strconv"
	"strings"
)

// Life recipes use the existing cube reservation protocol. Counts supplied by
// the client are presentation only; spending is bounded by selected identities.
func matchLifeCubeInputs(ctx CubeContext, input EndgameRequest) ([]EndgameRequest, []CubePlan, bool, error) {
	active := false
	for _, r := range ctx.Rules.Recipes() {
		if strings.HasPrefix(r.Operation, "life.") {
			active = true
			break
		}
	}
	if !active {
		return nil, nil, false, nil
	}
	fail := func(code, msg string) ([]EndgameRequest, []CubePlan, bool, error) {
		return nil, nil, true, cubeError(code, msg)
	}
	mats := map[ItemID]int32{}
	seen := map[int64]bool{}
	var gear, card string
	special := false
	for _, in := range input.CubeInputs {
		if in.UID == "" {
			d, ok := ctx.Items[in.ItemID]
			if !ok || !d.Stackable || d.InstanceKind != ItemInstanceNone || !d.InventoryTabKnown || mats[in.ItemID] != 0 {
				return fail("invalid_request", "投料材料无效或重复")
			}
			n := ctx.Bag.UsableCountOf(in.ItemID)
			if n <= 0 {
				return fail("materials", "投料材料不足或已锁定")
			}
			mats[in.ItemID] = n
			continue
		}
		uid, e := strconv.ParseInt(in.UID, 10, 64)
		if e != nil || uid <= 0 || strconv.FormatInt(uid, 10) != in.UID || in.ItemID != 0 || seen[uid] {
			return fail("invalid_request", "投料实例无效或重复")
		}
		seen[uid] = true
		var st Stack
		found := 0
		ctx.Bag.Each(func(_ int, s Stack) {
			if s.UID == uid && !s.Empty() {
				st = s
				found++
			}
		})
		if found != 1 {
			return fail("not_owner", "投料物品已不在自己的背包")
		}
		d, ok := ctx.Items[st.Item]
		if !ok || st.Locked || st.Card.Locked {
			return fail("locked", "投料物品不可用或已锁定")
		}
		switch {
		case d.Equip != nil && d.InstanceKind == ItemInstanceEquipment:
			if gear != "" {
				return nil, nil, false, nil
			}
			gear = in.UID
		case st.Item >= FirstAstralRune && st.Item <= LastAstralRune:
			if !IsAstralRuneStack(st) || !st.Card.Initialized || st.Card.Count > 8 || d.InstanceKind != ItemInstanceSocketCard {
				return fail("invalid_rune", "星篆状态无效")
			}
			mats[st.Item] += st.Count
			special = true
		case d.InstanceKind == ItemInstanceSocketCard:
			if card != "" || st.Count != 1 || !st.Card.Initialized || st.Card.Count > 8 {
				return fail("requirements", "只可投入一张引灵卡片")
			}
			card = in.UID
			special = true
		default:
			return nil, nil, false, nil
		}
	}
	var choices []EndgameRequest
	var plans []CubePlan
	partial := false
	for _, r := range ctx.Rules.Recipes() {
		if !strings.HasPrefix(r.Operation, "life.") || gear == "" {
			continue
		}
		same := len(mats) == len(r.Materials)
		short := false
		for _, m := range r.Materials {
			n := mats[m.ItemID]
			if n > 0 {
				partial = true
			}
			if n == 0 {
				same = false
			}
			if n < m.Count {
				short = true
			}
		}
		if !same || card != "" && r.Operation != "life.affix.type" {
			continue
		}
		if short {
			return fail("insufficient_count", "材料虽合，数目尚缺。")
		}
		req := input
		req.RecipeID = r.ID
		req.TargetUID = gear
		req.ItemUIDs = nil
		if card != "" {
			req.ItemUIDs = []string{card}
		}
		plan, err := PlanCube(ctx, req, false)
		if err != nil {
			if strings.HasPrefix(r.Operation, "life.salvage.") {
				continue
			}
			return nil, nil, true, err
		}
		choices = append(choices, req)
		plans = append(plans, plan)
	}
	if len(choices) > 1 {
		return fail("configuration", "炼方相争，暂不可炼。")
	}
	if len(choices) == 1 {
		if len(input.ItemUIDs) > 0 {
			return fail("invalid_request", "额外投料参数无效")
		}
		return choices, plans, true, nil
	}
	if special && gear != "" {
		if partial {
			return fail("partial_match", "部分材料相合，尚需调配。")
		}
		return fail("no_match", "鼎中诸物，未合炼方。")
	}
	return nil, nil, false, nil
}

// Consume only selected rune identities (not another locked/different stack of
// the same rune). Legacy recipe requests without reservations are disallowed.
func lifeConsumeRunes(ctx CubeContext, p *CubePlan, req EndgameRequest, id ItemID, count int32, diagnostic bool) (int32, error) {
	d, exists := ctx.Items[id]
	if !exists || !d.InventoryTabKnown || d.InstanceKind != ItemInstanceSocketCard || !d.Stackable {
		return 0, cubeError("invalid_rune", "星篆模板无效")
	}
	var owned int32
	seen := map[int64]bool{}
	for _, in := range req.CubeInputs {
		uid := parseCubeUID(in.UID)
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		ctx.Bag.Each(func(slot int, st Stack) {
			if ctx.Bag.AllowsSlot(d, slot) && st.InstanceKind == ItemInstanceSocketCard && st.UID == uid && st.Item == id && IsAstralRuneStack(st) && !st.Locked && !st.Card.Locked {
				owned += st.Count
			}
		})
	}
	if owned < count && !diagnostic {
		return owned, cubeError("insufficient_count", "材料虽合，数目尚缺。")
	}
	if diagnostic {
		return owned, nil
	}
	remain := count
	for _, in := range req.CubeInputs {
		uid := parseCubeUID(in.UID)
		p.Bag.Each(func(slot int, st Stack) {
			if remain > 0 && p.Bag.AllowsSlot(d, slot) && st.InstanceKind == ItemInstanceSocketCard && st.UID == uid && st.Item == id && IsAstralRuneStack(st) && !st.Locked && !st.Card.Locked {
				n := min(remain, st.Count)
				p.Bag.RemoveAt(slot, n)
				remain -= n
				if n == st.Count {
					p.ConsumedUIDs = append(p.ConsumedUIDs, uid)
				}
			}
		})
	}
	if remain != 0 {
		return owned, cubeError("requirements", "星篆状态已变化")
	}
	return owned, nil
}

func lifeApplyCubeAffix(ctx CubeContext, req EndgameRequest, recipe CubeRecipeDef, target Stack, def ItemDef, p *CubePlan, execute, diagnostic bool) (Stack, error) {
	var rng func(int) int
	if execute {
		rng = ctx.Intn
		if rng == nil {
			return target, cubeError("configuration", "炼化机缘尚未就绪")
		}
	}
	slot, grade := 0, 1
	if recipe.Operation == "life.affix.type" || recipe.Operation == "life.affix.value" {
		if len(recipe.Materials) == 0 {
			return target, cubeError("configuration", "星篆炼方无效")
		}
		id := recipe.Materials[0].ItemID
		if id < FirstAstralRune || id > LastAstralRune {
			return target, cubeError("configuration", "星篆炼方无效")
		}
		slot = int((id - FirstAstralRune) / 7)
		grade = int((id-FirstAstralRune)%7) + 1
	}
	var bias map[int32]int
	if len(req.ItemUIDs) > 0 {
		if recipe.Operation != "life.affix.type" || len(req.ItemUIDs) != 1 {
			return target, cubeError("invalid_request", "引灵卡片投料无效")
		}
		uid := parseCubeUID(req.ItemUIDs[0])
		if uid <= 0 || strconv.FormatInt(uid, 10) != req.ItemUIDs[0] {
			return target, cubeError("invalid_request", "引灵卡片身份无效")
		}
		found := -1
		var card Stack
		p.Bag.Each(func(i int, st Stack) {
			if st.UID == uid {
				found = i
				card = st
			}
		})
		d, ok := ctx.Items[card.Item]
		if found < 0 || !ok || !d.InventoryTabKnown || !p.Bag.AllowsSlot(d, found) || card.InstanceKind != ItemInstanceSocketCard || d.InstanceKind != ItemInstanceSocketCard || card.Count != 1 || card.Item >= FirstAstralRune && card.Item <= LastAstralRune || card.Locked || card.Card.Locked || !card.Card.Initialized || card.Card.Count > 8 {
			return target, cubeError("requirements", "引灵卡片状态无效")
		}
		bias = LifeCardBias(d, card)
		if len(bias) == 0 {
			return target, cubeError("requirements", "此卡未藏可引之属性")
		}
		p.Materials = append(p.Materials, EndgameMaterialView{ItemID: int32(card.Item), Name: d.Name, Count: 1, Owned: 1})
		if !diagnostic {
			p.Bag.RemoveAt(found, 1)
			p.ConsumedUIDs = append(p.ConsumedUIDs, uid)
		}
	}
	var out Stack
	var err error
	switch recipe.Operation {
	case "life.affix.type":
		out, err = LifeRerollAffix(target, def, ctx.EquipmentRoll, slot, 1, bias, rng)
	case "life.affix.value":
		out, err = LifeRerollAffixValue(target, slot, grade, rng)
	case "life.affix.shuffle":
		out, err = LifeShuffleAffixes(target, rng)
	case "life.affix.temper":
		out, err = LifeTemperAffix(target, rng)
	default:
		return target, cubeError("configuration", "炼方未明")
	}
	if err != nil {
		return target, cubeError("requirements", err.Error())
	}
	return out, nil
}
