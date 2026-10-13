package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

type riftPortalAreaQuery struct{ rows *v5SingleUserRows }

func (q riftPortalAreaQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "map_to") || strings.Contains(sql, "dst.") || strings.Contains(sql, "t.script") ||
		!strings.Contains(sql, "JOIN map_defs src") || !strings.Contains(sql, "ORDER BY src.id, t.proc_id") {
		return nil, fmt.Errorf("rift geometry query must not depend on public destination or script")
	}
	return q.rows, nil
}

func TestRiftPortalAreasDoNotRequirePublicDestination(t *testing.T) {
	polygon := []int32{100, 200, 140, 200, 140, 240, 100, 240}
	rows := &v5SingleUserRows{at: -1}
	for _, points := range [][]int32{polygon, nil, {100, 200, 100, 300}, {100, 200, 100, 300, 100, 300, 100}} {
		a, _ := json.Marshal(int32(346))
		b, _ := json.Marshal(points)
		rows.values = append(rows.values, []json.RawMessage{a, b})
	}
	areas, err := LoadRiftPortalAreas(context.Background(), riftPortalAreaQuery{rows})
	if err != nil || len(areas[346]) != 1 || len(areas[346][0]) != 4 || !areas[346][0].Contains(domain.Pos{MapID: 346, X: 120, Y: 220}) {
		t.Fatalf("source geometry missing or malformed polygons admitted: %+v %v", areas, err)
	}
	// The corresponding ordinary gate still has no valid destination and must
	// not become usable in a public scene merely because rifts need its geometry.
	ordinary := &v5SingleUserRows{at: -1}
	var row []json.RawMessage
	for _, v := range []any{int32(346), int32(1), int32(351), int32(0), int32(0), int32(0), int32(0), int32(0), int32(0), polygon, "", false, "client"} {
		b, _ := json.Marshal(v)
		row = append(row, b)
	}
	ordinary.values = append(ordinary.values, row)
	portals, stats, err := LoadTeleports(context.Background(), portalOwnershipQuery{ordinary}, nil)
	if err != nil || stats.NoDest != 1 || len(portals) != 0 {
		t.Fatalf("ordinary missing-destination admission changed: %+v %+v %v", portals, stats, err)
	}
}

func TestRiftPortalAreasActualReadOnlyPostgres(t *testing.T) {
	path := os.Getenv("FANTASY_RIFT_PORTAL_PG_DATA")
	if path == "" {
		t.Skip("set an idle disposable migrated PostgreSQL cluster")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("cluster is not idle")
	}
	database := os.Getenv("FANTASY_RIFT_PORTAL_PG_DATABASE")
	if database == "" {
		database = "postgres"
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: database}
	areas, err := LoadRiftPortalAreas(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	dungeons, err := LoadDungeons(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, stats, err := LoadTeleports(context.Background(), q, dungeons)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int32{346, 347, 349, 350, 961} {
		if len(areas[id]) <= len(ordinary[id]) {
			t.Fatalf("map %d missing authored native geometry: rift=%d ordinary=%d", id, len(areas[id]), len(ordinary[id]))
		}
	}
	t.Logf("PASS actual read-only native gate geometry: ordinary=%d missing destination=%d; rift source maps=%d", stats.Loaded, stats.NoDest, len(areas))
}
