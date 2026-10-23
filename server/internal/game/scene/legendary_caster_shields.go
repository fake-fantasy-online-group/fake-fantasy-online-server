package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
	"sort"
)

func (s *Scene) legendaryCasterShieldResolved(target, attacker *entity.Entity, before []domain.Active) {

}
func init() {

	registerLegendarySynergyHandler("SS-A14", legendaryHandler{
		TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
			if c.Skill == legendaryIceBurst {
				return specialScale(n, 120)
			}
			return n
		}),
		RootFinished: func(c *legendaryContext) {
			if c.Skill != legendaryIceBurst {
				return
			}
			var enemies []*entity.Entity
			seen := map[domain.EntityID]bool{}
			for _, h := range c.Hits {
				t := c.Scene.entities[h.dst]
				if t != nil && !seen[t.ID] && t.Alive() && t.Monster != nil && !t.Status.SuppressesHarmfulAt(c.Scene.tick) {
					seen[t.ID] = true
					enemies = append(enemies, t)
				}
			}
			sort.Slice(enemies, func(i, j int) bool { return sqDist(enemies[i].Pos, c.Center) > sqDist(enemies[j].Pos, c.Center) })
			if len(enemies) > 5 {
				enemies = enemies[:5]
			}
			moved := false
			for _, t := range enemies {
				before := t.Pos
				legendaryCasterPull(c, t, c.Center, 90)
				moved = moved || before != t.Pos
			}
			if moved {
				c.VisualAt("stage-01", c.Center, 0)
			}
		},
	})
}
func legendaryCasterPull(c *legendaryContext, t *entity.Entity, center domain.Pos, distance float64) {
	if c.Scene.legendaryMeleeDisplacementImmune(t) {
		return
	}
	if t == nil || !t.Alive() || t.Monster == nil {
		return
	}
	if t.Monster.Kind == domain.MonsterBoss || t.Status != nil && t.Status.SuppressesHarmfulAt(c.Scene.tick) {
		return
	}
	if c.Scene.defs != nil {
		if d, ok := c.Scene.defs.Def(t.Monster.TypeID); ok {
			if _, immune := d.ImmuneSkills[c.Skill]; immune {
				return
			}
		}
	}
	dx, dy := center.X-t.Pos.X, center.Y-t.Pos.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	distance = math.Min(distance, length)
	from, to := t.Pos, t.Pos
	for n := 5.0; n <= distance+5; n += 5 {
		step := math.Min(n, distance)
		p := domain.Pos{MapID: from.MapID, X: from.X + dx/length*step, Y: from.Y + dy/length*step}
		if !c.Scene.straightPathWalkable(to, p) {
			break
		}
		blocked := false
		for _, other := range c.Scene.entities {
			if other.ID != t.ID && other.Alive() && sqDist(p, other.Pos) < 20*20 {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		to = p
		if step == distance {
			break
		}
	}
	if to == from {
		return
	}
	c.Scene.aoi.Move(t, to)
	c.Scene.legendaryMeleeMoved(t, from, to, true)
	c.Scene.emit(event.EntityMoved{ID: t.ID, To: to, Dir: t.Dir, Snap: true})
	t.Monster.CombatPathActive = false
	t.Monster.Roaming = false
}
