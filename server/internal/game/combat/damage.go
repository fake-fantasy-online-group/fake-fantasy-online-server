package combat

// 伤害计算。三条铁律，都来自 docs/战斗公式.md 第一节第 2 步：
//
//	① 均匀分布      基础伤害 = uniform(攻击下限, 攻击上限)
//	② 暴击固定 2 倍  没有等级差修正
//	③ **防御不出现在这一步**，一点都不减 —— 这是本作与仙境传说最大的不同

// RollPhysical 掷一次物理伤害。
//
//	基础伤害 = uniform(攻击下限, 攻击上限)     ← 闭区间, 均匀
//	命中伤害 = max(1, 基础伤害 − 守方伤害抗性)
//
// 伤害抗性是**固定值减法**（1 点 = 减 1 点物理伤害）。**[单源]**
// 60 级以下只有两个来源：装备附加 attr69（23 件）、卡牌「石肤」（1~14，只能镶衣服）。
//
// 浮动来自**武器**不来自属性 —— 抓包印证：怪没武器所以打我恒为 4，
// 我有小剑(1~5)所以打它是 4~7。
func RollPhysical(minAtk, maxAtk, resist int32, rng Rand) int32 {
	return RollPhysicalSkill(minAtk, maxAtk, resist, 100, rng)
}

// RollPhysicalSkill 与 RollPhysical 相同, 但先乘技能倍率。
//
// **倍率乘在扣抗性之前**: 「造成180%伤害」说的是这一击的攻击力翻 1.8 倍,
// 抗性是对方挡下来的固定值, 挡的是最终那一下。反过来算会让抗性也被放大。
func RollPhysicalSkill(minAtk, maxAtk, resist, skillPct int32, rng Rand) int32 {
	if maxAtk < minAtk {
		minAtk, maxAtk = maxAtk, minAtk // 配错了也别崩, 按区间理解
	}
	if skillPct <= 0 {
		skillPct = 100
	}
	base := minAtk
	if span := maxAtk - minAtk; span > 0 && rng != nil {
		base += int32(rng.Intn(int(span) + 1)) // +1: 上限要能取到
	}
	return floor(int32(int64(base)*int64(skillPct)/100) - resist)
}

// MagicMitigation 返回旧比例模型的减伤率，仅保留给历史数值诊断调用。
// Deprecated: 正式战斗走 RollMagicFixedDefense。
//
//	减伤比例 = 守方魔防 / (攻方等级 × 10 + 守方魔防)
//
// 魔法侧的两条减伤线是**独立**的，别混：
//
//	魔法防御 → 百分比减伤，上限 75%（精神÷2 + 装备 mdef + 卡牌「天护」）
//	魔法抗性 → 固定值减法，1 点减 1 点（装备 attr70 + 卡牌「抗魔」）
//
// **[实证·反证]** 贴吧 8067411843 的作者原本以为「魔防类似物理防御, 是闪避」，
// 实测后发现大错特错 —— 这既证明魔防是减伤，也再次印证物理防御确实是闪避。
func MagicMitigation(atkLevel, mdef int32) float64 {
	if mdef <= 0 {
		return 0
	}
	if atkLevel <= 0 {
		atkLevel = 1 // 等级 0 会让分母塌成 mdef, 减伤直接顶到上限
	}
	r := float64(mdef) / (float64(atkLevel)*magicDefLevelFactor + float64(mdef))
	if r > MaxMagicMitigation {
		return MaxMagicMitigation
	}
	return r
}

// RollMagic 保留旧比例模型，仅供历史数值诊断调用。
// Deprecated: 正式战斗走 RollMagicFixedDefense。
//
//	魔法伤害 = f(魔攻, 技能倍率) × (1 − 减伤比例) − 守方魔法抗性
//
// ⚠️ **f(魔攻, 技能倍率) 的确切形式还没定**（docs/战斗公式.md 第五节第 3 条）。
// 这里按最直白的读法实现成 `魔攻 × 倍率/100` —— 技能倍率在 ov_skilldesc 里
// 写作「130%伤害」这种形式，字面意思就是乘。**抓到实测数据前这是个占位，不是结论。**
//
// 魔法不掷区间：法系伤害没有「武器上下限」那回事，浮动只能来自技能本身。
func RollMagic(matk, skillPct, atkLevel, mdef, mresist int32) int32 {
	if skillPct <= 0 {
		skillPct = 100 // 没写倍率就是 100%
	}
	base := float64(matk) * float64(skillPct) / 100
	after := base * (1 - MagicMitigation(atkLevel, mdef))
	return floor(int32(after) - mresist)
}

// RollMagicFixedDefense 按当前规则计算魔法伤害。
//
//	伤害 = floor(魔攻 × 技能倍率 / 100 − 魔防 × 0.63) − 魔法抗性
//
// 魔防与魔法抗性都是固定减伤，但换算率不同；最终命中伤害至少为 1。
func RollMagicFixedDefense(matk, skillPct, mdef, mresist int32) int32 {
	if skillPct <= 0 {
		skillPct = 100
	}
	damageHundredths := int64(matk)*int64(skillPct) -
		int64(mdef)*MagicDefenseReductionNumerator
	damage := damageHundredths / MagicDefenseReductionDenominator
	return floor(int32(damage) - mresist)
}

// Crit 掷暴击。rate 是**万分比**（attr23 的口径，10000 = 100%）。
func Crit(rate int32, rng Rand) bool {
	if rate <= 0 || rng == nil {
		return false
	}
	if rate >= critScale {
		return true
	}
	return int32(rng.Intn(critScale)) < rate
}

// floor 把伤害压到至少 1 点。
func floor(d int32) int32 {
	if d < MinDamage {
		return MinDamage
	}
	return d
}
