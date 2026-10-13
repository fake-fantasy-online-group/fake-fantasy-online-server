package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type CubeContext struct {
	Rules        *CubeRuleTable
	Catalog      *EndgameCatalog
	Level        int32
	Bag          *Bag
	Money        int64
	Items        map[ItemID]ItemDef
	IsUniqueBase func(ItemID) bool
	Intn         func(int) int
	NewUID       func() int64
}
type CubePlan struct {
	Bag                      *Bag
	Money                    int64
	Recipe                   EndgameRecipeView
	Materials                []EndgameMaterialView
	Outputs                  []Stack
	Target                   Stack
	Description, Fingerprint string
	ConsumedUIDs             []int64
}
type CubeError struct{ Code, Message string }

func (e *CubeError) Error() string         { return e.Message }
func cubeError(code, message string) error { return &CubeError{code, message} }
func CubeErrorCode(err error) string {
	if e, ok := err.(*CubeError); ok {
		return e.Code
	}
	return "invalid_request"
}

// Includes native mutations that do not advance the sidecar revision.
func CubeFingerprint(bag *Bag, money int64, req EndgameRequest) string {
	var slots []Stack
	if bag != nil {
		for i := 0; i < bag.Cap(); i++ {
			slots = append(slots, bag.At(i))
		}
	}
	b, _ := json.Marshal(struct {
		Slots        []Stack
		Money        int64
		Recipe       int32
		Target, Rune string
		Sources      []string
		Inputs       []CubeInput
	}{slots, money, req.RecipeID, req.TargetUID, req.RuneUID, req.ItemUIDs, req.CubeInputs})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Capacity and costs are checked without RNG or permanent UID allocation first.
// execute still produces only a candidate for the actor's atomic transaction.
func PlanCube(ctx CubeContext, req EndgameRequest, execute bool) (CubePlan, error) {
	p, err := planCube(ctx, req, false)
	if err != nil || !execute {
		return p, err
	}
	return planCube(ctx, req, true)
}
func planCube(ctx CubeContext, req EndgameRequest, execute bool) (CubePlan, error) {
	p := CubePlan{Money: ctx.Money, Fingerprint: CubeFingerprint(ctx.Bag, ctx.Money, req)}
	if ctx.Rules == nil {
		return p, cubeError("configuration", "周天鼎配置尚未载入")
	}
	recipe, exists := ctx.Rules.Recipe(req.RecipeID)
	if !exists {
		return p, cubeError("not_found", "周天鼎配方不存在")
	}
	if ctx.Bag == nil || ctx.Money < 0 {
		return p, cubeError("requirements", "背包或铜币状态无效")
	}
	p.Bag = ctx.Bag.Clone()
	owned := map[int64]bool{}
	duplicate := false
	ctx.Bag.Each(func(_ int, st Stack) {
		if st.Empty() {
			return
		}
		for _, uid := range append([]int64{st.UID}, st.SocketUIDs[:]...) {
			if uid <= 0 {
				continue
			}
			if owned[uid] {
				duplicate = true
			}
			owned[uid] = true
		}
	})
	if duplicate {
		return p, cubeError("invalid_request", "背包中存在重复物品实例")
	}
	find := func(raw string) (int, Stack, error) {
		uid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || uid <= 0 {
			return -1, Stack{}, cubeError("invalid_request", "物品实例号无效")
		}
		for i := 0; i < p.Bag.Cap(); i++ {
			st := p.Bag.At(i)
			if st.Empty() || st.UID != uid {
				continue
			}
			d, ok := ctx.Items[st.Item]
			if !ok || !d.InventoryTabKnown || !p.Bag.AllowsSlot(d, i) || st.Count != 1 {
				return -1, Stack{}, cubeError("requirements", "物品位置或模板无效")
			}
			if st.Locked || st.InstanceKind == ItemInstanceSocketCard && st.Card.Locked {
				return -1, Stack{}, cubeError("locked", "物品已锁定")
			}
			return i, st, nil
		}
		return -1, Stack{}, cubeError("not_owner", "物品不在自己的背包中")
	}
	runeValid := func(st Stack) bool {
		_, ok := AstralRune(st.Item, ctx.Catalog)
		d, exists := ctx.Items[st.Item]
		return ok && exists && d.InstanceKind == ItemInstanceSocketCard && st.InstanceKind == ItemInstanceSocketCard && st.UID > 0 && st.Card.Initialized && st.Card.Count <= 8 && st.Count == 1 && !st.Locked && !st.Card.Locked
	}
	inscribe, convert := recipe.Operation == "rune.inscribe", recipe.Operation == "rune.convert"
	var outputRune ItemID
	if inscribe || convert {
		outputRune = recipe.Outputs[0].ItemID
	}
	needsTarget := recipe.TargetRequired
	slot := -1
	var target Stack
	var def ItemDef
	if needsTarget {
		var err error
		slot, target, err = find(req.TargetUID)
		if err != nil {
			return p, err
		}
		def = ctx.Items[target.Item]
		if def.Equip == nil || def.InventoryTab != 2 || (target.InstanceKind != ItemInstanceNone && target.InstanceKind != ItemInstanceEquipment) {
			return p, cubeError("requirements", "请将未锁定装备放在装备背包中")
		}
		if err := cubeValidateSockets(target, def, ctx.Items); err != nil {
			return p, err
		}
		target.InstanceKind = ItemInstanceEquipment
	} else if req.TargetUID != "" {
		return p, cubeError("invalid_request", "此配方无需装备目标")
	}
	r := cubeRecipeView(recipe, ctx.Items)
	p.Recipe = r
	p.Description = r.Description
	if req.Socket != 0 || req.Tier != 0 || req.RunID != "" || len(req.ItemUIDs) > 5 {
		return p, cubeError("invalid_request", "周天鼎参数不适用于该配方")
	}
	usesSources := convert || recipe.Operation == "socket.reorder" || recipe.Operation == "socket.array"
	if !usesSources && len(req.ItemUIDs) > 0 || recipe.Operation != "salvage.rune" && req.RuneUID != "" {
		return p, cubeError("invalid_request", "额外投料参数无效")
	}
	fee := recipe.Money
	if ctx.Money < fee {
		return p, cubeError("funds", fmt.Sprintf("铜币不足：需要%d，当前%d", fee, ctx.Money))
	}
	for _, m := range r.Materials {
		d, exists := ctx.Items[ItemID(m.ItemID)]
		if !exists || !d.Stackable || d.InstanceKind != ItemInstanceNone || !d.InventoryTabKnown || m.Count <= 0 {
			return p, cubeError("requirements", "材料模板缺失")
		}
		m.Owned = ctx.Bag.UsableCountOf(ItemID(m.ItemID))
		p.Materials = append(p.Materials, m)
		if !p.Bag.Remove(ItemID(m.ItemID), m.Count) {
			return p, cubeError("materials", fmt.Sprintf("%s不足或已锁定：需要%d，当前可用%d", m.Name, m.Count, m.Owned))
		}
	}
	p.Money -= fee
	add := func(st Stack) error {
		d, exists := ctx.Items[st.Item]
		if !exists || !d.InventoryTabKnown {
			return cubeError("requirements", "产物模板缺失")
		}
		if p.Bag.AddStack(d, st) != 0 {
			return cubeError("bag_full", "背包空间不足，未消耗任何物品")
		}
		p.Outputs = append(p.Outputs, st)
		return nil
	}
	material := func(id ItemID, n int32) error { return add(Stack{Item: id, Count: n, Bound: true}) }
	newRune := func(id ItemID) error {
		d, exists := ctx.Items[id]
		if !exists || d.Stackable || d.InstanceKind != ItemInstanceSocketCard || !d.CardDefaults.Initialized {
			return cubeError("invalid_rune", "星篆模板无效")
		}
		st := Stack{Item: id, Count: 1, InstanceKind: ItemInstanceSocketCard, Card: d.CardDefaults, Bound: true}
		st.Card.Bound = true
		if execute {
			allocate := ctx.NewUID
			if allocate == nil {
				allocate = NewItemInstanceID
			}
			st.UID = allocate()
			if st.UID <= 0 || owned[st.UID] {
				return cubeError("invalid_request", "新物品实例号无效")
			}
		}
		return add(st)
	}
	extract := func() error {
		for i := 0; i < int(target.SocketCount); i++ {
			if target.Sockets[i] == 0 {
				continue
			}
			card := target.SocketCards[i]
			st := Stack{Item: target.Sockets[i], UID: target.SocketUIDs[i], Count: 1, InstanceKind: ItemInstanceSocketCard, Card: card, Bound: card.Bound, Locked: card.Locked}
			if err := add(st); err != nil {
				return err
			}
			target.Sockets[i] = 0
			target.SocketUIDs[i] = 0
			target.SocketCards[i] = CardInstanceState{}
		}
		return nil
	}
	_, hasUnique := UniqueOf(target, ctx.Catalog)
	eligible := func() bool {
		if _, ok := UniqueForBase(target.Item, ctx.Catalog); !ok {
			return false
		}
		if ctx.IsUniqueBase != nil {
			return ctx.IsUniqueBase(target.Item)
		}
		_, ok := UniqueForBase(target.Item, ctx.Catalog)
		return ok
	}
	switch recipe.Operation {
	case "salvage.normal", "salvage.unique":
		// V5 narrows new unique generation only. Keep the previously supported
		// ordinary and legacy-unique salvage scope independent of that list.
		_, salvageEligible := ctx.Catalog.LegacyUniqueForBase(target.Item)
		if ctx.IsUniqueBase != nil {
			salvageEligible = ctx.IsUniqueBase(target.Item)
		}
		if !salvageEligible || recipe.Operation == "salvage.normal" && (target.Endgame.UniqueID != 0 || hasUnique) || recipe.Operation == "salvage.unique" && !hasUnique {
			return p, cubeError("requirements", "此配方不适用于所选装备")
		}
		p.Bag.Set(slot, Stack{})
		p.ConsumedUIDs = append(p.ConsumedUIDs, target.UID)
		if err := extract(); err != nil {
			return p, err
		}
		for _, out := range recipe.Outputs {
			count, err := out.Quantity(def.Equip.LevelReq)
			if err != nil {
				return p, cubeError("configuration", "炼化产物数量无效")
			}
			if err := material(out.ItemID, count); err != nil {
				return p, err
			}
			p.Description += fmt.Sprintf(" 本次获得%s%d。", ctx.Items[out.ItemID].Name, count)
		}
		target = Stack{}
	case "salvage.rune":
		i, st, err := find(req.RuneUID)
		if err != nil {
			return p, err
		}
		if !runeValid(st) {
			return p, cubeError("invalid_rune", "请选择一枚背包星篆")
		}
		p.Bag.Set(i, Stack{})
		p.ConsumedUIDs = append(p.ConsumedUIDs, st.UID)
		for _, out := range recipe.Outputs {
			count, err := out.Quantity(0)
			if err != nil {
				return p, cubeError("configuration", "炼化产物数量无效")
			}
			if err = material(out.ItemID, count); err != nil {
				return p, err
			}
		}
	case "socket.open":
		capacity := min(5, EquipmentSocketCapacity(def, target))
		if target.SocketCount != 0 || capacity < 1 {
			return p, cubeError("requirements", "随机开窍仅限有剩余属性容量的无孔装备；已有孔不可重开")
		}
		p.Description += fmt.Sprintf(" 本次随机1至%d孔，每种孔数概率1/%d；不损毁装备。", capacity, capacity)
		if execute {
			if ctx.Intn == nil {
				return p, cubeError("requirements", "随机源不可用")
			}
			n := ctx.Intn(capacity)
			if n < 0 || n >= capacity {
				return p, cubeError("requirements", "随机源无效")
			}
			target.SocketCount = uint8(n + 1)
		}
	case "socket.extract":
		p.Description += fmt.Sprintf(" 本次原样取回%d枚孔内卡。", UsedEquipmentSockets(target))
		if UsedEquipmentSockets(target) == 0 {
			return p, cubeError("requirements", "没有可取回的孔内卡")
		}
		if err := extract(); err != nil {
			return p, err
		}
	case "socket.reorder":
		if reason := AstralBaseIneligibility(def, target); reason != "" {
			return p, cubeError("requirements", reason)
		}
		if target.SocketCount < 2 || len(req.ItemUIDs) != int(target.SocketCount) || UsedEquipmentSockets(target) != int(target.SocketCount) {
			return p, cubeError("requirements", "须按新顺序选择所有已镶满的星篆")
		}
		original := target
		seen := map[int64]bool{}
		for i, raw := range req.ItemUIDs {
			uid, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || uid <= 0 || seen[uid] {
				return p, cubeError("invalid_rune", "星篆顺序存在重复或无效实例")
			}
			seen[uid] = true
			found := -1
			for j := 0; j < int(original.SocketCount); j++ {
				if original.SocketUIDs[j] == uid {
					found = j
					break
				}
			}
			if found < 0 {
				return p, cubeError("not_owner", "所选星篆未镶在此装备")
			}
			if _, ok := AstralRune(original.Sockets[found], ctx.Catalog); !ok {
				return p, cubeError("invalid_rune", "移星换斗只接受星篆")
			}
			target.Sockets[i] = original.Sockets[found]
			target.SocketUIDs[i] = uid
			target.SocketCards[i] = original.SocketCards[found]
		}
		if ctx.Level < EquipmentRequiredLevel(def, target) {
			return p, cubeError("requirements", "角色等级未达到星篆或宿阵要求")
		}
		if target.SocketUIDs == original.SocketUIDs {
			return p, cubeError("requirements", "孔位顺序没有变化")
		}
	case "socket.array":
		if reason := AstralBaseIneligibility(def, target); reason != "" {
			return p, cubeError("requirements", reason)
		}
		if target.SocketCount < 2 || UsedEquipmentSockets(target) != 0 || len(req.ItemUIDs) != int(target.SocketCount) {
			return p, cubeError("requirements", "须选择与全部空孔数量相同的有序星篆")
		}
		for i, raw := range req.ItemUIDs {
			idx, st, err := find(raw)
			if err != nil {
				return p, err
			}
			if !runeValid(st) {
				return p, cubeError("invalid_rune", "只能用背包星篆布阵")
			}
			p.Bag.Set(idx, Stack{})
			card := st.Card
			card.Bound = st.Bound || card.Bound || target.Bound
			target.Bound = target.Bound || card.Bound
			target.Sockets[i] = st.Item
			target.SocketUIDs[i] = st.UID
			target.SocketCards[i] = card
		}
		if ctx.Level < EquipmentRequiredLevel(def, target) {
			return p, cubeError("requirements", "角色等级未达到星篆或宿阵要求")
		}
		if _, ok := MatchAstralRecipe(def, target); !ok {
			return p, cubeError("requirements", "孔数、顺序与部位未匹配任何宿阵")
		}
	case "unique.awaken":
		if named, ok := UniqueForBase(target.Item, ctx.Catalog); ok {
			p.Description += " 本次唤醒：" + named.Name + "。"
		}
		if !eligible() || target.Endgame.UniqueID != 0 {
			return p, cubeError("requirements", "请选择尚未唤醒的可染色装备")
		}
		if execute {
			if ctx.Intn == nil || !ApplyUnique(&target, def, ctx.Intn) {
				return p, cubeError("requirements", "无法唤醒此器物")
			}
		}
	case "unique.reforge":
		d, ok := UniqueOf(target, ctx.Catalog)
		if !ok || !UniqueHasVariableStats(d) {
			return p, cubeError("requirements", "这件仙遗没有可重铸的浮动属性")
		}
		if execute {
			if ctx.Intn == nil {
				return p, cubeError("requirements", "随机源不可用")
			}
			candidates := make([]int32, 0, 4001)
			for roll := UniqueRollMinBP; roll <= UniqueRollMaxBP; roll++ {
				if !sameUniqueVariableValues(d, target.Endgame.UniqueRollBP, roll) {
					candidates = append(candidates, roll)
				}
			}
			if len(candidates) == 0 {
				return p, cubeError("requirements", "浮动属性没有其他数值")
			}
			n := ctx.Intn(len(candidates))
			if n < 0 || n >= len(candidates) {
				return p, cubeError("requirements", "随机源无效")
			}
			target.Endgame.UniqueRollBP = candidates[n]
		}
	case "unique.temper":
		d, ok := UniqueOf(target, ctx.Catalog)
		if !ok || !UniqueHasVariableStats(d) || sameUniqueVariableValues(d, target.Endgame.UniqueRollBP, UniqueRollMaxBP) {
			return p, cubeError("requirements", "须选择浮动属性尚未满值的仙遗")
		}
		next := min(UniqueRollMaxBP, target.Endgame.UniqueRollBP+recipe.QualityStepBP)
		for next < UniqueRollMaxBP && sameUniqueVariableValues(d, target.Endgame.UniqueRollBP, next) {
			next++
		}
		target.Endgame.UniqueRollBP = next

	default:
		if inscribe {
			p.Description += " 产物：" + ctx.Items[outputRune].Name + "×1。"
			if err := newRune(outputRune); err != nil {
				return p, err
			}
		} else if convert {
			if len(req.ItemUIDs) != int(recipe.SourceCount) {
				return p, cubeError("requirements", fmt.Sprintf("须选择%d枚相同的同象星篆", recipe.SourceCount))
			}
			out, _ := AstralRune(outputRune, ctx.Catalog)
			var source ItemID
			for _, raw := range req.ItemUIDs {
				i, st, err := find(raw)
				if err != nil {
					return p, err
				}
				r, ok := AstralRune(st.Item, ctx.Catalog)
				if !runeValid(st) || !ok || r.Quadrant != out.Quadrant || st.Item == outputRune || source != 0 && st.Item != source {
					return p, cubeError("invalid_rune", fmt.Sprintf("须用%d枚相同且不同于产物的同象星篆", recipe.SourceCount))
				}
				source = st.Item
				p.Bag.Set(i, Stack{})
				p.ConsumedUIDs = append(p.ConsumedUIDs, st.UID)
			}
			p.Description += " 产物：" + ctx.Items[outputRune].Name + "×1。"
			if err := newRune(outputRune); err != nil {
				return p, err
			}
		} else {
			return p, cubeError("not_found", "配方不存在")
		}
	}
	if !target.Empty() {
		if (recipe.Operation == "socket.array" || recipe.Operation == "socket.reorder") && adoptCurrentFormationOnCompletion(def, ctx.Bag.At(slot), &target) {
			p.Description += " 重新成阵采用V5宿阵规则，保留原灵性值。"
		}
		if execute && recipe.Operation != "unique.awaken" {
			target.Endgame.Revision++
		}
		p.Bag.Set(slot, target)
		p.Target = target
	}
	if cubeTextUnits(p.Description) > 8192 {
		return CubePlan{}, cubeError("configuration", "炼化说明超出协议限制")
	}
	quote := EndgameQuoteView{QuoteID: strings.Repeat("x", 32), RecipeID: req.RecipeID, TargetUID: req.TargetUID, Revision: "9223372036854775807", Money: p.Recipe.Money, Materials: p.Materials, ExpiresAt: "9999-12-31T23:59:59.999999999Z", Description: p.Description}
	wire, err := json.Marshal(EndgameResponse{Kind: "cube.quote", RequestID: strings.Repeat("x", 64), OK: true, Quote: &quote})
	if err != nil || len(wire) > EndgameMaxJSONBytes {
		return CubePlan{}, cubeError("configuration", "炼化预览超出协议限制")
	}
	return p, nil
}
func cubeValidateSockets(st Stack, def ItemDef, items map[ItemID]ItemDef) error {
	if st.SocketCount > 5 || int(st.SocketCount) > EquipmentSocketCapacity(def, st) {
		return cubeError("invalid_socket", "装备孔位容量无效")
	}
	for i := range st.Sockets {
		id, uid, card := st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i]
		if i >= int(st.SocketCount) {
			if id != 0 || uid != 0 || card.Initialized {
				return cubeError("invalid_socket", "孔位数据超出容量")
			}
			continue
		}
		if id == 0 && uid == 0 && !card.Initialized {
			continue
		}
		d, ok := items[id]
		if !ok || d.InstanceKind != ItemInstanceSocketCard || uid <= 0 || !card.Initialized || card.Count > 8 {
			return cubeError("invalid_socket", "孔内卡实例不完整")
		}
		if card.Locked {
			return cubeError("locked", "孔内卡已锁定")
		}
	}
	return nil
}
