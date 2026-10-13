package domain

import "encoding/json"

// EndgameSkillBehaviorV1 is opt-in because older clients reject unknown keys.
const EndgameSkillBehaviorV1 = "endgame.skill-behavior.v1"

// SkillBehaviorContract separates always-present learning identity from the
// resolved current and next payloads. Neither payload is live cast permission.
type SkillBehaviorContract struct {
	Name                 string                `json:"name"`
	Icon                 int32                 `json:"icon"`
	Profession           int32                 `json:"profession"`
	Kind                 string                `json:"kind"`
	UnlockLevel          int32                 `json:"unlockLevel"`
	PrerequisiteSkillIDs []SkillID             `json:"prerequisiteSkillIds"`
	Current              *SkillBehaviorPayload `json:"current"`
	Next                 *SkillBehaviorPayload `json:"next"`
}

type SkillBehaviorPayload struct {
	Variant            *EquipmentSkillVariant `json:"variant,omitempty"`
	MPCost             int32                  `json:"mpCost"`
	PrepareMS          int32                  `json:"prepareMs"`
	CooldownMS         int32                  `json:"cooldownMs"`
	CastDistance       int32                  `json:"castDistance"`
	Usable             bool                   `json:"usable"`
	RequiresInvisible  bool                   `json:"requiresInvisible"`
	RequiredWeaponType int32                  `json:"requiredWeaponType"`
	InputMode          string                 `json:"inputMode"`
	PointRequired      bool                   `json:"pointRequired"`
	SelfFallback       bool                   `json:"selfFallback"`
	TargetSelf         bool                   `json:"targetSelf"`
	TargetEnemy        bool                   `json:"targetEnemy"`
	TargetTeam         bool                   `json:"targetTeam"`
	AllowsDeadTarget   bool                   `json:"allowsDeadTarget"`
	DeadTargetOnly     bool                   `json:"deadTargetOnly"`
	GroundEffect       bool                   `json:"groundEffect"`
	Area               SkillBehaviorArea      `json:"area"`
}

type SkillBehaviorArea struct {
	Shape  string `json:"shape"`
	Center string `json:"center"`
	Radius int32  `json:"radius"`
	Length int32  `json:"length"`
	Width  int32  `json:"width"`
	Angle  int32  `json:"angle"`
}

// MarshalJSON distinguishes a negotiated per-skill hold from an old connection.
// The latter omits contract entirely, preserving strict older client schemas.
func (v SkillRankView) MarshalJSON() ([]byte, error) {
	type rankWire SkillRankView
	if v.ContractUnavailable {
		return json.Marshal(struct {
			*rankWire
			Contract *SkillBehaviorContract `json:"contract"`
		}{rankWire: (*rankWire)(&v), Contract: nil})
	}
	return json.Marshal(rankWire(v))
}
