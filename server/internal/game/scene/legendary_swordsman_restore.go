package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

type legendarySwordRestorePayload struct {
	src    *entity.Entity
	def    domain.SkillDef
	amount int32
	mana   bool
}

// Capture only after native RequiresPreviousLanded admission. Secondary hits
// establish a successful cast but cannot repeat its native self restoration.
func (s *Scene) queueLegendarySwordRestore(src, dst *entity.Entity, def domain.SkillDef, amount int32, mana bool) bool {
	root := s.equipmentState().root
	if root == nil || src != dst || root.src != src.ID || mana && def.ID != 10119 || !mana && def.ID != 10104 {
		return false
	}
	if root.swordRestore == nil {
		root.swordRestore = &legendarySwordRestorePayload{src, def, amount, mana}
	}
	return true
}
func (s *Scene) flushLegendarySwordRestore() {
	root := s.equipmentState().root
	if root == nil || root.swordRestore == nil {
		return
	}
	payload := root.swordRestore
	root.swordRestore = nil
	src, def := payload.src, payload.def
	if src == nil || !src.Alive() {
		return
	}
	amount := s.legendarySwordsmanNativeRestoreAmount(src, def, payload.amount, root.hits)
	if payload.mana {
		before := src.MP
		src.MP = min(src.MaxMP, domain.SaturateInt32(int64(src.MP)+int64(amount)))
		effective := src.MP - before
		overflow := max(int32(0), amount-effective)
		if def.SpecialWeaponBaseID == 1522 {
			s.specialWeaponShield(src, 1522, overflow/2, 5, src.ID)
		}
		s.legendaryCasterManaRestored(src, src, def, effective, overflow)
		s.legendarySwordsmanNativeRestoreResolved(src, def, effective, overflow)
	} else {
		amount = s.uniqueHealing(src, src, amount, def)
		amount = s.classSetCardHealing(src, amount)
		s.casterSetCopyHeal(src, src, amount, def.ID)
		amount = s.startVariantMend(src, src, def, amount)
		amount = s.legendaryCasterModifyHeal(src, amount)
		before := src.HP
		ev := combat.Restore(src, src, amount, int32(def.ID))
		overflow := max(int32(0), src.Status.HealingReceivedAmount(amount)-ev.Amount)
		s.legendaryCasterHealed(src, src, def, before, ev.Amount)
		s.legendaryHealResolved(src, src, def, amount, ev.Amount, overflow)
		s.legendarySwordsmanNativeRestoreResolved(src, def, ev.Amount, overflow)
		s.emit(ev)
	}
	s.emitAttributesIfPlayer(src)
}
