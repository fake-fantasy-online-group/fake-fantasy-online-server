package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) passiveManaEfficiencyRanks(caster *entity.Entity, def domain.SkillDef, levels domain.Learned) int32 {
	if s == nil || s.skills == nil || caster == nil || caster.Player == nil || caster.Player.Char == nil {
		return 0
	}
	// Membership follows the authored fifth-rank action, not rounded high-rank
	// milliseconds. Reaching sub-millisecond casting must not remove savings.
	chanting := def.PrepareMS > 0
	if original, ok := s.skills[domain.SkillKey{ID: def.ID, Level: 5}]; ok {
		chanting = original.PrepareMS > 0
	}
	var total int64
	for id, rank := range levels {
		p, ok := s.skills.Get(id, rank)
		if !ok || p.Kind != domain.SkillPassive || !caster.Player.Char.AllowsSkill(p.ID, p.Prof) {
			continue
		}
		for _, m := range p.PassiveModifiers {
			if m.ProgressionOverflow <= 0 {
				continue
			}
			switch m.Kind {
			case domain.PassiveCastTimeReductionPct, domain.PassiveCastInterruptReductionPct:
				if chanting && (m.TargetSkill == 0 || m.TargetSkill == def.ID) {
					total += int64(m.ProgressionOverflow)
				}
			case domain.PassiveSkillDurabilityReductionPct, domain.PassivePreserveInvisibleChance, domain.PassiveStatusPhysicalDamageTakenPct:
				if m.TargetSkill == def.ID {
					total += int64(m.ProgressionOverflow)
				}
			}
		}
		for _, m := range p.PassiveTriggers {
			if m.TargetSkill == def.ID {
				total += int64(m.ManaEfficiencyRanks)
			}
		}
		for _, m := range p.PassiveTriggerEffects {
			if m.TargetSkill == def.ID {
				total += int64(m.ManaEfficiencyRanks)
			}
		}
		for _, m := range p.PassiveStatusAffixes {
			if m.TargetSkill == def.ID {
				total += int64(m.ManaEfficiencyRanks)
			}
		}
	}
	return domain.SaturateInt32(total)
}

func (s *Scene) passiveOverflow(target *entity.Entity, kind domain.PassiveModifierKind, weaponOnly bool) int32 {
	if s == nil || s.skills == nil || target == nil || target.Player == nil || target.Player.Char == nil {
		return 0
	}
	var total int64
	for id, rank := range s.rawSkillLevels(target) {
		p, ok := s.skills.Get(id, rank)
		if !ok || p.Kind != domain.SkillPassive || !target.Player.Char.AllowsSkill(p.ID, p.Prof) {
			continue
		}
		if weaponOnly && p.RequiredWeaponType != 0 && !s.hasFunctionalEquipType(target, p.RequiredWeaponType) {
			continue
		}
		for _, m := range p.PassiveModifiers {
			if m.Kind == kind {
				total += int64(m.ProgressionOverflow)
			}
		}
	}
	return domain.SaturateInt32(total)
}

// Keep the original half-damage rounding exactly until the native chance cap.
// Beyond it the same hit category receives greater mitigation, never immunity.
func (s *Scene) passiveHalfDamage(target *entity.Entity, kind domain.PassiveModifierKind, incoming int32) int32 {
	overflow := s.passiveOverflow(target, kind, false)
	denominator := int64(2) * (50 + int64(overflow))
	return domain.SaturateInt32((int64(incoming)*50 + denominator - 1) / denominator)
}

func (s *Scene) restorePassiveResistMP(target *entity.Entity, status domain.StatusID) {
	if s == nil || s.skills == nil || target == nil || target.Player == nil || target.Player.Char == nil {
		return
	}
	var restore int64
	for id, rank := range s.rawSkillLevels(target) {
		p, ok := s.skills.Get(id, rank)
		if !ok || p.Kind != domain.SkillPassive || !target.Player.Char.AllowsSkill(p.ID, p.Prof) {
			continue
		}
		for _, r := range p.PassiveResists {
			if r.Status == status && r.Pct >= 100 {
				restore += int64(r.ProgressionRestoreMP)
			}
		}
	}
	if restore <= 0 || target.MP >= target.MaxMP {
		return
	}
	target.MP = min(target.MaxMP, domain.SaturateInt32(int64(target.MP)+restore))
	target.Player.MarkDirty()
	s.emitAttributesIfPlayer(target)
}
