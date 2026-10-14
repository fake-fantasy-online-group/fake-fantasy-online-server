package domain

import (
	"fmt"
)

const UniqueCatalogVersion = "endgame-v6-scope259"
const UniqueContentVersion int32 = 6
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

func validateUniqueDefinition(d UniqueDefinition, catalog *EndgameCatalog) error {
	if d.BaseID <= 0 || d.ID != int32(d.BaseID) || d.Name == "" || len(d.Traits) > 3 || !validEndgamePowerTier(d.PowerTier) {
		return fmt.Errorf("invalid unique catalogue row %d", d.BaseID)
	}
	if err := validateEndgameExtraAffixes(d.Affixes, true); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	if err := validateEndgameRollModel(d.Affixes, d.VariableAffixes, d.LocalEnhancements); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	if err := ValidateEquipmentEffects(d.TypedEffects, catalog); err != nil {
		return fmt.Errorf("unique %d: %w", d.BaseID, err)
	}
	if err := ValidateSkillRankBonuses(d.SkillBonuses, catalog.skills); err != nil {
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
func UniqueCatalog(catalog *EndgameCatalog) []UniqueDefinition {
	return catalog.Uniques(UniqueContentVersion)
}

// UniqueAppearanceBaseIDs keeps legacy-owned appearance aliases available.
func UniqueAppearanceBaseIDs(catalog *EndgameCatalog) []ItemID {
	var out []ItemID
	for _, d := range catalog.Uniques(0) {
		out = append(out, d.BaseID)
	}
	return out
}
func UniqueForBase(id ItemID, catalog *EndgameCatalog) (UniqueDefinition, bool) {
	return catalog.Unique(UniqueContentVersion, id)
}
func UniqueOf(st Stack, catalog *EndgameCatalog) (UniqueDefinition, bool) {
	if st.UID == 0 || st.Count != 1 || st.Endgame.UniqueID != int32(st.Item) || st.Endgame.UniqueRollBP < UniqueRollMinBP || st.Endgame.UniqueRollBP > UniqueRollMaxBP {
		return UniqueDefinition{}, false
	}
	switch st.Endgame.UniqueContentVersion {
	case 0:
		d, ok := catalog.Unique(0, st.Item)
		return cloneUnique(d), ok
	case 5:
		d, ok := catalog.Unique(5, st.Item)
		return uniqueV5PersonalDefense(cloneUnique(d)), ok
	case UniqueContentVersion:
		return UniqueForBase(st.Item, catalog)
	default:
		return UniqueDefinition{}, false
	}
}
func UniqueName(st Stack, catalog *EndgameCatalog) (string, bool) {
	d, ok := UniqueOf(st, catalog)
	return d.Name, ok
}
func ApplyUnique(st *Stack, def ItemDef, intn func(int) int) bool {
	if st == nil || st.UID == 0 || st.Count != 1 || st.Item != def.ID || def.Equip == nil || st.Endgame.UniqueID != 0 || intn == nil {
		return false
	}
	if _, ok := UniqueForBase(st.Item, def.EndgameCatalog); !ok {
		return false
	}
	// Promotion must never destroy owned socket cards.
	if !CanPromoteSocketlessUnique(*st) {
		return false
	}
	roll := intn(int(UniqueRollMaxBP - UniqueRollMinBP + 1))
	if roll < 0 || roll > int(UniqueRollMaxBP-UniqueRollMinBP) {
		return false
	}
	st.SocketCount = 0
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
	d, ok := UniqueForBase(st.Item, def.EndgameCatalog)
	if !ok || !d.NativeDropEligible {
		return false
	}
	if roll := intn(100); roll != 0 {
		return false
	}
	return ApplyUnique(st, def, intn)
}
func UniqueAffixes(st Stack, catalog *EndgameCatalog) []Affix {
	d, ok := UniqueOf(st, catalog)
	if !ok {
		return nil
	}
	return ResolveVariableAffixes(d.Affixes, d.VariableAffixes, st.Endgame.UniqueRollBP)
}
func ValidateUniqueCatalog(defs map[ItemID]ItemDef, catalog *EndgameCatalog) error {
	c := catalog
	if c == nil {
		c = EndgameCatalogOf(defs)
	}
	if c == nil {
		return fmt.Errorf("endgame catalog is not loaded")
	}
	// V5 constrains only new unique generation; the ordinary equipment dictionary
	// deliberately remains larger, including ascended and conversion-only bases.
	for _, d := range c.Uniques(UniqueContentVersion) {
		id := d.BaseID
		def, ok := defs[id]
		if !ok || def.Equip == nil || def.Equip.NoTypeDrop {
			return fmt.Errorf("V5 unique %d missing eligible equipment", id)
		}
		if err := validateUniqueDefinition(d, c); err != nil {
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

// V5 retired its three team auras. Preserve every stored instance and all other
// derived properties, converting only those effects into equal personal defense.
func uniqueV5PersonalDefense(d UniqueDefinition) UniqueDefinition {
	out := d.TypedEffects[:0]
	for _, e := range d.TypedEffects {
		if e.Kind != "aura" || e.Aura == nil {
			out = append(out, e)
			continue
		}
		a := e.Aura
		if a.Modifier == "physical_mitigation_bp" {
			d.Traits = append(d.Traits, UniqueTrait{Kind: "physical_guard", PowerBP: a.Value})
		} else {
			out = append(out, EquipmentEffect{Kind: "element_resist_bp", Element: a.Element, Value: a.Value})
		}
	}
	d.TypedEffects = out
	if d.BaseID == 1107 || d.BaseID == 1111 || d.BaseID == 2603 {
		d.Benefit = "旧版团队光环已等值转换为个人防护；其余固定属性与随机值保持原版。"
		d.Identity = "旧版护持之力内敛于自身，仅为佩戴者提供对应防护。"
		d.Tradeoff = "不再向队友或宠物提供团队光环；旧版其它属性、代价及本件白字范围保持原样。"
	}
	return d
}

// CanPromoteSocketlessUnique rejects all stored socket content, including stale
// entries outside SocketCount. Empty holes may be removed by explicit promotion.
func CanPromoteSocketlessUnique(st Stack) bool {
	return st.Sockets == [5]ItemID{} && st.SocketUIDs == [5]int64{} && st.SocketCards == [5]CardInstanceState{}
}
