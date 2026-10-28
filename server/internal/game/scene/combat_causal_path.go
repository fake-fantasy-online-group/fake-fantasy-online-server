package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"slices"
)

// Causal paths belong to an individual branch, not to a cast-wide budget. A
// different effect may react; the same owner/effect cannot revisit that branch.
// Slices are immutable after admission so sibling targets never share mutations.
func (s *Scene) combatEffectKey(owner domain.EntityID, id string) string {
	if e := s.entities[owner]; e != nil && e.Player != nil && e.Player.Char != nil && e.Player.Char.ID != 0 {
		return fmt.Sprintf("character:%d:%s", e.Player.Char.ID, id)
	}
	return fmt.Sprintf("entity:%d:%s", owner, id)
}
func (s *Scene) combatEffectActive(owner domain.EntityID, id string) bool {
	return slices.Contains(s.legendaryState().causalPath, s.combatEffectKey(owner, id))
}
func (s *Scene) withCombatCausalPath(path []string) func() {
	rt := s.legendaryState()
	old := rt.causalPath
	rt.causalPath = path
	return func() { rt.causalPath = old }
}
func (c *legendaryContext) effectPath() []string {
	path := append([]string(nil), c.CausalPath...)
	if c.Owner != nil && c.Power.ID != "" {
		path = append(path, c.Scene.combatEffectKey(c.Owner.ID, c.Power.ID))
	}
	return path
}
func (c *legendaryContext) invoke(fn func(*legendaryContext)) {
	c.dispatchEquipmentHook("event", true, func() { fn(c) })
}

func (c *legendaryContext) RestoreHealth(target *entity.Entity, amount int32, skill domain.SkillID) {
	if c.Preview {
		return
	}
	defer c.Scene.withCombatCausalPath(c.effectPath())()
	c.Scene.restoreHealthWithReactions(c.Owner, target, amount, skill)
}
func (s *Scene) restoreHealthWithReactions(source, target *entity.Entity, amount int32, skill domain.SkillID) {
	if source == nil || target == nil || !target.Alive() || amount <= 0 {
		return
	}
	before := target.HP
	amount = s.legendaryIncomingHeal(target, amount)
	ev := combat.Restore(source, target, amount, int32(skill))
	s.emit(ev)
	def := domain.SkillDef{ID: skill}
	s.legendaryCasterHealed(source, target, def, before, ev.Amount)
	s.legendaryHealResolved(source, target, def, amount, ev.Amount, max(int32(0), amount-ev.Amount))
	s.emitAttributesIfPlayer(target)
}

// enterCombatEffect is used by native/set producers which do not own a legendary context.
func (s *Scene) enterCombatEffect(owner domain.EntityID, id string) (bool, func()) {
	if s.combatEffectActive(owner, id) {
		return false, func() {}
	}
	path := append(append([]string(nil), s.legendaryState().causalPath...), s.combatEffectKey(owner, id))
	return true, s.withCombatCausalPath(path)
}
