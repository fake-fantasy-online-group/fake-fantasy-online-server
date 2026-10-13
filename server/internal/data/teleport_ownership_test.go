package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

type portalOwnershipQuery struct{ rows *v5SingleUserRows }

func (q portalOwnershipQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "t.transition_owner") || !strings.Contains(sql, "ORDER BY src.id, t.proc_id") {
		return nil, fmt.Errorf("portal query omitted explicit ownership/stable order")
	}
	return q.rows, nil
}

func TestLoadTeleportsExplicitOwnership(t *testing.T) {
	rows := &v5SingleUserRows{at: -1}
	add := func(src, proc, target int32, script, owner string) {
		var row []json.RawMessage
		for _, v := range []any{src, proc, target, int32(100), int32(200), int32(67),
			int32(10), int32(20), int32(67), []int32{0, 0, 100, 0, 100, 100}, script, true, owner} {
			b, _ := json.Marshal(v)
			row = append(row, b)
		}
		rows.values = append(rows.values, row)
	}
	add(919, 1, 58, "", "client")
	add(919, 2, 920, "", "server")
	add(919, 3, 920, "", "unknown")
	add(919, 4, 920, "command-gate", "client")
	add(507, 2, 0, "Trap20502", "client")
	dungeons := domain.DungeonTable{20502: {Enter: domain.Pos{MapID: 20502, X: 500, Y: 600}}}
	table, st, err := LoadTeleports(context.Background(), portalOwnershipQuery{rows}, dungeons)
	if err != nil || st.Loaded != 5 || st.Dungeon != 1 {
		t.Fatalf("load stats=%+v err=%v", st, err)
	}
	for i, p := range table[919] {
		if p.ClientOwned != (i == 0) {
			t.Fatalf("proc%d ownership=%v", p.ProcID, p.ClientOwned)
		}
	}
	if p := table[507][0]; p.ClientOwned || !p.Dungeon || p.To.MapID != 20502 {
		t.Fatalf("script gate bypassed server admission: %+v", p)
	}
}

// Uses an idle, disposable migrated cluster; never opens a listening socket.
func TestTeleportOwnershipActualReadOnlyPostgres(t *testing.T) {
	path := os.Getenv("FANTASY_PORTAL_OWNER_PG_DATA")
	if path == "" {
		t.Skip("set an isolated migration-65 cluster path")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required for explicit single-user validation")
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	dungeons, err := LoadDungeons(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	table, st, err := LoadTeleports(context.Background(), q, dungeons)
	if err != nil {
		t.Fatal(err)
	}
	client, server, same := 0, 0, 0
	for source, portals := range table {
		for _, p := range portals {
			if p.ClientOwned {
				client++
				if source == p.To.MapID {
					same++
				}
			} else {
				server++
			}
		}
	}
	if client != 457 || server != 6 || same != 26 || st.Dungeon != 1 {
		t.Fatalf("client=%d server=%d same=%d stats=%+v", client, server, same, st)
	}
	for _, source := range []int32{21100, 21200, 21300, 21400, 21500} {
		found := false
		for _, p := range table[source] {
			if p.ProcID == 90001 {
				found = true
				if p.ClientOwned || p.To.MapID != 7 {
					t.Fatalf("server-added exit changed: %+v", p)
				}
			}
		}
		if !found {
			t.Fatalf("missing server-added exit on %d", source)
		}
	}
	t.Logf("PASS actual LoadTeleports:457 imported client-owned,26 same-map,5 server exits+Trap20502 server-owned; joined total=%d", st.Total)
}
