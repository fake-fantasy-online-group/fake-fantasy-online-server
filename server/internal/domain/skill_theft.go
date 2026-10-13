package domain

import "math"

// TheftEffect reports the explicitly configured loot-theft effect. Skill IDs and
// item IDs do not grant theft implicitly; the database owns the effect identity.
func (d SkillDef) TheftEffect() (SkillEffect, bool) {
	for _, effect := range d.DirectEffects() {
		if effect.Kind == EffectStealItem {
			return effect, true
		}
	}
	return SkillEffect{}, false
}

// TheftDropWeight interprets the existing monster drop percentage as a relative
// weight for one theft selection, not as a second probability check. The million
// parts per percentage-point precision retains the smallest authored rates.
func TheftDropWeight(entry DropEntry) int64 {
	if entry.RatePct <= 0 || entry.RatePct > 100 || math.IsNaN(entry.RatePct) || math.IsInf(entry.RatePct, 0) {
		return 0
	}
	return int64(math.Round(entry.RatePct * 1_000_000))
}

// PickTheftDrop selects exactly one eligible row. Callers retain the existing
// per-row Choices distribution, instance generation, and item ownership rules.
func PickTheftDrop(entries []DropEntry, intn func(int64) int64) (DropEntry, bool) {
	var total int64
	for _, entry := range entries {
		weight := TheftDropWeight(entry)
		if weight > math.MaxInt64-total {
			return DropEntry{}, false
		}
		total += weight
	}
	if total == 0 || intn == nil {
		return DropEntry{}, false
	}
	roll := intn(total)
	if roll < 0 || roll >= total {
		return DropEntry{}, false
	}
	for _, entry := range entries {
		weight := TheftDropWeight(entry)
		if roll < weight {
			return entry, true
		}
		roll -= weight
	}
	return DropEntry{}, false
}
