package main

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Existing sprite IDs, reviewed from the installed-format res_q21_001 and
// res_05diff package pixels. These are sprite IDs, not original item IDs.
// Columns follow 木、金、土、日、月、火、水. Four distinct seal families keep
// the mansions recognizable without implying that reused artwork depicts each animal.
// See docs/daoist-endgame/ICONS.md. Artwork remains external to this repository.
var astralRuneIcons = [...]int32{
	26279, 26280, 4312, 4233, 39032, 26282, 26281, // 青龙: inscribed scrolls
	26522, 26520, 26521, 41117, 39033, 26514, 26517, // 玄武: elemental sigils
	26301, 26298, 26302, 39194, 27731, 26303, 26297, // 白虎: engraved medallions
	39636, 39634, 39635, 4094, 4024, 39633, 39637, // 朱雀: branching glyphs
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
