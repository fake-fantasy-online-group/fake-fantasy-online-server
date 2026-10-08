package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
	"strings"
)

// 宠物：捕捉 → 孵化 → 召唤 → 跟随并向主人提供已启用技能的效果。
//
// **`pet_id` 就是怪物 id**（18/18 逐行核对过：海龟 1006 既是可捕捉宠也是 15 级的怪，
// 名字都一样），所以捕捉直接对着场上的怪下手，不需要任何映射表。
// 18 只全部有刷怪点（最少的大闸蟹也有 8 个），整套机制数据是完备的。
//
// 宠物是**第五种实体**，不混进 monsters，也不参与任何战斗结算。

const (
	// petFollowDist 超过这个距离就往主人身边走。
	//
	// 定得比索敌半径(300)小：宠物跟太松会掉在怪堆里，
	// 而它一旦落单就是白送 —— 它不会自己跑回家。
	petFollowDist = 120
	// petLeashDist 离主人超过这个距离直接瞬移回去。
	//
	// 没有这条的话，主人跑图时宠物会永远差一截、越掉越远，
	// 最后卡在某个角落里跟着走不动。追不上就直接拉回来，不要"慢慢追"。
	petLeashDist = 900
	// enhancedCaptureAlias 是正式客户端 UseItem 对增强捕捉路径发送的虚拟物品号。
	// 服务端不会凭它造道具，而是从权威背包选出真实的万能绳索并扣除。
	enhancedCaptureAlias domain.ItemID = 0x182c3
)

// inRange 报告两点距离是否在 r 以内。
func inRange(a, b domain.Pos, r int32) bool {
	return sqDist(a, b) <= float64(r)*float64(r)
}

// onCapturePet 对一只怪下手抓。
//
// 失败**不惩罚**：怪不消失、不掉血、道具照扣一个。
// 让失败额外掉血或让怪逃跑，只会把"抓宠"变成"读档"。
func (s *Scene) onCapturePet(cmd CapturePet) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "CapturePet", Reason: r})
	}
	if s.petDefs == nil {
		reject(event.RejectUnknown)
		return
	}
	m, ok := s.monsters[cmd.Target]
	if !ok || !m.Alive() || m.Monster == nil {
		reject(event.RejectNoTarget)
		return
	}
	def, ok := s.petDefs[domain.PetID(m.Monster.TypeID)]
	if !ok || !def.Capturable() {
		reject(event.RejectNotCapturable)
		return
	}

	ch := p.Player.Char
	hpRatio := 0.0
	if m.MaxHP > 0 {
		hpRatio = float64(m.HP) / float64(m.MaxHP)
	}
	// 正式客户端明确回传本次使用的捕捉道具。内部命令的零值兼容既有调用，
	// 生产入口则必须与宠物定义一致，不能把任意 itemId 当作合法捕捉工具。
	tool := cmd.Tool
	if tool == 0 {
		tool = def.CaptureTool
	} else if tool == enhancedCaptureAlias {
		if resolved, ok := s.captureBoostTool(p); ok {
			tool = resolved
		}
	}
	toolDef, toolKnown := s.itemDef(tool)
	boost := domain.CaptureToolBoost{}
	if toolKnown {
		boost = toolDef.CaptureBoost
	}
	standardTool := tool == def.CaptureTool
	enhancedTool := boost.Valid()
	hasTool := def.CaptureTool == 0 && tool == 0 || (standardTool || enhancedTool) &&
		p.Player.Bag.UsableCountOf(tool) > 0

	switch domain.CanCapture(def, ch.Level, hasTool, len(ch.Pets) < domain.MaxPets, hpRatio) {
	case domain.CaptureNotCapturable:
		reject(event.RejectNotCapturable)
		return
	case domain.CaptureLevelTooLow:
		reject(event.RejectLevelTooLow)
		return
	case domain.CaptureNoTool:
		reject(event.RejectNoCaptureTool)
		return
	case domain.CaptureBagFull:
		reject(event.RejectPetSlotsFull)
		return
	case domain.CaptureTargetNotWeak:
		reject(event.RejectTargetNotWeak)
		return
	}

	// 宠物栏有余位之后才看背包：捕捉成功会在背包第 4 页放一个宠物载体物品。
	// 顺序反过来的话，栏位已满的玩家会先收到“背包已满”，而真正该清理的
	// 宠物栏反而没有任何提示。
	if _, ok := s.petCarrierDef(def.ID); !ok {
		// 配置里没有这只宠的载体物品，收了也放不进背包 —— 不能静默吞掉道具。
		reject(event.RejectUnknown)
		return
	}
	if !s.petCarrierRoom(p, 1) {
		reject(event.RejectBagFull)
		return
	}

	// 道具**掷之前**就扣。放在成功之后扣的话, 失败就是免费的,
	// 那最优解永远是满血狂点直到中奖
	if tool != 0 {
		if !p.Player.Bag.Remove(tool, 1) {
			reject(event.RejectNoCaptureTool)
			return
		}
		s.pushInventory(p)
	}
	p.Player.MarkDirty()

	rate := domain.CaptureRate(def, hpRatio)
	if enhancedTool {
		rate = int32(int64(rate) * int64(boost.SuccessRatePct) / 100)
		if rate > 100 {
			rate = 100
		}
	}
	if int32(s.rng.Intn(100)) >= rate {
		s.emitTo(p.ID, event.PetCaptureFailed{Who: p.ID, Target: m.ID, Rate: rate})
		s.log.Debug("捕捉失败", "char", ch.Name, "目标", def.Name, "成功率", rate)
		return
	}

	inst := domain.NewPetInstance(def)
	inst.ID = s.nextPetInstID(ch)
	inst.Slot = s.nextPetSlot(ch)
	if enhancedTool && boost.PrefixRatePct > 100 {
		inst.HatchPrefixRatePct = boost.PrefixRatePct
	}
	if !s.appendPet(p, inst) {
		// 上面的空间检查已经保证过，真走到这里说明状态被并发改动或配置不一致。
		// 退还刚扣掉的捕捉道具并把怪留在场上：不能既扣道具又丢怪还没宠物。
		if tool != 0 && toolKnown {
			p.Player.Bag.Add(toolDef, 1)
			s.pushInventory(p)
			p.Player.MarkDirty()
		}
		s.log.Error("宠物入栏失败，已退还捕捉道具", "char", ch.Name, "宠", def.Name)
		reject(event.RejectBagFull)
		return
	}

	// 抓到了, 怪从场上消失 —— 它现在在你兜里
	s.removeMonsterCaptured(m)
	s.emitTo(p.ID, event.PetCaptured{Who: p.ID, Inst: inst.ID,
		Pet: int32(def.ID), Name: inst.DisplayName(def)})
	s.pushPetSnapshot(p) // 宠物栏多了一只, 客户端那边是整份覆盖
	s.log.Info("捕捉成功", "char", ch.Name, "宠", def.Name, "成功率", rate)
}

// 宠物集合增删会移动底层数组，必须按变更前的实例 ID 重新绑定出战实体。
// 不从变更后的旧指针复制数据：删除前面的元素后，该地址可能已经是另一只宠物。
func (s *Scene) appendPet(p *entity.Entity, inst domain.PetInstance) bool {
	carrier, ok := s.petCarrierDef(inst.Def)
	if !ok || !s.petItemRoom(p, 1) {
		return false
	}
	if inst.ItemUID == 0 {
		st := domain.NewStack(carrier, 1)
		if p.Player.Bag.AddStack(carrier, st) != 0 {
			return false
		}
		inst.ItemUID = st.UID
	}

	active := s.entities[p.Player.Pet]
	var id domain.PetInstID
	if active != nil && active.Pet != nil && active.Pet.Inst != nil {
		id = active.Pet.Inst.ID
	}
	p.Player.Char.Pets = append(p.Player.Char.Pets, inst)
	if id != 0 {
		active.Pet.Inst = p.Player.Char.FindPet(id)
	}
	return true
}

func (s *Scene) removePet(p *entity.Entity, id domain.PetInstID) {
	active := s.entities[p.Player.Pet]
	var activeID domain.PetInstID
	if active != nil && active.Pet != nil && active.Pet.Inst != nil {
		activeID = active.Pet.Inst.ID
	}
	for i := range p.Player.Char.Pets {
		if p.Player.Char.Pets[i].ID != id {
			continue
		}
		if activeID == id {
			s.recallPet(p, event.PetRecalledByPlayer)
			activeID = 0
		}
		uid := p.Player.Char.Pets[i].ItemUID
		p.Player.Bag.Each(func(slot int, st domain.Stack) {
			if uid > 0 && st.UID == uid {
				p.Player.Bag.RemoveAt(slot, 1)
			}
		})
		pets := p.Player.Char.Pets
		p.Player.Char.Pets = append(pets[:i], pets[i+1:]...)
		if activeID != 0 {
			active.Pet.Inst = p.Player.Char.FindPet(activeID)
		}
		return
	}
}

func (s *Scene) captureBoostTool(p *entity.Entity) (domain.ItemID, bool) {
	if p == nil || p.Player == nil || p.Player.Bag == nil {
		return 0, false
	}
	var selected domain.ItemID
	bestSuccess := int32(0)
	p.Player.Bag.Each(func(_ int, stack domain.Stack) {
		if stack.Empty() || stack.Locked || stack.Count <= 0 {
			return
		}
		def, ok := s.itemDef(stack.Item)
		if !ok || !def.CaptureBoost.Valid() {
			return
		}
		if def.CaptureBoost.SuccessRatePct > bestSuccess ||
			def.CaptureBoost.SuccessRatePct == bestSuccess && (selected == 0 || stack.Item < selected) {
			selected, bestSuccess = stack.Item, def.CaptureBoost.SuccessRatePct
		}
	})
	return selected, selected != 0
}

// pushPetSnapshot 把整个宠物栏重发给本人。
//
// 客户端那边是整份覆盖, 没有增量, 所以宠物栏一变就要重发:
// 抓到新宠、放出、收回都算。
func (s *Scene) pushPetSnapshot(owner *entity.Entity) {
	if owner == nil || owner.Player == nil {
		return
	}
	oldMaxWeight := owner.Stats.MaxWeight
	if owner.Player.Char != nil && owner.Player.Worn != nil {
		s.refreshStats(owner)
	}
	// 宠物资料和神力负重都反映在背包；会话层过滤未变化的快照。
	if owner.Stats.MaxWeight != oldMaxWeight || len(owner.Player.Char.Pets) > 0 {
		if err := s.pushInventory(owner); err != nil {
			s.log.Error("宠物属性变化后推送背包失败", "char", owner.Name, "err", err)
		}
	}
	s.emitTo(owner.ID, s.petSnapshot(owner))
}

// petSnapshot 由角色身上的宠物栏造出完整快照。
//
// **槽位号就是 Pets 的下标** —— 存储层按 inst_id 排序读出来, 顺序稳定;
// 客户端随后的"放出第 N 只"就是拿这个下标回报的, 两边必须是同一个口径。
func (s *Scene) petSnapshot(owner *entity.Entity) event.PetSnapshot {
	ch := owner.Player.Char
	normalizePetSlots(ch)
	snap := event.PetSnapshot{
		PPAiCap:  s.petPPAiCap,
		FightCap: s.petRule.MaxFightSkills, LifeCap: s.petRule.MaxLifeSkills,
		Who:        owner.ID,
		ActiveSlot: event.NoActivePet,
		Show:       owner.Player.Pet != 0 && !owner.Player.Riding,
	}
	// 出战的那只用**实体上那份指针**认, 不按下标猜 —— 抓宠会往 Pets 尾部追加,
	// 下标会变, 而实例号不会。
	var active domain.PetInstID
	if owner.Player.Pet != 0 {
		if pe, ok := s.entities[owner.Player.Pet]; ok && pe.Pet != nil && pe.Pet.Inst != nil {
			active = pe.Pet.Inst.ID
		}
	}
	carriers := make(map[int64]domain.Stack)
	if owner.Player.Bag != nil {
		owner.Player.Bag.Each(func(_ int, st domain.Stack) {
			if st.UID > 0 {
				carriers[st.UID] = st
			}
		})
	}
	for _, i := range orderedPetIndices(ch) {
		inst := &ch.Pets[i]
		def, ok := s.petDefs[inst.Def]
		if !ok {
			// 种族表里没有这一行就整只跳过。发一行没有名字和模型的空宠,
			// 客户端会当成一只真宠显示出来。
			s.log.Warn("宠物种族缺定义, 快照跳过", "char", ch.Name, "宠种", inst.Def)
			continue
		}
		slot := event.PetSlotView{
			Model:   inst.RenderModel(def, s.now()),
			Slot:    int32(len(snap.Pets)),
			Name:    inst.Name,
			Species: def.Name,
			Opened:  inst.Hatched,
		}
		if carrier, ok := carriers[inst.ItemUID]; inst.ItemUID > 0 && ok {
			slot.Bound, slot.Locked = carrier.Bound, carrier.Locked
		}
		// 未孵化只告诉客户端“这里有一枚什么种族的宠物蛋”；属性、前缀和
		// 初始技能等孵化后再公开。
		if inst.Hatched {
			slot.Gender, slot.Habit = inst.Gender, def.Habit
			slot.Prename = s.petPrefixName(inst.Prefix)
			slot.Level = inst.Level
			slot.Base = inst.Base
			slot.Starve = inst.Starve
			slot.Trust = inst.Trust
			slot.FreePoints = inst.TotalFreePoints()
			slot.UsedPoints = inst.DisplayAllocatedPoints()
			slot.PPAiUsed = inst.PPAiUsed
			slot.Skills = s.petSkillViews(inst)
		}
		snap.Pets = append(snap.Pets, slot)
		if active != 0 && inst.ID == active {
			snap.ActiveSlot = len(snap.Pets) - 1
			snap.ActiveBound = slot.Bound
			snap.Active = s.petView(inst, def)
			snap.Ridable = (def.Rideable() && s.petHasMountSkill(inst)) ||
				(inst.RidingSaddle != 0 && s.saddleForPet(ch, inst, inst.RidingSaddle) != nil)
		}
	}
	if snap.ActiveSlot == event.NoActivePet && owner.Player.RideAnchor != 0 && owner.Player.Riding {
		if driver := s.players[owner.Player.RideAnchor]; driver != nil && driver.Player != nil && driver.Player.Riding {
			snap.SharedMountModel = driver.Player.MountModel
		}
	}
	return snap
}

// petView 把一只宠的领域状态摊成协议无关的完整视图。
func (s *Scene) petView(inst *domain.PetInstance, def domain.PetDef) event.PetView {
	st := domain.DeriveFromBase(inst.Base)
	var need int64
	if s.petLvls != nil {
		need = s.petLvls.Need(inst.Level)
	}
	return event.PetView{
		PPAiUsed:    inst.PPAiUsed,
		Gender:      inst.Gender,
		Habit:       def.Habit,
		Model:       inst.RenderModel(def, s.now()),
		Species:     def.Name,
		Prename:     s.petPrefixName(inst.Prefix),
		Name:        inst.Name,
		Level:       inst.Level,
		Exp:         clampI32(inst.Exp),
		ExpToNext:   clampI32(need),
		Starve:      inst.Starve,
		Trust:       inst.Trust,
		FreePoints:  inst.FreePoints,
		HP:          inst.HP,
		MaxHP:       inst.MaxHP(def),
		MP:          inst.MP,
		MaxMP:       inst.MaxMP(def),
		Base:        inst.Base,
		MinAtk:      st.MinAtk,
		MaxAtk:      st.MaxAtk,
		Def:         st.Def,
		MAtk:        st.MAtk,
		MDef:        st.MDef,
		Hit:         st.Hit,
		ActiveSkill: int32(inst.ActiveSkill),
		Skills:      s.petSkillViews(inst),
	}
}

func (s *Scene) petPrefixName(id int32) string {
	if d, ok := s.petPrefixes.Find(id); ok {
		return d.Name
	}
	return ""
}

func (s *Scene) petSkillViews(inst *domain.PetInstance) []event.PetSkillView {
	views := make([]event.PetSkillView, 0, len(inst.Skills))
	for _, learned := range inst.Skills {
		d, ok := s.petSkills[learned.ID]
		if !ok {
			continue
		}
		views = append(views, event.PetSkillView{
			ID: int32(d.ID), Name: d.Name, Icon: d.Icon, Description: d.Description,
			Fight: d.Fight, Active: d.Active,
			Level: learned.Level, MaxLevel: d.MaxLevel,
		})
	}
	return views
}

// clampI32 把 int64 的经验压进客户端的 I32 字段。
//
// 宠物经验曲线里 59 级往上就是 int32 饱和值, 直接转换会翻成负数,
// 客户端的经验条会倒着走。
func clampI32(v int64) int32 {
	const max = int64(^uint32(0) >> 1)
	switch {
	case v > max:
		return int32(max)
	case v < 0:
		return 0
	}
	return int32(v)
}

// nextPetInstID 给新宠一个角色内唯一的实例号。
//
// **不是全局自增**：宠物是角色的附属数据，与背包格子同级。
// 用角色内序号的好处是存档时不需要一张全局序列表，
// 代价是两个角色的宠会有相同的实例号 —— 而它们从不出现在同一个句子里。
func (s *Scene) nextPetInstID(ch *domain.Character) domain.PetInstID {
	var max domain.PetInstID
	for _, p := range ch.Pets {
		if p.ID > max {
			max = p.ID
		}
	}
	return max + 1
}

func (s *Scene) nextPetSlot(ch *domain.Character) int32 {
	var used [domain.MaxPets]bool
	for _, pet := range ch.Pets {
		if pet.Slot >= 0 && pet.Slot < domain.MaxPets {
			used[pet.Slot] = true
		}
	}
	for slot := int32(0); slot < domain.MaxPets; slot++ {
		if !used[slot] {
			return slot
		}
	}
	return domain.MaxPets
}

// petAtVisibleSlot 按下发给客户端的宠物栏槽位找实例。缺种族定义的脏数据不会
// 出现在快照里，因此这里也必须跳过，保证双击的槽位与服务端指向同一只宠物。
func (s *Scene) petAtVisibleSlot(ch *domain.Character, slot int32) (*domain.PetInstance, domain.PetDef, bool) {
	if ch == nil || slot < 0 {
		return nil, domain.PetDef{}, false
	}
	visible := int32(0)
	for _, i := range orderedPetIndices(ch) {
		def, ok := s.petDefs[ch.Pets[i].Def]
		if !ok {
			continue
		}
		if visible == slot {
			for _, owner := range s.players {
				if owner.Player != nil && owner.Player.Char == ch && !s.petItemAvailable(owner, &ch.Pets[i]) {
					return nil, domain.PetDef{}, false
				}
			}
			return &ch.Pets[i], def, true
		}
		visible++
	}
	return nil, domain.PetDef{}, false
}

func orderedPetIndices(ch *domain.Character) []int {
	if ch == nil || len(ch.Pets) == 0 {
		return nil
	}
	indices := make([]int, len(ch.Pets))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left, right := ch.Pets[indices[i]], ch.Pets[indices[j]]
		if left.Slot != right.Slot {
			return left.Slot < right.Slot
		}
		return left.ID < right.ID
	})
	return indices
}

// onHatchPetAt 把捕捉得到的宠物蛋变成可出战宠物。前缀与初始技能只在这里
// 掷一次并立即落入角色快照；重复双击不会重抽。
func (s *Scene) onHatchPetAt(cmd HatchPetAt) {
	owner, ok := s.players[cmd.ID]
	if !ok || owner.Player == nil || !owner.Alive() {
		return
	}
	reject := func(reason event.RejectReason) {
		s.emitTo(owner.ID, event.Rejected{Who: owner.ID, Cmd: "HatchPet", Reason: reason})
	}
	if s.petDefs == nil || s.petSkills == nil || !s.petRule.Valid() {
		reject(event.RejectUnknown)
		return
	}
	inst, def, ok := s.petAtVisibleSlot(owner.Player.Char, cmd.Slot)
	if !ok {
		reject(event.RejectNoPet)
		return
	}
	if inst.Hatched {
		reject(event.RejectPetAlreadyHatched)
		return
	}

	inst.Prefix = s.rollPetPrefix(inst.HatchPrefixRatePct)
	inst.HatchPrefixRatePct = 0
	s.assignInitialPetSkills(inst, def)
	inst.Gender = uint8(s.rng.Intn(2))
	inst.Hatched = true
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
	s.log.Info("宠物孵化", "char", owner.Name, "宠", def.Name,
		"前缀", s.petPrefixName(inst.Prefix), "技能数", len(inst.Skills))
}

func (s *Scene) rollPetPrefix(ratePct int32) int32 {
	if ratePct <= 0 {
		ratePct = 100
	}
	roll := int32(s.rng.Intn(100))
	cumulative := int32(0)
	for _, def := range s.petPrefixes {
		if def.GetRate <= 0 {
			continue
		}
		cumulative += def.GetRate * ratePct / 100
		if roll < cumulative {
			return def.ID
		}
	}
	return domain.NoPetPrefix
}

func (s *Scene) assignInitialPetSkills(inst *domain.PetInstance, def domain.PetDef) {
	inst.Skills = nil
	inst.ActiveSkill = 0
	inst.PendingLearn = domain.PetLearningBoost{}
	for _, initial := range def.InitialSkills {
		skillDef, ok := s.petSkills[initial.ID]
		if !ok || !s.learnPetSkill(inst, initial.ID) {
			continue
		}
		level := initial.Level
		if level < 1 {
			level = 1
		}
		if skillDef.MaxLevel > 0 && level > skillDef.MaxLevel {
			level = skillDef.MaxLevel
		}
		inst.Skill(initial.ID).Level = level
	}
}

// reconcilePetSkills 按 PostgreSQL 中的 pets.json 种族配置清理存量数据。
// 旧实现把 ov_petskillno 误当成学习池，海龟因此会带上明确禁止的坐骑/刚力；
// 这里保留合法后天技能、补齐天生技能，并移除不可能出现的技能与药丸状态。
func (s *Scene) reconcilePetSkills(ch *domain.Character) {
	if ch == nil || s.petDefs == nil || s.petSkills == nil {
		return
	}
	normalizePetSlots(ch)
	deployedFound := false
	for i := range ch.Pets {
		inst := &ch.Pets[i]
		def, ok := s.petDefs[inst.Def]
		if !ok {
			inst.ActiveSkill = 0
			inst.Deployed = false
			inst.Riding = false
			inst.RidingSaddle = 0
			continue
		}
		if !inst.Hatched {
			inst.Skills = nil
			inst.ActiveSkill = 0
			inst.Deployed = false
			inst.Riding = false
			inst.PendingLearn = domain.PetLearningBoost{}
			inst.RidingSaddle = 0
			continue
		}

		innate := make(map[domain.SkillID]domain.PetSkill, len(def.InitialSkills))
		allowed := make(map[domain.SkillID]bool, len(def.InitialSkills)+len(def.Learnable))
		for _, skill := range def.InitialSkills {
			innate[skill.ID] = skill
			allowed[skill.ID] = true
		}
		for _, id := range def.Learnable {
			allowed[id] = true
		}

		clean := make([]domain.PetSkill, 0, len(inst.Skills)+len(def.InitialSkills))
		seen := make(map[domain.SkillID]bool, cap(clean))
		for _, skill := range inst.Skills {
			skillDef, exists := s.petSkills[skill.ID]
			_, isInnate := innate[skill.ID]
			if !exists || !allowed[skill.ID] || seen[skill.ID] ||
				(!isInnate && skillDef.LearnLevel > inst.Level) {
				continue
			}
			if skill.Level < 1 {
				skill.Level = 1
			}
			if skillDef.MaxLevel > 0 && skill.Level > skillDef.MaxLevel {
				skill.Level = skillDef.MaxLevel
			}
			clean = append(clean, skill)
			seen[skill.ID] = true
		}
		inst.Skills = clean
		for _, initial := range def.InitialSkills {
			if seen[initial.ID] {
				continue
			}
			if s.learnPetSkill(inst, initial.ID) {
				level := initial.Level
				if level < 1 {
					level = 1
				}
				inst.Skill(initial.ID).Level = level
			}
		}
		if inst.Deployed && !deployedFound {
			deployedFound = true
		} else {
			inst.Deployed = false
			inst.Riding = false
			inst.ActiveSkill = 0
		}
		if inst.Deployed && inst.Riding && inst.RidingSaddle != 0 &&
			s.saddleForPet(ch, inst, inst.RidingSaddle) != nil {
			inst.ActiveSkill = 0
		} else if inst.Deployed && inst.ActiveSkill != 0 {
			inst.RidingSaddle = 0
			activeDef, exists := s.petSkills[inst.ActiveSkill]
			mount := exists && s.isMountPetSkill(inst.ActiveSkill)
			if !exists || !activeDef.Active || !inst.Knows(inst.ActiveSkill) ||
				(mount && !def.Rideable()) {
				inst.ActiveSkill = 0
				inst.Riding = false
			} else if inst.Riding && !mount {
				inst.Riding = false
			}
		} else if inst.Deployed {
			inst.Riding = false
		}
		if !inst.Riding {
			inst.RidingSaddle = 0
		}
		if pending := inst.PendingLearn; pending.Item != 0 {
			skillDef, exists := s.petSkills[pending.Skill]
			if !exists || !s.canLearnPetSkill(inst, def, pending.Skill) ||
				skillDef.LearnLevel > inst.Level+1 {
				inst.PendingLearn = domain.PetLearningBoost{}
			}
		}
	}
}

func normalizePetSlots(ch *domain.Character) {
	if ch == nil {
		return
	}
	validSlots := true
	seenSlots := make(map[int32]bool, len(ch.Pets))
	for i := range ch.Pets {
		slot := ch.Pets[i].Slot
		if slot < 0 || slot >= domain.MaxPets || seenSlots[slot] {
			validSlots = false
			break
		}
		seenSlots[slot] = true
	}
	if !validSlots {
		for slot, index := range orderedPetIndices(ch) {
			ch.Pets[index].Slot = int32(slot)
		}
	}
}

// onSetPetSkill 是客户端取消当前技能的入口。非零技能只能通过
// UsePetSkill 启用，避免单击选中或面板同步误触发效果。
func (s *Scene) onSetPetSkill(cmd SetPetSkill) {
	owner, pet, ok := s.activePetForSkill(cmd.ID)
	if !ok {
		return
	}
	if cmd.Skill != 0 {
		s.emitTo(owner.ID, event.Rejected{Who: owner.ID, Cmd: "SetPetSkill", Reason: event.RejectInvalid})
		return
	}
	if pet.Pet.Inst.ActiveSkill == 0 && !owner.Player.Riding {
		return
	}
	pet.Pet.Inst.ActiveSkill = 0
	owner.Player.MarkDirty()
	if owner.Player.Riding {
		s.setRiding(owner, false, true)
	}
	s.pushPetSnapshot(owner)
}

// onUsePetSkill 是技能双击与快捷栏点击的唯一生效入口。
func (s *Scene) onUsePetSkill(cmd UsePetSkill) {
	owner, pet, ok := s.activePetForSkill(cmd.ID)
	if !ok {
		return
	}
	inst := pet.Pet.Inst
	def, exists := s.petSkills[cmd.Skill]
	if cmd.Skill == 0 || !exists || !def.Active {
		s.emitTo(owner.ID, event.Rejected{Who: owner.ID, Cmd: "UsePetSkill", Reason: event.RejectInvalid})
		return
	}
	if !inst.Knows(cmd.Skill) {
		s.emitTo(owner.ID, event.Rejected{Who: owner.ID, Cmd: "UsePetSkill", Reason: event.RejectSkillNotLearned})
		return
	}
	mountSkill := s.isMountPetSkill(cmd.Skill)
	if mountSkill && !pet.Pet.Def.Rideable() {
		s.emitTo(owner.ID, event.Rejected{Who: owner.ID, Cmd: "UsePetSkill", Reason: event.RejectInvalid})
		return
	}
	if mountSkill && owner.Player.Riding {
		inst.ActiveSkill = 0
		pet.Pet.NextPetSkillAt = 0
		s.setRiding(owner, false, true)
	} else {
		inst.ActiveSkill = cmd.Skill
		pet.Pet.NextPetSkillAt = 0
		if owner.Player.Riding {
			s.setRiding(owner, false, true)
		}
		if mountSkill {
			s.setRiding(owner, true, true)
		}
	}
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
}

func (s *Scene) onAddPetPoint(cmd AddPetPoint) {
	owner, pet, ok := s.activePetForSkill(cmd.ID)
	if !ok {
		return
	}
	inst := pet.Pet.Inst
	if inst.FreePoints <= 0 {
		s.emitTo(owner.ID, event.Rejected{
			Who: owner.ID, Cmd: "AddPetPoint", Reason: event.RejectNotEnough,
		})
		return
	}

	switch cmd.AttributeCode {
	case 1:
		inst.Base.STR++
	case 5:
		inst.Base.VIT++
	case 7:
		inst.Base.AGI++
	case 3:
		inst.Base.INT++
	case 9:
		inst.Base.DEX++
	case 61:
		inst.Base.SPI++
	default:
		s.emitTo(owner.ID, event.Rejected{
			Who: owner.ID, Cmd: "AddPetPoint", Reason: event.RejectInvalid,
		})
		return
	}

	inst.FreePoints--
	if inst.AllocatedPoints != domain.PetAllocatedUnknown {
		inst.AllocatedPoints++
	}
	pet.Stats = domain.DeriveFromBase(inst.Base)
	pet.MaxHP, pet.MaxMP = inst.MaxHP(pet.Pet.Def), inst.MaxMP(pet.Pet.Def)
	pet.Stats.MaxHP, pet.Stats.MaxMP = pet.MaxHP, pet.MaxMP
	if pet.HP > pet.MaxHP {
		pet.HP = pet.MaxHP
	}
	if pet.MP > pet.MaxMP {
		pet.MP = pet.MaxMP
	}
	inst.HP, inst.MP = pet.HP, pet.MP
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
}

func (s *Scene) onRenamePet(cmd RenamePet) {
	owner, pet, ok := s.activePetForSkill(cmd.ID)
	if !ok {
		return
	}
	name := strings.TrimSpace(cmd.Name)
	if err := domain.ValidatePetName(name); err != nil {
		s.emitTo(owner.ID, event.Rejected{
			Who: owner.ID, Cmd: "RenamePet", Reason: event.RejectInvalid,
		})
		return
	}
	if pet.Pet.Inst.Name == name {
		return
	}
	pet.Pet.Inst.Name = name
	pet.Name = name
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
}

func (s *Scene) onSetPetShown(cmd SetPetShown) {
	owner, ok := s.players[cmd.ID]
	if !ok || owner.Player == nil || owner.Player.Pet == 0 {
		if ok {
			s.emitTo(owner.ID, event.Rejected{
				Who: owner.ID, Cmd: "SetPetShown", Reason: event.RejectNoPet,
			})
		}
		return
	}
	if cmd.Show {
		if owner.Player.Riding {
			s.setRiding(owner, false, true)
			owner.Player.MarkDirty()
			s.pushPetSnapshot(owner)
		}
		return
	}
	s.recallPet(owner, event.PetRecalledByPlayer)
}

func (s *Scene) onSwapPetSlots(cmd SwapPetSlots) {
	owner, ok := s.players[cmd.ID]
	if !ok || owner.Player == nil || cmd.From < 0 || cmd.From >= domain.MaxPets ||
		cmd.To < 0 || cmd.To >= domain.MaxPets || cmd.From == cmd.To {
		return
	}
	ch := owner.Player.Char
	visible := make([]int, 0, len(ch.Pets))
	for _, index := range orderedPetIndices(ch) {
		if _, exists := s.petDefs[ch.Pets[index].Def]; exists {
			visible = append(visible, index)
		}
	}
	if int(cmd.From) >= len(visible) {
		s.emitTo(owner.ID, event.Rejected{
			Who: owner.ID, Cmd: "SwapPetSlots", Reason: event.RejectNoPet,
		})
		return
	}
	target := int(cmd.To)
	if target >= len(visible) {
		target = len(visible) - 1
	}
	from, to := visible[int(cmd.From)], visible[target]
	ch.Pets[from].Slot = cmd.To
	ch.Pets[to].Slot = cmd.From
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
}

func (s *Scene) activePetForSkill(ownerID domain.EntityID) (*entity.Entity, *entity.Entity, bool) {
	owner, ok := s.players[ownerID]
	if !ok || owner.Player == nil || owner.Player.Pet == 0 {
		return nil, nil, false
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return nil, nil, false
	}
	return owner, pet, true
}

func (s *Scene) isMountPetSkill(skill domain.SkillID) bool {
	d, ok := s.petSkills[skill]
	return ok && d.Name == "坐骑"
}

func (s *Scene) petHasMountSkill(inst *domain.PetInstance) bool {
	_, ok := s.petMountSkill(inst)
	return ok
}

func (s *Scene) petMountSkill(inst *domain.PetInstance) (domain.PetSkill, bool) {
	if inst != nil {
		for _, learned := range inst.Skills {
			if s.isMountPetSkill(learned.ID) {
				return learned, true
			}
		}
	}
	return domain.PetSkill{}, false
}

// setRiding 只管理本次在线生命周期的骑乘态。宠物实体仍留在场景内推进出战期
// 需求，但骑上时对客户端移除；正常下马再在主人当前位置恢复。recallPet 传
// revealPet=false，避免收宠时先闪现一帧宠物再消失。
func (s *Scene) setRiding(owner *entity.Entity, on, revealPet bool) {
	if !on {
		s.endSharedRide(owner)
	}
	if owner == nil || owner.Player == nil || owner.Player.Riding == on {
		return
	}
	pet := s.entities[owner.Player.Pet]
	if on {
		if pet == nil || pet.Pet == nil {
			return
		}
		owner.Player.StopResting()
		owner.Player.Riding = true
		owner.Player.MountModel = s.ridingModel(owner, pet.Pet.Inst, pet.Pet.Def)
		if pet.Pet.Inst != nil {
			pet.Pet.Inst.Riding = true
		}
		if pet.Pos != owner.Pos {
			s.aoi.Move(pet, owner.Pos)
		}
		s.emit(event.EntityDespawned{ID: pet.ID, Kind: pet.Kind, Reason: event.DespawnRemoved})
		s.emit(s.ridingEvent(owner))
		// 0x8012 不带速度；紧跟一份 0x8007，让本人实际移动也切到骑速。
		s.emitTo(owner.ID, s.attributeSnapshot(owner))
		s.log.Debug("骑上宠物", "char", owner.Name, "pet", pet.Pet.Def.Name,
			"speed_px", s.playerMoveSpeedPX(owner))
		return
	}
	owner.Player.Riding = false
	owner.Player.MountModel = 0
	if pet != nil && pet.Pet != nil && pet.Pet.Inst != nil {
		pet.Pet.Inst.Riding = false
		pet.Pet.Inst.RidingSaddle = 0
	}
	s.emit(event.RidingChanged{Who: owner.ID})
	// 下马同理，立即恢复人物步行属性，不等下一次装备/Buff 刷新。
	s.emitTo(owner.ID, s.attributeSnapshot(owner))
	if revealPet && pet != nil && pet.Pet != nil {
		if pet.Pos != owner.Pos {
			s.aoi.Move(pet, owner.Pos)
		}
		s.emit(s.spawnEvent(pet))
	}
}

// playerMoveSpeedPX 返回正式客户端场景使用的像素/秒。
//
// 步行时，客户端已实测为 attrs[A_MOVESPEED] × pxPerSpeed(2)。骑乘时由
// 鞍具的基础步速加成，或 ov_petgrow 的技能骑速，算出坐骑基础像素速度，再叠加
// 人物相对自身基础移速的全部增减量。这样坐骑只替换基础步速，冲锋、隐身术等
// Buff 仍依照它们在人物属性管线中算出的值叠加，不会把人物基础步速重复加一次。
func (s *Scene) playerMoveSpeedPX(owner *entity.Entity) int32 {
	if owner == nil || owner.Player == nil {
		return 0
	}
	if owner.Player.RideAnchor != 0 {
		if driver := s.players[owner.Player.RideAnchor]; driver != nil && driver.Player != nil && driver.Player.RideAnchor == 0 {
			return s.playerMoveSpeedPX(driver)
		}
	}
	walk := int64(owner.Stats.MoveSpeed) * int64(clientPixelsPerMoveSpeed)
	if !owner.Player.Riding {
		return nonnegativeSpeed(walk)
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return nonnegativeSpeed(walk)
	}
	baseWalk := int64(characterBaseMoveSpeed(owner.Player.Char)) * int64(clientPixelsPerMoveSpeed)
	if pet.Pet.Inst.RidingSaddle != 0 {
		if d := s.saddleForPet(owner.Player.Char, pet.Pet.Inst, pet.Pet.Inst.RidingSaddle); d != nil {
			if d.UsePetMaxSpeed {
				return nonnegativeSpeed(int64(pet.Pet.Def.HorseSpeedLimit) + walk - baseWalk)
			}
			return nonnegativeSpeed(walk + baseWalk*int64(d.SpeedBonusBP)/10000)
		}
	}
	mountSkill, ok := s.petMountSkill(pet.Pet.Inst)
	if !ok {
		return nonnegativeSpeed(walk)
	}
	ride := int64(pet.Pet.Def.RideSpeed(mountSkill.Level))
	if ride <= 0 {
		return nonnegativeSpeed(walk)
	}
	return nonnegativeSpeed(ride + walk - baseWalk)
}

func nonnegativeSpeed(speed int64) int32 {
	if speed <= 0 {
		return 0
	}
	if speed > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(speed)
}

// removeMonsterCaptured 把被抓走的怪从场上摘掉。
//
// 与打死不同：**不给经验、不掉落、不排重生**。
// 排重生的话，抓走一只反而等于凭空多刷一只，刷怪点就成了宠物自动贩卖机。
func (s *Scene) removeMonsterCaptured(m *entity.Entity) {
	s.emit(event.EntityDespawned{ID: m.ID, Kind: m.Kind, Reason: event.DespawnRemoved})
	s.flush() // 立刻冲刷: 下面就摘掉它了, 留到帧末就发不出去
	s.aoi.Leave(m)
	delete(s.entities, m.ID)
	delete(s.monsters, m.ID)
	if s.spawner != nil && m.Monster.SpawnID != 0 {
		// 释放点位但**不排重生** —— 让它按正常节奏等下一轮
		s.spawner.Release(m.Monster.SpawnID)
	}
}

// onSummonPetAt 按宠物栏槽位放出一只宠。
//
// 槽位号必须与 petSnapshot 发下去的那份列表同一个口径 —— 那份列表跳过了
// 种族表里没有的行, 所以这里也要按同样的规则数, 不能直接拿 ch.Pets 下标。
// 两边数法不一致的表现是"点第三只放出来的是第二只"。
func (s *Scene) onSummonPetAt(cmd SummonPetAt) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	if cmd.Slot < 0 {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "SummonPet", Reason: event.RejectNoPet})
		return
	}
	inst, _, ok := s.petAtVisibleSlot(p.Player.Char, cmd.Slot)
	if ok {
		s.onSummonPet(SummonPet{ID: cmd.ID, Inst: inst.ID})
		return
	}
	s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "SummonPet", Reason: event.RejectNoPet})
}

// onTogglePetAt 处理客户端 CharData.SwitchPet(idx)。宠物栏双击只发这一条，
// 并不会替服务端再发 PetHatch/PetDeploy/PetRecall，因此孵化与切换语义必须在这里闭合。
func (s *Scene) onTogglePetAt(cmd TogglePetAt) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		return
	}
	inst, _, ok := s.petAtVisibleSlot(p.Player.Char, cmd.Slot)
	if !ok {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "TogglePet", Reason: event.RejectNoPet})
		return
	}

	// 魂之精双击先孵化，再继续切换出战；孵化失败时保留当前出战宠物。
	if !inst.Hatched {
		s.onHatchPetAt(HatchPetAt{ID: cmd.ID, Slot: cmd.Slot})
		if !inst.Hatched {
			return
		}
	}

	if p.Player.Pet != 0 {
		active := s.entities[p.Player.Pet]
		if active != nil && active.Pet != nil && active.Pet.Inst != nil && active.Pet.Inst.ID == inst.ID {
			s.recallPet(p, event.PetRecalledByPlayer)
			return
		}
		// 双击另一只已孵化宠表示换宠。先收回当前宠，再让统一的放出入口执行等级、
		// 信赖、拒绝出战和饥饿结算，避免这里复制一套规则后逐渐分叉。
		s.recallPet(p, event.PetRecalledByPlayer)
	}
	s.onSummonPet(SummonPet{ID: cmd.ID, Inst: inst.ID})
}

// onSummonPet 把一只宠放出来。
func (s *Scene) onSummonPet(cmd SummonPet) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "SummonPet", Reason: r})
	}
	if s.alloc == nil || s.petDefs == nil || !s.petRule.Valid() {
		reject(event.RejectUnknown)
		return
	}
	if p.Player.Pet != 0 {
		reject(event.RejectPetAlreadyOut)
		return
	}
	ch := p.Player.Char
	inst := ch.FindPet(cmd.Inst)
	if !s.petItemAvailable(p, inst) {
		reject(event.RejectNoPet)
		return
	}
	def, ok := s.petDefs[inst.Def]
	if !ok {
		reject(event.RejectUnknown)
		return
	}
	if !inst.Hatched {
		reject(event.RejectPetUnhatched)
		return
	}
	// 0 信赖也允许放出，留出一小段时间让玩家喂无尽淳恢复信赖。
	// 既然是确定性的宽限窗口，就不再对 0 信赖抽低信赖拒绝。
	if inst.Trust > 0 && inst.Trust <= 20 && int32(s.rng.Intn(10000)) < s.petRule.LowTrustRefuseBP {
		reject(event.RejectPetRefused)
		return
	}
	inst.AddStarve(s.petRule.DeployHungerAdd)
	if inst.HP <= 0 {
		inst.HP = inst.MaxHP(def)
	}
	if inst.MP < 0 {
		inst.MP = 0
	}
	// 放出宠物只建立出战实体，不代表玩家使用了任何主动技能。
	inst.ActiveSkill = 0
	inst.Riding = false
	p.Player.MarkDirty()
	s.spawnPet(p, inst, def)
}

// spawnPet 真正把宠物实体放进场景。
func (s *Scene) spawnPet(owner *entity.Entity, inst *domain.PetInstance, def domain.PetDef) {
	e := s.newPetEntity(owner, inst, def)
	s.registerPet(owner, e)
	s.resetPetNeedTimers(e)
	s.emit(s.spawnEvent(e))
	s.emitTo(owner.ID, event.PetSummoned{Who: owner.ID, Entity: e.ID, Inst: inst.ID})
	s.pushPetSnapshot(owner) // 出战位变了
	s.log.Debug("召唤宠物", "char", owner.Name, "宠", e.Name, "等级", inst.Level)
}

func (s *Scene) newPetEntity(owner *entity.Entity, inst *domain.PetInstance, def domain.PetDef) *entity.Entity {
	e := &entity.Entity{
		ID:    s.alloc.Pet(),
		Kind:  domain.KindPet,
		Name:  inst.DisplayName(def),
		Level: inst.Level,
		HP:    inst.HP,
		MaxHP: inst.MaxHP(def),
		MP:    inst.MP,
		MaxMP: inst.MaxMP(def),
		Pos:   owner.Pos,
		Look:  domain.Look{ModelID: inst.RenderModel(def, s.now())},
		Stats: domain.DeriveFromBase(inst.Base),
		Pet: &entity.Pet{
			Owner: owner.ID,
			Inst:  inst, // 指向角色身上那一份, 不拷贝 —— 拷了涨的经验会丢
			Def:   def,
		},
	}
	// 生命上限来自"保留手调差值"那条式子, 不是 DeriveFromBase 里的体质×9。
	// 两者对 460 只宠相同, 对那 44 只活动宠不同 —— 以宠物那条为准。
	e.Stats.MaxHP, e.Stats.MaxMP = e.MaxHP, e.MaxMP
	e.Stats.MoveSpeed = petFollowSpeed(owner)
	if e.HP > e.MaxHP {
		e.HP = e.MaxHP
	}
	return e
}

func (s *Scene) registerPet(owner *entity.Entity, e *entity.Entity) {
	s.entities[e.ID] = e
	s.pets[e.ID] = e
	s.aoi.Enter(e)
	owner.Player.Pet = e.ID
	if e.Pet != nil && e.Pet.Inst != nil {
		e.Pet.Inst.Deployed = true
		e.Pet.Inst.Riding = owner.Player.Riding
	}
}

// persistedPetEntity 在一次全新登录中按存档重建出战宠物。它不执行召唤判定，
// 因而不会重复增加饥饿、抽低信赖拒绝或清掉当前技能；离线期间的需求计时冻结，
// 登录后从新的在线生命周期重新开始计时。
func (s *Scene) persistedPetEntity(owner *entity.Entity) *entity.Entity {
	if owner == nil || owner.Player == nil || owner.Player.Char == nil || s.alloc == nil {
		return nil
	}
	for i := range owner.Player.Char.Pets {
		inst := &owner.Player.Char.Pets[i]
		if !inst.Deployed {
			continue
		}
		def, ok := s.petDefs[inst.Def]
		if !ok || !inst.Hatched {
			inst.Deployed = false
			inst.Riding = false
			inst.ActiveSkill = 0
			owner.Player.MarkDirty()
			return nil
		}
		e := s.newPetEntity(owner, inst, def)
		owner.Player.Riding = inst.Riding
		if inst.Riding {
			owner.Player.MountModel = s.ridingModel(owner, inst, def)
		}
		s.resetPetNeedTimers(e)
		return e
	}
	return nil
}

// transferredPetEntity 在目标场景重建同一只出战宠物，但不执行召唤规则：
// 不增加饥饿、不重置计时器、不重抽信赖，也不改变当前技能。
func (s *Scene) transferredPetEntity(owner *entity.Entity, runtime *entity.PetRuntime) *entity.Entity {
	if owner == nil || owner.Player == nil || owner.Player.Char == nil || runtime == nil ||
		s.alloc == nil || s.petDefs == nil {
		return nil
	}
	inst := owner.Player.Char.FindPet(runtime.Inst)
	if inst == nil || !inst.Hatched {
		return nil
	}
	def, ok := s.petDefs[inst.Def]
	if !ok {
		return nil
	}
	e := s.newPetEntity(owner, inst, def)
	e.RestorePetRuntime(*runtime, s.tick)
	inst.HP, inst.MP = e.HP, e.MP
	return e
}

// detachPetForTransfer 只把实体所有权从源场景摘除。它不是“召回”：
// 不清当前技能、不下马、不发 PetRecalled，也不重推宠物快照。
func (s *Scene) detachPetForTransfer(owner *entity.Entity) *entity.PetRuntime {
	if owner == nil || owner.Player == nil || owner.Player.Pet == 0 {
		return nil
	}
	id := owner.Player.Pet
	pet := s.entities[id]
	owner.Player.Pet = 0 // 场景实体号不能带到目标地图
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return nil
	}
	runtime := pet.CapturePetRuntime(s.tick)
	pet.Pet.Inst.HP, pet.Pet.Inst.MP = pet.HP, pet.MP
	owner.Player.MarkDirty()
	// 旧图其他玩家需要移除跟宠；主人自己的客户端正在进行整图切换，
	// 不能向它伪造一次收宠生命周期。
	s.emitExcept(event.EntityDespawned{ID: id, Kind: pet.Kind, Reason: event.DespawnTimeout}, owner.ID)
	s.aoi.Leave(pet)
	delete(s.entities, id)
	delete(s.pets, id)
	return runtime
}

// detachPetForLogout 结束场景实体生命周期，但保留玩家可感知的宠物状态。
// 下线不是“主动收回”：同一只宠、当前技能和坐骑状态要在下次登录重建。
func (s *Scene) detachPetForLogout(owner *entity.Entity) {
	if owner == nil || owner.Player == nil || owner.Player.Pet == 0 {
		return
	}
	id := owner.Player.Pet
	pet := s.entities[id]
	owner.Player.Pet = 0
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return
	}
	inst := pet.Pet.Inst
	inst.HP, inst.MP = pet.HP, pet.MP
	inst.Deployed = true
	inst.Riding = owner.Player.Riding
	owner.Player.MarkDirty()
	s.emitExcept(event.EntityDespawned{ID: id, Kind: pet.Kind, Reason: event.DespawnRemoved}, owner.ID)
	s.aoi.Leave(pet)
	delete(s.entities, id)
	delete(s.pets, id)
}

// onRecallPet 把宠收回去。
func (s *Scene) onRecallPet(cmd RecallPet) {
	p, ok := s.players[cmd.ID]
	if !ok {
		return
	}
	if p.Player.Pet == 0 {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "RecallPet", Reason: event.RejectNoPet})
		return
	}
	s.recallPet(p, event.PetRecalledByPlayer)
}

// recallPet 收回一个人的宠并广播。没放出来时什么都不做。
//
// **收回前先把血抄回实例** —— 不抄的话召回再召唤就是免费满血。
func (s *Scene) recallPet(owner *entity.Entity, why event.PetRecallReason) {
	if owner.Player == nil || owner.Player.Pet == 0 {
		return
	}
	if owner.Player.Riding {
		s.setRiding(owner, false, false)
	}
	id := owner.Player.Pet
	owner.Player.Pet = 0
	e, ok := s.entities[id]
	if !ok {
		return
	}
	if e.Pet != nil && e.Pet.Inst != nil {
		e.Pet.Inst.HP, e.Pet.Inst.MP = e.HP, e.MP
		// 召回会结束技能效果；下次放出后必须等玩家再次双击
		// 或点击快捷栏，不能沿用上次的“正在使用”。
		e.Pet.Inst.ActiveSkill = 0
		e.Pet.Inst.Deployed = false
		e.Pet.Inst.Riding = false
		owner.Player.MarkDirty()
	}
	s.emit(event.EntityDespawned{ID: id, Kind: e.Kind, Reason: event.DespawnRemoved})
	s.emitTo(owner.ID, event.PetRecalled{Who: owner.ID, Entity: id, Reason: why})
	s.flush()
	s.aoi.Leave(e)
	delete(s.entities, id)
	delete(s.pets, id)
	// 出战位空了 —— 血也刚抄回实例, 这时候的快照才是权威状态。
	// 跨图交接那条路会先摘实体再重发, 所以这里不判 why。
	s.pushPetSnapshot(owner)
	s.log.Debug("收回宠物", "char", owner.Name, "宠", e.Name)
}

// onPetOwnerDeath 只处理当前出战宠物：人物死亡扣 10 点信赖并立即召回。
// 未出战宠物没有场景实体，因此不会被时间或死亡规则误伤。
func (s *Scene) onPetOwnerDeath(owner *entity.Entity) {
	if owner == nil || owner.Player == nil || owner.Player.Pet == 0 {
		return
	}
	pet := s.entities[owner.Player.Pet]
	if pet != nil && pet.Pet != nil && pet.Pet.Inst != nil {
		pet.Pet.Inst.AddTrust(-s.petRule.DeathTrustLoss)
		owner.Player.MarkDirty()
	}
	s.recallPet(owner, event.PetRecalledByOwnerDeath)
}

// stepPets 推进所有宠物：只跟随主人，并在出战期间推进饥饿与信赖。
func (s *Scene) stepPets() {
	for _, e := range s.pets {
		if e.Pet == nil {
			continue
		}
		owner, ok := s.players[e.Pet.Owner]
		if !ok {
			// 主人不在了(离场/换图)。宠物留在场上就是一只没人管的野东西
			s.despawnOrphanPet(e)
			continue
		}
		if !owner.Alive() {
			// 正常死亡入口会先扣信赖再召回；这是给异常状态留下的兜底。
			s.recallPet(owner, event.PetRecalledByOwnerDeath)
			continue
		}
		s.stepOnePet(e, owner)
	}
}

// stepOnePet 推进一只宠。
func (s *Scene) stepOnePet(e *entity.Entity, owner *entity.Entity) {
	s.expirePetTransmog(e, owner)
	s.stepPetNeeds(e)
	if s.stepPetDistrust(e, owner) {
		return
	}
	if _, _, ok := s.petEffect(e.Pet.Inst, "escape_low_hp"); ok && e.MaxHP > 0 &&
		int64(e.HP)*100 < int64(e.MaxHP)*10 {
		s.recallPet(owner, event.PetRecalledByEscape)
		return
	}
	s.stepPetActiveSkill(e, owner)
	speed := petFollowSpeed(owner)
	e.Stats.MoveSpeed = speed
	if owner.Player != nil && owner.Player.Riding {
		// 骑乘时宠物模型由玩家坐骑层承载；实体只在服务端跟住主人，不能继续
		// 广播一条已经从客户端世界移除的跟宠路径。
		if e.Pos != owner.Pos {
			s.aoi.Move(e, owner.Pos)
		}
		return
	}

	// 跟丢太远直接拉回来。直线路径碰到 MASK 障碍时本帧停步；主人继续移动
	// 并超过牵引距离后，也会走同一条瞬移兜底，不会永久卡在墙角。
	if !inRange(e.Pos, owner.Pos, petLeashDist) {
		s.warpPet(e, owner.Pos)
		return
	}
	if s.petPickup(e, owner) {
		return
	}
	if !inRange(e.Pos, owner.Pos, petFollowDist) {
		to := ai.StepToward(e.Pos, owner.Pos, float64(speed))
		if s.straightPathWalkable(e.Pos, to) {
			s.movePet(e, to)
		}
	}
}

func (s *Scene) expirePetTransmog(pet, owner *entity.Entity) {
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || owner == nil || owner.Player == nil ||
		!pet.Pet.Inst.ExpireTransmog(s.now()) {
		return
	}
	model := pet.Pet.Def.ModelID()
	pet.Look.ModelID = model
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
	if owner.Player.Riding {
		owner.Player.MountModel = s.ridingModel(owner, pet.Pet.Inst, pet.Pet.Def)
		s.emit(s.ridingEvent(owner))
	} else {
		s.emit(event.EntityDespawned{ID: pet.ID, Kind: pet.Kind, Reason: event.DespawnRemoved})
		s.emit(s.spawnEvent(pet))
	}
	s.log.Info("宠物幻化到期", "char", owner.Name, "pet", pet.Pet.Inst.ID)
}

// clientPixelsPerMoveSpeed 是角色属性移速到场景像素速度的换算系数。
// 正式 1.3.4 客户端实测 attrs[9]=100、PlayerController.pxPerSpeed=2，
// MoveStepPx() 返回 200；MonsterEntity.MovePath 接收的则直接是像素/秒。
// 所以宠物若原样发送属性值 100，画面只会有主人的一半速度。
const clientPixelsPerMoveSpeed int32 = 2

// petFollowSpeed 返回与主人画面实际移动一致的像素速度；装备和状态改变主人
// 最终移速后，下一帧跟随立即同步。零值只会出现在不完整测试实体上。
func petFollowSpeed(owner *entity.Entity) int32 {
	if owner != nil && owner.Stats.MoveSpeed > 0 {
		return owner.Stats.MoveSpeed * clientPixelsPerMoveSpeed
	}
	return domain.DefaultPlayerMoveSpeed * clientPixelsPerMoveSpeed
}

// movePet 移动宠物并广播。与 moveMonster 同一套跨格处理。
func (s *Scene) movePet(e *entity.Entity, to domain.Pos) {
	if to == e.Pos {
		return
	}
	s.moveEntity(e, to)
}

// warpPet 把宠物直接挪到主人身边（跟丢时用）。
func (s *Scene) warpPet(e *entity.Entity, to domain.Pos) {
	if e == nil || to == e.Pos {
		return
	}
	s.aoi.Move(e, to)
	// speed=-1 的 Snap 才是客户端立即校正；普通 moveEntity 会让它继续沿
	// 一条 900px 的路径慢慢走，达不到“超距直接传送”的效果。
	s.emit(event.EntityMoved{ID: e.ID, To: to, Dir: e.Dir, Snap: true})
}

// despawnOrphanPet 主人已经不在场景里了，把宠物摘掉。
func (s *Scene) despawnOrphanPet(e *entity.Entity) {
	if e.Pet != nil && e.Pet.Inst != nil {
		e.Pet.Inst.HP, e.Pet.Inst.MP = e.HP, e.MP
	}
	s.emit(event.EntityDespawned{ID: e.ID, Kind: e.Kind, Reason: event.DespawnRemoved})
	s.flush()
	s.aoi.Leave(e)
	delete(s.entities, e.ID)
	delete(s.pets, e.ID)
}

// grantPetExp 主人杀怪后给宠物分经验。
//
// **不从主人那份里扣**（见 domain.SharedExp）。
// 上限按 PetCapFor 算：宠物不能超过主人。
func (s *Scene) grantPetExp(owner *entity.Entity, exp int64) {
	if owner.Player == nil || owner.Player.Pet == 0 || s.petLvls == nil {
		return
	}
	e, ok := s.entities[owner.Player.Pet]
	if !ok || e.Pet == nil || e.Pet.Inst == nil {
		return
	}
	share := domain.SharedExp(exp)
	if bonus := owner.Player.PetExperienceBonusPct(s.tick); bonus > 0 {
		share += share * int64(bonus) / 100
	}
	if share <= 0 {
		return
	}
	def := e.Pet.Def
	// 上限按**角色行**上的等级算, 不用 owner.Level。
	// 两者本该相等, 但权威的是 Char.Level(grantExp 也读它);
	// 从两个地方读同一个值, 迟早会读到那个走样的 —— 这个坑在生命上限上踩过一次了。
	s.addPetExperience(owner, e, share, domain.PetCapFor(def, owner.Player.Char.Level))
}

func (s *Scene) addPetExperience(owner, e *entity.Entity, experience int64, cap int32) domain.GainResult {
	inst := e.Pet.Inst
	def := e.Pet.Def
	prefix, _ := s.petPrefixes.Find(inst.Prefix)
	fromLevel := inst.Level
	res := inst.AddExpWithPrefix(def, prefix, s.petLvls, experience, cap)
	owner.Player.MarkDirty()

	if res.Levels > 0 {
		for level := fromLevel + 1; level <= inst.Level; level++ {
			s.resolvePetSkillLevelUp(owner, inst, def, level)
		}
		// 升级要重推属性。**先算上限再补血** —— 反过来会拿旧上限补,
		// 升级后那几点新增的血就白涨了(角色那边踩过同一个坑)
		e.Level = inst.Level
		e.Stats = domain.DeriveFromBase(inst.Base)
		e.MaxHP, e.MaxMP = inst.MaxHP(def), inst.MaxMP(def)
		e.Stats.MaxHP, e.Stats.MaxMP = e.MaxHP, e.MaxMP
		e.HP, e.MP = e.MaxHP, e.MaxMP
		inst.HP, inst.MP = e.HP, e.MP
	}
	s.emitTo(owner.ID, event.PetExpGained{Who: owner.ID, Inst: inst.ID,
		Exp: experience, Level: inst.Level, Ups: res.Levels})
	s.pushPetSnapshot(owner)
	return res
}

// resolvePetSkillLevelUp 在一次真实升级上结算技能。先升级已会技能，再处理药丸
// 指定领悟；药丸失败后仍保留本级自然领悟机会，成功则本级不再额外抽一个技能。
func (s *Scene) resolvePetSkillLevelUp(owner *entity.Entity, inst *domain.PetInstance,
	def domain.PetDef, level int32) {
	upgraded := inst.LevelKnownSkills(s.petSkills)
	learned := domain.SkillID(0)

	if pending := inst.PendingLearn; pending.Item != 0 {
		inst.PendingLearn = domain.PetLearningBoost{}
		skillDef, exists := s.petSkills[pending.Skill]
		chance := s.petLearningChance(inst, pending.ChanceBP)
		if exists && skillDef.LearnLevel <= level && s.canLearnPetSkill(inst, def, pending.Skill) && chance > 0 &&
			int32(s.rng.Intn(10000)) < chance && s.learnPetSkill(inst, pending.Skill) {
			learned = pending.Skill
		}
	}
	naturalChance := s.petLearningChance(inst, s.petRule.NaturalLearnChanceBP)
	if learned == 0 && naturalChance > 0 && int32(s.rng.Intn(10000)) < naturalChance {
		pool := make([]domain.SkillID, 0, len(def.Learnable))
		for _, id := range def.Learnable {
			if skillDef, ok := s.petSkills[id]; ok && skillDef.LearnLevel <= level &&
				s.canNaturallyLearnPetSkill(inst, def, id) {
				pool = append(pool, id)
			}
		}
		if len(pool) > 0 {
			candidate := pool[s.rng.Intn(len(pool))]
			if s.learnPetSkill(inst, candidate) {
				learned = candidate
			}
		}
	}
	if len(upgraded) > 0 || learned != 0 {
		learnedName := ""
		if skill, ok := s.petSkills[learned]; ok {
			learnedName = skill.Name
		}
		s.log.Info("宠物升级技能结算", "char", owner.Name, "宠实例", inst.ID,
			"等级", level, "升级技能数", len(upgraded), "新领悟", learnedName)
	}
}

// ── 饥饿与信赖 ──

func petTrustBand(hunger int32) uint8 {
	switch {
	case hunger <= 20:
		return 0
	case hunger <= 50:
		return 1
	case hunger <= 80:
		return 2
	default:
		return 3
	}
}

func (s *Scene) resetPetNeedTimers(e *entity.Entity) {
	if e == nil || e.Pet == nil || e.Pet.Inst == nil {
		return
	}
	e.Pet.NextHungerAt = s.tick + domain.Ticks(int(s.petHungerInterval(e.Pet.Inst))*1000)
	e.Pet.TrustBand = petTrustBand(e.Pet.Inst.Starve)
	delta, seconds := s.petRule.TrustDeltaAt(e.Pet.Inst.Starve)
	seconds = s.petTrustInterval(e.Pet.Inst, delta, seconds)
	if seconds > 0 {
		e.Pet.NextTrustAt = s.tick + domain.Ticks(int(seconds)*1000)
	} else {
		e.Pet.NextTrustAt = 0
	}
	if e.Pet.Inst.Trust == 0 && s.petRule.ZeroTrustRecallSec > 0 {
		e.Pet.ZeroTrustRecallAt = s.tick + domain.Ticks(int(s.petRule.ZeroTrustRecallSec)*1000)
	} else {
		e.Pet.ZeroTrustRecallAt = 0
	}
}

// stepPetNeeds 只会由场上的宠物实体调用。因此召回、未出战和离线期间，两个
// 数值都会自然冻结。
func (s *Scene) stepPetNeeds(e *entity.Entity) {
	if e == nil || e.Pet == nil || e.Pet.Inst == nil || !s.petRule.Valid() {
		return
	}
	inst := e.Pet.Inst
	changed := false
	if e.Pet.NextHungerAt == 0 {
		s.resetPetNeedTimers(e)
	}
	if e.Pet.NextHungerAt != 0 && s.tick >= e.Pet.NextHungerAt {
		before := inst.Starve
		inst.AddStarve(1)
		changed = changed || inst.Starve != before
		e.Pet.NextHungerAt += domain.Ticks(int(s.petHungerInterval(inst)) * 1000)
	}

	band := petTrustBand(inst.Starve)
	if band != e.Pet.TrustBand {
		e.Pet.TrustBand = band
		delta, seconds := s.petRule.TrustDeltaAt(inst.Starve)
		seconds = s.petTrustInterval(inst, delta, seconds)
		if seconds > 0 {
			e.Pet.NextTrustAt = s.tick + domain.Ticks(int(seconds)*1000)
		} else {
			e.Pet.NextTrustAt = 0
		}
	}
	if delta, seconds := s.petRule.TrustDeltaAt(inst.Starve); delta != 0 &&
		e.Pet.NextTrustAt != 0 && s.tick >= e.Pet.NextTrustAt {
		changed = inst.AddTrust(delta) != 0 || changed
		seconds = s.petTrustInterval(inst, delta, seconds)
		e.Pet.NextTrustAt += domain.Ticks(int(seconds) * 1000)
	}
	if !changed {
		return
	}
	owner, ok := s.players[e.Pet.Owner]
	if !ok || owner.Player == nil {
		return
	}
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
}

// stepPetDistrust 给 0 信赖宠物一段仍可喂食的出战窗口。窗口内恢复到
// 正信赖就取消截止时间；否则到期走现有的不信赖召回链路。
func (s *Scene) stepPetDistrust(e *entity.Entity, owner *entity.Entity) bool {
	if e == nil || e.Pet == nil || e.Pet.Inst == nil || owner == nil || owner.Player == nil {
		return false
	}
	if e.Pet.Inst.Trust > 0 {
		e.Pet.ZeroTrustRecallAt = 0
		return false
	}
	if e.Pet.ZeroTrustRecallAt == 0 {
		if s.petRule.ZeroTrustRecallSec <= 0 {
			return false
		}
		e.Pet.ZeroTrustRecallAt = s.tick + domain.Ticks(int(s.petRule.ZeroTrustRecallSec)*1000)
		return false
	}
	if s.tick < e.Pet.ZeroTrustRecallAt {
		return false
	}
	s.recallPet(owner, event.PetRecalledByDistrust)
	return true
}

// stepPetStarve/retimeStarve 保留给旧的内部调用口径，运行时统一走新规则。
func (s *Scene) stepPetStarve(e *entity.Entity) { s.stepPetNeeds(e) }

func (s *Scene) retimeStarve(e *entity.Entity, _ domain.PetState) { s.resetPetNeedTimers(e) }

// markOwnerDirty 宠物数据变了要标主人脏 —— 宠物是跟角色行同事务存的。
func (s *Scene) markOwnerDirty(e *entity.Entity) {
	if owner, ok := s.players[e.Pet.Owner]; ok {
		owner.Player.MarkDirty()
	}
}

// onFeedPet 喂宠物。
func (s *Scene) onFeedPet(cmd FeedPet) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "FeedPet", Reason: r})
	}
	inst := p.Player.Char.FindPet(cmd.Inst)
	if inst == nil {
		reject(event.RejectNoPet)
		return
	}
	food, ok := s.petFoods[cmd.Item]
	if !ok {
		reject(event.RejectNotFood)
		return
	}
	if p.Player.Bag.UsableCountOf(cmd.Item) <= 0 {
		reject(event.RejectNoItem)
		return
	}
	// **档位不对不能静默吞掉** —— 玩家会以为喂了却没效果, 然后一直喂
	if !food.Usable(inst.Starve) {
		reject(event.RejectWrongFoodTier)
		return
	}

	dStarve, dTrust, ok := inst.Feed(food)
	if !ok {
		reject(event.RejectWrongFoodTier)
		return
	}
	if !p.Player.Bag.Remove(cmd.Item, 1) {
		reject(event.RejectNoItem)
		return
	}
	p.Player.MarkDirty()
	s.pushInventory(p)
	s.emitTo(p.ID, event.PetFed{Who: p.ID, Inst: inst.ID,
		Starve: inst.Starve, Trust: inst.Trust,
		DeltaStarve: dStarve, DeltaTrust: dTrust})
	s.log.Debug("喂宠物", "char", p.Name, "食物", food.Name,
		"饥渴", inst.Starve, "信赖", inst.Trust)
}

// ── 自动拾取 ──
//
// `ov_petgrow.pickup_trust` 是个**布尔标志**（504 行只有 0/1），不是信赖阈值 ——
// 列名骗人。123 只为 1，**18 只可捕捉宠全在其中**，所以这个功能在 MVP 里立刻有用。

// petPickupRange 宠物愿意跑多远去捡东西。
//
// 比跟随距离(120)大一点，但远小于牵引距离(900)：
// 宠物该顺路把主人脚边的东西叼回来，不该为了一根树枝跑到屏幕外面 ——
// 那样它会一直掉队，而且看起来像坏了。
const petPickupRange = 250

// petPickup 让宠物去捡一件东西。返回 true 表示这一帧它在忙这件事，
// 调用方不要再让它跟随。
//
// **只在没架打的时候捡。** 打着架跑去捡东西的宠会一直脱战，
// 而它本来就是替主人挨打的。
func (s *Scene) petPickup(e *entity.Entity, owner *entity.Entity) bool {
	if len(s.ground) == 0 {
		return false
	}
	if _, _, ok := s.petEffect(e.Pet.Inst, "auto_pickup"); !ok {
		return false
	}
	drop := s.nearestPickable(e, owner)
	if drop == nil {
		return false
	}
	if !inRange(e.Pos, drop.Pos, ai.MeleeRange) {
		s.movePet(e, ai.StepToward(e.Pos, drop.Pos, float64(petFollowSpeed(owner))))
		return true
	}
	s.petTake(e, owner, drop)
	return true
}

// nearestPickable 找宠物该去捡的那一件。
//
// **归属保护照样管用**：别人打死的怪掉的东西，在保护期内宠物不能碰 ——
// 否则"派宠物去抢"就成了绕开归属规则的官方外挂。
func (s *Scene) nearestPickable(e *entity.Entity, owner *entity.Entity) *entity.Entity {
	var best *entity.Entity
	bestD := float64(petPickupRange) * float64(petPickupRange)
	for _, d := range s.ground {
		if d.Drop == nil {
			continue
		}
		if d.Drop.Owner != 0 && d.Drop.Owner != owner.ID && s.tick < d.Drop.OwnerUntil {
			continue // 别人的, 还在保护期
		}
		// 距离从**主人**身上量: 宠物再怎么跑也不该把主人拖出视野之外
		if sqDist(owner.Pos, d.Pos) > float64(petPickupRange)*float64(petPickupRange) {
			continue
		}
		if dd := sqDist(e.Pos, d.Pos); dd < bestD {
			best, bestD = d, dd
		}
	}
	return best
}

// petTake 宠物把一件东西捡进**主人的**背包。
//
// 宠物没有自己的背包 —— 给它一个就得回答"宠物死了里面的东西去哪"，
// 而那个问题的任何答案都会让人丢东西。
func (s *Scene) petTake(e *entity.Entity, owner *entity.Entity, drop *entity.Entity) {
	if owner.Player.TradeBusy {
		return // Keep the drop on the ground; the pet retries after settlement.
	}
	def, ok := s.itemDef(drop.Drop.Item)
	if !ok {
		s.removeLoot(drop.ID, event.DespawnPickedUp) // 配置没了, 别让它永远躺着
		return
	}
	left := owner.Player.Bag.Add(def, drop.Drop.Count)
	if left == drop.Drop.Count {
		// 一件都装不下: 说一次就别再刷屏了, 让它回去跟着主人
		if !e.Pet.BagWarned {
			e.Pet.BagWarned = true
			s.emitTo(owner.ID, event.Rejected{Who: owner.ID,
				Cmd: "PetPickUp", Reason: event.RejectBagFull})
		}
		return
	}
	e.Pet.BagWarned = false
	owner.Player.MarkDirty()
	s.pushInventory(owner)

	if left > 0 {
		drop.Drop.Count = left // 只装下一部分, 剩下的留在地上
		return
	}
	s.removeLoot(drop.ID, event.DespawnPickedUp)
	s.emitTo(owner.ID, event.PetPickedUp{Who: owner.ID, Pet: e.ID,
		Item: int32(def.ID), Name: def.Name})
	s.log.Debug("宠物拾取", "char", owner.Name, "宠", e.Name, "物品", def.Name)
}
