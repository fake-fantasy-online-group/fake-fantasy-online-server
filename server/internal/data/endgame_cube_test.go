package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"github.com/jackc/pgx/v5"
)

type cubeConfigRows struct {
	pgx.Rows
	records [][]any
	next    int
	closed  bool
	failure error
}

func (r *cubeConfigRows) Next() bool {
	if r.next >= len(r.records) {
		return false
	}
	r.next++
	return true
}
func (r *cubeConfigRows) Close()     { r.closed = true }
func (r *cubeConfigRows) Err() error { return r.failure }
func (r *cubeConfigRows) Scan(dst ...any) error {
	if r.next < 1 || r.next > len(r.records) || len(dst) != len(r.records[r.next-1]) {
		return fmt.Errorf("invalid cube row scan")
	}
	for i, v := range r.records[r.next-1] {
		target := reflect.ValueOf(dst[i]).Elem()
		value := reflect.ValueOf(v)
		if value.Type() != target.Type() {
			return fmt.Errorf("cube scan type %s != %s", value.Type(), target.Type())
		}
		target.Set(value)
	}
	return nil
}

type cubeConfigQuery struct {
	head, materials, outputs *cubeConfigRows
	failTable                string
}

func (q *cubeConfigQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "WHERE") {
		return nil, fmt.Errorf("configuration rows filtered before validation")
	}
	if q.failTable != "" && strings.Contains(sql, q.failTable) {
		return nil, errors.New("missing configuration table")
	}
	switch {
	case strings.Contains(sql, "FROM game_cube_recipes"):
		return q.head, nil
	case strings.Contains(sql, "FROM game_cube_materials"):
		return q.materials, nil
	case strings.Contains(sql, "FROM game_cube_outputs"):
		return q.outputs, nil
	}
	return nil, fmt.Errorf("unexpected cube query")
}
func cubeDataFixture(t *testing.T) (*cubeConfigQuery, map[domain.ItemID]domain.ItemDef, []domain.CubeRecipeDef) {
	t.Helper()
	var items []domain.ItemDef
	var defs []domain.CubeRecipeDef
	if err := testfixture.LoadEndgameItems(&items); err != nil {
		t.Fatal(err)
	}
	if err := testfixture.LoadCubeRules(&defs); err != nil {
		t.Fatal(err)
	}
	byID := map[domain.ItemID]domain.ItemDef{}
	for _, d := range items {
		byID[d.ID] = d
	}
	q := &cubeConfigQuery{head: &cubeConfigRows{}, materials: &cubeConfigRows{}, outputs: &cubeConfigRows{}}
	for _, d := range defs {
		q.head.records = append(q.head.records, []any{d.ID, d.Name, d.Operation, d.Description, d.TargetRequired, d.Money, d.SourceCount, d.QualityStepBP, int32(len(d.Materials)), int32(len(d.Outputs))})
		for i, m := range d.Materials {
			q.materials.records = append(q.materials.records, []any{d.ID, int32(i + 1), m.ItemID, m.Count})
		}
		for i, o := range d.Outputs {
			q.outputs.records = append(q.outputs.records, []any{d.ID, int32(i + 1), o.ItemID, o.Count, o.LevelDivisor, o.LevelCap, o.Multiplier})
		}
	}
	return q, byID, defs
}
func TestLoadCubeRulesAllSQLDefinitionsAndReferences(t *testing.T) {
	q, items, defs := cubeDataFixture(t)
	got, err := LoadCubeRules(context.Background(), q, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Recipes()) != 66 || !q.head.closed || !q.materials.closed || !q.outputs.closed {
		t.Fatal("incomplete or leaked configuration")
	}
	for _, want := range defs {
		r, ok := got.Recipe(want.ID)
		if !ok {
			t.Fatal("missing", want.ID)
		}
		if len(want.Materials) == 0 {
			want.Materials = nil
		}
		if len(want.Outputs) == 0 {
			want.Outputs = nil
		}
		if !reflect.DeepEqual(r, want) {
			t.Fatalf("changed recipe%d: got%+v want%+v", want.ID, r, want)
		}
	}
}
func TestLoadCubeRulesFailsClosedOnCorruptSQL(t *testing.T) {
	cases := map[string]func(*cubeConfigQuery, map[domain.ItemID]domain.ItemDef){
		"missing table":   func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.failTable = "game_cube_recipes" },
		"empty catalogue": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.head.records = nil },
		"missing recipe":  func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.head.records = q.head.records[1:] },
		"missing last rune": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) {
			q.head.records = q.head.records[:len(q.head.records)-1]
		},
		"unknown operation": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.head.records[0][2] = "eval" },
		"dangling material": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) {
			q.materials.records[0][2] = domain.ItemID(1999999999)
		},
		"unresolved output": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) {
			q.outputs.records[0][2] = domain.ItemID(1999999999)
		},
		"duplicate recipe": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) {
			q.head.records = append(q.head.records, q.head.records[0])
		},
		"missing ingredient rows": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.materials.records = nil },
		"missing output rows":     func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.outputs.records = nil },
		"bad ordering":            func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) { q.materials.records[0][1] = int32(2) },
		"query error": func(q *cubeConfigQuery, _ map[domain.ItemID]domain.ItemDef) {
			q.outputs.failure = errors.New("read interrupted")
		},
		"invalid item semantics": func(_ *cubeConfigQuery, items map[domain.ItemID]domain.ItemDef) {
			d := items[domain.CubeStarMarrow]
			d.Stackable = false
			items[d.ID] = d
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			q, items, _ := cubeDataFixture(t)
			change(q, items)
			if _, err := LoadCubeRules(context.Background(), q, items); err == nil {
				t.Fatal("invalid SQL configuration accepted")
			}
		})
	}
}

// Explicit, serial, read-only validation against an idle disposable PG cluster.
// This never creates a cluster or touches an application database.
func TestCubeActualPostgresLoader(t *testing.T) {
	path := os.Getenv("FANTASY_CUBE_PG_DATA")
	if path == "" {
		t.Skip("FANTASY_CUBE_PG_DATA not set")
	}
	bin := os.Getenv("PG_BIN")
	if bin == "" {
		t.Fatal("PG_BIN required")
	}
	database := os.Getenv("FANTASY_CUBE_PG_DATABASE")
	if database == "" {
		database = "postgres"
	}
	q := &v5SingleUserQuerier{bin: bin, path: path, database: database}
	items, err := LoadItems(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := LoadCubeRules(context.Background(), q, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Recipes()) != 66 {
		t.Fatalf("got %d recipes", len(rules.Recipes()))
	}
	var seed []domain.CubeRecipeDef
	if err := testfixture.LoadCubeRules(&seed); err != nil {
		t.Fatal(err)
	}
	for _, want := range seed {
		got, ok := rules.Recipe(want.ID)
		if len(want.Materials) == 0 {
			want.Materials = nil
		}
		if len(want.Outputs) == 0 {
			want.Outputs = nil
		}
		if !ok || !reflect.DeepEqual(want, got) {
			t.Fatalf("SQL recipe %d differs from migration", want.ID)
		}
	}
	t.Logf("Actual LoadItems -> LoadCubeRules: %d items, 66 recipes; all definitions exactly match SQL61", len(items))
}
