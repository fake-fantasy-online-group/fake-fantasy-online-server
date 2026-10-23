package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func init() {

	registerLegendarySynergyHandler("YS-U07", legendaryHandler{
		Dispelled: func(c *legendaryContext, a []domain.Active) {
			if c.Skill == 10306 && len(a) > 0 {
				st := c.State(0, "cleanse-result")
				st.RootID = c.RootID
				st.Stacks = 1
			}
		},
		RootFinished: func(c *legendaryContext) {
			if c.Skill != 10306 {
				return
			}
			st := c.State(0, "cleanse-result")
			if st.Stacks > 0 && st.RootID == c.RootID {
				c.RefundMP(specialScale(c.PaidMP, 30))
			} else {
				c.RefundMP(specialScale(c.PaidMP, 90))
				c.Owner.Player.ScaleRemainingSkillCooldown(c.Skill, c.Scene.tick, 20)
			}
		},
	})

	registerLegendarySynergyHandler("SS-A30", legendaryHandler{
		PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if d.HasDamageEffect() && d.PrepareMS > 0 && legendaryCasterNativeReady(c.Owner, false, true, false) {
				d.LegendaryNativeInstant = true
			}
		},
		RootFinished: func(c *legendaryContext) {
			if c.Def.LegendaryNativeInstant && len(c.Hits) > 0 && c.Target != nil {
				d, ok := legendaryCasterSkill(c, c.Skill)
				if ok {
					c.ChildSkill(c.Target.ID, d, legendaryChildOptions{ScaleBP: 8000, TargetOnly: true, SuppressGeneration: true})
				}
			}
		},
	})
}
