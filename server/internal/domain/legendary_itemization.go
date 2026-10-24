package domain

import (
	"fmt"
	"math"
	"sort"
)

const LegendaryAffixCount = 6

// These values are immutable instance facts. Legacy native five-affix arrays are
// deliberately unchanged; loading never rerolls or scales an owned item.
type LegendaryAffixBounds struct{ Min, Max int32 }
type LegendaryInstance struct {
	PowerID string
	Tier    LegendaryTier `json:",omitempty"`
	Affixes [LegendaryAffixCount]InstanceAffix
	Bounds  [LegendaryAffixCount]LegendaryAffixBounds
}

func (v LegendaryInstance) Validate() error {
	if v.PowerID == "" {
		if v != (LegendaryInstance{}) {
			return fmt.Errorf("legendary rolls without power")
		}
		return nil
	}
	if _, ok := LegendaryPower(v.PowerID); !ok {
		return fmt.Errorf("unknown legendary power %q", v.PowerID)
	}
	if !v.Tier.Valid() {
		return fmt.Errorf("unknown legendary tier %q", v.Tier)
	}
	seen := map[int32]bool{}
	for i, a := range v.Affixes {
		b := v.Bounds[i]
		low, high := a.Affix, a.Affix
		low.Value, high.Value = b.Min, b.Max
		if v.Tier == LegendaryTierSecret && b.Min != b.Max {
			return fmt.Errorf("secret legendary affix %d is not locked", i)
		}
		if !BeneficialEndgameAffix(low) || !BeneficialEndgameAffix(high) || !BeneficialEndgameAffix(a.Affix) || seen[a.Affix.Attr] || b.Min > b.Max || a.Affix.Value < b.Min || a.Affix.Value > b.Max {
			return fmt.Errorf("invalid legendary affix %d", i)
		}
		seen[a.Affix.Attr] = true
	}
	return nil
}

// LegendaryAffixPool uses the original native pool, not enhanced white stats,
// character level or a previously multiplied instance. Units remain typed:
// mode 1 is percentage points; absolute crit is basis points; 69/70 are flat.
// Unsupported percent crit/resistance modes are excluded rather than advertised.
func (t EquipmentRollTable) LegendaryAffixPool(def ItemDef) []InstanceAffix {
	if def.Equip == nil {
		return nil
	}
	options := append([]EquipmentRollOption(nil), t.Options[EquipmentRollKey{def.Equip.ResourceLevel, def.Equip.Type}]...)
	if def.Equip.Type != 0 {
		options = append(options, t.Options[EquipmentRollKey{Level: def.Equip.ResourceLevel}]...)
	}
	best := map[[2]int32]InstanceAffix{}
	for _, o := range options {
		a := o.Affix
		if o.Weight <= 0 || !BeneficialEndgameAffix(a) {
			continue
		}
		mag := int64(a.Value)
		if mag < 0 {
			mag = -mag
		}
		if mag > math.MaxInt32/6 {
			continue
		}
		key := [2]int32{a.Attr, a.Mode}
		old, ok := best[key]
		oldmag := int64(old.Affix.Value)
		if oldmag < 0 {
			oldmag = -oldmag
		}
		if !ok || mag > oldmag || mag == oldmag && o.CardID < old.CardID {
			best[key] = InstanceAffix{CardID: o.CardID, Affix: a}
		}
	}
	out := make([]InstanceAffix, 0, len(best))
	for _, a := range best {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Affix.Attr != out[j].Affix.Attr {
			return out[i].Affix.Attr < out[j].Affix.Attr
		}
		return out[i].Affix.Mode < out[j].Affix.Mode
	})
	return out
}
func (t EquipmentRollTable) LegendaryReady(def ItemDef) bool {
	seen := map[int32]bool{}
	for _, a := range t.LegendaryAffixPool(def) {
		seen[a.Affix.Attr] = true
	}
	return len(seen) >= LegendaryAffixCount
}
func LegendaryCandidates(def ItemDef) []LegendaryPowerDefinition {
	return LegendaryCandidatesAtLevel(def, LegendaryGenerationLevel(def, Stack{}))
}

func LegendaryCandidatesAtLevel(def ItemDef, sourceLevel int32) []LegendaryPowerDefinition {
	var out []LegendaryPowerDefinition
	if !def.LegendaryEnabled {
		return out
	}
	for _, p := range LegendaryPowers() {
		if LegendaryPowerEligibleAtLevel(p, def, sourceLevel) && LegendaryPowerExecutable(p.ID) {
			out = append(out, p)
		}
	}
	return out
}

// ApplyLegendary is for non-monster awakening: preserve known provenance, or
// retain the historical base-level eligibility of a legacy/non-monster item.
func (t EquipmentRollTable) ApplyLegendary(st *Stack, def ItemDef, rng func(int) int) bool {
	if st == nil {
		return false
	}
	return t.applyLegendary(st, def, LegendaryGenerationLevel(def, *st), rng, true, false)
}

func (t EquipmentRollTable) ApplyLegendaryAtLevel(st *Stack, def ItemDef, sourceLevel int32, rng func(int) int) bool {
	return t.applyLegendary(st, def, sourceLevel, rng, true, true)
}

// ApplyLegendaryLegacy preserves pre-variant generation and exact RNG consumption.
func (t EquipmentRollTable) ApplyLegendaryLegacy(st *Stack, def ItemDef, rng func(int) int) bool {
	if st == nil {
		return false
	}
	return t.applyLegendary(st, def, LegendaryGenerationLevel(def, *st), rng, false, false)
}

func (t EquipmentRollTable) ApplyLegendaryLegacyAtLevel(st *Stack, def ItemDef, sourceLevel int32, rng func(int) int) bool {
	return t.applyLegendary(st, def, sourceLevel, rng, false, true)
}

func (t EquipmentRollTable) applyLegendary(st *Stack, def ItemDef, sourceLevel int32, rng func(int) int, variants, recordSource bool) bool {
	if st == nil || sourceLevel < 1 || (st.Endgame.SourceLevel != 0 && st.Endgame.SourceLevel != sourceLevel) || rng == nil || st.UID <= 0 || st.Count != 1 || st.Item != def.ID || st.Legendary.PowerID != "" || st.Endgame.UniqueID != 0 || !CanPromoteSocketlessUnique(*st) || !t.LegendaryReady(def) {
		return false
	}
	powers := LegendaryCandidatesAtLevel(def, sourceLevel)
	if len(powers) == 0 {
		return false
	}
	n := rng(len(powers))
	if n < 0 || n >= len(powers) {
		return false
	}
	v := LegendaryInstance{PowerID: powers[n].ID}
	options := t.LegendaryAffixPool(def)
	for i := range v.Affixes {
		n = rng(len(options))
		if n < 0 || n >= len(options) {
			return false
		}
		a := options[n]
		lo, hi := a.Affix.Value*3, a.Affix.Value*6
		if lo > hi {
			lo, hi = hi, lo
		}
		n = rng(int(int64(hi) - int64(lo) + 1))
		if n < 0 || int64(n) > int64(hi)-int64(lo) {
			return false
		}
		a.Affix.Value = lo + int32(n)
		v.Affixes[i] = a
		v.Bounds[i] = LegendaryAffixBounds{lo, hi}
		keep := options[:0]
		for _, o := range options {
			if o.Affix.Attr != a.Affix.Attr {
				keep = append(keep, o)
			}
		}
		options = keep
	}
	if variants && !v.rollTier(rng) {
		return false
	}
	if v.Validate() != nil {
		return false
	}
	st.Legendary = v
	if recordSource {
		st.Endgame.SourceLevel = sourceLevel
	}
	st.SocketCount = 0
	st.RolledAffixCount = 0
	st.RolledAffixes = [5]InstanceAffix{}
	st.Endgame.Revision++
	return true
}
func (v LegendaryInstance) HasVariableAffixes() bool {
	for _, b := range v.Bounds {
		if b.Min < b.Max {
			return true
		}
	}
	return false
}

func (v *LegendaryInstance) Reforge(rng func(int) int) bool {
	if v == nil || v.PowerID == "" || rng == nil || v.Validate() != nil {
		return false
	}
	next := *v
	for i, b := range v.Bounds {
		n := rng(int(int64(b.Max) - int64(b.Min) + 1))
		if n < 0 || int64(n) > int64(b.Max)-int64(b.Min) {
			return false
		}
		next.Affixes[i].Affix.Value = b.Min + int32(n)
	}
	if next == *v {
		for i, b := range next.Bounds {
			if b.Min < b.Max {
				if next.Affixes[i].Affix.Value == b.Max {
					next.Affixes[i].Affix.Value = b.Min
				} else {
					next.Affixes[i].Affix.Value++
				}
				break
			}
		}
	}
	if next == *v {
		return false
	}
	*v = next
	return true
}

func LegendaryItemBonuses(def ItemDef, st Stack) []Affix {
	p, ok := LegendaryPower(st.Legendary.PowerID)
	if !ok || !LegendaryPowerEligibleForItem(p, def, st) || st.Legendary.Validate() != nil {
		return nil
	}
	out := make([]Affix, 0, LegendaryAffixCount)
	for _, a := range st.Legendary.Affixes {
		out = append(out, a.Affix)
	}
	return out
}
