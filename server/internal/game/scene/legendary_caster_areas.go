package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func legendaryCasterArea(c *legendaryContext, key string, center domain.Pos, radius int32, damage int32, seconds int32) {
	st := c.State(0, key)
	st.Position = center
	st.Value = int64(damage)
	st.Stacks = seconds
	st.ExpiresMS = c.NowMS + int64(seconds)*1000 + 1
	timer := c.State(0, key+"-timer")
	timer.Value = c.NowMS + 1000
	timer.Stacks = radius
	timer.ExpiresMS = st.ExpiresMS
}
func legendaryCasterTickArea(c *legendaryContext, key string) {
	st := c.State(0, key)
	timer := c.State(0, key+"-timer")
	if st.Stacks <= 0 || timer.Value > c.NowMS {
		return
	}
	// One payload per authored tick, even when the scheduler resumes late.
	for st.Stacks > 0 && timer.Value <= c.NowMS {
		for _, t := range c.Enemies(st.Position, float64(timer.Stacks), 256) {
			c.Damage(t, clampI32(st.Value), true, false)
		}
		st.Stacks--
		timer.Value += 1000
	}
	if st.Stacks <= 0 {
		c.DeleteState(0, key)
		c.DeleteState(0, key+"-timer")
	}
}
func init() {
	registerLegendarySynergyHandler("SS-A04", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill != legendaryFireball {
			return
		}
		c.VisualAt("stage-01", c.Center, 0)
		radius := max(c.Def.Radius, c.Def.Area.Radius)
		for _, t := range c.Enemies(c.Center, float64(radius), 256) {
			if d, ok := legendaryCasterSkill(c, c.Skill); ok {
				c.ChildSkill(t.ID, d, legendaryChildOptions{ScaleBP: 7000, TargetOnly: true, SuppressGeneration: true, SuppressVisual: true})
			}
		}
	}})
	registerLegendarySynergyHandler("SS-A05", legendaryHandler{TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
		if c.Skill != legendaryFireball {
			return n
		}
		count := 0
		cached := c.State(0, "fireball-target-count")
		if c.RootID != 0 && cached.RootID == c.RootID && cached.Stacks > 0 {
			count = int(cached.Value)
		} else {
			for _, id := range c.Targets {
				if t := c.Scene.entities[id]; t != nil && t.Alive() && c.Scene.equipmentProcEnemyTarget(c.Owner, t) {
					count++
				}
			}
			if c.RootID != 0 {
				cached.RootID = c.RootID
				cached.Stacks = 1
				cached.Value = int64(count)
			}
		}
		if count >= 1 && count <= 2 {
			return specialScale(n, 175)
		}
		if c.Target != nil && c.Target.ID == c.Primary {
			return specialScale(n, 135)
		}
		return n
	})})

	registerLegendarySynergyHandler("YS-A07", legendaryHandler{
		TargetDefinition: legendaryCasterTargetBonus(func(c *legendaryContext, n int32) int32 {
			if c.Skill == legendaryHolyLight {
				return specialScale(n, 110)
			}
			return n
		}),
		RootFinished: func(c *legendaryContext) {
			if c.Skill != legendaryHolyLight {
				return
			}
			a, b := c.State(0, "light-field-0"), c.State(0, "light-field-1")
			key := "light-field-0"
			if b.ExpiresMS < a.ExpiresMS {
				key = "light-field-1"
			}
			c.Scene.legendaryStopStateGroundEffect(c.key(0, key), c.State(0, key))
			legendaryCasterArea(c, key, c.Center, 180, specialScale(legendaryCasterBase(c, c.Skill), 30), 4)
			// Slot identity replaces the old visual when the two-zone pool wraps.
			if fx, _, ok := legendaryVisualEffect(c.Power.ID, "stage-01"); ok && !c.Preview {
				groundKey := fmt.Sprintf("%s/%d/%s", c.Power.ID, c.SourceUID, key)
				c.State(0, key).GroundEffectKey = groundKey
				c.Scene.emit(event.GroundEffect{Owner: c.Owner.ID, MapID: c.Center.MapID, Pos: c.Center, Key: groundKey, Effect: fx, DurationMS: 4000, Mode: 1})
			}
		},
		Tick: func(c *legendaryContext) {
			// Both zones share a per-victim one-second maximum, rather than stacking.
			for i := 0; i < 2; i++ {
				key := fmt.Sprintf("light-field-%d", i)
				st := c.State(0, key)
				timer := c.State(0, key+"-timer")
				if st.Stacks <= 0 || timer.Value > c.NowMS {
					continue
				}
				c.VisualAt("stage-01", st.Position, 0)
				for _, t := range c.Enemies(st.Position, 180, 256) {
					hit := c.State(t.ID, "light-field-tick")
					if hit.ExpiresMS > c.NowMS {
						continue
					}
					best := st.Value
					for j := 0; j < 2; j++ {
						other := c.State(0, fmt.Sprintf("light-field-%d", j))
						if other.Stacks > 0 && other.ExpiresMS > c.NowMS && sqDist(other.Position, t.Pos) <= 180*180 {
							best = max(best, other.Value)
						}
					}
					hit.ExpiresMS = c.NowMS + 1000
					c.Damage(t, clampI32(best), true, false)
				}
				st.Stacks--
				timer.Value += 1000
				if st.Stacks <= 0 {
					c.DeleteState(0, key)
					c.DeleteState(0, key+"-timer")
				}
			}
		},
	})

}
