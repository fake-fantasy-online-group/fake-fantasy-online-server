package domain

// SpiritDefinition maps original nine NPCs and real game item templates. Ring
// spirits are deliberately not part of this registry. Currency is copper.
type SpiritDefinition struct {
	Kind                           uint8
	Name                           string
	ItemID, EssenceID, SeparatorID ItemID
	NPCID, MapID                   int32
	NPCName, NPCScript             string
	Slot                           EquipSlot
	UIKey                          string
	ProcessMoney                   int64
}

var spiritDefinitions = [...]SpiritDefinition{
	{1, "鞋灵", 24640, 36041, 14924, 163, 7, "鞋灵仙子", "Nxieling", SlotShoe, "shoering", 100000},
	{2, "拳灵", 47028, 27354, 42012, 241, 7, "拳灵仙子", "Nquanling", SlotGlove, "glovesoul", 100000},
	{3, "羽灵", 47501, 36320, 42656, 257, 7, "羽灵仙子", "Nyuling1", SlotBag, "wingsoul", 100000},
	{4, "衣灵", 29874, 36353, 42984, 273, 7, "衣灵仙子", "Nyiling", SlotBody, "ClothSoul", 100000},
	{5, "项灵", 24608, 36038, 14912, 79, 7, "项灵仙子", "Nxiangling", SlotNeck, "neckring", 100000},
	{6, "面灵", 47490, 36318, 42645, 122, 7, "面灵仙子", "Nmianling", SlotFace, "facesoul", 100000},
	{7, "帽灵", 47004, 36223, 25095, 279, 7, "帽灵仙子", "Nmaoling", SlotHead, "capsoul", 100000},
	{8, "法宝灵", 47640, 36417, 43194, 283, 7, "法宝灵仙子", "Nfabaoling", SlotTreasure, "HonorSoul", 50000},
	{9, "武器灵", 8038, 4230, 13697, 287, 7, "武灵仙子", "Nwuling", SlotWeapon, "WeaponSoul", 100000},
}

func SpiritDefinitions() []SpiritDefinition {
	return append([]SpiritDefinition(nil), spiritDefinitions[:]...)
}
func SpiritDefinitionForKind(kind uint8) (SpiritDefinition, bool) {
	for _, d := range spiritDefinitions {
		if d.Kind == kind {
			return d, true
		}
	}
	return SpiritDefinition{}, false
}
func SpiritDefinitionForItem(id ItemID) (SpiritDefinition, bool) {
	for _, d := range spiritDefinitions {
		if d.ItemID == id {
			return d, true
		}
	}
	return SpiritDefinition{}, false
}
func (d SpiritDefinition) FourOnly() bool { return d.Kind == 1 }
func (d SpiritDefinition) AcceptsSlot(slot EquipSlot) bool {
	return slot == d.Slot || (d.Kind == 9 && slot == SlotTwoHand)
}

// Weights are recovered from assets.fenfentu.org/db/lings.json, in original
// attribute order. Return a copy so callers cannot mutate the registry.
func (d SpiritDefinition) Weights() []SpiritWeight {
	attrs := [...]int32{AttrSTR, AttrAGI, AttrSPI, AttrVIT, AttrINT, AttrAtk, AttrMAtk, AttrPhysRes, AttrDEX, AttrMagicRes, AttrDef, AttrMDef}
	weights := [...]int32{700, 700, 800, 700, 700, 700, 800, 700, 1100, 1100, 1000, 1000}
	if d.Kind == 8 || d.Kind == 9 {
		weights = [...]int32{500, 700, 800, 400, 500, 400, 500, 960, 1360, 1360, 1260, 1260}
	}
	out := make([]SpiritWeight, len(attrs))
	for i, a := range attrs {
		out[i] = SpiritWeight{Attr: a, Weight: weights[i]}
	}
	return out
}
