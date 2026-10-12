package domain

// EndgameCombatStatsV5 contains authoritative raw equipment channels, including
// current friendly aura grants. Per-enemy penetration, PvP scaling and shared
// conditional caps are applied only at the damage boundary.
type EndgameCombatStatsV5 struct {
	MagicFind            int64                        `json:"magicFind"`
	Elements             []EndgameElementStatsV5      `json:"elements"`
	PhysicalMitigationBP int32                        `json:"physicalMitigationBP"`
	EquipmentAuras       []EquipmentAuraView          `json:"equipmentAuras"`
	ProcReadiness        []EquipmentProcReadinessView `json:"procReadiness"`
}
type EndgameElementStatsV5 struct {
	Element      string `json:"element"`
	DamageBP     int32  `json:"damageBP"`
	ResistBP     int32  `json:"resistBP"`
	PierceBP     int32  `json:"pierceBP"`
	MitigationBP int32  `json:"mitigationBP"`
	AbsorbBP     int32  `json:"absorbBP"`
}
type EquipmentAuraView struct {
	AuraID         string   `json:"auraId"`
	Family         string   `json:"family"`
	SourceEntityID EntityID `json:"sourceEntityId"`
	SourceUID      int64    `json:"sourceUID,string"`
	Rank           int32    `json:"rank"`
	AssetKey       string   `json:"assetKey"`
	Modifier       string   `json:"modifier"`
	Element        string   `json:"element,omitempty"`
	Value          int32    `json:"value"`
	LeaseMS        int32    `json:"leaseMs"`
}
type EquipmentProcReadinessView struct {
	EffectID          string  `json:"effectId"`
	SourceUID         int64   `json:"sourceUID,string"`
	Trigger           string  `json:"trigger"`
	SkillID           SkillID `json:"skillId"`
	FixedRank         int32   `json:"fixedRank"`
	ChanceBP          int32   `json:"chanceBP"`
	ICDMS             int32   `json:"icdMs"`
	RemainingMS       int64   `json:"remainingMs"`
	GlobalRemainingMS int64   `json:"globalRemainingMs"`
	Ready             bool    `json:"ready"`
}
