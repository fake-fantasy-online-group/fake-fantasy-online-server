package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func init() {
	registerLegendarySynergyHandler("CK-A26", legendaryHandler{TheftAttempt: legendaryAssassinTheftPalm})
}
func (s *Scene) legendaryAssassinTheftAttempt(owner, target *entity.Entity, def domain.SkillDef, eligible bool) {
	if !eligible {
		return
	}
	for _, c := range s.legendaryContexts(owner, target, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		c.Skill = def.ID
		if h := legendaryHandlers[c.Power.ID].TheftAttempt; h != nil {
			c.invoke(h)
		}
	}
}
func legendaryAssassinTheftPalm(c *legendaryContext) {
	if c.Skill != 10212 || c.Target == nil {
		return
	}
	d := domain.SkillDef{ID: 10212, Name: "妙手震掌", Kind: domain.SkillArea, TargetEnemy: true, HurtType: domain.HurtPhysical, Radius: 180, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 180}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: c.RollValue("proc_pct", 240)}}
	center := c.Owner.Pos
	if c.ChildSkill(c.Target.ID, d, legendaryChildOptions{ScaleBP: 10000, SuppressVisual: true}) {
		c.VisualAt("stage-01", center, 0) // Immediate palm, not the retired delayed ground mark.
	}
}
