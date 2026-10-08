package domain

// 装备穿戴与属性叠加。
//
// **这是战斗公式第一次跑在完整参数上的地方。** 在此之前 Stats 里的暴击率、
// 各种抗性、攻速修正全是零 —— 因为那些数只从装备来。
//
// 叠加顺序见 docs/战斗公式.md〇.2, 一步都不能调换:
//
//	1. 六维   = 基础六维 + Σ装备六维(绝对) , 再 ×(1 + Σ六维百分比)
//	2. 二级   = DeriveFromBase(六维)                    ← 先由六维推
//	3. 二级  += 装备主表 ov_arm 自带的攻防命中
//	4. 二级  += Σ词条绝对值
//	5. 二级  ×= (1 + Σ词条百分比)
//
// 顺序错了结果就不一样: 装备加的力量必须先进六维、再参与"力量÷2=攻击"的推导,
// 直接当二级攻击加上去会少算一半。

// EquipSlot 是装备槽位。取值来自 game_equipment.slot(客户端自己的编号)。
type EquipSlot int32

// 飞升前用得到的槽位。14~25 号槽几乎全是飞升后的东西(60 级以内各只有 2~10 件)。
const (
	SlotFace     EquipSlot = 1  // 面饰
	SlotHead     EquipSlot = 2  // 头
	SlotNeck     EquipSlot = 3  // 项链
	SlotWeapon   EquipSlot = 4  // 单手武器
	SlotShield   EquipSlot = 5  // 盾
	SlotGlove    EquipSlot = 6  // 手套
	SlotRing     EquipSlot = 7  // 戒指
	SlotBody     EquipSlot = 8  // 衣服
	SlotShoe     EquipSlot = 9  // 鞋
	SlotBag      EquipSlot = 10 // 背包(外观)
	SlotTreasure EquipSlot = 11 // 法宝
	// SlotCharm 保留为旧代码兼容名；客户端与 ov_arm 已确认 11 号槽实际是法宝。
	SlotCharm    EquipSlot = SlotTreasure
	SlotTwoHand  EquipSlot = 13 // 双手武器
	maxEquipSlot EquipSlot = 13
)

// 装备属性词条的 attr_id。
//
// **[实证]** 字典由 541 件装备与粉粉兔逐项对拍投票得出, 见 docs/表索引.md 开头。
// 42 种里飞升前用得上的在这里, 其余(飞升属性/宠物属性)先不认。
const (
	AttrSTR       = 1   // 力量
	AttrINT       = 3   // 智慧
	AttrVIT       = 5   // 体质
	AttrAGI       = 7   // 敏捷
	AttrDEX       = 9   // 灵巧
	AttrSPI       = 61  // 精神 —— 注意不在 1..9 那一段里
	AttrDef       = 13  // 防御
	AttrMAtk      = 15  // 魔法攻击
	AttrMDef      = 17  // 魔法防御
	AttrHit       = 19  // 命中
	AttrCrit      = 23  // 爆击率, **万分比**
	AttrAtkSpeed  = 25  // 攻击速度, 负数表示提速
	AttrMoveSpeed = 27  // 移动速度
	AttrMinAtk    = 29  // 最小攻击
	AttrMaxAtk    = 31  // 最大攻击
	AttrMaxHP     = 34  // 最大生命
	AttrMaxMP     = 36  // 最大法力
	AttrHPRegen   = 38  // 生命恢复
	AttrMaxWeight = 40  // 最大负重
	AttrAtk       = 47  // 攻击(同时加上下限)
	AttrMCrit     = 66  // 魔法爆击率, 万分比
	AttrPhysRes   = 69  // 伤害抗性
	AttrMagicRes  = 70  // 魔法抗性
	AttrStatusRes = 118 // 不良状态抗性
)

// 词条的取值方式。**[实证]** ov_card_entry.mode: 0 绝对值, 1 百分比。
const (
	ModeAbsolute = 0
	ModePercent  = 1
)

// Affix 是装备上的一条属性词条。
type Affix struct {
	Attr  int32
	Value int32
	Mode  int32
}

// Requirement 是穿戴要求。
//
// **[实证]** 来自 ov_arm 的 level_need / str_need / vit_need / agi_need /
// int_need / dex_need / spi_need / sex，以及五个基础职业与五个飞升职业开关。
// 属性门槛和职业限制是两套独立规则，必须分别判定。
type Requirement struct {
	Level int32
	Base  Base
	// Sex 是 ov_arm 的性别限制: 0 不限、1 男、2 女。
	// 注意角色协议不是同一套编码：原客户端建角 Lua 明定男=1、女=0。
	Sex uint8
	// Professions 的位号与 Character.Race 一致；同职业的基础/飞升开关
	// 合并到同一位。
	Professions uint8
	// BeginnerAllowed 对应 ov_arm.newbie。ProfessionRestricted 区分“所有职业位
	// 都是 0，表示不限”与“只允许初行者，职业位恰好也是 0”。
	BeginnerAllowed      bool
	ProfessionRestricted bool
}

// Meet 分别判定等级、性别与六维门槛。职业使用 AllowsProfession
// 判定，让上层能给客户端区分两类拒绝原因。
func (r Requirement) Meet(level int32, b Base, gender uint8) RejectReason {
	if level < r.Level {
		return RejectLevelTooLow
	}
	// 角色创建资源中的 malecheck 写 1、femalecheck 写 0；ov_arm.sex 则是 1 男、2 女。
	// 两种性别编码不同，不能直接比较。
	equipSex := uint8(2) // 角色 gender=0（女）
	if gender == 1 {
		equipSex = 1 // 男
	}
	if r.Sex != 0 && r.Sex != equipSex {
		return RejectWrongSex
	}
	if b.STR < r.Base.STR || b.VIT < r.Base.VIT || b.INT < r.Base.INT ||
		b.SPI < r.Base.SPI || b.AGI < r.Base.AGI || b.DEX < r.Base.DEX {
		return RejectStatTooLow
	}
	return RejectNone
}

// AllowsProfession 独立判定职业开关。它不与等级/六维/性别合成一个
// “通用限制”字段，因为客户端会分别展示并给出不同的交互反馈。
func (r Requirement) AllowsProfession(race Race) bool {
	restricted := r.ProfessionRestricted || r.Professions != 0
	return !restricted || race.Valid() && r.Professions&(1<<uint8(race)) != 0
}

// AllowsCharacter 在职业位之外还检查就职状态。初行者可以穿无职业限制的通用
// 装备，但不能因为建角时预选了成长路线就提前穿对应职业装备。
func (r Requirement) AllowsCharacter(c *Character) bool {
	restricted := r.ProfessionRestricted || r.Professions != 0
	if !restricted {
		return true
	}
	if c == nil {
		return false
	}
	if !c.HasProfession() {
		return r.BeginnerAllowed
	}
	return r.AllowsProfession(c.Race)
}

// RejectReason 是穿戴被拒的原因。与 event.RejectReason 分开定义 ——
// domain 不认识 event 包, 由上层翻译。
type RejectReason uint8

const (
	RejectNone RejectReason = iota
	RejectLevelTooLow
	RejectStatTooLow
	RejectWrongSex
	RejectWrongSlot
)

// EquipSet 是身上穿着的一整套。按槽位索引, 空槽的 Item 为 0。
type EquipSet struct {
	worn map[EquipSlot]Stack
}

// NewEquipSet 建一套空的。
func NewEquipSet() *EquipSet { return &EquipSet{worn: map[EquipSlot]Stack{}} }

// At 返回某槽位上穿着的东西。
func (e *EquipSet) At(s EquipSlot) Stack {
	if e == nil {
		return Stack{}
	}
	return e.worn[s]
}

// Set 直接写某个槽位。给存档加载用。
func (e *EquipSet) Set(s EquipSlot, st Stack) {
	if e == nil {
		return
	}
	if st.Empty() {
		delete(e.worn, s)
		return
	}
	e.worn[s] = st
}

// Each 按客户端槽位号从小到大遍历穿着的东西。
//
// worn 的底层是 map，直接 range 会让同一套装备在不同进程里产生不同顺序。
// 属性加法虽然不依赖顺序，但任何派生视图都不该被 Go 的随机 map 顺序影响；
// 固定顺序也让存档、日志和后续协议取证可重复。
func (e *EquipSet) Each(fn func(EquipSlot, Stack)) {
	if e == nil {
		return
	}
	for s := SlotFace; s <= maxEquipSlot; s++ {
		if st := e.worn[s]; !st.Empty() {
			fn(s, st)
		}
	}
}

// Count 返回穿了几件。
func (e *EquipSet) Count() int {
	if e == nil {
		return 0
	}
	return len(e.worn)
}

// Clone 做一份独立副本。
func (e *EquipSet) Clone() *EquipSet {
	if e == nil {
		return nil
	}
	out := NewEquipSet()
	for s, st := range e.worn {
		out.worn[s] = st
	}
	return out
}

// ChangeSet 是 1.5.8 客户端的一套备用装备。键使用客户端 0x109a/0x8073
// 的部位号，而不是背包格号。双手武器在客户端与单手武器共用 4 号武器格，
// 真正穿戴时仍由物品定义恢复为 SlotTwoHand。
type ChangeSet struct {
	items map[EquipSlot]Stack
}

func NewChangeSet() *ChangeSet { return &ChangeSet{items: map[EquipSlot]Stack{}} }

// ChangeSetCell 把真实装备槽归一为客户端备用套装格。0 表示该槽不属于
// 1.5.8 的十一格快速换装面板。
func ChangeSetCell(slot EquipSlot) EquipSlot {
	if slot == SlotTwoHand {
		return SlotWeapon
	}
	if slot >= SlotFace && slot <= SlotTreasure {
		return slot
	}
	return 0
}

func ValidChangeSetCell(cell EquipSlot) bool {
	return cell >= SlotFace && cell <= SlotTreasure
}

func (c *ChangeSet) At(cell EquipSlot) Stack {
	if c == nil {
		return Stack{}
	}
	return c.items[cell]
}

func (c *ChangeSet) Set(cell EquipSlot, st Stack) bool {
	if c == nil || !ValidChangeSetCell(cell) {
		return false
	}
	if st.Empty() {
		delete(c.items, cell)
	} else {
		c.items[cell] = st
	}
	return true
}

func (c *ChangeSet) Each(fn func(EquipSlot, Stack)) {
	if c == nil {
		return
	}
	for cell := SlotFace; cell <= SlotTreasure; cell++ {
		if st := c.items[cell]; !st.Empty() {
			fn(cell, st)
		}
	}
}

func (c *ChangeSet) Count() int {
	if c == nil {
		return 0
	}
	return len(c.items)
}

func (c *ChangeSet) Clone() *ChangeSet {
	if c == nil {
		return nil
	}
	out := NewChangeSet()
	for cell, st := range c.items {
		out.items[cell] = st
	}
	return out
}

// ConflictSlots 返回穿上 slot 时必须先脱下的槽位。
//
// 单手武器与双手武器互斥: 拿双手武器时不能同时拿盾。
// 这是唯一一组冲突 —— 其余槽位互不干涉。
func ConflictSlots(slot EquipSlot) []EquipSlot {
	switch slot {
	case SlotTwoHand:
		return []EquipSlot{SlotWeapon, SlotShield}
	case SlotWeapon, SlotShield:
		return []EquipSlot{SlotTwoHand}
	}
	return nil
}

func AppearancePartAcceptsSlot(part AppearancePart, slot EquipSlot) bool {
	switch part {
	case AppearanceBody:
		return slot == SlotBody
	case AppearanceCap:
		return slot == SlotHead
	case AppearanceBackpack:
		return slot == SlotBag
	case AppearanceWeaponR:
		return slot == SlotWeapon || slot == SlotTwoHand
	case AppearanceWeaponL:
		return slot == SlotShield
	case AppearanceFace:
		return slot == SlotFace
	}
	return false
}

// AppearanceFromEquipment 从持久化穿戴重算 0x800a 的基础外观。
//
// EquipView 与攻击三字段都是穿戴的派生缓存，不是第二份权威数据。每次都从零开始，
// 这样脱下一件装备会真正清掉旧模型，重启后也能用 char_equipment 修复 characters
// 行里可能遗留的旧外观。Gender/Hair/Face 是建角外观，不属于装备，原样保留。
func AppearanceFromEquipment(base Appearance, worn *EquipSet, defs func(ItemID) (ItemDef, bool)) Appearance {
	out := base
	out.EquipView = [appearancePartCount]uint16{}
	out.AtkVariant = 0
	out.WeaponCType = 0
	out.AtkDist = 0

	if defs == nil {
		return out
	}
	worn.Each(func(slot EquipSlot, st Stack) {
		def, ok := defs(st.Item)
		if !ok || def.Equip == nil || EquipSlot(def.Equip.Slot) != slot || !EquipmentFunctional(st, def) {
			return
		}
		baseAppearance := def.Equip.Appearance
		if baseAppearance.Known && baseAppearance.Model != 0 && baseAppearance.Part < appearancePartCount {
			out.EquipView[baseAppearance.Part] = baseAppearance.Model
		}
		if st.FusedAppearance != 0 {
			if fused, ok := defs(st.FusedAppearance); ok && fused.AvatarFusion != nil {
				ap := fused.AvatarFusion.Appearance
				if ap.Known && ap.Part < appearancePartCount && AppearancePartAcceptsSlot(ap.Part, slot) {
					// model=0 是已验证的“隐藏这一部位”换形，不等同于未知。
					out.EquipView[ap.Part] = ap.Model
				}
			}
		}
		if baseAppearance.AttackKnown {
			out.AtkVariant = baseAppearance.AtkVariant
			out.WeaponCType = baseAppearance.WeaponCType
			out.AtkDist = baseAppearance.AtkDist
		}
	})
	return out
}

// AppearanceFromEquipmentAndWardrobe 先从真实穿戴重算属性装备外观，再叠加衣柜。
// 衣柜激活只改模型，不改武器攻击动作、距离或任何战斗属性，也不要求人物先穿
// 对应部位装备；属性始终来自真实装备及其附加内容。
func AppearanceFromEquipmentAndWardrobe(base Appearance, worn *EquipSet, wardrobe *Wardrobe,
	items func(ItemID) (ItemDef, bool), wardrobeDefs WardrobeTable, gender uint8) Appearance {
	out := AppearanceFromEquipment(base, worn, items)
	if wardrobe == nil || len(wardrobeDefs) == 0 {
		return out
	}
	wardrobe.Each(func(_ int, entry WardrobeEntry) {
		if !entry.Worn {
			return
		}
		def, ok := wardrobeDefs[entry.Item]
		if !ok || def.Category != entry.Category || WardrobeBlocked(def, gender) ||
			!def.Appearance.Known || def.Appearance.Part >= appearancePartCount {
			return
		}
		out.EquipView[def.Appearance.Part] = def.Appearance.Model
	})
	return out
}

// WardrobeBlocked 只保留静态数据明确给出的角色性别限制。CannotAttach 表达的是
// “不能附加到装备”，不能据此禁用魔法衣橱的纯外观激活。
func WardrobeBlocked(def WardrobeDef, gender uint8) bool {
	if !def.Category.Valid() || !def.Appearance.Known {
		return true
	}
	if def.Sex != 0 {
		characterSex := uint8(2)
		if gender == 1 {
			characterSex = 1
		}
		if def.Sex != characterSex {
			return true
		}
	}
	return false
}

// ── 属性叠加 ──

// StatSource 是算属性要的全部输入。
type StatSource struct {
	Base  Base // 角色自己的六维(不含装备)
	Level int32
	// Innate 是不由六维推导、但属于角色基础面板的二级属性。当前玩家移速
	// 来自客户端兼容属性 attrs[9]；把它放进基线后，装备/状态的 attr27 才能
	// 以正常基础值计算百分比，而不是永远对 0 加成。
	Innate Stats
	Worn   *EquipSet // 身上穿的
	Defs   func(ItemID) (ItemDef, bool)
	// Passive 是已学被动技能在当前装备条件下产生的属性词条。它属于角色
	// 基线，与装备百分比同桶相加；状态仍在二者之后单独累计。
	Passive []Affix
	// Status 是身上的状态。状态词条与装备词条**共用同一套 attr_id/mode**,
	// 所以走同一条管线, 只是排在装备之后 —— buff 是加在装备之上的。
	Status *StatusSet
}

// Compute 算出角色的最终六维与二级属性。
//
// 五步顺序见本文件开头。**这是唯一一处允许把装备加成算进属性的地方** ——
// 散落在各处的话, 换一件装备就要记得所有地方都重算一遍, 那是必然会漏的。
func Compute(in StatSource) (Base, Stats) {
	src := in
	base := src.Base
	var basePct Base    // 六维的百分比加成
	var flat Stats      // 二级属性的绝对加成
	var pct statPercent // 二级属性的百分比加成
	seenSuits := make(map[int32]bool)

	src.Worn.Each(func(slot EquipSlot, st Stack) {
		def, ok := src.Defs(st.Item)
		if !ok || def.Equip == nil || !EquipmentFunctional(st, def) {
			return
		}
		if suit := def.Equip.Suit; suit != nil && !seenSuits[suit.ID] {
			seenSuits[suit.ID] = true
			pieces, _ := suit.WornMembers(src.Worn, src.Defs)
			for _, bonus := range suit.Bonuses {
				if bonus.DisabledReason == "" && pieces >= bonus.Pieces {
					applyAffix(bonus.Affix, &base, &basePct, &flat, &pct)
				}
			}
		}
		// 步骤 3 的一半: 装备自带的攻防命中先记着, 等六维算完再加
		equipBase, equipStats := def.Equip.RefinedIntrinsic(st.RefineLevel)
		flat = addStats(flat, equipStats)
		// 步骤 1: 装备直接给的六维
		base = base.Add(equipBase)

		for _, a := range def.Equip.Affixes {
			applyAffix(a, &base, &basePct, &flat, &pct)
		}
		for i := 0; i < int(st.RolledAffixCount) && i < len(st.RolledAffixes); i++ {
			applyAffix(st.RolledAffixes[i].Affix, &base, &basePct, &flat, &pct)
		}
		if st.FusedAppearance != 0 {
			if fused, ok := src.Defs(st.FusedAppearance); ok && fused.AvatarFusion != nil &&
				AppearancePartAcceptsSlot(fused.AvatarFusion.Appearance.Part, slot) {
				for _, a := range fused.AvatarFusion.Affixes {
					applyAffix(a, &base, &basePct, &flat, &pct)
				}
			}
		}
		if st.FusedSoul != 0 {
			if soul, ok := src.Defs(st.FusedSoul); ok && soul.EquipmentSoul != nil &&
				soul.EquipmentSoul.AcceptsSlot(slot) {
				for _, a := range soul.EquipmentSoul.Affixes {
					applyAffix(a, &base, &basePct, &flat, &pct)
				}
			}
		}
		if st.FusedDragon != 0 {
			if dragon, ok := src.Defs(st.FusedDragon); ok && dragon.DragonFusion != nil && dragon.DragonFusion.AcceptsSlot(slot) {
				for _, a := range dragon.DragonFusion.Affixes {
					applyAffix(a, &base, &basePct, &flat, &pct)
				}
			}
		}
		for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
			if st.Sockets[i] == 0 {
				continue
			}
			for _, a := range st.SocketCards[i].StaticAffixes() {
				applyAffix(a, &base, &basePct, &flat, &pct)
			}
		}
		for i := 0; i < int(st.WashCount) && i < len(st.WashAffixes); i++ {
			applyAffix(st.WashAffixes[i], &base, &basePct, &flat, &pct)
		}
	})
	for _, a := range src.Passive {
		applyAffix(a, &base, &basePct, &flat, &pct)
	}

	// 先完整算出“角色 + 装备”的基线。状态必须与这份基线分开累计：
	// 两个 +10% 状态应是基线 × 20%，不能把前一个 buff 后的结果再乘 10%。
	base = applyBasePercent(base, basePct)
	baselineBase := base
	baselineStats := applyStatPercent(addStats(addStats(DeriveFromBase(base), src.Innate), flat), pct)

	var statusBase, statusBasePct Base
	var statusFlat Stats
	var statusPct statPercent
	for _, a := range in.Status.Affixes() {
		applyAffix(a, &statusBase, &statusBasePct, &statusFlat, &statusPct)
	}

	// 六维状态同样只以装备基线为百分比基数；固定 buff 不再被百分比 buff 放大。
	base = addBaseStatus(baselineBase, statusBase, statusBasePct)
	primaryDelta := subStats(DeriveFromBase(base), DeriveFromBase(baselineBase))
	out := addStats(baselineStats, primaryDelta)
	out = addStats(out, statusFlat)
	out = addStatPercentFrom(out, baselineStats, statusPct)
	return base, out
}

// ApplyStatusStats 给没有六维/装备管线的怪物叠状态。所有百分比都以模板属性
// 为基数，固定状态和百分比状态彼此不会相乘。
func ApplyStatusStats(baseline Stats, affixes []Affix) Stats {
	var ignoredBase, ignoredBasePct Base
	var flat Stats
	var pct statPercent
	for _, a := range affixes {
		applyAffix(a, &ignoredBase, &ignoredBasePct, &flat, &pct)
	}
	out := addStats(baseline, flat)
	return addStatPercentFrom(out, baseline, pct)
}

// statPercent 攒各二级属性的百分比加成(单位: %)。
type statPercent struct {
	maxHP, maxMP         int32
	atk, def, hit        int32
	matk, mdef           int32
	moveSpeed, atkSpeed  int32
	physRes, magicRes    int32
	statusRes, maxWeight int32
}

func applyAffix(a Affix, base *Base, basePct *Base, flat *Stats, pct *statPercent) {
	pctMode := a.Mode == ModePercent
	v := a.Value

	// 六维词条: 进第 1 步, 不能当二级属性加
	switch a.Attr {
	case AttrSTR:
		addBase(base, basePct, pctMode, func(b *Base) *int32 { return &b.STR }, v)
		return
	case AttrVIT:
		addBase(base, basePct, pctMode, func(b *Base) *int32 { return &b.VIT }, v)
		return
	case AttrINT:
		addBase(base, basePct, pctMode, func(b *Base) *int32 { return &b.INT }, v)
		return
	case AttrSPI:
		addBase(base, basePct, pctMode, func(b *Base) *int32 { return &b.SPI }, v)
		return
	case AttrAGI:
		addBase(base, basePct, pctMode, func(b *Base) *int32 { return &b.AGI }, v)
		return
	case AttrDEX:
		addBase(base, basePct, pctMode, func(b *Base) *int32 { return &b.DEX }, v)
		return
	}

	// 二级属性词条
	switch a.Attr {
	case AttrMaxHP:
		addStat(&flat.MaxHP, &pct.maxHP, pctMode, v)
	case AttrMaxMP:
		addStat(&flat.MaxMP, &pct.maxMP, pctMode, v)
	case AttrMinAtk:
		addStat(&flat.MinAtk, &pct.atk, pctMode, v)
	case AttrMaxAtk:
		addStat(&flat.MaxAtk, &pct.atk, pctMode, v)
	case AttrAtk:
		// 攻击同时加上下限(docs/战斗公式.md: 上下限都 + Σattr47)
		addStat(&flat.MinAtk, &pct.atk, pctMode, v)
		addStat(&flat.MaxAtk, nil, pctMode, v)
	case AttrDef:
		addStat(&flat.Def, &pct.def, pctMode, v)
	case AttrHit:
		addStat(&flat.Hit, &pct.hit, pctMode, v)
	case AttrMAtk:
		addStat(&flat.MAtk, &pct.matk, pctMode, v)
	case AttrMDef:
		addStat(&flat.MDef, &pct.mdef, pctMode, v)
	case AttrCrit:
		addStat(&flat.CritRate, nil, pctMode, v) // 本身就是万分比, 不再叠百分比
	case AttrMCrit:
		addStat(&flat.MCritRate, nil, pctMode, v)
	case AttrAtkSpeed:
		addStat(&flat.AtkSpeedMS, &pct.atkSpeed, pctMode, v)
		if pctMode {
			flat.AtkSpeedPct -= v
		}
	case AttrMoveSpeed:
		addStat(&flat.MoveSpeed, &pct.moveSpeed, pctMode, v)
	case AttrPhysRes:
		addStat(&flat.PhysResist, &pct.physRes, pctMode, v)
	case AttrMagicRes:
		addStat(&flat.MagicResist, &pct.magicRes, pctMode, v)
	case AttrStatusRes:
		addStat(&flat.StatusResist, &pct.statusRes, pctMode, v)
	case AttrHPRegen:
		addStat(&flat.HPRegen, nil, pctMode, v)
	case AttrMaxWeight:
		addStat(&flat.MaxWeight, &pct.maxWeight, pctMode, v)
	}
	// 认不出的 attr_id 直接忽略。42 种里有一批是飞升/宠物属性,
	// 在这里静静跳过比猜一个语义安全。
}

func addBase(base, pct *Base, isPct bool, sel func(*Base) *int32, v int32) {
	if isPct {
		*sel(pct) += v
		return
	}
	*sel(base) += v
}

func addStat(flat, pct *int32, isPct bool, v int32) {
	if isPct && pct != nil {
		*pct += v
		return
	}
	if isPct {
		return // 这一项不支持百分比(如暴击率本身就是万分比), 丢掉
	}
	*flat += v
}

func applyBasePercent(b Base, p Base) Base {
	return Base{
		STR: pctOf(b.STR, p.STR), VIT: pctOf(b.VIT, p.VIT), INT: pctOf(b.INT, p.INT),
		SPI: pctOf(b.SPI, p.SPI), AGI: pctOf(b.AGI, p.AGI), DEX: pctOf(b.DEX, p.DEX),
	}
}

func addBaseStatus(baseline, flat, pct Base) Base {
	return Base{
		STR: baseline.STR + flat.STR + baseline.STR*pct.STR/100,
		VIT: baseline.VIT + flat.VIT + baseline.VIT*pct.VIT/100,
		INT: baseline.INT + flat.INT + baseline.INT*pct.INT/100,
		SPI: baseline.SPI + flat.SPI + baseline.SPI*pct.SPI/100,
		AGI: baseline.AGI + flat.AGI + baseline.AGI*pct.AGI/100,
		DEX: baseline.DEX + flat.DEX + baseline.DEX*pct.DEX/100,
	}
}

func applyStatPercent(s Stats, p statPercent) Stats {
	s.MaxHP = pctOf(s.MaxHP, p.maxHP)
	s.MaxMP = pctOf(s.MaxMP, p.maxMP)
	s.MinAtk = pctOf(s.MinAtk, p.atk)
	s.MaxAtk = pctOf(s.MaxAtk, p.atk)
	s.Def = pctOf(s.Def, p.def)
	s.Hit = pctOf(s.Hit, p.hit)
	s.MAtk = pctOf(s.MAtk, p.matk)
	s.MDef = pctOf(s.MDef, p.mdef)
	s.MoveSpeed = pctOf(s.MoveSpeed, p.moveSpeed)
	s.AtkSpeedMS = pctOf(s.AtkSpeedMS, p.atkSpeed)
	s.PhysResist = pctOf(s.PhysResist, p.physRes)
	s.MagicResist = pctOf(s.MagicResist, p.magicRes)
	s.StatusResist = pctOf(s.StatusResist, p.statusRes)
	s.MaxWeight = pctOf(s.MaxWeight, p.maxWeight)
	return s
}

func addStatPercentFrom(out, baseline Stats, p statPercent) Stats {
	out.MaxHP += baseline.MaxHP * p.maxHP / 100
	out.MaxMP += baseline.MaxMP * p.maxMP / 100
	out.MinAtk += baseline.MinAtk * p.atk / 100
	out.MaxAtk += baseline.MaxAtk * p.atk / 100
	out.Def += baseline.Def * p.def / 100
	out.Hit += baseline.Hit * p.hit / 100
	out.MAtk += baseline.MAtk * p.matk / 100
	out.MDef += baseline.MDef * p.mdef / 100
	out.MoveSpeed += baseline.MoveSpeed * p.moveSpeed / 100
	out.AtkSpeedMS += baseline.AtkSpeedMS * p.atkSpeed / 100
	out.PhysResist += baseline.PhysResist * p.physRes / 100
	out.MagicResist += baseline.MagicResist * p.magicRes / 100
	out.StatusResist += baseline.StatusResist * p.statusRes / 100
	out.MaxWeight += baseline.MaxWeight * p.maxWeight / 100
	return out
}

// pctOf 返回 v ×(1 + p/100)。用整数算 —— 属性是整数, 中间过浮点只会引入
// "同样的装备两次算出不同结果"这种极难查的问题。
func pctOf(v, p int32) int32 {
	if p == 0 {
		return v
	}
	return v + v*p/100
}

func addStats(a, b Stats) Stats {
	a.MaxHP += b.MaxHP
	a.MaxMP += b.MaxMP
	a.HPRegen += b.HPRegen
	a.MPRegen += b.MPRegen
	a.MinAtk += b.MinAtk
	a.MaxAtk += b.MaxAtk
	a.Def += b.Def
	a.Hit += b.Hit
	a.MAtk += b.MAtk
	a.MDef += b.MDef
	a.CritRate += b.CritRate
	a.MCritRate += b.MCritRate
	a.AtkSpeedMS += b.AtkSpeedMS
	a.AtkSpeedPct += b.AtkSpeedPct
	a.MoveSpeed += b.MoveSpeed
	a.PhysResist += b.PhysResist
	a.MagicResist += b.MagicResist
	a.StatusResist += b.StatusResist
	a.MaxWeight += b.MaxWeight
	return a
}

func subStats(a, b Stats) Stats {
	return Stats{
		MaxHP: a.MaxHP - b.MaxHP, MaxMP: a.MaxMP - b.MaxMP,
		HPRegen: a.HPRegen - b.HPRegen, MPRegen: a.MPRegen - b.MPRegen,
		MinAtk: a.MinAtk - b.MinAtk, MaxAtk: a.MaxAtk - b.MaxAtk,
		Def: a.Def - b.Def, Hit: a.Hit - b.Hit,
		MAtk: a.MAtk - b.MAtk, MDef: a.MDef - b.MDef,
		CritRate: a.CritRate - b.CritRate, MCritRate: a.MCritRate - b.MCritRate,
		AtkSpeedMS: a.AtkSpeedMS - b.AtkSpeedMS, AtkSpeedPct: a.AtkSpeedPct - b.AtkSpeedPct, MoveSpeed: a.MoveSpeed - b.MoveSpeed,
		PhysResist:   a.PhysResist - b.PhysResist,
		MagicResist:  a.MagicResist - b.MagicResist,
		StatusResist: a.StatusResist - b.StatusResist,
		MaxWeight:    a.MaxWeight - b.MaxWeight,
	}
}
