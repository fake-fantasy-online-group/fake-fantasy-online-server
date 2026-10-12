package domain

import (
	"fmt"
	"math"
	"sort"
)

// The V5 catalogue is intentionally explicit. A skill ID being inside a class's
// numeric range does not make it eligible (11005 is a profession/life skill).
// These are authored gameplay proposals, not claims about the legacy server.
// Original effects, ordering, targeting, equipment conditions and status IDs are
// retained. Bounded legacy quantities interpolate from original rank 1 to 5 at
// new ranks 1 to 20, then hold their effect boundary. Every entry additionally
// names an actually consumed, non-saturating numeric carrier.
type v5Carrier uint8

const (
	v5Damage v5Carrier = iota + 1
	v5ControlDamage
	v5TargetDamage
	v5TargetResource
	v5PassiveStat
	v5StatusStat
	v5TargetStatusStat
	v5Shield
	v5Taunt
	v5Heal
	v5RestoreMP
	v5PeriodicHP
	v5StatusShield
	v5StatusReflect
	v5NextAttack
)

type v5AuthoredSkillCurve struct {
	name, note                    string
	carrier                       v5Carrier
	attr, equip                   int32
	targets                       []SkillID
	effect                        EffectKind
	status                        StatusID
	hurt                          int32
	damage1, damage20, damageStep int32
	flat1, flat20, flatStep       int32
	pct1, pct20                   int32
}

var v5SkillCurves = map[SkillID]v5AuthoredSkillCurve{
	10001: {carrier: v5TargetDamage, targets: []SkillID{10002, 10003, 10004, 10005, 10006, 10007, 10009, 10013, 10015, 10017, 10021, 10022, 10023, 10027}, flatStep: 1, name: "枪法修炼", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10002: {carrier: v5Damage, damage1: 130, damage20: 150, damageStep: 1, flatStep: 1, name: "连环枪法", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10003: {carrier: v5Damage, damage1: 180, damage20: 220, damageStep: 2, flatStep: 1, name: "裂空之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10004: {carrier: v5Damage, damage1: 200, damage20: 240, damageStep: 2, flatStep: 1, name: "荆棘之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10005: {carrier: v5Damage, damage1: 95, damage20: 115, damageStep: 1, flatStep: 1, name: "穿刺之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10006: {carrier: v5Damage, damage1: 130, damage20: 150, damageStep: 1, flatStep: 1, name: "震地之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10007: {carrier: v5Damage, damage1: 175, damage20: 215, damageStep: 2, flatStep: 1, name: "死翔之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10008: {carrier: v5PassiveStat, attr: AttrMaxHP, equip: 0, flatStep: 2, name: "意志", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10009: {carrier: v5Damage, damage1: 100, damage20: 100, damageStep: 1, flatStep: 1, name: "破防之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10010: {carrier: v5StatusStat, attr: AttrAtk, flatStep: 1, name: "狂暴术", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10011: {carrier: v5NextAttack, attr: AttrAtk, flatStep: 2, name: "怒火杀意", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10012: {carrier: v5TargetDamage, targets: []SkillID{10009}, flatStep: 1, name: "破甲", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10013: {carrier: v5Damage, damage1: 100, damage20: 100, damageStep: 1, flatStep: 1, name: "破法之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10014: {carrier: v5TargetResource, targets: []SkillID{10013}, effect: EffectDamageMP, flatStep: 1, name: "破魔", note: "保留原攻击力转资源比例；为同一目标对应资源段增加每级+1固定量。"},
	10015: {carrier: v5Damage, damage1: 165, damage20: 181, damageStep: 1, flatStep: 1, name: "心眼之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10016: {carrier: v5ControlDamage, hurt: HurtPhysical, flatStep: 1, name: "石化之枪", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10017: {carrier: v5Damage, damage1: 105, damage20: 125, damageStep: 1, flatStep: 1, name: "飞翔之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10018: {carrier: v5StatusStat, attr: AttrDef, flatStep: 1, name: "冲锋", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。 移速保持安全边界；附带固定防御代表疾行中的闪避。"},
	10019: {carrier: v5TargetDamage, targets: []SkillID{10017}, flatStep: 1, name: "全力投掷", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10020: {carrier: v5TargetDamage, targets: []SkillID{10006}, flatStep: 1, name: "震空枪气", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10021: {carrier: v5Damage, damage1: 145, damage20: 165, damageStep: 1, flatStep: 1, name: "流虹之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10022: {carrier: v5Damage, damage1: 155, damage20: 175, damageStep: 1, flatStep: 1, name: "野战八荒", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10023: {carrier: v5Damage, damage1: 165, damage20: 185, damageStep: 1, flatStep: 1, name: "战神之枪", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10024: {carrier: v5TargetDamage, targets: []SkillID{10003}, flatStep: 1, name: "维护", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10025: {carrier: v5PassiveStat, attr: AttrMaxHP, equip: 0, flatStep: 2, name: "强健", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10026: {carrier: v5PassiveStat, attr: AttrMDef, equip: 0, flatStep: 1, name: "抗魔", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10027: {carrier: v5Damage, damage1: 100, damage20: 100, damageStep: 1, flatStep: 1, name: "重击", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10028: {carrier: v5Taunt, flatStep: 1, name: "战吼", note: "保留范围；真正参与仇恨覆盖优先级的强度直接等于有效等级。"},
	10029: {carrier: v5ControlDamage, hurt: HurtPhysical, flatStep: 1, name: "震慑", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10030: {carrier: v5Shield, flatStep: 2, name: "缴械", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10101: {carrier: v5TargetDamage, targets: []SkillID{10102, 10103, 10104, 10111, 10113, 10116, 10117, 10119, 10121, 10122, 10123, 10125, 10126}, flatStep: 1, name: "剑术修炼", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10102: {carrier: v5Damage, damage1: 130, damage20: 142, damageStep: 1, flatStep: 1, name: "罡气剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10103: {carrier: v5Damage, damage1: 145, damage20: 145, damageStep: 1, flatStep: 1, name: "幻影剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10104: {carrier: v5Damage, damage1: 150, damage20: 170, damageStep: 1, flatStep: 1, name: "饮血剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10105: {carrier: v5Damage, damage1: 100, damage20: 100, damageStep: 0, flatStep: 1, name: "龙杀剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10106: {carrier: v5PassiveStat, attr: AttrDef, equip: 105, flatStep: 1, name: "盾防修炼", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10107: {carrier: v5StatusStat, attr: AttrDef, flatStep: 1, name: "铁城神盾", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10108: {carrier: v5PassiveStat, attr: AttrMaxHP, equip: 0, flatStep: 2, name: "心镜", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10109: {carrier: v5StatusStat, attr: AttrDef, flatStep: 1, name: "仙风云体", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10110: {carrier: v5TargetDamage, targets: []SkillID{10102}, flatStep: 1, name: "通背", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10111: {carrier: v5Damage, damage1: 145, damage20: 145, damageStep: 1, flatStep: 1, name: "金缚剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10112: {carrier: v5TargetResource, targets: []SkillID{10104}, effect: EffectRestoreHP, flatStep: 1, name: "血影", note: "保留原攻击力转资源比例；为同一目标对应资源段增加每级+1固定量。"},
	10113: {carrier: v5Damage, damage1: 155, damage20: 155, damageStep: 1, flatStep: 1, name: "封法剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10114: {carrier: v5Shield, flatStep: 2, name: "锁灵剑", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10115: {carrier: v5StatusStat, attr: AttrAtk, flatStep: 1, name: "火字诀", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10116: {carrier: v5Damage, damage1: 175, damage20: 215, damageStep: 2, flatStep: 1, name: "狮心剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10117: {carrier: v5Damage, damage1: 95, damage20: 115, damageStep: 1, flatStep: 1, name: "真空剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10118: {carrier: v5TargetDamage, targets: []SkillID{10117}, flatStep: 1, name: "风斩", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10119: {carrier: v5Damage, damage1: 150, damage20: 170, damageStep: 1, flatStep: 1, name: "噬魂剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10120: {carrier: v5TargetResource, targets: []SkillID{10119}, effect: EffectRestoreMP, flatStep: 1, name: "魔光", note: "保留原攻击力转资源比例；为同一目标对应资源段增加每级+1固定量。"},
	10121: {carrier: v5Damage, damage1: 130, damage20: 150, damageStep: 1, flatStep: 1, name: "旋风剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10122: {carrier: v5Damage, damage1: 160, damage20: 160, damageStep: 1, flatStep: 1, name: "混沌剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10123: {carrier: v5Damage, damage1: 165, damage20: 205, damageStep: 2, flatStep: 1, name: "陨星剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10124: {carrier: v5StatusStat, attr: AttrDef, flatStep: 1, name: "山字诀", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10125: {carrier: v5Damage, damage1: 150, damage20: 190, damageStep: 2, flatStep: 1, name: "血雨剑", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10126: {carrier: v5Damage, damage1: 100, damage20: 100, damageStep: 1, flatStep: 1, name: "强击", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10127: {carrier: v5Taunt, flatStep: 1, name: "挑衅", note: "保留范围；真正参与仇恨覆盖优先级的强度直接等于有效等级。"},
	10128: {carrier: v5ControlDamage, hurt: HurtPhysical, flatStep: 1, name: "煞气", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10129: {carrier: v5Shield, flatStep: 2, name: "致残", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。 修正旧五阶攻击上限0→4的倒退：攻击上限维持0，持续5→9秒。"},
	10130: {carrier: v5NextAttack, attr: AttrAtk, flatStep: 2, name: "雷霆之怒", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10201: {carrier: v5TargetDamage, targets: []SkillID{10209}, flatStep: 1, name: "进阶背刺", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10202: {carrier: v5PassiveStat, attr: AttrAtk, equip: 8, flatStep: 1, name: "双刃修炼", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10203: {carrier: v5PassiveStat, attr: AttrAtk, equip: 8, flatStep: 1, name: "反击术", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10204: {carrier: v5PassiveStat, attr: AttrAtk, equip: 32, flatStep: 1, name: "暗器修炼", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10205: {carrier: v5Damage, damage1: 150, damage20: 170, damageStep: 1, flatStep: 1, name: "连续投掷", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10206: {carrier: v5Damage, damage1: 145, damage20: 165, damageStep: 1, flatStep: 1, name: "多重投掷", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10207: {carrier: v5TargetStatusStat, attr: AttrAtk, targets: []SkillID{10208}, status: StatusStealth, flatStep: 1, name: "进阶隐身", note: "保留原隐身修饰；为同一隐身状态增加首击固定伤害，状态消耗前快照，不新增协同目标。"},
	10208: {carrier: v5NextAttack, attr: AttrAtk, flatStep: 1, name: "隐身术", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10209: {carrier: v5Damage, damage1: 300, damage20: 380, damageStep: 2, flatStep: 1, name: "背刺", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10210: {carrier: v5Damage, damage1: 400, damage20: 440, damageStep: 2, flatStep: 1, name: "暗杀术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10211: {carrier: v5StatusStat, attr: AttrAtk, flatStep: 1, name: "淬毒术", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10212: {carrier: v5Damage, damage1: 100, damage20: 112, damageStep: 1, flatStep: 1, name: "游龙掌", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10213: {carrier: v5Damage, damage1: 200, damage20: 240, damageStep: 2, flatStep: 1, name: "陷阱：爆弹", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10214: {carrier: v5Damage, damage1: 130, damage20: 130, damageStep: 1, flatStep: 1, name: "陷阱：毒雾", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10215: {carrier: v5Damage, damage1: 150, damage20: 150, damageStep: 1, flatStep: 1, name: "陷阱：封印", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10216: {carrier: v5Damage, damage1: 170, damage20: 200, damageStep: 1, flatStep: 1, name: "陷阱：冻气", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10217: {carrier: v5Damage, damage1: 185, damage20: 210, damageStep: 1, flatStep: 1, name: "陷阱：新星", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10218: {carrier: v5Shield, flatStep: 2, name: "解除陷阱", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10219: {carrier: v5Damage, damage1: 130, damage20: 130, damageStep: 1, flatStep: 1, name: "击晕", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10220: {carrier: v5TargetDamage, targets: []SkillID{10210}, flatStep: 1, name: "进阶暗杀", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10221: {carrier: v5Damage, damage1: 135, damage20: 160, damageStep: 1, flatStep: 1, name: "狙击", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10222: {carrier: v5PassiveStat, attr: AttrAtk, equip: 32, flatStep: 1, name: "暗器回收", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10223: {carrier: v5Damage, damage1: 160, damage20: 180, damageStep: 1, flatStep: 1, name: "雷火弹", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10224: {carrier: v5ControlDamage, hurt: HurtPhysical, flatStep: 1, name: "烟雾弹", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10225: {carrier: v5Damage, damage1: 175, damage20: 207, damageStep: 1, flatStep: 1, name: "环形爆发", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10226: {carrier: v5TargetDamage, targets: []SkillID{10225}, flatStep: 1, name: "爆发冲击", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10227: {carrier: v5Shield, flatStep: 2, name: "阴影跳跃", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10228: {carrier: v5Shield, flatStep: 2, name: "侦测陷阱", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10229: {carrier: v5PassiveStat, attr: AttrDef, equip: 0, flatStep: 1, name: "陷阱回避", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10230: {carrier: v5PassiveStat, attr: AttrMDef, equip: 0, flatStep: 1, name: "魔法回避", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10301: {carrier: v5Heal, flat1: 100, flat20: 220, flatStep: 4, pct1: 10, pct20: 30, name: "治愈术", note: "采用v5精确治疗固定量和最大生命比例锚点；超过20级只增长固定治疗。"},
	10302: {carrier: v5RestoreMP, flat1: 100, flat20: 500, flatStep: 10, name: "复法术", note: "固定法力恢复100→500；超过20级每级+10，满法力溢出不转伤害。"},
	10303: {carrier: v5PeriodicHP, flatStep: 1, name: "回春术", note: "保留周期与状态身份；每级提高每跳恢复/损失的固定生命量。"},
	10304: {carrier: v5Heal, flat1: 180, flat20: 340, flatStep: 4, pct1: 5, pct20: 25, name: "群疗术", note: "采用v5精确治疗固定量和最大生命比例锚点；超过20级只增长固定治疗。"},
	10305: {carrier: v5Shield, flatStep: 4, name: "复活术", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10306: {carrier: v5Shield, flatStep: 2, name: "净化术", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10307: {carrier: v5StatusStat, attr: AttrAtk, flatStep: 1, name: "祝福术", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10308: {carrier: v5StatusShield, flatStep: 10, name: "神圣庇护", note: "保留原护盾/冻结保护与免即死语义；在原盾基础上每级增加10点吸收。"},
	10309: {carrier: v5Shield, flatStep: 4, name: "光之奇迹", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10310: {carrier: v5ControlDamage, hurt: HurtHoly, flatStep: 1, name: "减速术", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10311: {carrier: v5StatusStat, attr: AttrDef, flatStep: 1, name: "加速术", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。 移速保持安全边界；附带固定防御代表疾行中的闪避。"},
	10312: {carrier: v5Damage, damage1: 280, damage20: 320, damageStep: 2, flatStep: 1, name: "驱魔术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10313: {carrier: v5Damage, damage1: 300, damage20: 340, damageStep: 2, flatStep: 1, name: "天罚", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10314: {carrier: v5PassiveStat, attr: AttrMAtk, equip: 0, flatStep: 1, name: "术法精研", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10315: {carrier: v5StatusStat, attr: AttrAtk, flatStep: 1, name: "瞬击术", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10316: {carrier: v5Shield, flatStep: 2, name: "清身术", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10317: {carrier: v5Damage, damage1: 240, damage20: 280, damageStep: 2, flatStep: 1, name: "圣光术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10318: {carrier: v5Shield, flatStep: 2, name: "解咒术", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10319: {carrier: v5Damage, damage1: 210, damage20: 270, damageStep: 2, flatStep: 1, name: "光弹术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10320: {carrier: v5StatusShield, flatStep: 10, name: "光盾术", note: "保留原护盾/冻结保护与免即死语义；在原盾基础上每级增加10点吸收。"},
	10321: {carrier: v5StatusStat, attr: AttrDef, flatStep: 1, name: "防御术", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。"},
	10322: {carrier: v5StatusStat, attr: AttrMaxMP, flatStep: 2, name: "神术：集中", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。 集中状态附带固定最大法力。"},
	10324: {carrier: v5ControlDamage, hurt: HurtHoly, flatStep: 1, name: "衰弱术", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10325: {carrier: v5ControlDamage, hurt: HurtHoly, flatStep: 1, name: "枷锁术", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10326: {carrier: v5Damage, damage1: 260, damage20: 280, damageStep: 1, flatStep: 1, name: "神威术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10327: {carrier: v5Shield, flatStep: 2, name: "神术：鹰眼", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10328: {carrier: v5NextAttack, attr: AttrMAtk, flatStep: 1, name: "神术：释能", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10329: {carrier: v5NextAttack, attr: AttrMAtk, flatStep: 1, name: "神术：瞬发", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10330: {carrier: v5StatusReflect, flatStep: 1, name: "针刺护体", note: "保留原反射/反弹概率与伤害类型；每级增加1点固定反伤，不触发递归反射。"},
	10401: {carrier: v5Damage, damage1: 208, damage20: 240, damageStep: 2, flatStep: 1, name: "火弹术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10402: {carrier: v5Damage, damage1: 195, damage20: 215, damageStep: 1, flatStep: 1, name: "火球术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10405: {carrier: v5Damage, damage1: 250, damage20: 274, damageStep: 1, flatStep: 1, name: "风焰术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10408: {carrier: v5Damage, damage1: 300, damage20: 360, damageStep: 2, flatStep: 1, name: "陨石术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10409: {carrier: v5Damage, damage1: 100, damage20: 120, damageStep: 1, flatStep: 1, name: "冰箭术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10410: {carrier: v5Damage, damage1: 175, damage20: 195, damageStep: 1, flatStep: 1, name: "冰爆术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10411: {carrier: v5Damage, damage1: 250, damage20: 282, damageStep: 1, flatStep: 1, name: "绝对零度", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10412: {carrier: v5Damage, damage1: 200, damage20: 232, damageStep: 1, flatStep: 1, name: "暴雪术", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10413: {carrier: v5ControlDamage, hurt: HurtIce, flatStep: 1, name: "时之封印", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10414: {carrier: v5Damage, damage1: 350, damage20: 390, damageStep: 2, flatStep: 1, name: "虚空", note: "旧首阶/五阶作为1/20级倍率参考；超过20级继续增长；同渠道固定伤害每级+1。"},
	10415: {carrier: v5PassiveStat, attr: AttrMAtk, equip: 0, flatStep: 1, name: "魔法修炼", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10416: {carrier: v5PassiveStat, attr: AttrMaxMP, equip: 0, flatStep: 2, name: "魔法精研", note: "保留原被动概率/比例；增加身份相符的固定属性，继承武器/盾牌条件。"},
	10417: {carrier: v5StatusReflect, flatStep: 1, name: "魔镜术", note: "保留原反射/反弹概率与伤害类型；每级增加1点固定反伤，不触发递归反射。"},
	10418: {carrier: v5StatusStat, attr: AttrMaxMP, flatStep: 2, name: "秘术：集中", note: "保留原状态；在同一状态持续期间增加固定属性，随状态结束移除。 集中状态附带固定最大法力。"},
	10419: {carrier: v5NextAttack, attr: AttrMAtk, flatStep: 1, name: "秘术：释能", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10420: {carrier: v5NextAttack, attr: AttrMAtk, flatStep: 1, name: "秘术：瞬发", note: "保留原状态与触发条件；下一次符合条件的攻击额外结算同渠道固定伤害，状态消耗前快照该数值，不依赖已移除的临时攻击属性。"},
	10421: {carrier: v5PeriodicHP, hurt: HurtFire, flatStep: -1, name: "燃烧", note: "保留周期与状态身份；每级提高每跳恢复/损失的固定生命量。 首次施法附带同火渠道固定伤害，允许燃烧强化在原目标上提供固定增伤。"},
	10422: {carrier: v5TargetDamage, targets: []SkillID{10421}, flatStep: 1, name: "燃烧强化", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10423: {carrier: v5TargetDamage, targets: []SkillID{10405}, flatStep: 1, name: "高热风暴", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10424: {carrier: v5TargetDamage, targets: []SkillID{10401, 10402, 10405, 10408}, flatStep: 1, name: "红莲之魂", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10425: {carrier: v5StatusReflect, flatStep: 1, name: "火盾术", note: "保留原反射/反弹概率与伤害类型；每级增加1点固定反伤，不触发递归反射。"},
	10426: {carrier: v5TargetDamage, targets: []SkillID{10401, 10402}, flatStep: 1, name: "火系掌握", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10427: {carrier: v5TargetDamage, targets: []SkillID{10409, 10410, 10412, 10411}, flatStep: 1, name: "冰系掌握", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10428: {carrier: v5ControlDamage, hurt: HurtIce, flatStep: 1, name: "冰封", note: "保留原控制、概率与目标；增加对应流派的初始固定伤害，每级+1，先伤害后控制。"},
	10429: {carrier: v5TargetDamage, targets: []SkillID{10428}, flatStep: 1, name: "冰封强化", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10430: {carrier: v5TargetDamage, targets: []SkillID{10412}, flatStep: 1, name: "淬冰风暴", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10431: {carrier: v5TargetDamage, targets: []SkillID{10409, 10410, 10411, 10412}, flatStep: 1, name: "寒冰之魂", note: "保留原定向被动与同一目标；额外固定伤害每级+1，不产生新的来源边。"},
	10432: {carrier: v5Shield, flatStep: 2, name: "秘术：幻影", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10433: {carrier: v5Shield, flatStep: 2, name: "秘术：消除", note: "保留净化/复活/位移/侦测/保护机制；成功后附带固定保护盾，概率不超过100%。"},
	10435: {carrier: v5StatusShield, flatStep: 10, name: "冰盾术", note: "保留原护盾/冻结保护与免即死语义；在原盾基础上每级增加10点吸收。"},
}

// HasV5SkillCurve is a content eligibility check, not a rank cap.
func HasV5SkillCurve(id SkillID) bool { _, ok := v5SkillCurves[id]; return ok }

// V5SkillCurveIDs returns an owned, sorted copy for catalogue validation.
func V5SkillCurveIDs() []SkillID {
	out := make([]SkillID, 0, len(v5SkillCurves))
	for id := range v5SkillCurves {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// EvaluateV5Skill evaluates the authored V5 curve against immutable legacy
// rank-1/rank-5 reference payloads. Rank is already checked for representability
// by the unbounded loadout resolver. No rank is clamped to 5, 20 or a wire limit:
// if any resulting engine field cannot be represented, this function fails.
// A caller must report this error, never fall back to a lower-rank payload.
func EvaluateV5Skill(first, fifth SkillDef, rank int32) (SkillDef, error) {
	curve, ok := v5SkillCurves[first.ID]
	if !ok {
		return SkillDef{}, fmt.Errorf("v5 curve: skill %d is not in the authored class-battle catalogue", first.ID)
	}
	if rank < 1 {
		return SkillDef{}, fmt.Errorf("v5 curve %d: rank must be positive", first.ID)
	}
	if first.ID != fifth.ID || first.Kind != fifth.Kind || first.Prof != fifth.Prof || first.HurtType != fifth.HurtType {
		return SkillDef{}, fmt.Errorf("v5 curve %d: legacy anchor identity mismatch", first.ID)
	}
	e := v5Evaluator{id: first.ID, rank: rank}
	out := e.interpolate(first, fifth)
	if e.err != nil {
		return SkillDef{}, e.err
	}
	// Stun table levels are identities, not ordinal strengths: the original
	// 震慑/煞气 route is 16,11,12,13,14 (5,6,7,8,9 seconds). Interpolating
	// 16 -> 14 would accidentally visit level15 (10 seconds), then regress.
	// Keep the five-second control identity and author duration independently.
	if (out.ID == 10029 || out.ID == 10128) && len(out.Statuses) > 0 {
		out.Statuses[0].Level = first.Statuses[0].Level
		out.Statuses[0].DurationSec = e.blend(5, 9)
	}
	out.Level = rank
	out.FormulaVersion = 5
	// Appending to the copied description does not alter source data or claim that
	// the extra effects were part of the original skill text.
	out.Description = first.Description + " [V5] " + curve.note
	delta := e.checked(int64(rank-1)*int64(curve.flatStep), "carrier")
	switch curve.carrier {
	case v5Damage:
		found := false
		for i := range out.Effects {
			fx := &out.Effects[i]
			if fx.Kind != EffectDamage {
				continue
			}
			if found {
				e.fail("multiple main damage effects require an authored per-effect curve")
				break
			}
			found = true
			fx.DamagePctNumerator, fx.DamagePctDenominator = e.curveFraction(curve.damage1, curve.damage20, curve.damageStep, "damage percentage")
			fx.DamagePct = e.checked(fx.DamagePctNumerator/fx.DamagePctDenominator, "damage percentage")
			fx.DamageFlat = e.checked(int64(fx.DamageFlat)+int64(delta), "fixed damage")
		}
		if !found {
			e.fail("damage carrier has no original damage effect")
		}
	case v5ControlDamage:
		// Initial damage resolves before applying the original control, so freezing
		// the target does not immediately break itself with a new periodic tick.
		out.HurtType = curve.hurt
		if delta > 0 {
			out.Effects = append(out.Effects, SkillEffect{Kind: EffectDamage, DamageFlat: delta, DamagePctDenominator: 1, GuaranteedHit: true})
		}
	case v5TargetDamage:
		for _, target := range curve.targets {
			out.PassiveModifiers = append(out.PassiveModifiers, PassiveModifier{Kind: PassiveSkillDamageFlat, TargetSkill: target, Value: delta})
		}
	case v5TargetResource:
		for _, target := range curve.targets {
			out.PassiveModifiers = append(out.PassiveModifiers, PassiveModifier{Kind: PassiveEffectRestoreFlat, TargetSkill: target, EffectKind: curve.effect, Value: delta})
		}
	case v5PassiveStat:
		out.PassiveStats = append(out.PassiveStats, PassiveStatModifier{Attr: curve.attr, Value: delta, Mode: ModeAbsolute, RequiredEquipType: curve.equip})
	case v5StatusStat:
		if len(out.Statuses) == 0 {
			e.fail("status stat carrier has no original status")
		} else {
			out.Statuses[0].ProgressionAffixes = append(out.Statuses[0].ProgressionAffixes, Affix{Attr: curve.attr, Value: delta, Mode: ModeAbsolute})
		}
	case v5NextAttack:
		if len(out.Statuses) == 0 {
			e.fail("next-attack carrier has no original status")
		} else {
			out.Statuses[0].ProgressionAttackFlat = delta
		}
	case v5TargetStatusStat:
		for _, target := range curve.targets {
			out.PassiveStatusAffixes = append(out.PassiveStatusAffixes, PassiveStatusAffix{TargetSkill: target, Status: curve.status, Affix: Affix{Attr: curve.attr, Value: delta, Mode: ModeAbsolute}})
		}
	case v5Shield:
		out.ProgressionShieldFlat = delta
		// Old 致残 increased the attack cap 0 -> 4 at higher ranks. Keep the
		// beneficial cap at zero while independently growing time and protection.
		if out.ID == 10129 && len(out.Statuses) > 0 {
			out.Statuses[0].Level = first.Statuses[0].Level
			out.Statuses[0].DurationSec = e.blend(5, 9)
		}
	case v5Taunt:
		found := false
		for i := range out.Effects {
			if out.Effects[i].Kind == EffectTaunt {
				out.Effects[i].TauntPower = rank
				found = true
			}
		}
		if !found {
			e.fail("taunt carrier has no original taunt effect")
		}
	case v5Heal:
		found := false
		for i := range out.Effects {
			fx := &out.Effects[i]
			if fx.Kind != EffectHeal {
				continue
			}
			found = true
			fx.HealFlatNumerator, fx.HealFlatDenominator = e.curveFraction(curve.flat1, curve.flat20, curve.flatStep, "healing flat")
			fx.HealFlat = e.checked(fx.HealFlatNumerator/fx.HealFlatDenominator, "healing flat")
			fx.HealPctNumerator, fx.HealPctDenominator = e.curveFraction(curve.pct1, curve.pct20, 0, "healing percentage")
			fx.HealPctOfMax = e.checked(fx.HealPctNumerator/fx.HealPctDenominator, "healing percentage")
		}
		if !found {
			e.fail("heal carrier has no original heal effect")
		}
	case v5RestoreMP:
		found := false
		for i := range out.Effects {
			if out.Effects[i].Kind != EffectRestoreMP {
				continue
			}
			n, d := e.curveFraction(curve.flat1, curve.flat20, curve.flatStep, "MP restore")
			out.Effects[i].RestoreMPFlat = e.checked(n/d, "MP restore")
			found = true
		}
		if !found {
			e.fail("MP carrier has no original restoration effect")
		}
	case v5PeriodicHP:
		if len(out.Statuses) == 0 {
			e.fail("periodic carrier has no original status")
		} else {
			out.Statuses[0].ProgressionHPPerTick = delta
		}
		if curve.hurt != HurtNone {
			out.HurtType = curve.hurt
			// A real initial fire payload permits the existing 燃烧强化 source to
			// enhance this same target without inventing a fourth source or global rule.
			if delta < 0 {
				out.Effects = append(out.Effects, SkillEffect{Kind: EffectDamage, DamageFlat: e.checked(-int64(delta), "initial periodic damage"), DamagePctDenominator: 1, GuaranteedHit: true})
			}
		}
	case v5StatusShield:
		if len(out.Statuses) == 0 {
			e.fail("shield carrier has no original status")
		} else {
			out.Statuses[0].ProgressionDamageShield = delta
		}
	case v5StatusReflect:
		if len(out.Statuses) == 0 {
			e.fail("reflect carrier has no original status")
		} else {
			out.Statuses[0].ProgressionReflectFlat = delta
		}
	default:
		e.fail("unknown authored carrier")
	}
	if e.err != nil {
		return SkillDef{}, e.err
	}
	if len(out.Effects) > 0 {
		out.Effect = out.Effects[0]
	} else {
		out.Effect = SkillEffect{}
	}
	return out, nil
}

type v5Evaluator struct {
	id   SkillID
	rank int32
	err  error
}

func (e *v5Evaluator) fail(reason string) {
	if e.err == nil {
		e.err = fmt.Errorf("v5 curve %d rank %d: %s", e.id, e.rank, reason)
	}
}
func (e *v5Evaluator) checked(v int64, field string) int32 {
	if v < math.MinInt32 || v > math.MaxInt32 {
		e.fail(field + " is outside engine int32 representation")
		return 0
	}
	return int32(v)
}
func (e *v5Evaluator) blend(a, b int32) int32 {
	n := e.rank - 1
	if n > 19 {
		n = 19
	}
	return e.checked(int64(a)+(int64(b)-int64(a))*int64(n)/19, "legacy anchor interpolation")
}
func (e *v5Evaluator) curveFraction(a, b, step int32, field string) (int64, int64) {
	if e.rank <= 20 {
		return int64(a)*19 + (int64(b)-int64(a))*int64(e.rank-1), 19
	}
	v := int64(b) + int64(step)*int64(e.rank-20)
	e.checked(v, field)
	return v, 1
}
func (e *v5Evaluator) bounded(a, b, lo, hi int32, field string) int32 {
	if a < lo || b < lo || a > hi || b > hi {
		e.fail(field + " anchor violates its effect bounds")
		return 0
	}
	return e.blend(a, b)
}

func (e *v5Evaluator) interpolate(a, b SkillDef) SkillDef {
	out := a
	out.Prerequisites = append([]SkillID(nil), a.Prerequisites...)
	out.Dist = e.blend(a.Dist, b.Dist)
	out.Radius = e.blend(a.Radius, b.Radius)
	out.PrepareMS = e.blend(a.PrepareMS, b.PrepareMS)
	out.CooldownMS = e.blend(a.CooldownMS, b.CooldownMS)
	out.MPCost = e.blend(a.MPCost, b.MPCost)
	if a.Area.Shape != b.Area.Shape || a.Area.Center != b.Area.Center {
		e.fail("area identity mismatch")
	}
	out.Area.Radius = e.blend(a.Area.Radius, b.Area.Radius)
	out.Area.Angle = e.blend(a.Area.Angle, b.Area.Angle)
	out.Area.Length = e.blend(a.Area.Length, b.Area.Length)
	out.Area.Width = e.blend(a.Area.Width, b.Area.Width)
	af, bf := a.DirectEffects(), b.DirectEffects()
	if len(af) != len(bf) {
		e.fail("effect anchor count mismatch")
		return out
	}
	out.Effects = make([]SkillEffect, len(af))
	for i := range af {
		out.Effects[i] = e.effect(af[i], bf[i])
	}
	if len(a.Statuses) != len(b.Statuses) || len(a.PassiveModifiers) != len(b.PassiveModifiers) || len(a.PassiveStats) != len(b.PassiveStats) || len(a.PassiveResists) != len(b.PassiveResists) || len(a.PassiveTriggers) != len(b.PassiveTriggers) || len(a.PassiveStatusAffixes) != len(b.PassiveStatusAffixes) || len(a.PassiveTriggerEffects) != len(b.PassiveTriggerEffects) {
		e.fail("status/passive anchor count mismatch")
		return out
	}
	out.Statuses = make([]StatusApplication, len(a.Statuses))
	for i, x := range a.Statuses {
		y := b.Statuses[i]
		if x.ID != y.ID {
			e.fail("status identity mismatch")
		}
		z := x
		z.Level = e.blend(x.Level, y.Level)
		z.ChanceBP = e.bounded(x.ChanceBP, y.ChanceBP, 0, 10000, "status chance")
		z.DurationSec = e.blend(x.DurationSec, y.DurationSec)
		z.StealthDamagePct = e.blend(x.StealthDamagePct, y.StealthDamagePct)
		z.PhysicalDamageTakenPct = e.bounded(x.PhysicalDamageTakenPct, y.PhysicalDamageTakenPct, 0, 100, "physical vulnerability")
		z.ExtraAffixes = e.affixes(x.ExtraAffixes, y.ExtraAffixes)
		z.ProgressionAffixes = e.affixes(x.ProgressionAffixes, y.ProgressionAffixes)
		out.Statuses[i] = z
	}
	out.PassiveModifiers = append([]PassiveModifier(nil), a.PassiveModifiers...)
	for i, x := range a.PassiveModifiers {
		y := b.PassiveModifiers[i]
		if x.Kind != y.Kind || x.TargetSkill != y.TargetSkill || x.EffectKind != y.EffectKind {
			e.fail("passive modifier identity mismatch")
		}
		out.PassiveModifiers[i].Value = e.blend(x.Value, y.Value)
	}
	out.PassiveStats = append([]PassiveStatModifier(nil), a.PassiveStats...)
	for i, x := range a.PassiveStats {
		y := b.PassiveStats[i]
		if x.Attr != y.Attr || x.Mode != y.Mode || x.RequiredEquipType != y.RequiredEquipType {
			e.fail("passive stat identity mismatch")
		}
		out.PassiveStats[i].Value = e.blend(x.Value, y.Value)
	}
	out.PassiveResists = append([]PassiveStatusResist(nil), a.PassiveResists...)
	for i, x := range a.PassiveResists {
		y := b.PassiveResists[i]
		if x.Status != y.Status {
			e.fail("passive resistance identity mismatch")
		}
		out.PassiveResists[i].Pct = e.bounded(x.Pct, y.Pct, 0, 100, "passive status resistance")
	}
	out.PassiveTriggers = append([]PassiveTriggerStatus(nil), a.PassiveTriggers...)
	for i, x := range a.PassiveTriggers {
		y := b.PassiveTriggers[i]
		if x.TargetSkill != y.TargetSkill || x.Status != y.Status {
			e.fail("passive trigger identity mismatch")
		}
		z := &out.PassiveTriggers[i]
		z.StatusLevel = e.blend(x.StatusLevel, y.StatusLevel)
		z.ChanceBP = e.bounded(x.ChanceBP, y.ChanceBP, 0, 10000, "passive trigger chance")
		z.DurationSec = e.blend(x.DurationSec, y.DurationSec)
	}
	out.PassiveStatusAffixes = append([]PassiveStatusAffix(nil), a.PassiveStatusAffixes...)
	for i, x := range a.PassiveStatusAffixes {
		y := b.PassiveStatusAffixes[i]
		if x.TargetSkill != y.TargetSkill || x.Status != y.Status || x.Affix.Attr != y.Affix.Attr || x.Affix.Mode != y.Affix.Mode {
			e.fail("passive status affix identity mismatch")
		}
		out.PassiveStatusAffixes[i].Affix.Value = e.blend(x.Affix.Value, y.Affix.Value)
	}
	out.PassiveTriggerEffects = append([]PassiveTriggerEffect(nil), a.PassiveTriggerEffects...)
	for i, x := range a.PassiveTriggerEffects {
		y := b.PassiveTriggerEffects[i]
		if x.TargetSkill != y.TargetSkill {
			e.fail("passive trigger effect target mismatch")
		}
		out.PassiveTriggerEffects[i].Effect = e.effect(x.Effect, y.Effect)
	}
	if (a.PassiveCounter == nil) != (b.PassiveCounter == nil) {
		e.fail("counter anchor presence mismatch")
	} else if a.PassiveCounter != nil {
		x, y := a.PassiveCounter, b.PassiveCounter
		z := *x
		if x.RequiredEquipType != y.RequiredEquipType {
			e.fail("counter equipment condition mismatch")
		}
		z.ChanceBP = e.bounded(x.ChanceBP, y.ChanceBP, 0, 10000, "counter chance")
		z.DamagePct = e.blend(x.DamagePct, y.DamagePct)
		out.PassiveCounter = &z
	}
	return out
}

func (e *v5Evaluator) effect(a, b SkillEffect) SkillEffect {
	if a.Kind != b.Kind || a.Target != b.Target || a.RequiresPreviousLanded != b.RequiresPreviousLanded || a.GuaranteedHit != b.GuaranteedHit || a.ClearHarmful != b.ClearHarmful {
		e.fail("direct effect identity mismatch")
	}
	z := a
	z.DamagePct = e.blend(a.DamagePct, b.DamagePct)
	z.DamageFlat = e.blend(a.DamageFlat, b.DamageFlat)
	z.HealPctOfMax = e.bounded(a.HealPctOfMax, b.HealPctOfMax, 0, 100, "healing percentage")
	z.HealFlat = e.blend(a.HealFlat, b.HealFlat)
	z.RestoreMPFlat = e.blend(a.RestoreMPFlat, b.RestoreMPFlat)
	z.SourceAttackPct = e.blend(a.SourceAttackPct, b.SourceAttackPct)
	z.MaxResourcePct = e.bounded(a.MaxResourcePct, b.MaxResourcePct, 0, 100, "maximum resource percentage")
	z.TrapRadius = e.blend(a.TrapRadius, b.TrapRadius)
	z.RevealRadius = e.blend(a.RevealRadius, b.RevealRadius)
	z.WeaponDurabilityCostPct = e.bounded(a.WeaponDurabilityCostPct, b.WeaponDurabilityCostPct, 0, 100, "durability cost")
	z.DispelChanceBP = e.bounded(a.DispelChanceBP, b.DispelChanceBP, 0, 10000, "dispel chance")
	z.TauntPower = e.blend(a.TauntPower, b.TauntPower)
	z.ChanceBP = e.bounded(a.ChanceBP, b.ChanceBP, 0, 10000, "effect chance")
	z.LossRefundPct = e.bounded(a.LossRefundPct, b.LossRefundPct, 0, 100, "death loss refund")
	z.KnockbackDistance = e.blend(a.KnockbackDistance, b.KnockbackDistance)
	z.RemoveStatusIDs = append([]StatusID(nil), a.RemoveStatusIDs...)
	if len(a.RemoveStatusIDs) != len(b.RemoveStatusIDs) {
		e.fail("dispel whitelist mismatch")
	} else {
		for i, id := range a.RemoveStatusIDs {
			if id != b.RemoveStatusIDs[i] {
				e.fail("dispel whitelist identity mismatch")
			}
		}
	}
	return z
}
func (e *v5Evaluator) affixes(a, b []Affix) []Affix {
	out := append([]Affix(nil), a...)
	if len(a) != len(b) {
		e.fail("status affix anchor count mismatch")
		return out
	}
	for i, x := range a {
		if x.Attr != b[i].Attr || x.Mode != b[i].Mode {
			e.fail("status affix identity mismatch")
		}
		out[i].Value = e.blend(x.Value, b[i].Value)
	}
	return out
}
