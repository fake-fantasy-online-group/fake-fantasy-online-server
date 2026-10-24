package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Explicit stable IDs, not name matching. These IDs fit the signed native wire
// representation and never overlap the 1.9-billion endgame item allocation.
const HardDungeonItemOffset ItemID = 1800000000

//go:embed dungeon_equipment_manifest.json
var dungeonEquipmentJSON []byte

type DungeonEquipmentWeapon struct {
	BeginnerAllowedOverride bool            `json:"beginner_allowed_override"`
	TaskOriginal            bool            `json:"task_original"`
	MechanismEnabled        bool            `json:"mechanism_enabled"`
	BaseID                  ItemID          `json:"base_id"`
	HardID                  ItemID          `json:"hard_id"`
	Name                    string          `json:"name"`
	Affixes                 []Affix         `json:"affixes"`
	Effect                  json.RawMessage `json:"effect"`
	Sources                 [][2]int32      `json:"sources"`
}
type DungeonEquipmentSource struct {
	BaseID    ItemID    `json:"base_id"`
	MapID     int32     `json:"map_id"`
	MonsterID MonsterID `json:"monster_id"`
}
type DungeonEquipmentAddedDrop struct {
	DungeonEquipmentSource
	RatePct float64 `json:"rate_pct"`
}

type DungeonEquipmentSet struct {
	BaseID        int32                `json:"base_id"`
	HardID        int32                `json:"hard_id"`
	Members       []ItemID             `json:"members"`
	NormalBonuses []EquipmentSuitBonus `json:"normal_bonuses"`
}

type dungeonEquipmentCatalog struct {
	Weapons    []DungeonEquipmentWeapon
	Sets       []DungeonEquipmentSet
	Sources    []DungeonEquipmentSource
	AddedDrops []DungeonEquipmentAddedDrop `json:"added_drops"`
}

var dungeonEquipmentManifest = func() dungeonEquipmentCatalog {
	var m dungeonEquipmentCatalog
	if err := json.Unmarshal(dungeonEquipmentJSON, &m); err != nil {
		panic(err)
	}
	return m
}()

func EnhancedWeaponBaseID(id ItemID) ItemID {
	for _, w := range dungeonEquipmentManifest.Weapons {
		if w.MechanismEnabled && ((w.HardID != 0 && id == w.HardID) || (w.TaskOriginal && id == w.BaseID)) {
			return w.BaseID
		}
	}
	return 0
}

// RegisterDungeonEquipment derives each version once from the loaded native
// template. It changes catalog definitions only, never persisted player stacks.
// Repeated invocation is rejected rather than compounding bonuses.
func RegisterDungeonEquipment(items map[ItemID]ItemDef) error {
	for _, w := range dungeonEquipmentManifest.Weapons {
		if w.HardID != 0 {
			if _, exists := items[w.HardID]; exists {
				return fmt.Errorf("hard dungeon item ID collision: %d", w.HardID)
			}
		}
	}
	for _, set := range dungeonEquipmentManifest.Sets {
		for _, id := range set.Members {
			if _, exists := items[HardDungeonItemOffset+id]; exists {
				return fmt.Errorf("hard dungeon set item ID collision: %d", id)
			}
		}
	}
	// Work against copies until validation is complete.
	changes := make(map[ItemID]ItemDef)
	for _, w := range dungeonEquipmentManifest.Weapons {
		if w.HardID == 0 && !w.TaskOriginal {
			continue
		}
		base, ok := items[w.BaseID]
		if !ok || base.Equip == nil {
			return fmt.Errorf("missing dungeon weapon base: %d", w.BaseID)
		}
		def := base
		equip := *base.Equip
		def.Equip = &equip
		equip.Affixes = append([]Affix(nil), w.Affixes...)
		if w.TaskOriginal && w.BeginnerAllowedOverride {
			equip.Need.BeginnerAllowed = true
		}
		def.Description = strings.TrimSpace(base.Description + "\n" + dungeonWeaponEffectText(w.Effect))
		if w.HardID != 0 {
			def.ID = w.HardID
			def.Name = prefixedEquipmentName("真*", base.Name)
			def.HardDungeonOnly = true
			def.DungeonBaseID = base.ID
			equip.NativeDropEligible = false
			equip.NoTypeDrop = true
		}
		changes[def.ID] = def
	}
	for _, set := range dungeonEquipmentManifest.Sets {
		var original *EquipmentSuit
		for _, id := range set.Members {
			def, ok := items[id]
			if !ok || def.Equip == nil || def.Equip.Suit == nil || def.Equip.Suit.ID != set.BaseID {
				return fmt.Errorf("missing class set member: %d/%d", set.BaseID, id)
			}
			original = def.Equip.Suit
		}
		hard := *original
		hard.ID = set.HardID
		hard.Name = prefixedEquipmentName("真*", original.Name)
		hard.EnhancedClass = set.BaseID
		hard.Bonuses = append([]EquipmentSuitBonus(nil), original.Bonuses...)
		hard.Groups = make([][]ItemID, len(original.Groups))
		for i, g := range original.Groups {
			for _, id := range g {
				hard.Groups[i] = append(hard.Groups[i], HardDungeonItemOffset+id)
			}
		}
		normal := *original
		normal.EnhancedClass = 0
		normal.Bonuses = append([]EquipmentSuitBonus(nil), set.NormalBonuses...)
		for _, id := range set.Members {
			base := items[id]
			n := base
			ne := *base.Equip
			n.Equip = &ne
			ne.Suit = &normal
			changes[id] = n
			h := base
			he := *base.Equip
			h.Equip = &he
			h.ID = HardDungeonItemOffset + id
			h.Name = prefixedEquipmentName("真*", base.Name)
			h.HardDungeonOnly = true
			h.DungeonBaseID = id
			he.Suit = &hard
			he.NoTypeDrop = true
			he.NativeDropEligible = false
			changes[h.ID] = h
		}
	}
	for id, def := range changes {
		items[id] = def
	}
	return nil
}

// HardDungeonDropVariant is an exact item/map/monster provenance allowlist.
// A known name, a copied monster template, or merely being in an instance does
// not authorize a hard-mode item to drop.
func HardDungeonDropVariant(item ItemID, mapID int32, monster MonsterID) ItemID {
	for _, source := range dungeonEquipmentManifest.Sources {
		if source.BaseID == item && source.MapID == mapID && source.MonsterID == monster {
			return HardDungeonItemOffset + item
		}
	}
	return 0
}

// DungeonEquipmentIconMappings copies a base item's explicit native icon
// override first, falling back to its loaded icon. Runtime-only hard IDs do not
// exist in the client's static resource table, so they need 0x804d mappings.
func DungeonEquipmentIconMappings(existing []ItemIconMapping, items map[ItemID]ItemDef) ([]ItemIconMapping, error) {
	out := append([]ItemIconMapping(nil), existing...)
	icons := map[ItemID]int32{}
	for _, m := range existing {
		icons[m.Item] = m.Icon
	}
	add := func(id ItemID) error {
		def, ok := items[id]
		if !ok || !def.HardDungeonOnly {
			return fmt.Errorf("hard icon template missing: %d", id)
		}
		icon := icons[def.DungeonBaseID]
		if icon <= 0 {
			icon = def.Icon
		}
		if icon <= 0 {
			return fmt.Errorf("hard item base icon missing: %d", id)
		}
		if old, exists := icons[id]; exists {
			if old != icon {
				return fmt.Errorf("hard item icon collision: %d", id)
			}
			return nil
		}
		icons[id] = icon
		out = append(out, ItemIconMapping{Item: id, Icon: icon})
		return nil
	}
	for _, w := range dungeonEquipmentManifest.Weapons {
		if w.HardID != 0 {
			if err := add(w.HardID); err != nil {
				return nil, err
			}
		}
	}
	for _, set := range dungeonEquipmentManifest.Sets {
		for _, id := range set.Members {
			if err := add(HardDungeonItemOffset + id); err != nil {
				return nil, err
			}
		}
	}
	if len(out) > 65535 {
		return nil, fmt.Errorf("item icon mappings exceed U16 count")
	}
	return out, nil
}

func dungeonWeaponEffectText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var byClass map[string]string
	if err := json.Unmarshal(raw, &byClass); err != nil {
		return ""
	}
	names := make([]string, 0, len(byClass))
	for name := range byClass {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, name+"："+byClass[name])
	}
	return strings.Join(lines, "\n")
}

// AdditionalLostTempleDrops is an explicit additive map/monster allowlist.
// Only eight newly assigned ancient weapons occur here; the four existing
// copied drops retain their original rows and probabilities.
func AdditionalLostTempleDrops(mapID int32, monster MonsterID) []DropEntry {
	var out []DropEntry
	for _, d := range dungeonEquipmentManifest.AddedDrops {
		if d.MapID == mapID && d.MonsterID == monster {
			out = append(out, DropEntry{Item: d.BaseID, RatePct: d.RatePct, DungeonOnly: true})
		}
	}
	return out
}
