package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

const UniqueCatalogVersion = "xianyi-v1-1619"
const UniqueCatalogCount = 1619
const UniqueRollMinBP int32 = 8000
const UniqueRollMaxBP int32 = 12000

type UniqueTrait struct {
	Kind         string
	PowerBP      int32
	ThresholdPct int32
}

type UniqueDefinition struct {
	ID                                       int32
	BaseID                                   ItemID
	BaseName, Name, Lore, LoreRoot, Role     string
	Professions                              uint8
	BeginnerAllowed, Unrestricted            bool
	Sex                                      uint8
	Slot, LevelRequirement, ResourceLevel    int32
	NativeDropEligible, LegacyAffixPoolReady bool
	Affixes                                  []Affix
	Traits                                   []UniqueTrait
	SourceArm, SourceDesc                    string
}

// Frozen rows are split by class so each source remains small and reviewable.
//
//go:embed unique_catalog_*.json
var uniqueCatalogFiles embed.FS

var uniqueDefinitions, uniqueByBase = loadUniqueCatalog()

func loadUniqueCatalog() ([]UniqueDefinition, map[ItemID]UniqueDefinition) {
	entries, err := uniqueCatalogFiles.ReadDir(".")
	if err != nil {
		panic(err)
	}
	var all []UniqueDefinition
	byBase := make(map[ItemID]UniqueDefinition)
	names := make(map[string]bool)
	for _, entry := range entries {
		body, err := uniqueCatalogFiles.ReadFile(entry.Name())
		if err != nil {
			panic(err)
		}
		var rows []UniqueDefinition
		if err := json.Unmarshal(body, &rows); err != nil {
			panic(err)
		}
		for _, d := range rows {
			if d.BaseID <= 0 || d.ID != int32(d.BaseID) || d.Name == "" || names[d.Name] || byBase[d.BaseID].ID != 0 || len(d.Affixes) != 3 || len(d.Traits) != 2 {
				panic(fmt.Sprintf("invalid unique catalogue row %d", d.BaseID))
			}
			for _, tr := range d.Traits {
				if !SupportedUniqueTrait(tr.Kind) || tr.PowerBP <= 0 {
					panic(fmt.Sprintf("invalid unique trait %d/%s", d.BaseID, tr.Kind))
				}
			}
			names[d.Name] = true
			byBase[d.BaseID] = d
			all = append(all, d)
		}
	}
	if len(all) != UniqueCatalogCount {
		panic(fmt.Sprintf("unique catalogue has %d rows, want %d", len(all), UniqueCatalogCount))
	}
	sort.Slice(all, func(i, j int) bool { return all[i].BaseID < all[j].BaseID })
	return all, byBase
}

func cloneUnique(d UniqueDefinition) UniqueDefinition {
	d.Affixes = append([]Affix(nil), d.Affixes...)
	d.Traits = append([]UniqueTrait(nil), d.Traits...)
	return d
}
func UniqueCatalog() []UniqueDefinition {
	out := make([]UniqueDefinition, len(uniqueDefinitions))
	for i, d := range uniqueDefinitions {
		out[i] = cloneUnique(d)
	}
	return out
}
func UniqueForBase(id ItemID) (UniqueDefinition, bool) {
	d, ok := uniqueByBase[id]
	return cloneUnique(d), ok
}
func UniqueOf(st Stack) (UniqueDefinition, bool) {
	if st.UID == 0 || st.Count != 1 || st.Endgame.UniqueID != int32(st.Item) || st.Endgame.UniqueRollBP < UniqueRollMinBP || st.Endgame.UniqueRollBP > UniqueRollMaxBP {
		return UniqueDefinition{}, false
	}
	return UniqueForBase(st.Item)
}
func UniqueName(st Stack) (string, bool) { d, ok := UniqueOf(st); return d.Name, ok }
func ApplyUnique(st *Stack, def ItemDef, intn func(int) int) bool {
	if st == nil || st.UID == 0 || st.Count != 1 || st.Item != def.ID || def.Equip == nil || st.Endgame.UniqueID != 0 || intn == nil {
		return false
	}
	if _, ok := uniqueByBase[st.Item]; !ok {
		return false
	}
	roll := intn(int(UniqueRollMaxBP - UniqueRollMinBP + 1))
	if roll < 0 || roll > int(UniqueRollMaxBP-UniqueRollMinBP) {
		return false
	}
	st.Endgame.UniqueID = int32(st.Item)
	st.Endgame.UniqueRollBP = UniqueRollMinBP + int32(roll)
	st.Endgame.Revision++
	return true
}
func RollUniqueDrop(st *Stack, def ItemDef, intn func(int) int) bool {
	if st == nil || intn == nil || def.Equip == nil || def.Equip.NoTypeDrop {
		return false
	}
	d, ok := uniqueByBase[st.Item]
	if !ok || !d.NativeDropEligible {
		return false
	}
	if roll := intn(100); roll != 0 {
		return false
	}
	return ApplyUnique(st, def, intn)
}
func UniqueAffixes(st Stack) []Affix {
	d, ok := UniqueOf(st)
	if !ok {
		return nil
	}
	for i := range d.Affixes {
		d.Affixes[i].Value = scaledUniquePower(d.Affixes[i].Value, st.Endgame.UniqueRollBP)
	}
	return d.Affixes
}
func ValidateUniqueCatalog(defs map[ItemID]ItemDef) error {
	count := 0
	for id, def := range defs {
		if id <= 0 || def.Equip == nil || def.Equip.NoTypeDrop {
			continue
		}
		count++
		d, ok := uniqueByBase[id]
		if !ok {
			return fmt.Errorf("eligible equipment %d lacks unique", id)
		}
		e := def.Equip
		if d.Slot != e.Slot || d.Professions != e.Need.Professions || d.BeginnerAllowed != e.Need.BeginnerAllowed || d.Sex != e.Need.Sex || d.LevelRequirement != e.LevelReq {
			return fmt.Errorf("unique %d requirement metadata differs from loaded equipment", id)
		}
	}
	if count != UniqueCatalogCount {
		return fmt.Errorf("eligible equipment count %d differs from catalogue %d", count, UniqueCatalogCount)
	}
	return nil
}
