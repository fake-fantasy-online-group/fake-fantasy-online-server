package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"slices"
)

func legendaryMeleeReach(c *legendaryContext) float64 {
	if c.Owner.Look.AtkDist > 0 {
		return float64(c.Owner.Look.AtkDist)
	}
	return 75
}
func legendaryMeleeAppendUnique(ids []domain.EntityID, id domain.EntityID) []domain.EntityID {
	if !slices.Contains(ids, id) {
		ids = append(ids, id)
	}
	return ids
}
func init() {

	registerLegendaryHandler("JK-A02", legendaryHandler{BeforeDamage: legendaryMeleeSkillDamage(10117, 115), SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill != 10117 || len(ids) == 0 {
			return ids
		}
		for _, e := range c.Enemies(c.Owner.Pos, 150, 8) {
			ids = legendaryMeleeAppendUnique(ids, e.ID)
		}
		return ids
	}, AfterHit: func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && c.Skill == 10117 && sqDist(c.Owner.Pos, c.Target.Pos) <= 150*150 {
			c.Visual("stage-01", c.Target)
		}
	}})

}
func legendaryMeleeStripControl(c *legendaryContext, d *domain.SkillDef) {
	keep := make([]domain.StatusApplication, 0, len(d.Statuses))
	for _, app := range d.Statuses {
		control := app.ID == domain.StatusStun || app.ID == domain.StatusPetrify || app.ID == domain.StatusFreeze || app.ID == domain.StatusSilence || app.ID == domain.StatusSlow
		if c.Scene.statuses != nil {
			if def, ok := c.Scene.statuses.Get(app.ID, app.Level); ok {
				control = control || def.Control != domain.ControlNone || def.BlockMove || def.Immobile != 0
				for _, a := range def.Affixes {
					control = control || a.Attr == domain.AttrMoveSpeed && a.Value < 0
				}
			}
		}
		if !control {
			keep = append(keep, app)
		}
	}
	d.Statuses = keep
	effects := make([]domain.SkillEffect, 0, len(d.DirectEffects()))
	for _, effect := range d.DirectEffects() {
		if effect.Kind != domain.EffectKnockback && effect.Kind != domain.EffectTaunt {
			effects = append(effects, effect)
		}
	}
	d.Effects = effects
}
