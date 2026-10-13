package domain

import (
	"fmt"
	"math"
	"math/big"
)

// LootProbabilityScaleV5 is the precision of the positive-MF probability tables.
// Every tail is rounded down before adjacent tails are subtracted. The resulting
// probabilities sum to exactly this scale. MF=0 uses the original integer weights
// directly when rolling, preserving even the elite profile's 99-part distribution.
const LootProbabilityScaleV5 int64 = 1_000_000_000

const LootRuleVersionV5 = 5

// LootContextV5 is a value snapshot taken at the lethal-event boundary, before
// kill experience or other rewards can change the owner's effective equipment.
// No party member or subsequent picker contributes magic find.
type LootContextV5 struct {
	Owner     EntityID
	MagicFind int64
	Version   int
}

// NaturalAffixProbabilitiesV5 transforms the ORIGINAL support first. The caller
// then folds the result into the equipment's remaining fixed+socket+random budget.
// Q' = T*(200+2*M) / (W*(200+M)+T*M), for tail weight T and total W.
// big.Int intermediates prevent overflow for every legal int64 MF value.
func NaturalAffixProbabilitiesV5(counts []WeightedAffixCount, magicFind int64) ([MaxEquipmentAttributes + 1]int64, error) {
	var result [MaxEquipmentAttributes + 1]int64
	var weights [MaxEquipmentAttributes + 1]int64
	var seen [MaxEquipmentAttributes + 1]bool
	var total int64
	for _, c := range counts {
		if int(c.Count) > MaxEquipmentAttributes || c.Weight < 0 || seen[c.Count] {
			return result, fmt.Errorf("invalid v5 color weight: %+v", c)
		}
		seen[c.Count] = true
		if c.Weight > math.MaxInt64-total {
			return result, fmt.Errorf("v5 color weight total exceeds int64")
		}
		weights[c.Count] = c.Weight
		total += c.Weight
	}
	if total == 0 {
		return result, fmt.Errorf("empty v5 color weight distribution")
	}
	m := big.NewInt(max(int64(0), magicFind))
	boost := new(big.Int).Add(new(big.Int).Lsh(new(big.Int).Set(m), 1), big.NewInt(200))
	base := new(big.Int).Mul(big.NewInt(total), new(big.Int).Add(m, big.NewInt(200)))
	var tails [MaxEquipmentAttributes + 2]int64
	tails[0] = LootProbabilityScaleV5
	var tailWeight int64
	for k := MaxEquipmentAttributes; k >= 1; k-- {
		tailWeight += weights[k]
		numerator := new(big.Int).Mul(big.NewInt(tailWeight), boost)
		numerator.Mul(numerator, big.NewInt(LootProbabilityScaleV5))
		denominator := new(big.Int).Add(base, new(big.Int).Mul(big.NewInt(tailWeight), m))
		tails[k] = numerator.Quo(numerator, denominator).Int64()
	}
	for k := range result {
		result[k] = tails[k] - tails[k+1]
	}
	return result, nil
}

// ValidateMagicFindColorProfilesV5 rejects malformed or overflowing configured
// weights during data loading, rather than silently generating white equipment.
func ValidateMagicFindColorProfilesV5(t EquipmentRollTable) error {
	for profile, counts := range t.Counts {
		if _, err := NaturalAffixProbabilitiesV5(counts, 0); err != nil {
			return fmt.Errorf("color profile %d: %w", profile, err)
		}
	}
	return nil
}

// UniqueProbabilityV5 is the natural-drop 1% baseline with the approved bounded
// curve 0.01*(1+2*M/(250+M)). This is not a scripted-reward or crafting chance.
func UniqueProbabilityV5(magicFind int64, rule *LootRuleSet) int64 {
	if rule == nil {
		return 0
	}
	m := big.NewInt(max(int64(0), magicFind))
	numerator := new(big.Int).Add(new(big.Int).Mul(m, big.NewInt(1+rule.UniqueMFBoost)), big.NewInt(rule.UniqueMFDivisor))
	numerator.Mul(numerator, big.NewInt(rule.UniqueBasePPB))
	denominator := new(big.Int).Add(m, big.NewInt(rule.UniqueMFDivisor))
	return numerator.Quo(numerator, denominator).Int64()
}

// RollNaturalUniqueWithMagicFindV5 is ONLY for explicit natural monster drop
// sources. New-generation catalog membership and NativeDropEligible are both
// required. The independent variable stream preserves the original unique rolls.
func RollNaturalUniqueWithMagicFindV5(st *Stack, def ItemDef, magicFind int64, chance func(int64) int64, variable func(int) int, rule *LootRuleSet) bool {
	if st == nil || chance == nil || variable == nil || def.Equip == nil || def.Equip.NoTypeDrop {
		return false
	}
	u, ok := UniqueForBase(st.Item, def.EndgameCatalog)
	if !ok || !u.NativeDropEligible {
		return false
	}
	roll := chance(LootProbabilityScaleV5)
	if roll < 0 || roll >= UniqueProbabilityV5(magicFind, rule) {
		return false
	}
	return ApplyUnique(st, def, variable)
}

// RollWithMagicFindV5 keeps natural-count, affix-value, socket and unique RNG
// domains separate. MF changes only the requested count; option weights/values
// are untouched and insufficient distinct attributes still produce fewer lines.
func (t EquipmentRollTable) RollWithMagicFindV5(def ItemDef, profile ColorProfile, sockets uint8, magicFind int64, countRNG, valueRNG func(int64) int64) []InstanceAffix {
	if !t.Enabled() || def.Equip == nil || countRNG == nil || valueRNG == nil {
		return nil
	}
	budget := max(0, MaxEquipmentAttributes-len(def.Equip.Affixes)-int(sockets))
	if budget == 0 {
		return nil
	}
	counts := t.CountsFor(profile)
	var count uint8
	if magicFind <= 0 {
		count = weightedAffixCount(counts, countRNG)
	} else {
		probabilities, err := NaturalAffixProbabilitiesV5(counts, magicFind)
		if err != nil {
			return nil // invalid tables are rejected at startup
		}
		roll := countRNG(LootProbabilityScaleV5)
		if roll < 0 || roll >= LootProbabilityScaleV5 {
			return nil
		}
		for n, weight := range probabilities {
			if roll < weight {
				count = uint8(n)
				break
			}
			roll -= weight
		}
	}
	return t.rollAffixOptions(def, uint8(min(int(count), budget)), valueRNG)
}
