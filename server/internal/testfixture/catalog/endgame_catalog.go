package catalog

import (
	_ "embed"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"sync"
)

//go:embed endgame_catalog.json
var catalogJSON []byte
var catalogOnce sync.Once
var catalog *domain.EndgameCatalog

func EndgameCatalogData() domain.EndgameCatalogData {
	var d domain.EndgameCatalogData
	if err := json.Unmarshal(catalogJSON, &d); err != nil {
		panic(err)
	}
	d.Skills = SkillProgressionCatalog()
	return d
}
func EndgameCatalog() *domain.EndgameCatalog {
	catalogOnce.Do(func() {
		var err error
		catalog, err = domain.NewEndgameCatalog(EndgameCatalogData())
		if err != nil {
			panic(err)
		}
	})
	return catalog
}
