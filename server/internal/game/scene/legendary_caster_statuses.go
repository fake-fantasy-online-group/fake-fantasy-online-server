package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func legendaryCasterSourceStatus(t *entity.Entity, id domain.StatusID, source domain.EntityID) (domain.Active, bool) {
	var found domain.Active
	ok := false
	if t != nil && t.Status != nil {
		t.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == id && a.Source == source {
				found = a
				ok = true
			}
		})
	}
	return found, ok
}
func init() {

}
