package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Keep the stage on the queued payload: presentation must not precede an
// impact which may be discarded because another payload killed its target.
func (c *legendaryContext) damageVisual(stage string, target *entity.Entity, amount int32, magic, settled bool) {
	copy := *c
	copy.PresentationStage = stage
	copy.Damage(target, amount, magic, settled)
}

// Persistent effects belong only to explicitly wired current gameplay states.
// Expiry, equipment removal, map departure and owner cleanup stop its exact
// cast key. No old mark, ring or flag is reconstructed from a status name.
func (s *Scene) legendaryStopStateGroundEffect(k legendaryStateKey, st *legendaryPowerState) {
	if st == nil || st.GroundEffectKey == "" {
		return
	}
	s.emit(event.GroundEffect{Owner: k.Owner, MapID: st.Position.MapID, Pos: st.Position, Key: st.GroundEffectKey, Mode: 2})
}

func legendaryWarriorImpactVisual(c *legendaryContext) {
	if !legendaryMeleeLanded(c) {
		return
	}
	skill := map[string]domain.SkillID{"ZS-A02": 10002, "ZS-A05": 10005, "ZS-A13": 10006, "ZS-A16": 10003, "ZS-A22": 10022}[c.Power.ID]
	if skill != 0 && c.Skill == skill {
		stage := "stage-01"
		if c.Power.ID == "ZS-A16" {
			if !legendaryMeleePrimary(c) {
				return
			}
			stage = "stage-02"
		}
		c.Visual(stage, c.Target)
	}
	// The old spear mark/next-hit trigger is gone. Only the approved explosion
	// art remains appropriate for the current immediate primary-centered burst.
	if c.Power.ID == "ZS-A12" && c.Skill == 10017 && legendaryMeleePrimary(c) {
		c.VisualAt("stage-02", c.Target.Pos, 0)
	}
}
func legendaryWarriorCastVisual(c *legendaryContext) {
	switch c.Power.ID {
	case "ZS-A04":
		if c.Skill == 10005 {
			c.Visual("stage-01", c.Owner)
		}
	case "ZS-A15":
		if c.Skill == 10006 {
			c.VisualAt("stage-01", c.Center, 0)
		}
	case "ZS-A16":
		if c.Skill == 10003 {
			c.Visual("stage-01", c.Owner)
		}
	case "ZS-U04", "ZS-U08", "ZS-U10":
		if c.Skill == 10028 {
			for _, id := range []string{"ZS-U04", "ZS-U08", "ZS-U10"} {
				if warriorHas(c, id) {
					if c.Power.ID != id {
						return
					}
					break
				}
			}
			c.Visual("stage-01", c.Owner)
		}
	}
}
