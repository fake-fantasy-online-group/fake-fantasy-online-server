package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) legendaryCasterManaAttempt(owner *entity.Entity, needed int32) {

}
func legendaryCasterAdvanceSkill(id domain.SkillID) bool {
	return id == legendaryLightBolt || id == legendaryHolyLight || id == legendaryExorcism || id == legendaryDivine
}
func (s *Scene) legendaryCasterCooldownPermit(owner *entity.Entity, d domain.SkillDef) bool {
	return false
}
func init() {

	registerLegendarySynergyHandler("YS-U05", legendaryHandler{
		PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if d.ID != 10311 {
				return
			}
			d.LegendaryEntityCenter = true
			d.LegendaryFriendlyCenter = true
			specialCircle(d, 220, domain.SkillAreaCenterCaster, 0)
			if c.Preview || c.Target != nil && c.Target != c.Owner && sameParty(c.Owner, c.Target) {
				d.Area.Center = domain.SkillAreaCenterTarget
				d.Dist = 600
			}
			d.LegendaryModifiers.StatusPowerBP += c.RollValue("buff_bp", 3000)
		},
		SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
			if c.Skill != 10311 || c.Target == nil || c.Target == c.Owner || !sameParty(c.Owner, c.Target) {
				return ids
			}
			for _, id := range ids {
				if id == c.Owner.ID {
					return ids
				}
			}
			return append(ids, c.Owner.ID)
		},
	})
	registerLegendarySynergyHandler("YS-A05", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID != legendaryHolyLight {
			return
		}
		d.LegendaryEntityCenter = true
		d.LegendaryFriendlyCenter = true
		if c.Preview {
			d.Dist = 600
			specialCircle(d, 200, domain.SkillAreaCenterTarget, 0)
			return
		}
		if c.Target == nil || c.Target != c.Owner && !sameParty(c.Owner, c.Target) {
			return
		}
		d.Dist = 600
		specialCircle(d, 200, domain.SkillAreaCenterTarget, 0)
		d.TargetSelf = true
		d.TargetTeam = true
		d.LegendaryModifiers.DamageBP += c.RollValue("damage_bp", 2000)
	}, SelectTargets: func(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
		if c.Skill != legendaryHolyLight || c.Target == nil || c.Target != c.Owner && !sameParty(c.Owner, c.Target) {
			return ids
		}
		out := ids[:0]
		for _, id := range ids {
			if c.Scene.equipmentProcEnemyTarget(c.Owner, c.Scene.entities[id]) {
				out = append(out, id)
			}
		}
		return out
	}})
}
