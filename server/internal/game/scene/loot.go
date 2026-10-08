package scene

import (
	"fmt"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 掉落与拾取。
//
// 掉落物是**实体**, 不是挂在尸体上的一个列表 —— 它要出现在地上、被别人看见、
// 过一会儿消失, 这些都是实体的行为。抓包里的 0x8013 也确实是按实体发的。

const (
	// lootTTLTicks 掉落物在地上待多久。
	//
	// ⚠️ **服务端定的**, 客户端数据里没有这一项。
	// 3 分钟: 长到打完一群怪回头捡还在, 短到不会让地图堆满没人要的树枝。
	lootTTLTicks = 1800

	// maxDropsPerKill 一只怪最多掉几样。
	//
	// 掉落表每条独立掷, 单怪合计概率能到 3200% —— 不封顶的话一只 BOSS
	// 可能一次吐出几十个实体, 把 AOI 广播撑爆。
	maxDropsPerKill = 8

	// monsterLootGridSpacing 是怪物掉落九宫格相邻落点的间距。最远的对角格
	// 距离尸体约 57 像素，仍在 75 像素的正常拾取范围内。
	monsterLootGridSpacing = 40.0
)

// monsterLootGridOffsets 从中心开始沿外圈顺时针排列。每次死亡会根据怪物实体
// 与当前帧轮换起点，因此单件掉落不会永远压在尸体脚下；连续九件则恰好占满
// 九个不同格子，第十件起才循环复用。
var monsterLootGridOffsets = [...]struct{ x, y float64 }{
	{0, 0},
	{0, -monsterLootGridSpacing},
	{monsterLootGridSpacing, -monsterLootGridSpacing},
	{monsterLootGridSpacing, 0},
	{monsterLootGridSpacing, monsterLootGridSpacing},
	{0, monsterLootGridSpacing},
	{-monsterLootGridSpacing, monsterLootGridSpacing},
	{-monsterLootGridSpacing, 0},
	{-monsterLootGridSpacing, -monsterLootGridSpacing},
}

// rollLoot 掷掉落并把东西放到地上。
func (s *Scene) rollLoot(dead *entity.Entity, killer domain.EntityID) {
	if s.items == nil || dead == nil || dead.Monster == nil {
		return
	}
	entries := s.lootTable[dead.Monster.TypeID]
	fixed := s.rollBossFixedEquipment(dead)
	if len(entries) == 0 && len(fixed) == 0 {
		return
	}
	// 跨级惩罚的掉率倍率按击杀者等级对怪模板等级算；宠物补刀归主人。
	// BOSS 固定装备池（保底）不乘 —— 保底就是保底，惩罚只作用于原掉落池。
	dropMult := s.dropMultBP(killer, dead.Monster.TypeID)
	startSlot := int((uint64(dead.ID) + uint64(s.tick)) % uint64(len(monsterLootGridOffsets)))
	// 染色档位取自这只怪的模板：精英与 BOSS 是原档（可出到 5 条），
	// 普通怪与副本小怪是上限 2 条的普通档。
	profile := domain.DefaultColorProfile
	if dead.Monster != nil && dead.Monster.ColorProfile != 0 {
		profile = dead.Monster.ColorProfile
	}
	n := 0
	for _, choice := range fixed {
		def, ok := s.items[choice.Item]
		if !ok || def.Equip == nil {
			continue
		}
		stack := domain.NewStack(def, 1)
		if choice.RollAffixes {
			rolled := s.equipmentRoll.Roll(def, profile, s.rng.Int63n)
			stack.RolledAffixCount = uint8(len(rolled))
			copy(stack.RolledAffixes[:], rolled)
		}
		s.spawnGeneratedLoot(def, stack,
			s.monsterLootPosition(dead.Pos, startSlot+n), killer)
		n++
	}
	originalDrops := 0
	for _, e := range entries {
		if originalDrops >= maxDropsPerKill {
			break
		}
		// 已进入本地图固定装备阶段的显式装备行不能在原掉落池再掷一次；
		// 普通道具 item_id 与类型池不在固定装备表中，仍完整执行原规则。
		if len(e.Choices) == 0 && s.fixedEquipmentManaged(dead.Monster.TypeID, e.Item) {
			continue
		}
		if !e.RollWithMult(s.rng.Intn, dropMult) {
			continue
		}
		itemID := e.Item
		if len(e.Choices) > 0 {
			itemID = e.Choices[s.rng.Intn(len(e.Choices))]
		}
		def, ok := s.items[itemID]
		if !ok {
			continue
		}
		stack := domain.NewStack(def, 1)
		if e.RollAffixes && def.Equip != nil {
			rolled := s.equipmentRoll.Roll(def, profile, s.rng.Int63n)
			stack.RolledAffixCount = uint8(len(rolled))
			copy(stack.RolledAffixes[:], rolled)
		}
		s.spawnGeneratedLoot(def, stack, s.monsterLootPosition(dead.Pos, startSlot+n), killer)
		n++
		originalDrops++
	}
}

func (s *Scene) fixedEquipmentManaged(monster domain.MonsterID, item domain.ItemID) bool {
	rule, ok := s.bossFixedDrops[domain.BossFixedDropKey{MapID: s.id.MapID, Monster: monster}]
	if !ok {
		return false
	}
	for _, choice := range rule.Pool {
		if choice.Item == item {
			return true
		}
	}
	return false
}

// dropMultBP 返回这次击杀的掉率倍率（万分数，10000 = 满）。
//
// 按击杀者对怪模板等级算；宠物补的最后一刀归主人（与 awardKill 同一口径）；
// 凶手已离开场景时没有玩家等级可用，按满倍率 —— 掉落本身仍照常掷。
func (s *Scene) dropMultBP(killer domain.EntityID, monster domain.MonsterID) int32 {
	if !s.penalty.Enabled() {
		return 10000
	}
	if pet, ok := s.pets[killer]; ok && pet.Pet != nil {
		killer = pet.Pet.Owner
	}
	p, ok := s.players[killer]
	if !ok || p.Player == nil {
		return 10000
	}
	d, ok := s.defs.Def(monster)
	if !ok {
		return 10000
	}
	return s.penalty.DropMultBP(p.Player.Char.Level, d.Level)
}

// rollBossFixedEquipment 先处理显式配置的固定装备怪掉落池：各行按自身
// 概率正常掷，低于最低件数时再按权重、优先不重复模板地补足。调用方随后才
// 运行原有怪物完整掉落表，因此保底不会替代精英/BOSS的其它掉落规则。
func (s *Scene) rollBossFixedEquipment(dead *entity.Entity) []domain.BossFixedDropChoice {
	if dead == nil || dead.Monster == nil {
		return nil
	}
	rule, ok := s.bossFixedDrops[domain.BossFixedDropKey{
		MapID: s.id.MapID, Monster: dead.Monster.TypeID,
	}]
	if !ok || rule.Minimum == 0 || len(rule.Pool) == 0 {
		return nil
	}
	out := make([]domain.BossFixedDropChoice, 0, len(rule.Pool))
	picked := make(map[domain.ItemID]struct{}, len(rule.Pool))
	for _, choice := range rule.Pool {
		if (domain.DropEntry{RatePct: choice.RatePct}).Roll(s.rng.Intn) {
			out = append(out, choice)
			picked[choice.Item] = struct{}{}
		}
	}
	for len(out) < int(rule.Minimum) {
		var total int64
		for _, choice := range rule.Pool {
			if _, exists := picked[choice.Item]; !exists && choice.Weight > 0 {
				total += choice.Weight
			}
		}
		if total <= 0 {
			break
		}
		roll := s.rng.Int63n(total)
		for _, choice := range rule.Pool {
			if _, exists := picked[choice.Item]; exists || choice.Weight <= 0 {
				continue
			}
			if roll < choice.Weight {
				out = append(out, choice)
				picked[choice.Item] = struct{}{}
				break
			}
			roll -= choice.Weight
		}
	}
	return out
}

// monsterLootPosition 返回一次怪物死亡中第 slot 件掉落的九宫格落点。地图边缘或
// 障碍物挡住标准格时，沿同一方向逐级收近；都不可走才回到尸体位置。这样正常
// 地形保持清晰分布，狭窄地形也不会把掉落物放到玩家无法到达的位置。
func (s *Scene) monsterLootPosition(center domain.Pos, slot int) domain.Pos {
	offset := monsterLootGridOffsets[slot%len(monsterLootGridOffsets)]
	if offset.x == 0 && offset.y == 0 {
		return center
	}
	for _, scale := range [...]float64{1, 0.75, 0.5, 0.25} {
		candidate := center
		candidate.X += offset.x * scale
		candidate.Y += offset.y * scale
		if s.walkable == nil || s.walkable(candidate) {
			return candidate
		}
	}
	return center
}

// grantQuestDrops 处理只在对应任务当前步骤生效的收集品。它与普通掉落完全
// 分开：不分配 groundId、不创建地面实体，怪物死亡结算时直接写入击杀者背包。
func (s *Scene) grantQuestDrops(dead *entity.Entity, killer domain.EntityID) {
	if dead == nil || dead.Monster == nil || killer == 0 || s.quests == nil || s.items == nil {
		return
	}
	if pet, ok := s.pets[killer]; ok && pet.Pet != nil {
		killer = pet.Pet.Owner
	}
	p, ok := s.players[killer]
	if !ok || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil {
		return
	}
	if s.deferTradeMutation(killer, func() { s.grantQuestDrops(dead, killer) }) {
		return
	}

	quests := p.Player.Char.Quests
	ids := make([]int, 0, len(quests))
	for id, entry := range quests {
		if entry.State != domain.QuestFinished {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)

	changed := false
	for _, rawID := range ids {
		id := domain.QuestID(rawID)
		def, ok := s.quests[id]
		if !ok {
			continue
		}
		step, ok := quests.CurrentStep(def)
		if !ok {
			continue
		}
		rule, ok := step.DropRule(dead.Monster.TypeID)
		if !ok || rule.RatePct <= 0 || s.rng.Intn(100) >= int(rule.RatePct) {
			continue
		}
		type candidate struct {
			goal    domain.QuestItemGoal
			item    domain.ItemDef
			missing int32
		}
		candidates := make([]candidate, 0, len(step.Collect))
		var totalMissing int32
		for _, goal := range step.Collect {
			if goal.Qty <= 0 || !questDropSource(goal.Sources, dead.Monster.TypeID) {
				continue
			}
			have := p.Player.Bag.CountOf(goal.Item)
			if have >= goal.Qty {
				continue
			}
			item, ok := s.itemDef(goal.Item)
			if !ok {
				continue // 启动校验会拦住；运行时仍保持防御性
			}
			missing := goal.Qty - have
			candidates = append(candidates, candidate{goal: goal, item: item, missing: missing})
			totalMissing += missing
		}
		if totalMissing <= 0 {
			continue
		}
		pick := int32(s.rng.Intn(int(totalMissing)))
		chosen := candidates[len(candidates)-1]
		for _, candidate := range candidates {
			if pick < candidate.missing {
				chosen = candidate
				break
			}
			pick -= candidate.missing
		}
		if p.Player.Bag.Add(chosen.item, 1) != 0 {
			s.emitTo(p.ID, event.QuestItemBlocked{
				Who: p.ID, Quest: id, Item: chosen.goal.Item, Name: chosen.item.Name,
			})
			continue
		}
		changed = true
		have := p.Player.Bag.CountOf(chosen.goal.Item)
		s.emitTo(p.ID, event.QuestItemObtained{
			Who: p.ID, Quest: id, Item: chosen.goal.Item, Name: chosen.item.Name,
			Have: have, Need: chosen.goal.Qty,
		})
		s.log.Debug("获得任务专属物品", "char", p.Player.Char.Name,
			"task", id, "monster", dead.Monster.TypeID, "item", chosen.item.Name,
			"have", have, "need", chosen.goal.Qty, "ratePct", rule.RatePct,
			"tier", rule.Tier)
	}
	if !changed {
		return
	}
	p.Player.MarkDirty()
	s.pushInventory(p)
}

func questDropSource(sources []domain.MonsterID, monster domain.MonsterID) bool {
	for _, source := range sources {
		if source == monster {
			return true
		}
	}
	return false
}

// spawnLoot 在地上放一件东西。
func (s *Scene) spawnLoot(def domain.ItemDef, at domain.Pos, owner domain.EntityID) {
	s.spawnGeneratedLoot(def, domain.NewStack(def, 1), at, owner)
}

// spawnGeneratedLoot 把已经创建好的实例放到地上。随机词条与 UID 在此之前
// 已经确定，拾取、丢弃和再次拾取都只移动这份 Stack，不会重新抽取。
func (s *Scene) spawnGeneratedLoot(def domain.ItemDef, stack domain.Stack, at domain.Pos, owner domain.EntityID) {
	if s.alloc == nil {
		return // 没接分配器(测试场景), 不产出实体
	}
	if stack.Empty() || stack.Item != def.ID {
		return
	}
	id := s.alloc.Drop()
	e := &entity.Entity{
		ID:   id,
		Kind: domain.KindDrop,
		Name: def.Name,
		Pos:  at,
		Drop: &entity.Drop{
			Item:       stack.Item,
			Count:      stack.Count,
			Stack:      stack,
			Owner:      owner,
			OwnerUntil: s.tick + lootOwnerTicks,
		},
	}
	s.entities[id] = e
	s.ground[id] = e
	s.aoi.Enter(e)
	s.emit(s.spawnEvent(e))

	// 到期自动消失。不设过期的话, 一张图刷一整天就会堆满没人捡的树枝。
	s.timer.after(s.tick, lootTTLTicks, func() {
		if _, ok := s.entities[id]; !ok {
			return // 已经被捡走了
		}
		s.removeLoot(id, event.DespawnTimeout)
	})
}

// spawnPlayerDrop 把玩家背包中的真实实例放到地面。客户端地面包只展示物品号
// 和数量，但服务端实体保留完整 Stack，重新拾取时不会刷新耐久或丢失 UID。
func (s *Scene) spawnPlayerDrop(def domain.ItemDef, st domain.Stack, at domain.Pos, owner domain.EntityID) {
	id := s.alloc.Drop()
	e := &entity.Entity{
		ID: id, Kind: domain.KindDrop, Name: def.Name, Pos: at,
		Drop: &entity.Drop{
			Item: st.Item, Count: st.Count, Stack: st,
			Owner: owner, OwnerUntil: s.tick + lootOwnerTicks,
		},
	}
	s.entities[id] = e
	s.ground[id] = e
	s.aoi.Enter(e)
	s.emit(s.spawnEvent(e))
	s.timer.after(s.tick, lootTTLTicks, func() {
		if _, ok := s.entities[id]; ok {
			s.removeLoot(id, event.DespawnTimeout)
		}
	})
}

// removeLoot 把地上的东西收走并广播。
func (s *Scene) removeLoot(id domain.EntityID, why event.DespawnReason) {
	e, ok := s.entities[id]
	if !ok {
		return
	}
	s.emit(event.EntityDespawned{ID: id, Kind: e.Kind, Reason: why})
	s.flush() // 立刻冲刷: 下面就摘掉它了, 留到帧末就发不出去
	s.aoi.Leave(e)
	delete(s.entities, id)
	delete(s.ground, id)
}

// onPickUp 拾取。
func (s *Scene) onPickUp(cmd PickUp) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	if p.Player != nil && p.Player.Stall != nil {
		s.stallNotice(p, "摆摊中不能拾取物品，请先结束摊位")
		return
	}
	// Rejected 事件没有下行编码器，因此在服务端日志中记录拾取拒绝原因。
	reject := func(r event.RejectReason) {
		s.log.Debug("拾取被拒", "char", p.Name, "目标", cmd.Target, "原因", r)
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "PickUp", Reason: r})
	}
	e, ok := s.ground[cmd.Target]
	if !ok || e.Drop == nil {
		reject(event.RejectNoTarget)
		return
	}
	// 够得着才捡得到。不判距离的话, 客户端可以隔着半张图把地上的东西吸走。
	rangePX := s.pickupRange
	if rangePX <= 0 {
		rangePX = pickUpRange
	} // 仅用于无数据库的场景夹具
	if sqDist(p.Pos, e.Pos) > float64(rangePX)*float64(rangePX) {
		reject(event.RejectTooFar)
		return
	}
	// 归属保护: 谁打死的谁先捡。过了保护期谁都能捡。
	if e.Drop.Owner != 0 && e.Drop.Owner != p.ID && s.tick < e.Drop.OwnerUntil {
		reject(event.RejectNoTarget)
		return
	}

	def, ok := s.items[e.Drop.Item]
	if !ok {
		s.removeLoot(e.ID, event.DespawnPickedUp) // 配置没了, 别让它永远躺着
		return
	}
	bag := p.Player.Bag
	originalCount := e.Drop.Count
	left := originalCount
	if e.Drop.Stack.Empty() {
		left = bag.Add(def, e.Drop.Count)
	} else {
		left = bag.AddStack(def, e.Drop.Stack)
	}
	if left == e.Drop.Count {
		reject(event.RejectBagFull)
		return
	}

	if left > 0 {
		// 只装下一部分: 剩下的留在地上, 别凭空吞掉
		e.Drop.Count = left
		if !e.Drop.Stack.Empty() {
			e.Drop.Stack.Count = left
		}
		s.pushInventory(p)
		p.Player.MarkDirty()
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: fmt.Sprintf("获得了 %s ×%d", def.Name, originalCount-left)})
		s.requestNewbieTip(p.ID, 2)
		return
	}
	s.removeLoot(e.ID, event.DespawnPickedUp)
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: fmt.Sprintf("获得了 %s ×%d", def.Name, originalCount)})
	s.pushInventory(p)
	p.Player.MarkDirty()
	s.requestNewbieTip(p.ID, 2)
	s.log.Debug("拾取", "char", p.Name, "物品", def.Name, "数量", e.Drop.Count)
}

// pickUpRange 拾取距离。取近战攻击距离的量级 —— 够得着打就够得着捡。
const pickUpRange = 75

// lootOwnerTicks 归属保护时长。两条真实 0x8013 样本均为 owned=1、lockMs=30000；
// TickMS=100，因此是 300 帧。这个值来自掉落包本身，不再沿用“怪物所有权”的类推。
const lootOwnerTicks = 300
