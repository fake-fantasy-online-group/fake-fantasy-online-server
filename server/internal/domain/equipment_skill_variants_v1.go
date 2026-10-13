package domain

import "fmt"

// EquipmentSkillVariantsV1 gates both execution and the optional wire metadata.
const EquipmentSkillVariantsV1 = "endgame.skill-variants.v1"

// EquipmentVariantBehavior identifies a reusable, finite engine operation.
// Item/variant IDs select configuration, never a damage formula or behavior.
type EquipmentVariantBehavior string

const (
	VariantAreaFan           EquipmentVariantBehavior = "area_fan"
	VariantAreaLine          EquipmentVariantBehavior = "area_line"
	VariantDetonateOwnedTrap EquipmentVariantBehavior = "detonate_owned_trap"
	VariantDelayedHeal       EquipmentVariantBehavior = "delayed_heal"
	VariantInstantCast       EquipmentVariantBehavior = "instant_cast"
)

type EquipmentVariantParams struct {
	Area                                              SkillAreaTargeting
	DamagePercent, SecondaryDamagePercent             int32
	SecondaryNoStatuses, SecondaryNoKnockback         bool
	MinCooldownMS                                     int32
	TrapSkillID                                       SkillID
	TrapSearchRadius, TrapMinRadius, TrapMaxTargets   int32
	ImmediateHealPercent, DelayedHealPercent, DelayMS int32
}

// EquipmentSkillVariantRule is database configuration, separate from the
// deliberately small client DTO. It contains no executable script/formula.
type EquipmentSkillVariantRule struct {
	ID, Name, Description  string
	SkillID                SkillID
	Profession, MaxTargets int32
	Behavior               EquipmentVariantBehavior
	Params                 EquipmentVariantParams
}

func (r EquipmentSkillVariantRule) Variant() EquipmentSkillVariant {
	return EquipmentSkillVariant{ID: r.ID, Name: r.Name, Description: r.Description,
		SkillID: r.SkillID, Profession: r.Profession, MaxTargets: r.MaxTargets, Behavior: r.Behavior, Params: r.Params}
}
func (r EquipmentSkillVariantRule) Validate() error {
	if r.ID == "" || r.Name == "" || r.Description == "" || r.SkillID <= 0 || r.Profession < 1 || r.Profession > 5 || r.MaxTargets < 1 || r.MaxTargets > 16 {
		return fmt.Errorf("invalid skill variant identity %q", r.ID)
	}
	p := r.Params
	valid := false
	switch r.Behavior {
	case VariantAreaFan, VariantAreaLine:
		shape := SkillAreaCone
		if r.Behavior == VariantAreaLine {
			shape = SkillAreaRectangle
		}
		valid = p.Area.Valid() && p.Area.Shape == shape && p.Area.Center == SkillAreaCenterCaster &&
			p.Area.Radius <= 10000 && p.Area.Length <= 10000 && p.Area.Width <= 10000 &&
			p.DamagePercent > 0 && p.DamagePercent <= 100 && p.SecondaryDamagePercent > 0 && p.SecondaryDamagePercent <= 100
	case VariantDetonateOwnedTrap:
		valid = p.TrapSkillID > 0 && p.TrapSearchRadius > 0 && p.TrapSearchRadius <= 10000 && p.TrapMinRadius > 0 && p.TrapMinRadius <= 10000 && p.TrapMaxTargets > 0 && p.TrapMaxTargets <= 16
	case VariantDelayedHeal:
		valid = p.ImmediateHealPercent > 0 && p.ImmediateHealPercent <= 100 && p.DelayedHealPercent > 0 && p.DelayedHealPercent <= 100 && p.DelayMS >= 100 && p.DelayMS <= 60000
	case VariantInstantCast:
		valid = p.MinCooldownMS > 0 && p.MinCooldownMS <= 60000
	}
	if !valid {
		return fmt.Errorf("invalid skill variant behavior %q/%s", r.ID, r.Behavior)
	}
	return nil
}

type EquipmentSkillVariant struct {
	EquipmentRevision uint64                   `json:"-"`
	ID                string                   `json:"id"`
	Name              string                   `json:"name"`
	SourceUID         int64                    `json:"sourceUID,string"`
	Item              ItemID                   `json:"item"`
	Description       string                   `json:"description"`
	MaxTargets        int32                    `json:"maxTargets"`
	SkillID           SkillID                  `json:"-"`
	Profession        int32                    `json:"-"`
	Behavior          EquipmentVariantBehavior `json:"-"`
	Params            EquipmentVariantParams   `json:"-"`
}
