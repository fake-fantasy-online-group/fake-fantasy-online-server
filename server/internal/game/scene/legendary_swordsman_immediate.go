package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// These revisions retain the explicitly unchanged original coefficients and
// remove only the additional history/window required by the proposal.
func init() {
	registerLegendarySynergyHandler("JK-U02", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill == 10104 {
			swordCleanse(c, true)
		}
	}})
	registerLegendarySynergyHandler("JK-A20", legendaryHandler{AfterHit: func(c *legendaryContext) {
		if !legendaryMeleeLanded(c) || c.Skill != 10116 || !legendaryMeleePrimary(c) || !swordNativeOnce(c, 0) {
			return
		}
		for _, e := range legendaryMeleeNearbyOthers(c, 150, len(c.Scene.entities)) {
			c.damageVisual("stage-01", e, legendaryMeleeScale(c.Attack(false), int64(c.RollValue("proc_pct", 160))), false, false)
		}
	}})
	registerLegendarySynergyHandler("JK-U03", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill == 10119 && swordLandedCount(c.Owner.ID, c.Skill, c.Hits) > 0 {
			c.RefundMP(specialScale(c.PaidMP, c.RollValue("refund_pct", 60)))
		}
	}})
	registerLegendarySynergyHandler("JK-U05", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill == 10103 && warriorFunctionalLearned(c, 10103) && !c.IsChild && c.Blow.ProcDepth == 0 {
			warriorFunctionalRelease(10103, int64(c.RollValue("duration_ms", 8000)), false)(c)
			swordCleanse(c, false)
		}
	}, Tick: warriorFunctionalTick(10103, false), StatusReceiving: func(c *legendaryContext, d *domain.StatusDef) bool {
		if !warriorFunctionalActive(c, 10103) {
			return true
		}
		// Use the native mixed-payload preservation rules, but retain roots:
		// this power immunizes movement slow only, not immobility.
		block, immobile := d.BlockMove, d.Immobile
		changed, empty := d.StripMovementImpairments()
		d.BlockMove, d.Immobile = block, immobile
		return !changed || !empty || block || immobile != 0
	}})
	registerLegendarySynergyHandler("JK-U09", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Skill != 0 && swordLandedCount(c.Owner.ID, c.Skill, c.Hits) > 0 {
			c.RefundMP(specialScale(c.PaidMP, c.RollValue("refund_pct", 80)))
		}
	}})
	// Equipment-active movement immunity, not a timer or hard-control cleanse.
	registerLegendarySynergyHandler("JK-U08", legendaryHandler{
		StatusReceiving: func(c *legendaryContext, d *domain.StatusDef) bool {
			changed, empty := d.StripMovementImpairments()
			return !changed || !empty
		},
		EquipmentChanged: swordClearMovementImpairments,
	})
}

func swordClearMovementImpairments(c *legendaryContext) {
	if c.Owner.Status == nil {
		return
	}
	changed, removed := c.Owner.Status.StripMovementImpairments()
	if !changed {
		return
	}
	if len(removed) > 0 {
		c.Scene.statusesRemoved(c.Owner, removed)
		return
	}
	// Mixed states remain, but their movement contribution and presentation
	// changed. EquipmentChanged's signature guards this refresh from recursion.
	c.Scene.refreshEntityStats(c.Owner)
	c.Scene.emitAttributesIfPlayer(c.Owner)
	c.Scene.pushStatusSnapshots(c.Owner)
}

func (s *Scene) legendarySwordsmanDisplacementImmune(target *entity.Entity) bool {
	immune := false
	s.forEachLegendary(target, func(c *legendaryContext) {
		if h := legendaryHandlers[c.Power.ID].DisplacementImmune; h != nil && h(c) {
			immune = true
		}
	})
	return immune
}

func swordCleanse(c *legendaryContext, poison bool) {
	if c.Owner.Status == nil {
		return
	}
	removed := c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		if d.BlockMove || d.Control != domain.ControlNone || d.ID == domain.StatusFreeze || d.ID == domain.StatusPetrify {
			return false
		}
		if poison && d.ID == domain.StatusPoison {
			return true
		}
		if d.ID == domain.StatusSlow {
			return true
		}
		for _, a := range d.Affixes {
			if a.Attr == domain.AttrMoveSpeed && a.Value < 0 {
				return true
			}
		}
		return false
	})
	c.Scene.statusesRemoved(c.Owner, removed)
	if len(removed) > 0 {
		c.Visual("stage-01", c.Owner)
	}
}
