package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func universalNativeStatus(c *legendaryContext, d *domain.StatusDef) {
	if d == nil || d.LegendaryModifiers.Applied || d.LegendaryPowerID != "" || (!d.SkillApplied && d.SkillSnapshotID == 0 && d.SourceSkillID == 0) {
		return
	}
	switch c.Power.ID {
	case "TY-A18":
		if d.ID == domain.StatusPoison && d.IntervalSec > 0 {
			d.LegendaryModifiers.DamageTickBP += c.RollValue("benefit", universalTuning.PoisonTickBP)
		}
	case "TY-A27":
		if d.ID == 1036 && d.IntervalSec > 0 {
			d.LegendaryModifiers.DamageTickBP += c.RollValue("benefit", universalTuning.BurnTickBP)
		}
	case "TY-D16":
		if d.DamageShield > 0 {
			d.LegendaryModifiers.ShieldBP += c.RollValue("benefit", universalTuning.ShieldStrengthBP)
		}
	case "TY-U05":
		if d.IntervalSec > 0 && universalStatusHeals(*d) {
			d.LegendaryModifiers.HealingTickBP += c.RollValue("benefit", universalTuning.HealingTickBP)
		}
	}
}
func universalStatusHeals(d domain.StatusDef) bool {
	for _, v := range d.HPChanges {
		if v.Value > 0 {
			return true
		}
	}
	for _, v := range d.Affixes {
		if v.Attr == domain.AttrHPRegen && v.Value > 0 {
			return true
		}
	}
	return false
}
func (s *Scene) legendaryInheritStatusModifiers(skill domain.SkillDef, d *domain.StatusDef) {
	bp := skill.LegendaryModifiers.StatusPowerBP
	if d == nil || bp == 0 || d.Harmful() {
		return
	}
	d.LegendaryModifiers.ShieldBP += bp
	d.LegendaryModifiers.BuffMagnitudeBP += bp
	if d.IntervalSec > 0 && universalStatusHeals(*d) {
		d.LegendaryModifiers.HealingTickBP += bp
	}
}
func (s *Scene) legendaryFinalizeStatus(d *domain.StatusDef) {
	if d == nil || d.LegendaryModifiers.Applied {
		return
	}
	m := &d.LegendaryModifiers
	m.Applied = true
	if (d.DurationSec > 0 || d.LegendaryDurationMS > 0) && (m.DurationBP != 0 || m.ControlDurationReductionBP != 0 || m.DurationBonusMS != 0) {
		base := d.LegendaryDurationMS
		if base <= 0 {
			base = domain.SaturateInt32(int64(d.DurationSec) * 1000)
		}
		base = domain.SaturateInt32(int64(base) + int64(m.DurationBonusMS))
		d.LegendaryDurationMS = max(int32(100), universalScaleBP(base, max(int32(-10000), m.DurationBP-m.ControlDurationReductionBP)))
		d.DurationSec = (d.LegendaryDurationMS + 999) / 1000
	}
	if m.LifeStealBP != 0 {
		d.LifeStealPct = universalScaleBP(d.LifeStealPct, m.LifeStealBP)
	}
	if m.PoisonChancePct != 0 && d.PoisonOnHitChance > 0 {
		d.PoisonOnHitChance = min(int32(100), d.PoisonOnHitChance+m.PoisonChancePct)
	}
	if m.EffectHitBonusBP != 0 {
		d.SourceEffectHitBP += m.EffectHitBonusBP
		d.SourceEffectHitKnown = true
	}
	if d.DamageShield > 0 {
		d.DamageShield = universalScaleBP(d.DamageShield, m.ShieldBP)
	}
	legendaryScaleNativeBuffFields(d, m.BuffMagnitudeBP)
	d.PhysicalDamageReductionPct = domain.SaturateInt32(int64(d.PhysicalDamageReductionPct) + int64(m.PhysicalReductionBonusPct))
	d.MagicDamageReductionPct = domain.SaturateInt32(int64(d.MagicDamageReductionPct) + int64(m.MagicReductionBonusPct))
	d.Affixes = append([]domain.Affix(nil), d.Affixes...)
	for i := range d.Affixes {
		a := &d.Affixes[i]
		if a.Value < 0 && (a.Attr == domain.AttrDef || a.Attr == domain.AttrMDef) && m.DefenseLossReductionBP > 0 {
			a.Value = universalScaleBP(a.Value, -min(int32(10000), m.DefenseLossReductionBP))
		}
		if a.Attr == domain.AttrAtkSpeed && a.Value > 0 && m.AttackSlowReductionBP > 0 {
			a.Value = universalScaleBP(a.Value, -min(int32(10000), m.AttackSlowReductionBP))
		}
		if a.Attr == domain.AttrMoveSpeed && a.Value < 0 && (m.SlowMagnitudeBP != 0 || m.SlowMagnitudeReductionBP != 0) {
			a.Value = universalScaleBP(a.Value, max(int32(-10000), m.SlowMagnitudeBP-m.SlowMagnitudeReductionBP))
			if a.Mode == domain.ModePercent {
				a.Value = max(int32(-99), a.Value)
			}
		} else if m.BuffMagnitudeBP > 0 && (a.Value > 0 && a.Attr != domain.AttrHPRegen || !d.Harmful() && a.Attr == domain.AttrAtkSpeed && a.Value < 0) {
			// Native haste uses a negative attack-interval modifier. It is a
			// benefit, unlike negative defense/movement penalties on other buffs.
			a.Value = universalScaleBP(a.Value, m.BuffMagnitudeBP)
		}
	}
	if m.MoveSpeedBonusPct != 0 {
		d.Affixes = append(d.Affixes, domain.Affix{Attr: domain.AttrMoveSpeed, Value: m.MoveSpeedBonusPct, Mode: domain.ModePercent})
	}
}
func (s *Scene) legendaryPeriodicAmount(a domain.Active, delta int64) int64 {
	n, _ := s.legendaryPeriodicAmountResult(a, delta)
	return n
}
func (s *Scene) legendaryPeriodicAmountResult(a domain.Active, delta int64) (int64, bool) {
	crit := false
	if a.Def.LegendaryPowerID != "" {
		return delta, false
	}
	bp := a.Def.LegendaryModifiers.HealingTickBP
	if delta < 0 {
		bp = a.Def.LegendaryModifiers.DamageTickBP
	}
	delta = delta * (10000 + int64(bp)) / 10000
	if delta < 0 && a.Def.LegendaryModifiers.PeriodicCanCrit && s.rng.Intn(10000) < int(min(int32(10000), a.Def.LegendaryModifiers.PeriodicCritChanceBP)) {
		crit = true
		delta = delta * (20000 + int64(a.Def.LegendaryModifiers.PeriodicCritBonusBP)) / 10000
	}
	return delta, crit
}
func universalNativeStatusReceiving(c *legendaryContext, d *domain.StatusDef) bool {
	if c.Owner == nil || c.Target == nil || c.Owner == c.Target || sameParty(c.Owner, c.Target) || d == nil || !d.Harmful() {
		return true
	}
	switch c.Power.ID {
	case "TY-D09":
		if d.Control != domain.ControlNone || d.BlockMove {
			d.LegendaryModifiers.ControlDurationReductionBP += c.RollValue("benefit", universalTuning.ControlDurationReductionBP)
		}
	case "TY-U08":
		for _, a := range d.Affixes {
			if a.Attr == domain.AttrMoveSpeed && a.Value < 0 {
				d.LegendaryModifiers.SlowMagnitudeReductionBP += c.RollValue("benefit", universalTuning.SlowMagnitudeReductionBP)
				break
			}
		}
	}
	return true
}
func universalSynergyIncoming(c *legendaryContext, n int32) int32 {
	if n <= 0 || c.Owner == nil || c.Target == nil || c.Owner == c.Target || sameParty(c.Owner, c.Target) {
		return n
	}
	switch c.Power.ID {
	case "TY-D05":
		if c.Blow.Area && c.Blow.Origin != domain.DamageDOTOrigin {
			return universalScaleBP(n, -c.RollValue("benefit", universalTuning.AreaMitigationBP))
		}
	case "TY-D19":
		if c.Blow.Origin == domain.DamageDOTOrigin {
			return universalScaleBP(n, -c.RollValue("benefit", universalTuning.PeriodicMitigationBP))
		}
	case "TY-D11":
		taken := min(max(int32(0), c.Owner.MP), domain.SaturateInt32(int64(n)*int64(c.RollValue("benefit", universalTuning.ManaAbsorbBP))/10000))
		c.Owner.MP -= taken
		if taken > 0 {
			c.Scene.emitAttributesIfPlayer(c.Owner)
		}
		return n - taken
	}
	return n
}
func (s *Scene) universalPeriodicIncomingContexts(src, dst *entity.Entity, sourceID ...domain.EntityID) []legendaryContext {
	if src == nil && len(sourceID) > 0 && sourceID[0] != 0 {
		src = &entity.Entity{ID: sourceID[0]}
	}
	if src == nil || dst == nil || src.ID == dst.ID || sameParty(src, dst) {
		return nil
	}
	return s.legendaryIncomingContexts(src, dst, combat.Blow{Magic: true, Origin: domain.DamageDOTOrigin}, event.DamageDealt{Src: src.ID, Dst: dst.ID, Origin: domain.DamageDOTOrigin, Flag: event.DamageMagic})
}
func universalPeriodicReduce(contexts []legendaryContext, n int32, critical ...bool) int32 {
	// Match the direct incoming additive bucket. Periodic critical mitigation
	// is a value consumer, never a new strike or a critical roll.
	delta := int64(0)
	for _, c := range contexts {
		if c.Power.ID == "TY-D19" {
			delta += int64(universalSynergyIncoming(&c, n)) - int64(n)
		}
		if c.Power.ID == "TY-D08" || c.Power.ID == "TY-D17" {
			delta += int64(universalExtendedIncoming(&c, n)) - int64(n)
		}
		if c.Power.ID == "TY-D10" && len(critical) > 0 && critical[0] {
			delta += int64(universalScaleBP(n, -c.RollValue("benefit", 3000))) - int64(n)
		}
	}
	return max(int32(0), domain.SaturateInt32(int64(n)+delta))
}
func universalPeriodicAbsorb(contexts []legendaryContext, n int32) int32 {
	for _, c := range contexts {
		if c.Power.ID == "TY-D11" {
			n = universalSynergyIncoming(&c, n)
		}
	}
	return n
}

// Keep the standalone adapter; the real tick boundary inserts all-damage pure
// reductions between the two phases before spending mana and absorbing shields.
func (s *Scene) universalPeriodicIncoming(src, dst *entity.Entity, n int32, sourceID ...domain.EntityID) int32 {
	contexts := s.universalPeriodicIncomingContexts(src, dst, sourceID...)
	return universalPeriodicAbsorb(contexts, universalPeriodicReduce(contexts, n))
}
func universalBasicMana(c *legendaryContext) {
	if !c.IsBasic || c.IsChild || c.RootID == 0 || c.Blow.ProcDepth != 0 || c.Blow.Origin != domain.DamageNormalOrigin || c.Hit.Flag.Has(event.DamageMiss) || c.Owner == nil || c.Target == nil || c.Owner == c.Target || sameParty(c.Owner, c.Target) {
		return
	}
	st := c.State(0, "basic-root")
	if st.RootID == c.RootID {
		return
	}
	st.RootID = c.RootID
	c.Owner.MP = min(c.Owner.MaxMP, c.Owner.MP+c.RollValue("benefit", universalTuning.BasicManaFlat))
	c.Scene.emitAttributesIfPlayer(c.Owner)
}
