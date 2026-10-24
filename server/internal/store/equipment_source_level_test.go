package store

import (
	"context"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestEquipmentSourceLevelExactSaveAndMemoryReload(t *testing.T) {
	for _, legendary := range []bool{false, true} {
		st := legendaryStoreFixture()
		if !legendary {
			st.Legendary = domain.LegendaryInstance{}
		}
		st.Endgame.SourceLevel = 43
		q := &revisionRecordingQuerier{}
		if err := saveEndgameItem(context.Background(), q, st); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(q.statement, "source_level=EXCLUDED.source_level") || len(q.args) != 8 || q.args[7] != int32(43) {
			t.Fatal("source level lost during SQL save", q.statement, q.args)
		}
		m, s := endgameFixture(t)
		s.Bag.Set(0, st)
		if err := m.SaveSnapshot(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		bag, err := m.LoadBag(context.Background(), s.Char.ID, 400)
		if err != nil || bag.At(0) != st {
			t.Fatal("source level lost on reload", err)
		}
		if legendary {
			st.Endgame.SourceLevel = 1
			if validateEndgameItem(st, nil) == nil {
				t.Fatal("underlevel source accepted for power")
			}
		}
		st.Endgame.SourceLevel = -1
		if validateEndgameItem(st, nil) == nil {
			t.Fatal("negative provenance accepted")
		}
	}
}
