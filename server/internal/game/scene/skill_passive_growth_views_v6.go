package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// These are intrinsic defensive/resource benefits only. Incoming skill mana
// efficiency remains on the affected skill, not an outgoing target list here.
func (s *Scene) passiveOverflowEffectLines(def domain.SkillDef) []string {
	var lines []string
	if def.Level <= 5 || def.Kind != domain.SkillPassive {
		return lines
	}
	if def.PassiveCounter != nil {
		lines = append(lines, fmt.Sprintf("反击伤害: %d%%。", def.PassiveCounter.DamagePct))
	}
	for _, r := range def.PassiveResists {
		if r.ProgressionRestoreMP > 0 {
			lines = append(lines, fmt.Sprintf("成功抵抗%s时恢复%d点法力。", s.skillStatusName(r.Status), r.ProgressionRestoreMP))
		}
	}
	for _, m := range def.PassiveModifiers {
		if m.ProgressionOverflow <= 0 {
			continue
		}
		switch m.Kind {
		case domain.PassiveIncomingMagicHalfChance, domain.PassiveTrapHalfDamageChance:
			kind := "法术"
			if m.Kind == domain.PassiveTrapHalfDamageChance {
				kind = "陷阱"
			}
			lines = append(lines, fmt.Sprintf("减伤触发后，承受的%s伤害为原来的%s%%。", kind, skillFractionText(0, 2500, 50+int64(m.ProgressionOverflow))))
		case domain.PassiveAttackDurabilityReductionPct:
			lines = append(lines, fmt.Sprintf("装备暗器时，普通攻击伤害额外提高%d%%。", m.ProgressionOverflow))
		}
	}
	return lines
}
