package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
)

// No credit is created on a rejected cast. Admission always pays the displayed
// integer cost, so even a sub-one-MP skill cannot be cast at zero MP. Returning
// prepaid fractions conserves total mana (within floating-point roundoff).
// Logout forfeits less than one prepaid MP; it cannot mint free casts.
func settleSkillManaCredit(caster *entity.Entity, def domain.SkillDef) {
	if caster == nil || caster.Player == nil || def.ManaCostDenominator <= 0 {
		return
	}
	average := float64(def.ManaCostNumerator) / float64(def.ManaCostDenominator)
	credit := caster.Player.SkillManaCredit + float64(def.MPCost) - average
	if math.IsNaN(credit) || math.IsInf(credit, 0) || credit < 0 {
		return
	}
	refund := int32(math.Floor(credit))
	caster.Player.SkillManaCredit = credit - float64(refund)
	caster.MP = min(caster.MaxMP, domain.SaturateInt32(int64(caster.MP)+int64(refund)))
}
