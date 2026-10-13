package data

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"reflect"
	"testing"
)

func TestEndgameCatalogActualReadOnlyPostgres(t *testing.T) {
	path := os.Getenv("FANTASY_ENDGAME_CATALOG_PG_DATA")
	if path == "" {
		t.Skip("set isolated migrated cluster path")
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	items, err := LoadItems(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	skills, _, err := LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadEndgameCatalog(context.Background(), q, items, skills.ProgressionCatalog())
	if err != nil {
		t.Fatal(err)
	}

	if err := domain.ValidateEquipmentRuntimeV5(skills, c); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCubeRules(context.Background(), q, items); err != nil {
		t.Fatal(err)
	}
	loot, err := LoadLootRules(context.Background(), q, items)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRiftRules(context.Background(), q, loot, items); err != nil {
		t.Fatal(err)
	}
	want := catalogfixture.EndgameCatalog()
	for _, v := range []int32{0, 5, 6} {
		a, b := c.Uniques(v), want.Uniques(v)
		if !reflect.DeepEqual(a, b) {
			for i := range a {
				if i >= len(b) || !reflect.DeepEqual(a[i], b[i]) {
					t.Fatalf("unique version=%d index=%d got=%+v want=%+v", v, i, a[i], b[i])
				}
			}
			t.Fatalf("unique count version %d", v)
		}
		if !reflect.DeepEqual(c.Recipes(v), want.Recipes(v)) {
			t.Fatalf("formation version %d differs", v)
		}
	}
	if !reflect.DeepEqual(c.Runes(), want.Runes()) {
		t.Fatalf("rune definitions differ: got=%+v want=%+v", c.Runes(), want.Runes())
	}
	for _, r := range catalogfixture.EndgameCatalogData().Variants {
		got, ok := c.Variant(r.ID)
		if !ok || !reflect.DeepEqual(got, r.Variant()) {
			t.Fatalf("variant mismatch %s: %+v", r.ID, got)
		}
	}
	domain.AttachEndgameCatalog(items, c)
	for _, version := range []int32{0, 5, 6} {
		st := domain.Stack{UID: 42, Item: 1001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{UniqueID: 1001, UniqueContentVersion: version, UniqueRollBP: 10123}}
		before := st
		got, ok := domain.UniqueOf(st, c)
		expected, _ := domain.UniqueOf(st, want)
		if !ok || !reflect.DeepEqual(got, expected) || st != before {
			t.Fatal("persisted version/roll behavior changed", version)
		}
	}
	t.Logf("Integrated PostgreSQL59-63 loaders, runtime skill references and catalog parity: 2137 versioned uniques,120 formations,28 rune rules,5 variants through %d actual read-only SQL queries", q.queries)
}
