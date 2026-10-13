package session

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

// Session integration uses the original skill rows with the explicit migrated
// progression snapshot, just as startup does. No implicit skill catalog remains.
func sessionSkillTableFixture(t *testing.T) domain.SkillTable {
	t.Helper()
	b, err := os.ReadFile("../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []domain.SkillDef
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 149*5 {
		t.Fatalf("original skill fixture rows = %d, want 745", len(rows))
	}
	skills := domain.SkillTable{}
	for _, def := range rows {
		key := domain.SkillKey{ID: def.ID, Level: def.Level}
		if _, exists := skills[key]; exists {
			t.Fatalf("duplicate original skill rank: %+v", key)
		}
		skills[key] = def
	}
	if err := domain.InstallV5SkillProgression(skills, catalogfixture.SkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	return skills
}
