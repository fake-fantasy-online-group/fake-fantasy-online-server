package domain

import "fmt"

// EquipmentEffect is a versioned, typed channel. Values are basis points except
// magic_find_pct (percentage points). It never reinterprets legacy numeric attrs.
type EquipmentEffect struct {
	VariantID string         `json:"variantId,omitempty"`
	Kind      string         `json:"kind"`
	Element   string         `json:"element,omitempty"`
	Value     int32          `json:"value,omitempty"`
	Proc      *EquipmentProc `json:"proc,omitempty"`
	Aura      *EquipmentAura `json:"aura,omitempty"`
}
type EquipmentProc struct {
	ID           string  `json:"id"`
	Trigger      string  `json:"trigger"`
	SkillID      SkillID `json:"skillId"`
	FixedRank    int32   `json:"fixedRank"`
	ChanceBP     int32   `json:"chanceBP"`
	ICDMS        int32   `json:"icdMs"`
	SharedGroup  string  `json:"sharedGroup"`
	Payload      string  `json:"payload"`
	TargetRule   string  `json:"targetRule"`
	MaxTargets   int32   `json:"maxTargets"`
	ScaleBP      int32   `json:"scaleBP"`
	PVPScaleBP   int32   `json:"pvpScaleBP"`
	HPBelowPct   int32   `json:"hpBelowPct,omitempty"`
	RequireMagic bool    `json:"requireMagic,omitempty"`
	Priority     int32   `json:"priority,omitempty"`
	VisualKey    string  `json:"visualKey,omitempty"`
}
type EquipmentAura struct {
	ID               string `json:"id"`
	FixedRank        int32  `json:"fixedRank"`
	Family           string `json:"family"`
	Modifier         string `json:"modifier"`
	Element          string `json:"element,omitempty"`
	Value            int32  `json:"value"`
	RadiusPX         int32  `json:"radiusPx"`
	RefreshMS        int32  `json:"refreshMs"`
	LeaseMS          int32  `json:"leaseMs"`
	MaxPlayers       int32  `json:"maxPlayers"`
	IncludeActivePet bool   `json:"includeActivePet"`
	VisualKey        string `json:"visualKey"`
	Priority         int32  `json:"priority,omitempty"`
	PVPScaleBP       int32  `json:"pvpScaleBP"`
}
type ActiveEquipmentEffect struct {
	SourceUID int64           `json:"sourceUID,string"`
	Item      ItemID          `json:"item"`
	Effect    EquipmentEffect `json:"effect"`
}

func CloneEquipmentEffects(in []EquipmentEffect) []EquipmentEffect {
	out := append([]EquipmentEffect(nil), in...)
	for i := range out {
		if out[i].Proc != nil {
			v := *out[i].Proc
			out[i].Proc = &v
		}
		if out[i].Aura != nil {
			v := *out[i].Aura
			out[i].Aura = &v
		}
	}
	return out
}
func ElementIndex(element string) int {
	switch element {
	case "fire":
		return 0
	case "ice":
		return 1
	case "holy":
		return 2
	case "dark":
		return 3
	}
	return -1
}
func HurtElementIndex(hurt int32) int {
	switch hurt {
	case HurtFire:
		return 0
	case HurtIce:
		return 1
	case HurtHoly:
		return 2
	case HurtDark:
		return 3
	}
	return -1
}

func ValidateEquipmentEffects(effects []EquipmentEffect, catalog *EndgameCatalog) error {
	c := catalog
	if len(effects) > 16 {
		return fmt.Errorf("too many typed equipment effects")
	}
	for _, e := range effects {
		valid := false
		if e.Kind != "skill_variant" && e.VariantID != "" {
			return fmt.Errorf("variant ID on non-variant effect")
		}
		switch e.Kind {
		case "skill_variant":
			_, known := c.Variant(e.VariantID)
			valid = known && e.Value == 0 && e.Element == "" && e.Proc == nil && e.Aura == nil
		case "magic_find_pct":
			valid = e.Value >= 0 && e.Value <= 1000000 && e.Element == "" && e.Proc == nil && e.Aura == nil
		case "element_damage_bp", "element_resist_bp", "enemy_element_pierce_bp", "element_mitigation_bp", "element_absorb_bp":
			valid = ElementIndex(e.Element) >= 0 && e.Value >= 0 && e.Value <= 100000 && e.Proc == nil && e.Aura == nil
		case "proc":
			p := e.Proc
			if p != nil && e.Aura == nil && e.Value == 0 && e.Element == "" {
				valid = p.ID != "" && p.SharedGroup != "" && p.FixedRank > 0 && p.FixedRank <= 100000 && p.ChanceBP >= 0 && p.ChanceBP <= 10000 && p.ICDMS >= 4000 && p.ICDMS <= 60000 && p.MaxTargets == 1 && p.ScaleBP > 0 && p.ScaleBP <= 10000 && p.PVPScaleBP >= 0 && p.PVPScaleBP <= 10000 && p.HPBelowPct >= 0 && p.HPBelowPct <= 100
				valid = valid && (p.Trigger == "on_direct_hit" || p.Trigger == "on_direct_damage_taken" || p.Trigger == "on_attack")
				if p.Payload == "heal" {
					valid = valid && p.SkillID > 0 && p.TargetRule == "self"
				} else {
					valid = valid && p.Payload == "damage" && p.TargetRule == "hit_target" && p.SkillID > 0
				}
			}
		case "aura":
			a := e.Aura
			if a != nil && e.Proc == nil && e.Value == 0 && e.Element == "" {
				valid = a.ID != "" && a.Family != "" && a.FixedRank > 0 && a.Value > 0 && a.Value <= 4500 && a.RadiusPX > 0 && a.RadiusPX <= 2000 && a.RefreshMS >= int32(TickMS) && a.RefreshMS <= 60000 && a.RefreshMS%int32(TickMS) == 0 && a.LeaseMS >= a.RefreshMS && a.LeaseMS <= 120000 && a.LeaseMS%int32(TickMS) == 0 && a.MaxPlayers >= 1 && a.MaxPlayers <= 6 && a.VisualKey != "" && a.PVPScaleBP >= 0 && a.PVPScaleBP <= 10000 && ((a.Modifier == "physical_mitigation_bp" && a.Element == "") || ((a.Modifier == "element_resist_bp" || a.Modifier == "element_damage_bp") && ElementIndex(a.Element) >= 0))
			}
		}
		if !valid {
			return fmt.Errorf("invalid typed equipment effect %s/%s", e.Kind, e.Element)
		}
	}
	return nil
}

// ActiveEquipmentEffects uses the same effective-wear and formation winner gates
// as ordinary stats. No effect is inherited by a pet from its owner's equipment.
func ActiveEquipmentEffects(worn *EquipSet, defs func(ItemID) (ItemDef, bool), ch *Character) []ActiveEquipmentEffect {
	if worn == nil || defs == nil || ch == nil {
		return nil
	}
	var out []ActiveEquipmentEffect
	worn.Each(func(slot EquipSlot, st Stack) {
		def, ok := defs(st.Item)
		if !ok || def.Equip == nil || int32(slot) != def.Equip.Slot || !EquipmentFunctional(st, def) || !def.Equip.Need.AllowsCharacter(ch) || ch.Level < EquipmentRequiredLevel(def, st) {
			return
		}
		if d, ok := UniqueOf(st, def.EndgameCatalog); ok {
			for _, e := range d.TypedEffects {
				out = append(out, ActiveEquipmentEffect{st.UID, st.Item, e})
			}
		}
	})
	for _, a := range ActiveAstralRecipes(worn, defs, ch) {
		for _, e := range a.Recipe.TypedEffects {
			out = append(out, ActiveEquipmentEffect{a.UID, a.Item, e})
		}
	}
	return out
}

// ValidateEquipmentRuntimeV5 refuses content whose advertised fixed rank has no
// executable approved payload. Catalog-only validation cannot establish this.
func ValidateEquipmentRuntimeV5(skills SkillTable, catalog *EndgameCatalog) error {
	c := catalog
	if c == nil {
		return fmt.Errorf("endgame catalog is not loaded")
	}
	check := func(effects []EquipmentEffect) error {
		if err := ValidateEquipmentEffects(effects, c); err != nil {
			return err
		}
		procs, auras := 0, 0
		variantSkills := map[SkillID]bool{}
		for _, e := range effects {
			if e.Kind == "skill_variant" {
				v, _ := c.Variant(e.VariantID)
				def, ok := skills.Get(v.SkillID, 1)
				if !ok || !def.Usable() || def.Prof != v.Profession || variantSkills[v.SkillID] {
					return fmt.Errorf("invalid or duplicate skill variant %s", v.ID)
				}
				expected := EffectDamage
				if v.Behavior == VariantDelayedHeal {
					expected = EffectHeal
				}
				executable := false
				for _, effect := range def.DirectEffects() {
					executable = executable || effect.Kind == expected
				}
				if !executable {
					return fmt.Errorf("skill variant %s has no required payload", v.ID)
				}
				if v.Behavior == VariantDetonateOwnedTrap {
					trap, exists := skills.Get(v.Params.TrapSkillID, 1)
					placed, damage := false, false
					for _, effect := range trap.DirectEffects() {
						placed = placed || effect.Kind == EffectPlaceTrap
						damage = damage || effect.Kind == EffectDamage
					}
					if !exists || trap.Prof != v.Profession || !placed || !damage {
						return fmt.Errorf("skill variant %s has no executable bomb", v.ID)
					}
				}
				variantSkills[v.SkillID] = true
			}
			if e.Aura != nil {
				auras++
			}
			if p := e.Proc; p != nil {
				procs++
				def, ok := skills.Get(p.SkillID, p.FixedRank)
				if !ok || def.Level != p.FixedRank {
					return fmt.Errorf("equipment proc %s unresolved skill %d rank %d", p.ID, p.SkillID, p.FixedRank)
				}
				allowed := false
				for _, effect := range def.DirectEffects() {
					allowed = allowed || (p.Payload == "damage" && effect.Kind == EffectDamage) || (p.Payload == "heal" && effect.Kind == EffectHeal)
				}
				if !allowed {
					return fmt.Errorf("equipment proc %s has no approved %s payload", p.ID, p.Payload)
				}
			}
		}
		if procs > 1 || auras > 1 || procs > 0 && auras > 0 {
			return fmt.Errorf("equipment proc/aura content budgets exceeded")
		}
		return nil
	}
	for _, version := range []int32{0, 5, UniqueContentVersion} {
		for _, d := range c.Uniques(version) {
			if err := check(d.TypedEffects); err != nil {
				return fmt.Errorf("unique %d/%d: %w", version, d.ID, err)
			}
		}
		for _, d := range c.Recipes(version) {
			if err := check(d.TypedEffects); err != nil {
				return fmt.Errorf("formation %d/%d: %w", version, d.ID, err)
			}
		}
	}
	return nil
}
