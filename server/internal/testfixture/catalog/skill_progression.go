// Package catalog constructs explicit regression inputs. Only tests import it;
// production loaders never link fixtures or fall back to these constructors.
package catalog

import (
	_ "embed"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"sync"
)

//go:embed skill_progression.json
var skillJSON []byte
var skillOnce sync.Once
var skills *domain.SkillProgressionCatalog

func SkillProgressionCatalog() *domain.SkillProgressionCatalog {
	skillOnce.Do(func() {
		var in domain.SkillProgressionData
		if err := json.Unmarshal(skillJSON, &in); err != nil {
			panic(err)
		}
		var err error
		skills, err = domain.NewSkillProgressionCatalog(in)
		if err != nil {
			panic(err)
		}
	})
	return skills
}
