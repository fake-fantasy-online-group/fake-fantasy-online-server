package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func init() {
	typifiedReplaceHandler("TY-U01", legendaryHandler{PrepareSkill: typifiedPrepare, CastReleased: typifiedCastReleased})
	typifiedReplaceHandler("TY-U02", legendaryHandler{AfterHit: typifiedAfterHit, HealResolved: typifiedHealResolved, RootFinished: func(c *legendaryContext) { c.DeleteState(0, "restore") }})
	typifiedReplaceHandler("TY-U03", legendaryHandler{PrepareSkill: typifiedPrepare, AfterHit: typifiedAfterHit, HealResolved: typifiedHealResolved})
	typifiedReplaceHandler("TY-U04", legendaryHandler{HealResolved: typifiedHealResolved})
	typifiedReplaceHandler("TY-U05", legendaryHandler{StatusApplied: typifiedStatusApplied, HealResolved: typifiedHealResolved})
	typifiedReplaceHandler("TY-U06", legendaryHandler{PassiveStats: typifiedPassiveStats, AfterHit: typifiedAfterHit, HealResolved: typifiedHealResolved})
	typifiedReplaceHandler("TY-U07", legendaryHandler{PrepareSkill: typifiedPrepare, Teleported: typifiedTeleported})
	typifiedReplaceHandler("TY-U08", legendaryHandler{StatusReceiving: typifiedStatusReceiving})
	typifiedReplaceHandler("TY-U09", legendaryHandler{PassiveStats: typifiedPassiveStats})
	typifiedReplaceHandler("TY-U10", legendaryHandler{ComposeSkill: typifiedPrepare, HealResolved: typifiedHealResolved})
	typifiedReplaceHandler("TY-U11", legendaryHandler{StatusApplied: typifiedStatusApplied})
	typifiedReplaceHandler("TY-U12", legendaryHandler{PrepareSkill: typifiedPrepare, AfterHit: typifiedAfterHit, BeforeDamage: typifiedBeforeDamage})
	typifiedReplaceHandler("TY-U13", legendaryHandler{CastReleased: typifiedCastReleased, EquipmentChanged: typifiedEquipmentChanged})
	typifiedReplaceHandler("TY-U14", legendaryHandler{PrepareSkill: typifiedPrepare, BeforeDamage: typifiedBeforeDamage})
	typifiedReplaceHandler("TY-U15", legendaryHandler{PrepareSkill: typifiedPrepare, AfterHit: typifiedAfterHit, HealResolved: typifiedHealResolved})
	typifiedReplaceHandler("TY-U16", legendaryHandler{PrepareSkill: typifiedPrepare, HealResolved: typifiedHealResolved, EquipmentChanged: typifiedEquipmentChanged})
	typifiedReplaceHandler("TY-U17", legendaryHandler{PrepareSkill: typifiedPrepare, Resurrected: typifiedResurrected})
	typifiedReplaceHandler("TY-U18", legendaryHandler{PrepareSkill: typifiedPrepare, Dispelled: typifiedDispelled, EquipmentChanged: typifiedEquipmentChanged})
	typifiedReplaceHandler("TY-U19", legendaryHandler{PrepareSkill: typifiedPrepare, AfterHit: typifiedAfterHit, BeforeDamage: typifiedBeforeDamage, Tick: typifiedTick})
	typifiedReplaceHandler("TY-U20", legendaryHandler{CastReleased: typifiedCastReleased, AfterIncoming: typifiedAfterIncoming})
	typifiedReplaceHandler("TY-D01", legendaryHandler{PassiveStats: typifiedPassiveStats})
	typifiedReplaceHandler("TY-D02", legendaryHandler{PassiveStats: typifiedPassiveStats})
	typifiedReplaceHandler("TY-D03", legendaryHandler{PassiveStats: typifiedPassiveStats})
	typifiedReplaceHandler("TY-D04", legendaryHandler{AfterHit: typifiedDrinkBlood})
	typifiedReplaceHandler("TY-D05", legendaryHandler{ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D06", legendaryHandler{ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D07", legendaryHandler{ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D08", legendaryHandler{ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D09", legendaryHandler{StatusReceiving: typifiedStatusReceiving, EquipmentChanged: typifiedEquipmentChanged})
	typifiedReplaceHandler("TY-D10", legendaryHandler{ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D11", legendaryHandler{AbsorbIncoming: typifiedAbsorbIncoming})
	typifiedReplaceHandler("TY-D12", legendaryHandler{IncomingHeal: typifiedIncomingHeal, ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D13", legendaryHandler{StatusReceiving: typifiedStatusReceiving})
	typifiedReplaceHandler("TY-D14", legendaryHandler{StatusReceiving: typifiedStatusReceiving})
	typifiedReplaceHandler("TY-D15", legendaryHandler{StatusReceiving: typifiedStatusReceiving})
	typifiedReplaceHandler("TY-D16", legendaryHandler{StatusApplied: typifiedStatusApplied, AfterIncoming: typifiedAfterIncoming, EquipmentChanged: typifiedEquipmentChanged})
	typifiedReplaceHandler("TY-D17", legendaryHandler{ModifyIncoming: typifiedModifyIncoming, EquipmentChanged: typifiedEquipmentChanged})
	typifiedReplaceHandler("TY-D18", legendaryHandler{StatusApplied: typifiedStatusApplied, StatusReceiving: typifiedStatusReceiving})
	typifiedReplaceHandler("TY-D19", legendaryHandler{ModifyIncoming: typifiedModifyIncoming})
	typifiedReplaceHandler("TY-D20", legendaryHandler{StatusReceiving: typifiedStatusReceiving})
}

func typifiedPrepare(c *legendaryContext, d *domain.SkillDef) {
	if d == nil || !d.Kind.Active() {
		return
	}
	m := &d.LegendaryModifiers
	switch c.Power.ID {
	case "TY-U01":
		if !d.LegendaryBasic {
			m.PrepareReductionBP += c.RollValue("prepare_bp", 6000)
		}
	case "TY-U03":
		if !d.LegendaryBasic {
			m.ManaReductionBP += c.RollValue("mana_bp", 3000)
		}
	case "TY-U07":
		if universalHasEffect(d, domain.EffectTeleportPoint, domain.EffectTeleportTarget) {
			d.Dist = universalScaleBP(d.Dist, c.RollValue("move_bp", 12000))
		}
		if typifiedHasWindow(c.Owner, "TY-U07") {
			d.Dist = universalScaleBP(d.Dist, c.RollValue("range_bp", 5000))
		}
	case "TY-U10":
		if universalHealingSkill(d) && ((d.Area.Valid() && (d.Area.Shape == domain.SkillAreaCircle || d.Area.Shape == domain.SkillAreaCone)) || (!d.Area.Valid() && d.Kind == domain.SkillArea && d.Radius > 0)) {
			m.RadiusBP += c.RollValue("radius_bp", 8000)
		}
	case "TY-U12":
		if typifiedHasWindow(c.Owner, "TY-U12") && d.PrepareMS > 0 {
			m.PrepareReductionBP += c.RollValue("prepare_bp", 4000)
		}
		if typifiedHasWindow(c.Owner, "TY-U12") && d.PrepareMS <= 0 {
			m.HealingBP += c.RollValue("power_bp", 2500)
		}
	case "TY-U15":
		if !d.LegendaryBasic {
			m.CooldownReductionBP += c.RollValue("benefit", 4000)
		}
	case "TY-U16":
		m.HealingBP += c.RollValue("benefit", 10000)
	case "TY-U17":
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectResurrect {
				if e.HealPctOfMax < c.RollValue("life_pct", 100) {
					e.HealPctOfMax = c.RollValue("life_pct", 100)
				}
			}
		})
	case "TY-U18":
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.ClearHarmful || len(e.RemoveStatusIDs) > 0 {
				e.DispelChanceBP = min(int32(10000), e.DispelChanceBP+c.RollValue("benefit", 7000))
			}
		})
	case "TY-U19":
		if universalHasEffect(d, domain.EffectDetectTrap) {
			m.RadiusBP += c.RollValue("radius_bp", 15000)
		}
		universalEditEffects(d, func(e *domain.SkillEffect) {
			if e.Kind == domain.EffectRevealInvisible {
				e.RevealRadius = universalScaleBP(e.RevealRadius, c.RollValue("radius_bp", 15000))
			}
		})
	}
}

func typifiedPassiveStats(c *legendaryContext, st *domain.Stats) {
	if st == nil {
		return
	}
	switch c.Power.ID {
	case "TY-D01":
		st.Def = typifiedScaleStat(st.Def, c.RollValue("benefit", 80))
	case "TY-D02":
		st.MDef = typifiedScaleStat(st.MDef, c.RollValue("benefit", 80))
	case "TY-D03":
		st.MaxHP = typifiedScaleStat(st.MaxHP, c.RollValue("benefit", 35))
	case "TY-U06":
		st.MaxMP = typifiedScaleStat(st.MaxMP, c.RollValue("maxmp_pct", 35))
	case "TY-U09":
		_, bare := universalEquipmentContribution(c)
		base := bare.MoveSpeed
		if base <= 0 {
			base = st.MoveSpeed
		}
		st.MoveSpeed = domain.SaturateInt32(int64(st.MoveSpeed) + int64(base)*int64(c.RollValue("benefit", 60))/100)
	}
}

func typifiedStatusApplied(c *legendaryContext, d *domain.StatusDef) {
	if d == nil || d.LegendaryModifiers.Applied {
		return
	}
	switch c.Power.ID {
	case "TY-U05":
		if d.LegendaryPowerID == "" && d.IntervalSec > 0 && universalStatusHeals(*d) && (d.SkillApplied || d.SourceSkillID != 0 || d.SkillSnapshotID != 0) {
			d.LegendaryModifiers.HealingTickBP += c.RollValue("benefit", 10000)
		}
	case "TY-U11":
		if !d.Harmful() && d.LegendaryPowerID == "" && (d.SkillApplied || d.SourceSkillID != 0 || d.SkillSnapshotID != 0) {
			d.LegendaryModifiers.DurationBP += c.RollValue("benefit", 10000)
		}
	case "TY-D16":
		if d.DamageShield > 0 && d.LegendaryPowerID == "" && (d.SkillApplied || d.SourceSkillID != 0 || d.SkillSnapshotID != 0) {
			d.LegendaryModifiers.ShieldBP += c.RollValue("benefit", 10000)
		}
	}
}

func typifiedStatusReceiving(c *legendaryContext, d *domain.StatusDef) bool {
	if d == nil || c.Owner == nil {
		return true
	}
	// legendaryStatusReceiving builds contexts as (recipient, caster).
	// Owner is the wearer receiving the status; Target is the status source.
	selfCast := c.Target != nil && c.Owner.ID == c.Target.ID
	switch c.Power.ID {
	case "TY-U08":
		// Movement cleanse only. Never discard a mixed stun/DoT/shield package.
		if typifiedMovementImpairment(d) && typifiedHasWindow(c.Owner, "TY-U08") {
			changed, empty := d.StripMovementImpairments()
			return !changed || !empty
		}
		if typifiedMovementImpairment(d) && c.Ready(8000) {
			typifiedApplyTimedAffix(c, c.Owner, max(int32(1), c.RollValue("duration_ms", 4000)/1000))
			if c.Owner.Status != nil {
				changed, removed := c.Owner.Status.StripMovementImpairments()
				if len(removed) > 0 {
					c.Scene.statusesRemoved(c.Owner, removed)
				} else if changed {
					c.Scene.refreshEntityStats(c.Owner)
					c.Scene.emitAttributesIfPlayer(c.Owner)
					c.Scene.pushStatusSnapshots(c.Owner)
				}
			}
			changed, empty := d.StripMovementImpairments()
			return !changed || !empty
		}
	case "TY-D09":
		if typifiedControlLike(d) {
			d.LegendaryModifiers.ControlDurationReductionBP += c.RollValue("benefit", 5000)
		}
	case "TY-D13":
		if !selfCast && !d.Harmful() && d.DamageShield > 0 && d.LegendaryPowerID == "" {
			d.LegendaryModifiers.ShieldBP += c.RollValue("benefit", 20000)
			if !typifiedInResourcePath(c) {
				source := domain.EntityID(0)
				if c.Target != nil {
					source = c.Target.ID
				}
				if source == 0 {
					break
				}
				st := c.State(source, "d13-heal")
				if st.Stacks == 0 {
					st.Stacks = 1
					typifiedHeal(c, c.Owner, typifiedPctOf(universalScaleBP(d.DamageShield, d.LegendaryModifiers.ShieldBP), c.RollValue("heal_pct", 20)))
				}
			}
		}
	case "TY-D14":
		if typifiedDefenseLoss(d) {
			if typifiedHasWindow(c.Owner, "TY-D14") || c.Ready(10000) {
				typifiedApplyTimedAffix(c, c.Owner, c.RollValue("duration_ms", 8000)/1000)
				return false
			}
		}
	case "TY-D15":
		if typifiedAttackSlow(d) {
			return false
		}
		if (typifiedControlLike(d) || typifiedMoveLimit(d)) && c.Ready(8000) {
			spd := c.RollValue("speed_pct", 30)
			typifiedApplyTimedAffix(c, c.Owner, c.RollValue("duration_ms", 5000)/1000,
				domain.Affix{Attr: domain.AttrMoveSpeed, Value: spd, Mode: domain.ModePercent},
				domain.Affix{Attr: domain.AttrAtkSpeed, Value: -spd, Mode: domain.ModePercent})
		}
	case "TY-D18":
		if d.DamageShield > 0 {
			d.LegendaryModifiers.DurationBP += c.RollValue("benefit", 10000)
		}
	case "TY-D20":
		if typifiedPoisonOrBurn(d) {
			d.LegendaryModifiers.ControlDurationReductionBP += c.RollValue("benefit", 6000)
			if c.Ready(10000) {
				if c.Owner.Status != nil {
					cleared := 0
					removed := c.Owner.Status.RemoveWhere(func(existing domain.StatusDef) bool {
						if cleared >= 1 {
							return false
						}
						if typifiedPoisonOrBurn(&existing) {
							cleared++
							return true
						}
						return false
					})
					c.Scene.statusesRemoved(c.Owner, removed)
				}
				typifiedApplyTimedAffix(c, c.Owner, c.RollValue("duration_ms", 5000)/1000)
				return false
			}
		}
	}
	return true
}
func typifiedAbsorbIncoming(c *legendaryContext, n int32) int32 {
	if n <= 0 || c.Owner == nil || typifiedInResourcePath(c) {
		return n
	}
	if c.Target != nil && (c.Owner.ID == c.Target.ID || sameParty(c.Owner, c.Target)) {
		return n
	}
	if c.Power.ID != "TY-D11" {
		return n
	}
	pct := c.RollValue("benefit", 4500)
	if c.Owner.MaxHP > 0 && int64(c.Owner.HP)*100 < int64(c.Owner.MaxHP)*30 {
		pct = c.RollValue("low_bp", 7000)
	}
	taken := min(max(int32(0), c.Owner.MP), domain.SaturateInt32(int64(n)*int64(pct)/10000))
	if taken <= 0 {
		return n
	}
	defer c.Scene.withCombatCausalPath(append(append([]string{}, c.CausalPath...), typifiedResourcePath(c.Power.ID)...))()
	c.Owner.MP -= taken
	c.Scene.emitAttributesIfPlayer(c.Owner)
	return n - taken
}
