package domain

import (
	"encoding/json"
	"os"
	"sync"
)

// Explicit fixture input for regression tests and migration parity checks only.
// The production loader never calls these functions and never falls back here.
//

var fixtureCatalogOnce sync.Once
var fixtureCatalog *EndgameCatalog

func testEndgameCatalogData() EndgameCatalogData {
	endgameCatalogFixtureJSON, err := os.ReadFile("../testfixture/catalog/endgame_catalog.json")
	if err != nil {
		panic(err)
	}
	var d EndgameCatalogData
	if err := json.Unmarshal(endgameCatalogFixtureJSON, &d); err != nil {
		panic(err)
	}
	d.Skills = testSkillProgressionCatalog()
	return d
}
func testEndgameCatalog() *EndgameCatalog {
	fixtureCatalogOnce.Do(func() {
		var err error
		fixtureCatalog, err = NewEndgameCatalog(testEndgameCatalogData())
		if err != nil {
			panic(err)
		}
	})
	return fixtureCatalog
}
