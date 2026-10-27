package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// LegendaryPowerDefinition is approved design metadata, not executable code.
// Mechanics are explicitly registered by the authoritative scene runtime.
type LegendaryPowerDefinition struct {
	DesignRevision string   `json:"design_revision,omitempty"`
	DesignDecision string   `json:"design_decision,omitempty"`
	DesignStatus   string   `json:"design_status,omitempty"`
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Class          string   `json:"class"`
	MinLevel       int32    `json:"min_level"`
	Slots          []string `json:"slots"`
	Effect         string   `json:"effect"`
	DesignNote     string   `json:"design_note"`
	Implementation string   `json:"implementation"`
	SkillBasis     string   `json:"skill_basis"`
	RevisionNote   string   `json:"revision_note,omitempty"`
}

//go:embed legendary_powers.json
var legendaryPowerFS embed.FS
var legendaryPowerDefinitions = loadLegendaryPowers()

func loadLegendaryPowers() map[string]LegendaryPowerDefinition {
	data, err := legendaryPowerFS.ReadFile("legendary_powers.json")
	if err != nil {
		panic(err)
	}
	var defs []LegendaryPowerDefinition
	if err = json.Unmarshal(data, &defs); err != nil {
		panic(err)
	}
	out := make(map[string]LegendaryPowerDefinition, len(defs))
	for _, p := range defs {
		if p.ID == "" || p.Name == "" || p.Effect == "" || p.MinLevel < 1 {
			panic("invalid legendary power")
		}
		if _, ok := out[p.ID]; ok {
			panic("duplicate legendary power " + p.ID)
		}
		out[p.ID] = p
	}
	if len(out) != 330 {
		panic(fmt.Sprintf("legendary design count: %d", len(out)))
	}
	return out
}
func LegendaryPower(id string) (LegendaryPowerDefinition, bool) {
	p, ok := legendaryPowerDefinitions[id]
	p.Slots = append([]string(nil), p.Slots...)
	return p, ok
}
func LegendaryPowers() []LegendaryPowerDefinition {
	out := make([]LegendaryPowerDefinition, 0, len(legendaryPowerDefinitions))
	for id := range legendaryPowerDefinitions {
		p, _ := LegendaryPower(id)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func legendaryClassMask(class string) uint8 {
	switch class {
	case "战士":
		return 1
	case "剑客":
		return 2
	case "刺客":
		return 4
	case "药师":
		return 8
	case "术士":
		return 16
	case "全职业":
		return 31
	}
	return 0
}

// LegendaryPowerEligible uses the legacy generation-level fallback for callers
// with no generation context. Monster drops must use LegendaryPowerEligibleAtLevel.
func LegendaryPowerEligible(p LegendaryPowerDefinition, base ItemDef) bool {
	return LegendaryPowerEligibleAtLevel(p, base, LegendaryGenerationLevel(base, Stack{}))
}

// LegendaryGenerationLevel retains the immutable origin of a monster drop through
// awakening and reforge. Zero is an old/non-monster item without provenance, so
// its historical base-level behavior remains intact; player level is never used.
func LegendaryGenerationLevel(base ItemDef, st Stack) int32 {
	if st.Endgame.SourceLevel != 0 {
		return st.Endgame.SourceLevel
	}
	if base.Equip != nil {
		return base.Equip.LevelReq
	}
	return 0
}

func LegendaryPowerEligibleForItem(p LegendaryPowerDefinition, base ItemDef, st Stack) bool {
	// Owned instances are never rerolled. Accept the new all-slot profession
	// placement as well as every historically legal placement. Generation uses
	// its separately frozen policy, not this runtime compatibility predicate.
	return legendaryPowerEligibleAllClassSlots(p, base, LegendaryGenerationLevel(base, st))
}

// LegendaryPowerSourceLevelEligible validates provenance for every power, but
// only profession-specific powers retain a minimum drop-level requirement.
// Universal A/D/U powers keep catalog MinLevel as historical design metadata.
func LegendaryPowerSourceLevelEligible(p LegendaryPowerDefinition, sourceLevel int32) bool {
	return sourceLevel > 0 && (p.Class == "全职业" || p.MinLevel > 0 && sourceLevel >= p.MinLevel)
}

// LegendaryPowerEligibleAtLevel separates the monster's generation level from
// the item's unchanged wear and native affix levels. Slot/type/class restrictions
// remain authoritative; neither the looter's level nor learned skills are inputs.
func LegendaryPowerEligibleAtLevel(p LegendaryPowerDefinition, base ItemDef, sourceLevel int32) bool {
	e := base.Equip
	if e == nil || !LegendaryPowerSourceLevelEligible(p, sourceLevel) {
		return false
	}
	parts := strings.Split(p.ID, "-")
	if len(parts) != 2 || len(parts[1]) < 1 {
		return false
	}
	slot := EquipSlot(e.Slot)
	allowed := false
	switch parts[1][0] {
	case 'A':
		allowed = slot == SlotWeapon || slot == SlotTwoHand || slot == SlotRing || slot == SlotNeck || slot == SlotBody || slot == SlotHead || slot == SlotGlove || slot == SlotShoe
	case 'D':
		allowed = slot == SlotBody || slot == SlotHead || slot == SlotGlove || slot == SlotFace || slot == SlotBag || slot == SlotShield || slot == SlotShoe
	case 'U':
		allowed = slot == SlotWeapon || slot == SlotTwoHand || slot == SlotRing || slot == SlotNeck || slot == SlotFace || slot == SlotBag || slot == SlotShield || slot == SlotShoe
	}
	if !allowed {
		return false
	}
	if p.Class == "全职业" {
		return true
	}
	if slot != SlotWeapon && slot != SlotTwoHand && slot != SlotBody && slot != SlotHead {
		return false
	}
	mask := legendaryClassMask(p.Class)
	return mask != 0 && e.Need.Professions&mask != 0
}

type ActiveLegendaryPower struct {
	SourceUID   int64
	PowerRollBP int32
	Item        ItemID
	Power       LegendaryPowerDefinition
}

// Duplicate powers never stack. Choose a stable instance, independently of
// iteration order, and honor functional wear and native requirements.
func ActiveLegendaryPowers(worn *EquipSet, defs func(ItemID) (ItemDef, bool), ch *Character) []ActiveLegendaryPower {
	if worn == nil || defs == nil || ch == nil {
		return nil
	}
	selected := map[string]ActiveLegendaryPower{}
	worn.Each(func(slot EquipSlot, st Stack) {
		d, ok := defs(st.Item)
		if !ok || d.Equip == nil || int32(slot) != d.Equip.Slot || !EquipmentFunctional(st, d) || !d.Equip.Need.AllowsCharacter(ch) || ch.Level < EquipmentRequiredLevel(d, st) {
			return
		}
		accept := func(p LegendaryPowerDefinition, roll int32) {
			var valid bool
			p, valid = LegendaryPowerForRoll(p.ID, roll)
			if !valid {
				return
			}
			if p.Class != "全职业" && (!ch.HasProfession() || legendaryClassMask(p.Class)&(1<<uint8(ch.Race)) == 0) {
				return
			}
			if old, exists := selected[p.ID]; !exists || EffectiveLegendaryPowerRollBP(p.ID, roll) > EffectiveLegendaryPowerRollBP(p.ID, old.PowerRollBP) || EffectiveLegendaryPowerRollBP(p.ID, roll) == EffectiveLegendaryPowerRollBP(p.ID, old.PowerRollBP) && st.UID < old.SourceUID {
				selected[p.ID] = ActiveLegendaryPower{SourceUID: st.UID, PowerRollBP: roll, Item: st.Item, Power: p}
			}
		}
		if p, valid := LegendaryPower(st.Legendary.PowerID); valid && LegendaryPowerEligibleForItem(p, d, st) {
			accept(p, st.Legendary.PowerRollBP)
		}
		// Wing sockets deliberately accept A/D/U independently of the original
		// donor slot, but never bypass the wearer's class or the wing's wear state.
		if slot == SlotBag && st.Legendary.PowerID == "" && st.WingPowers.Validate() == nil {
			for _, power := range st.WingPowers {
				if p, valid := LegendaryPower(power.PowerID); valid && LegendaryPowerExecutable(p.ID) {
					accept(p, power.PowerRollBP)
				}
			}
		}
	})
	out := make([]ActiveLegendaryPower, 0, len(selected))
	for _, p := range selected {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Power.ID < out[j].Power.ID })
	return out
}

// Registration is startup-only. Catalog presence alone never permits a drop.
var legendaryExecutable = map[string]bool{}

func RegisterLegendaryPowerExecutable(id string) {
	if _, ok := legendaryPowerDefinitions[id]; !ok {
		panic("unknown legendary handler " + id)
	}
	legendaryExecutable[id] = true
}
func LegendaryPowerExecutable(id string) bool { return legendaryExecutable[id] }

// Natural replacement is not enabled by a partially registered catalog.
func LegendaryCatalogComplete() bool {
	for id := range legendaryPowerDefinitions {
		if !legendaryExecutable[id] {
			return false
		}
	}
	return true
}
