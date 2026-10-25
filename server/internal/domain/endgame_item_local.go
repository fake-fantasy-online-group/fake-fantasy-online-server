package domain

import (
	"fmt"
	"math/rand"
)

// VariableAffix names an existing authored line. All unnamed lines and traits
// are fixed. Min/Max are numeric bounds, not additional affixes.
type VariableAffix struct {
	Attr int32
	Min  int32
	Max  int32
}

// ItemLocalEnhancement scales only this item's unrefined intrinsic attack or
// defense. It never scales character stats, refinement, sockets or other extras.
// Zero MinPercent/MaxPercent means fixed; otherwise Percent is a reference value.
type ItemLocalEnhancement struct {
	Attr       int32
	Percent    int32
	MinPercent int32
	MaxPercent int32
}

// Eleven approximately equally likely quality bands make the perfect shared roll roughly
// 1 in 11 instead of 1 in 4001. Historical rolls stay untouched; only explicitly
// variable lines consume their band. One band drives all lines on the item.
func endgameRollBand(roll int32) int32 {
	if roll == 0 {
		roll = 10000
	} // old formation instances have a stable midpoint
	roll = max(UniqueRollMinBP, min(UniqueRollMaxBP, roll))
	return (roll - UniqueRollMinBP) * 11 / (UniqueRollMaxBP - UniqueRollMinBP + 1)
}
func variableValue(v VariableAffix, roll int32) int32 {
	band := endgameRollBand(roll)
	if v.Attr == AttrAtkSpeed {
		band = 10 - band
	}
	return int32(int64(v.Min) + (int64(v.Max)-int64(v.Min))*int64(band)/10)
}
func ResolveVariableAffixes(affixes []Affix, variables []VariableAffix, roll int32) []Affix {
	out := append([]Affix(nil), affixes...)
	for i := range out {
		for _, v := range variables {
			if v.Attr == out[i].Attr {
				out[i].Value = variableValue(v, roll)
				break
			}
		}
	}
	return out
}
func (v ItemLocalEnhancement) ValueAt(roll int32) int32 {
	if v.MinPercent == 0 && v.MaxPercent == 0 {
		return v.Percent
	}
	return variableValue(VariableAffix{Min: v.MinPercent, Max: v.MaxPercent}, roll)
}
func ResolveItemLocalEnhancements(e *EquipDef, local []ItemLocalEnhancement, roll int32) []Affix {
	var input []Affix
	for _, v := range local {
		input = append(input, Affix{Attr: v.Attr, Value: v.ValueAt(roll), Mode: ModePercent})
	}
	out, ok := ResolveRefineBonuses(e, input)
	if !ok {
		return nil
	} // invalid/overflowing templates never become global %
	return out
}
func validateEndgameRollModel(affixes []Affix, variables []VariableAffix, local []ItemLocalEnhancement) error {
	seen := map[int32]bool{}
	variableCount := len(variables)
	for _, v := range variables {
		if seen[v.Attr] || v.Min >= v.Max {
			return fmt.Errorf("invalid variable affix %+v", v)
		}
		seen[v.Attr] = true
		found := false
		for _, a := range affixes {
			if a.Attr != v.Attr {
				continue
			}
			found = true
			if a.Value < v.Min || a.Value > v.Max {
				return fmt.Errorf("variable reference outside range %+v", v)
			}
			a.Value = v.Min
			if !BeneficialEndgameAffix(a) {
				return fmt.Errorf("variable minimum is not beneficial %+v", v)
			}
			a.Value = v.Max
			if !BeneficialEndgameAffix(a) {
				return fmt.Errorf("variable maximum is not beneficial %+v", v)
			}
		}
		if !found {
			return fmt.Errorf("variable has no authored affix %+v", v)
		}
	}
	seen = map[int32]bool{}
	if len(local) > 2 {
		return fmt.Errorf("too many local enhancements")
	}
	for _, v := range local {
		if seen[v.Attr] || v.Percent <= 0 || v.Percent > 1000 {
			return fmt.Errorf("invalid local enhancement %+v", v)
		}
		seen[v.Attr] = true
		switch v.Attr {
		case AttrAtk, AttrMAtk, AttrDef, AttrMDef:
		default:
			return fmt.Errorf("unsupported local attribute %d", v.Attr)
		}
		if v.MinPercent != 0 || v.MaxPercent != 0 {
			variableCount++
			if v.MinPercent <= 0 || v.MinPercent >= v.MaxPercent || v.Percent < v.MinPercent || v.Percent > v.MaxPercent || v.MaxPercent > 1000 {
				return fmt.Errorf("invalid local range %+v", v)
			}
		}
	}
	if variableCount > 2 {
		return fmt.Errorf("at most two variable lines are allowed")
	}
	return nil
}

func UniqueItemBonuses(def ItemDef, st Stack) []Affix {
	d, ok := UniqueOf(st,def.EndgameCatalog)
	if !ok {
		return nil
	}
	out := ResolveVariableAffixes(d.Affixes, d.VariableAffixes, st.Endgame.UniqueRollBP)
	return append(out, ResolveItemLocalEnhancements(def.Equip, d.LocalEnhancements, st.Endgame.UniqueRollBP)...)
}
func AstralItemBonuses(def ItemDef, st Stack, r AstralRecipe) []Affix {
	out := ResolveVariableAffixes(r.Affixes, r.VariableAffixes, st.Endgame.FormationRollBP)
	out = resolveWeaponAttackAffixes(def, st, out)
	return append(out, ResolveItemLocalEnhancements(def.Equip, r.LocalEnhancements, st.Endgame.FormationRollBP)...)
}

// Created exactly once, with the equipment UID, before publication/persistence.
// Loading, moving, awakening and changing sockets never call this function.
func newFormationRollBP() int32 {
	return UniqueRollMinBP + int32(rand.Intn(int(UniqueRollMaxBP-UniqueRollMinBP+1)))
}
func UniqueHasVariableStats(d UniqueDefinition) bool {
	if len(d.VariableAffixes) > 0 {
		return true
	}
	for _, v := range d.LocalEnhancements {
		if v.MinPercent != 0 {
			return true
		}
	}
	return false
}
func sameUniqueVariableValues(d UniqueDefinition, a, b int32) bool {
	for _, v := range d.VariableAffixes {
		if variableValue(v, a) != variableValue(v, b) {
			return false
		}
	}
	for _, v := range d.LocalEnhancements {
		if v.ValueAt(a) != v.ValueAt(b) {
			return false
		}
	}
	return true
}

// EnhancedIntrinsic is shared by native white-number display and stat addition.
// Refinement and local enhancement each use the original template independently.
func EnhancedIntrinsic(e *EquipDef, refine int32, local []ItemLocalEnhancement, roll int32) (Base, Stats) {
	base, stats := e.RefinedIntrinsic(refine)
	var basePct Base
	var pct statPercent
	for _, a := range ResolveItemLocalEnhancements(e, local, roll) {
		applyAffix(a, &base, &basePct, &stats, &pct)
	}
	return base, stats
}
