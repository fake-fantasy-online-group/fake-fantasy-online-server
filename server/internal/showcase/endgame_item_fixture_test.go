package showcase
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"


import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

func endgameItemsFixture() []domain.ItemDef {
	var items []domain.ItemDef
	if err := testfixture.LoadEndgameItems(&items); err != nil {
		panic(err)
	}
	for i := range items {
		items[i].EndgameCatalog = catalogfixture.EndgameCatalog()
	}
	return items
}
func astralRuneItemsFixture(_ int32) []domain.ItemDef {
	var out []domain.ItemDef
	for _, d := range endgameItemsFixture() {
		if d.ID >= domain.FirstAstralRune && d.ID <= domain.LastAstralRune {
			out = append(out, d)
		}
	}
	return out
}
func cubeMaterialDefinitionsFixture() []domain.ItemDef {
	var out []domain.ItemDef
	for _, d := range endgameItemsFixture() {
		if d.ID >= domain.CubeStarMarrow && d.ID <= domain.CubeImmortalEcho {
			out = append(out, d)
		}
	}
	return out
}
