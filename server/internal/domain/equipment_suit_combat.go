package domain

// Server-owned card attributes; imported native IDs remain unchanged. Values
// are basis points in absolute mode, never raw percentages or attack points.
const (
	AttrSuitPhysicalSkillBP int32 = 1000101 + iota
	AttrSuitTrapDamageBP
	AttrSuitPhysicalMitigationBP
	AttrSuitPhysicalCritDamageBP
	AttrSuitHealingBP
	AttrSuitFireDamageBP
	AttrSuitIceDamageBP
	AttrSuitHolyDamageBP
	AttrSuitDarkDamageBP
	AttrSuitFireMitigationBP
	AttrSuitIceMitigationBP
	AttrSuitHolyMitigationBP
	AttrSuitDarkMitigationBP
)

type SuitCombatBonuses struct {
	PhysicalSkillBP, TrapDamageBP, PhysicalMitigationBP, PhysicalCritDamageBP, HealingBP int32
	DamageBP, MitigationBP                                                               [4]int32
}

func SuitCombatAffixName(attr int32) string {
	switch attr {
	case AttrSuitPhysicalSkillBP:
		return "物理技能伤害"
	case AttrSuitTrapDamageBP:
		return "陷阱伤害"
	case AttrSuitPhysicalMitigationBP:
		return "物理伤害减免"
	case AttrSuitPhysicalCritDamageBP:
		return "物理暴击伤害"
	case AttrSuitHealingBP:
		return "治疗量"
	case AttrSuitFireDamageBP:
		return "火系伤害"
	case AttrSuitIceDamageBP:
		return "冰系伤害"
	case AttrSuitHolyDamageBP:
		return "圣系伤害"
	case AttrSuitDarkDamageBP:
		return "暗系伤害"
	case AttrSuitFireMitigationBP:
		return "火系伤害减免"
	case AttrSuitIceMitigationBP:
		return "冰系伤害减免"
	case AttrSuitHolyMitigationBP:
		return "圣系伤害减免"
	case AttrSuitDarkMitigationBP:
		return "暗系伤害减免"
	}
	return ""
}
