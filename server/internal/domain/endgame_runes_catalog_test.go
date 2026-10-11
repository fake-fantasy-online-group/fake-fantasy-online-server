package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The persisted machine-readable design must never drift from playable data.
func TestAstralJSONMatchesExecutableCatalog(t *testing.T) {
	var doc struct {
		SchemaVersion int
		Runes         []struct {
			ID                      ItemID
			MinLevel                int32
			Name, Mansion, Quadrant string
			Affixes                 []Affix
		}
		Formations []struct {
			ID                  int32
			Name, Channel, Lore string
			Runes               []ItemID
			MinLevel            int32
			Professions         uint8
			Slots               []EquipSlot
			Affixes             []Affix
		}
	}
	raw, err := os.ReadFile("../../docs/endgame/astral_runes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != 1 || len(doc.Runes) != 28 || len(doc.Formations) != 40 {
		t.Fatal("incomplete external catalog")
	}
	for i, r := range AstralRunes() {
		d := doc.Runes[i]
		if d.ID != r.Item || d.MinLevel != r.MinLevel || d.Name != r.Name || d.Mansion != r.Mansion || d.Quadrant != r.Quadrant || !reflect.DeepEqual(d.Affixes, r.Affixes) {
			t.Fatalf("rune row %d drift", i)
		}
	}
	for i, r := range AstralRecipes() {
		d := doc.Formations[i]
		if d.ID != r.ID || d.Name != r.Name || d.Channel != r.Channel.String() || d.MinLevel != r.MinLevel || d.Professions != r.Professions || d.Lore != r.Lore || !reflect.DeepEqual(d.Runes, r.Runes) || !reflect.DeepEqual(d.Slots, r.Slots) || !reflect.DeepEqual(d.Affixes, r.Affixes) {
			t.Fatalf("formation row %d drift", i)
		}
	}
}
