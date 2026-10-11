package domain

import "fmt"

type UniqueContext struct {
	HPPercent, MPPercent, TargetHPPercent                                     int32
	Magic, Critical, TargetElite, Invisible, Trap, Area, Skill, Healing, Ally bool
	HurtType                                                                  int32
}
type UniqueEffects struct {
	DamageBP, MitigationBP, HealingBP                                      int32
	ManaCostReductionBP, CastReductionBP, AreaExpansionBP, TrapExpansionBP int32
	KillHealBP, KillManaBP                                                 int32
}

var uniqueTraitNames = map[string]string{
	"healthy_assault": "盈生破阵", "desperate_assault": "绝境回锋", "execution": "断劫", "ambush": "初锋", "elite_hunter": "诛邪", "critical_focus": "会心问道",
	"physical_guard": "金身", "magic_guard": "清心", "desperate_guard": "危身护命", "healthy_guard": "圆满护体",
	"healing_grace": "济世", "rescue_heal": "救急", "selfless_heal": "渡人", "healing_cost": "悬壶归元",
	"spell_economy": "节炁", "swift_cast": "瞬念", "area_expansion": "广法", "trap_expansion": "罗网",
	"fire_mastery": "红莲", "ice_mastery": "寒山", "holy_mastery": "净光", "single_focus": "一心", "area_focus": "八相", "invisible_focus": "无痕",
	"kill_mend": "还丹", "kill_mana": "纳炁", "basic_focus": "砺锋", "skill_focus": "通玄",
}

func UniqueTraitName(kind string) string        { return uniqueTraitNames[kind] }
func SupportedUniqueTrait(kind string) bool     { return uniqueTraitNames[kind] != "" }
func scaledUniquePower(value, roll int32) int32 { return int32(int64(value) * int64(roll) / 10000) }

// Only the strongest active instance of a named power counts. Different powers
// add within capped channels, avoiding multiplicative/per-item proc loops.
func EvaluateUniqueTraits(traits []UniqueTrait, c UniqueContext) UniqueEffects {
	active := make(map[string]int32)
	for _, t := range traits {
		ok := false
		switch t.Kind {
		case "healthy_assault", "healthy_guard":
			ok = c.HPPercent >= t.ThresholdPct
		case "desperate_assault", "desperate_guard":
			ok = c.HPPercent <= t.ThresholdPct
		case "execution":
			ok = c.TargetHPPercent <= t.ThresholdPct
		case "ambush":
			ok = c.TargetHPPercent >= t.ThresholdPct
		case "elite_hunter":
			ok = c.TargetElite
		case "critical_focus":
			ok = c.Critical
		case "physical_guard":
			ok = !c.Magic
		case "magic_guard":
			ok = c.Magic
		case "healing_grace", "healing_cost":
			ok = c.Healing
		case "rescue_heal":
			ok = c.Healing && c.TargetHPPercent <= t.ThresholdPct
		case "selfless_heal":
			ok = c.Healing && c.Ally
		case "spell_economy":
			ok = c.Skill && c.Magic
		case "swift_cast", "skill_focus":
			ok = c.Skill
		case "area_expansion", "area_focus":
			ok = c.Skill && c.Area
		case "trap_expansion":
			ok = c.Skill && c.Trap
		case "fire_mastery":
			ok = c.Magic && c.HurtType == HurtFire
		case "ice_mastery":
			ok = c.Magic && c.HurtType == HurtIce
		case "holy_mastery":
			ok = c.Magic && c.HurtType == HurtHoly
		case "single_focus":
			ok = c.Skill && !c.Area && !c.Trap
		case "invisible_focus":
			ok = c.Invisible
		case "kill_mend", "kill_mana":
			ok = true
		case "basic_focus":
			ok = !c.Skill
		}
		if ok && t.PowerBP > active[t.Kind] {
			active[t.Kind] = t.PowerBP
		}
	}
	var out UniqueEffects
	for kind, power := range active {
		switch kind {
		case "physical_guard", "magic_guard", "healthy_guard", "desperate_guard":
			out.MitigationBP += power
		case "healing_grace", "rescue_heal", "selfless_heal":
			out.HealingBP += power
		case "healing_cost", "spell_economy":
			out.ManaCostReductionBP += power
		case "swift_cast":
			out.CastReductionBP += power
		case "area_expansion":
			out.AreaExpansionBP += power
		case "trap_expansion":
			out.TrapExpansionBP += power
		case "kill_mend":
			out.KillHealBP += power
		case "kill_mana":
			out.KillManaBP += power
		default:
			out.DamageBP += power
		}
	}
	out.DamageBP = min(out.DamageBP, 10000)
	out.MitigationBP = min(out.MitigationBP, 4500)
	out.HealingBP = min(out.HealingBP, 10000)
	out.ManaCostReductionBP = min(out.ManaCostReductionBP, 5000)
	out.CastReductionBP = min(out.CastReductionBP, 5000)
	out.AreaExpansionBP = min(out.AreaExpansionBP, 5000)
	out.TrapExpansionBP = min(out.TrapExpansionBP, 5000)
	out.KillHealBP = min(out.KillHealBP, 800)
	out.KillManaBP = min(out.KillManaBP, 800)
	return out
}
func ActiveUniqueTraits(worn *EquipSet, defs func(ItemID) (ItemDef, bool), ch *Character) []UniqueTrait {
	if defs == nil || ch == nil {
		return nil
	}
	var out []UniqueTrait
	worn.Each(func(slot EquipSlot, st Stack) {
		def, ok := defs(st.Item)
		if !ok || def.Equip == nil || int32(slot) != def.Equip.Slot || !EquipmentFunctional(st, def) || !def.Equip.Need.AllowsCharacter(ch) {
			return
		}
		d, ok := UniqueOf(st)
		if !ok {
			return
		}
		for _, tr := range d.Traits {
			tr.PowerBP = scaledUniquePower(tr.PowerBP, st.Endgame.UniqueRollBP)
			out = append(out, tr)
		}
	})
	return out
}

// ApplyUniqueBP adds a basis-point change with integer saturation.
func ApplyUniqueBP(value, bp int32) int32 {
	if value <= 0 {
		return value
	}
	n := int64(value) * (10000 + int64(bp)) / 10000
	if n > 2147483647 {
		return 2147483647
	}
	if n < 0 {
		return 0
	}
	return int32(n)
}
func (t UniqueTrait) Describe(rollBP int32) string {
	p := float64(scaledUniquePower(t.PowerBP, rollBP)) / 100
	condition, effect := "", "伤害提高"
	switch t.Kind {
	case "healthy_assault", "healthy_guard":
		condition = fmt.Sprintf("自身生命不低于%d%%时，", t.ThresholdPct)
	case "desperate_assault", "desperate_guard":
		condition = fmt.Sprintf("自身生命不高于%d%%时，", t.ThresholdPct)
	case "execution":
		condition = fmt.Sprintf("目标生命不高于%d%%时，", t.ThresholdPct)
	case "ambush":
		condition = fmt.Sprintf("目标生命不低于%d%%时，", t.ThresholdPct)
	case "elite_hunter":
		condition = "对精英与首领，"
	case "critical_focus":
		condition = "暴击时，"
	case "physical_guard":
		condition = "受到物理攻击时，"
	case "magic_guard":
		condition = "受到法术攻击时，"
	case "healing_grace":
		effect = "治疗提高"
	case "rescue_heal":
		condition = fmt.Sprintf("目标生命不高于%d%%时，", t.ThresholdPct)
		effect = "治疗提高"
	case "selfless_heal":
		condition = "治疗他人时，"
		effect = "治疗提高"
	case "healing_cost":
		effect = "治疗技能法力消耗降低"
	case "spell_economy":
		effect = "法术技能法力消耗降低"
	case "swift_cast":
		effect = "技能吟唱时间缩短"
	case "area_expansion":
		effect = "范围技能尺寸扩大"
	case "trap_expansion":
		effect = "陷阱触发半径扩大"
	case "fire_mastery":
		effect = "火系伤害提高"
	case "ice_mastery":
		effect = "冰系伤害提高"
	case "holy_mastery":
		effect = "光系伤害提高"
	case "single_focus":
		effect = "单体技能伤害提高"
	case "area_focus":
		effect = "范围技能伤害提高"
	case "invisible_focus":
		condition = "隐身起手时，"
	case "kill_mend":
		effect = "击杀怪物恢复最大生命的"
	case "kill_mana":
		effect = "击杀怪物恢复最大法力的"
	case "basic_focus":
		effect = "普通攻击伤害提高"
	case "skill_focus":
		effect = "技能伤害提高"
	}
	switch t.Kind {
	case "physical_guard", "magic_guard", "healthy_guard", "desperate_guard":
		effect = "受到伤害降低"
	}
	return fmt.Sprintf("%s：%s%s%.2f%%", UniqueTraitName(t.Kind), condition, effect, p)
}
