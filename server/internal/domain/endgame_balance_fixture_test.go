package domain

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"sync"
)

func balanceItemsFixture() map[ItemID]ItemDef {
	var rows []ItemDef
	if err := testfixture.LoadEndgameItems(&rows); err != nil {
		panic(err)
	}
	items := map[ItemID]ItemDef{}
	for _, d := range rows {
		items[d.ID] = d
	}
	return items
}

var lootFixtureOnce sync.Once
var lootFixtureTable *LootRuleTable

func lootRulesTableFixture() *LootRuleTable {
	lootFixtureOnce.Do(func() { lootFixtureTable = loadLootRulesFixture() })
	return lootFixtureTable
}
func loadLootRulesFixture() *LootRuleTable {
	var row LootRuleSet
	if err := testfixture.LoadLootRules(&row); err != nil {
		panic(err)
	}
	t, err := NewLootRuleTable(1, []LootRuleSet{row}, balanceItemsFixture())
	if err != nil {
		panic(err)
	}
	return t
}
func lootRulesFixture() *LootRuleSet { return lootRulesTableFixture().Active() }

var riftFixtureOnce sync.Once
var riftFixtureTable *RiftRuleTable

func riftRulesTableFixture() *RiftRuleTable {
	riftFixtureOnce.Do(func() { riftFixtureTable = loadRiftRulesFixture() })
	return riftFixtureTable
}
func loadRiftRulesFixture() *RiftRuleTable {
	var row RiftRules
	if err := testfixture.LoadRiftRules(&row); err != nil {
		panic(err)
	}
	t, err := NewRiftRuleTable(1, []RiftRules{row}, lootRulesTableFixture(), balanceItemsFixture())
	if err != nil {
		panic(err)
	}
	return t
}
func riftRulesFixture() *RiftRules { return riftRulesTableFixture().Active() }
