package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

func init() {
	for _, id := range []string{"CK-A28", "CK-A30"} {
		registerLegendarySynergyHandler(id, legendaryHandler{TransformSkill: legendaryAssassinPullTransform, PrepareSkill: legendaryAssassinPullPrepare, RootFinished: legendaryAssassinPull})
	}
	registerLegendarySynergyHandler("CK-U10", legendaryHandler{StatusApplied: legendaryAssassinStealthSpeed})
}
func legendaryAssassinPullTransform(c *legendaryContext, d *domain.SkillDef) {
	if c.Power.ID == "CK-A30" && d.ID == 10225 && c.Owner.Player.Char.Skills[10226] > 0 {
		d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
		for i := range d.Effects {
			if d.Effects[i].Kind == domain.EffectKnockback {
				d.Effects[i].KnockbackDistance = 0
			}
		}
	}
}
func legendaryAssassinPullPrepare(c *legendaryContext, d *domain.SkillDef) {
	if d.ID != 10225 || c.Owner.Player.Char.Skills[10226] <= 0 {
		return
	}
	if c.Power.ID == "CK-A28" {
		d.LegendaryModifiers.DamageBP += 4500
	}
	if c.Power.ID == "CK-A30" {
		d.LegendaryModifiers.RadiusBP += 5000
	}
}
func legendaryAssassinPull(c *legendaryContext) {
	if c.Skill != 10225 || c.Owner.Player.Char.Skills[10226] <= 0 {
		return
	}
	if c.Power.ID == "CK-A28" && legendaryAssassinHas(c.Scene, c.Owner, "CK-A30") {
		return
	}
	seen := map[domain.EntityID]bool{}
	for _, h := range c.Hits {
		if h.src != c.Owner.ID || h.ev.Flag.Has(event.DamageMiss) || seen[h.dst] {
			continue
		}
		seen[h.dst] = true
		target := c.Scene.entities[h.dst]
		if target == nil || !target.Alive() {
			continue
		}
		maximum := float64(120)
		if c.Power.ID == "CK-A30" {
			maximum = math.MaxFloat64
		}
		c.Scene.legendaryAssassinPullTarget(c.Owner, target, maximum)
	}
}
func (s *Scene) legendaryAssassinPullTarget(owner, target *entity.Entity, maximum float64) {
	distance := math.Sqrt(sqDist(owner.Pos, target.Pos))
	if distance <= 40 {
		return
	}
	source := *owner
	source.Pos = target.Pos
	source.Pos.X += (target.Pos.X - owner.Pos.X) / distance
	source.Pos.Y += (target.Pos.Y - owner.Pos.Y) / distance
	s.knockbackMonster(&source, target, int32(math.Min(maximum, distance-40)))
}
func legendaryAssassinStealthSpeed(c *legendaryContext, d *domain.StatusDef) {
	if d.Invisible && (d.SourceSkillID == 10208 || d.SkillSnapshotID == 10208) {
		d.LegendaryModifiers.MoveSpeedBonusPct += 40
	}
}
