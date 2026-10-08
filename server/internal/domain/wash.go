package domain

type WashChoice struct {
	Affix  Affix
	Weight int32
}

type WashRecipe struct {
	RowNo       int32
	EquipClass  int32
	EquipTier   int32
	Quality     uint8
	Material    ItemID
	MaterialQty int32
	Cost        int64
	Choices     []WashChoice
}

type WashTable map[[3]int32]WashRecipe

func (t WashTable) Get(class, tier int32, quality uint8) (WashRecipe, bool) {
	r, ok := t[[3]int32{class, tier, int32(quality)}]
	return r, ok
}
