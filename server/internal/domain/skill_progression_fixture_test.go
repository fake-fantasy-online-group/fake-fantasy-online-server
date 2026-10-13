package domain

import (
	"encoding/json"
	"os"
	"sync"
)

// Explicit regression fixture only. No production path calls this function;
// loaders must read SQL and fail if the configured rules are absent.
//

var skillFixtureOnce sync.Once
var skillFixtureCatalog *SkillProgressionCatalog

func testSkillProgressionCatalog() *SkillProgressionCatalog {
	skillFixtureOnce.Do(func() {
		raw, err := os.ReadFile("../testfixture/catalog/skill_progression.json")
		if err != nil {
			panic(err)
		}
		var in SkillProgressionData
		if err := json.Unmarshal(raw, &in); err != nil {
			panic(err)
		}
		skillFixtureCatalog, err = NewSkillProgressionCatalog(in)
		if err != nil {
			panic(err)
		}
	})
	return skillFixtureCatalog
}
