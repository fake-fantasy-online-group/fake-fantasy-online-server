package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

func (s *Scene) legendaryStatusAccuracy(src, dst *entity.Entity, d *domain.StatusDef) {
	if src == nil || dst == nil || d == nil {
		return
	}
	for _, c := range s.legendaryContextsFor(src, dst, combat.Blow{}, event.DamageDealt{}, true) {
		if fn := legendaryHandlers[c.Power.ID].StatusAccuracy; fn != nil {
			fn(&c, d)
		}
	}
}
func (s *Scene) legendaryIncomingHeal(dst *entity.Entity, n int32) int32 {
	if dst == nil || n <= 0 {
		return n
	}
	total := int64(n)
	for _, c := range s.legendaryContextsFor(dst, nil, combat.Blow{}, event.DamageDealt{}, true) {
		if fn := legendaryHandlers[c.Power.ID].IncomingHeal; fn != nil {
			total += int64(fn(&c, n)) - int64(n)
		}
	}
	return max(int32(0), domain.SaturateInt32(total))
}
func init() {
	registerLegendarySynergyHandler("TY-A20", legendaryHandler{Teleported: universalLandingPull})
}
func universalLandingPull(c *legendaryContext, from domain.Pos) {
	if c.Skill == 0 || c.Owner == nil || from == c.Owner.Pos {
		return
	}
	d := c.Def
	if d.ID != c.Skill {
		var ok bool
		d, ok = legendaryCasterSkill(c, c.Skill)
		if !ok {
			return
		}
	}
	if !universalHasEffect(&d, domain.EffectTeleportPoint, domain.EffectTeleportTarget) {
		return
	}
	for _, e := range c.Enemies(c.Owner.Pos, 200, 0) {
		if e.Monster == nil {
			continue
		}
		distance := math.Sqrt(sqDist(c.Owner.Pos, e.Pos))
		if distance <= 1 {
			continue
		}
		source := *c.Owner
		source.Pos = e.Pos
		source.Pos.X += (e.Pos.X - c.Owner.Pos.X) / distance
		source.Pos.Y += (e.Pos.Y - c.Owner.Pos.Y) / distance
		c.Scene.knockbackMonster(&source, e, int32(math.Min(float64(c.RollValue("benefit", 100)), math.Max(0, distance-1))))
	}
}
