package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestSupplementalRuneLootUsesInjectedThresholdAndRate(t *testing.T) {
	s, mid := lootScene(t, nil)
	for id, d := range balanceItemsFixture() {
		s.items[id] = d
	}
	base := *lootRulesFixture()
	s.lootRules = lootRulesTableFixture()
	s.rollAstralRuneLoot(s.entities[mid], 0, 0)
	if len(s.ground) != 0 {
		t.Fatal("legacy level20 threshold ignored")
	}
	base.MinRuneLevel = 14
	base.Grades[0].RuneRateBP = 10000
	var err error
	s.lootRules, err = domain.NewLootRuleTable(1, []domain.LootRuleSet{base}, s.items)
	if err != nil {
		t.Fatal(err)
	}
	s.rollAstralRuneLoot(s.entities[mid], 0, 0)
	if len(s.ground) != 1 {
		t.Fatal("configured live supplemental rune did not drop exactly one")
	}
	base.Grades[0].RuneRateBP = 0
	s.lootRules, err = domain.NewLootRuleTable(1, []domain.LootRuleSet{base}, s.items)
	if err != nil {
		t.Fatal(err)
	}
	s.rollAstralRuneLoot(s.entities[mid], 0, 1)
	if len(s.ground) != 1 {
		t.Fatal("disabled live rune rule ignored")
	}
	s.lootRules = nil
	s.rollAstralRuneLoot(s.entities[mid], 0, 1)
	if len(s.ground) != 1 {
		t.Fatal("missing configuration silently substituted defaults")
	}
}
