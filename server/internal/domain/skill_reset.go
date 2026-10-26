package domain

import "fmt"

// ResetAllocatedSkills refunds only permanent ranks proven by the V5 budget.
// Equipment grants and non-budget skills (including life skills) are untouched.
// No level-derived points are minted: the existing balance and removed hard
// ranks must conserve the already validated budget before and after the reset.
func ResetAllocatedSkills(ch *Character, catalog *SkillProgressionCatalog) (map[SkillID]bool, int32, error) {
	if ch == nil || !ch.SkillPointsKnown || ch.SkillAllocationVersion != SkillFormulaVersion {
		return nil, 0, fmt.Errorf("技能存档尚未完成迁移，未消耗遗忘水晶")
	}
	if err := ValidateSkillAllocation(ch.Level, int32(ch.Race)+1, ch.Skills, ch.SkillPoints, catalog); err != nil {
		return nil, 0, fmt.Errorf("技能点记录校验失败，未消耗遗忘水晶")
	}
	removed := map[SkillID]bool{}
	var refund int32
	for id, rank := range ch.Skills {
		_, budgeted := catalog.Identity(id)
		if !budgeted && id != SkillBeginnerBolt && id != SkillBeginnerGuard {
			continue
		}
		if rank > 0 {
			removed[id] = true
			refund += rank
		}
	}
	if refund == 0 {
		return nil, 0, fmt.Errorf("尚无已分配的技能点，无需使用遗忘水晶")
	}
	next := ch.Clone()
	for id := range removed {
		delete(next.Skills, id)
	}
	next.SkillPoints += refund
	for i, slot := range next.Hotbar {
		if slot.Kind == HotbarKindSkill && removed[SkillID(slot.ID)] {
			next.Hotbar[i] = HotbarSlot{}
		}
	}
	if err := ValidateSkillAllocation(next.Level, int32(next.Race)+1, next.Skills, next.SkillPoints, catalog); err != nil {
		return nil, 0, fmt.Errorf("重置后技能点校验失败，未消耗遗忘水晶")
	}
	*ch = *next
	return removed, refund, nil
}
