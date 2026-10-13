package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestEndgameCatalogFrozenVersionsAndCopyIsolation(t *testing.T) {
	in := testEndgameCatalogData()
	c, err := NewEndgameCatalog(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int32{0, 5, 6} {
		if len(c.Uniques(version)) == 0 || len(c.Recipes(version)) == 0 {
			t.Fatal("missing historical version", version)
		}
	}
	before := c.Uniques(in.Uniques[0].Version)
	in.Uniques[0].Definition.Affixes[0].Value++
	if !reflect.DeepEqual(before, c.Uniques(in.Uniques[0].Version)) {
		t.Fatal("constructor retained caller slices")
	}
	u := c.Uniques(6)
	u[0].Affixes[0].Value++
	if reflect.DeepEqual(u, c.Uniques(6)) {
		t.Fatal("unique read exposed mutable slices")
	}
	r := c.Recipes(0)
	r[0].Runes[0]++
	if reflect.DeepEqual(r, c.Recipes(0)) {
		t.Fatal("historical formation read exposed mutable slices")
	}
	rs := c.Runes()
	rs[0].Affixes[0].Value++
	if reflect.DeepEqual(rs, c.Runes()) {
		t.Fatal("rune read exposed mutable slices")
	}
	if _, ok := UniqueForBase(1001, nil); ok {
		t.Fatal("missing dependency used implicit fixture")
	}
}
func TestEndgameCatalogRejectsIncompleteAndInvalidHistoricalData(t *testing.T) {
	cases := map[string]func(*EndgameCatalogData){
		"missing skills":    func(d *EndgameCatalogData) { d.Skills = nil },
		"missing manifest":  func(d *EndgameCatalogData) { d.Manifest = nil },
		"missing unique":    func(d *EndgameCatalogData) { d.Uniques = d.Uniques[1:] },
		"duplicate variant": func(d *EndgameCatalogData) { d.Variants = append(d.Variants, d.Variants[0]) },
		"unknown variant":   func(d *EndgameCatalogData) { d.Variants[0].Behavior = "arbitrary_script" },
		"legacy channel": func(d *EndgameCatalogData) {
			for i := range d.Formations {
				if d.Formations[i].Version == 0 {
					d.Formations[i].Definition.Channel = 4
					break
				}
			}
		},
		"v5 channel": func(d *EndgameCatalogData) {
			for i := range d.Formations {
				if d.Formations[i].Version == 5 {
					d.Formations[i].Definition.Channel = 4
					break
				}
			}
		},
		"legacy slot":         func(d *EndgameCatalogData) { d.Formations[0].Definition.Slots = []EquipSlot{26} },
		"legacy unknown rune": func(d *EndgameCatalogData) { d.Formations[0].Definition.Runes[0] = 123 },
		"legacy unsupported effect": func(d *EndgameCatalogData) {
			d.Formations[0].Definition.TypedEffects = []EquipmentEffect{{Kind: "arbitrary_script"}}
		},
		"unrepresentable string":   func(d *EndgameCatalogData) { d.Uniques[0].Definition.Lore = strings.Repeat("😀", 4097) },
		"bad regeneration cadence": func(d *EndgameCatalogData) { d.Runes[len(d.Runes)-1].RegenerationIntervalMS = 1550 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := testEndgameCatalogData()
			mutate(&in)
			if _, err := NewEndgameCatalog(in); err == nil {
				t.Fatal("invalid configured catalog accepted")
			}
		})
	}
}
func TestEndgameCatalogDataOnlyGrowthAndLegacyRequirement(t *testing.T) {
	in := testEndgameCatalogData()
	v := in.Variants[0]
	v.ID = "another_configured_preset"
	in.Variants = append(in.Variants, v)
	for i := range in.Manifest {
		if in.Manifest[i].Kind == "variant" {
			in.Manifest[i].Count++
		}
	}
	// Legacy data has frozen requirements; changing today's rune base requirement
	// must not force the old content's authored level to equal the current value.
	for i := range in.Formations {
		if in.Formations[i].Version == 0 {
			in.Formations[i].Definition.MinLevel = 1
			break
		}
	}
	c, err := NewEndgameCatalog(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Variant(v.ID); !ok {
		t.Fatal("data-only extra preset missing")
	}
}
func TestEndgameCatalogPresentationRejectsOversizedHeader(t *testing.T) {
	in := testEndgameCatalogData()
	c, err := NewEndgameCatalog(in)
	if err != nil {
		t.Fatal(err)
	}
	items := map[ItemID]ItemDef{}
	for _, d := range endgameItemsFixture() {
		items[d.ID] = d
	}
	if err = ValidateEndgameCatalogPresentation(items, c); err != nil {
		t.Fatal(err)
	}
	for id, d := range items {
		d.Name = strings.Repeat("界", 8192)
		items[id] = d
	}
	if err = ValidateEndgameCatalogPresentation(items, c); err == nil {
		t.Fatal("oversized fixed snapshot header accepted")
	}
}
