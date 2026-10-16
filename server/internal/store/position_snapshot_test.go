package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type positionRecordingTx struct {
	pgx.Tx  // All unexpected database operations fail this test without a real DB.
	calls   int
	sql     string
	args    []any
	err     error
	missing bool
}

func (q *positionRecordingTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.calls++
	q.sql = sql
	q.args = args
	if q.missing {
		return pgconn.NewCommandTag("UPDATE 0"), q.err
	}
	return pgconn.NewCommandTag("UPDATE 1"), q.err
}

func TestPositionSnapshotOnlyWritesCoordinates(t *testing.T) {
	q := &positionRecordingTx{}
	tx := &pgTx{tx: q}
	snap := domain.Snapshot{Parts: domain.SnapshotPosition, Char: &domain.Character{ID: 7, Pos: domain.Pos{MapID: 3, X: 40, Y: 50}, SceneInstance: 9}, Bag: domain.NewBag(3)}
	if err := tx.SaveSnapshot(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	if q.calls != 1 || q.sql != "UPDATE characters SET map_id=$2, pos_x=$3, pos_y=$4, scene_instance=$5 WHERE id=$1" || len(q.args) != 5 {
		t.Fatalf("unexpected persistence: %d %s %#v", q.calls, q.sql, q.args)
	}
	if strings.Contains(q.sql, "items") {
		t.Fatal("position save touched inventory")
	}
	q.err = errors.New("injected write failure")
	if err := tx.SaveSnapshot(context.Background(), snap); !errors.Is(err, q.err) {
		t.Fatalf("lost write failure: %v", err)
	}
	q.err = nil
	q.missing = true
	if err := tx.SaveSnapshot(context.Background(), snap); !errors.Is(err, domain.ErrCharNotFound) {
		t.Fatalf("missing character: %v", err)
	}
}

func TestPositionSnapshotPreservesInventoryAndBalances(t *testing.T) {
	m, snap := endgameFixture(t)
	ctx := context.Background()
	// A position snapshot is deliberately incomplete and must not zero assets.
	pos := domain.Snapshot{Parts: domain.SnapshotPosition, Char: &domain.Character{ID: snap.Char.ID, Pos: domain.Pos{MapID: 8, X: 12, Y: 13}, SceneInstance: 4}}
	if err := m.SaveSnapshot(ctx, pos); err != nil {
		t.Fatal(err)
	}
	c, err := m.CharByName(ctx, snap.Char.Name)
	if err != nil || c.Pos != pos.Char.Pos || c.SceneInstance != 4 || c.Caiyu != snap.Char.Caiyu || c.Level != snap.Char.Level {
		t.Fatalf("character corrupted: %+v %v", c, err)
	}
	bag, err := m.LoadBag(ctx, snap.Char.ID, 400)
	if err != nil || bag.At(0) != snap.Bag.At(0) {
		t.Fatalf("inventory changed: %v", err)
	}
	// Legacy snapshots remain full, including inventory removal and currency.
	snap.Bag.Set(0, domain.Stack{})
	snap.Char.Caiyu = 123
	if err := m.SaveSnapshot(ctx, snap); err != nil {
		t.Fatal(err)
	}
	bag, _ = m.LoadBag(ctx, snap.Char.ID, 400)
	c, _ = m.CharByName(ctx, snap.Char.Name)
	if !bag.At(0).Empty() || c.Caiyu != 123 {
		t.Fatal("full snapshot lost inventory or currency mutation")
	}
}
