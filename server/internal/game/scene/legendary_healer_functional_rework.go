package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

const legendaryHealerEscortStatus domain.StatusID = 1811000002

// Window eligibility uses invested points, not equipment-granted skill ranks.
func legendaryHealerInvested(c *legendaryContext, skill domain.SkillID) bool {
	return c.Owner != nil && c.Owner.Alive() && c.Owner.Player != nil && c.Owner.Player.Char != nil && c.Owner.Player.Char.Skills[skill] > 0
}

func legendaryHealerCleanseEscort() legendaryHandler {
	return legendaryHandler{
		RootFinished: func(c *legendaryContext) {
			if c.Skill != 10306 || c.IsChild || c.Blow.ProcDepth != 0 || !legendaryHealerInvested(c, 10306) || !legendaryCasterOnce(c, 0, "cleanse-escort") {
				return
			}
			// One source-bound native status refreshes both effects together. It
			// expires normally and the existing equipment prune removes it at once.
			c.Scene.applyStatusDef(c.Owner, domain.StatusDef{
				ID: legendaryHealerEscortStatus, Level: 1, Name: c.Power.Name, Desc: c.Power.Effect,
				Icon: legendaryCasterIcon(c), IconKnown: true, TypeKnown: true, Type: domain.StatusGood,
				DurationSec: 8, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
				Affixes:                    []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: c.RollValue("move_pct", 60)}},
				PhysicalDamageReductionPct: 25, MagicDamageReductionPct: 25,
			}, c.Owner.ID)
		},
		Tick:             legendaryHealerPruneEscort,
		EquipmentChanged: legendaryHealerPruneEscort,
	}
}

func legendaryHealerPruneEscort(c *legendaryContext) {
	if legendaryHealerInvested(c, 10306) || c.Owner == nil || c.Owner.Status == nil {
		return
	}
	removed := c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		return d.ID == legendaryHealerEscortStatus && d.LegendaryPowerID == c.Power.ID && d.LegendarySourceUID == c.SourceUID
	})
	if len(removed) > 0 {
		c.Scene.statusesRemoved(c.Owner, removed)
		c.Scene.refreshEntityStats(c.Owner)
		c.Scene.pushStatusSnapshots(c.Owner)
	}
}

func legendaryHealerFarClinic() legendaryHandler {
	return legendaryHandler{
		CastReleased: func(c *legendaryContext) {
			if c.Skill != 10327 || c.IsChild || c.Blow.ProcDepth != 0 || !legendaryHealerInvested(c, 10327) {
				return
			}
			st := c.State(0, "far-clinic")
			st.ExpiresMS = c.NowMS + 12000
			st.Name, st.Description, st.IconID, st.Good = c.Power.Name, c.Power.Effect, legendaryCasterIcon(c), true
		},
		PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
			if legendaryHealerInvested(c, 10327) && c.State(0, "far-clinic").ExpiresMS > c.NowMS &&
				(d.TargetSelf || d.TargetTeam) && !d.TargetEnemy && d.Area.Shape == domain.SkillAreaNone {
				d.Dist += c.RollValue("range", 400)
				d.LegendaryModifiers.ManaReductionBP += 4500
			}
			if d.ID == 10327 {
				d.LegendaryRevealPoint = true
				d.Dist = 600
			}
		},
		Tick: func(c *legendaryContext) {
			if !legendaryHealerInvested(c, 10327) {
				c.DeleteState(0, "far-clinic")
			}
		},
	}
}

func legendaryHealerRejuvenationGuard() legendaryHandler {
	return legendaryHandler{AllDamageIncoming: true, BeforeIncoming: func(c *legendaryContext, n int32) int32 {
		if legendaryHealerInvested(c, 10303) && legendaryCasterHasSkill(c.Owner, 10303) {
			return specialScale(n, 100-c.RollValue("reduction_pct", 25))
		}
		return n
	}}
}
