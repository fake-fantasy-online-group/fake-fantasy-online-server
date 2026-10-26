package data

import (
	"encoding/json"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

func TestRuneStackMigrationCoversExactlyAuthoredRunes(t *testing.T) {
	raw, err := testfixture.ReadMigrationJSON("000000_init-113.sql", "$rune_stack_ids$")
	if err != nil {
		t.Fatal(err)
	}
	var ids []domain.ItemID
	if err = json.Unmarshal(raw, &ids); err != nil {
		t.Fatal(err)
	}
	if len(ids) != int(domain.LastAstralRune-domain.FirstAstralRune+1) {
		t.Fatalf("wrong rune count: %d", len(ids))
	}
	seen := map[domain.ItemID]bool{}
	for _, id := range ids {
		if id < domain.FirstAstralRune || id > domain.LastAstralRune || seen[id] {
			t.Fatalf("invalid or duplicate rune %d", id)
		}
		seen[id] = true
	}
	items := endgameDefinitionFixture(t)
	for _, id := range ids {
		def := items[id]
		st := domain.NewStack(def, 37)
		if !domain.IsAstralRuneStack(st) || st.Card != def.CardDefaults {
			t.Fatalf("rune %d not stackable with exact card: %+v", id, st)
		}
	}
}
