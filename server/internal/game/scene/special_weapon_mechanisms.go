package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
)

// Native templates remain inert. The difficulty registry is the sole authority
// for deciding which worn template carries the approved enhanced mechanics.
func (s *Scene) specialWeapon(p *entity.Entity) domain.ItemID {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return 0
	}
	worn := s.functionalWorn(p)
	if worn == nil {
		return 0
	}
	for _, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotTwoHand} {
		st := worn.At(slot)
		def, ok := s.itemDef(st.Item)
		if !ok || !domain.EquipmentFunctional(st, def) {
			continue
		}
		if id := domain.EnhancedWeaponBaseID(st.Item); id != 0 {
			return id
		}
	}
	return 0
}

func specialScale(v, pct int32) int32 { return domain.SaturateInt32(int64(v) * int64(pct) / 100) }
func specialCooldown(d *domain.SkillDef, pct int32) {
	d.CooldownMS = specialScale(d.CooldownMS, pct)
	d.CooldownFractionNumeratorMS *= int64(pct)
	if d.CooldownFractionDenominator > 0 {
		d.CooldownFractionDenominator *= 100
	}
	if pct == 0 {
		d.CooldownFractionNumeratorMS = 0
		d.CooldownFractionDenominator = 0
	}
}
func specialMana(d *domain.SkillDef, pct int32) {
	d.MPCost = specialScale(d.MPCost, pct)
	d.ManaCostFractionNumerator = d.ManaCostFractionNumerator * int64(pct) / 100
}
func specialDamage(d *domain.SkillDef, pct int32) { scaleVariantDamage(d, int64(pct)) }
func specialRadius(d *domain.SkillDef, pct int32) {
	d.Radius = specialScale(d.Radius, pct)
	d.Area.Radius = specialScale(d.Area.Radius, pct)
	for i := range d.Effects {
		d.Effects[i].TrapRadius = specialScale(d.Effects[i].TrapRadius, pct)
	}
}
func specialWithout(d *domain.SkillDef, kind domain.EffectKind) {
	out := make([]domain.SkillEffect, 0, len(d.Effects))
	for _, e := range d.Effects {
		if e.Kind != kind {
			out = append(out, e)
		}
	}
	d.Effects = out
}
func specialCircle(d *domain.SkillDef, radius int32, center domain.SkillAreaCenter, limit int32) {
	d.Kind = domain.SkillArea
	d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: center, Radius: radius}
	d.Radius = radius
	d.SpecialWeaponTargetLimit = limit
}

// This function is pure: copied skill definitions can be used for both UI
// contracts and accepted casts without mutating the learned/static catalogue.
func applySpecialWeaponDefinition(base domain.ItemID, d *domain.SkillDef) {
	if d == nil || !specialWeaponSkillMatches(base, d.ID, d.Prof) {
		return
	}
	d.SpecialWeaponBaseID = base
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	d.Statuses = append([]domain.StatusApplication(nil), d.Statuses...)
	switch base {
	case 1021:
		specialWithout(d, domain.EffectKnockback)
		specialDamage(d, 130)
	case 1022:
		specialCircle(d, 100, domain.SkillAreaCenterCaster, 3)
		specialDamage(d, 80)
	case 1023:
		specialDamage(d, 90)
		for i := range d.Effects {
			if d.Effects[i].Kind == domain.EffectRestoreHP {
				d.Effects[i].SourceAttackPct = specialScale(d.Effects[i].SourceAttackPct, 160)
				domain.ApplySkillHealingBP(&d.Effects[i], 6000)
			}
		}
	case 1024:
		specialCooldown(d, 40)
	case 1025:
		specialCooldown(d, 30)
	case 1026:
		specialCooldown(d, 60)
	case 1520:
		d.Dist = specialScale(d.Dist, 140)
		specialDamage(d, 115)
	case 1521:
		specialCircle(d, 100, domain.SkillAreaCenterTarget, 3)
		specialDamage(d, 65)
		specialCooldown(d, 50)
	case 1523:
		specialDamage(d, 135)
		for i := range d.Effects {
			if d.Effects[i].Kind == domain.EffectRestoreMP {
				d.Effects[i].SourceAttackPct = specialScale(d.Effects[i].SourceAttackPct, 50)
				d.Effects[i].RestoreMPFlat = specialScale(d.Effects[i].RestoreMPFlat, 50)
			}
		}
	case 1524:
		specialRadius(d, 135)
		specialDamage(d, 90)
	case 1526:
		specialRadius(d, 160)
	case 1527:
		specialWithout(d, domain.EffectInstantDeath)
	case 1620:
		d.Area.Width = specialScale(d.Area.Width, 160)
		specialDamage(d, 110)
	case 1621:
		d.Dist = specialScale(d.Dist, 140)
		specialDamage(d, 115)
	case 1623:
		d.Area.Angle = min(int32(360), specialScale(d.Area.Angle, 150))
		if d.Area.Angle == 360 {
			specialCircle(d, d.Area.Radius, domain.SkillAreaCenterCaster, 0)
		}
	case 1625:
		d.Area.Width = specialScale(d.Area.Width, 200)
		d.Area.Length = specialScale(d.Area.Length, 80)
	case 1626:
		if d.ID == 10027 {
			specialCooldown(d, 35)
		}
	case 1627:
		specialRadius(d, 80)
		specialDamage(d, 135)
	case 1821:
		if d.PassiveCounter != nil {
			counter := *d.PassiveCounter
			counter.DamagePct = specialScale(counter.DamagePct, 150)
			d.PassiveCounter = &counter
		}
	case 1820:
		specialCooldown(d, 25)
		specialDamage(d, 70)
	case 1822:
		d.Dist = specialScale(d.Dist, 120)
	case 1825:
		if d.ID == 10224 {
			d.RequiredWeaponType = 8
			specialCircle(d, 120, domain.SkillAreaCenterCaster, 5)
			specialCooldown(d, 150)
		}
	case 1826:
		d.RequiresInvisible = false
		specialDamage(d, 65)
	case 1827:
		specialDamage(d, 135)
		specialCooldown(d, 120)
	case 1922:
		specialCooldown(d, 25)
		specialDamage(d, 70)
	case 1923:
		specialDamage(d, 130)
		specialMana(d, 120)
	case 1924:
		d.Dist = specialScale(d.Dist, 150)
		specialRadius(d, 120)
	case 1925:
		specialWithout(d, domain.EffectKnockback)
		specialDamage(d, 130)
	case 1926:
		r := d.Area.Radius
		if r <= 0 {
			r = d.Radius
		}
		specialCircle(d, specialScale(r, 80), domain.SkillAreaCenterCaster, 0)
		specialDamage(d, 120)
	case 1927:
		specialCircle(d, 150, domain.SkillAreaCenterTarget, 5)
		specialCooldown(d, 150)
	case 1928:
		out := d.Statuses[:0]
		for _, a := range d.Statuses {
			if a.ID != domain.StatusFreeze {
				out = append(out, a)
			}
		}
		d.Statuses = out
		specialDamage(d, 140)
		specialCooldown(d, 70)
	case 1929:
		specialCooldown(d, 75)
		specialMana(d, 80)
	case 1320:
		if d.Prof == 4 {
			for i := range d.Effects {
				if d.Effects[i].Kind == domain.EffectHeal {
					domain.ApplySkillHealingBP(&d.Effects[i], 3000)
				}
			}
			specialMana(d, 110)
		} else {
			specialDamage(d, 130)
		}
	case 1321:
		if d.Prof == 4 {
			specialCircle(d, 150, domain.SkillAreaCenterTarget, 3)
			specialMana(d, 200)
			specialCooldown(d, 150)
		} else {
			specialRadius(d, 140)
			specialDamage(d, 90)
		}
	case 1324:
		if d.Prof == 4 {
			d.Area.Center = domain.SkillAreaCenterTarget
			d.Dist = 300
			d.TargetTeam = true
		}
	case 1326:
		if d.Prof == 5 {
			specialDamage(d, 125)
			specialRadius(d, 120)
		}
	case 1327:
		specialCooldown(d, 0)
		if d.Prof == 5 {
			specialMana(d, 130)
			for i := range d.Effects {
				if d.Effects[i].Kind == domain.EffectTeleportPoint || d.Effects[i].Kind == domain.EffectTeleportTarget {
					d.Effects[i].ChanceBP = 10000
					d.Effects[i].ChanceExplicit = true
				}
			}
		}
	}
	if len(d.Effects) > 0 {
		d.Effect = d.Effects[0]
	}
}

func (s *Scene) applySpecialWeaponModifiers(p *entity.Entity, d *domain.SkillDef, hard domain.Learned) {
	if p == nil || p.Player == nil || hard[d.ID] <= 0 {
		return
	}
	applySpecialWeaponDefinition(s.specialWeapon(p), d)
}
func (s *Scene) limitSpecialWeaponTargets(src *entity.Entity, d domain.SkillDef, ids []domain.EntityID, primary domain.EntityID) []domain.EntityID {
	if d.SpecialWeaponTargetLimit <= 0 || len(ids) <= int(d.SpecialWeaponTargetLimit) {
		return ids
	}
	sort.SliceStable(ids, func(i, j int) bool {
		if ids[i] == primary {
			return true
		}
		if ids[j] == primary {
			return false
		}
		a, b := s.entities[ids[i]], s.entities[ids[j]]
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		da, db := sqDist(src.Pos, a.Pos), sqDist(src.Pos, b.Pos)
		if da == db {
			return ids[i] < ids[j]
		}
		return da < db
	})
	return ids[:d.SpecialWeaponTargetLimit]
}
func specialWeaponStatus(d domain.SkillDef, status *domain.StatusDef) {
	if d.SpecialWeaponBaseID == 0 && d.SpecialWeaponStatusPowerPct == 0 {
		return
	}
	status.Affixes = append([]domain.Affix(nil), status.Affixes...)
	status.SpecialWeaponSourceBase = d.SpecialWeaponBaseID
	if d.SpecialWeaponBaseID == 1326 && d.ID == 10322 {
		status.SpecialWeaponBuffBonusPct = 25
	}
	status.SpecialWeaponStatusPowerPct = d.SpecialWeaponStatusPowerPct
	specialWeaponBuffPower(status, d.SpecialWeaponStatusPowerPct)
	pct := int32(100)
	switch d.SpecialWeaponBaseID {
	case 1022, 1024, 1521, 1927:
		pct = 50
	case 1025:
		pct = 30
	case 1026:
		pct = 20
		status.DamageReductionCapPct = max(status.DamageReductionCapPct, int32(80), status.PhysicalDamageReductionPct, status.MagicDamageReductionPct)
		for i := range status.Affixes {
			a := &status.Affixes[i]
			if a.Attr == domain.AttrPhysRes || a.Attr == domain.AttrMagicRes {
				a.Value = max(a.Value, min(int32(80), specialScale(a.Value, 150)))
			}
		}
		status.PhysicalDamageReductionPct = max(status.PhysicalDamageReductionPct, min(int32(80), specialScale(status.PhysicalDamageReductionPct, 150)))
		status.MagicDamageReductionPct = max(status.MagicDamageReductionPct, min(int32(80), specialScale(status.MagicDamageReductionPct, 150)))
	case 1626:
		if d.ID == 10027 {
			pct = 50
		}
	case 1820:
		pct = 20
	case 1823:
		if d.ID == 10208 {
			pct = 50
			status.SpecialWeaponNextBasicPct = 60
		}
	case 1825:
		if d.ID == 10224 {
			status.DurationSec = 6
			for i := range status.Affixes {
				status.Affixes[i].Value = specialScale(status.Affixes[i].Value, 50)
			}
		}
	case 1922:
		pct = 30
	}
	if pct != 100 {
		status.DurationSec = max(int32(1), specialScale(status.DurationSec, pct))
	}
}
