package domain

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

func endgameItemsFixture() []ItemDef {
	var items []ItemDef
	if err := testfixture.LoadEndgameItems(&items); err != nil {
		panic(err)
	}
	for i := range items {
		items[i].EndgameCatalog = testEndgameCatalog()
	}
	return items
}
func AstralRuneItems(_ int32) []ItemDef {
	var out []ItemDef
	for _, d := range endgameItemsFixture() {
		if d.ID >= FirstAstralRune && d.ID <= LastAstralRune {
			out = append(out, d)
		}
	}
	return out
}
func CubeMaterialDefinitions() []ItemDef {
	var out []ItemDef
	for _, d := range endgameItemsFixture() {
		if d.ID >= CubeStarMarrow && d.ID <= CubeImmortalEcho {
			out = append(out, d)
		}
	}
	return out
}
