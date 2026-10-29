package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Regression of independently reproduced int32->uint8 aliasing. Check the real
// loader and both admission and the existing native receipt, including Done.
func TestQuestDungeonRawSavedStateCannotAliasAdmissionOrFinishedHistory(t *testing.T) {
	for _, pair := range [][2]int32{{29200, 355}, {29400, 356}, {29900, 359}, {30000, 361}} {
		for _, raw := range []int32{0, 1, 2, 3, 255, 256, 257, 258, -256, -255, -254} {
			t.Run(fmt.Sprintf("map%d-raw%d", pair[0], raw), func(t *testing.T) {
				q := &questAccessQuery{rows: &questAccessRows{values: [][]any{{pair[1], raw, int32(0), int64(0), "", []byte("{}")}}}}
				log, err := loadQuests(context.Background(), q, 17)
				if err != nil {
					t.Fatal(err)
				}
				id := domain.QuestID(pair[1])
				want := domain.QuestState(3)
				if raw >= 0 && raw <= 2 {
					want = domain.QuestState(raw)
				}
				if log[id].State != want {
					t.Fatal("invalid narrowing or valid history changed", raw, log[id].State)
				}
				allowed := raw == 0 || raw == 1
				rows := domain.ActiveQuestsOf(domain.QuestTable{id: {ID: id}}, &domain.Character{Quests: log}, nil)
				if domain.CanEnterQuestDungeon(pair[0], log) != allowed || (len(rows) == 1) != allowed {
					t.Fatal("loader/receipt/admission disagree", raw, log, rows)
				}
				if (len(log.FinishedIDs()) == 1) != (raw == 2) {
					t.Fatal("invalid state became completed history", raw, log.FinishedIDs())
				}
			})
		}
	}
}

func TestQuestDungeonRawStateFixPreservesOtherTaskLoading(t *testing.T) {
	for _, raw := range []int32{0, 1, 2, 3, 256, 257, -256, -255} {
		q := &questAccessQuery{rows: &questAccessRows{values: [][]any{{int32(999), raw, int32(0), int64(0), "", []byte("{}")}}}}
		log, err := loadQuests(context.Background(), q, 17)
		if err != nil || log[999].State != domain.QuestState(raw) {
			t.Fatal("unrelated task loading changed", raw, log, err)
		}
	}
}
