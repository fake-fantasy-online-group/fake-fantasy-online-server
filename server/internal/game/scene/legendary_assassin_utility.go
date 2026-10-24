package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Static reductions use the shared additive buckets and global caps. Defaults
// for newly unspecified balance values are30%, not temporary proc rewards.
func init() {
	for _, id := range []string{"CK-U01", "CK-U02", "CK-U03", "CK-U04", "CK-U07", "CK-U09"} {
		registerLegendarySynergyHandler(id, legendaryHandler{TransformSkill: legendaryAssassinUtilityTransform, PrepareSkill: legendaryAssassinUtilityPrepare, CastReleased: legendaryAssassinUtilityRelease, PassiveStats: legendaryAssassinUtilityStats})
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
		if d.ID == 10208 {
			d.LegendaryModifiers.ManaReductionBP += 3000
		}
	case "CK-U02":
		if d.ID == 10224 {
			d.Dist = specialScale(d.Dist, 150)
			d.LegendaryModifiers.RadiusBP += 3000
		}
	case "CK-U04":
		if d.ID == 10228 {
			d.LegendaryModifiers.RadiusBP += 5000
		}
	case "CK-U07":
		if legendaryAssassinTrapSkill(d.ID) {
			d.LegendaryModifiers.ManaReductionBP += 3000
		}
	case "CK-U09":
		if d.ID == 10221 || d.ID == 10219 {
			d.LegendaryModifiers.ManaReductionBP += 3000
			d.LegendaryModifiers.CooldownReductionBP += 2000
		}
	}
}
func legendaryAssassinUtilityRelease(c *legendaryContext) {
	if c.Power.ID == "CK-U01" && c.Skill == 10208 {
		c.RefundMP(clampI32(int64(c.Owner.MaxMP) * 5 / 100))
	}
}
func legendaryAssassinUtilityStats(c *legendaryContext, st *domain.Stats) {
	if c.Power.ID == "CK-U04" {
		st.MoveSpeed = specialScale(st.MoveSpeed, 120)
	}
}
func legendaryAssassinTrapPoison(c *legendaryContext, d *domain.StatusDef) {
	if d.ID == domain.StatusPoison && (d.SourceSkillID == 10214 || d.SkillSnapshotID == 10214) {
		d.LegendaryModifiers.DamageTickBP += 10000
	}
}

func init() {
	registerLegendarySynergyHandler("CK-U08", legendaryHandler{StatusApplied: legendaryAssassinStealthBudget, PrepareSkill: legendaryAssassinStealthTrapPrepare, CastAccepted: legendaryAssassinStealthTrapAccepted})
}
func legendaryAssassinStealthBudget(c *legendaryContext, d *domain.StatusDef) {
	if d.Invisible && (d.SourceSkillID == 10208 || d.SkillSnapshotID == 10208) {
		d.LegendaryStealthTrapCharges = 2
		d.LegendaryModifiers.DurationBonusMS += 5000
	}
}
func legendaryAssassinStealthTrapPrepare(c *legendaryContext, d *domain.SkillDef) {
	if !legendaryAssassinTrapSkill(d.ID) || c.Owner.Status == nil {
		return
	}
	c.Owner.Status.EachEffective(func(a domain.Active) {
		if a.Def.Invisible && a.Def.LegendaryStealthTrapCharges > 0 && a.ExpireAt > c.Scene.tick {
			d.PreserveInvisibleChanceBP = 10000
		}
	})
}
func legendaryAssassinStealthTrapAccepted(c *legendaryContext) {
	if legendaryAssassinTrapSkill(c.Skill) && c.Owner.Status != nil {
		c.Owner.Status.ConsumeLegendaryStealthTrap()
		c.Scene.pushStatusSnapshots(c.Owner)
	}
}
