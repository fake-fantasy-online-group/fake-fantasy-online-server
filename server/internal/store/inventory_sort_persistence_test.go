package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type sortedBagRecordingTx struct {
	pgx.Tx
	deleted bool
	rows    [][]any
	facts   map[int64]bool
	cards   map[int64][]any
}

func (q *sortedBagRecordingTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "INSERT INTO item_instances"):
		if q.facts == nil {
			q.facts = map[int64]bool{}
		}
		q.facts[args[0].(int64)] = true
	case strings.Contains(sql, "INSERT INTO card_instances"):
		if q.cards == nil {
			q.cards = map[int64][]any{}
		}
		q.cards[args[0].(int64)] = args
	case strings.Contains(sql, "DELETE FROM character_items"):
		q.deleted = true
		q.rows = nil
	default:
		return pgconn.CommandTag{}, fmt.Errorf("unexpected SQL: %s", sql)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (q *sortedBagRecordingTx) CopyFrom(_ context.Context, table pgx.Identifier, _ []string, src pgx.CopyFromSource) (int64, error) {
	if !q.deleted || len(table) != 1 || table[0] != "character_items" {
		return 0, fmt.Errorf("not a full bag replacement")
	}
	for src.Next() {
		values, err := src.Values()
		if err != nil {
			return 0, err
		}
		q.rows = append(q.rows, values)
	}
	return int64(len(q.rows)), src.Err()
}

func TestSortedRuneBagPersistenceReplacesPositionsAndKeepsSurvivor(t *testing.T) {
	// After 2+1 merge, only the surviving UID owns three units. The historical
	// item/card fact for the consumed UID need not be deleted: loadBag starts from
	// character_items, not all item_instances, so unowned facts cannot respawn.
	st := persistedRuneFixture()
	st.Count = 3
	bag := domain.NewBag(domain.DefaultBagSlots)
	bag.Set(100, st)
	q := &sortedBagRecordingTx{rows: [][]any{{int64(7), 100, int64(123)}, {int64(7), 101, int64(456)}}}
	if err := saveBag(context.Background(), q, 7, bag); err != nil {
		t.Fatal(err)
	}
	if len(q.rows) != 1 || q.rows[0][2] != st.UID || q.rows[0][4] != int32(3) || q.facts[456] {
		t.Fatal("retired stack location retained or rewritten")
	}
	card := q.cards[st.UID]
	if len(card) != 9 || card[7] != true || card[8] != st.Card.Locked || card[6].([]int32)[0] != st.Card.Effects[0].Value {
		t.Fatal("survivor card protection/effect lost")
	}
	fact := st
	fact.Count = 1
	restored, err := hydrateStack(domain.Stack{UID: st.UID, Item: st.Item, Count: q.rows[0][4].(int32)}, map[int64]domain.Stack{st.UID: fact})
	if err != nil || restored != st {
		t.Fatalf("persisted count and identity do not hydrate: %v", err)
	}
}
