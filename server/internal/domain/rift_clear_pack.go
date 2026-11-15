package domain

import "sort"

// RiftClearPackItemID is the extra gift granted once on a successful 洞天 clear.
// It lives in the reserved endgame consumable range after the power-codex items.
const RiftClearPackItemID ItemID = 1900028304

// RiftClearPackTrueSetChanceBP is the independent roll for a class 真* set.
// 1/10000 is the lowest chance the gacha schema can represent; it is an
// implementer choice, not a player-specified rate, and stays deterministic in tests.
const RiftClearPackTrueSetChanceBP int32 = 1

// The static gacha MinFreeSlots field is left at 0. A fixed empty-slot floor
// cannot see existing stacks; open uses RiftClearPackFitsWorstCase instead.

// RiftClearPackName is the catalog display name of the clear gift.
const RiftClearPackName = "洞天通关礼包"

// RiftClearPackPotionRewards is the guaranteed/high-rate potion table.
// 天山雪莲 is the instant percent restore; 长白参 is a separate percent HoT item.
// Red and blue potions are granted in larger quantities. Chance 10000 = always.
func RiftClearPackPotionRewards() []GachaBoxReward {
	return []GachaBoxReward{
		{Item: 3121, MinQty: 3, MaxQty: 5, ChanceBP: 10000},   // 天山雪莲 50% HP/MP
		{Item: 3151, MinQty: 3, MaxQty: 5, ChanceBP: 10000},   // 长白参
		{Item: 3012, MinQty: 20, MaxQty: 30, ChanceBP: 10000}, // (小)生命药水
		{Item: 3013, MinQty: 20, MaxQty: 30, ChanceBP: 10000}, // (小)法力药水
		{Item: 3014, MinQty: 20, MaxQty: 30, ChanceBP: 10000}, // (中)生命药水
		{Item: 3015, MinQty: 20, MaxQty: 30, ChanceBP: 10000}, // (中)法力药水
		{Item: 3016, MinQty: 15, MaxQty: 25, ChanceBP: 10000}, // (大)生命药水
		{Item: 3017, MinQty: 15, MaxQty: 25, ChanceBP: 10000}, // (大)法力药水
		{Item: 3018, MinQty: 15, MaxQty: 25, ChanceBP: 10000}, // (强)生命药水
		{Item: 3019, MinQty: 15, MaxQty: 25, ChanceBP: 10000}, // (强)法力药水
		{Item: 3122, MinQty: 10, MaxQty: 20, ChanceBP: 10000}, // 特级生命药水
		{Item: 3123, MinQty: 10, MaxQty: 20, ChanceBP: 10000}, // 特级法力药水
		{Item: 42432, MinQty: 8, MaxQty: 15, ChanceBP: 10000}, // 超强生命药水
		{Item: 42433, MinQty: 8, MaxQty: 15, ChanceBP: 10000}, // 生命之源
	}
}

// NewRiftClearPackItem returns the runtime definition used when the catalog row
// has not been loaded yet (tests, or a database that has not applied the migration).
func NewRiftClearPackItem() ItemDef {
	return ItemDef{
		ID:                RiftClearPackItemID,
		Name:              RiftClearPackName,
		Description:       "洞天成功通关后的慰劳礼包。开启可获得天山雪莲、长白参和较多红蓝药水，并有极低概率开出本职业真*套装。永久绑定，不可交易。",
		Stackable:         true,
		ConsumeOnUse:      true,
		CanMail:           false,
		CanTrade:          false,
		FixedBinding:      ItemBindingFixedBound,
		InventoryTab:      0,
		InventoryTabKnown: true,
		GachaBox: &GachaBoxDef{
			Rewards: RiftClearPackPotionRewards(),
		},
	}
}

// RegisterRiftClearPack installs the pack definition when absent, or fills a
// missing gacha table on an already-loaded row. It never invents 真* item IDs.
func RegisterRiftClearPack(items map[ItemID]ItemDef) {
	if items == nil {
		return
	}
	def, ok := items[RiftClearPackItemID]
	if !ok {
		items[RiftClearPackItemID] = NewRiftClearPackItem()
		return
	}
	if def.GachaBox == nil {
		def.GachaBox = &GachaBoxDef{Rewards: RiftClearPackPotionRewards()}
	}
	def.GachaBox.MinFreeSlots = 0
	items[RiftClearPackItemID] = def
}

// TrueSetPiecesForCharacter returns the original hard-dungeon 真* set pieces the
// character can wear (profession and sex). Missing catalog entries are skipped.
// The result is sorted for deterministic tests and grant order.
func TrueSetPiecesForCharacter(items map[ItemID]ItemDef, ch *Character) []ItemID {
	if items == nil || ch == nil || !ch.HasProfession() || !ch.Race.Valid() {
		return nil
	}
	sex := uint8(2)
	if ch.Appear.Gender == 1 {
		sex = 1
	}
	var out []ItemID
	seen := map[ItemID]bool{}
	consider := func(hardID ItemID) {
		if seen[hardID] {
			return
		}
		def, ok := items[hardID]
		if !ok || def.Equip == nil || !def.HardDungeonOnly {
			return
		}
		if !def.Equip.Need.AllowsCharacter(ch) {
			return
		}
		if def.Equip.Need.Sex != 0 && def.Equip.Need.Sex != sex {
			return
		}
		seen[hardID] = true
		out = append(out, hardID)
	}
	for _, set := range dungeonEquipmentManifest.Sets {
		for _, id := range set.Members {
			consider(HardDungeonItemOffset + id)
		}
	}
	for _, w := range dungeonEquipmentManifest.Weapons {
		if w.HardID != 0 {
			consider(w.HardID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// RiftClearPackTrueSetHit is the independent 1/10000 roll. Tests inject intn.
func RiftClearPackTrueSetHit(intn func(int) int) bool {
	if intn == nil {
		return false
	}
	return intn(10000) < int(RiftClearPackTrueSetChanceBP)
}

// AddRiftClearPackReward places a preview or real open reward using the same
// stacking rules as Bag.Add, and binds 真* equipment so it cannot be traded.
func AddRiftClearPackReward(bag *Bag, def ItemDef, qty int32) int32 {
	if bag == nil || qty <= 0 || def.ID == 0 {
		return qty
	}
	if def.Equip != nil {
		left := qty
		for left > 0 {
			st := NewStack(def, 1)
			st.Bound = true
			if bag.AddStack(def, st) != 0 {
				return left
			}
			left--
		}
		return 0
	}
	return bag.Add(def, qty)
}

// RiftClearPackFitsWorstCase reports whether the bag can hold every potion at
// MaxQty plus every wearable 真* piece for this character, after consuming the
// pack at packSlot. It reuses Bag stacking, including room already in existing
// stacks, so a rare hit cannot be rolled and then refused for lack of space.
func RiftClearPackFitsWorstCase(bag *Bag, items map[ItemID]ItemDef, ch *Character, packSlot int) bool {
	if bag == nil || items == nil {
		return false
	}
	preview := bag.Clone()
	if preview == nil || !preview.RemoveAt(packSlot, 1) {
		return false
	}
	for _, reward := range RiftClearPackPotionRewards() {
		def, ok := items[reward.Item]
		if !ok || AddRiftClearPackReward(preview, def, reward.MaxQty) != 0 {
			return false
		}
	}
	for _, id := range TrueSetPiecesForCharacter(items, ch) {
		def, ok := items[id]
		if !ok || AddRiftClearPackReward(preview, def, 1) != 0 {
			return false
		}
	}
	return true
}
