package domain

import (
	"encoding/json"
	"fmt"
)

// SpecialWeaponEffectBranch preserves the approved profession/skill membership.
// Item eligibility remains the explicit dungeon manifest rule, not an ID offset
// or displayed weapon-name match. Low-level stat-only weapons have no branch.
type SpecialWeaponEffectBranch struct {
	BaseID     ItemID
	Profession int32
	Skills     []SkillID
}

var specialWeaponEffectBranches = []SpecialWeaponEffectBranch{
	{BaseID: 1021, Profession: 2, Skills: []SkillID{10102}},
	{BaseID: 1022, Profession: 2, Skills: []SkillID{10103}},
	{BaseID: 1023, Profession: 2, Skills: []SkillID{10104}},
	{BaseID: 1024, Profession: 2, Skills: []SkillID{10111}},
	{BaseID: 1025, Profession: 2, Skills: []SkillID{10113}},
	{BaseID: 1026, Profession: 2, Skills: []SkillID{10107}},
	{BaseID: 1027, Profession: 2, Skills: []SkillID{10122}},
	{BaseID: 1028, Profession: 2, Skills: []SkillID{10116}},
	{BaseID: 1520, Profession: 2, Skills: []SkillID{10117}},
	{BaseID: 1521, Profession: 2, Skills: []SkillID{10111}},
	{BaseID: 1522, Profession: 2, Skills: []SkillID{10119}},
	{BaseID: 1523, Profession: 2, Skills: []SkillID{10119}},
	{BaseID: 1524, Profession: 2, Skills: []SkillID{10121}},
	{BaseID: 1525, Profession: 2, Skills: []SkillID{10122}},
	{BaseID: 1526, Profession: 2, Skills: []SkillID{10127}},
	{BaseID: 1527, Profession: 2, Skills: []SkillID{10123}},
	{BaseID: 1620, Profession: 1, Skills: []SkillID{10005}},
	{BaseID: 1621, Profession: 1, Skills: []SkillID{10017}},
	{BaseID: 1622, Profession: 1, Skills: []SkillID{10013}},
	{BaseID: 1623, Profession: 1, Skills: []SkillID{10006}},
	{BaseID: 1624, Profession: 1, Skills: []SkillID{10028}},
	{BaseID: 1625, Profession: 1, Skills: []SkillID{10021}},
	{BaseID: 1626, Profession: 1, Skills: []SkillID{10027, 10009}},
	{BaseID: 1627, Profession: 1, Skills: []SkillID{10022}},
	{BaseID: 1820, Profession: 3, Skills: []SkillID{10219}},
	{BaseID: 1821, Profession: 3, Skills: []SkillID{10203}},
	{BaseID: 1822, Profession: 3, Skills: []SkillID{10213}},
	{BaseID: 1823, Profession: 3, Skills: []SkillID{10208}},
	{BaseID: 1824, Profession: 3, Skills: []SkillID{10209, 10208}},
	{BaseID: 1825, Profession: 3, Skills: []SkillID{10224, 10208, 10209}},
	{BaseID: 1826, Profession: 3, Skills: []SkillID{10209}},
	{BaseID: 1827, Profession: 3, Skills: []SkillID{10209}},
	{BaseID: 1922, Profession: 3, Skills: []SkillID{10221}},
	{BaseID: 1923, Profession: 3, Skills: []SkillID{10205}},
	{BaseID: 1924, Profession: 3, Skills: []SkillID{10213}},
	{BaseID: 1925, Profession: 3, Skills: []SkillID{10223}},
	{BaseID: 1926, Profession: 3, Skills: []SkillID{10206}},
	{BaseID: 1927, Profession: 3, Skills: []SkillID{10224}},
	{BaseID: 1928, Profession: 3, Skills: []SkillID{10216}},
	{BaseID: 1929, Profession: 3, Skills: []SkillID{10225}},
	{BaseID: 1320, Profession: 4, Skills: []SkillID{10301}},
	{BaseID: 1320, Profession: 5, Skills: []SkillID{10409}},
	{BaseID: 1321, Profession: 4, Skills: []SkillID{10306}},
	{BaseID: 1321, Profession: 5, Skills: []SkillID{10410}},
	{BaseID: 1322, Profession: 4, Skills: []SkillID{10302}},
	{BaseID: 1322, Profession: 5, Skills: []SkillID{10421, 10401}},
	{BaseID: 1323, Profession: 4, Skills: []SkillID{10320}},
	{BaseID: 1323, Profession: 5, Skills: []SkillID{10428, 10410}},
	{BaseID: 1324, Profession: 4, Skills: []SkillID{10304}},
	{BaseID: 1324, Profession: 5, Skills: []SkillID{10433}},
	{BaseID: 1325, Profession: 4, Skills: []SkillID{10316}},
	{BaseID: 1325, Profession: 5, Skills: []SkillID{10405, 10421}},
	{BaseID: 1326, Profession: 4, Skills: []SkillID{10322}},
	{BaseID: 1326, Profession: 5, Skills: []SkillID{10412}},
	{BaseID: 1327, Profession: 4, Skills: []SkillID{10303}},
	{BaseID: 1327, Profession: 5, Skills: []SkillID{10432}},
}

func specialWeaponDefinitionID(base ItemID, profession int32) string {
	return fmt.Sprintf("special-weapon:%d:%d", base, profession)
}

func init() {
	for _, branch := range specialWeaponEffectBranches {
		description := ""
		for _, weapon := range dungeonEquipmentManifest.Weapons {
			if weapon.BaseID == branch.BaseID {
				description = dungeonWeaponEffectText(weapon.Effect)
				var byClass map[string]string
				if json.Unmarshal(weapon.Effect, &byClass) == nil {
					description = byClass[map[int32]string{4: "药师", 5: "术士"}[branch.Profession]]
				}
				break
			}
		}
		RegisterEquipmentEffectDefinition(EquipmentEffectDefinition{
			ID: specialWeaponDefinitionID(branch.BaseID, branch.Profession), SourceKind: EffectSourceSpecialWeapon,
			Trigger: "skill-and-combat", Description: description,
		})
	}
}

// SpecialWeaponEffectBranches returns a defensive copy for presentation/audits.
func SpecialWeaponEffectBranches() []SpecialWeaponEffectBranch {
	out := append([]SpecialWeaponEffectBranch(nil), specialWeaponEffectBranches...)
	for i := range out {
		out[i].Skills = append([]SkillID(nil), out[i].Skills...)
	}
	return out
}

func SpecialWeaponSkillMatches(base ItemID, skill SkillID, profession int32) bool {
	for _, branch := range specialWeaponEffectBranches {
		if branch.BaseID != base || branch.Profession != profession {
			continue
		}
		for _, id := range branch.Skills {
			if id == skill {
				return true
			}
		}
	}
	return false
}

// RuntimeSpecialWeaponEffect also accepts a captured base identity: delayed
// statuses and accepted casts must retain their source after a weapon swap.
// The hook is the causal identity, independent of item instance and profession.
func RuntimeSpecialWeaponEffect(base ItemID, profession int32, hook string, producer bool) (RuntimeEquipmentEffect, bool) {
	for _, branch := range specialWeaponEffectBranches {
		if branch.BaseID != base || (profession != 0 && branch.Profession != profession) {
			continue
		}
		id := specialWeaponDefinitionID(base, branch.Profession)
		definition, ok := EquipmentEffectDefinitionByID(id)
		if !ok {
			return RuntimeEquipmentEffect{}, false
		}
		return RuntimeEquipmentEffect{DefinitionID: id, ID: fmt.Sprintf("special-weapon:%d:%s", base, hook),
			SourceKind: EffectSourceSpecialWeapon, Trigger: hook, Description: definition.Description, Producer: producer,
			Parameters: EquipmentEffectParameters{SpecialWeaponBase: base, Profession: branch.Profession, Skills: append([]SkillID(nil), branch.Skills...)},
		}, true
	}
	return RuntimeEquipmentEffect{}, false
}

// SpecialWeaponSourceCandidate is a raw worn-item eligibility candidate, not an
// actor-active profession branch. Profession zero selects a representative row;
// skill dispatch must resolve its exact profession before executing. It does not learn a
// skill, expand eligibility, or treat ordinary dungeon originals as enhanced.
func SpecialWeaponSourceCandidate(stack Stack, definition ItemDef) (RuntimeEquipmentEffect, bool) {
	if !EquipmentFunctional(stack, definition) {
		return RuntimeEquipmentEffect{}, false
	}
	effect, ok := RuntimeSpecialWeaponEffect(EnhancedWeaponBaseID(stack.Item), 0, "equipped", false)
	if !ok {
		return RuntimeEquipmentEffect{}, false
	}
	effect.Item, effect.SourceUID = stack.Item, stack.UID
	return effect, true
}
