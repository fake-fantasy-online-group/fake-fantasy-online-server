package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func init() {

	registerLegendarySynergyHandler("YS-U07", legendaryHealerCleanseEscort())

	registerLegendarySynergyHandler("SS-A30", legendaryHandler{
		PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if d.HasDamageEffect() && d.PrepareMS > 0 && legendaryCasterNativeReady(c.Owner, false, true, false) {
				d.LegendaryNativeInstant = true
			}
		},
		RootFinished: func(c *legendaryContext) {
			if c.Def.LegendaryNativeInstant && len(c.Hits) > 0 && c.Target != nil {
				c.ChildSkill(c.Target.ID, c.Def, legendaryChildOptions{Center: &c.Center, Primary: &c.Primary, PreparedDefinition: true, ScaleBP: c.RollValue("child_bp", 8000), TargetOnly: true, SuppressGeneration: true})
			}
		},
	})
}
