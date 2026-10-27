package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func init() {
	registerLegendarySynergyHandler("CK-A25", legendaryHandler{SelectTargets: legendaryAssassinCounterTargets})
}
func legendaryAssassinCounterTargets(c *legendaryContext, ids []domain.EntityID) []domain.EntityID {
	if c.Skill != 10203 || c.Target == nil {
		return ids
	}
	out := []domain.EntityID{c.Target.ID}
	for _, v := range c.Enemies(c.Target.Pos, float64(c.RollValue("radius", 100)), 0) {
		if v.ID != c.Target.ID {
			out = append(out, v.ID)
		}
	}
	return out
}
func (s *Scene) legendaryAssassinCounterArea(owner, primary *entity.Entity, b combat.Blow) {
	c, ok := legendaryAssassinPowerContext(s, owner, "CK-A25")
	if !ok {
		return
	}
	c.Target, c.Skill = primary, 10203
	ids := legendaryAssassinCounterTargets(&c, []domain.EntityID{primary.ID})
	for _, id := range ids {
		if id == primary.ID {
			continue
		}
		v := s.entities[id]
		if v == nil || !v.Alive() {
			continue
		}
		tr := s.hits[owner.ID]
		if tr == nil {
			tr = &combat.HitTracker{}
			s.hits[owner.ID] = tr
		}
		ev, raw, reflect := s.resolveStrikeStatuses(owner, v, tr, b)
		s.emit(ev)
		s.afterStrikeStatuses(owner, v, ev, raw, reflect)
		s.consumeHitDurability(v, ev)
		s.emitAttributesIfPlayer(v)
		s.aggroAfterHit(v, owner.ID, ev)
		if !v.Alive() {
			s.onDeath(v, owner.ID)
		}
	}
}
