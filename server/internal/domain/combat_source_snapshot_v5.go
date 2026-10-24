package domain

// ChildPreparation pins pure, equipment-dependent derived-skill inputs once at
// admission, including granted buffs and borrowed numerical bases. Ordinary
// sources without a derived preparation consumer do not allocate it.
type LegendaryChildPreparationSnapshot struct {
	MaximumMP      int32
	Hard, Raw      Learned
	Equipped, Bare Stats
	SpecialWeapon  ItemID
	Variants       []EquipmentSkillVariant
}

// CombatSourceSnapshotV5 is an immutable, accepted-action source snapshot.
// Target conditions and per-hit random rolls are deliberately not captured.
type CombatSourceSnapshotV5 struct {
	ChildPreparation        *LegendaryChildPreparationSnapshot `json:",omitempty"`
	Position                Pos
	LegendaryPowers         []ActiveLegendaryPower
	SuitBonuses             SuitCombatBonuses
	Stats                   Stats
	Equipment               EquipmentCombatStats
	Traits                  []UniqueTrait
	HPPercent, MPPercent    int32
	Invisible               bool
	StatusPresent           bool
	PhysicalFlat, MagicFlat int32
	PetExtras               []PetOffenseSnapshotV5
}

type PetOffenseSnapshotV5 struct {
	PetID            PetInstID
	SkillID          SkillID
	Magic            bool
	ChanceBP, Amount int32
}
