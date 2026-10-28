package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Every accepted special-weapon runtime hook resolves the same registered
// source as the skill-definition adapter. Snapshot identities deliberately do
// not re-check current equipment: accepted casts and delayed effects retain it.
func (s *Scene) dispatchSpecialWeaponEffect(owner *entity.Entity, base domain.ItemID, profession int32, hook string, producer bool, fn func()) bool {
	effect, ok := domain.RuntimeSpecialWeaponEffect(base, profession, hook, producer)
	if !ok {
		return false
	}
	return s.dispatchEquipmentEffect(owner, effect, fn)
}

func (s *Scene) specialWeaponCastReleased(p *entity.Entity, d domain.SkillDef) {
	s.dispatchSpecialWeaponEffect(p, d.SpecialWeaponBaseID, d.Prof, "cast-released", true, func() { s.specialWeaponCastReleasedRuntime(p, d) })
}

func (s *Scene) specialWeaponCastFinished(p *entity.Entity, d domain.SkillDef, hits []domain.EntityID) {
	s.dispatchSpecialWeaponEffect(p, d.SpecialWeaponBaseID, d.Prof, "cast-finished", true, func() { s.specialWeaponCastFinishedRuntime(p, d, hits) })
}

func (s *Scene) specialWeaponBeforeHit(src, dst *entity.Entity, d *domain.SkillDef) {
	if d == nil {
		return
	}
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "before-hit", false, func() { s.specialWeaponBeforeHitRuntime(src, dst, d) })
}

func (s *Scene) specialWeaponAfterHit(src, dst *entity.Entity, d domain.SkillDef, landed, killed bool) {
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "after-hit", true, func() { s.specialWeaponAfterHitRuntime(src, dst, d, landed, killed) })
}

func (s *Scene) specialWeaponAfterDispel(src, dst *entity.Entity, d domain.SkillDef, removed []domain.StatusID, stolen []domain.Active) {
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "after-dispel", true, func() { s.specialWeaponAfterDispelRuntime(src, dst, d, removed, stolen) })
}

func (s *Scene) specialWeaponConvertShield(src, dst *entity.Entity, d domain.SkillDef) bool {
	var result bool
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "convert-shield", true, func() { result = s.specialWeaponConvertShieldRuntime(src, dst, d) })
	return result
}

func (s *Scene) specialWeaponAim(src *entity.Entity, d *domain.SkillDef, target domain.EntityID) {
	if d == nil {
		return
	}
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "aim", false, func() { s.specialWeaponAimRuntime(src, d, target) })
}

func (s *Scene) specialWeaponConsumeFreeze(src *entity.Entity, d domain.SkillDef) {
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "consume-freeze", true, func() { s.specialWeaponConsumeFreezeRuntime(src, d) })
}

func (s *Scene) specialWeaponImmediateTrap(src *entity.Entity, d domain.SkillDef, at domain.Pos, radius int32, duration int32, source *domain.CombatSourceSnapshotV5) bool {
	var result bool
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "instant-trap", true, func() { result = s.specialWeaponImmediateTrapRuntime(src, d, at, radius, duration, source) })
	return result
}

func (s *Scene) specialWeaponHeal(src, dst *entity.Entity, amount int32, d domain.SkillDef) {
	s.dispatchSpecialWeaponEffect(src, d.SpecialWeaponBaseID, d.Prof, "heal", true, func() { s.specialWeaponHealRuntime(src, dst, amount, d) })
}
