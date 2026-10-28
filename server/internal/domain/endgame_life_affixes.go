package domain

import (
	"fmt"
	"math"
)

// Life crafting never mutates its input. The caller owns inventory consumption,
// revision/CAS and persistence in one transaction with the returned instance.
// A nil RNG performs eligibility preflight only and returns the unchanged input.
func lifeTarget(st Stack) error {
	if st.UID <= 0 || st.Count != 1 || st.Legendary.PowerID == "" {
		return fmt.Errorf("须以暗金装备入鼎")
	}
	return st.Legendary.Validate()
}
func lifeDraw(rng func(int) int, n int) (int, error) {
	if rng == nil || n <= 0 {
		return 0, fmt.Errorf("invalid life crafting random source")
	}
	v := rng(n)
	if v < 0 || v >= n {
		return 0, fmt.Errorf("invalid life crafting random draw")
	}
	return v, nil
}

// All seven grades retain a uniform component; even the highest grade can roll
// the minimum and the lowest can roll the maximum. Higher grades increasingly
// choose the better of two uniform draws (grade 1: 0%, grade 7: 75%).
func lifeAffixValue(b LegendaryAffixBounds, attr int32, grade int, rng func(int) int) (int32, error) {
	if grade < 1 || grade > 7 || b.Min >= b.Max {
		return 0, fmt.Errorf("此词缀无可洗炼之数")
	}
	width := int(int64(b.Max) - int64(b.Min) + 1)
	a, err := lifeDraw(rng, width)
	if err != nil {
		return 0, err
	}
	chance, err := lifeDraw(rng, 8)
	if err != nil {
		return 0, err
	}
	if chance < grade-1 {
		other, e := lifeDraw(rng, width)
		if e != nil {
			return 0, e
		}
		if attr == AttrAtkSpeed {
			a = min(a, other)
		} else {
			a = max(a, other)
		}
	}
	return int32(int64(b.Min) + int64(a)), nil
}

func LifeRerollAffixValue(st Stack, slot, grade int, rng func(int) int) (Stack, error) {
	if err := lifeTarget(st); err != nil {
		return st, err
	}
	if slot < 0 || slot >= 4 {
		return st, fmt.Errorf("四象只应前四词缀")
	}
	if grade < 1 || grade > 7 || st.Legendary.Bounds[slot].Min >= st.Legendary.Bounds[slot].Max {
		return st, fmt.Errorf("此词缀无可洗炼之数")
	}
	if rng == nil {
		return st, nil
	}
	value, err := lifeAffixValue(st.Legendary.Bounds[slot], st.Legendary.Affixes[slot].Affix.Attr, grade, rng)
	if err != nil {
		return st, err
	}
	st.Legendary.Affixes[slot].Affix.Value = value
	return st, nil
}

// cardBias maps matching attribute IDs to additional selection weight. Callers
// derive it from a validated consumed card's level; a higher level gives a larger
// bonus. All unblocked attributes keep weight 1 and remain possible.
func LifeRerollAffix(st Stack, def ItemDef, table EquipmentRollTable, slot, grade int, cardBias map[int32]int, rng func(int) int) (Stack, error) {
	if err := lifeTarget(st); err != nil {
		return st, err
	}
	if slot < 0 || slot >= 4 || grade < 1 || grade > 7 || def.ID != st.Item {
		return st, fmt.Errorf("四象与装备不合")
	}
	occupied := map[int32]bool{}
	for i, a := range st.Legendary.Affixes {
		if i != slot {
			occupied[a.Affix.Attr] = true
		}
	}
	var options []InstanceAffix
	total := 0
	for _, a := range table.LegendaryAffixPool(def) {
		if !occupied[a.Affix.Attr] {
			options = append(options, a)
			total += 1 + max(0, min(100, cardBias[a.Affix.Attr]))
		}
	}
	if total == 0 {
		return st, fmt.Errorf("无可用词缀")
	}
	if rng == nil {
		return st, nil
	}
	draw, err := lifeDraw(rng, total)
	if err != nil {
		return st, err
	}
	var chosen InstanceAffix
	for _, a := range options {
		draw -= 1 + max(0, min(100, cardBias[a.Affix.Attr]))
		if draw < 0 {
			chosen = a
			break
		}
	}
	lo, hi := int64(chosen.Affix.Value)*3, int64(chosen.Affix.Value)*6
	if lo > hi {
		lo, hi = hi, lo
	}
	if st.Legendary.Tier == LegendaryTierTrue || st.Legendary.Tier == LegendaryTierSecret {
		lo, hi = lo*3/2, hi*3/2
	}
	if lo < math.MinInt32 || hi > math.MaxInt32 {
		return st, fmt.Errorf("词缀数值超出范围")
	}
	b := LegendaryAffixBounds{int32(lo), int32(hi)}
	var value int32
	if st.Legendary.Tier == LegendaryTierSecret {
		value = b.Max
		if chosen.Affix.Attr == AttrAtkSpeed {
			value = b.Min
		}
		b = LegendaryAffixBounds{value, value}
	} else {
		value, err = lifeAffixValue(b, chosen.Affix.Attr, grade, rng)
		if err != nil {
			return st, err
		}
	}
	chosen.Affix.Value = value
	next := st
	next.Legendary.Affixes[slot] = chosen
	next.Legendary.Bounds[slot] = b
	if err := next.Legendary.Validate(); err != nil {
		return st, err
	}
	return next, nil
}

func LifeShuffleAffixes(st Stack, rng func(int) int) (Stack, error) {
	if err := lifeTarget(st); err != nil {
		return st, err
	}
	if rng == nil {
		return st, nil
	}
	next := st
	for i := LegendaryAffixCount - 1; i > 0; i-- {
		j, err := lifeDraw(rng, i+1)
		if err != nil {
			return st, err
		}
		next.Legendary.Affixes[i], next.Legendary.Affixes[j] = next.Legendary.Affixes[j], next.Legendary.Affixes[i]
		next.Legendary.Bounds[i], next.Legendary.Bounds[j] = next.Legendary.Bounds[j], next.Legendary.Bounds[i]
		switch next.Legendary.TemperedSlot {
		case uint8(i + 1):
			next.Legendary.TemperedSlot = uint8(j + 1)
		case uint8(j + 1):
			next.Legendary.TemperedSlot = uint8(i + 1)
		}
	}
	return next, nil
}

func LifeTemperAffix(st Stack, rng func(int) int) (Stack, error) {
	if err := lifeTarget(st); err != nil {
		return st, err
	}
	if st.RefineLevel < 15 {
		return st, fmt.Errorf("强化未达十五，尚不可淬炼")
	}
	var slots []int
	for i, b := range st.Legendary.Bounds {
		if b.Min < b.Max {
			slots = append(slots, i)
		}
	}
	if len(slots) == 0 {
		return st, fmt.Errorf("此装备无可淬炼之词缀")
	}
	if rng == nil {
		return st, nil
	}
	n, err := lifeDraw(rng, len(slots))
	if err != nil {
		return st, err
	}
	next := st
	next.Legendary.TemperedSlot = uint8(slots[n] + 1)
	if err := next.Legendary.Validate(); err != nil {
		return st, err
	}
	return next, nil
}

// EffectiveAffix leaves the durable raw roll untouched. Signed reductions are
// strengthened in magnitude too. Round halves away from zero so +1 gains value.
func (v LegendaryInstance) EffectiveAffix(slot int) (InstanceAffix, LegendaryAffixBounds) {
	a, b := v.Affixes[slot], v.Bounds[slot]
	if v.TemperedSlot == uint8(slot+1) {
		boost := func(n int32) int32 {
			x := int64(n) * 3
			if x > 0 {
				x++
			} else if x < 0 {
				x--
			}
			return int32(x / 2)
		}
		a.Affix.Value = boost(a.Affix.Value)
		b.Min = boost(b.Min)
		b.Max = boost(b.Max)
	}
	return a, b
}

// LifeCardBias uses the owned card's static effects, not mutable template stats.
func LifeCardBias(def ItemDef, card Stack) map[int32]int {
	out := map[int32]int{}
	affixes := def.SocketAffixes
	if card.Card.Initialized {
		affixes = card.Card.StaticAffixes()
	}
	for _, a := range affixes {
		if BeneficialEndgameAffix(a) {
			out[a.Attr] = int(max(int32(1), min(int32(100), def.Level)))
		}
	}
	return out
}
