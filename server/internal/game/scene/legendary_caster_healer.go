package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) legendaryCasterAccepted(src *entity.Entity, def domain.SkillDef, primary domain.EntityID, targets []domain.EntityID, paid int32) {
	for _, c := range s.legendaryContexts(src, s.entities[primary], combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		c.PaidMP = paid
		c.Primary = primary
		c.Targets = targets
		if h := legendaryHandlers[c.Power.ID].CastAccepted; h != nil {
			h(&c)
		}
	}
}
func (s *Scene) legendaryCasterResurrected(src, dst *entity.Entity, def domain.SkillDef) {
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].Resurrected; h != nil {
			h(&c)
		}
	}
}
func legendaryCasterHasSkill(e *entity.Entity, skill domain.SkillID) bool {
	if e == nil || e.Status == nil {
		return false
	}
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if a.Def.SkillSnapshotID == skill || a.Def.SourceSkillID == int32(skill) {
			found = true
		}
	})
	return found
}
func init() {

}
func (s *Scene) legendaryCasterSteadyCast(e *entity.Entity, skill domain.SkillID) bool {
	if skill != 10301 && skill != 10304 && skill != 10305 {
		return false
	}
	ready := false
	s.forEachLegendary(e, func(c *legendaryContext) {
		if c.Power.ID == "YS-D05" && legendaryHealerInvested(c, 10303) && legendaryCasterHasSkill(e, 10303) || c.Power.ID == "YS-D06" && skill == 10304 {
			ready = true
		}
	})
	return ready
}
