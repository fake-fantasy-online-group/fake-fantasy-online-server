package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestLifeTemperNativeAndSidecarAgree(t *testing.T) {
	s, def, st := legendaryTooltipFixture(t, false)
	st.RefineLevel = 15
	next, err := domain.LifeTemperAffix(st, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	view := s.endgameItem(next, "bag", 0)
	effective, bounds := next.Legendary.EffectiveAffix(0)
	if view.Stats[0].Value != effective.Affix.Value || view.Legendary.Affixes[0].Value != effective.Affix.Value || view.Legendary.Affixes[0].Min != bounds.Min || view.Legendary.Affixes[0].Max != bounds.Max {
		t.Fatal("sidecar does not show effective values", view)
	}
	tip := s.ownedItemTooltip(def, next, s.players[1])
	if !strings.Contains(tip, affixText(effective.Affix)) {
		t.Fatal("native tooltip missing boosted value", tip)
	}
	if next.Legendary.Affixes != st.Legendary.Affixes {
		t.Fatal("display mutated raw values")
	}
}
