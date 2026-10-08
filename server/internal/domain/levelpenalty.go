package domain

// LevelPenalty 是跨级打怪的等级差惩罚参数（game_level_penalty 单行表）。
//
// 分界与客户端怪物名颜色档一致（fake-fantasy-online TargetNameColor 实测）：
//
//	diff = 怪物等级 − 玩家等级
//	diff ≥ 10   紫色：经验开始递减
//	3 ≤ diff ≤ 9 红色：满经验、满掉率
//	0 ≤ diff ≤ 2 黄色：满经验、满掉率
//	−9 ≤ diff ≤ −1 绿色：经验开始递减（掉率 |diff|>5 才减）
//	diff ≤ −10  灰色：经验继续递减到 0
//
// 定案数值（用户拍板）：经验每级 −8%（含紫色）；掉率 |diff|≤5 满、
// 之后每级 −8%、下限 50%（双向）。全部参数落库，这里是运行时的只读视图。
type LevelPenalty struct {
	// ExpFullAbove 怪比玩家高 ≤ 此级差时经验满（客户端红色档上限 9）。
	ExpFullAbove int32
	// ExpStepBP 经验每超过满区间 1 级递减的万分数（800 = 8%）。
	ExpStepBP int32
	// ExpBelowStart 怪比玩家低 ≥ 此级差时经验开始递减（1 = 低 1 级即绿档）。
	ExpBelowStart int32
	// DropFull 等级差绝对值 ≤ 此级差时掉率满（5）。
	DropFull int32
	// DropStepBP 掉率每超过满区间 1 级递减的万分数（800 = 8%）。
	DropStepBP int32
	// DropFloorBP 掉率下限万分数（5000 = 50%），到线后不再递减。
	DropFloorBP int32
}

// Enabled 报告这组参数是否可用。任何关键字段缺省都视为未启用。
func (p *LevelPenalty) Enabled() bool {
	return p != nil && p.ExpFullAbove >= 0 && p.DropFull >= 0 &&
		p.ExpStepBP >= 0 && p.ExpStepBP <= 10000 &&
		p.DropStepBP >= 0 && p.DropStepBP <= 10000 &&
		p.DropFloorBP >= 0 && p.DropFloorBP <= 10000 &&
		p.ExpBelowStart >= 1
}

// ExpMultBP 返回玩家击杀这只怪的经验倍率（万分数，10000 = 100%）。
//
// 形状（每级 −ExpStepBP）：
//
//	diff < 0（绿/灰）：从低 ExpBelowStart 级起每低 1 级 −8%，到 0 为止
//	0 ≤ diff ≤ ExpFullAbove（黄/红）：100%
//	diff > ExpFullAbove（紫）：从满区间上限起每高 1 级 −8%，到 0 为止
func (p *LevelPenalty) ExpMultBP(playerLevel, monsterLevel int32) int32 {
	if p == nil {
		return 10000
	}
	diff := monsterLevel - playerLevel
	var over int32
	switch {
	case diff > p.ExpFullAbove:
		over = diff - p.ExpFullAbove
	case diff < 0:
		over = -diff - (p.ExpBelowStart - 1)
		if over < 0 {
			over = 0
		}
	default:
		return 10000
	}
	v := 10000 - over*p.ExpStepBP
	if v < 0 {
		return 0
	}
	return v
}

// DropMultBP 返回这只怪的掉率倍率（万分数，10000 = 100%）。
//
// |diff| ≤ DropFull 满；之后每级 −DropStepBP，最低 DropFloorBP（双向）。
func (p *LevelPenalty) DropMultBP(playerLevel, monsterLevel int32) int32 {
	if p == nil {
		return 10000
	}
	diff := monsterLevel - playerLevel
	if diff < 0 {
		diff = -diff
	}
	over := diff - p.DropFull
	if over <= 0 {
		return 10000
	}
	v := 10000 - over*p.DropStepBP
	if v < p.DropFloorBP {
		return p.DropFloorBP
	}
	return v
}
