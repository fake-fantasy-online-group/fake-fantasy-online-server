package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Functional windows belong to the equipment source, not a saved character buff.
// Their fixed duration is refreshed by real casts; learning is checked again at
// consumption so a respec never leaves an unlearned skill's benefit active.
func warriorFunctionalLearned(c *legendaryContext, skill domain.SkillID) bool {
	return c.Owner != nil && c.Owner.Alive() && c.Owner.Player != nil && c.Owner.Player.Char != nil && c.Owner.Player.Char.Skills[skill] > 0
}
func warriorFunctionalActive(c *legendaryContext, skill domain.SkillID) bool {
	if !warriorFunctionalLearned(c, skill) || c.Scene.equipment == nil || c.Scene.equipment.legendary == nil {
		return false
	}
	st := c.Scene.equipment.legendary.states[c.key(0, "functional-window")]
	return st != nil && st.ExpiresMS > c.NowMS
}
func warriorFunctionalRelease(skill domain.SkillID, duration int64, movement bool) func(*legendaryContext) {
	return func(c *legendaryContext) {
		if c.Skill != skill || c.IsChild || c.IsBasic || c.Blow.ProcDepth != 0 || !warriorFunctionalLearned(c, skill) {
			return
		}
		st := c.State(0, "functional-window")
		st.ExpiresMS = c.NowMS + duration
		st.Name, st.Description, st.IconID, st.Good = c.Power.Name, c.Power.Effect, legendaryMeleeIcon(c), true
		if movement {
			c.Scene.refreshEntityStats(c.Owner)
			c.Scene.emitAttributesIfPlayer(c.Owner)
		}
	}
}
func warriorFunctionalTick(skill domain.SkillID, movement bool) func(*legendaryContext) {
	return func(c *legendaryContext) {
		if c.Scene.equipment == nil || c.Scene.equipment.legendary == nil {
			return
		}
		st := c.Scene.equipment.legendary.states[c.key(0, "functional-window")]
		if st == nil || st.ExpiresMS == 0 || warriorFunctionalActive(c, skill) {
			return
		}
		c.DeleteState(0, "functional-window")
		if movement {
			c.Scene.refreshEntityStats(c.Owner)
			c.Scene.emitAttributesIfPlayer(c.Owner)
		}
	}
}
func warriorFunctionalMove(skill domain.SkillID, percent int32) func(*legendaryContext, *domain.Stats) {
	return func(c *legendaryContext, stats *domain.Stats) {
		if warriorFunctionalActive(c, skill) {
			stats.MoveSpeed = specialScale(stats.MoveSpeed, 100+c.RollValue("move_pct", percent))
		}
	}
}
func warriorFunctionalFreeMana(d *domain.SkillDef) {
	d.MPCost = 0
	d.ManaCostFractionNumerator = 0
	d.ManaCostFractionDenominator = 1
	d.RoundingPreviewReady = false
}
