package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"sync"
)

func balanceItemsFixture() map[domain.ItemID]domain.ItemDef {
	var rows []domain.ItemDef
	if err := testfixture.LoadEndgameItems(&rows); err != nil {
		panic(err)
	}
	items := map[domain.ItemID]domain.ItemDef{}
	for _, d := range rows {
		items[d.ID] = d
	}
	return items
}

var lootFixtureOnce sync.Once
var lootFixtureTable *domain.LootRuleTable

func lootRulesTableFixture() *domain.LootRuleTable {
	lootFixtureOnce.Do(func() { lootFixtureTable = loadLootRulesFixture() })
	return lootFixtureTable
}
func loadLootRulesFixture() *domain.LootRuleTable {
	var row domain.LootRuleSet
	if err := testfixture.LoadLootRules(&row); err != nil {
		panic(err)
	}
	t, err := domain.NewLootRuleTable(1, []domain.LootRuleSet{row}, balanceItemsFixture())
	if err != nil {
		panic(err)
	}
	return t
}
func lootRulesFixture() *domain.LootRuleSet { return lootRulesTableFixture().Active() }

var riftFixtureOnce sync.Once
var riftFixtureTable *domain.RiftRuleTable

func riftRulesTableFixture() *domain.RiftRuleTable {
	riftFixtureOnce.Do(func() { riftFixtureTable = loadRiftRulesFixture() })
	return riftFixtureTable
}
func loadRiftRulesFixture() *domain.RiftRuleTable {
	var row domain.RiftRules
	if err := testfixture.LoadRiftRules(&row); err != nil {
		panic(err)
	}
	t, err := domain.NewRiftRuleTable(1, []domain.RiftRules{row}, lootRulesTableFixture(), balanceItemsFixture())
	if err != nil {
		panic(err)
	}
	return t
}
func riftRulesFixture() *domain.RiftRules { return riftRulesTableFixture().Active() }
