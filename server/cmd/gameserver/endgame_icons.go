package main

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Prefer the mansion's native monster collectible, then ov_desc.small_map. For
// the other fourteen mansions, use explicitly labelled related native collectibles
// under the revised selection policy; do not claim these are direct animal drops.
// These are sprite IDs, not item IDs. All 28 rune icons have distinct visible
// pixels in res_q21_001 and res_05diff. Direct and substitute provenance:
// docs/daoist-endgame/rune-monster-icons.json. Artwork stays outside this repo.
var astralRuneIcons = [...]int32{
	26018, 4332, 4134, 4141, 4121, 4102, 4122, // 龙鳞、龙牙、温暖的皮毛、兔尾巴、火灵珠、老虎皮、白虎皮
	4118, 4313, 4158, 4304, 4117, 4132, 4112, // 灵兽角、牛角、飞天猪翅膀、山鼠尾巴、羽毛、野猪牙、血牙
	4115, 4131, 4345, 4142, 39439, 4136, 4350, // 狼皮、小狗牙、鹅翅面具、步行鸟嘴、神奇的羽毛、猴皮、猴爪
	4137, 27653, 3583, 4314, 4344, 4116, 4338, // 猫爪、幸运金羊毛、洁净的鹿角之心、马面具、鹿角面具、蛇胆、泥块
}

func endgameItemIcon(id domain.ItemID) int32 {
	if n := int(id - domain.FirstAstralRune); n >= 0 && n < len(astralRuneIcons) {
		return astralRuneIcons[n]
	}
	switch id {
	case domain.RiftKeyItemID:
		return 3387 // golden key
	case domain.CubeStarMarrow:
		return 26017 // luminous star crystal
	case domain.CubePurifiedDust:
		return 26214 // pale particulate dust
	case domain.CubeImmortalEcho:
		return 26804 // yin-yang essence
	}
	return 0
}

// Unique rolls keep their original equipment ItemID. Publish those base sprites
// too: relying only on a client's installed item_icons table leaves renamed
// equipment vulnerable to stale/missing aliases (for example boots 22581 use
// sprite 30041, not sprite 22581). Explicit ov_item_icon_special mappings remain
// authoritative, including the unique-eligible extended equipment slots.
func registerEndgameEquipmentIcons(w *worldData) {
	mapped := make(map[domain.ItemID]bool, len(w.itemIcons))
	for _, pair := range w.itemIcons {
		mapped[pair.Item] = true
	}
	for _, unique := range domain.UniqueCatalog() { // stable BaseID order
		def := w.items[unique.BaseID]
		if mapped[unique.BaseID] || def.Equip == nil || def.Icon <= 0 {
			continue // Do not guess a generic weapon sprite for unknown artwork.
		}
		w.itemIcons = append(w.itemIcons, domain.ItemIconMapping{Item: unique.BaseID, Icon: def.Icon})
	}
}
