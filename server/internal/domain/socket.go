package domain

type SocketRecipe struct {
	RowNo         int32
	Hole          uint8
	EquipClass    int32
	EquipTier     int32
	DrillCost     int64
	InlayCost     int64
	RequiredSkill SkillID
	DestroyRate   int32
	SuccessRate   int32
	ProtectItem   ItemID
	Materials     []CraftMaterial
}

type SocketTable map[[3]int32]SocketRecipe

func (t SocketTable) Get(hole uint8, class, tier int32) (SocketRecipe, bool) {
	r, ok := t[[3]int32{int32(hole), class, tier}]
	return r, ok
}
