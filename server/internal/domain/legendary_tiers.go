package domain

import (
	"math"
	"strings"
)

// LegendaryTier is an immutable instance fact, independent from dungeon true
// set template IDs. Missing Tier in old JSON means an ordinary legendary.
type LegendaryTier string

const (
	LegendaryTierNormal LegendaryTier = ""
	LegendaryTierTrue   LegendaryTier = "true"
	LegendaryTierSecret LegendaryTier = "secret"
)

func (t LegendaryTier) Valid() bool {
	return t == LegendaryTierNormal || t == LegendaryTierTrue || t == LegendaryTierSecret
}

// One mutually exclusive draw per new legendary: 1 secret, 10 true, 89 normal.
// Tier is drawn after the unchanged power/affix selection, never during reads.
func (v *LegendaryInstance) rollTier(rng func(int) int) bool {
	n := rng(100)
	if n < 0 || n >= 100 {
		return false
	}
	switch {
	case n == 0:
		v.Tier = LegendaryTierSecret
	case n <= 10:
		v.Tier = LegendaryTierTrue
	default:
		return true
	}
	for i, b := range v.Bounds {
		// Integer division truncates toward zero, preserving signed beneficial
		// interval semantics. Convert via int64 before multiplication.
		scaledLo, scaledHi := int64(b.Min)*3/2, int64(b.Max)*3/2
		if scaledLo < math.MinInt32 || scaledHi > math.MaxInt32 {
			return false
		}
		lo, hi := int32(scaledLo), int32(scaledHi)
		value := hi
		if v.Tier == LegendaryTierSecret {
			// Attack-interval reductions are beneficial when negative: the strongest
			// original endpoint is Min, not the numerically largest (weaker) Max.
			if v.Affixes[i].Affix.Value < 0 {
				value = lo
			}
			lo, hi = value, value
		} else {
			n = rng(int(int64(hi) - int64(lo) + 1))
			if n < 0 || int64(n) > int64(hi)-int64(lo) {
				return false
			}
			value = lo + int32(n)
		}
		v.Bounds[i] = LegendaryAffixBounds{Min: lo, Max: hi}
		v.Affixes[i].Affix.Value = value
	}
	return true
}

func prefixedEquipmentName(prefix, name string) string {
	if strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

func LegendaryInstanceDisplayName(base string, power LegendaryPowerDefinition, instance LegendaryInstance) string {
	name := LegendaryDisplayName(base, power)
	switch instance.Tier {
	case LegendaryTierTrue:
		return prefixedEquipmentName("真*", name)
	case LegendaryTierSecret:
		return prefixedEquipmentName("秘*", name)
	default:
		return name
	}
}

// LegendaryIntrinsic scales only the template's innate white attributes.
// Fixed refinement increments, six random affixes, set bonuses, socket/fusion
// affixes and power/skill mechanisms remain independent and are not multiplied.
// Read/compute calls are pure; they never rewrite persisted rolls or templates.
func LegendaryIntrinsic(def ItemDef, st Stack) (Base, Stats) {
	base, stats := def.Equip.RefinedIntrinsic(st.RefineLevel)
	multiplier := int32(1)
	if p, ok := LegendaryPower(st.Legendary.PowerID); ok && LegendaryPowerEligibleForItem(p, def, st) && st.Legendary.Validate() == nil {
		switch st.Legendary.Tier {
		case LegendaryTierTrue:
			multiplier = 2
		case LegendaryTierSecret:
			multiplier = 3
		}
	}
	if multiplier == 1 || def.Equip == nil {
		return base, stats
	}
	// Saturation avoids wraparound if an imported template is already near int32.
	add := func(total, innate int32) int32 {
		n := int64(total) + int64(innate)*int64(multiplier-1)
		return int32(max(int64(math.MinInt32), min(int64(math.MaxInt32), n)))
	}
	b, s := def.Equip.Base, def.Equip.Bonus
	base = Base{STR: add(base.STR, b.STR), VIT: add(base.VIT, b.VIT), INT: add(base.INT, b.INT), SPI: add(base.SPI, b.SPI), AGI: add(base.AGI, b.AGI), DEX: add(base.DEX, b.DEX)}
	stats = Stats{
		MaxHP: add(stats.MaxHP, s.MaxHP), MaxMP: add(stats.MaxMP, s.MaxMP), HPRegen: add(stats.HPRegen, s.HPRegen), MPRegen: add(stats.MPRegen, s.MPRegen),
		MinAtk: add(stats.MinAtk, s.MinAtk), MaxAtk: add(stats.MaxAtk, s.MaxAtk), Def: add(stats.Def, s.Def), Hit: add(stats.Hit, s.Hit), MAtk: add(stats.MAtk, s.MAtk), MDef: add(stats.MDef, s.MDef),
		EffectHitBP: add(stats.EffectHitBP, s.EffectHitBP), CritRate: add(stats.CritRate, s.CritRate), MCritRate: add(stats.MCritRate, s.MCritRate),
		AtkSpeedPct: add(stats.AtkSpeedPct, s.AtkSpeedPct), AtkSpeedMS: add(stats.AtkSpeedMS, s.AtkSpeedMS), MoveSpeed: add(stats.MoveSpeed, s.MoveSpeed),
		PhysResist: add(stats.PhysResist, s.PhysResist), MagicResist: add(stats.MagicResist, s.MagicResist), StatusResist: add(stats.StatusResist, s.StatusResist), MaxWeight: add(stats.MaxWeight, s.MaxWeight),
	}
	return base, stats
}
