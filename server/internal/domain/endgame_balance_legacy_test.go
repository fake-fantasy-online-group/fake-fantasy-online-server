package domain

import (
	"math"
	"math/big"
)

// Frozen pre-SQL63 algorithms copied from repository HEAD. Independent of SQL fixtures.
func legacyBalanceRiftScaledStat(base int32, tier int32, party int, class int32, hp bool) int32 {
	if base <= 0 {
		return 0
	}
	tier = legacyBalanceclampRiftTier(tier)
	party = max(1, min(party, 5))
	class = max(int32(0), min(class, int32(3)))
	v := float64(base)
	rate := 1.04
	if hp {
		rate = 1.08
		v *= 1 + 0.6*float64(party-1)
	}
	v *= math.Pow(rate, float64(tier-1))
	if hp {
		switch class {
		case 1:
			v *= 4
		case 2:
			v *= 8
		case 3:
			v *= 40
		}
	} else {
		switch class {
		case 1:
			v *= 1.2
		case 2:
			v *= 1.4
		case 3:
			v *= 1.5
		}
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(math.Ceil(v))
}

func legacyBalancePlanRiftRewards(tier int32, timely bool) RiftRewardPlan {
	tier = legacyBalanceclampRiftTier(tier)
	p := RiftRewardPlan{Equipment: 1, Rune: 1, Dust: 6 + (tier-1)/5, UniqueChance: 20 + 2*tier}
	if p.UniqueChance > 60 {
		p.UniqueChance = 60
	}
	if timely {
		p.Equipment++
		p.Rune++
		p.Dust += 4
	}
	return p
}

func legacyBalanceclampRiftTier(tier int32) int32 {
	return max(int32(1), min(tier, RiftMaxTier))
}

func legacyBalanceRollDroppedSocketCount(def ItemDef, grade AstralDropGrade, intn func(int64) int64) uint8 {
	if def.Equip == nil || intn == nil || def.Equip.LevelReq < 10 {
		return 0
	}
	slot := EquipSlot(def.Equip.Slot)
	if slot < SlotFace || slot > SlotShoe && slot != SlotTwoHand {
		return 0
	}
	cap := max(0, MaxEquipmentAttributes-len(def.Equip.Affixes))
	levelCap := 2
	if def.Equip.LevelReq >= 50 {
		levelCap = 5
	} else if def.Equip.LevelReq >= 40 {
		levelCap = 4
	} else if def.Equip.LevelReq >= 20 {
		levelCap = 3
	}
	cap = min(cap, levelCap)
	if cap == 0 {
		return 0
	}
	weights := [4][6]int64{{72, 21, 6, 1, 0, 0}, {45, 25, 18, 9, 3, 0}, {25, 25, 23, 17, 8, 2}, {15, 20, 25, 22, 13, 5}}
	if grade > AstralDropRealm {
		grade = AstralDropOrdinary
	}
	roll := intn(100)
	if roll < 0 || roll >= 100 {
		return 0
	}
	for n, w := range weights[grade] {
		if roll < w {
			return uint8(min(n, cap))
		}
		roll -= w
	}
	return 0
}

func legacyBalanceUniqueProbabilityV5(magicFind int64) int64 {
	m := big.NewInt(max(int64(0), magicFind))
	numerator := new(big.Int).Add(new(big.Int).Mul(m, big.NewInt(3)), big.NewInt(250))
	numerator.Mul(numerator, big.NewInt(LootProbabilityScaleV5/100))
	denominator := new(big.Int).Add(m, big.NewInt(250))
	return numerator.Quo(numerator, denominator).Int64()
}
