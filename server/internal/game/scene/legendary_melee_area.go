package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func legendaryMeleePrimary(c *legendaryContext) bool {
	if c.Target == nil {
		return false
	}
	return c.Primary == 0 || c.Primary == c.Target.ID
}
func legendaryMeleeBase(c *legendaryContext) int32 {
	pct := c.Blow.SkillPct
	if pct == 0 {
		pct = 100
	}
	if c.Blow.SkillPctDenominator > 0 {
		return domain.SaturateInt32(int64(c.Attack(false)) * c.Blow.SkillPctNumerator / (100 * c.Blow.SkillPctDenominator))
	}
	return legendaryMeleeScale(c.Attack(false), int64(pct))
}
func legendaryMeleeNearbyOthers(c *legendaryContext, radius float64, limit int) []*entity.Entity {
	all := c.Enemies(c.Target.Pos, radius, limit+1)
	out := make([]*entity.Entity, 0, limit)
	for _, e := range all {
		if e.ID != c.Target.ID {
			out = append(out, e)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}
func init() {

	registerLegendaryHandler("JK-A04", legendaryHandler{RootFinished: legendaryMeleeFarBlade})

	registerLegendaryHandler("JK-A07", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && c.Skill == 10103 {
			return legendaryMeleeScale(n, 110)
		}
		return n
	}, AfterHit: func(c *legendaryContext) {
		if !legendaryMeleeLanded(c) || c.Skill != 10103 || !legendaryMeleePrimary(c) || !legendaryMeleeOnce(c, 0, "shadows") {
			return
		}
		amount := legendaryMeleeScale(legendaryMeleeBase(c), 45)
		for i := 0; i < 2; i++ {
			if c.Target.Alive() && sqDist(c.Owner.Pos, c.Target.Pos) <= 150*150 {
				c.damageVisual("stage-01", c.Target, amount, false, false)
			}
		}
	}})
	registerLegendaryHandler("JK-A08", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if !legendaryMeleeDirect(c) || c.Skill != 10103 || !legendaryMeleePrimary(c) {
			return n
		}
		st := c.State(0, "alone")
		st.RootID = c.RootID
		st.Value = 0
		if len(legendaryMeleeNearbyOthers(c, 75, 1)) == 0 {
			st.Value = 1
			return legendaryMeleeScale(n, 190)
		}
		return n
	}, AfterHit: func(c *legendaryContext) {
		if legendaryMeleeLanded(c) && c.Skill == 10103 && legendaryMeleePrimary(c) && c.State(0, "alone").Value == 0 && legendaryMeleeOnce(c, 0, "splash") {
			for _, e := range legendaryMeleeNearbyOthers(c, 75, 2) {
				c.damageVisual("stage-01", e, legendaryMeleeScale(c.Attack(false), 130), false, false)
			}
		}
	}})
	registerLegendaryHandler("JK-A22", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && c.Skill == legendarySwordMeteor {
			return legendaryMeleeScale(n, 120)
		}
		return n
	}, AfterHit: legendaryMeleeMeteor})
	registerLegendaryHandler("JK-A23", legendaryHandler{BeforeDamage: func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && c.Skill == legendarySwordMeteor && legendaryMeleePrimary(c) {
			st := c.State(0, "low")
			st.RootID = c.RootID
			st.Value = 0
			if int64(c.Target.HP)*2 <= int64(c.Target.MaxHP) {
				st.Value = 1
			}
		}
		return n
	}, AfterHit: func(c *legendaryContext) {
		if !legendaryMeleeLanded(c) || c.Skill != legendarySwordMeteor || !legendaryMeleePrimary(c) || !legendaryMeleeOnce(c, 0, "burst") {
			return
		}
		if c.Hit.Flag.Has(event.DamageFatal) || c.State(0, "low").Value != 0 {
			for _, e := range legendaryMeleeNearbyOthers(c, 150, 5) {
				c.damageVisual("stage-01", e, legendaryMeleeScale(legendaryMeleeBase(c), 110), false, false)
			}
		}
	}})
}

func legendaryMeleeFarBlade(c *legendaryContext) {
	var chosen *equipmentHit
	far := float64(75 * 75)
	for i := range c.Hits {
		h := &c.Hits[i]
		if h.src != c.Owner.ID || h.blow.SkillID != 10117 || h.blow.Origin != domain.DamageNormalOrigin || h.blow.ProcDepth != 0 || h.ev.Flag.Has(event.DamageMiss) {
			continue
		}
		t := c.Scene.entities[h.dst]
		if t == nil {
			continue
		}
		d := sqDist(c.Owner.Pos, t.Pos)
		if d >= far && (chosen == nil || d > far || h.dst < chosen.dst) {
			chosen = h
			far = d
		}
	}
	if chosen == nil {
		return
	}
	copy := *c
	copy.Blow = chosen.blow
	center := c.Scene.entities[chosen.dst].Pos
	amount := legendaryMeleeScale(legendaryMeleeBase(&copy), 75)
	for _, e := range c.Enemies(center, 100, 5) {
		c.damageVisual("stage-01", e, amount, false, false)
	}
}

func legendaryMeleeMeteor(c *legendaryContext) {
	if !legendaryMeleeLanded(c) || c.Skill != legendarySwordMeteor || !legendaryMeleePrimary(c) || !legendaryMeleeOnce(c, 0, "meteor") {
		return
	}
	base := legendaryMeleeBase(c)
	for _, e := range legendaryMeleeNearbyOthers(c, 120, 5) {
		c.damageVisual("stage-01", e, base, false, false)
	}
	for _, e := range c.Enemies(c.Target.Pos, 120, 6) {
		c.damageVisual("stage-02", e, legendaryMeleeScale(base, 70), false, false)
	}
}
