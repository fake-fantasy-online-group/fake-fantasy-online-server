package domain

// 技能。
//
// 数据出处: `gamedata.ov_skilldesc` —— 飞升前五个职业共 **150 个技能 × 5 级 = 750 行**。
//
// 几个字段的含义是查出来的, 记在这里免得下次再查一遍:
//
//	skill_type   1 单体主动 / 2 范围主动 / 18 被动 / 17 特殊(只有龙杀剑)
//	hurt_type    1 物理 / 8 火 / 9 冰 / 12 圣 / 13 暗 / 0 不造成伤害
//	dist         施法距离。法系单体是 450 —— 与远程武器的 atk_dist 同一个值
//	radius       范围技能的半径
//	prepare      吟唱时间(毫秒)
//	desc_        效果文本, 伤害倍率与治疗量**只能从这里解**
//
// 主动技能的直接伤害/治疗从 desc_ 解析；能与 ov_exceptdetail 逐字段闭合的
// buff/debuff 另外装进 Statuses。位移等特殊机制只有在 PostgreSQL 显式配置了
// 服务端结果参数后才可用，不靠名字相似度猜效果。

// SkillID 是技能的配置 id。
type SkillID int32

// 初行者出生即会的两个技能。客户端技能描述明确注明它们会在就职后消失。
const (
	SkillBeginnerBolt  SkillID = 13601 // 魔弹术
	SkillBeginnerGuard SkillID = 13602 // 守护术
	SkillFishing       SkillID = 11006 // 钓鱼（数值是熟练度，刚学会时可为 0）
	SkillMining        SkillID = 11007 // 挖矿（数值是熟练度，刚学会时可为 0）
)

// IsLifeSkill 报告这是否是以熟练度作为数值的生活技能。
// 它们与战斗技能不同：数值 0 也可以表示“已学会，熟练度为 0”。
func IsLifeSkill(id SkillID) bool { return id == SkillFishing || id == SkillMining }

// SkillKind 是技能形态。
type SkillKind uint8

const (
	SkillSingle  SkillKind = iota // skill_type 1: 单体
	SkillArea                     // skill_type 2: 范围
	SkillPassive                  // skill_type 18: 被动, 学了就一直生效
	SkillSpecial                  // skill_type 17: 只有龙杀剑一个
)

// ParseSkillKind 把 ov_skilldesc.skill_type 映射成形态。
func ParseSkillKind(t int32) SkillKind {
	switch t {
	case 2:
		return SkillArea
	case 17:
		return SkillSpecial
	case 18:
		return SkillPassive
	}
	return SkillSingle
}

// Active 报告这是不是主动技能(要玩家按出来的)。龙杀剑在原表里单独
// 标为 17，但它仍是需要目标、消耗法力并进入施法流程的主动技能。
func (k SkillKind) Active() bool {
	return k == SkillSingle || k == SkillArea || k == SkillSpecial
}

// SkillAreaShape 是客户端 ov_skilldesc.range_type 已经给出的范围几何。
// 0 只用于陷阱、位移、侦测等由独立场景机制处理的技能。
type SkillAreaShape uint8

const (
	SkillAreaNone SkillAreaShape = iota
	SkillAreaRectangle
	SkillAreaCircle
	SkillAreaCone
)

// ParseSkillAreaShape 把客户端范围类型原样映射成服务端几何类型。
func ParseSkillAreaShape(raw int32) SkillAreaShape {
	switch raw {
	case 1:
		return SkillAreaRectangle
	case 2:
		return SkillAreaCircle
	case 3:
		return SkillAreaCone
	default:
		return SkillAreaNone
	}
}

// SkillAreaCenter 决定范围相对谁展开。客户端原始字段没有单独保存这个语义；
// PostgreSQL 的 game_skill_area_rules 根据技能说明把它显式闭合。
type SkillAreaCenter uint8

const (
	SkillAreaCenterNone SkillAreaCenter = iota
	SkillAreaCenterCaster
	SkillAreaCenterTarget
)

// SkillAreaTargeting 是范围技能完整的选目标几何。
type SkillAreaTargeting struct {
	Shape  SkillAreaShape
	Center SkillAreaCenter
	Radius int32 // 圆/扇形半径
	Angle  int32 // 扇形总角度（度）
	Length int32 // 矩形沿施法方向的长度；来自原始 width
	Width  int32 // 矩形横向宽度；来自原始 height
}

// Valid 报告这是不是一份可以参与目标裁决的闭合规则。
func (a SkillAreaTargeting) Valid() bool {
	if a.Center != SkillAreaCenterCaster && a.Center != SkillAreaCenterTarget {
		return false
	}
	switch a.Shape {
	case SkillAreaRectangle:
		return a.Length > 0 && a.Width > 0 && a.Radius == 0 && a.Angle == 0
	case SkillAreaCircle:
		return a.Radius > 0 && a.Angle >= 0 && a.Length == 0 && a.Width == 0
	case SkillAreaCone:
		return a.Center == SkillAreaCenterCaster && a.Radius > 0 && a.Angle > 0 && a.Angle < 360 &&
			a.Length == 0 && a.Width == 0
	default:
		return false
	}
}

// 伤害类型。**[实证]** 由 ov_skilldesc.prof 与技能名交叉确认:
// prof=4 是治愈术(hurt_type 12 圣)所以是药师, prof=5 是火弹术(8 火)所以是术士。
const (
	HurtNone     = 0
	HurtPhysical = 1
	HurtFire     = 8
	HurtIce      = 9
	HurtHoly     = 12
	HurtDark     = 13
)

// EffectKind 是技能效果的种类。
type EffectKind uint8

const (
	// EffectNone 效果还做不了。**不是"没有效果"** ——
	// 是那 280 行 buff/debuff/位移技能, 要等状态系统。
	EffectNone EffectKind = iota
	EffectDamage
	EffectHeal
	EffectRestoreMP
	EffectRestoreHP
	EffectDamageMP
	EffectTaunt
	EffectTeleportPoint
	EffectTeleportTarget
	EffectPlaceTrap
	EffectDetectTrap
	EffectDisarmTrap
	EffectDamageMPMax
	EffectRevealInvisible
	EffectInstantDeath
	EffectResurrect
	EffectKnockback
)

// SkillEffectTarget 决定一个有序效果作用于已解析目标还是施法者自身。
type SkillEffectTarget uint8

const (
	EffectTargetResolved SkillEffectTarget = iota
	EffectTargetCaster
)

// SkillEffect 是从 desc_ 解出来的效果。
type SkillEffect struct {
	Kind EffectKind
	// Target 默认为客户端/场景解析出的目标；Caster 用于饮血剑、噬魂剑这类
	// 命中敌人后恢复施法者自身资源的后续段。
	Target SkillEffectTarget
	// RequiresPreviousLanded 用于有序链：前一段 MISS 时，本段不执行。
	RequiresPreviousLanded bool
	// DamagePct 伤害倍率(百分比)。「对敌人造成208%伤害」→ 208。
	DamagePct int32
	// DamageFlat 是命中后追加的同类型固定伤害，不参与基础攻击的护甲/抗性扣除，
	// 但仍参与后续状态增减伤。龙杀剑用它表达“每消耗 1 点法力追加 1 点物伤”。
	DamageFlat int32
	// HealPctOfMax / HealFlat: 「目标恢复最大生命的10%+100点当前生命」→ 10, 100。
	HealPctOfMax int32
	HealFlat     int32
	// RestoreMPFlat: 「目标恢复100点法力」→ 100。
	RestoreMPFlat int32
	// SourceAttackPct: 「攻击力的20%转化为生命/法力」→ 20。
	SourceAttackPct int32
	// MaxResourcePct 用于新星陷阱“失去最大法力的 N%”。它与破法之枪按
	// 施法者攻击力削减法力是两种公式，不能共用 SourceAttackPct。
	MaxResourcePct int32
	// TrapRadius 是放置陷阱的触发/效果半径；原始技能行的 radius=0，精确值
	// 只存在于技能说明，因此作为结构化服务端参数落库。
	TrapRadius int32
	// RevealRadius 是鹰眼按技能说明执行破隐的范围。原始 radius 与说明文字
	// 存在版本差异，因此显式落库，不在运行时猜用哪一个。
	RevealRadius int32
	// WeaponDurabilityCostPct 是释放时按武器满耐久扣除的百分比。它属于
	// 施法资源而不是命中结果：裂空之枪即使 MISS，也已经消耗了武器耐久。
	WeaponDurabilityCostPct int32
	// ClearHarmful 是法宝“魔咒退去/神火罩”这类立即驱散效果。
	// 它不是持续状态，必须在技能命中时直接结算。
	ClearHarmful bool
	// RemoveStatusIDs 是净化术、解咒术、秘术消除这类只驱散指定状态的
	// 白名单。DispelChanceBP 使用万分比；10000 表示本次驱散必定成功。
	RemoveStatusIDs []StatusID
	DispelChanceBP  int32
	// GuaranteedHit 是技能结果明确声明的必中语义。它与装备词条的“无视防御”
	// 不是一回事：前者属于技能配置，不能借后者的字段冒充。
	GuaranteedHit bool
	// TauntPower 是战吼/挑衅的仇恨覆盖优先级。客户端只保留“升级后效果
	// 更佳”而没有旧服绝对仇恨值，因此以技能等级作为可比较强度；普通受击
	// 仇恨为 0，高等级嘲讽可以从低等级嘲讽手里接管目标。
	TauntPower int32
	// ChanceBP 是特殊机制自身的成功率（万分比）。普通伤害命中和选择性驱散
	// 各有独立结算字段，不能复用这里制造两次概率判定。
	ChanceBP int32
	// LossRefundPct 仅用于复活效果，按本次死亡时已经实际扣除的经验与金钱
	// 返还；不能按复活时余额重新推算。
	LossRefundPct int32
	// KnockbackDistance 是命中后沿施法者→目标方向推动怪物的世界距离。
	KnockbackDistance int32
}

type PassiveModifierKind uint8

const (
	PassiveSkillDamagePct PassiveModifierKind = iota + 1
	PassiveEffectSourceAttackPct
	PassiveCastTimeReductionPct
	PassiveIncomingMagicHalfChance
	PassiveStealthDurationSec
	PassiveStealthMoveSpeedPct
	PassiveStealthDamagePct
	PassivePreserveInvisibleChance
	PassiveSkillDurabilityReductionPct
	PassiveAttackDurabilityReductionPct
	PassiveTrapHalfDamageChance
	PassiveStatusPhysicalDamageTakenPct
	PassiveCastInterruptReductionPct
)

// PassiveModifier 是已学被动技能对战斗结算的参数化修正。
// TargetSkill=0 表示全局规则；EffectKind 只供效果数值增强使用。
type PassiveModifier struct {
	Kind        PassiveModifierKind
	TargetSkill SkillID
	EffectKind  EffectKind
	Value       int32
}

// PassiveStatModifier 是被动技能对角色基础战斗属性的确定性修正。
// RequiredEquipType=0 表示无装备条件；非零时必须实际装备对应 ov_arm.type。
type PassiveStatModifier struct {
	Attr              int32
	Value             int32
	Mode              int32
	RequiredEquipType int32
}

// PassiveStatusResist 是只针对一个状态的额外抵抗率。它不能塞进通用
// Stats.StatusResist，否则“昏迷抗性”会错误地同时抵抗中毒、封印等全部减益。
type PassiveStatusResist struct {
	Status StatusID
	Pct    int32
}

// PassiveTriggerStatus 让一个已学被动为指定主动技能追加概率状态。
// 状态定义与持续时间仍来自 PostgreSQL 状态表；这里仅保存关联和概率。
type PassiveTriggerStatus struct {
	TargetSkill SkillID
	Status      StatusID
	StatusLevel int32
	ChanceBP    int32
	DurationSec int32
}

// PassiveStatusAffix 把被动提供的属性修正合并进目标主动技能原本施加的状态，
// 从而严格继承该状态的持续时间与覆盖规则。
type PassiveStatusAffix struct {
	TargetSkill SkillID
	Status      StatusID
	Affix       Affix
}

// PassiveTriggerEffect 让被动为指定主动技能追加一个有序直接效果。
// 当前用于“命中后概率击退”，效果排在原伤害之后且要求前段命中。
type PassiveTriggerEffect struct {
	TargetSkill SkillID
	Effect      SkillEffect
}

// PassiveCounterAttack 是成功闪避后触发的近战反击规则。
type PassiveCounterAttack struct {
	ChanceBP          int32
	DamagePct         int32
	RequiredEquipType int32
}

// SkillDef 是一个技能在某一级上的定义。
type SkillDef struct {
	ID    SkillID
	Level int32 // 1~5
	Name  string
	// Description 是 PostgreSQL ov_skilldesc.desc_ 的原始技能说明。
	Description string
	// Icon 来自 ov_skilldesc.small_map；该列与客户端 skills.json.icon 逐行闭合。
	Icon int32
	Prof int32 // 1~5, = Race + 1
	Kind SkillKind
	// BindingArm 非 0 表示该技能由对应法宝临时授予，不进入角色永久已学技能。
	BindingArm ItemID
	// RequiredWeaponType 来自 ov_skilldesc.arm_type。主动技能用它校验额外耐久
	// 代价对应的武器；耐久类被动用它限定实际生效的武器子类型。
	RequiredWeaponType int32

	LevelNeed int32 // 学习所需角色等级
	// Book/LearnMoney 来自 ov_skilldesc.book_id/learn_money。生活技能学习必须
	// 在服务端消耗对应技能书；没有书时客户端按钮也应保持不可学习。
	Book       ItemID
	LearnMoney int64
	// Prerequisites 来自 depend1_id～depend4_id。当前正式数据的 qty 全为 0，
	// 语义是“至少已学习该技能”，不能把 0 错当成无需前置。
	Prerequisites []SkillID
	HurtType      int32
	Effect        SkillEffect
	// Effects 是 PostgreSQL 中显式配置的有序直接效果。为空时仍兼容从 desc_
	// 解析出的单个 Effect；一旦非空，它就是直接效果的唯一事实来源。
	Effects []SkillEffect
	// Statuses 是该技能命中后施加的状态。它与 Effect 独立，因为重击这类技能
	// 会同时造成伤害并施加昏迷。
	Statuses []StatusApplication
	// PassiveModifiers 只存在于被动技能定义上，由已学等级决定当前规则。
	PassiveModifiers      []PassiveModifier
	PassiveStats          []PassiveStatModifier
	PassiveResists        []PassiveStatusResist
	PassiveTriggers       []PassiveTriggerStatus
	PassiveStatusAffixes  []PassiveStatusAffix
	PassiveTriggerEffects []PassiveTriggerEffect
	PassiveCounter        *PassiveCounterAttack

	// Dist 施法距离, Radius 范围半径(单体为 0)。
	Dist   int32
	Radius int32
	// Area 是范围技能的完整几何。Radius 保留为客户端原始字段，供陷阱侦测、
	// 破隐等独立机制使用；普通群攻的目标选择只读 Area。
	Area SkillAreaTargeting
	// HitEffect 是命中目标后由 0x8045 挂到该实体位置的一次性技能特效名。
	// 来源是 PostgreSQL ov_skillshow 的 be_h/r_be_h show id。
	HitEffect string
	// GroundEffect 是原表明确的位置表现（不是附着施法者的动作特效）。
	// 必须通过0x800e.Aim传实际施法中心，不能用首个受击者替代。
	GroundEffect bool
	// ProjectileSpeedPXPerSec 非零表示客户端有 fly_show 飞行阶段。速度由
	// 最大施法距离 / fly_show_delay 推导，运行时再按释放瞬间实际距离计算命中时间。
	ProjectileSpeedPXPerSec int32
	// ImpactDelayMS 是无弹道技能在 phase=1 释放后到客户端表现命中点的延迟。
	// 它来自 ov_skillshow 的 atk/be_h/r_be_h show delay；弹道技能只使用距离/速度。
	ImpactDelayMS int32
	// PrepareMS 吟唱时间(毫秒)。
	PrepareMS int32
	// CooldownMS 是技能自己的冷却。ov_skilldesc.sep 的单位是 0.1 秒；
	// 玩家与怪物最终还要分别经过各自的全局出手间隔裁决。
	CooldownMS int32

	// MPCost 消耗的法力。
	//
	// **[实证]** ov_skilldesc.sp_chg_control 决定怎么读 sp_chg_start:
	//   control=0 → 直接就是消耗值(515 行, 值域 0~554; 那 190 个 0 正好是全部被动技能)
	//   control=1 → **低 16 位**才是消耗(235 行, 23~1092)
	// 高 16 位(13~90)是另一个参数, **含义未知, 没有用它**。
	MPCost int32

	// 目标类型。来自 target_self / target_mon / target_team 等一组开关。
	TargetSelf  bool
	TargetEnemy bool
	TargetTeam  bool
	// RequiresInvisible 来自“只能在隐身状态下使用”这类明确技能说明。
	RequiresInvisible bool
	// PreserveInvisibleChanceBP 是已学被动对本次技能的破隐豁免概率。
	PreserveInvisibleChanceBP int32
	// WeaponDurabilityReductionPct 是已学被动对本次技能额外耐久代价的减免。
	// 100 表示仍需要正确且未损坏的武器，但本次不再扣耐久。
	WeaponDurabilityReductionPct int32
}

// Usable 报告这个技能现在能不能真的放出效果。
//
// 定义加载了不等于做得了。直接效果和已闭合的状态效果至少有一个才允许释放，
// 避免位移或缺参数的触发技能白扣法力。
func (d SkillDef) Usable() bool {
	return d.Kind.Active() && (len(d.DirectEffects()) > 0 || len(d.Statuses) > 0)
}

// DirectEffects 返回应按顺序结算的直接效果。显式结构化配置优先；旧技能在
// 逐批迁移完成前继续使用 desc_ 解析出的单效果。
func (d SkillDef) DirectEffects() []SkillEffect {
	if len(d.Effects) > 0 {
		return d.Effects
	}
	if d.Effect.Kind != EffectNone || d.Effect.ClearHarmful || len(d.Effect.RemoveStatusIDs) > 0 {
		return []SkillEffect{d.Effect}
	}
	return nil
}

// HasDamageEffect 报告这次技能结算是否包含直接伤害。
func (d SkillDef) HasDamageEffect() bool {
	for _, effect := range d.DirectEffects() {
		if effect.Kind == EffectDamage {
			return true
		}
	}
	return false
}

// HasResurrectEffect 报告该技能是否允许把死亡的队友作为目标。
func (d SkillDef) HasResurrectEffect() bool {
	for _, effect := range d.DirectEffects() {
		if effect.Kind == EffectResurrect {
			return true
		}
	}
	return false
}

// ResurrectOnly 报告技能对存活目标没有任何结果。复活术属于此类；光之奇迹
// 还带净化与治疗，所以存活队友仍是合法目标。
func (d SkillDef) ResurrectOnly() bool {
	if len(d.Statuses) > 0 {
		return false
	}
	effects := d.DirectEffects()
	if len(effects) == 0 {
		return false
	}
	for _, effect := range effects {
		if effect.Kind != EffectResurrect {
			return false
		}
		if effect.ClearHarmful || len(effect.RemoveStatusIDs) > 0 {
			return false
		}
	}
	return true
}

// WeaponDurabilityCostPct 返回释放技能时应按武器满耐久扣除的百分比。
// 同一技能只能配置一次这种施法资源；加载器负责拒绝重复配置。
func (d SkillDef) WeaponDurabilityCostPct() int32 {
	for _, effect := range d.DirectEffects() {
		if effect.WeaponDurabilityCostPct > 0 {
			return effect.WeaponDurabilityCostPct
		}
	}
	return 0
}

// SkillKey 唯一定位一个技能的某一级。
type SkillKey struct {
	ID    SkillID
	Level int32
}

// SkillTable 是全部技能定义。
type SkillTable map[SkillKey]SkillDef

// Get 取某技能某一级的定义。
func (t SkillTable) Get(id SkillID, level int32) (SkillDef, bool) {
	d, ok := t[SkillKey{ID: id, Level: level}]
	return d, ok
}

// MaxLevelOf 返回某技能的最高等级。0 表示没有这个技能。
func (t SkillTable) MaxLevelOf(id SkillID) int32 {
	var max int32
	for k := range t {
		if k.ID == id && k.Level > max {
			max = k.Level
		}
	}
	return max
}

// Learned 是一个角色学会的技能: 技能 id → 已学等级。
type Learned map[SkillID]int32

// NewBeginnerSkills 返回新角色出生时的已学技能集合。
//
// 初行者的 13601/13602 出生时尚未学习，不能冒充成已学等级持久化或写进
// 0x8015；学习入口仍按建角路线只放行其中一项。
func NewBeginnerSkills() Learned {
	return Learned{}
}

// RemoveBeginnerSkills 在就职时移除全部初行者技能。
func (c *Character) RemoveBeginnerSkills() {
	if c == nil {
		return
	}
	if c.Skills != nil {
		delete(c.Skills, SkillBeginnerBolt)
		delete(c.Skills, SkillBeginnerGuard)
	}
	// 技能书是整份覆盖，但快捷栏单独持久化；只删技能不清快捷栏会让
	// 就职后的重登仍残留两个已经不能释放的初行者图标。
	for i := range c.Hotbar {
		id := SkillID(c.Hotbar[i].ID)
		if c.Hotbar[i].Kind == HotbarKindSkill &&
			(id == SkillBeginnerBolt || id == SkillBeginnerGuard) {
			c.Hotbar[i] = HotbarSlot{}
		}
	}
}

// LevelOf 返回已学等级, 0 表示没学。
func (l Learned) LevelOf(id SkillID) int32 {
	if l == nil {
		return 0
	}
	return l[id]
}

// Clone 做一份独立副本。存档时用 —— 交出去之后场景还会继续改。
func (l Learned) Clone() Learned {
	if l == nil {
		return nil
	}
	out := make(Learned, len(l))
	for k, v := range l {
		out[k] = v
	}
	return out
}

// HealAmount 算这次治疗恢复多少。
//
// 「目标恢复最大生命的10%+100点当前生命」= 上限的 10% 再加 100 点。
// 百分比部分按**目标的**上限算, 不是施法者的 —— 奶妈给坦克加血时这个差别很大。
func (e SkillEffect) HealAmount(targetMaxHP int32) int32 {
	if e.Kind != EffectHeal {
		return 0
	}
	n := targetMaxHP*e.HealPctOfMax/100 + e.HealFlat
	if n < 1 {
		return 1
	}
	return n
}
