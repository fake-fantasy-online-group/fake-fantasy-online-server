package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Utility windows are refreshed by learned manual casts, never stacked.
func init() {
	for _, id := range []string{"CK-U01", "CK-U02", "CK-U03", "CK-U04", "CK-U07", "CK-U09"} {
		registerLegendarySynergyHandler(id, legendaryHandler{TransformSkill: legendaryAssassinUtilityTransform, PrepareSkill: legendaryAssassinUtilityPrepare, CastReleased: legendaryAssassinUtilityRelease, PassiveStats: legendaryAssassinUtilityStats, Tick: legendaryAssassinUtilityTick})
	}
	registerLegendarySynergyHandler("CK-A15", legendaryHandler{StatusApplied: legendaryAssassinTrapPoison})
}
func legendaryAssassinUtilityTransform(c *legendaryContext, d *domain.SkillDef) {
	if c.Power.ID == "CK-U02" && d.ID == 10224 {
		d.Kind = domain.SkillArea
		d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: max(int32(100), d.Radius)}
	}
}
func legendaryAssassinUtilityPrepare(c *legendaryContext, d *domain.SkillDef) {
	switch c.Power.ID {
	case "CK-U03":
		if d.ID == 10218 {
			d.MPCost = 0
			d.ManaCostFractionNumerator = 0
			d.ManaCostFractionDenominator = 0
		}
	case "CK-U01":
		if legendaryAssassinWindow(c, 10208) {
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 4500)
		}
		if d.ID == 10208 {
			d.LegendaryModifiers.ManaReductionBP += 3000
		}
	case "CK-U02":
		if d.ID == 10224 {
			d.Dist = specialScale(d.Dist, 100+c.RollValue("range_pct", 50))
			d.LegendaryModifiers.RadiusBP += 3000
		}
	case "CK-U04":
		if d.ID == 10228 {
			d.LegendaryModifiers.RadiusBP += 5000
		}
	case "CK-U07":
		if legendaryAssassinTrapSkill(d.ID) && legendaryAssassinWindow(c, 0) {
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 5500)
		}
	case "CK-U09":
		if d.ID == 10221 || d.ID == 10219 {
			d.LegendaryModifiers.ManaReductionBP += c.RollValue("mana_bp", 3000)
			d.LegendaryModifiers.CooldownReductionBP += 2000
		}
	}
}
func legendaryAssassinLearned(c *legendaryContext, skill domain.SkillID) bool {
	return c.Owner != nil && c.Owner.Alive() && c.Owner.Player != nil && c.Owner.Player.Char != nil && c.Owner.Player.Char.Skills[skill] > 0
}
func legendaryAssassinWindow(c *legendaryContext, skill domain.SkillID) bool {
	st := c.State(0, "utility-window")
	if skill == 0 {
		skill = domain.SkillID(st.Value)
	}
	if !legendaryAssassinLearned(c, skill) {
		c.DeleteState(0, "utility-window")
		return false
	}
	return st.ExpiresMS > c.NowMS
}
func legendaryAssassinUtilityRelease(c *legendaryContext) {
	if c.IsChild || c.Blow.ProcDepth > 0 || !legendaryAssassinLearned(c, c.Skill) {
		return
	}
	if c.Power.ID == "CK-U01" && c.Skill == 10208 || c.Power.ID == "CK-U04" && c.Skill == 10228 || c.Power.ID == "CK-U07" && legendaryAssassinTrapSkill(c.Skill) {
		st := c.State(0, "utility-window")
		st.ExpiresMS, st.Value = c.NowMS+10000, int64(c.Skill)
		if c.Power.ID == "CK-U04" {
			st.Stacks = 1
			c.Scene.refreshStats(c.Owner)
		}
	}
}
func legendaryAssassinUtilityStats(c *legendaryContext, st *domain.Stats) {
	if c.Power.ID == "CK-U04" && legendaryAssassinWindow(c, 10228) {
		st.MoveSpeed = specialScale(st.MoveSpeed, 100+c.RollValue("move_pct", 60))
	}
}
func legendaryAssassinUtilityTick(c *legendaryContext) {
	if c.Power.ID != "CK-U04" {
		return
	}
	st := c.State(0, "utility-window")
	if st.Stacks != 0 && !legendaryAssassinWindow(c, 10228) {
		st.Stacks = 0
		c.Scene.refreshStats(c.Owner)
	}
}
func legendaryAssassinTrapPoison(c *legendaryContext, d *domain.StatusDef) {
	if d.ID == domain.StatusPoison && (d.SourceSkillID == 10214 || d.SkillSnapshotID == 10214) {
		d.LegendaryModifiers.DamageTickBP += c.RollValue("tick_bp", 10000)
	}
}

func init() {
	registerLegendarySynergyHandler("CK-U08", legendaryHandler{StatusApplied: legendaryAssassinStealthBudget, PrepareSkill: legendaryAssassinStealthTrapPrepare, CastAccepted: legendaryAssassinStealthTrapAccepted})
}
func legendaryAssassinStealthBudget(c *legendaryContext, d *domain.StatusDef) {
	if d.Invisible && (d.SourceSkillID == 10208 || d.SkillSnapshotID == 10208) {
		d.LegendaryStealthTrapCharges = 0
		d.LegendaryModifiers.DurationBonusMS += c.RollValue("duration_ms", 5000)
	}
}
func legendaryAssassinStealthTrapPrepare(c *legendaryContext, d *domain.SkillDef) {
	if !legendaryAssassinTrapSkill(d.ID) || c.Owner.Status == nil || !legendaryAssassinLearned(c, 10208) {
		return
	}
	c.Owner.Status.EachEffective(func(a domain.Active) {
		if a.Def.Invisible && (a.Def.SourceSkillID == 10208 || a.Def.SkillSnapshotID == 10208) && a.ExpireAt > c.Scene.tick {
			d.PreserveInvisibleChanceBP = 10000
		}
	})
}
func legendaryAssassinStealthTrapAccepted(c *legendaryContext) {}
