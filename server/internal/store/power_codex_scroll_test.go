package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"reflect"
	"strings"
	"testing"
)

func TestPowerCodexScrollStorageValidation(t *testing.T) {
	good := domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
	if err := validateEndgameItem(good, nil); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.Stack){func(s *domain.Stack) { s.Bound = false }, func(s *domain.Stack) { s.Count = 2 }, func(s *domain.Stack) { s.Item = 1001 }, func(s *domain.Stack) { s.Endgame.PowerScrollRollBP = 123 }, func(s *domain.Stack) { s.Endgame.SourceLevel = 100 }, func(s *domain.Stack) { s.Endgame.PowerScrollID = "missing" }, func(s *domain.Stack) { s.Endgame.PowerScrollID = "" }} {
		bad := good
		mutate(&bad)
		if validateEndgameItem(bad, nil) == nil {
			t.Fatal("invalid scroll accepted", bad)
		}
	}
	m, snapshot := endgameFixture(t)
	snapshot.Bag.Set(6, good)
	if err := m.SaveSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	// Memory backend snapshot is the same aggregate consumed by receipt replay.
	saved, err := m.LoadBag(context.Background(), snapshot.Char.ID, snapshot.Bag.Cap())
	if err != nil || saved.At(6) != good {
		t.Fatal("scroll snapshot not durable", err)
	}
}

type powerScrollRows struct {
	pgx.Rows
	values []any
	read   bool
}

func (r *powerScrollRows) Next() bool {
	if r.read {
		return false
	}
	r.read = true
	return true
}
func (r *powerScrollRows) Scan(dest ...any) error {
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}
func (r *powerScrollRows) Close()     {}
func (r *powerScrollRows) Err() error { return nil }

type powerScrollQuerier struct {
	revisionRecordingQuerier
	rows  *powerScrollRows
	query string
}

func (q *powerScrollQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.query = sql
	return q.rows, nil
}
func TestPowerCodexScrollPostgresExactSaveLoadShape(t *testing.T) {
	st := domain.Stack{Item: domain.PowerScrollItemID, UID: 92, Count: 1, Bound: true, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
	q := &powerScrollQuerier{}
	if err := saveEndgameItem(context.Background(), q, st); err != nil {
		t.Fatal(err)
	}
	if len(q.args) != 10 || q.args[8] != "TY-A02" || q.args[9] != int32(8123) || !strings.Contains(q.statement, "power_scroll_id=EXCLUDED.power_scroll_id") || !strings.Contains(q.statement, "power_scroll_roll_bp=EXCLUDED.power_scroll_roll_bp") {
		t.Fatal("SQL lost snapshot fields", q.args)
	}
	q.rows = &powerScrollRows{values: q.args}
	cached := st
	cached.Endgame = domain.EndgameItemState{}
	items := map[int64]domain.Stack{92: cached}
	if err := loadEndgameItems(context.Background(), q, items); err != nil {
		t.Fatal(err)
	}
	if items[92] != st || !strings.Contains(q.query, "source_level,power_scroll_id,power_scroll_roll_bp FROM") {
		t.Fatal("SQL reload lost scroll", items[92])
	}
}
