package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestV5StartupRejectsIncompleteSkillStatusRuntime(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	overlay := domain.StatusOverlay{domain.OverlayKey{Existing: domain.StatusStun, Incoming: domain.StatusStun}: domain.OverlayReplace}
	if err := domain.ValidateSkillStatusRuntimeV5(skills, statuses, overlay); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateSkillStatusRuntimeV5(skills, statuses, nil); err == nil {
		t.Fatal("empty overlay accepted")
	}
	if err := domain.ValidateSkillStatusRuntimeV5(nil, statuses, overlay); err == nil {
		t.Fatal("empty skills accepted")
	}
	if err := domain.ValidateSkillStatusRuntimeV5(skills, nil, overlay); err == nil {
		t.Fatal("empty statuses accepted")
	}
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		def, _ := skills.Get(id, 20)
		if len(def.Statuses) > 0 {
			a := def.Statuses[0]
			delete(statuses, domain.StatusKey{ID: a.ID, Level: a.Level})
			if err := domain.ValidateSkillStatusRuntimeV5(skills, statuses, overlay); err == nil {
				t.Fatal("missing referenced status accepted")
			}
			return
		}
	}
	t.Fatal("fixture missing status-bearing skill")
}
