package domain

// CombatSourceSnapshotV5 is an immutable, accepted-action source snapshot.
// Target conditions and per-hit random rolls are deliberately not captured.
type CombatSourceSnapshotV5 struct {
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
