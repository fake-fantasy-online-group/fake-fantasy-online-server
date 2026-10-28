package domain

import (
	"fmt"
	"sort"
	"sync"
)

// Source kinds describe runtime provenance, never loot or unlock eligibility.
type EquipmentEffectSourceKind string

const (
	EffectSourceLegendary     EquipmentEffectSourceKind = "legendary"
	EffectSourceSpecialWeapon EquipmentEffectSourceKind = "special_weapon"
	EffectSourceUniqueTrait   EquipmentEffectSourceKind = "unique_trait"
	EffectSourceTyped         EquipmentEffectSourceKind = "typed"
	EffectSourceNative        EquipmentEffectSourceKind = "native"
	EffectSourceSet           EquipmentEffectSourceKind = "set"
)

type EquipmentEffectDefinition struct {
	ID          string
	SourceKind  EquipmentEffectSourceKind
	Trigger     string
	Description string
}

// Typed payloads retain all variant parameters; identity is not inferred from prose.
type EquipmentEffectParameters struct {
	Legendary         *ActiveLegendaryPower
	Typed             *EquipmentEffect
	Unique            *UniqueTrait
	SpecialWeaponBase ItemID
	BaseID            ItemID
	Profession        int32
	Skills            []SkillID
	Values            map[string]int64
}
type RuntimeEquipmentEffect struct {
	DefinitionID string
	ID           string // Stable producer identity, deliberately independent of rolls and item UID.
	SourceKind   EquipmentEffectSourceKind
	SourceUID    int64
	Item         ItemID
	Trigger      string
	Description  string
	Parameters   EquipmentEffectParameters
	Producer     bool
}

var equipmentEffectRegistry = struct {
	sync.RWMutex
	defs map[string]EquipmentEffectDefinition
}{defs: make(map[string]EquipmentEffectDefinition)}

// RegisterEquipmentEffectDefinition rejects collisions, including identical duplicate
// registrations: one runtime mechanic must have exactly one authoritative owner.
func RegisterEquipmentEffectDefinition(d EquipmentEffectDefinition) {
	if d.ID == "" || d.SourceKind == "" || d.Trigger == "" || d.Description == "" {
		panic("invalid equipment effect definition")
	}
	equipmentEffectRegistry.Lock()
	defer equipmentEffectRegistry.Unlock()
	if _, exists := equipmentEffectRegistry.defs[d.ID]; exists {
		panic("duplicate equipment effect definition " + d.ID)
	}
	equipmentEffectRegistry.defs[d.ID] = d
}
func EquipmentEffectDefinitionByID(id string) (EquipmentEffectDefinition, bool) {
	equipmentEffectRegistry.RLock()
	defer equipmentEffectRegistry.RUnlock()
	d, ok := equipmentEffectRegistry.defs[id]
	return d, ok
}
func EquipmentEffectDefinitions() []EquipmentEffectDefinition {
	equipmentEffectRegistry.RLock()
	defer equipmentEffectRegistry.RUnlock()
	out := make([]EquipmentEffectDefinition, 0, len(equipmentEffectRegistry.defs))
	for _, d := range equipmentEffectRegistry.defs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (e RuntimeEquipmentEffect) Registered() bool {
	d, ok := EquipmentEffectDefinitionByID(e.DefinitionID)
	return ok && e.ID != "" && d.SourceKind == e.SourceKind
}
func init() {
	for _, p := range LegendaryPowers() {
		RegisterEquipmentEffectDefinition(EquipmentEffectDefinition{p.ID, EffectSourceLegendary, "legendary_hooks", p.Effect})
	}
	for kind, name := range uniqueTraitNames {
		RegisterEquipmentEffectDefinition(EquipmentEffectDefinition{"unique:" + kind, EffectSourceUniqueTrait, "unique_context", name})
	}
	for _, kind := range []string{"skill_variant", "magic_find_pct", "element_damage_bp", "element_resist_bp", "enemy_element_pierce_bp", "element_mitigation_bp", "element_absorb_bp", "proc", "aura"} {
		RegisterEquipmentEffectDefinition(EquipmentEffectDefinition{"typed:" + kind, EffectSourceTyped, "typed:" + kind, "Typed equipment channel: " + kind})
	}
}
func RuntimeLegendaryEquipmentEffect(active ActiveLegendaryPower, producer bool) RuntimeEquipmentEffect {
	return RuntimeEquipmentEffect{DefinitionID: active.Power.ID, ID: active.Power.ID, SourceKind: EffectSourceLegendary, SourceUID: active.SourceUID, Item: active.Item, Trigger: "legendary_hooks", Description: active.Power.Effect, Parameters: EquipmentEffectParameters{Legendary: &active}, Producer: producer}
}
func RuntimeTypedEquipmentEffect(active ActiveEquipmentEffect, producer bool) RuntimeEquipmentEffect {
	e := CloneEquipmentEffects([]EquipmentEffect{active.Effect})[0]
	id, trigger := "typed:"+e.Kind+":"+e.Element, "typed:"+e.Kind
	if e.Proc != nil {
		id, trigger = "equipment:"+e.Proc.ID, e.Proc.Trigger
	}
	if e.Aura != nil {
		id = "aura:" + e.Aura.ID
	}
	if e.VariantID != "" {
		id = "variant:" + e.VariantID
	}
	d, _ := EquipmentEffectDefinitionByID("typed:" + e.Kind)
	return RuntimeEquipmentEffect{DefinitionID: d.ID, ID: id, SourceKind: EffectSourceTyped, SourceUID: active.SourceUID, Item: active.Item, Trigger: trigger, Description: d.Description, Parameters: EquipmentEffectParameters{Typed: &e}, Producer: producer}
}
func RuntimeUniqueEquipmentEffect(trait UniqueTrait) RuntimeEquipmentEffect {
	d, _ := EquipmentEffectDefinitionByID("unique:" + trait.Kind)
	return RuntimeEquipmentEffect{DefinitionID: d.ID, ID: fmt.Sprintf("unique:%s:%d", trait.Kind, trait.SkillID), SourceKind: EffectSourceUniqueTrait, Trigger: d.Trigger, Description: trait.Describe(10000), Parameters: EquipmentEffectParameters{Unique: &trait}}
}

// DispatchEquipmentEffect is also usable by domain-only pure evaluators and
// accepted-source snapshots. Scene adds causal admission for event producers.
func DispatchEquipmentEffect(effect RuntimeEquipmentEffect, fn func()) bool {
	if fn == nil || !effect.Registered() {
		return false
	}
	fn()
	return true
}
