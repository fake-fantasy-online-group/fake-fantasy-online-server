package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func init() {
	registerWarriorPower("ZS-A03", legendaryHandler{BaseSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if !d.LegendaryBasic && d.ID != 10002 {
			return
		}
		num, den := int64(100), int64(1)
		if chain, ok := warriorLearned(c, 10002, false); ok {
			for _, e := range chain.DirectEffects() {
				if e.Kind == domain.EffectDamage {
					n, k := e.DamagePctNumerator, e.DamagePctDenominator
					if k <= 0 {
						n, k = int64(e.DamagePct), 1
					}
					if n*den > num*k {
						num, den = n, k
					}
				}
			}
		}
		warriorEffects(d, func(e *domain.SkillEffect) {
			if e.Kind != domain.EffectDamage {
				return
			}
			n, k := e.DamagePctNumerator, e.DamagePctDenominator
			if k <= 0 {
				n, k = int64(e.DamagePct), 1
			}
			if num*k > n*den {
				e.DamagePctNumerator = num
				e.DamagePctDenominator = den
				e.DamagePct = int32(num / den)
			}
		})
	}, PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.LegendaryBasic {
			d.LegendaryModifiers.DamageBP += 12000
		}
	}})
	registerWarriorPower("ZS-A14", legendaryHandler{CastReleased: warriorQuakeZone, Tick: warriorQuakeTick})
	registerWarriorPower("ZS-A26", legendaryHandler{TransformSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10016 {
			d.LegendaryDamageThroughControlImmunity = true
			d.Effects = append([]domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 240, GuaranteedHit: true}}, d.DirectEffects()...)
		}
	}, PrepareSkill: warriorSkill(10016, 0, 0, 0, 50).PrepareSkill})
}
func warriorQuakeZone(c *legendaryContext) {
	if c.Skill != 10006 {
		return
	}
	phase := "quake-1"
	a, b := c.State(0, "quake-1"), c.State(0, "quake-2")
	if a.ExpiresMS > c.NowMS && (b.ExpiresMS <= c.NowMS || b.ExpiresMS < a.ExpiresMS) {
		phase = "quake-2"
	}
	st := c.State(0, phase)
	// Replacing one of the two fields stops only its old presentation.
	c.Scene.legendaryStopStateGroundEffect(c.key(0, phase), st)
	st.RootID = c.RootID
	st.Position = c.Center
	st.ExpiresMS = c.NowMS + 3000
	st.NextMS = c.NowMS + 1000
	st.GroundEffectKey = fmt.Sprintf("%s/%d/stage-01", c.Power.ID, c.RootID)
	c.VisualAt("stage-01", st.Position, 3000)
	st.Value = int64(c.SnapshotNativeDamage()) * 35 / 100
	st.Stacks = max(c.Def.Area.Radius, c.Def.Radius)

	st.Name = c.Power.Name
	st.Description = c.Power.Effect
	st.IconID = legendaryMeleeIcon(c)
	st.Good = true
}
func warriorQuakeTick(c *legendaryContext) {
	for _, phase := range []string{"quake-1", "quake-2"} {
		st := c.State(0, phase)
		if st.Position.MapID != c.Owner.Pos.MapID {
			c.DeleteState(0, phase)
			continue
		}
		for st.NextMS > 0 && st.NextMS <= c.NowMS && st.NextMS <= st.ExpiresMS {
			at := st.NextMS
			st.NextMS += 1000
			visual := *c
			visual.RootID = st.RootID
			visual.VisualAt("stage-02", st.Position, 0)
			for _, t := range c.Enemies(st.Position, float64(st.Stacks), 256) {
				if !c.Scene.validTarget(c.Owner, t, domain.SkillDef{ID: 10006, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage}}) {
					continue
				}
				last := c.State(t.ID, "quake-last")
				if last.Value != 0 && at-last.Value < 1000 {
					continue
				}
				last.Value = at
				c.Damage(t, domain.SaturateInt32(st.Value), false, false)
			}
		}
	}
}
