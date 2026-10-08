package domain

// CraftMaterial 是一条合成材料需求。
type CraftMaterial struct {
	Item ItemID
	Qty  int32
}

// CraftRecipe 是 gamedata.ov_combine 的可执行服务端投影。
type CraftRecipe struct {
	RowNo                          int32
	ShowType                       uint8
	Product                        ItemID
	OutputQty                      int32
	Cost                           int64
	SuccessRate                    int32
	DisappearRate                  int32
	RequiredSkill                  SkillID
	ProficiencyMin, ProficiencyMax int32
	ProficiencyGain                int32
	SkillLevelNeed                 int32
	Materials                      []CraftMaterial
}

// CraftTable 同时按展示类型与产物索引；同产物允许多条材料路线。
type CraftTable struct {
	ByType    map[uint8][]CraftRecipe
	ByProduct map[uint8]map[ItemID][]CraftRecipe
}

func (t CraftTable) Recipes(showType uint8) []CraftRecipe {
	return t.ByType[showType]
}

func (t CraftTable) ProductRecipes(showType uint8, product ItemID) []CraftRecipe {
	return t.ByProduct[showType][product]
}
