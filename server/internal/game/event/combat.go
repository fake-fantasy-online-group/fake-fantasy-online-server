package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// DamageFlag 描述这一击是怎么打中的。**一次结算只产生一条 DamageDealt**,
// 命中与否、暴击与否都是这条事件上的标志, 不是三种不同的事件 ——
// 因为对客户端来说它们是同一个飘字位置的三种表现。
type DamageFlag uint8

const (
	DamageNormal  DamageFlag = 0
	DamageMiss    DamageFlag = 1 << iota // 未命中。此时 Amount 必为 0
	DamageCrit                           // 暴击。伤害已经乘过 2, 不要在协议层再乘
	DamageFatal                          // 致命一击, 目标 HP 归零
	DamageMagic                          // 法术伤害(不判命中, 见 docs/战斗公式.md)
	DamageOpening                        // 交战的第一击。仅供服务端交战跟踪；客户端线上没有“首击”表现位
)

func (f DamageFlag) Has(x DamageFlag) bool { return f&x != 0 }

// DamageDealt 一次伤害结算的结果。
//
// 数值口径见 docs/战斗公式.md:
//
//	命中率 = 攻方命中 / (攻方命中 + 守方防御)     ← 本作的"防御"走闪避, 不减伤
//	伤害   = uniform(攻击下限, 攻击上限) − 守方伤害抗性
//
// Amount 是**结算完的最终值**(已扣抗性、已乘暴击倍率), 协议层不做任何再计算。
type DamageDealt struct {
	Src, Dst domain.EntityID
	Amount   int32
	Flag     DamageFlag
	DstHP    int32 // 结算后目标剩余 HP, 客户端拿它更新血条
	SkillID  int32 // 0 = 普通攻击
}

// Subject 取受击方 —— 广播范围以"挨打的人在哪"为准。攻击者可能在视野外
// (远程/法术), 但飘字必须出现在被打的人头上。
func (e DamageDealt) Subject() domain.EntityID { return e.Dst }

// HealDone 治疗结算。与伤害分开是因为客户端飘字颜色/音效不同,
// 用负数伤害表达会让协议层多一个符号判断。
type HealDone struct {
	Src, Dst domain.EntityID
	Amount   int32
	DstHP    int32
	SkillID  int32
	DstKind  domain.EntityKind
	DstOwner domain.EntityID // 仅宠物有主人；用于把治疗数字投到主人的宠物锚点
}

func (e HealDone) Subject() domain.EntityID { return e.Dst }

// SkillCastPhase 是客户端需要同步的技能表现阶段。
type SkillCastPhase uint8

const (
	// 已有客户端恢复结果闭合了 CombatWorld.PlayerCast 的 phase guard：
	// wire phase=1 才调用 PlayCast 并创建技能特效；0 是吟唱/准备阶段。
	SkillCastChant SkillCastPhase = iota
	SkillCastRelease
)

// SkillCastChanged 同步一次技能的施法表现。技能数值结算仍由 DamageDealt、
// HealDone 和 StatusApplied 表达；这条事件只让施法者与旁观者播放技能动画/特效。
type SkillCastChanged struct {
	Caster      domain.EntityID
	Skill       domain.SkillID
	Phase       SkillCastPhase
	ChantHoldMS int32
	CooldownMS  int32
	Target      domain.EntityID
	// Aim 是范围/落点技能的表现中心。0x800e 通过 hasAim+aimX+aimY 把它交给
	// SkillCaster.Cast；数值结算仍只认场景在收包时解析出的权威目标。
	Aim *domain.Pos
}

func (e SkillCastChanged) Subject() domain.EntityID { return e.Caster }

// SkillHitEffect 在一个真实命中的实体位置播放技能自己的受击特效。
// 它与 SkillCastChanged 的施法者/范围中心特效是两条独立表现链。
type SkillHitEffect struct {
	Target domain.EntityID
	Effect string
}

func (e SkillHitEffect) Subject() domain.EntityID { return e.Target }

// EntityDied 实体死亡。与 EntityDespawned(DespawnDeath) 的区别:
// 死亡是**战斗结果**(要结算经验/掉落/仇恨), 消失是**视野事件**(只影响渲染)。
// 一次死亡通常先发这条, 尸体停留若干帧后才发消失。
type EntityDied struct {
	ID     domain.EntityID
	Kind   domain.EntityKind
	Killer domain.EntityID // 0 = 非战斗死亡(跌落/剧本)
	// 玩家死亡面板的额外复活入口。默认复活不受这两项影响；当前 MVP
	// 没有复活道具/复活技能，所以生产事件保持 false。
	AllowItemRevive  bool
	AllowSkillRevive bool
}

func (e EntityDied) Subject() domain.EntityID { return e.ID }

// PlayerRevived 玩家重新站起。只用于玩家；怪物重生是一个全新的实体出场。
type PlayerRevived struct{ Who domain.EntityID }

func (e PlayerRevived) Subject() domain.EntityID { return e.Who }

// ExpGained 获得经验。只发给当事人, 不广播。
//
// 数值口径: 经验跟**血量**走不跟等级走(经验 ≈ 0.052 × 血量^1.30, R²=0.956;
// 二元回归里血量偏系数 0.926、等级只有 0.633), 见 docs/怪物数值-定案.md。
type ExpGained struct {
	Who   domain.EntityID
	Delta int64
	Total int64
}

func (e ExpGained) Subject() domain.EntityID { return e.Who }

// LevelUp 升级。广播(周围人要看到升级特效), 但属性明细只发给当事人。
type LevelUp struct {
	Who   domain.EntityID
	Level int32
}

func (e LevelUp) Subject() domain.EntityID { return e.Who }
