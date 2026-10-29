package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"reflect"
	"strings"
	"testing"
)

type questAccessRows struct {
	pgx.Rows
	values [][]any
	index  int
}

func (r *questAccessRows) Next() bool { r.index++; return r.index <= len(r.values) }
func (r *questAccessRows) Scan(dest ...any) error {
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(r.values[r.index-1][i]))
	}
	return nil
}
func (r *questAccessRows) Close()     {}
func (r *questAccessRows) Err() error { return nil }

type questAccessQuery struct {
	revisionRecordingQuerier
	rows    *questAccessRows
	queries []string
}

func (q *questAccessQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	q.queries = append(q.queries, sql)
	if strings.Contains(sql, "FROM character_quests ") {
		return q.rows, nil
	}
	return &questAccessRows{}, nil
}

func TestQuestDungeonFinishedHistoryLoadsThroughPostgresQuery(t *testing.T) {
	for _, state := range []domain.QuestState{domain.QuestActive, domain.QuestDeliver, domain.QuestFinished} {
		rows := &questAccessRows{}
		for _, qid := range []int32{355, 356, 359, 361} {
			rows.values = append(rows.values, []any{qid, int32(state), int32(0), int64(123), "", []byte("{}")})
		}
		q := &questAccessQuery{rows: rows}
		log, err := loadQuests(context.Background(), q, 17)
		if err != nil || len(log) != 4 {
			t.Fatal("load lost states", log, err)
		}
		// Actual production query must fetch completed history too, without a state predicate.
		if len(q.queries) != 2 || !strings.Contains(q.queries[0], "WHERE char_id = $1") || strings.Contains(q.queries[0], "state <") || strings.Contains(q.queries[0], "state =") {
			t.Fatal("query filtered history", q.queries)
		}
		for _, pair := range [][2]int32{{29400, 356}, {29900, 359}, {30000, 361}} {
			if log[domain.QuestID(pair[1])].State != state || domain.CanEnterQuestDungeon(pair[0], log) != (state != domain.QuestFinished) {
				t.Fatal("reloaded admission differs", pair, state)
			}
		}
		if state == domain.QuestFinished && !reflect.DeepEqual(log.FinishedIDs(), []int32{355, 356, 359, 361}) {
			t.Fatal("done history missing from existing native log", log.FinishedIDs())
		}
	}
}
func TestQuestDungeonFinishedSnapshotSurvivesReload(t *testing.T) {
	m, s := endgameFixture(t)
	s.Char.Quests = domain.QuestLog{355: {State: domain.QuestFinished}, 356: {State: domain.QuestDeliver}, 359: {State: domain.QuestActive}, 361: {State: domain.QuestFinished}}
	if err := m.SaveSnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	s.Char.Quests = domain.QuestLog{}
	log, err := m.LoadQuests(context.Background(), s.Char.ID)
	if err != nil || !domain.CanEnterQuestDungeon(29200, log) || !domain.CanEnterQuestDungeon(29400, log) || !domain.CanEnterQuestDungeon(29900, log) || domain.CanEnterQuestDungeon(30000, log) {
		t.Fatal("reload changed unfinished/completed admission", log, err)
	}
}
