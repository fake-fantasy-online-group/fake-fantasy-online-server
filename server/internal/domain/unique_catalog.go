package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

const UniqueCatalogVersion = "endgame-v5-scope259"
const UniqueContentVersion int32 = 5
const UniqueCatalogCount = 259
const UniqueRollMinBP int32 = 8000
const UniqueRollMaxBP int32 = 12000

type UniqueTrait struct {
	Kind         string
	PowerBP      int32
	ThresholdPct int32
	SkillID      SkillID // optional filter on an existing skill, not a granted skill
	SkillName    string
}

type UniqueDefinition struct {
	ID                                       int32
	BaseID                                   ItemID
	BaseName, Name, Lore, LoreRoot, Role     string
	Identity, Benefit, Tradeoff, PowerTier   string
	ReservedSkills                           []ReservedSkillAttribute
	SkillBonuses                             []SkillRankBonus
	TypedEffects                             []EquipmentEffect
	Professions                              uint8
	BeginnerAllowed, Unrestricted            bool
	Sex                                      uint8
	Slot, LevelRequirement, ResourceLevel    int32
	NativeDropEligible, LegacyAffixPoolReady bool
	Affixes                                  []Affix
	VariableAffixes                          []VariableAffix
	LocalEnhancements                        []ItemLocalEnhancement
	Traits                                   []UniqueTrait
	SourceArm, SourceDesc                    string
}

// Frozen rows are split by class so each source remains small and reviewable.
//
//go:embed unique_catalog_*.json
var uniqueCatalogFiles embed.FS

// Frozen legacy definitions preserve derived values for every existing instance.
//
//go:embed legacy_unique_v4/*.json
var legacyUniqueCatalogFiles embed.FS

var legacyUniqueDefinitions, legacyUniqueByBase = loadUniqueCatalogFS(legacyUniqueCatalogFiles, "legacy_unique_v4", 1619, false)
var uniqueDefinitions, uniqueByBase = loadUniqueCatalogFS(uniqueCatalogFiles, ".", UniqueCatalogCount, true)

func loadUniqueCatalogFS(files embed.FS, directory string, expected int, v5 bool) ([]UniqueDefinition, map[ItemID]UniqueDefinition) {
	entries, err := files.ReadDir(directory)
	if err != nil {
		panic(err)
	}
	var all []UniqueDefinition
	byBase := make(map[ItemID]UniqueDefinition)
	names := make(map[string]bool)
	for _, entry := range entries {
		path := entry.Name()
		if directory != "." {
			path = directory + "/" + path
		}
		body, err := files.ReadFile(path)
		if err != nil {
			panic(err)
		}
		var rows []UniqueDefinition
		if err := json.Unmarshal(body, &rows); err != nil {
			panic(err)
		}
		for _, d := range rows {
			if names[d.Name] || byBase[d.BaseID].ID != 0 {
				panic(fmt.Sprintf("invalid unique catalogue row %d", d.BaseID))
			}
			if err := validateUniqueDefinition(d); err != nil {
				panic(err)
			}
			if v5 && (d.LevelRequirement > 60 || !d.NativeDropEligible || !d.LegacyAffixPoolReady) {
				panic(fmt.Sprintf("invalid V5 scope row %d", d.BaseID))
			}
			names[d.Name] = true
			byBase[d.BaseID] = d
			all = append(all, d)
		}
	}
	if len(all) != expected {
		panic(fmt.Sprintf("unique catalogue has %d rows, want %d", len(all), expected))
	}
	sort.Slice(all, func(i, j int) bool { return all[i].BaseID < all[j].BaseID })
	return all, byBase
}

func validateUniqueDefinition(d UniqueDefinition) error {
	if d.BaseID <= 0 || d.ID != int32(d.BaseID) || d.Name == "" || len(d.Traits) > 3 || !validEndgamePowerTier(d.PowerTier) {
		return fmt.Errorf("invalid unique catalogue row %d", d.BaseID)
	}
	if err := validateEndgameExtraAffixes(d.Affixes, true); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	if err := validateEndgameRollModel(d.Affixes, d.VariableAffixes, d.LocalEnhancements); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	if err := ValidateEquipmentEffects(d.TypedEffects); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	if err := ValidateSkillRankBonuses(d.SkillBonuses); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	for _, b := range d.ReservedSkills {
		if !b.Valid() {
			return fmt.Errorf("invalid reserved skill attribute %d", d.BaseID)
		}
	}
	for _, tr := range d.Traits {
		if !ValidUniqueTrait(tr) || !TraitMatchesProfessions(tr, d.Professions) {
			return fmt.Errorf("invalid unique trait %d/%s", d.BaseID, tr.Kind)
		}
	}
	return nil
}

func cloneUnique(d UniqueDefinition) UniqueDefinition {
	d.TypedEffects = CloneEquipmentEffects(d.TypedEffects)
	d.SkillBonuses = append([]SkillRankBonus(nil), d.SkillBonuses...)
	d.Affixes = append([]Affix(nil), d.Affixes...)
	d.VariableAffixes = append([]VariableAffix(nil), d.VariableAffixes...)
	d.LocalEnhancements = append([]ItemLocalEnhancement(nil), d.LocalEnhancements...)
	d.Traits = append([]UniqueTrait(nil), d.Traits...)
	d.ReservedSkills = append([]ReservedSkillAttribute(nil), d.ReservedSkills...)
	return d
}
func UniqueCatalog() []UniqueDefinition {
	out := make([]UniqueDefinition, len(uniqueDefinitions))
	for i, d := range uniqueDefinitions {
		out[i] = cloneUnique(d)
	}
	return out
}

// UniqueAppearanceBaseIDs retains already-published native sprite aliases for
// legacy-owned equipment. It is NOT an eligibility list for drops or awakening.
func UniqueAppearanceBaseIDs() []ItemID {
	out := make([]ItemID, len(legacyUniqueDefinitions))
	for i, d := range legacyUniqueDefinitions {
		out[i] = d.BaseID
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
	switch st.Endgame.UniqueContentVersion {
	case 0:
		d, ok := legacyUniqueByBase[st.Item]
		return cloneUnique(d), ok
	case UniqueContentVersion:
		return UniqueForBase(st.Item)
	default:
		return UniqueDefinition{}, false
	}
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
	st.Endgame.UniqueContentVersion = UniqueContentVersion
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
	return ResolveVariableAffixes(d.Affixes, d.VariableAffixes, st.Endgame.UniqueRollBP)
}
func ValidateUniqueCatalog(defs map[ItemID]ItemDef) error {
	// V5 constrains only new unique generation; the ordinary equipment dictionary
	// deliberately remains larger, including ascended and conversion-only bases.
	for id, d := range uniqueByBase {
		def, ok := defs[id]
		if !ok || def.Equip == nil || def.Equip.NoTypeDrop {
			return fmt.Errorf("V5 unique %d missing eligible equipment", id)
		}
		if err := validateUniqueDefinition(d); err != nil {
			return err
		}
		e := def.Equip
		for _, local := range d.LocalEnhancements {
			if len(ResolveItemLocalEnhancements(e, []ItemLocalEnhancement{local}, UniqueRollMinBP)) == 0 {
				return fmt.Errorf("unique %d local enhancement has no intrinsic base", id)
			}
		}
		if d.Slot != e.Slot || d.Professions != e.Need.Professions || d.BeginnerAllowed != e.Need.BeginnerAllowed || d.Sex != e.Need.Sex || d.LevelRequirement != e.LevelReq {
			return fmt.Errorf("unique %d requirement metadata differs from loaded equipment", id)
		}
	}
	return nil
}

func (d UniqueDefinition) Description() string { return d.Lore }

// Higher potency must improve both positive and explicitly cursed packages.
// Attack interval is inverted: negative numbers make attacks faster.
func scaleUniqueAffix(a Affix, roll int32) int32 {
	drawback := a.Value < 0
	if a.Attr == AttrAtkSpeed {
		drawback = a.Value > 0
	}
	if drawback {
		roll = 20000 - roll
	}
	return scaledUniquePower(a.Value, roll)
}
