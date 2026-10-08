package domain

import (
	"sort"
	"time"
)

// 状态（buff / debuff）。
//
// 数据出处，三张表拼起来：
//
//	ov_exceptdesc               206 个状态的名字与说明（type = 状态号）
//	ov_exceptdetail_entry       每级的持续时间(last_time 秒)、周期(interval 秒)
//	ov_exceptdetail_effect_entry 每级的属性影响 —— **用的是和装备词条同一套 attr_id/mode**
//
// 效果那张表逐条与状态自己的说明文字对上了:
//
//	衰弱 → attr47(攻击) mode1 -10   说明「攻击力下降10%」   ✅
//	降幅 → attr15(魔攻) mode1 -5    说明「魔法攻击力下降5%」 ✅
//	致盲 → attr19(命中) mode1 -5    说明「命中下降5%」       ✅
//	减速 → attr27(移速) mode1 -10   说明「移动速度降低10%」   ✅
//	中毒 → attr38(生命恢复) mode0 -10, interval=2  说明「每两秒损失10点生命值」 ✅
//
// ⚠️ **只认 op_type=1 且 mode∈{0,1} 且 attr_id 在字典里的行。**
// 3195 行效果里只有 603 行(82 个状态)过得了这一关; 其余是 mode=87/176/219、
// value=25600 这种明显错位的残渣, 与装备词条那批"没有 attr_name"的行是同一类问题。
//
// 客户端包没有完整的 SkillResult 实表：ov_skilldesc.except_ 全零，ov_skilllimit
// 表达的是施法前置/互斥条件而不是产出。当前只挂接技能描述与状态等级在数值上
// 能逐字段闭合的部分；技能文本明确给出不同时长时由 StatusApplication 覆盖，
// 其余不按名字猜。

// StatusID 是状态号（ov_exceptdesc.type）。
type StatusID int32

// 飞升前常见的几个。
const (
	StatusPoison        StatusID = 1001 // 中毒: 周期掉血
	StatusStun          StatusID = 1002 // 昏迷
	StatusSilence       StatusID = 1003 // 封印: 不能放技能
	StatusFreeze        StatusID = 1004 // 冰冻
	StatusPetrify       StatusID = 1005 // 石化
	StatusWeaken        StatusID = 1006 // 衰弱: 攻击 -10%
	StatusDampen        StatusID = 1007 // 降幅: 魔攻魔防 -5%
	StatusBlind         StatusID = 1008 // 致盲: 命中 -5%
	StatusSlow          StatusID = 1009 // 减速: 移速 -10%
	StatusIronSkin      StatusID = 1012 // 钢铁
	StatusBerserk       StatusID = 1013 // 狂暴
	StatusPoisonHit     StatusID = 1018 // 使毒: 普通攻击概率附加中毒
	StatusDamageShield  StatusID = 1021 // 伤害吸收
	StatusSpiritLock    StatusID = 1022 // 锁灵: 免疫有害状态
	StatusKillingIntent StatusID = 1025 // 杀意: 强化下一次普通攻击
	StatusFocus         StatusID = 1026 // 集中: 延长下一个有时限技能
	StatusRelease       StatusID = 1027 // 释能: 强化下一个攻击技能
	StatusInstantCast   StatusID = 1028 // 瞬发: 缩短下一个技能吟唱
	StatusChaos         StatusID = 1029 // 混沌: 阻止增益状态
	StatusStealth       StatusID = 1032 // 隐身: 攻击时解除
	StatusFireShield    StatusID = 1037
	StatusIceShield     StatusID = 1038
	// StatusBeginnerGuard 没有独立的 ov_exceptdetail 行；客户端技能说明明确给出
	// 30 秒、伤害抗性 +10%，因此沿用技能号作为稳定的运行时状态号。
	StatusBeginnerGuard StatusID = StatusID(SkillBeginnerGuard)
)

// StatusType 是 ov_exceptdetail_entry.except_type 的官方分类。
//
// 数值与客户端 RES_EXCEPT_TYPE_* 常量一致。这个字段是逐级的：
// 同一状态在不同等级上可能不同，不能只按 StatusID 缓存。
type StatusType uint8

const (
	StatusGood     StatusType = iota // RES_EXCEPT_TYPE_GOOD
	StatusBad                        // RES_EXCEPT_TYPE_BAD
	StatusAttack                     // RES_EXCEPT_TYPE_ATTACK（光环/攻击态）
	StatusSign1                      // RES_EXCEPT_TYPE_SIGN1
	StatusSign2                      // RES_EXCEPT_TYPE_SIGN2
	StatusInternal                   // RES_EXCEPT_TYPE_INTER
	StatusGoodNew                    // RES_EXCEPT_TYPE_GOOD_NEW
)

// StatusPositionOrbit 是客户端 BuffView 的人物环绕展示模式。
// 状态样式由客户端按技能/状态号自行取得；这里仅决定它落在脚底还是环绕人物。
const StatusPositionOrbit uint8 = 1

// Control 是控制类状态的行为。
type Control uint8

const (
	ControlNone    Control = iota
	ControlStun            // 完全不能行动
	ControlSilence         // 不能放技能, 但能普通攻击与移动
)

// StatusDef 是一个状态在某一级上的定义。
type StatusDef struct {
	ID    StatusID
	Level int32
	Name  string
	Desc  string
	Type  StatusType
	// TypeKnown 区分“官方数据明确为 GOOD(0)”和旧测试/内存定义的零值。
	// 生产数据始终为 true；false 时保留旧的效果符号回退，不让手工定义静默变义。
	TypeKnown bool

	// 以下是客户端展示元数据。状态自身有图标时保留；没有时由本次实际施加
	// 来源补充：技能用技能图标，物品用物品图标。不同来源不得全局归一。
	Icon int32
	// SourceSkillID 只记录本次技能来源；物品来源或状态原图标为0。
	SourceSkillID int32
	Invisible     bool
	// PreventInvisible 表示该状态生效期间不能获得隐身。它用于神术：鹰眼
	// 破隐后的持续压制；不能借用减速/禁无敌等旧状态来附带实现。
	PreventInvisible bool
	Immobile         uint8
	// Hidden 来自 no_inform_client；这类运行时状态参与数值结算，但不得进入任何
	// 客户端 Buff、目标栏或实体状态列表。
	Hidden bool

	// DurationSec 持续秒数（last_time）。
	DurationSec int32
	// IntervalSec 周期触发间隔（interval）。0 表示不周期触发。
	IntervalSec int32

	// Affixes 属性影响，与装备词条共用同一套结构与语义。
	Affixes []Affix
	// HPChanges / MPChanges 是每个 interval 对当前生命/法力的修改。
	// attr 446/447 与最大值或恢复属性不同，不能塞进 Stats 当常驻词条。
	HPChanges []ResourceChange
	MPChanges []ResourceChange
	// Control 控制行为，非 ControlNone 时该状态会限制行动。
	Control Control

	// 以下字段是不能塞进 Stats 的战斗状态语义。数值均来自状态效果表或
	// 与状态逐级闭合的技能描述；百分比使用整数百分数。
	PhysicalDamageReductionPct int32
	MagicDamageReductionPct    int32
	// DamageTakenPct 是目标承受伤害的增幅，先于减伤结算。破甲同时增加
	// 物理/魔法伤害；冰封强化只增加物理伤害，不能伪装成负减伤。
	PhysicalDamageTakenPct int32
	MagicDamageTakenPct    int32
	PhysicalReflectPct     int32
	MagicReflectPct        int32
	// MagicFullReflectChancePct 是魔镜术这种“按概率把整次魔法伤害折回去”的
	// 触发率。它和冰盾 6 级的“固定反射 50% 伤害”不是同一种语义。
	MagicFullReflectChancePct int32
	// PhysicalReflectFlat / MagicReflectFlat 是按本次来袭类型触发的固定反伤。
	// ReflectFlatAsMagic 用于火焰护罩这类无论来袭类型都返回法术伤害的状态。
	PhysicalReflectFlat int32
	MagicReflectFlat    int32
	ReflectFlatAsMagic  bool
	// DamageShield 是可消耗的固定伤害池，例如光盾术 700/900/... 点。
	DamageShield int32

	BlockHarmful    bool
	BlockBeneficial bool
	BreakOnDamage   bool
	BreakOnAttack   bool
	// StealthDamagePct 是隐身期间对下一次攻击行为的伤害增幅。它本身不消费
	// 状态：普通攻击或技能是否破隐由 BreakOnAttack 与对应被动共同裁决；若
	// 被动保住隐身，后续攻击仍继续享受隐身加成。
	StealthDamagePct int32

	PoisonOnHitChance    int32
	PoisonOnHitLevel     int32
	FreezeAttackerChance int32

	NextBasicAttackPct       int32
	NextSkillDamagePct       int32
	NextStatusDurationPct    int32
	NextCastTimeReductionPct int32

	LifeStealPct    int32
	LifeStealChance int32
	ManaStealPct    int32
	ManaStealChance int32

	ClearHarmful bool
	Invulnerable bool
	// InstantDeathImmune 只阻挡技能附加的即死判定；普通伤害仍照常经过护盾和减伤。
	InstantDeathImmune bool
	CannotAttack       bool
	// AttackCapKnown 区分“没有攻击上限”和致残 1 级明确要求的上限 0。
	AttackCapKnown           bool
	AttackCap                int32
	MaxHPFloor               int32
	ExperienceBonusPct       int32
	OutgoingMagicDamageFlat  int32
	MagicDamageReductionFlat int32
	ManaToHealthRate         int32
}

// ResourceChange 是一次周期资源变化。Mode=0 为点数，Mode=1 为上限百分比。
type ResourceChange struct {
	Value int32
	Mode  int32
}

const (
	StatusAttrCurrentHP int32 = 446
	StatusAttrCurrentMP int32 = 447
)

// StatusApplication 是效果脚本里一条已经闭合的“施加某级状态”。
type StatusApplication struct {
	ID    StatusID
	Level int32
	// ChanceBP 是这条状态在技能真实命中后的独立施加概率（万分比）。零值用于
	// 旧的内存定义并按 10000 处理；PostgreSQL 结构化配置始终显式保存。
	ChanceBP int32
	// DurationSec 非 0 时覆盖状态表自身时长。部分技能与状态共用同一组数值，
	// 但技能描述明确给了不同持续时间（例如祝福术 5 分钟、强化状态 60 秒）。
	DurationSec int32
	// ExtraAffixes 是来源明确、但基础状态表没有携带的额外属性。
	// 例如祝福术复用“强化”外观，同时还明确提供魔攻加成；它们仍归入同一个
	// 状态实例，避免为了补数值额外展示一枚“增幅”图标。
	ExtraAffixes []Affix
	// Description 非空时覆盖基础状态说明。来源技能比共用状态定义包含更多
	// 已确认效果时，客户端提示也必须如实展示完整数值。
	Description string
	// InstantDeathImmune 是技能映射本身补充的语义。同一个官方护盾状态可被
	// 其它来源复用，只有明确写出“免疫即死”的技能才设置它。
	InstantDeathImmune bool
	// StealthDamagePct 是来源技能赋给隐身状态的攻击伤害增幅。官方 1032
	// 状态只保存了持续时间和移速，伤害数值存在于隐身术技能说明中。
	StealthDamagePct int32
	// PhysicalDamageTakenPct 是技能来源对该状态补充的物理易伤。
	// 冰封强化用它给冰封本身追加易伤，不创建第二个状态图标。
	PhysicalDamageTakenPct int32
	// BlockHarmful / Control 是技能来源比共用状态表更完整时的
	// 行为补充。典型是狂暴术：共用“狂暴”状态只保存攻防与移速，技能说明
	// 另外明确写了免疫异常、持续期间不能施法。
	BlockHarmful bool
	Control      Control
}

// Periodic 报告这是不是周期性状态（中毒那类）。
func (d StatusDef) Periodic() bool { return d.IntervalSec > 0 }

// Harmful 按官方 except_type 判断是否为受状态抗性影响的减益。
// 旧的手工定义没有 TypeKnown，对它们保留原有回退规则。
func (d StatusDef) Harmful() bool {
	if d.TypeKnown {
		return d.Type == StatusBad
	}
	if d.Control != ControlNone {
		return true
	}
	for _, a := range d.Affixes {
		if a.Value < 0 {
			return true
		}
	}
	return false
}

// StatusKey 唯一定位一个状态的某一级。
type StatusKey struct {
	ID    StatusID
	Level int32
}

// StatusTable 是全部状态定义。
type StatusTable map[StatusKey]StatusDef

// Get 取某状态某一级的定义。
func (t StatusTable) Get(id StatusID, level int32) (StatusDef, bool) {
	d, ok := t[StatusKey{ID: id, Level: level}]
	return d, ok
}

// OverlayRule 是 ov_exceptoverlay 的官方状态相互作用码。
// 方向由本地矩阵不对称样本确认：Existing 是矩阵行，Incoming 是列。
type OverlayRule uint8

const (
	OverlayBlock       OverlayRule = iota // RES_EXCEPT_MUTABLE：现有状态拦住新状态
	OverlayCoexist                        // RES_EXCEPT_COEXIST：共存
	OverlayCounteract                     // RES_EXCEPT_COUNTERACT：相消
	OverlayReplace                        // RES_EXCEPT_REPLACE：新等级不低时替换
	OverlayMustReplace                    // RES_EXCEPT_MUSTREPLACE：无条件替换
)

type OverlayKey struct {
	Existing StatusID
	Incoming StatusID
}

// StatusOverlay 是启动时载入内存的状态叠加矩阵。
type StatusOverlay map[OverlayKey]OverlayRule

func (m StatusOverlay) Rule(existing, incoming StatusID) (OverlayRule, bool) {
	r, ok := m[OverlayKey{Existing: existing, Incoming: incoming}]
	return r, ok
}

// Active 是一个实体身上正在生效的一个状态。
type Active struct {
	Def StatusDef
	// ExpireAt 到期帧。
	ExpireAt Tick
	// NextTickAt 下次周期触发的帧。非周期状态为 0。
	NextTickAt Tick
	// Source 是施加者，用于把中毒掉的血算到它头上。
	Source EntityID
	// ShieldLeft 是这个状态实例还剩多少固定伤害吸收量。
	ShieldLeft int32
}

// SavedStatus 是角色离场时一个状态实例的完整快照。正增益保存剩余帧，
// 减益保存 UTC 到期时间；RemainingTicks=-1 表示永久状态。
// Source 是场景实体号，重登后不能沿用，故不进入存档。
type SavedStatus struct {
	Def             StatusDef
	RemainingTicks  int64
	ExpiresAtUnixMS int64
	NextTickIn      uint64
	ShieldLeft      int32
	SourceSelf      bool
}

func CloneSavedStatuses(src []SavedStatus) []SavedStatus {
	out := append([]SavedStatus(nil), src...)
	for i := range out {
		out[i].Def.Affixes = append([]Affix(nil), src[i].Def.Affixes...)
		out[i].Def.HPChanges = append([]ResourceChange(nil), src[i].Def.HPChanges...)
		out[i].Def.MPChanges = append([]ResourceChange(nil), src[i].Def.MPChanges...)
	}
	return out
}

// StatusSet 是一个实体身上的全部状态。同一状态 id 可有多个实例：
// ov_exceptoverlay 有 150 个对角线是 COEXIST，不能再用 map[id]Active 静默覆盖。
type StatusSet struct {
	act map[StatusID][]Active
}

// NewStatusSet 建一个空的。
func NewStatusSet() *StatusSet { return &StatusSet{act: map[StatusID][]Active{}} }

// Apply 是给旧手工场景定义的兼容入口。生产场景必须用 ApplyWithOverlay。
func (s *StatusSet) Apply(def StatusDef, now Tick, src EntityID) {
	if s == nil {
		return
	}
	if s.act == nil {
		s.act = map[StatusID][]Active{}
	}
	a := newActive(def, now, src)
	old := s.act[def.ID]
	if len(old) > 0 && old[0].Def.Level > def.Level {
		// 低等级盖不掉高等级的效果, 但可以续时长
		a.Def = old[0].Def
		if old[0].ExpireAt > a.ExpireAt {
			a.ExpireAt = old[0].ExpireAt
		}
		a.NextTickAt = old[0].NextTickAt
	}
	s.act[def.ID] = []Active{a}
}

func newActive(def StatusDef, now Tick, src EntityID) Active {
	a := Active{
		Def: def, ExpireAt: now + Ticks(int(def.DurationSec)*1000), Source: src,
		ShieldLeft: def.DamageShield,
	}
	if def.Periodic() {
		a.NextTickAt = now + Ticks(int(def.IntervalSec)*1000)
	}
	return a
}

// StatusApplyResult 描述一次矩阵解算的原子结果。
type StatusApplyResult struct {
	Applied      bool
	Blocked      bool
	Counteracted bool
	Removed      []StatusID
}

// BlockedByOverlay 只做预检，不修改状态集。
// REPLACE 比较等级；MUSTREPLACE 不比较。
func (s *StatusSet) BlockedByOverlay(def StatusDef, overlay StatusOverlay) bool {
	if s == nil || len(s.act) == 0 || len(overlay) == 0 {
		return false
	}
	for id, list := range s.act {
		rule, ok := overlay.Rule(id, def.ID)
		if !ok {
			continue
		}
		if rule == OverlayBlock {
			return true
		}
		if rule == OverlayReplace {
			for _, old := range list {
				if old.Def.Level > def.Level {
					return true
				}
			}
		}
	}
	return false
}

// ApplyWithOverlay 按矩阵一次性解算所有已有状态与新状态的关系。
func (s *StatusSet) ApplyWithOverlay(def StatusDef, now Tick, src EntityID, overlay StatusOverlay) StatusApplyResult {
	if s == nil {
		return StatusApplyResult{Blocked: true}
	}
	// 没注入矩阵的是旧测试/手工场景，保持 Apply 的单实例、高等级优先语义。
	// 正式服启动时会载入完整矩阵，不会走这里。
	if len(overlay) == 0 {
		s.Apply(def, now, src)
		return StatusApplyResult{Applied: true}
	}
	if s.act == nil {
		s.act = map[StatusID][]Active{}
	}
	if s.BlockedByOverlay(def, overlay) {
		return StatusApplyResult{Blocked: true}
	}

	var counteract, replace []StatusID
	ids := make([]int, 0, len(s.act))
	for id := range s.act {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, raw := range ids {
		id := StatusID(raw)
		rule, ok := overlay.Rule(id, def.ID)
		if !ok {
			// 矩阵没覆盖的新版状态：异 id 共存，同 id 用旧的高级优先。
			if id == def.ID {
				rule = OverlayReplace
			} else {
				rule = OverlayCoexist
			}
		}
		switch rule {
		case OverlayCounteract:
			counteract = append(counteract, id)
		case OverlayReplace, OverlayMustReplace:
			replace = append(replace, id)
		}
	}
	if len(counteract) > 0 {
		for _, id := range counteract {
			delete(s.act, id)
		}
		return StatusApplyResult{Counteracted: true, Removed: counteract}
	}
	for _, id := range replace {
		delete(s.act, id)
	}
	s.act[def.ID] = append(s.act[def.ID], newActive(def, now, src))
	return StatusApplyResult{Applied: true, Removed: replace}
}

// Remove 驱散一个状态。
func (s *StatusSet) Remove(id StatusID) {
	if s == nil {
		return
	}
	delete(s.act, id)
}

// RemoveWhere 清除满足条件的全部状态，返回真正消失的状态号。
func (s *StatusSet) RemoveWhere(match func(StatusDef) bool) []StatusID {
	if s == nil || match == nil {
		return nil
	}
	ids := s.sortedIDs()
	removed := make([]StatusID, 0, len(ids))
	for _, id := range ids {
		list := s.act[id]
		kept := list[:0]
		for _, a := range list {
			if !match(a.Def) {
				kept = append(kept, a)
			}
		}
		if len(kept) == 0 {
			if len(list) > 0 {
				removed = append(removed, id)
			}
			delete(s.act, id)
		} else {
			s.act[id] = kept
		}
	}
	return removed
}

// BlocksStatus 报告现有的锁灵/混沌是否禁止新状态进入。
func (s *StatusSet) BlocksStatus(def StatusDef) bool {
	if s == nil {
		return false
	}
	harmful := def.Harmful()
	blocked := false
	s.Each(func(a Active) {
		if harmful && a.Def.BlockHarmful || !harmful && a.Def.BlockBeneficial {
			blocked = true
		}
	})
	return blocked
}

// InstantDeathImmune 报告当前任一持续状态是否阻挡即死判定。
func (s *StatusSet) InstantDeathImmune() bool {
	if s == nil {
		return false
	}
	immune := false
	s.Each(func(a Active) {
		immune = immune || a.Def.InstantDeathImmune || a.Def.Invulnerable
	})
	return immune
}

// MitigateDamage 应用百分比减伤和固定伤害盾，并更新可消耗的护盾余量。
// 返回最终伤害及因耗尽而消失的状态号。
func (s *StatusSet) MitigateDamage(magic bool, damage int32) (int32, []StatusID) {
	if s == nil || damage <= 0 {
		return damage, nil
	}
	var increase, reduction, flatReduction int32
	s.Each(func(a Active) {
		if a.Def.Invulnerable {
			reduction = 100
		}
		if magic {
			increase += a.Def.MagicDamageTakenPct
			reduction += a.Def.MagicDamageReductionPct
			flatReduction += a.Def.MagicDamageReductionFlat
		} else {
			increase += a.Def.PhysicalDamageTakenPct
			reduction += a.Def.PhysicalDamageReductionPct
		}
	})
	if increase > 0 {
		damage += damage * increase / 100
	}
	if reduction > 100 {
		reduction = 100
	}
	if reduction > 0 {
		damage -= damage * reduction / 100
	}
	damage -= flatReduction
	if damage < 0 {
		damage = 0
	}

	var removed []StatusID
	for _, id := range s.sortedIDs() {
		list := s.act[id]
		kept := list[:0]
		for _, a := range list {
			if damage > 0 && a.ShieldLeft > 0 {
				absorbed := damage
				if absorbed > a.ShieldLeft {
					absorbed = a.ShieldLeft
				}
				damage -= absorbed
				a.ShieldLeft -= absorbed
			}
			if a.Def.DamageShield > 0 && a.ShieldLeft == 0 {
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == 0 {
			if len(list) > 0 {
				removed = append(removed, id)
			}
			delete(s.act, id)
		} else {
			s.act[id] = kept
		}
	}
	return damage, removed
}

// CannotDealDamage 报告实体是否处在“不能攻击别人”的状态。
func (s *StatusSet) CannotDealDamage() bool {
	if s == nil {
		return false
	}
	blocked := false
	s.Each(func(a Active) { blocked = blocked || a.Def.CannotAttack })
	return blocked
}

// OutgoingDamageFlat 是状态对本次最终攻击伤害的固定修正。
func (s *StatusSet) OutgoingDamageFlat(magic bool) int32 {
	if s == nil || !magic {
		return 0
	}
	var out int32
	s.Each(func(a Active) { out += a.Def.OutgoingMagicDamageFlat })
	return out
}

func (s *StatusSet) MinimumMaxHP() int32 {
	if s == nil {
		return 0
	}
	var floor int32
	s.Each(func(a Active) {
		if a.Def.MaxHPFloor > floor {
			floor = a.Def.MaxHPFloor
		}
	})
	return floor
}

// MaximumAttack 返回状态要求的最终物理攻击上限。多个上限同时存在时取最严的；
// 致残 1 级的 0 是有效配置，必须用第二个返回值与“未配置”区分。
func (s *StatusSet) MaximumAttack() (int32, bool) {
	if s == nil {
		return 0, false
	}
	var cap int32
	known := false
	s.Each(func(a Active) {
		if !a.Def.AttackCapKnown {
			return
		}
		if !known || a.Def.AttackCap < cap {
			cap = a.Def.AttackCap
			known = true
		}
	})
	return cap, known
}

func (s *StatusSet) ExpBonusPct() int32 {
	if s == nil {
		return 0
	}
	var pct int32
	s.Each(func(a Active) { pct += a.Def.ExperienceBonusPct })
	return pct
}

// ReflectPct 返回本次物理/魔法伤害应反射的比例。
func (s *StatusSet) ReflectPct(magic bool) int32 {
	if s == nil {
		return 0
	}
	var pct int32
	s.Each(func(a Active) {
		if magic {
			pct += a.Def.MagicReflectPct
		} else {
			pct += a.Def.PhysicalReflectPct
		}
	})
	if pct > 100 {
		return 100
	}
	return pct
}

// MagicFullReflectChance 返回把整次魔法伤害反弹的总触发率。
// 同时存在多个来源时相加，最终封顶 100%。
func (s *StatusSet) MagicFullReflectChance() int32 {
	if s == nil {
		return 0
	}
	var chance int32
	s.Each(func(a Active) { chance += a.Def.MagicFullReflectChancePct })
	if chance > 100 {
		return 100
	}
	return chance
}

// ReflectFlat 返回本次物理/魔法来袭应追加的固定反伤，以及该固定部分是否按
// 法术伤害展示。它与百分比反射独立，法宝火焰护罩会同时适用于两种来袭类型。
func (s *StatusSet) ReflectFlat(magic bool) (amount int32, asMagic bool) {
	if s == nil {
		return 0, false
	}
	s.Each(func(a Active) {
		if magic {
			amount += a.Def.MagicReflectFlat
		} else {
			amount += a.Def.PhysicalReflectFlat
		}
		if a.Def.ReflectFlatAsMagic && (a.Def.PhysicalReflectFlat > 0 || a.Def.MagicReflectFlat > 0) {
			asMagic = true
		}
	})
	return amount, asMagic
}

// CombatTriggers 汇总不消耗的攻击触发状态。
func (s *StatusSet) CombatTriggers() (poisonChance, poisonLevel, freezeChance, lifePct, lifeChance, manaPct, manaChance int32) {
	if s == nil {
		return
	}
	s.Each(func(a Active) {
		if a.Def.PoisonOnHitChance > poisonChance {
			poisonChance, poisonLevel = a.Def.PoisonOnHitChance, a.Def.PoisonOnHitLevel
		}
		freezeChance += a.Def.FreezeAttackerChance
		lifePct += a.Def.LifeStealPct
		manaPct += a.Def.ManaStealPct
		if a.Def.LifeStealChance > lifeChance {
			lifeChance = a.Def.LifeStealChance
		}
		if a.Def.ManaStealChance > manaChance {
			manaChance = a.Def.ManaStealChance
		}
	})
	if freezeChance > 100 {
		freezeChance = 100
	}
	if lifeChance == 0 && lifePct > 0 {
		lifeChance = 100
	}
	if manaChance == 0 && manaPct > 0 {
		manaChance = 100
	}
	return
}

// StealthDamageBonus 汇总当前隐身状态提供的攻击增伤。通常只有一个隐身实例；
// 仍按状态集合求和，保持与其他状态属性的叠加规则一致。
func (s *StatusSet) StealthDamageBonus() int32 {
	if s == nil {
		return 0
	}
	var bonus int32
	s.Each(func(a Active) {
		if a.Def.Invisible && a.Def.StealthDamagePct > 0 {
			bonus += a.Def.StealthDamagePct
		}
	})
	return bonus
}

// ConsumeAttackModifiers 取出并清除本次出手真正用得上的一次性强化。
// 普通攻击只传 skill=false；技能按自身是否有伤害、时限和吟唱分别传入。
func (s *StatusSet) ConsumeAttackModifiers(skill, hasDamage, hasDuration, hasCast bool) (damagePct, durationPct, castReductionPct int32, removed []StatusID) {
	if s == nil {
		return
	}
	ids := s.sortedIDs()
	for _, id := range ids {
		list := s.act[id]
		consume := false
		for _, a := range list {
			if skill {
				if hasDamage && a.Def.NextSkillDamagePct > 0 {
					damagePct += a.Def.NextSkillDamagePct
					consume = true
				}
				if hasDuration && a.Def.NextStatusDurationPct > 0 {
					durationPct += a.Def.NextStatusDurationPct
					consume = true
				}
				if hasCast && a.Def.NextCastTimeReductionPct > 0 {
					castReductionPct += a.Def.NextCastTimeReductionPct
					consume = true
				}
			} else if a.Def.NextBasicAttackPct > 0 {
				damagePct += a.Def.NextBasicAttackPct
				consume = true
			}
		}
		if consume {
			delete(s.act, id)
			removed = append(removed, id)
		}
	}
	return
}

func (s *StatusSet) sortedIDs() []StatusID {
	if s == nil || len(s.act) == 0 {
		return nil
	}
	ids := make([]int, 0, len(s.act))
	for id := range s.act {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	out := make([]StatusID, len(ids))
	for i, id := range ids {
		out[i] = StatusID(id)
	}
	return out
}

// Has 报告身上有没有某个状态。
func (s *StatusSet) Has(id StatusID) bool {
	if s == nil {
		return false
	}
	return len(s.act[id]) > 0
}

// Invisible 报告状态集合中是否存在真正的隐身实例。不能只认固定状态号：
// 客户端数据还存在其他隐身来源，领域语义应跟随定义字段。
func (s *StatusSet) Invisible() bool {
	if s == nil {
		return false
	}
	for _, list := range s.act {
		for _, active := range list {
			if active.Def.Invisible {
				return true
			}
		}
	}
	return false
}

// PreventsInvisible 报告当前是否有状态禁止再次隐身。
func (s *StatusSet) PreventsInvisible() bool {
	if s == nil {
		return false
	}
	for _, list := range s.act {
		for _, active := range list {
			if active.Def.PreventInvisible {
				return true
			}
		}
	}
	return false
}

// Count 返回身上有几个状态。
func (s *StatusSet) Count() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, list := range s.act {
		n += len(list)
	}
	return n
}

// Each 遍历生效中的状态。
func (s *StatusSet) Each(fn func(Active)) {
	if s == nil {
		return
	}
	for _, list := range s.act {
		for _, a := range list {
			fn(a)
		}
	}
}

// SaveForLogout 按状态实例保存可恢复的运行态，保留同 ID 多层叠加。
func (s *StatusSet) SaveForLogout(now Tick, wall time.Time, owner EntityID) []SavedStatus {
	if s == nil {
		return nil
	}
	var out []SavedStatus
	for _, id := range s.sortedIDs() {
		for _, a := range s.act[id] {
			if a.ExpireAt != 0 && a.ExpireAt <= now {
				continue
			}
			record := SavedStatus{Def: a.Def, ShieldLeft: a.ShieldLeft, SourceSelf: owner != 0 && a.Source == owner}
			if a.ExpireAt == 0 {
				record.RemainingTicks = -1
			} else {
				remaining := a.ExpireAt - now
				record.RemainingTicks = int64(remaining)
				if a.Def.Harmful() {
					record.ExpiresAtUnixMS = wall.UTC().Add(time.Duration(remaining.Millis()) * time.Millisecond).UnixMilli()
				}
			}
			if a.NextTickAt > now {
				record.NextTickIn = uint64(a.NextTickAt - now)
			}
			out = append(out, record)
		}
	}
	return out
}

// RestoreSavedStatuses 在新的场景时间轴恢复角色状态。离线期不给周期伤害补账；
// 减益仍按 UTC 到期时间扣除离线流逝，来源实体号清零。
func RestoreSavedStatuses(saved []SavedStatus, now Tick, wall time.Time, owner EntityID) *StatusSet {
	out := NewStatusSet()
	for _, record := range saved {
		if record.Def.ID <= 0 || record.RemainingTicks == 0 || record.RemainingTicks < -1 {
			continue
		}
		a := Active{Def: record.Def, ShieldLeft: record.ShieldLeft}
		if record.SourceSelf {
			a.Source = owner
		}
		var elapsedMS int64
		if record.RemainingTicks > 0 {
			remaining := uint64(record.RemainingTicks)
			if record.Def.Harmful() {
				if record.ExpiresAtUnixMS <= 0 {
					continue
				}
				delta := record.ExpiresAtUnixMS - wall.UTC().UnixMilli()
				if delta <= 0 {
					continue
				}
				remaining = uint64(Ticks(int(delta)))
				elapsedMS = wall.UTC().UnixMilli() - (record.ExpiresAtUnixMS - record.RemainingTicks*TickMS)
				if elapsedMS < 0 {
					elapsedMS = 0
				}
			}
			a.ExpireAt = now + Tick(remaining)
		}
		if record.Def.Periodic() {
			next := record.NextTickIn
			interval := uint64(Ticks(int(record.Def.IntervalSec) * 1000))
			if next == 0 {
				next = interval
			}
			if record.Def.Harmful() && elapsedMS > 0 && interval > 0 {
				elapsedTicks := uint64(Ticks(int(elapsedMS)))
				if elapsedTicks >= next {
					next = interval - (elapsedTicks-next)%interval
				} else {
					next -= elapsedTicks
				}
			}
			a.NextTickAt = now + Tick(next)
		}
		out.act[a.Def.ID] = append(out.act[a.Def.ID], a)
	}
	return out
}

// Expire 清掉所有到期的状态，返回被清掉的状态号。
func (s *StatusSet) Expire(now Tick) []StatusID {
	if s == nil || len(s.act) == 0 {
		return nil
	}
	var gone []StatusID
	for id, list := range s.act {
		kept := list[:0]
		removed := false
		for _, a := range list {
			if a.ExpireAt != 0 && now >= a.ExpireAt {
				removed = true
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == 0 {
			delete(s.act, id)
		} else {
			s.act[id] = kept
		}
		if removed {
			gone = append(gone, id)
		}
	}
	return gone
}

// DueTicks 返回本帧该周期触发的状态，并把它们的下次触发时间往后排。
func (s *StatusSet) DueTicks(now Tick) []Active {
	if s == nil || len(s.act) == 0 {
		return nil
	}
	var due []Active
	for id, list := range s.act {
		for i, a := range list {
			if a.NextTickAt == 0 || now < a.NextTickAt {
				continue
			}
			due = append(due, a)
			a.NextTickAt = now + Ticks(int(a.Def.IntervalSec)*1000)
			list[i] = a
		}
		s.act[id] = list
	}
	return due
}

// Controlled 返回身上最强的控制效果。
//
// 昏迷压过封印 —— 昏迷本来就包含"不能放技能"。
func (s *StatusSet) Controlled() Control {
	if s == nil {
		return ControlNone
	}
	out := ControlNone
	for _, list := range s.act {
		for _, a := range list {
			if a.Def.Control == ControlStun {
				return ControlStun
			}
			if a.Def.Control != ControlNone {
				out = a.Def.Control
			}
		}
	}
	return out
}

// Affixes 把身上所有状态的属性影响摊平，交给属性管线。
func (s *StatusSet) Affixes() []Affix {
	if s == nil || len(s.act) == 0 {
		return nil
	}
	var out []Affix
	for _, list := range s.act {
		for _, a := range list {
			out = append(out, a.Def.Affixes...)
		}
	}
	return out
}

// Rebase 把状态里的场景绝对帧换算到另一张场景的时间轴。
//
// 每张场景都有独立的 Tick；跨场景时直接沿用 ExpireAt/NextTickAt 会让状态
// 提前结束，或凭空延长。这里保留的是从 from 起还剩多少帧，再挂到 to 上。
// 已经到期的 deadline 落在 to，交给目标场景本帧的到期处理清掉。
func (s *StatusSet) Rebase(from, to Tick) {
	if s == nil || len(s.act) == 0 {
		return
	}
	for id, list := range s.act {
		for i, a := range list {
			if a.ExpireAt != 0 {
				a.ExpireAt = rebaseDeadline(a.ExpireAt, from, to)
			}
			// 0 是“非周期状态”的哨兵，不能重基准成 to。
			if a.NextTickAt != 0 {
				a.NextTickAt = rebaseDeadline(a.NextTickAt, from, to)
			}
			list[i] = a
		}
		s.act[id] = list
	}
}

func rebaseDeadline(at, from, to Tick) Tick {
	if at <= from {
		return to
	}
	return to + (at - from)
}

// Clone 做一份独立副本。
func (s *StatusSet) Clone() *StatusSet {
	if s == nil {
		return nil
	}
	out := NewStatusSet()
	for k, v := range s.act {
		out.act[k] = append([]Active(nil), v...)
	}
	return out
}

// ResistChance 返回抗性把状态挡下来的概率(百分比, 0~90)。
//
// **这是「不良状态抗性」第一次真的起作用** —— 1404 条装备词条带这个属性,
// 但在此之前没有任何东西会施加状态, 所以那个数是死的。
//
// 上限 90%: 100% 免疫会让一件装备直接废掉整条控制路线。
// ⚠️ 上限本身是**服务端定的**, 原作数据里没有说法。
func ResistChance(statusResist int32) int32 {
	if statusResist <= 0 {
		return 0
	}
	if statusResist > 90 {
		return 90
	}
	return statusResist
}
