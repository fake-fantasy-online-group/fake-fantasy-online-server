package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

func cubeRulesFixture(items map[domain.ItemID]domain.ItemDef) *domain.CubeRuleTable {
	var rows []domain.CubeRecipeDef
	if err := testfixture.LoadCubeRules(&rows); err != nil {
		panic(err)
	}
	rules, err := domain.NewCubeRuleTable(rows, items)
	if err != nil {
		panic(err)
	}
	return rules
}
